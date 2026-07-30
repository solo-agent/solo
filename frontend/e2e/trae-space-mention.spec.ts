import { execFileSync } from 'node:child_process';
import { expect, test, type APIRequestContext } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = {
  email: 'trae-space-mention-e2e@solo.local',
  password: 'SoloTraeE2E-2026!',
};

interface AuthResponse {
  access_token: string;
}

interface Entity {
  id: string;
}

interface TaskEntity extends Entity {
  message_id: string;
}

interface RoutingState {
  mentioned: boolean;
  runs: number;
  claimed: boolean;
  status: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, {
    data: credentials,
  });
  if (login.ok()) return login.json();

  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Trae Space Mention E2E' },
  });
  if (!register.ok()) {
    throw new Error(
      `E2E authentication failed: ${register.status()} ${await register.text()}`,
    );
  }
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
  if (!response.ok()) {
    throw new Error(
      `${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`,
    );
  }
  if (response.status() === 204) return undefined as T;
  return response.json();
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec',
    process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql',
    '-U',
    process.env.POSTGRES_USER ?? 'solo',
    '-d',
    process.env.POSTGRES_DB ?? 'solo',
    '-tA',
    '-c',
    query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function greetingFinished(agentID: string): boolean {
  return databaseJSON<{ done: boolean }>(`
    SELECT json_build_object('done', EXISTS(
      SELECT 1
        FROM agent_runs run
       WHERE run.agent_id = '${agentID}'
         AND run.finished_at IS NOT NULL
         AND NOT EXISTS (
           SELECT 1 FROM agent_run_task_links link WHERE link.run_id = run.id
         )
    ))::text
  `).done;
}

function routingState(taskID: string, messageID: string, agentID: string): RoutingState {
  return databaseJSON<RoutingState>(`
    SELECT json_build_object(
      'mentioned', '${agentID}'::uuid = ANY(message.mentioned_agent_ids),
      'runs', (
        SELECT COUNT(*)
          FROM agent_runs run
          JOIN agent_run_task_links link ON link.run_id = run.id
         WHERE link.task_id = task.id
           AND run.agent_id = '${agentID}'
      ),
      'claimed', task.claimer_id = '${agentID}'::uuid,
      'status', task.status
    )::text
      FROM tasks task
      JOIN messages message ON message.id = '${messageID}'
     WHERE task.id = '${taskID}'
  `);
}

test('task text mentioning a Trae agent with spaces routes and claims correctly', async ({ request }) => {
  test.skip(
    process.env.SOLO_E2E_REAL_TRAE_SPACE_MENTION !== '1',
    'requires the make-managed stack and authenticated local Trae runtime',
  );
  test.setTimeout(420000);

  const auth = await authenticate(request);
  const suffix = Date.now().toString(36);
  const agentName = `Trae Coding Developer ${suffix}`;
  let channel: Entity | null = null;

  try {
    channel = await api<Entity>(
      request,
      auth.access_token,
      'post',
      '/api/v1/channels',
      {
        name: `trae-space-mention-${suffix}`,
        description: 'Trae space-containing mention routing E2E',
      },
    );
    const agent = await api<Entity>(
      request,
      auth.access_token,
      'post',
      `/api/v1/channels/${channel.id}/agents`,
      {
        name: agentName,
        model_provider: 'trae',
        model_name: '',
        system_prompt:
          'For every assigned task, immediately claim it with solo task claim, then reply in the task thread.',
      },
    );

    await expect.poll(() => greetingFinished(agent.id), {
      timeout: 120000,
      intervals: [1000, 2000, 5000],
    }).toBe(true);

    const task = await api<TaskEntity>(
      request,
      auth.access_token,
      'post',
      `/api/v1/channels/${channel.id}/messages`,
      {
        content: `@${agentName} claim this routing test task and reply ROUTING_OK.`,
        as_task: true,
      },
    );

    await expect.poll(() => routingState(task.id, task.message_id, agent.id), {
      timeout: 300000,
      intervals: [1000, 2000, 5000],
    }).toMatchObject({
      mentioned: true,
      runs: 1,
      claimed: true,
      status: 'in_progress',
    });
  } finally {
    if (channel) {
      await api<void>(
        request,
        auth.access_token,
        'delete',
        `/api/v1/channels/${channel.id}`,
      );
    }
  }
});
