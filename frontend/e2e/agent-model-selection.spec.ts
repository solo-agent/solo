import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'agent-model-selection-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface Agent extends Entity {
  computer_id: string;
  model_name: string;
  model_provider: string;
}

interface SessionState {
  status: string;
  model_name: string;
  external_session_id: string;
  transcript_path: string;
  visible_message: boolean;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec',
    process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA',
    '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function agentModel(agentID: string): string {
  return databaseJSON<{ model_name: string }>(`
    SELECT json_build_object('model_name', model_name)::text
      FROM agents
     WHERE id = '${agentID}'
  `).model_name;
}

function latestSessionState(agentID: string, expectedContent: string, triggerMessageID = ''): SessionState {
  const triggerFilter = triggerMessageID ? `AND run.trigger_message_id = '${triggerMessageID}'` : '';
  return databaseJSON<SessionState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'status', run.status,
        'model_name', agent.model_name,
        'external_session_id', COALESCE(session.external_session_id, ''),
        'transcript_path', COALESCE(session.transcript_path, ''),
        'visible_message', EXISTS (
          SELECT 1
            FROM messages
           WHERE metadata->>'agent_run_id' = run.id::text
             AND content = '${expectedContent}'
        )
      )::text
        FROM agent_runs run
        JOIN agents agent ON agent.id = run.agent_id
        LEFT JOIN agent_sessions session ON session.id = run.session_id
       WHERE run.agent_id = '${agentID}'
         ${triggerFilter}
       ORDER BY run.started_at DESC
       LIMIT 1
    ), '{"status":"","model_name":"","external_session_id":"","transcript_path":"","visible_message":false}')
  `);
}

function daemonStartedWithModel(since: number, model: string): boolean {
  const path = process.env.SOLO_DAEMON_LOG ?? resolve(process.cwd(), '..', 'daemon.log');
  return readFileSync(path, 'utf8').split('\n').some((line) => {
    try {
      const entry = JSON.parse(line) as { time?: string; msg?: string; args?: string[] };
      const modelIndex = entry.args?.indexOf('--model') ?? -1;
      return entry.msg === 'claude: starting persistent session'
        && Date.parse(entry.time ?? '') >= since
        && modelIndex >= 0
        && entry.args?.[modelIndex + 1] === model;
    } catch {
      return false;
    }
  });
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Agent Model Selection E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

test.describe('real Agent model selection', () => {
  test.skip(process.env.SOLO_E2E_REAL_MODEL_SELECTION !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(360000);

  test('creates, edits, and applies a model through the real product flow', async ({ page, request }) => {
    const auth = await authenticate(request);
    const headers = { authorization: `Bearer ${auth.access_token}` };
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Agent | null = null;
    let localComputer: LocalComputerLease | null = null;
    const startedAt = Date.now();

    try {
      localComputer = await acquireLocalComputer(request, apiBase, auth.access_token);
      const channelResponse = await request.post(`${apiBase}/api/v1/channels`, {
        headers,
        data: { name: `model-selection-e2e-${suffix}`, description: 'Real model selection E2E' },
      });
      expect(channelResponse.ok()).toBeTruthy();
      channel = await channelResponse.json();

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel!.id}`);
      await page.getByRole('button', { name: 'Teams', exact: true }).click();
      await page.getByRole('button', { name: 'Create first Agent' }).click();

      const dialog = page.getByRole('dialog');
      await dialog.getByLabel('Name *').fill(`Model E2E ${suffix}`);
      await dialog.getByRole('button', { name: 'Select where this Agent runs' }).click();
      await page.locator('[role="option"]').filter({ hasText: localComputer.name }).click();
      await dialog.getByRole('button', { name: 'Select Runtime...' }).click();
      await page.locator('[role="option"]').filter({ hasText: 'Claude' }).click();

      const modelInput = dialog.getByLabel('Model (optional)');
      await modelInput.fill('opus');
      await dialog.getByRole('button', { name: /Claude/ }).click();
      await page.locator('[role="option"]').filter({ hasText: 'Codex' }).click();
      await expect(modelInput).toHaveValue('');
      await dialog.getByRole('button', { name: /Codex/ }).click();
      await page.locator('[role="option"]').filter({ hasText: 'Claude' }).click();

      await modelInput.fill('sonnet');
      await dialog.getByLabel('System Prompt').fill([
        `Always deliver replies to the channel root with solo message send --target '#${channel!.name}'. Never reply in a thread.`,
        'When introducing yourself, send exactly MODEL_SONNET_READY.',
        'When a user says SWITCH_CHECK, send exactly MODEL_HAIKU_READY.',
      ].join(' '));
      await dialog.getByRole('button', { name: 'Create Agent' }).click();
      await expect(dialog).toBeHidden();

      const agentsResponse = await request.get(`${apiBase}/api/v1/channels/${channel!.id}/agents`, { headers });
      expect(agentsResponse.ok()).toBeTruthy();
      const agents = await agentsResponse.json() as Agent[];
      agent = agents.find((candidate) => candidate.name === `Model E2E ${suffix}`) ?? null;
      expect(agent).not.toBeNull();
      expect(agent!.computer_id).toBe(localComputer.id);
      expect(agent!.model_name).toBe('sonnet');
      expect(agentModel(agent!.id)).toBe('sonnet');

      await expect.poll(() => {
        const state = latestSessionState(agent!.id, 'MODEL_SONNET_READY');
        return `${state.status}/${Boolean(state.external_session_id)}/${Boolean(state.transcript_path)}/${state.visible_message}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/true/true/true');
      const sonnetSession = latestSessionState(agent!.id, 'MODEL_SONNET_READY');
      expect(daemonStartedWithModel(startedAt, 'sonnet')).toBe(true);
      await expect(page.getByText('MODEL_SONNET_READY', { exact: true }).first()).toBeVisible();

      await page.locator('.react-flow__node').filter({ hasText: agent!.name }).click();
      await expect(page.getByRole('heading', { name: 'Agent Detail' })).toBeVisible();
      await page.getByRole('button', { name: 'Edit Model (optional)' }).click();
      const runtimeModelInput = page.getByLabel('Model (optional)');
      await runtimeModelInput.fill('haiku');
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect.poll(() => agentModel(agent!.id)).toBe('haiku');

      const messageResponse = await request.post(`${apiBase}/api/v1/channels/${channel!.id}/messages`, {
        headers,
        data: { content: 'SWITCH_CHECK' },
      });
      expect(messageResponse.ok()).toBeTruthy();
      const triggerMessage = await messageResponse.json() as { id: string };

      await expect.poll(() => {
        const state = latestSessionState(agent!.id, 'MODEL_HAIKU_READY', triggerMessage.id);
        return `${state.status}/${Boolean(state.external_session_id)}/${Boolean(state.transcript_path)}/${state.visible_message}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/true/true/true');
      const haikuSession = latestSessionState(agent!.id, 'MODEL_HAIKU_READY', triggerMessage.id);
      expect(haikuSession.external_session_id).not.toBe(sonnetSession.external_session_id);
      expect(daemonStartedWithModel(startedAt, 'haiku')).toBe(true);
      await page.goto(`/dashboard?channel=${channel!.id}`);
      await expect(page.getByText('MODEL_HAIKU_READY', { exact: true }).first()).toBeVisible();

      const customModel = 'claude-sonnet-4-6';
      await page.locator('.react-flow__node').filter({ hasText: agent!.name }).click();
      await expect(page.getByRole('heading', { name: 'Agent Detail' })).toBeVisible();
      await page.getByRole('button', { name: 'Edit Model (optional)' }).click();
      await runtimeModelInput.fill(customModel);
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect.poll(() => agentModel(agent!.id)).toBe(customModel);
      await expect(page.getByText(customModel, { exact: true })).toBeVisible();

      const oversizedResponse = await request.patch(`${apiBase}/api/v1/agents/${agent!.id}`, {
        headers,
        data: { model_name: 'm'.repeat(101) },
      });
      expect(oversizedResponse.status()).toBe(400);
      expect(agentModel(agent!.id)).toBe(customModel);

      await page.getByRole('button', { name: 'Edit Model (optional)' }).click();
      await expect(runtimeModelInput).toHaveValue(customModel);
      await runtimeModelInput.fill('');
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect.poll(() => agentModel(agent!.id)).toBe('');
      await expect(page.getByText('Default', { exact: true }).first()).toBeVisible();
    } finally {
      if (agent) {
        await request.delete(`${apiBase}/api/v1/agents/${agent.id}`, { headers }).catch(() => undefined);
      }
      if (channel) {
        await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers }).catch(() => undefined);
      }
      if (localComputer) {
        await localComputer.release(request).catch(() => undefined);
      }
    }
  });
});
