import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const publicWorkspaceID = '00000000-0000-0000-0000-000000000001';

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  workspace_id: string;
}

interface Entity {
  id: string;
  name: string;
}

interface Computer extends Entity {
  daemon_id: string;
  status: string;
  pairing_status: string;
  my_role?: string;
}

interface Workspace extends Entity {
  is_personal: boolean;
}

interface PendingState {
  rows: number;
  first_seq: number;
  latest_seq: number;
  messages: number;
  runs: number;
}

interface FinalState {
  messages: number;
  runs: number;
  first_trigger_runs: number;
  second_trigger_runs: number;
  completed: number;
  pending: number;
  coalesced: boolean;
  wake_count: number;
  replies: number;
  linked_replies: number;
  reminders: number;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function register(request: APIRequestContext): Promise<AuthResponse> {
  const email = 'coalesce-agent-wakes@solo.local';
  const password = 'SoloCoalesce-2026!';
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: { email, password } });
  if (login.ok()) return login.json();
  const response = await registerVerified(request, apiBase, {
    data: {
      email,
      password,
      display_name: 'Coalesce E2E',
    },
  });
  if (!response.ok()) throw new Error(`register: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function call<T>(
  request: APIRequestContext,
  auth: AuthResponse,
  method: 'get' | 'post' | 'delete',
  path: string,
  workspaceID?: string,
  data?: unknown,
): Promise<T> {
  const options = {
    headers: {
      authorization: `Bearer ${auth.access_token}`,
      ...(workspaceID ? { 'X-Workspace-ID': workspaceID } : {}),
    },
    ...(data === undefined ? {} : { data }),
  };
  const response = method === 'get'
    ? await request.get(`${apiBase}${path}`, options)
    : method === 'post'
      ? await request.post(`${apiBase}${path}`, options)
      : await request.delete(`${apiBase}${path}`, options);
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse, workspaceID: string) {
  await page.addInitScript(({ accessToken, refreshToken, activeWorkspaceID }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo_active_workspace_id', activeWorkspaceID);
    localStorage.setItem('solo.locale', 'en');
  }, {
    accessToken: auth.access_token,
    refreshToken: auth.refresh_token,
    activeWorkspaceID: workspaceID,
  });
}

async function sendMessage(request: APIRequestContext, auth: AuthResponse, workspaceID: string, channelID: string, content: string): Promise<string> {
  const message = await call<{ id: string }>(request, auth, 'post', `/api/v1/channels/${channelID}/messages`, workspaceID, { content });
  return message.id;
}

function runActive(agentID: string, triggerID = ''): boolean {
  return databaseJSON<boolean>(`
    SELECT EXISTS(
      SELECT 1 FROM agent_runs
       WHERE agent_id='${agentID}' AND finished_at IS NULL
         AND ('${triggerID}'='' OR trigger_message_id='${triggerID}')
    )::text
  `);
}

test('merges every busy message and keeps public chatter mention-only', async ({ page, request }) => {
  test.setTimeout(420_000);
  const auth = await register(request);
  const workspaces = await call<Workspace[]>(request, auth, 'get', '/api/v1/workspaces');
  const privateWorkspaceID = workspaces.find((workspace) => workspace.is_personal)?.id;
  expect(privateWorkspaceID, 'personal Workspace').toBeTruthy();
  const suffix = Date.now().toString(36);
  const daemonID = process.env.SOLO_E2E_DAEMON_ID ?? 'daemon-e2e-coalesce';
  const computers = await call<Computer[]>(request, auth, 'get', '/api/v1/computers');
  const computer = computers.find((item) => item.daemon_id === daemonID && item.status === 'online');
  expect(computer, `online ${daemonID} Computer`).toBeTruthy();
  if (!computer!.my_role) {
    await call(request, auth, 'post', `/api/v1/computers/${computer!.id}/claim`);
  }

  const block = `COALESCE_BLOCK_${suffix}`;
  const one = `COALESCE_ONE_${suffix}`;
  const two = `COALESCE_TWO_${suffix}`;
  const blockReply = `COALESCE_BLOCK_ACK_${suffix.toUpperCase()}`;
  const batchReply = `COALESCE_BATCH_ACK_${suffix.toUpperCase()}`;
  const publicPlain = `PUBLIC_PLAIN_${suffix}`;
  const publicMention = `PUBLIC_MENTION_${suffix}`;
  const publicReply = `PUBLIC_ACK_${suffix.toUpperCase()}`;
  let privateChannel: Entity | undefined;
  let privateAgent: Entity | undefined;
  let publicAgent: Entity | undefined;

  try {
    privateChannel = await call<Entity>(request, auth, 'post', '/api/v1/channels', privateWorkspaceID, {
      name: `coalesce-${suffix}`,
      description: 'real Agent busy-message coalescing E2E',
    });
    privateAgent = await call<Entity>(request, auth, 'post', `/api/v1/channels/${privateChannel.id}/agents`, privateWorkspaceID, {
      name: `Coalesce${suffix}`,
      computer_id: computer!.id,
      model_provider: 'claude',
      model_name: 'sonnet',
      system_prompt: [
        'Use solo message send with the exact target from the incoming message.',
        'When introducing yourself, send exactly COALESCE_READY.',
        `For a message containing ${block}, run the foreground Bash command sleep 12 and wait, then send exactly ${blockReply}.`,
        `When one turn contains both ${one} and ${two}, send exactly ${batchReply}.`,
        'Send one visible message and no explanation.',
      ].join(' '),
    });
    await expect.poll(() => databaseJSON<boolean>(`
      SELECT EXISTS(SELECT 1 FROM agent_runs WHERE agent_id='${privateAgent!.id}' AND trigger_message_id IS NULL AND status='completed')::text
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toBe(true);

    const blockID = await sendMessage(request, auth, privateWorkspaceID!, privateChannel.id, `@${privateAgent.name} ${block}`);
    await expect.poll(() => runActive(privateAgent!.id, blockID), {
      timeout: 90_000, intervals: [250, 500, 1000],
    }).toBe(true);
    const oneID = await sendMessage(request, auth, privateWorkspaceID!, privateChannel.id, `@${privateAgent.name} ${one}`);
    const twoID = await sendMessage(request, auth, privateWorkspaceID!, privateChannel.id, `@${privateAgent.name} ${two}`);

    await expect.poll(() => databaseJSON<PendingState>(`
      WITH target_messages AS (
        SELECT seq FROM messages WHERE id IN ('${oneID}','${twoID}')
      )
      SELECT json_build_object(
        'rows', (SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id='${privateAgent!.id}' AND channel_id='${privateChannel!.id}'),
        'first_seq', COALESCE((SELECT first_message_seq FROM agent_pending_message_wakes WHERE agent_id='${privateAgent!.id}' AND channel_id='${privateChannel!.id}'),0),
        'latest_seq', COALESCE((SELECT latest_message_seq FROM agent_pending_message_wakes WHERE agent_id='${privateAgent!.id}' AND channel_id='${privateChannel!.id}'),0),
        'messages', (SELECT count(*) FROM target_messages),
        'runs', (SELECT count(*) FROM agent_runs WHERE agent_id='${privateAgent!.id}' AND trigger_message_id IN ('${blockID}','${oneID}','${twoID}'))
      )::text
    `), { timeout: 20_000, intervals: [100, 250, 500] }).toMatchObject({ rows: 1, messages: 2, runs: 1 });

    await expect.poll(() => databaseJSON<FinalState>(`
      WITH target_runs AS (
        SELECT * FROM agent_runs
         WHERE agent_id='${privateAgent!.id}'
           AND trigger_message_id IN ('${blockID}','${oneID}','${twoID}')
      ), coalesced_run AS (
        SELECT * FROM target_runs WHERE trigger_message_id='${twoID}' LIMIT 1
      ), started AS (
        SELECT payload FROM agent_run_events
         WHERE run_id=(SELECT id FROM coalesced_run) AND type='run_started'
         ORDER BY seq LIMIT 1
      )
      SELECT json_build_object(
        'messages', (SELECT count(*) FROM messages WHERE id IN ('${blockID}','${oneID}','${twoID}')),
        'runs', (SELECT count(*) FROM target_runs),
        'first_trigger_runs', (SELECT count(*) FROM target_runs WHERE trigger_message_id='${oneID}'),
        'second_trigger_runs', (SELECT count(*) FROM target_runs WHERE trigger_message_id='${twoID}'),
        'completed', (SELECT count(*) FROM target_runs WHERE status='completed'),
        'pending', (SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id='${privateAgent!.id}' AND channel_id='${privateChannel!.id}'),
        'coalesced', COALESCE((SELECT (payload->>'coalesced')::boolean FROM started),false),
        'wake_count', COALESCE((SELECT (payload->>'wake_message_count')::int FROM started),0),
        'replies', (SELECT count(*) FROM messages WHERE sender_id='${privateAgent!.id}' AND content IN ('${blockReply}','${batchReply}')),
        'linked_replies', (SELECT count(*) FROM messages m JOIN target_runs r ON m.metadata->>'agent_run_id'=r.id::text WHERE m.sender_id='${privateAgent!.id}' AND m.content IN ('${blockReply}','${batchReply}')),
        'reminders', (SELECT count(*) FROM agent_run_events WHERE run_id IN (SELECT id FROM target_runs) AND type='result_reminder')
      )::text
    `), { timeout: 300_000, intervals: [500, 1000, 2000] }).toEqual({
      messages: 3,
      runs: 2,
      first_trigger_runs: 0,
      second_trigger_runs: 1,
      completed: 2,
      pending: 0,
      coalesced: true,
      wake_count: 2,
      replies: 2,
      linked_replies: 2,
      reminders: 0,
    });

    await authenticatePage(page, auth, privateWorkspaceID!);
    await page.goto(`/dashboard?channel=${privateChannel.id}`);
    await page.reload();
    for (const text of [block, one, two, batchReply]) {
      await expect(page.getByText(new RegExp(text), { exact: false }).first()).toBeVisible();
    }
    const blockRoot = page.locator(`[data-message-id="${blockID}"]`);
    await blockRoot.hover();
    await blockRoot.getByRole('button', { name: /^Reply to / }).click();
    await expect(page.getByText(blockReply, { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Close thread panel' }).click();

    const publicGeneral = databaseJSON<Entity>(`
      SELECT json_build_object('id',id::text,'name',name)::text
        FROM channels
       WHERE workspace_id='${publicWorkspaceID}' AND type='channel' AND is_archived=false
       ORDER BY created_at LIMIT 1
    `);
    publicAgent = await call<Entity>(request, auth, 'post', `/api/v1/channels/${publicGeneral.id}/agents`, publicWorkspaceID, {
      name: `Public${suffix}`,
      computer_id: computer!.id,
      model_provider: 'claude',
      model_name: 'sonnet',
      system_prompt: [
        'Use solo message send with the exact incoming target.',
        'When introducing yourself, send exactly PUBLIC_READY.',
        `For a message containing ${publicMention}, send exactly ${publicReply}.`,
        'Send one visible message and no explanation.',
      ].join(' '),
    });
    await expect.poll(() => databaseJSON<boolean>(`
      SELECT EXISTS(SELECT 1 FROM agent_runs WHERE agent_id='${publicAgent!.id}' AND trigger_message_id IS NULL AND status='completed')::text
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toBe(true);
    const plainID = await sendMessage(request, auth, publicWorkspaceID, publicGeneral.id, publicPlain);
    await new Promise((resolve) => setTimeout(resolve, 2500));
    expect(databaseJSON<number>(`SELECT count(*)::text FROM agent_runs WHERE agent_id='${publicAgent.id}' AND trigger_message_id='${plainID}'`)).toBe(0);
    const mentionedID = await sendMessage(request, auth, publicWorkspaceID, publicGeneral.id, `@${publicAgent.name} ${publicMention}`);
    await expect.poll(() => databaseJSON<number>(`
      SELECT count(*)::text FROM agent_runs r
       WHERE r.agent_id='${publicAgent!.id}' AND r.trigger_message_id='${mentionedID}' AND r.status='completed'
         AND EXISTS(SELECT 1 FROM messages m WHERE m.metadata->>'agent_run_id'=r.id::text AND m.content='${publicReply}')
    `), { timeout: 240_000, intervals: [500, 1000, 2000] }).toBe(1);
  } finally {
    if (publicAgent) await call(request, auth, 'delete', `/api/v1/agents/${publicAgent.id}`, publicWorkspaceID).catch(() => undefined);
    if (privateAgent) await call(request, auth, 'delete', `/api/v1/agents/${privateAgent.id}`, privateWorkspaceID).catch(() => undefined);
    if (privateChannel) await call(request, auth, 'delete', `/api/v1/channels/${privateChannel.id}`, privateWorkspaceID).catch(() => undefined);
  }
});
