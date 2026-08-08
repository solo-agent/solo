import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const daemonBase = process.env.SOLO_E2E_DAEMON_URL ?? 'http://127.0.0.1:8081';
const credentials = { email: 'agent-message-flow-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface ScopeState {
  run_id: string;
  status: string;
  channel_id: string;
  thread_id: string;
  message_id: string;
  message_channel_id: string;
  message_thread_id: string;
  replies: number;
}

interface BusyState {
  runs: number;
  queued: number;
  started: number;
  completed: number;
  sessions: number;
  held: number;
  first_replies: number;
  second_replies: number;
  ordered: boolean;
}

interface FollowupState {
  runs: number;
  completed: number;
  held: number;
  replies: number;
  message_id: string;
}

interface RescueState {
  run_id: string;
  status: string;
  reminders: number;
  replies: number;
  leaked_replies: number;
  historical_runs: number;
}

interface DelegationState {
  task_id: string;
  task_number: number;
  status: string;
  creator_id: string;
  claimer_id: string;
  message_id: string;
  thread_id: string;
  worker_replies: number;
  worker_reply_id: string;
  creator_dm_id: string;
  creator_reply_id: string;
  creator_run_status: string;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Agent Message Flow E2E' },
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

function greetingCompleted(agentID: string): boolean {
  return databaseJSON<boolean>(`
    SELECT EXISTS(
      SELECT 1 FROM agent_runs
       WHERE agent_id = '${agentID}'
         AND trigger_message_id IS NULL
         AND status = 'completed'
    )::text
  `);
}

function greetingActive(agentID: string): boolean {
  return databaseJSON<boolean>(`
    SELECT EXISTS(
      SELECT 1 FROM agent_runs
       WHERE agent_id = '${agentID}'
         AND trigger_message_id IS NULL
         AND backend_started_at IS NOT NULL
         AND status IN ('thinking','running','streaming','waiting_input','waiting_approval')
    )::text
  `);
}

function messageID(channelID: string, content: string, threadOnly = false): string {
  return databaseJSON<string>(`
    SELECT to_json(COALESCE((
      SELECT id::text FROM messages
       WHERE channel_id = '${channelID}'
         AND content = '${content}'
         AND ${threadOnly ? 'thread_id IS NOT NULL' : 'thread_id IS NULL'}
       ORDER BY seq DESC LIMIT 1
    ), ''))::text
  `);
}

function scopeState(triggerMessageID: string, reply: string): ScopeState {
  return databaseJSON<ScopeState>(`
    WITH target_run AS (
      SELECT * FROM agent_runs
       WHERE trigger_message_id = '${triggerMessageID}'
       ORDER BY started_at DESC LIMIT 1
    ), reply AS (
      SELECT m.* FROM messages m
       WHERE m.metadata->>'agent_run_id' = (SELECT id::text FROM target_run)
         AND LOWER(m.content) = LOWER('${reply}')
       ORDER BY m.seq LIMIT 1
    )
    SELECT json_build_object(
      'run_id', COALESCE((SELECT id::text FROM target_run), ''),
      'status', COALESCE((SELECT status FROM target_run), ''),
      'channel_id', COALESCE((SELECT channel_id::text FROM target_run), ''),
      'thread_id', COALESCE((SELECT thread_id::text FROM target_run), ''),
      'message_id', COALESCE((SELECT id::text FROM reply), ''),
      'message_channel_id', COALESCE((SELECT channel_id::text FROM reply), ''),
      'message_thread_id', COALESCE((SELECT thread_id::text FROM reply), ''),
      'replies', (SELECT COUNT(*) FROM reply)
    )::text
  `);
}

function busyState(agentID: string, triggerMessageIDs: string[], firstReply: string, secondReply: string): BusyState {
  const ids = triggerMessageIDs.map((id) => `'${id}'`).join(',');
  return databaseJSON<BusyState>(`
    WITH target_runs AS (
      SELECT * FROM agent_runs
       WHERE agent_id = '${agentID}' AND trigger_message_id IN (${ids})
    ), replies AS (
      SELECT m.* FROM messages m
       WHERE m.metadata->>'agent_run_id' IN (SELECT id::text FROM target_runs)
    )
    SELECT json_build_object(
      'runs', (SELECT COUNT(*) FROM target_runs),
      'queued', (SELECT COUNT(*) FROM target_runs WHERE status = 'queued' AND backend_started_at IS NULL),
      'started', (SELECT COUNT(*) FROM target_runs WHERE backend_started_at IS NOT NULL),
      'completed', (SELECT COUNT(*) FROM target_runs WHERE status = 'completed'),
      'sessions', (SELECT COUNT(DISTINCT session_id) FROM target_runs WHERE session_id IS NOT NULL),
      'held', (SELECT COUNT(*) FROM target_runs WHERE freshness_held_at IS NOT NULL),
      'first_replies', (SELECT COUNT(*) FROM replies WHERE content = '${firstReply}'),
      'second_replies', (SELECT COUNT(*) FROM replies WHERE content = '${secondReply}'),
      'ordered', COALESCE(
        (SELECT MIN(seq) FROM replies WHERE content = '${firstReply}')
        < (SELECT MIN(seq) FROM replies WHERE content = '${secondReply}'),
        false
      )
    )::text
  `);
}

function followupState(agentID: string, triggerMessageID: string, reply: string): FollowupState {
  return databaseJSON<FollowupState>(`
    WITH target_runs AS (
      SELECT * FROM agent_runs
       WHERE agent_id = '${agentID}' AND trigger_message_id = '${triggerMessageID}'
    ), replies AS (
      SELECT m.* FROM messages m
       WHERE m.metadata->>'agent_run_id' IN (SELECT id::text FROM target_runs)
         AND m.content = '${reply}'
    )
    SELECT json_build_object(
      'runs', (SELECT COUNT(*) FROM target_runs),
      'completed', (SELECT COUNT(*) FROM target_runs WHERE status = 'completed'),
      'held', (SELECT COUNT(*) FROM agent_run_events WHERE run_id IN (SELECT id FROM target_runs) AND type = 'visible_message_held'),
      'replies', (SELECT COUNT(*) FROM replies),
      'message_id', COALESCE((SELECT id::text FROM replies ORDER BY seq LIMIT 1), '')
    )::text
  `);
}

function rescueState(agentID: string, triggerMessageID: string, historicalMessageID: string, reply: string, leakedReply: string): RescueState {
  return databaseJSON<RescueState>(`
    WITH rescued_run AS (
      SELECT * FROM agent_runs
       WHERE agent_id = '${agentID}' AND trigger_message_id = '${triggerMessageID}'
       ORDER BY started_at DESC LIMIT 1
    )
    SELECT json_build_object(
      'run_id', COALESCE((SELECT id::text FROM rescued_run), ''),
      'status', COALESCE((SELECT status FROM rescued_run), ''),
      'reminders', (SELECT COUNT(*) FROM agent_run_events WHERE run_id = (SELECT id FROM rescued_run) AND type = 'result_reminder'),
      'replies', (SELECT COUNT(*) FROM messages WHERE metadata->>'agent_run_id' = (SELECT id::text FROM rescued_run) AND content = '${reply}'),
      'leaked_replies', (SELECT COUNT(*) FROM messages WHERE sender_id = '${agentID}' AND content = '${leakedReply}'),
      'historical_runs', (SELECT COUNT(*) FROM agent_runs WHERE agent_id = '${agentID}' AND trigger_message_id = '${historicalMessageID}')
    )::text
  `);
}

function runsForTriggers(agentID: string, triggerMessageIDs: string[], status = ''): number {
  const ids = triggerMessageIDs.map((id) => `'${id}'`).join(',');
  return databaseJSON<number>(`
    SELECT COUNT(*)::text FROM agent_runs
     WHERE agent_id = '${agentID}' AND trigger_message_id IN (${ids})
       AND ('${status}' = '' OR status = '${status}')
  `);
}

function countMessages(channelID: string, senderID: string, prefix: string): number {
  return databaseJSON<number>(`
    SELECT COUNT(*)::text FROM messages
     WHERE channel_id = '${channelID}' AND sender_id = '${senderID}' AND content LIKE '${prefix}%'
  `);
}

function countTasks(channelID: string, title: string): number {
  return databaseJSON<number>(`
    SELECT COUNT(*)::text FROM tasks WHERE channel_id = '${channelID}' AND title = '${title}'
  `);
}

function delegationState(title: string, creatorID: string, workerID: string, workerReply: string, creatorReply: string): DelegationState {
  return databaseJSON<DelegationState>(`
    WITH target_task AS (
      SELECT * FROM tasks WHERE title = '${title}' AND creator_id = '${creatorID}' ORDER BY created_at DESC LIMIT 1
    ), target_thread AS (
      SELECT th.* FROM threads th WHERE th.root_message_id = (SELECT message_id FROM target_task) LIMIT 1
    ), creator_reply AS (
      SELECT m.* FROM messages m
       WHERE m.sender_id = '${creatorID}' AND m.content = '${creatorReply}'
       ORDER BY m.seq DESC LIMIT 1
    ), creator_run AS (
      SELECT r.* FROM agent_runs r
       WHERE r.id::text = (SELECT metadata->>'agent_run_id' FROM creator_reply)
       LIMIT 1
    )
    SELECT json_build_object(
      'task_id', COALESCE((SELECT id::text FROM target_task), ''),
      'task_number', COALESCE((SELECT task_number FROM target_task), 0),
      'status', COALESCE((SELECT status FROM target_task), ''),
      'creator_id', COALESCE((SELECT creator_id::text FROM target_task), ''),
      'claimer_id', COALESCE((SELECT claimer_id::text FROM target_task), ''),
      'message_id', COALESCE((SELECT message_id::text FROM target_task), ''),
      'thread_id', COALESCE((SELECT id::text FROM target_thread), ''),
      'worker_replies', (SELECT COUNT(*) FROM messages WHERE sender_id = '${workerID}' AND content = '${workerReply}' AND thread_id = (SELECT id FROM target_thread)),
      'worker_reply_id', COALESCE((SELECT id::text FROM messages WHERE sender_id = '${workerID}' AND content = '${workerReply}' AND thread_id = (SELECT id FROM target_thread) ORDER BY seq LIMIT 1), ''),
      'creator_dm_id', COALESCE((SELECT channel_id::text FROM creator_reply), ''),
      'creator_reply_id', COALESCE((SELECT id::text FROM creator_reply), ''),
      'creator_run_status', COALESCE((SELECT status FROM creator_run), '')
    )::text
  `);
}

function agentRunActive(agentID: string): boolean {
  return databaseJSON<boolean>(`
    SELECT EXISTS(
      SELECT 1 FROM agent_runs
       WHERE agent_id = '${agentID}'
         AND backend_started_at IS NOT NULL
         AND finished_at IS NULL
    )::text
  `);
}

async function sendMainMessage(page: Page, content: string) {
  const composer = page.locator('textarea[placeholder]').last();
  await composer.fill(content);
  await composer.press('Enter');
  await expect(composer).toHaveValue('');
  await expect(page.getByLabel('Message list').getByText(content, { exact: true })).toHaveCount(1);
}

test.describe('M8 real Agent message behavior', () => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_M8 !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(360_000);

  let computerID = '';
  let computerLease: LocalComputerLease | null = null;

  test.beforeAll(async ({ request }) => {
    const auth = await authenticate(request);
    computerLease = await acquireLocalComputer(request, apiBase, auth.access_token);
    computerID = computerLease.id;
  });

  test('routes real Agent replies to the exact Channel, Thread, and DM scopes', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const channelPrompt = `M8_SCOPE_CHANNEL_${suffix}`;
    const channelReply = `M8_CHANNEL_ACK_${suffix.toUpperCase()}`;
    const threadPrompt = `M8_SCOPE_THREAD_${suffix}`;
    const threadReply = `M8_THREAD_ACK_${suffix.toUpperCase()}`;
    const dmPrompt = `M8_SCOPE_DM_${suffix}`;
    const dmReply = `M8_DM_ACK_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    let dm: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `m8-scope-${suffix}`,
        description: 'M8 real message scope E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Scope${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'Copy the latest incoming target= field verbatim into solo message send --target.',
          'Never append the separate msg= value: a target without a suffix stays at the Channel or DM root, while a target that already has a suffix stays in that Thread.',
          'When introducing yourself, send exactly M8_SCOPE_READY.',
          `For a message containing ${channelPrompt}, send exactly ${channelReply}.`,
          `For a message containing ${threadPrompt}, send exactly ${threadReply}.`,
          `For a message containing ${dmPrompt}, send exactly ${dmReply}.`,
          'Send one visible message and no explanation.',
        ].join(' '),
      });
      await expect.poll(() => greetingCompleted(agent!.id), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toBe(true);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await sendMainMessage(page, channelPrompt);
      await expect.poll(() => messageID(channel!.id, channelPrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const channelTriggerID = messageID(channel.id, channelPrompt);
      await expect.poll(() => scopeState(channelTriggerID, channelReply), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        status: 'completed',
        channel_id: channel.id,
        thread_id: '',
        message_channel_id: channel.id,
        message_thread_id: '',
        replies: 1,
      });
      const channelResult = scopeState(channelTriggerID, channelReply);
      await expect(page.locator(`[data-message-id="${channelResult.message_id}"]`)).toContainText(new RegExp(channelReply, 'i'));

      const root = page.locator(`[data-message-id="${channelTriggerID}"]`);
      await root.hover();
      await root.getByRole('button', { name: /^Reply to / }).click();
      const threadComposer = page.getByLabel('Thread reply input');
      await threadComposer.fill(threadPrompt);
      await threadComposer.press('Enter');
      await expect(threadComposer).toHaveValue('');
      await expect(page.getByText(threadPrompt, { exact: true })).toBeVisible();
      await expect.poll(() => messageID(channel!.id, threadPrompt, true), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const threadTriggerID = messageID(channel.id, threadPrompt, true);
      await expect.poll(() => scopeState(threadTriggerID, threadReply), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        status: 'completed',
        channel_id: channel.id,
        thread_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        message_channel_id: channel.id,
        message_thread_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        replies: 1,
      });
      const threadResult = scopeState(threadTriggerID, threadReply);
      expect(threadResult.message_thread_id).toBe(threadResult.thread_id);
      await expect(page.locator(`[data-message-id="${threadResult.message_id}"]`)).toContainText(new RegExp(threadReply, 'i'));
      await page.getByRole('button', { name: 'Close thread panel' }).click();

      dm = await api<Entity>(request, auth.access_token, 'post', '/api/v1/dm', {
        member_type: 'agent',
        member_id: agent.id,
      });
      await page.goto(`/dashboard?dm=${dm.id}`);
      await sendMainMessage(page, dmPrompt);
      await expect.poll(() => messageID(dm!.id, dmPrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const dmTriggerID = messageID(dm.id, dmPrompt);
      await expect.poll(() => scopeState(dmTriggerID, dmReply), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        status: 'completed',
        channel_id: dm.id,
        thread_id: '',
        message_channel_id: dm.id,
        message_thread_id: '',
        replies: 1,
      });
      const dmResult = scopeState(dmTriggerID, dmReply);
      await expect(page.locator(`[data-message-id="${dmResult.message_id}"]`)).toContainText(new RegExp(dmReply, 'i'));
    } finally {
      if (dm) await api(request, auth.access_token, 'delete', `/api/v1/channels/${dm.id}`).catch(() => undefined);
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('queues distinct messages during the initial busy turn and continues normally', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const firstPrompt = `M8_BUSY_FIRST_${suffix}`;
    const firstReply = `M8_FIRST_ACK_${suffix.toUpperCase()}`;
    const secondPrompt = `M8_BUSY_SECOND_${suffix}`;
    const secondReply = `M8_SECOND_ACK_${suffix.toUpperCase()}`;
    const followupPrompt = `M8_CONTINUE_${suffix}`;
    const followupReply = `M8_CONTINUE_ACK_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `m8-busy-${suffix}`,
        description: 'M8 real busy Agent queue E2E',
      });
      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);

      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Busy${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'Always use solo message send with the exact target from the latest incoming message.',
          'When introducing yourself, first run the foreground Bash command sleep 20 and wait for it to finish, then send exactly M8_BUSY_READY.',
          `For a message containing ${firstPrompt}, send exactly ${firstReply}. If the send is HELD, the original goal is still unfinished: retry the same payload after reading the update.`,
          `For a message containing ${secondPrompt}, send exactly ${secondReply}.`,
          `For a message containing ${followupPrompt}, send exactly ${followupReply}.`,
          'For each incoming message, send one visible reply and no explanation.',
        ].join(' '),
      });
      await expect.poll(() => greetingActive(agent!.id), {
        timeout: 120_000, intervals: [250, 500, 1000],
      }).toBe(true);

      await sendMainMessage(page, firstPrompt);
      await sendMainMessage(page, secondPrompt);
      await expect.poll(() => [
        messageID(channel!.id, firstPrompt),
        messageID(channel!.id, secondPrompt),
      ], { timeout: 30_000, intervals: [250, 500, 1000] }).toEqual([
        expect.stringMatching(/^[0-9a-f-]{36}$/),
        expect.stringMatching(/^[0-9a-f-]{36}$/),
      ]);
      const triggerIDs = [messageID(channel.id, firstPrompt), messageID(channel.id, secondPrompt)];

      await expect.poll(() => {
        const state = busyState(agent!.id, triggerIDs, firstReply, secondReply);
        return { runs: state.runs, queued: state.queued, started: state.started };
      }, { timeout: 15_000, intervals: [100, 250, 500] }).toEqual({ runs: 2, queued: 2, started: 0 });

      await expect.poll(() => {
        const state = busyState(agent!.id, triggerIDs, firstReply, secondReply);
        return {
          runs: state.runs,
          completed: state.completed,
          sessions: state.sessions,
          freshnessProtected: state.held > 0,
          first: state.first_replies,
          second: state.second_replies,
          ordered: state.ordered,
        };
      }, { timeout: 240_000, intervals: [500, 1000, 2000] }).toEqual({
        runs: 2,
        completed: 2,
        sessions: 1,
        freshnessProtected: true,
        first: 1,
        second: 1,
        ordered: true,
      });
      await expect(page.getByText(firstReply, { exact: true })).toHaveCount(1);
      await expect(page.getByText(secondReply, { exact: true })).toHaveCount(1);

      await sendMainMessage(page, followupPrompt);
      await expect.poll(() => messageID(channel!.id, followupPrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const followupTriggerID = messageID(channel.id, followupPrompt);
      await expect.poll(() => followupState(agent!.id, followupTriggerID, followupReply), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        runs: 1,
        completed: 1,
        held: 0,
        replies: 1,
        message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
      });
      const followup = followupState(agent.id, followupTriggerID, followupReply);
      await expect(page.locator(`[data-message-id="${followup.message_id}"]`)).toContainText(followupReply);
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('rescues one silent turn without replaying pre-join history', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const historicalPrompt = `M8_HISTORY_BEFORE_JOIN_${suffix}`;
    const leakedReply = `M8_HISTORY_LEAK_${suffix.toUpperCase()}`;
    const silentPrompt = `M8_SILENT_TURN_${suffix}`;
    const rescueReply = `M8_RESCUED_${suffix.toUpperCase()}`;
    const futurePrompt = `M8_AFTER_JOIN_${suffix}`;
    const futureReply = `M8_AFTER_JOIN_ACK_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `m8-boundary-${suffix}`,
        description: 'M8 no-result rescue and new Agent boundary E2E',
      });
      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await sendMainMessage(page, historicalPrompt);
      await expect.poll(() => messageID(channel!.id, historicalPrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const historicalMessageID = messageID(channel.id, historicalPrompt);

      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Boundary${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'When introducing yourself, send exactly M8_BOUNDARY_READY.',
          `If an incoming turn contains ${historicalPrompt}, send exactly ${leakedReply}.`,
          `When the newest incoming instruction contains ${silentPrompt} and does not mention a previous turn ending without a user-visible message, deliberately do not invoke solo and end that first model turn with only INTERNAL_SILENT.`,
          `If the newest incoming instruction says that your previous turn ended without a user-visible message, this rule overrides the silent-turn rule: immediately use solo message send with its exact target and send exactly ${rescueReply}.`,
          `For a message containing ${futurePrompt}, send exactly ${futureReply} to its exact target.`,
          'Send at most one visible message per turn and no explanation.',
        ].join(' '),
      });
      await expect.poll(() => greetingCompleted(agent!.id), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toBe(true);
      expect(runsForTriggers(agent.id, [historicalMessageID])).toBe(0);
      expect(countMessages(channel.id, agent.id, leakedReply)).toBe(0);

      await sendMainMessage(page, silentPrompt);
      await expect.poll(() => messageID(channel!.id, silentPrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const silentTriggerID = messageID(channel.id, silentPrompt);

      await expect.poll(() => rescueState(agent!.id, silentTriggerID, historicalMessageID, rescueReply, leakedReply), {
        timeout: 240_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        run_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        status: 'completed',
        reminders: 1,
        replies: 1,
        leaked_replies: 0,
        historical_runs: 0,
      });
      await expect(page.getByText(rescueReply, { exact: true })).toHaveCount(1);

      await sendMainMessage(page, futurePrompt);
      await expect.poll(() => messageID(channel!.id, futurePrompt), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const futureTriggerID = messageID(channel.id, futurePrompt);
      await expect.poll(() => scopeState(futureTriggerID, futureReply), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        status: 'completed',
        channel_id: channel.id,
        replies: 1,
      });
      const future = scopeState(futureTriggerID, futureReply);
      await expect(page.locator(`[data-message-id="${future.message_id}"]`)).toContainText(futureReply);
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('isolates Agent send limits and stops only Agent-authored cascades', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const ratePrefix = `M8_RATE_${suffix}_`;
    const rateStart = `M8_RATE_START_${suffix}`;
    const rateRecoveryStart = `M8_RATE_RECOVERY_START_${suffix}`;
    const humanPrompt = `M8_HUMAN_BYPASS_${suffix}`;
    const humanReply = `M8_HUMAN_BYPASS_ACK_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let sender: Entity | null = null;
    let receiver: Entity | null = null;

    const proxySend = async (agentID: string, content: string) => request.post(`${daemonBase}/internal/daemon/proxy`, {
      data: { agent_id: agentID, action: 'message_send', channel_id: channel!.id, content },
    });

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `m8-safety-${suffix}`,
        description: 'M8 Agent rate and cascade safety E2E',
      });
      sender = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Sender${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'When introducing yourself, send exactly M8_SENDER_READY.',
          `For a human message containing ${rateStart} or ${rateRecoveryStart}, use Bash to run sleep 30, then finish without sending a visible message.`,
          'Otherwise send one visible message and no explanation.',
        ].join(' '),
      });
      receiver = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Receiver${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'When introducing yourself, send exactly M8_RECEIVER_READY.',
          `For an Agent message containing ${ratePrefix}, send exactly M8_CASCADE_ACK_${suffix.toUpperCase()} to the exact target; if the send is HELD, do not retry.`,
          `For a human message containing ${humanPrompt}, send exactly ${humanReply} to the exact target.`,
          'Send at most one visible message and no explanation.',
        ].join(' '),
      });
      await expect.poll(() => greetingCompleted(sender!.id) && greetingCompleted(receiver!.id), {
        timeout: 240_000, intervals: [500, 1000, 2000],
      }).toBe(true);
      await new Promise((resolveDelay) => setTimeout(resolveDelay, 3200));

      const start = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { content: `@${sender.name} ${rateStart}` },
      });
      expect(start.status(), await start.text()).toBe(201);
      await expect.poll(() => agentRunActive(sender!.id), {
        timeout: 60_000, intervals: [250, 500, 1000],
      }).toBe(true);

      const targetedMessages = [1, 2, 3].map((n) => `${ratePrefix}C${n} @${receiver!.name}`);
      for (const content of targetedMessages) {
        const response = await proxySend(sender.id, content);
        expect(response.status(), await response.text()).toBe(201);
      }
      for (const n of [4, 5]) {
        const response = await proxySend(sender.id, `${ratePrefix}PLAIN${n}`);
        expect(response.status(), await response.text()).toBe(201);
      }
      const rejected = await proxySend(sender.id, `${ratePrefix}REJECTED`);
      expect(rejected.status()).toBe(429);
      expect(Number(rejected.headers()['retry-after'])).toBeGreaterThan(0);

      await expect.poll(() => agentRunActive(receiver!.id), {
        timeout: 60_000, intervals: [250, 500, 1000],
      }).toBe(true);
      let isolated = await proxySend(receiver.id, `${ratePrefix}OTHER_AGENT_OK`);
      if (isolated.status() === 200) {
        isolated = await proxySend(receiver.id, `${ratePrefix}OTHER_AGENT_OK`);
      }
      expect(isolated.status(), await isolated.text()).toBe(201);

      await expect.poll(() => targetedMessages.map((content) => messageID(channel!.id, content)), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).toEqual(targetedMessages.map(() => expect.stringMatching(/^[0-9a-f-]{36}$/)));
      const targetedIDs = targetedMessages.map((content) => messageID(channel!.id, content));
      await expect.poll(() => runsForTriggers(receiver!.id, targetedIDs), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).toBe(2);
      await expect.poll(() => runsForTriggers(receiver!.id, targetedIDs, 'completed'), {
        timeout: 240_000, intervals: [500, 1000, 2000],
      }).toBe(2);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await sendMainMessage(page, `@${receiver.name} ${humanPrompt}`);
      const humanTriggerID = messageID(channel.id, `@${receiver.name} ${humanPrompt}`);
      await expect.poll(() => scopeState(humanTriggerID, humanReply), {
        timeout: 240_000, intervals: [500, 1000, 2000],
      }).toMatchObject({ status: 'completed', replies: 1 });
      const humanResult = scopeState(humanTriggerID, humanReply);
      await expect(page.locator(`[data-message-id="${humanResult.message_id}"]`)).toContainText(humanReply);

      await new Promise((resolveDelay) => setTimeout(resolveDelay, 3200));
      const recoveryStart = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { content: `@${sender.name} ${rateRecoveryStart}` },
      });
      expect(recoveryStart.status(), await recoveryStart.text()).toBe(201);
      await expect.poll(() => agentRunActive(sender!.id), {
        timeout: 60_000, intervals: [250, 500, 1000],
      }).toBe(true);
      let recovered = await proxySend(sender.id, `${ratePrefix}RECOVERED`);
      if (recovered.status() === 200) {
        recovered = await proxySend(sender.id, `${ratePrefix}RECOVERED`);
      }
      expect(recovered.status(), await recovered.text()).toBe(201);
      expect(countMessages(channel.id, sender.id, ratePrefix)).toBe(6);
      expect(countMessages(channel.id, receiver.id, ratePrefix)).toBe(1);
    } finally {
      if (receiver) await api(request, auth.access_token, 'delete', `/api/v1/agents/${receiver.id}`).catch(() => undefined);
      if (sender) await api(request, auth.access_token, 'delete', `/api/v1/agents/${sender.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('delegates a Task to one Agent and wakes its Agent creator for review', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const taskTitle = `M8_DELEGATE_${suffix}`;
    const missingTitle = `M8_MISSING_ASSIGNEE_${suffix}`;
    const delegationStart = `M8_DELEGATION_START_${suffix}`;
    const workerReply = `M8_WORKER_DONE_${suffix.toUpperCase()}`;
    const creatorReply = `M8_CREATOR_NOTIFIED_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let creator: Entity | null = null;
    let worker: Entity | null = null;
    let notificationDMID = '';

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `m8-delegation-${suffix}`,
        description: 'M8 explicit Task delegation E2E',
      });
      creator = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Creator${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'When introducing yourself, send exactly M8_CREATOR_READY.',
          `For a human message containing ${delegationStart}, run solo task create in channel ${channel.id} with title ${taskTitle}, description "Complete this exact E2E task and submit it for review.", and assignee ${`M8Worker${suffix}`}. Do not send a visible message for that request.`,
          `When a system message contains ${taskTitle} and the words ready for review, send exactly ${creatorReply} to its exact target.`,
          'Send one visible message and no explanation.',
        ].join(' '),
      });
      worker = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `M8Worker${suffix}`,
        computer_id: computerID,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'When introducing yourself, send exactly M8_WORKER_READY.',
          `When assigned a Task containing ${taskTitle}, send exactly ${workerReply} to the exact task thread target, then run solo task submit with that Task number and channel ${channel.id}.`,
          'Do not claim or modify any other Task. Send one visible message and no explanation.',
        ].join(' '),
      });
      await expect.poll(() => greetingCompleted(creator!.id) && greetingCompleted(worker!.id), {
        timeout: 240_000, intervals: [500, 1000, 2000],
      }).toBe(true);

      const missing = await request.post(`${apiBase}/api/v1/channels/${channel.id}/tasks`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { title: missingTitle, assignee: `missing-${suffix}` },
      });
      expect(missing.status()).toBe(404);
      expect(countTasks(channel.id, missingTitle)).toBe(0);

      const delegated = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { content: `@${creator.name} ${delegationStart}` },
      });
      expect(delegated.status(), await delegated.text()).toBe(201);

      await expect.poll(() => delegationState(taskTitle, creator!.id, worker!.id, workerReply, creatorReply), {
        timeout: 300_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        task_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        task_number: expect.any(Number),
        status: 'in_review',
        creator_id: creator.id,
        claimer_id: worker.id,
        message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        thread_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        worker_replies: 1,
        worker_reply_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        creator_dm_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        creator_reply_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        creator_run_status: 'completed',
      });
      const state = delegationState(taskTitle, creator.id, worker.id, workerReply, creatorReply);
      notificationDMID = state.creator_dm_id;

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      const taskRoot = page.locator(`[data-message-id="${state.message_id}"]`);
      await taskRoot.hover();
      await taskRoot.getByRole('button', { name: /^Reply to / }).click();
      await expect(page.locator(`[data-message-id="${state.worker_reply_id}"]`)).toContainText(workerReply);
      await page.getByRole('button', { name: 'Close thread panel' }).click();

      await page.goto(`/dashboard?dm=${notificationDMID}`);
      await expect(page.locator(`[data-message-id="${state.creator_reply_id}"]`)).toContainText(creatorReply);
    } finally {
      if (notificationDMID) await api(request, auth.access_token, 'delete', `/api/v1/channels/${notificationDMID}`).catch(() => undefined);
      if (worker) await api(request, auth.access_token, 'delete', `/api/v1/agents/${worker.id}`).catch(() => undefined);
      if (creator) await api(request, auth.access_token, 'delete', `/api/v1/agents/${creator.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });
});
