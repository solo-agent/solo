import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'agent-result-delivery-e2e@solo.local', password: 'SoloE2E-2026!' };

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
  status: string;
  contract: string;
  message_id: string;
  visible_event: boolean;
  input_tokens: number;
  output_tokens: number;
  failure_code: string;
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

interface TaskRetryState {
  status: string;
  claimer_id: string;
  attempts: number;
  failed_attempts: number;
  latest_status: string;
  latest_agent_id: string;
  latest_message_id: string;
  reassigned_events: number;
  retry_messages: number;
  exhausted_messages: number;
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
        ), '')
      )::text
      FROM latest r
    ), '{"run_id":"","status":"","contract":"","message_id":"","visible_event":false,"input_tokens":0,"output_tokens":0,"failure_code":""}')
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
      'reassigned_events', (
        SELECT COUNT(*) FROM agent_run_events e
         WHERE e.run_id IN (SELECT id FROM linked) AND e.type = 'task_reassigned'
      ),
      'retry_messages', (
        SELECT COUNT(*) FROM messages m
         WHERE m.metadata->>'task_id' = t.id::text
           AND m.metadata->>'auto_reassign' = 'true'
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
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Agent Result Delivery E2E' },
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

test.describe('real Agent result delivery contract', () => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_DELIVERY !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(240000);

  test('persists and renders a run-linked visible result before completing', async ({ page, request }) => {
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
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'When asked to introduce yourself, use solo message send to post the introduction to the current channel. Do not merely print it.',
      });

      await expect.poll(() => {
        const state = deliveryState(agent!.id);
        return `${state.status}/${state.contract}/${Boolean(state.message_id)}/${state.visible_event}`;
      }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed/visible_message/true/true');

      const state = deliveryState(agent.id);
      expect(state.input_tokens + state.output_tokens).toBeGreaterThan(0);

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      const persistedMessage = page.locator(`[data-message-id="${state.message_id}"]`);
      await expect(persistedMessage).toBeVisible();
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
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

      execFileSync('make', ['rebuild'], { cwd: '..', stdio: 'inherit' });

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
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: [
          'The sender label @Agent Result Delivery E2E identifies the human test user, not another Agent; its messages are directed to you.',
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

      execFileSync('make', ['rebuild'], { cwd: '..', stdio: 'inherit' });

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

  test('automatically reassigns a failed real Task run to another Agent', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let failingAgent: Entity | null = null;
    let succeedingAgent: Entity | null = null;
    let task: TaskEntity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `auto-reassign-e2e-${suffix}`,
        description: 'Real Task automatic reassignment E2E',
      });
      failingAgent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `FailingWorker${suffix}`,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'For every Task, first run the Bash command `sleep 60` and wait for it to finish. Only then use solo message send once with the requested target. Do not skip the wait.',
      });
      succeedingAgent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `SuccessfulWorker${suffix}`,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'For a Task, do not submit or close it. Use solo message send exactly once with the target from the request to deliver a concise completed result.',
      });
      task = await api<TaskEntity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/tasks`, {
        title: `@${failingAgent.name} Automatic reassignment ${suffix}`,
        description: 'Deliver a short confirmation through solo message send.',
      });

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        const active = ['queued', 'running', 'thinking', 'streaming', 'waiting_input', 'waiting_approval'].includes(state.latest_status);
        return `${state.attempts}/${state.latest_agent_id}/${active}`;
      }, { timeout: 120000, intervals: [500, 1000, 2000] }).toBe(`1/${failingAgent.id}/true`);

      execFileSync('make', ['rebuild'], { cwd: '..', stdio: 'inherit' });

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        return `${state.status}/${state.claimer_id}/${state.attempts}/${state.failed_attempts}/${state.latest_status}/${state.latest_agent_id}/${Boolean(state.latest_message_id)}/${state.reassigned_events}/${state.retry_messages}`;
      }, { timeout: 180000, intervals: [1000, 2000, 5000] }).toBe(
        `in_progress/${succeedingAgent.id}/2/1/completed/${succeedingAgent.id}/true/1/1`,
      );

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.getByText(/Solo 正在自动改派/).first()).toBeVisible();
    } finally {
      if (task && channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}/tasks/${task.id}`).catch(() => undefined);
      if (succeedingAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${succeedingAgent.id}`).catch(() => undefined);
      if (failingAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${failingAgent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });

  test('stops after three failed real Task attempts and returns it to TODO', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    let task: TaskEntity | null = null;

    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `retry-exhaustion-e2e-${suffix}`,
        description: 'Real Task retry exhaustion E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `Always Failing Worker ${suffix}`,
        model_provider: 'claude',
        model_name: 'sonnet',
        custom_env: {
          SOLO_API_URL: 'http://127.0.0.1:1',
          SOLO_DAEMON_URL: 'http://127.0.0.1:1',
        },
        system_prompt: 'For every request, run solo message send once with the requested target and append || true. Then stop without sending any other message.',
      });
      task = await api<TaskEntity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/tasks`, {
        title: `Retry exhaustion ${suffix}`,
        description: 'This real Task intentionally cannot deliver.',
      });

      await expect.poll(() => {
        const state = taskRetryState(task!.id);
        return `${state.status}/${Boolean(state.claimer_id)}/${state.attempts}/${state.failed_attempts}/${state.latest_status}/${state.reassigned_events}/${state.retry_messages}/${state.exhausted_messages}`;
      }, { timeout: 220000, intervals: [1000, 2000, 5000] }).toBe('todo/false/3/3/failed/2/2/1');

      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.getByText(/自动重派已尝试 3 次/).first()).toBeVisible();
    } finally {
      if (task && channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}/tasks/${task.id}`).catch(() => undefined);
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });
});
