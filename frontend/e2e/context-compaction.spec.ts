import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const suffix = Date.now().toString(36);

interface Auth { access_token: string; refresh_token: string }
interface Entity { id: string; name: string }
interface TaskEntity { id: string; title: string }
interface RunState {
  run_id: string;
  status: string;
  session_id: string;
  session_status: string;
  rollover_from_session_id: string;
  rollover_completed_events: number;
  task_lookup_events: number;
  message_id: string;
  message_content: string;
}
interface ContextState {
  completed_runs: number;
  snapshot_events: number;
  compaction_events: number;
  rollover_requested_events: number;
  rollover_completed_events: number;
  session_count: number;
  active_sessions: number;
  pending_sessions: number;
  closed_sessions: number;
  unbound_context_runs: number;
  latest_compaction_run_id: string;
  latest_compaction_session_id: string;
  latest_compaction_status: string;
  latest_compaction_message_id: string;
  latest_compaction_payload: Record<string, unknown>;
  latest_snapshot_payload: Record<string, unknown>;
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
  ], { encoding: 'utf8', timeout: 10_000 }).trim()) as T;
}

function seedContinuityTask(channelID: string, creatorEmail: string, agentID: string, title: string): TaskEntity {
  return databaseJSON<TaskEntity>(`
    WITH inserted AS (
      INSERT INTO tasks (id, task_number, channel_id, creator_id, title, status, claimer_id, priority)
      SELECT gen_random_uuid(),
             (SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = '${channelID}'),
             '${channelID}', id, '${title}', 'in_progress', '${agentID}', 'medium'
        FROM users WHERE email = '${creatorEmail}'
      RETURNING id, title
    )
    SELECT json_build_object('id', id::text, 'title', title)::text FROM inserted
  `);
}

function runState(triggerMessageID: string): RunState {
  return databaseJSON<RunState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'status', r.status,
        'session_id', COALESCE(r.session_id::text, ''),
        'session_status', COALESCE(s.status, ''),
        'rollover_from_session_id', COALESCE(r.rollover_from_session_id::text, ''),
        'rollover_completed_events', (SELECT count(*) FROM agent_run_events e WHERE e.run_id = r.id AND e.type = 'session_rollover_completed'),
        'task_lookup_events', (
          SELECT count(*)
            FROM agent_run_events e
           WHERE e.run_id = r.id
             AND e.type = 'tool_started'
             AND COALESCE(e.payload->>'input', '') ~* 'solo[[:space:]]+task'
        ),
        'message_id', COALESCE(reply.id::text, ''),
        'message_content', COALESCE(reply.content, '')
      )::text
        FROM agent_runs r
        LEFT JOIN agent_sessions s ON s.id = r.session_id
        LEFT JOIN LATERAL (
          SELECT id, content FROM messages
           WHERE metadata->>'agent_run_id' = r.id::text
           ORDER BY created_at LIMIT 1
        ) reply ON true
       WHERE r.trigger_message_id = '${triggerMessageID}'
       ORDER BY r.started_at DESC LIMIT 1
    ), '{"run_id":"","status":"","session_id":"","session_status":"","rollover_from_session_id":"","rollover_completed_events":0,"task_lookup_events":0,"message_id":"","message_content":""}')
  `);
}

function contextState(agentID: string): ContextState {
  return databaseJSON<ContextState>(`
    WITH context_events AS (
      SELECT e.*, r.status AS run_status, r.session_id
        FROM agent_run_events e
        JOIN agent_runs r ON r.id = e.run_id
       WHERE r.agent_id = '${agentID}'
         AND e.type IN ('context_snapshot', 'context_compaction', 'session_rollover_requested', 'session_rollover_completed')
    ), latest_compaction AS (
      SELECT * FROM context_events WHERE type = 'context_compaction' ORDER BY created_at DESC, seq DESC LIMIT 1
    ), latest_snapshot AS (
      SELECT * FROM context_events WHERE type = 'context_snapshot' ORDER BY created_at DESC, seq DESC LIMIT 1
    )
    SELECT json_build_object(
      'completed_runs', (SELECT count(*) FROM agent_runs WHERE agent_id = '${agentID}' AND status = 'completed'),
      'snapshot_events', (SELECT count(*) FROM context_events WHERE type = 'context_snapshot'),
      'compaction_events', (SELECT count(*) FROM context_events WHERE type = 'context_compaction'),
      'rollover_requested_events', (SELECT count(*) FROM context_events WHERE type = 'session_rollover_requested'),
      'rollover_completed_events', (SELECT count(*) FROM context_events WHERE type = 'session_rollover_completed'),
      'session_count', (SELECT count(*) FROM agent_sessions WHERE agent_id = '${agentID}'),
      'active_sessions', (SELECT count(*) FROM agent_sessions WHERE agent_id = '${agentID}' AND status = 'active'),
      'pending_sessions', (SELECT count(*) FROM agent_sessions WHERE agent_id = '${agentID}' AND status = 'rollover_pending'),
      'closed_sessions', (SELECT count(*) FROM agent_sessions WHERE agent_id = '${agentID}' AND status = 'closed'),
      'unbound_context_runs', (
        SELECT count(DISTINCT e.run_id) FROM context_events e WHERE e.session_id IS NULL
      ),
      'latest_compaction_run_id', COALESCE((SELECT run_id::text FROM latest_compaction), ''),
      'latest_compaction_session_id', COALESCE((SELECT session_id::text FROM latest_compaction), ''),
      'latest_compaction_status', COALESCE((SELECT run_status FROM latest_compaction), ''),
      'latest_compaction_message_id', COALESCE((
        SELECT m.id::text FROM messages m
         WHERE m.metadata->>'agent_run_id' = (SELECT run_id::text FROM latest_compaction)
         ORDER BY m.created_at LIMIT 1
      ), ''),
      'latest_compaction_payload', COALESCE((SELECT payload FROM latest_compaction), '{}'::jsonb),
      'latest_snapshot_payload', COALESCE((SELECT payload FROM latest_snapshot), '{}'::jsonb)
    )::text
  `);
}

function pressureMessage(agentName: string, provider: string, round: number, acknowledgement?: string): string {
  const reply = acknowledgement ? ` Reply with ${acknowledgement}.` : '';
  const head = `@${agentName} CONTEXT_PRESSURE_${provider}_${round}_${suffix}.${reply}`;
  if (provider === 'codex') return head;
  const words: string[] = [];
  let length = head.length + 27;
  for (let index = 0; length < 9300; index += 1) {
    const word = `${provider[0]}${round.toString(36)}${index.toString(36)}${((index + 1) * 48271 + round * 7919).toString(36)}`;
    words.push(word);
    length += word.length + 1;
  }
  return `${head} Ignore the padding below.\n${words.join(' ')}`.slice(0, 9800);
}

async function authenticatePage(page: Page, auth: Auth) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

async function expectCompactionUI(page: Page, channelID: string, agentName: string) {
  await page.goto(`/dashboard?channel=${channelID}`);
  await page.getByRole('button', { name: 'Teams', exact: true }).click();
  await page.locator('.relationship-agent-node').filter({ hasText: agentName }).click();
  await page.getByRole('tab', { name: 'Run history' }).click();
  const compaction = page.locator('details').filter({ has: page.getByText('Context compaction', { exact: true }) }).first();
  await expect(compaction.getByText('Context compaction', { exact: true })).toBeVisible();
  await compaction.locator(':scope > summary').click();
  await expect(compaction.getByText('Measurement', { exact: true })).toBeVisible();
}

test.describe('native context compaction telemetry', () => {
  test.skip(process.env.SOLO_E2E_REAL_CONTEXT_COMPACTION !== '1', 'requires real local Claude and Codex runtimes');
  test.setTimeout(1_200_000);

  // Claude covers healthy native compaction. Codex drives a real rollover and
  // proves the replacement Session received a Task-aware Continuity Packet.
  test('persists native Claude and Codex compaction signals with visible replies and live Sessions', async ({ page, request }) => {
    const email = `context-compaction-${suffix}@solo.local`;
    const registration = await registerVerified(request, apiBase, {
      data: { email, password: 'SoloE2E-2026!', display_name: 'Context Compaction E2E' },
    });
    expect(registration.ok()).toBeTruthy();
    const auth = await registration.json() as Auth;
    databaseJSON<boolean>(`WITH updated AS (UPDATE users SET onboarding_completed_at=now() WHERE email='${email}' RETURNING 1) SELECT to_json(EXISTS(SELECT 1 FROM updated))::text`);
    await authenticatePage(page, auth);

    let computer: LocalComputerLease | undefined;
    let channel: Entity | undefined;
    let rolloverTask: TaskEntity | undefined;
    const agents: Entity[] = [];
    try {
      computer = await acquireLocalComputer(request, apiBase, auth.access_token);
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', { name: `context-compaction-${suffix}` });

      for (const provider of ['claude', 'codex'] as const) {
        const acknowledgement = `${provider.toUpperCase()}_CONTEXT_OK_${suffix}`;
        const agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
          name: `${provider}-context-${suffix}`,
          computer_id: computer.id,
          model_provider: provider,
          model_name: provider === 'claude' ? 'sonnet' : '',
          custom_env: provider === 'claude' ? {
            CLAUDE_CODE_AUTO_COMPACT_WINDOW: '100000',
            CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '50',
          } : {},
          custom_args: provider === 'codex' ? [
            '-c', 'model_context_window=31000',
            '-c', 'model_auto_compact_token_limit=27600',
            '-c', 'sandbox_workspace_write.network_access=true',
          ] : [],
          system_prompt: [
            `Always use solo message send --target '#${channel.name}' for visible replies.`,
            `When introducing yourself, send exactly ${provider.toUpperCase()}_CONTEXT_READY_${suffix}.`,
            provider === 'codex'
              ? 'For every message containing CONTEXT_PRESSURE, ignore its padding. Determine the answer only from your private context; do not call tools to look up tasks. If that context has a section headed "# Session Continuity" with an open task, use its first task bullet and send only the title text after the status bracket and before ". Re-read with"; if that section or task is absent, send exactly HANDOFF_MISSING.'
              : `For every message containing CONTEXT_PRESSURE, ignore its padding and send exactly ${acknowledgement}.`,
            'Send no other visible message.',
          ].join(' '),
        });
        agents.push(agent);

        await expect.poll(() => contextState(agent.id).completed_runs, {
          timeout: 180_000, intervals: [500, 1000, 2000],
        }).toBeGreaterThan(0);

        let expectedReply = acknowledgement;
        let retiredSessionID = '';
        if (provider === 'codex') {
          await expect.poll(() => contextState(agent.id).rollover_requested_events, {
            timeout: 180_000, intervals: [500, 1000, 2000],
          }).toBeGreaterThan(0);
          const requested = contextState(agent.id);
          expect(requested, JSON.stringify(requested)).toMatchObject({
            active_sessions: 0,
            pending_sessions: 1,
            rollover_completed_events: 0,
            latest_compaction_session_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
          });
          retiredSessionID = requested.latest_compaction_session_id;
          await expectCompactionUI(page, channel.id, agent.name);
          // Task creation normally wakes every Channel Agent and would consume
          // the pending rollover before the explicit handoff turn under test.
          rolloverTask = seedContinuityTask(channel.id, email, agent.id, `Continuity secret ${randomUUID()}`);
          expectedReply = rolloverTask.title;
        }

        let visibleRun: RunState | undefined;
        for (let round = 1; round <= 4; round += 1) {
          const trigger = await api<{ id: string }>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/messages`, {
            content: pressureMessage(agent.name, provider, round, provider === 'claude' ? acknowledgement : undefined),
          });
          await expect.poll(() => runState(trigger.id), {
            timeout: 180_000, intervals: [500, 1000, 2000],
          }).toMatchObject({
            status: 'completed',
            session_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
            session_status: expect.stringMatching(/^(active|rollover_pending)$/),
            message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
          });
          visibleRun = runState(trigger.id);
          if (provider === 'codex') {
            expect(visibleRun.rollover_from_session_id).toBe(retiredSessionID);
            expect(visibleRun.session_id).not.toBe(retiredSessionID);
            expect(visibleRun.rollover_completed_events).toBeGreaterThan(0);
            expect(visibleRun.task_lookup_events).toBe(0);
          }
          expect(visibleRun.message_content).toBe(expectedReply);
          if (contextState(agent.id).compaction_events > 0) break;
        }

        const state = contextState(agent.id);
        expect(state, JSON.stringify(state)).toMatchObject({
          compaction_events: expect.any(Number),
          session_count: expect.any(Number),
          unbound_context_runs: 0,
          latest_compaction_run_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
          latest_compaction_status: 'completed',
          latest_compaction_message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
          latest_compaction_payload: {
            type: 'compaction_end',
            accuracy: provider === 'claude' ? 'reported' : 'snapshot',
          },
        });
        expect(state.compaction_events, JSON.stringify(state)).toBeGreaterThan(0);
        expect(state.session_count).toBeGreaterThan(0);
        expect(state.active_sessions + state.pending_sessions, JSON.stringify(state)).toBe(1);
        if (provider === 'codex') {
          expect(state.rollover_completed_events, JSON.stringify(state)).toBeGreaterThan(0);
          expect(state.closed_sessions, JSON.stringify(state)).toBeGreaterThan(0);
        }

        if (provider === 'claude') {
          expect(Number(state.latest_compaction_payload.before_tokens ?? 0), JSON.stringify(state)).toBeGreaterThan(0);
        } else {
          expect(state.snapshot_events, JSON.stringify(state)).toBeGreaterThan(0);
          expect(state.latest_snapshot_payload).toMatchObject({ type: 'usage', accuracy: 'snapshot' });
          expect(Number(state.latest_snapshot_payload.used_tokens ?? 0), JSON.stringify(state)).toBeGreaterThan(0);
          expect(Number(state.latest_snapshot_payload.window_tokens ?? 0), JSON.stringify(state)).toBeGreaterThan(0);
        }

        await page.goto(`/dashboard?channel=${channel.id}`);
        await expect(page.locator(`[data-message-id="${visibleRun!.message_id}"]`)).toContainText(expectedReply);
        if (provider === 'claude') await expectCompactionUI(page, channel.id, agent.name);
      }
    } finally {
      if (rolloverTask && channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}/tasks/${rolloverTask.id}`).catch(() => undefined);
      for (const agent of agents) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
      await computer?.release(request).catch(() => undefined);
    }
  });
});
