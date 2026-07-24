import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'agent-run-queue-e2e@solo.local', password: 'SoloE2E-2026!' };

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
}

interface RunState {
  total: number;
  queued: number;
  executing: number;
  started: number;
  finished: number;
  sessions: number;
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
    data: { ...credentials, display_name: 'Agent Run Queue E2E' },
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

function runState(agentID: string, taskIDs: string[]): RunState {
  const taskList = taskIDs.map((id) => `'${id}'`).join(',');
  return databaseJSON<RunState>(`
    SELECT json_build_object(
      'total', COUNT(*),
      'queued', COUNT(*) FILTER (WHERE run.status = 'queued' AND run.backend_started_at IS NULL),
      'executing', COUNT(*) FILTER (
        WHERE run.status IN ('thinking', 'running', 'streaming', 'waiting_input', 'waiting_approval')
          AND run.backend_started_at IS NOT NULL
      ),
      'started', COUNT(*) FILTER (WHERE run.backend_started_at IS NOT NULL),
      'finished', COUNT(*) FILTER (WHERE run.finished_at IS NOT NULL),
      'sessions', COUNT(DISTINCT run.session_id)
    )::text
      FROM agent_runs run
      JOIN agent_run_task_links link ON link.run_id = run.id
     WHERE run.agent_id = '${agentID}'
       AND link.task_id IN (${taskList})
  `);
}

function greetingFinished(agentID: string): boolean {
  return databaseJSON<{ done: boolean }>(`
    SELECT json_build_object('done', EXISTS(
      SELECT 1
        FROM agent_runs run
       WHERE run.agent_id = '${agentID}'
         AND run.finished_at IS NOT NULL
         AND NOT EXISTS (SELECT 1 FROM agent_run_task_links link WHERE link.run_id = run.id)
    ))::text
  `).done;
}

test('three real task triggers show one executing run and two queued runs', async ({ page, request }) => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_RUN_QUEUE !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(240000);

  const auth = await authenticate(request);
  const suffix = Date.now().toString(36);
  const agentName = `Queue E2E ${suffix}`;
  const taskTitles = [1, 2, 3].map((index) => `Queue lifecycle ${suffix}-${index}`);
  let channel: Entity | null = null;
  let agent: Entity | null = null;
  const tasks: TaskEntity[] = [];

  try {
    channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
      name: `queue-e2e-${suffix}`,
      description: 'Real queued Agent Run lifecycle E2E',
    });
    agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
      name: agentName,
      model_provider: 'claude',
      model_name: 'sonnet',
      system_prompt: 'If asked to introduce yourself, do it immediately. For a message containing Queue lifecycle, first use Bash to run sleep 20. After it finishes, answer with exactly QUEUE_E2E_DONE and nothing else.',
    });
    await expect.poll(() => greetingFinished(agent!.id), {
      timeout: 90000,
      intervals: [500, 1000, 2000],
    }).toBe(true);

    tasks.push(await api<TaskEntity>(request, auth.access_token, 'post', '/api/v1/tasks', {
      channel_id: channel.id,
      title: taskTitles[0],
    }));
    await expect.poll(() => runState(agent!.id, tasks.map((task) => task.id)).started, {
      timeout: 60000,
      intervals: [250, 500, 1000],
    }).toBe(1);

    tasks.push(...await Promise.all(taskTitles.slice(1).map((title) => (
      api<TaskEntity>(request, auth.access_token, 'post', '/api/v1/tasks', {
        channel_id: channel!.id,
        title,
      })
    ))));

    await expect.poll(() => {
      const state = runState(agent!.id, tasks.map((task) => task.id));
      return `${state.executing}/${state.queued}/${state.total}`;
    }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe('1/2/3');

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto('/observability/live');

    // The product emits one welcome run when an Agent joins a channel, then
    // the three task-triggered runs validated above.
    const card = page.locator('button').filter({ hasText: agentName }).filter({ hasText: '4 runs' }).first();
    await expect(card).toBeVisible({ timeout: 30000 });
    await expect(card).toContainText(/Thinking|Running|Streaming/);
    await card.click();
    const panel = page.locator('aside').filter({ hasText: agentName });
    await panel.getByRole('button', { name: 'Task', exact: true }).click();
    for (const title of taskTitles) {
      await expect(panel.getByRole('button').filter({ hasText: title })).toBeVisible();
    }

    await expect.poll(() => {
      const state = runState(agent!.id, tasks.map((task) => task.id));
      return `${state.started}/${state.finished}/${state.sessions}`;
    }, { timeout: 180000, intervals: [1000, 2000] }).toBe('3/3/1');
  } finally {
    if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
  }
});
