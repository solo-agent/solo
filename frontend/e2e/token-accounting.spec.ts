import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const suffix = Date.now().toString(36);

interface Auth { access_token: string }
interface Entity { id: string; name: string }
interface RunUsage {
  run_id: string;
  status: string;
  state: string;
  actual_tokens: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

async function api<T>(request: APIRequestContext, token: string, method: 'post' | 'delete', path: string, data?: unknown): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, { headers: { authorization: `Bearer ${token}` }, data });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

function databaseJSON<T>(query: string): T {
  return JSON.parse(execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim()) as T;
}

function latestUsage(agentID: string, trigger?: string): RunUsage {
  const triggerFilter = trigger
    ? `AND r.trigger_message_id=(SELECT id FROM messages WHERE content='${trigger}' ORDER BY created_at DESC LIMIT 1)`
    : '';
  return databaseJSON<RunUsage>(`
    SELECT COALESCE((SELECT json_build_object(
      'run_id',r.id::text,'status',r.status,'state',COALESCE(u.state,''),
      'actual_tokens',COALESCE(u.actual_tokens,0),'input_tokens',COALESCE(u.input_tokens,0),
      'output_tokens',COALESCE(u.output_tokens,0),'cache_read_tokens',COALESCE(u.cache_read_tokens,0),
      'cache_write_tokens',COALESCE(u.cache_write_tokens,0))::text
      FROM agent_runs r LEFT JOIN agent_run_token_usage u ON u.run_id=r.id
      WHERE r.agent_id='${agentID}' ${triggerFilter}
      ORDER BY r.started_at DESC LIMIT 1),
      '{"run_id":"","status":"","state":"","actual_tokens":0,"input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":0}')
  `);
}

function daemonReportedUsage(runID: string): Pick<RunUsage, 'input_tokens' | 'output_tokens' | 'cache_read_tokens' | 'cache_write_tokens'> | undefined {
  for (const line of readFileSync('../daemon.log', 'utf8').trim().split('\n').reverse()) {
    try {
      const entry = JSON.parse(line) as Record<string, unknown>;
      if (entry.msg === 'task backend completed' && entry.task_id === runID) {
        return {
          input_tokens: Number(entry.input_tokens ?? 0),
          output_tokens: Number(entry.output_tokens ?? 0),
          cache_read_tokens: Number(entry.cache_read_tokens ?? 0),
          cache_write_tokens: Number(entry.cache_write_tokens ?? 0),
        };
      }
    } catch { /* ignore process output */ }
  }
  return undefined;
}

function usageDiagnostics(runID: string): unknown {
  return databaseJSON(`
    SELECT json_build_object(
      'run_usage', (SELECT usage_json FROM agent_runs WHERE id='${runID}'),
      'delivery_events', COALESCE((
        SELECT json_agg(json_build_object('source_seq',source_seq,'event',event,'data',data) ORDER BY source_seq)
          FROM agent_run_delivery_events WHERE run_id='${runID}'
      ), '[]'::json),
      'run_events', COALESCE((
        SELECT json_agg(json_build_object('seq',seq,'type',type,'payload',payload) ORDER BY seq)
          FROM agent_run_events WHERE run_id='${runID}' AND type IN ('usage','done','error')
      ), '[]'::json)
    )::text
  `);
}

function expectDaemonUsage(run: RunUsage) {
  const daemon = daemonReportedUsage(run.run_id);
  expect(daemon).toBeDefined();
  expect(run, JSON.stringify(usageDiagnostics(run.run_id))).toMatchObject(daemon!);
  expect(Object.values(daemon!).reduce((total, value) => total + value, 0)).toBeGreaterThan(0);
}

test.describe('remote Token accounting', () => {
  test.skip(process.env.SOLO_E2E_REAL_TOKEN_ACCOUNTING !== '1', 'requires real local Claude and Codex runtimes');
  test.setTimeout(600_000);

  test('settles current-turn usage from both Daemon-owned runtimes', async ({ request }) => {
    const registration = await registerVerified(request, apiBase, {
      data: { email: `token-accounting-${suffix}@solo.local`, password: 'SoloE2E-2026!', display_name: 'Token Accounting E2E' },
    });
    expect(registration.ok()).toBeTruthy();
    const auth = await registration.json() as Auth;
    let computer: LocalComputerLease | undefined;
    let channel: Entity | undefined;
    const agents: Entity[] = [];
    try {
      computer = await acquireLocalComputer(request, apiBase, auth.access_token);
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', { name: `token-accounting-${suffix}` });

      for (const provider of ['claude', 'codex'] as const) {
        const agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
          name: `${provider}-token-${suffix}`,
          computer_id: computer.id,
          model_provider: provider,
          model_name: provider === 'claude' ? 'sonnet' : '',
          system_prompt: `Always reply using solo message send --target '#${channel.name}'. When introducing yourself, send exactly ${provider.toUpperCase()}_TOKEN_READY_${suffix}. For later messages send exactly the requested TOKEN payload.`,
        });
        agents.push(agent);
        await expect.poll(() => latestUsage(agent.id), { timeout: 180_000, intervals: [500, 1000, 2000] }).toMatchObject({
          status: 'completed', state: 'settled', actual_tokens: expect.any(Number),
        });
        const greeting = latestUsage(agent.id);
        expect(greeting.actual_tokens).toBeGreaterThan(0);
        expectDaemonUsage(greeting);

        const firstTrigger = `TOKEN_FIRST_${provider}_${suffix}`;
        const firstContent = `@${agent.name} ${firstTrigger}`;
        await api(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/messages`, { content: firstContent });
        await expect.poll(() => latestUsage(agent.id, firstContent), { timeout: 180_000, intervals: [500, 1000, 2000] }).toMatchObject({
          status: 'completed', state: 'settled', actual_tokens: expect.any(Number),
        });
        const first = latestUsage(agent.id, firstContent);
        expect(first.actual_tokens).toBeGreaterThan(0);
        expectDaemonUsage(first);

        const secondTrigger = `TOKEN_SECOND_${provider}_${suffix}`;
        const secondContent = `@${agent.name} ${secondTrigger}`;
        await api(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/messages`, { content: secondContent });
        await expect.poll(() => latestUsage(agent.id, secondContent), { timeout: 180_000, intervals: [500, 1000, 2000] }).toMatchObject({
          status: 'completed', state: 'settled', actual_tokens: expect.any(Number),
        });
        const second = latestUsage(agent.id, secondContent);
        expect(second.actual_tokens).toBeGreaterThan(0);
        expectDaemonUsage(second);
      }
    } finally {
      for (const agent of agents) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
      await computer?.release(request).catch(() => undefined);
    }
  });
});
