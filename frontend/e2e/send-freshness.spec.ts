import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'send-freshness-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface RelayMessage {
  id: string;
  sender_id: string;
  content: string;
}

interface RelayState {
  runs: number;
  completed: number;
  active: number;
  failed: number;
  held_runs: number;
  held_events: number;
  distinct_senders: number;
  messages: RelayMessage[];
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

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Send Freshness E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

async function api<T>(
  request: APIRequestContext,
  token: string,
  method: 'post' | 'delete',
  path: string,
  data?: unknown,
): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: { authorization: `Bearer ${token}` },
    data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

function agentsReady(agentIDs: string[]): boolean {
  const ids = agentIDs.map((id) => `'${id}'`).join(',');
  return databaseJSON<boolean>(`
    SELECT (
      COUNT(DISTINCT agent_id) FILTER (WHERE finished_at IS NOT NULL) = ${agentIDs.length}
      AND COUNT(*) FILTER (WHERE status IN ('queued','thinking','running','streaming','waiting_input','waiting_approval')) = 0
    )::text
      FROM agent_runs
     WHERE agent_id IN (${ids})
  `);
}

function relayState(triggerMessageID: string, agentIDs: string[]): RelayState {
  const ids = agentIDs.map((id) => `'${id}'`).join(',');
  return databaseJSON<RelayState>(`
    WITH relay_runs AS (
      SELECT *
        FROM agent_runs
       WHERE trigger_message_id = '${triggerMessageID}'
         AND agent_id IN (${ids})
    ), relay_messages AS (
      SELECT m.id, m.seq, m.sender_id, m.content
        FROM messages m
       WHERE m.metadata->>'agent_run_id' IN (SELECT id::text FROM relay_runs)
    )
    SELECT json_build_object(
      'runs', (SELECT COUNT(*) FROM relay_runs),
      'completed', (SELECT COUNT(*) FROM relay_runs WHERE status = 'completed'),
      'active', (SELECT COUNT(*) FROM relay_runs WHERE status IN ('queued','thinking','running','streaming','waiting_input','waiting_approval')),
      'failed', (SELECT COUNT(*) FROM relay_runs WHERE status IN ('failed','timeout','cancelled')),
      'held_runs', (SELECT COUNT(*) FROM relay_runs WHERE freshness_held_at IS NOT NULL),
      'held_events', (
        SELECT COUNT(*) FROM agent_run_events
         WHERE run_id IN (SELECT id FROM relay_runs) AND type = 'visible_message_held'
      ),
      'distinct_senders', (SELECT COUNT(DISTINCT sender_id) FROM relay_messages),
      'messages', COALESCE((
        SELECT json_agg(json_build_object(
          'id', id::text,
          'sender_id', sender_id::text,
          'content', content
        ) ORDER BY seq)
          FROM relay_messages
      ), '[]'::json)
    )::text
  `);
}

test('three explicitly mentioned real Agents relay exactly 1, 2, 3 through freshness holds', async ({ page, request }) => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_FRESHNESS !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(300_000);

  const auth = await authenticate(request);
  const suffix = Date.now().toString(36);
  let channel: Entity | null = null;
  const agents: Entity[] = [];

  try {
    channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
      name: `freshness-relay-${suffix}`,
      description: 'Three real Agent Send Freshness relay E2E',
    });

    for (const label of ['A', 'B', 'C']) {
      const ready = `RELAY_${label}_READY_${suffix.toUpperCase()}`;
      agents.push(await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Relay${label}${suffix}`,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          `When introducing yourself, use solo message send to send exactly ${ready}.`,
          'When a human message contains FRESHNESS_RELAY, follow its relay rules literally.',
          'Immediately attempt the number you currently believe is next.',
          'If solo message send returns HELD, read the new messages, recompute the next number, and call solo message send again.',
          'Stop only after one visible number is successfully sent. Send no other visible text.',
        ].join(' '),
      }));
    }

    await expect.poll(() => agentsReady(agents.map((agent) => agent.id)), {
      timeout: 180_000,
      intervals: [500, 1000, 2000],
    }).toBe(true);

    const relayPrompt = [
      agents.map((agent) => `@${agent.name}`).join(' '),
      'FRESHNESS_RELAY: This is a three-Agent counting relay.',
      'All three Agents must participate, and the channel must end with exactly 1, 2, 3 in that order.',
      'Each Agent may successfully send exactly one visible message.',
      'Act immediately from the messages currently in your context: if no relay number has been sent, attempt 1; if the latest relay number is N, attempt N+1.',
      'If the send is HELD, it did not succeed: read the returned new messages, recompute, and try again.',
      'After one successful send, stop. Send only one Arabic digit with no names, punctuation, mentions, or explanation.',
    ].join(' ');

    await authenticatePage(page, auth);
    await page.goto(`/dashboard?channel=${channel.id}`);
    const composer = page.getByPlaceholder('Type a message...');
    await composer.fill(relayPrompt);
    await composer.press('Enter');
    await expect(page.getByLabel('Message list').getByText(relayPrompt, { exact: true })).toBeVisible();

    await expect.poll(() => databaseJSON<string>(`
      SELECT to_json(COALESCE((
        SELECT id::text FROM messages
         WHERE channel_id = '${channel!.id}' AND content = $relay$${relayPrompt}$relay$
         ORDER BY seq DESC LIMIT 1
      ), ''))::text
    `), { timeout: 30_000, intervals: [250, 500, 1000] }).toMatch(/^[0-9a-f-]{36}$/);
    const triggerMessageID = databaseJSON<string>(`
      SELECT to_json(id::text)::text FROM messages
       WHERE channel_id = '${channel!.id}' AND content = $relay$${relayPrompt}$relay$
       ORDER BY seq DESC LIMIT 1
    `);

    await expect.poll(() => {
      const state = relayState(triggerMessageID, agents.map((agent) => agent.id));
      return {
        runs: state.runs,
        completed: state.completed,
        active: state.active,
        failed: state.failed,
        held: state.held_runs,
        heldEventsAtLeast2: state.held_events >= 2,
        senders: state.distinct_senders,
        contents: state.messages.map((message) => message.content).join(','),
      };
    }, { timeout: 240_000, intervals: [500, 1000, 2000] }).toEqual({
      runs: 3,
      completed: 3,
      active: 0,
      failed: 0,
      held: 2,
      heldEventsAtLeast2: true,
      senders: 3,
      contents: '1,2,3',
    });

    const state = relayState(triggerMessageID, agents.map((agent) => agent.id));
    expect(state.messages).toHaveLength(3);
    for (const [index, message] of state.messages.entries()) {
      await expect(page.locator(`[data-message-id="${message.id}"]`).getByText(String(index + 1), { exact: true })).toBeVisible();
    }
  } finally {
    for (const agent of agents.reverse()) {
      await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
    }
    if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
  }
});
