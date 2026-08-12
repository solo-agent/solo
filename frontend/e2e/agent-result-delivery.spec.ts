import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'agent-result-delivery-human-e2e@solo.local', password: 'SoloE2E-2026!' };
const daemonLogPath = join(process.cwd(), '..', 'daemon.log');

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface TaskEntity {
  id: string;
  task_number: number;
  title: string;
}

interface DeliveryState {
  run_id: string;
  daemon_id: string;
  bound_daemon_id: string;
  status: string;
  contract: string;
  message_id: string;
  visible_event: boolean;
  input_tokens: number;
  output_tokens: number;
  failure_code: string;
  external_session_id: string;
}

interface RecoveryState {
  run_id: string;
  status: string;
  backend_started: boolean;
  failure_code: string;
  retryable: boolean;
  message_id: string;
}

interface ChannelSessionState {
  run_id: string;
  status: string;
  external_session_id: string;
  message_id: string;
  message_content: string;
}

interface AgentHeartbeatState {
  agent_active: boolean;
  computer_online: boolean;
  heartbeat_has_agent: boolean;
  heartbeat_epoch: number;
}

interface TaskRetryState {
  run_id: string;
  status: string;
  claimer_id: string;
  attempts: number;
  failed_attempts: number;
  latest_status: string;
  latest_agent_id: string;
  latest_message_id: string;
  failure_code: string;
  recovery_previous_run_id: string;
  recovery_mode: string;
  workspace_reused: boolean;
  recovery_scheduled_events: number;
  recovery_blocked_events: number;
  retry_messages: number;
  exhausted_messages: number;
}

interface RouterState {
  runs: number;
  lead_runs: number;
  worker_runs: number;
  completed: number;
  lead_message_id: string;
  worker_replies: number;
  unresolved_runs: number;
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

function deliveryState(agentID: string): DeliveryState {
  return databaseJSON<DeliveryState>(`
    WITH latest AS (
      SELECT *
        FROM agent_runs
       WHERE agent_id = '${agentID}'
       ORDER BY started_at DESC
       LIMIT 1
    )
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'daemon_id', COALESCE(r.daemon_id, ''),
        'bound_daemon_id', COALESCE((
          SELECT c.daemon_id
            FROM agents a
            JOIN computers c ON c.id::text = a.runtime_id
           WHERE a.id = r.agent_id
        ), ''),
        'status', r.status,
        'contract', COALESCE((
          SELECT e.payload->>'result_contract'
            FROM agent_run_events e
           WHERE e.run_id = r.id AND e.type = 'run_started'
           ORDER BY e.seq
           LIMIT 1
        ), ''),
        'message_id', COALESCE((
          SELECT m.id::text
            FROM messages m
           WHERE m.metadata->>'agent_run_id' = r.id::text
           ORDER BY m.created_at
           LIMIT 1
        ), ''),
        'visible_event', EXISTS(
          SELECT 1 FROM agent_run_events e
           WHERE e.run_id = r.id AND e.type = 'visible_message_sent'
        ),
        'input_tokens', COALESCE((r.usage_json->>'input_tokens')::int, 0),
        'output_tokens', COALESCE((r.usage_json->>'output_tokens')::int, 0),
        'failure_code', COALESCE((
          SELECT e.payload->>'failure_code'
            FROM agent_run_events e
           WHERE e.run_id = r.id AND e.type = 'error'
           ORDER BY e.seq DESC
           LIMIT 1
        ), ''),
        'external_session_id', COALESCE((
          SELECT s.external_session_id
            FROM agent_sessions s
           WHERE s.id = r.session_id
        ), '')
      )::text
      FROM latest r
    ), '{"run_id":"","daemon_id":"","bound_daemon_id":"","status":"","contract":"","message_id":"","visible_event":false,"input_tokens":0,"output_tokens":0,"failure_code":"","external_session_id":""}')
  `);
}

function recoveryState(agentID: string): RecoveryState {
  return databaseJSON<RecoveryState>(`
    WITH latest AS (
      SELECT *
        FROM agent_runs
       WHERE agent_id = '${agentID}'
       ORDER BY started_at DESC
       LIMIT 1
    )
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'status', r.status,
        'backend_started', r.backend_started_at IS NOT NULL,
        'failure_code', COALESCE((
          SELECT e.payload->>'failure_code'
            FROM agent_run_events e
           WHERE e.run_id = r.id AND e.type = 'error'
           ORDER BY e.seq DESC
           LIMIT 1
        ), ''),
        'retryable', COALESCE((
          SELECT (e.payload->>'retryable')::boolean
            FROM agent_run_events e
           WHERE e.run_id = r.id AND e.type = 'error'
           ORDER BY e.seq DESC
           LIMIT 1
        ), false),
        'message_id', COALESCE((
          SELECT m.id::text
            FROM messages m
           WHERE m.metadata->>'agent_run_id' = r.id::text
           LIMIT 1
        ), '')
      )::text
      FROM latest r
    ), '{"run_id":"","status":"","backend_started":false,"failure_code":"","retryable":false,"message_id":""}')
  `);
}

function channelSessionState(triggerMessageID: string): ChannelSessionState {
  return databaseJSON<ChannelSessionState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'status', r.status,
        'external_session_id', COALESCE(s.external_session_id, ''),
        'message_id', COALESCE(m.id::text, ''),
        'message_content', COALESCE(m.content, '')
      )::text
        FROM agent_runs r
        LEFT JOIN agent_sessions s ON s.id = r.session_id
        LEFT JOIN LATERAL (
          SELECT id, content
            FROM messages
           WHERE metadata->>'agent_run_id' = r.id::text
           ORDER BY created_at
           LIMIT 1
        ) m ON true
       WHERE r.trigger_message_id = '${triggerMessageID}'
       ORDER BY r.started_at DESC
       LIMIT 1
    ), '{"run_id":"","status":"","external_session_id":"","message_id":"","message_content":""}')
  `);
}

function triggerMessageID(channelID: string, content: string): string {
  return databaseJSON<{ id: string }>(`
    SELECT json_build_object('id', COALESCE((
      SELECT id::text
        FROM messages
       WHERE channel_id = '${channelID}'
         AND sender_type = 'user'
         AND content = '${content}'
       ORDER BY created_at DESC
       LIMIT 1
    ), ''))::text
  `).id;
}

function agentHeartbeatState(agentID: string): AgentHeartbeatState {
  return databaseJSON<AgentHeartbeatState>(`
    SELECT json_build_object(
      'agent_active', a.is_active,
      'computer_online', EXISTS(SELECT 1 FROM computers c WHERE c.status = 'online'),
      'heartbeat_has_agent', EXISTS(
        SELECT 1 FROM computers c
         WHERE c.status = 'online'
           AND a.id = ANY(COALESCE(c.agent_ids, '{}'::uuid[]))
      ),
      'heartbeat_epoch', COALESCE((
        SELECT MAX(EXTRACT(EPOCH FROM c.last_heartbeat))
          FROM computers c
         WHERE c.status = 'online'
           AND a.id = ANY(COALESCE(c.agent_ids, '{}'::uuid[]))
      ), 0)
    )::text
      FROM agents a
     WHERE a.id = '${agentID}'
  `);
}

function logLinesAfterRun(runID: string): string[] {
  const lines = readFileSync(daemonLogPath, 'utf8').split('\n');
  const completed = lines.findLastIndex((line) =>
    line.includes('"msg":"task backend completed"') && line.includes(`"task_id":"${runID}"`));
  return completed < 0 ? [] : lines.slice(completed + 1);
}

async function expectAgentSessionSleptAfterRun(runID: string, sessionKey: string, providerSessionID: string) {
  await expect.poll(() => {
    const lines = logLinesAfterRun(runID);
    return lines.some((line) =>
      line.includes('"msg":"session: sleeping idle Agent process"')
      && line.includes(`"session_key":"${sessionKey}"`))
      && lines.some((line) =>
        line.includes('"msg":"claude: persistent session closed"')
        && line.includes(`"session_id":"${providerSessionID}"`));
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

async function expectAgentSessionResumed(logOffset: number, sessionKey: string, providerSessionID: string) {
  await expect.poll(() => {
    const log = readFileSync(daemonLogPath, 'utf8').slice(logOffset);
    return log.split('\n').some((line) =>
      line.includes('"msg":"session: creating"')
      && line.includes(`"session_key":"${sessionKey}"`)
      && line.includes(`"resume":"${providerSessionID}"`));
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

function taskRetryState(taskID: string): TaskRetryState {
  return databaseJSON<TaskRetryState>(`
    WITH linked AS (
      SELECT r.*
        FROM agent_runs r
        JOIN agent_run_task_links link ON link.run_id = r.id AND link.role = 'primary'
       WHERE link.task_id = '${taskID}'
         AND (
           SELECT e.payload->>'result_contract'
             FROM agent_run_events e
            WHERE e.run_id = r.id AND e.type = 'run_started'
            ORDER BY e.seq
            LIMIT 1
         ) = 'visible_message'
    ), latest AS (
      SELECT * FROM linked ORDER BY started_at DESC, id DESC LIMIT 1
    )
    SELECT json_build_object(
	  'run_id', COALESCE((SELECT id::text FROM latest), ''),
      'status', t.status,
      'claimer_id', COALESCE(t.claimer_id::text, ''),
      'attempts', (SELECT COUNT(*) FROM linked),
      'failed_attempts', (SELECT COUNT(*) FROM linked WHERE status IN ('failed', 'timeout')),
      'latest_status', COALESCE((SELECT status FROM latest), ''),
      'latest_agent_id', COALESCE((SELECT agent_id::text FROM latest), ''),
      'latest_message_id', COALESCE((
        SELECT m.id::text
          FROM messages m
         WHERE m.metadata->>'agent_run_id' = (SELECT id::text FROM latest)
         LIMIT 1
      ), ''),
      'failure_code', COALESCE((
        SELECT e.payload->>'failure_code'
          FROM agent_run_events e
         WHERE e.run_id = (SELECT id FROM latest) AND e.type = 'error'
         ORDER BY e.seq DESC LIMIT 1
      ), ''),
      'recovery_previous_run_id', COALESCE((
        SELECT e.payload->'recovery'->>'previous_run_id'
          FROM agent_run_events e
         WHERE e.run_id = (SELECT id FROM latest) AND e.type = 'run_started'
         ORDER BY e.seq LIMIT 1
      ), ''),
      'recovery_mode', COALESCE((
        SELECT e.payload->'recovery'->>'mode'
          FROM agent_run_events e
         WHERE e.run_id = (SELECT id FROM latest) AND e.type = 'run_started'
         ORDER BY e.seq LIMIT 1
      ), ''),
      'workspace_reused', COALESCE((
        SELECT (e.payload->'recovery'->>'workspace_reused')::boolean
          FROM agent_run_events e
         WHERE e.run_id = (SELECT id FROM latest) AND e.type = 'run_started'
         ORDER BY e.seq LIMIT 1
      ), false),
      'recovery_scheduled_events', (
        SELECT COUNT(*) FROM agent_run_events e
         WHERE e.run_id IN (SELECT id FROM linked) AND e.type = 'task_recovery_scheduled'
      ),
      'recovery_blocked_events', (
        SELECT COUNT(*) FROM agent_run_events e
         WHERE e.run_id IN (SELECT id FROM linked) AND e.type = 'task_recovery_blocked'
      ),
      'retry_messages', (
        SELECT COUNT(*) FROM messages m
         WHERE m.metadata->>'task_id' = t.id::text
           AND m.metadata->>'auto_recovery' = 'true'
           AND m.metadata->>'exhausted' = 'false'
      ),
      'exhausted_messages', (
        SELECT COUNT(*) FROM messages m
         WHERE m.metadata->>'task_id' = t.id::text
           AND m.metadata->>'exhausted' = 'true'
      )
    )::text
    FROM tasks t
    WHERE t.id = '${taskID}'
  `);
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Human E2E Tester' },
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

function rebuildIsolatedE2EStack() {
  const daemonID = process.env.SOLO_E2E_DAEMON_ID?.trim();
  const credentialFile = process.env.SOLO_DAEMON_CREDENTIAL_FILE?.trim();
  if (!daemonID?.startsWith('daemon-e2e-') || !credentialFile) {
    throw new Error('isolated E2E Daemon environment is required before restarting the stack');
  }
  execFileSync('make', [
    'rebuild',
    `DAEMON_SERVER_URL=${apiBase}`,
    `DAEMON_ID=${daemonID}`,
    `SOLO_DAEMON_CREDENTIAL_FILE=${credentialFile}`,
    'SOLO_COMPUTER_ID=',
    'SOLO_COMPUTER_CREDENTIAL=',
    'SOLO_ENROLLMENT_TOKEN=',
  ], { cwd: '..', stdio: 'inherit' });
}

test.describe('real Agent result delivery contract', () => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_DELIVERY !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(240000);
  let localComputer: LocalComputerLease;

  test.beforeAll(async ({ request }) => {
    const auth = await authenticate(request);
    localComputer = await acquireLocalComputer(request, apiBase, auth.access_token);
  });

  test.afterAll(async ({ request }) => {
    await localComputer?.release(request);
  });

  test('persists a run-linked result and reuses the declared persistent runtime', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `delivery-e2e-${suffix}`,
        description: 'Real Agent visible delivery contract E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Delivery E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'Always use solo message send for visible replies. When asked to introduce yourself, post the introduction. When the user says SECOND, send exactly SECOND_OK. Do not merely print replies.',
      });

      await expect.poll(() => {
        const state = deliveryState(agent!.id);
        return `${state.status}/${state.contract}/${Boolean(state.message_id)}/${state.visible_event}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/visible_message/true/true');

      const state = deliveryState(agent.id);
      expect(state.input_tokens + state.output_tokens).toBeGreaterThan(0);
      expect(state.external_session_id).not.toBe('');
      expect(state.daemon_id).not.toBe('');
      expect(state.daemon_id).toBe(state.bound_daemon_id);

      const secondMessage = await api<{ id: string }>(
        request,
        auth.access_token,
        'post',
        `/api/v1/channels/${channel.id}/messages`,
        { content: 'SECOND' },
      );
      await expect.poll(() => {
        const second = channelSessionState(secondMessage.id);
        return `${second.status}/${second.external_session_id}/${second.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(
        `completed/${state.external_session_id}/SECOND_OK`,
      );
      const second = channelSessionState(secondMessage.id);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      const persistedMessage = page.locator(`[data-message-id="${state.message_id}"]`);
      await expect(persistedMessage).toBeVisible();
      await expect(page.locator(`[data-message-id="${second.message_id}"]`)).toContainText('SECOND_OK');
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('lists templates through the current Run across a persistent Agent Session', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `template-proxy-e2e-${suffix}`,
        description: 'Persistent Agent template credential E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Template Proxy E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'Always deliver replies with solo message send.',
          'When introducing yourself, run solo template list --json first. If it succeeds, send exactly TEMPLATE_LIST_FIRST_OK.',
          'When the user says LIST_AGAIN, run solo template list --json again. If it succeeds, send exactly TEMPLATE_LIST_SECOND_OK.',
          'Do not guess or memorize the catalog. Do not send an OK result unless the command returned a non-empty JSON template array.',
        ].join(' '),
      });

      await expect.poll(() => {
        const state = deliveryState(agent!.id);
        return `${state.status}/${state.message_id ? 'visible' : 'missing'}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/visible');
      const first = deliveryState(agent.id);
      expect(first.external_session_id).not.toBe('');

      const secondTrigger = await api<{ id: string }>(
        request,
        auth.access_token,
        'post',
        `/api/v1/channels/${channel.id}/messages`,
        { content: 'LIST_AGAIN' },
      );
      await expect.poll(() => {
        const state = channelSessionState(secondTrigger.id);
        return `${state.status}/${state.external_session_id}/${state.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(
        `completed/${first.external_session_id}/TEMPLATE_LIST_SECOND_OK`,
      );

      const second = channelSessionState(secondTrigger.id);
      const persisted = databaseJSON<{
        runs: number;
        completed: number;
        sessions: number;
        visible_messages: number;
        transcript_path: string;
      }>(`
        SELECT json_build_object(
          'runs', COUNT(*),
          'completed', COUNT(*) FILTER (WHERE r.status = 'completed'),
          'sessions', COUNT(DISTINCT r.session_id),
          'visible_messages', COUNT(*) FILTER (WHERE EXISTS (
            SELECT 1 FROM messages m WHERE m.metadata->>'agent_run_id' = r.id::text
          )),
          'transcript_path', COALESCE(MAX(r.transcript_path), '')
        )::text
          FROM agent_runs r
         WHERE r.agent_id = '${agent.id}'
      `);
      expect(persisted).toMatchObject({ runs: 2, completed: 2, sessions: 1, visible_messages: 2 });
      expect(persisted.transcript_path).not.toBe('');

      const transcriptEntries = readFileSync(persisted.transcript_path, 'utf8')
        .trim()
        .split('\n')
        .map((line) => JSON.parse(line) as {
          message?: { content?: Array<{
            type?: string;
            id?: string;
            tool_use_id?: string;
            is_error?: boolean;
            content?: string;
            name?: string;
            input?: { command?: string };
          }> };
        });
      const templateToolIDs = new Set(transcriptEntries.flatMap((entry) => (
        entry.message?.content ?? []
      )).filter((content) => (
        content.type === 'tool_use'
        && content.name === 'Bash'
        && content.input?.command === 'solo template list --json'
        && content.id
      )).map((content) => content.id!));
      const successfulTemplateResults = transcriptEntries.flatMap((entry) => (
        entry.message?.content ?? []
      )).filter((content) => {
        if (content.type !== 'tool_result' || !content.tool_use_id || content.is_error) return false;
        if (!templateToolIDs.has(content.tool_use_id) || typeof content.content !== 'string') return false;
        try {
          const templates = JSON.parse(content.content) as unknown;
          return Array.isArray(templates) && templates.length > 0;
        } catch {
          return false;
        }
      });
      expect(templateToolIDs.size).toBe(2);
      expect(successfulTemplateResults).toHaveLength(2);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.locator(`[data-message-id="${first.message_id}"]`)).toContainText('TEMPLATE_LIST_FIRST_OK');
      await expect(page.locator(`[data-message-id="${second.message_id}"]`)).toContainText('TEMPLATE_LIST_SECOND_OK');
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('routes an unmentioned Channel message only to the unique Coordinator', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const leadAck = `ROUTER_LEAD_ACK_${suffix.toUpperCase()}`;
    const workerAck = `ROUTER_WORKER_ACK_${suffix.toUpperCase()}`;
    const unresolvedContent = `@MissingRouter${suffix} SHOULD_NOT_WAKE`;
    const routedContent = `ROUTER_E2E_${suffix}`;
    let channel: Entity | null = null;
    let lead: Entity | null = null;
    let worker: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `scope-router-e2e-${suffix}`,
        description: 'Real Agent scope router E2E',
      });
      lead = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `RouterLead${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: `When introducing yourself, use solo message send to send exactly LEAD_READY. For a human message beginning ROUTER_E2E_, use solo message send to send exactly ${leadAck}. Send no other visible text.`,
      });
      worker = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `RouterWorker${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: `When introducing yourself, use solo message send to send exactly WORKER_READY. For a human message beginning ROUTER_E2E_, use solo message send to send exactly ${workerAck}. Send no other visible text.`,
      });
      await api(request, auth.access_token, 'post', '/api/v1/agent-relationships', {
        from_agent_id: lead.id,
        to_agent_id: worker.id,
        rel_type: 'assigns_to',
      });

      await expect.poll(() => databaseJSON<{ done: boolean }>(`
        SELECT json_build_object('done',
          COUNT(DISTINCT agent_id) FILTER (WHERE status = 'completed') = 2
          AND COUNT(*) FILTER (WHERE status IN ('queued','thinking','running','streaming','waiting_input','waiting_approval')) = 0
        )::text
          FROM agent_runs
         WHERE agent_id IN ('${lead!.id}', '${worker!.id}')
      `).done, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(true);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      const composer = page.getByPlaceholder('Type a message...');
      await composer.fill(unresolvedContent);
      await composer.press('Enter');
      await expect(page.getByLabel('Message list').getByText(unresolvedContent, { exact: true })).toBeVisible();
      await composer.fill(routedContent);
      await composer.press('Enter');

      await expect.poll(() => databaseJSON<{ unresolved: string; routed: string }>(`
        SELECT json_build_object(
          'unresolved', COALESCE((SELECT id::text FROM messages WHERE channel_id = '${channel!.id}' AND content = '${unresolvedContent}' LIMIT 1), ''),
          'routed', COALESCE((SELECT id::text FROM messages WHERE channel_id = '${channel!.id}' AND content = '${routedContent}' LIMIT 1), '')
        )::text
      `), { intervals: [250, 500, 1000] }).toMatchObject({
        unresolved: expect.stringMatching(/^[0-9a-f-]{36}$/),
        routed: expect.stringMatching(/^[0-9a-f-]{36}$/),
      });
      const triggerIDs = databaseJSON<{ unresolved: string; routed: string }>(`
        SELECT json_build_object(
          'unresolved', COALESCE((SELECT id::text FROM messages WHERE channel_id = '${channel.id}' AND content = '${unresolvedContent}' LIMIT 1), ''),
          'routed', COALESCE((SELECT id::text FROM messages WHERE channel_id = '${channel.id}' AND content = '${routedContent}' LIMIT 1), '')
        )::text
      `);

      const readRouterState = () => databaseJSON<RouterState>(`
        SELECT json_build_object(
          'runs', COUNT(*) FILTER (WHERE trigger_message_id = '${triggerIDs.routed}'),
          'lead_runs', COUNT(*) FILTER (WHERE trigger_message_id = '${triggerIDs.routed}' AND agent_id = '${lead.id}'),
          'worker_runs', COUNT(*) FILTER (WHERE trigger_message_id = '${triggerIDs.routed}' AND agent_id = '${worker.id}'),
          'completed', COUNT(*) FILTER (WHERE trigger_message_id = '${triggerIDs.routed}' AND status = 'completed'),
          'lead_message_id', COALESCE((SELECT id::text FROM messages WHERE channel_id = '${channel.id}' AND sender_id = '${lead.id}' AND content = '${leadAck}' ORDER BY created_at DESC LIMIT 1), ''),
          'worker_replies', (SELECT COUNT(*) FROM messages WHERE channel_id = '${channel.id}' AND sender_id = '${worker.id}' AND content = '${workerAck}'),
          'unresolved_runs', COUNT(*) FILTER (WHERE trigger_message_id = '${triggerIDs.unresolved}')
        )::text
          FROM agent_runs
         WHERE agent_id IN ('${lead.id}', '${worker.id}')
      `);
      await expect.poll(readRouterState, { timeout: 180000, intervals: [500, 1000, 2000] }).toMatchObject({
        runs: 1,
        lead_runs: 1,
        worker_runs: 0,
        completed: 1,
        lead_message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        worker_replies: 0,
        unresolved_runs: 0,
      });

      const state = readRouterState();
      await expect(page.locator(`[data-message-id="${state.lead_message_id}"]`)).toContainText(leadAck);
      await expect(page.getByLabel('Message list').getByText(workerAck, { exact: true })).toHaveCount(0);
    } finally {
      if (worker) await api(request, auth.access_token, 'delete', `/api/v1/agents/${worker.id}`).catch(() => undefined);
      if (lead) await api(request, auth.access_token, 'delete', `/api/v1/agents/${lead.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('fails a completed backend that did not persist a visible result', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `missing-delivery-e2e-${suffix}`,
        description: 'Real Agent missing delivery contract E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Missing Delivery E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        custom_args: ['--tools', ''],
        system_prompt: 'Do not use tools. Respond with exactly INTERNAL_ONLY as plain text and stop.',
      });

      await expect.poll(() => {
        const state = deliveryState(agent!.id);
        return `${state.status}/${state.failure_code}/${Boolean(state.message_id)}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('failed/missing_visible_result/false');

      await authenticatePage(page, auth);
      await page.goto('/observability/live');
      const card = page.locator('button').filter({ hasText: agent.name }).first();
      await expect(card).toBeVisible();
      await expect(card).toContainText('Failed');
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('converges an interrupted real Agent run after the make-managed stack restarts', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `daemon-recovery-e2e-${suffix}`,
        description: 'Real daemon restart recovery E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Daemon Recovery E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'Before introducing yourself, you must run the Bash command `sleep 60` and wait for it to finish. Only then use solo message send. Do not skip the wait.',
      });

      await expect.poll(() => {
        const state = recoveryState(agent!.id);
        return Boolean(state.run_id) && state.backend_started
          && ['running', 'thinking', 'streaming', 'waiting_input', 'waiting_approval'].includes(state.status);
      }, { timeout: 120000, intervals: [500, 1000, 2000] }).toBe(true);
      const interruptedRunID = recoveryState(agent.id).run_id;

      rebuildIsolatedE2EStack();

      await expect.poll(() => {
        const state = recoveryState(agent!.id);
        return `${state.run_id}/${state.status}/${state.failure_code}/${state.retryable}/${Boolean(state.message_id)}`;
      }, { timeout: 60000, intervals: [500, 1000, 2000] }).toBe(`${interruptedRunID}/failed/daemon_lost/true/false`);

      await authenticatePage(page, auth);
      await page.goto('/observability/live');
      const card = page.locator('button').filter({ hasText: agent.name }).first();
      await expect(card).toBeVisible();
      await expect(card).toContainText('Stopped because the local daemon restarted');
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('resumes the same real Channel provider Session after the stack restarts', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const secret = `CHANNEL_MEMORY_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `channel-session-resume-e2e-${suffix}`,
        description: 'Real Channel provider Session restart continuity E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Channel Session Resume E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'Always deliver replies with solo message send.',
          'When introducing yourself, send exactly READY.',
          'When a user says REMEMBER followed by a value, retain it only in the current conversation and send exactly STORED.',
          'When a user says RECALL, send exactly RECALLED followed by the remembered value.',
          'Never use message read, files, MEMORY.md, databases, or other storage for REMEMBER or RECALL.',
        ].join(' '),
      });

      await expect.poll(() => deliveryState(agent!.id).status, {
        timeout: 180000, intervals: [500, 1000, 2000],
      }).toBe('completed');

      const rememberMessage = await api<{ id: string }>(
        request,
        auth.access_token,
        'post',
        `/api/v1/channels/${channel.id}/messages`,
        { content: `REMEMBER ${secret}` },
      );
      await expect.poll(() => {
        const state = channelSessionState(rememberMessage.id);
        return `${state.status}/${Boolean(state.external_session_id)}/${state.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/true/STORED');
      const first = channelSessionState(rememberMessage.id);

      rebuildIsolatedE2EStack();

      const recallMessage = await api<{ id: string }>(
        request,
        auth.access_token,
        'post',
        `/api/v1/channels/${channel.id}/messages`,
        { content: 'RECALL' },
      );
      await expect.poll(() => {
        const state = channelSessionState(recallMessage.id);
        return `${state.status}/${state.external_session_id}/${state.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(
        `completed/${first.external_session_id}/RECALLED ${secret}`,
      );

      const recalled = channelSessionState(recallMessage.id);
      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.locator(`[data-message-id="${recalled.message_id}"]`)).toContainText(`RECALLED ${secret}`);
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('sleeps an idle Channel Agent and resumes the same real provider Session', async ({ page, request }) => {
    test.skip(process.env.SOLO_E2E_EXPECT_AGENT_IDLE_REAPER !== '1', 'requires a short-TTL daemon started through make rebuild');
    test.setTimeout(360000);

    const auth = await authenticate(request);
    const suffix = `idle-${Date.now().toString(36)}`;
    const secret = `IDLE_MEMORY_${suffix.toUpperCase()}`;
    const rememberContent = `REMEMBER ${secret}`;
    const recallContent = `RECALL ${suffix}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `channel-idle-resume-e2e-${suffix}`,
        description: 'Real Channel Agent idle sleep and wake E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Channel Idle Resume E2E ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'Always deliver replies with solo message send.',
          'When introducing yourself, send exactly READY.',
          'When a user message starts with REMEMBER followed by a value, retain it only in the current provider conversation and send exactly STORED.',
          'When a user message starts with RECALL, send exactly RECALLED followed by the remembered value.',
          'Never use message read, files, MEMORY.md, databases, or any other storage for REMEMBER or RECALL.',
        ].join(' '),
      });

      await expect.poll(() => deliveryState(agent!.id).status, {
        timeout: 180000, intervals: [500, 1000, 2000],
      }).toBe('completed');

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      const composer = page.getByPlaceholder('Type a message...');
      await expect(composer).toBeVisible();
      await composer.fill(rememberContent);
      await composer.press('Enter');

      await expect.poll(() => triggerMessageID(channel!.id, rememberContent), {
        timeout: 30000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const rememberMessageID = triggerMessageID(channel.id, rememberContent);
      await expect.poll(() => {
        const state = channelSessionState(rememberMessageID);
        return `${state.status}/${Boolean(state.external_session_id)}/${state.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/true/STORED');
      const first = channelSessionState(rememberMessageID);
      await expect(page.locator(`[data-message-id="${first.message_id}"]`)).toContainText('STORED');

      await expect.poll(() => {
        const state = agentHeartbeatState(agent!.id);
        return `${state.agent_active}/${state.computer_online}/${state.heartbeat_has_agent}`;
      }, { timeout: 45000, intervals: [1000, 2000] }).toBe('true/true/true');
      const heartbeatBeforeSleep = agentHeartbeatState(agent.id).heartbeat_epoch;
      const sessionKey = `channel:${channel.id}:agent:${agent.id}`;

      await expectAgentSessionSleptAfterRun(first.run_id, sessionKey, first.external_session_id);
      await expect.poll(() => {
        const state = agentHeartbeatState(agent!.id);
        return state.agent_active
          && state.computer_online
          && state.heartbeat_has_agent
          && state.heartbeat_epoch > heartbeatBeforeSleep;
      }, { timeout: 45000, intervals: [1000, 2000] }).toBe(true);

      const resumeLogOffset = readFileSync(daemonLogPath, 'utf8').length;
      await composer.fill(recallContent);
      await composer.press('Enter');
      await expect.poll(() => triggerMessageID(channel!.id, recallContent), {
        timeout: 30000, intervals: [250, 500, 1000],
      }).not.toBe('');
      const recallMessageID = triggerMessageID(channel.id, recallContent);
      await expect.poll(() => {
        const state = channelSessionState(recallMessageID);
        return `${state.status}/${state.external_session_id}/${state.message_content}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(
        `completed/${first.external_session_id}/RECALLED ${secret}`,
      );

      await expectAgentSessionResumed(resumeLogOffset, sessionKey, first.external_session_id);
      const recalled = channelSessionState(recallMessageID);
      await expect(page.locator(`[data-message-id="${recalled.message_id}"]`)).toContainText(`RECALLED ${secret}`);
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('automatically recovers a failed real Task run with the same Agent', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let failingAgent: Entity | null = null;
    let succeedingAgent: Entity | null = null;
    let task: TaskEntity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `auto-recovery-e2e-${suffix}`,
        description: 'Real Task same-Agent recovery E2E',
      });
      failingAgent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `FailingWorker${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'For every Task, first run the Bash command `sleep 60` and wait for it to finish. Only then use solo message send once with the requested target. Do not skip the wait.',
      });
      succeedingAgent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `SuccessfulWorker${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'For a Task, do not submit or close it. Use solo message send exactly once with the target from the request to deliver a concise completed result.',
      });
      task = await api<TaskEntity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/tasks`, {
        title: `@${failingAgent.name} Automatic recovery ${suffix}`,
        description: 'Deliver a short confirmation through solo message send.',
      });

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        const active = ['queued', 'running', 'thinking', 'streaming', 'waiting_input', 'waiting_approval'].includes(state.latest_status);
        return `${state.attempts}/${state.latest_agent_id}/${active}`;
      }, { timeout: 120000, intervals: [500, 1000, 2000] }).toBe(`1/${failingAgent.id}/true`);
      const interruptedRunID = taskRetryState(task.id).run_id;

      rebuildIsolatedE2EStack();

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        return `${state.status}/${state.claimer_id}/${state.attempts}/${state.failed_attempts}/${state.latest_status}/${state.latest_agent_id}/${Boolean(state.latest_message_id)}/${state.recovery_scheduled_events}/${state.retry_messages}/${state.recovery_previous_run_id}/${state.recovery_mode}/${state.workspace_reused}`;
      }, { timeout: 180000, intervals: [1000, 2000, 5000] }).toBe(
        `in_progress/${failingAgent.id}/2/1/completed/${failingAgent.id}/true/1/1/${interruptedRunID}/resume_session/true`,
      );

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.getByText(/Solo 正由原 Agent/).first()).toBeVisible();
      await page.locator('.relationship-flow .react-flow__node').filter({ hasText: failingAgent.name }).click();
      await page.getByRole('tab', { name: 'Run history' }).click();
      await expect(page.getByText('Failure and recovery')).toBeVisible();
      await expect(page.getByText('Continue the previous conversation')).toBeVisible();
      await expect(page.getByText('Reuse the original workspace')).toBeVisible();
    } finally {
      if (task && channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}/tasks/${task.id}`).catch(() => undefined);
      if (succeedingAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${succeedingAgent.id}`).catch(() => undefined);
      if (failingAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${failingAgent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('returns a real Task configuration failure to its creator without retrying', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    let task: TaskEntity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `configuration-failure-e2e-${suffix}`,
        description: 'Real Task configuration failure E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Misconfigured Worker ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        custom_args: ['--solo-e2e-intentionally-invalid-flag'],
        system_prompt: 'This Task is intentionally used to verify configuration failure handling.',
      });
      task = await api<TaskEntity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/tasks`, {
        title: `Configuration failure ${suffix}`,
        description: 'This real Task intentionally cannot deliver.',
      });

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        return `${state.status}/${Boolean(state.claimer_id)}/${state.attempts}/${state.failed_attempts}/${state.latest_status}/${state.failure_code}/${state.recovery_scheduled_events}/${state.recovery_blocked_events}/${state.retry_messages}/${state.exhausted_messages}`;
      }, { timeout: 120000, intervals: [1000, 2000, 5000] }).toBe('todo/false/1/1/failed/configuration/0/1/1/0');

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.getByText(/配置问题未自动恢复/).first()).toBeVisible();
      await page.locator('.relationship-flow .react-flow__node').filter({ hasText: agent.name }).click();
      await page.getByRole('tab', { name: 'Run history' }).click();
      await expect(page.getByText('Configuration is missing or invalid')).toBeVisible();
      await expect(page.getByText('Waiting for human handling').first()).toBeVisible();
      await expect(page.getByText('Task creator')).toBeVisible();
    } finally {
      if (task && channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}/tasks/${task.id}`).catch(() => undefined);
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });
});
