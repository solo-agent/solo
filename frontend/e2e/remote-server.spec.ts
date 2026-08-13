import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { rmSync } from 'node:fs';
import { join } from 'node:path';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const repoRoot = join(process.cwd(), '..');
const credentialFile = `/tmp/solo-remote-e2e-${process.pid}.json`;
const pairedDaemonID = `remote-e2e-${process.pid}`;

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: { id: string };
}
interface Entity { id: string; name: string }
interface Computer extends Entity {
  status: string;
  pairing_status: string;
  enrollment_token?: string;
  runtime_inventory?: Runtime[];
}
interface Runtime { type: string; display_name: string; available: boolean }
interface Attachment { id: string; url: string }
interface RunState {
  run_id: string;
  status: string;
  computer_id: string;
  attempt_id: string;
  delivery_count: number;
  payload_saved: boolean;
  events: number;
  unique_events: number;
  message_id: string;
  message_content: string;
}
interface TranscriptEntry { text?: string }
interface SessionTimeline { entries: TranscriptEntry[] }

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function rebuildPaired(computerID: string, enrollmentToken: string) {
  execFileSync('make', [
    'rebuild',
    `SOLO_COMPUTER_ID=${computerID}`,
    `SOLO_ENROLLMENT_TOKEN=${enrollmentToken}`,
    `SOLO_DAEMON_CREDENTIAL_FILE=${credentialFile}`,
    `DAEMON_ID=${pairedDaemonID}`,
  ], { cwd: repoRoot, stdio: 'inherit', timeout: 240000 });
}

function rebuildDefault() {
  execFileSync('make', ['rebuild'], { cwd: repoRoot, stdio: 'inherit', timeout: 240000 });
}

function deactivateTestUsers(...auths: AuthResponse[]) {
  const ids = auths.map((auth) => `'${auth.user.id}'`).join(',');
  if (!ids) return;
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c', `
      BEGIN;
      DELETE FROM channel_members
       WHERE member_type='user' AND member_id IN (${ids});
      DELETE FROM workspace_members WHERE user_id IN (${ids});
      DELETE FROM sessions WHERE user_id IN (${ids});
      UPDATE agent_sessions SET status='closed',last_active_at=now()
       WHERE agent_id IN (SELECT id FROM agents WHERE owner_id IN (${ids}));
      UPDATE agents SET is_active=false,updated_at=now()
       WHERE owner_id IN (${ids});
      DELETE FROM computers WHERE owner_id IN (${ids});
      UPDATE users SET is_active=false,updated_at=now() WHERE id IN (${ids});
      COMMIT;
    `,
  ], { encoding: 'utf8' });
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const suffix = Date.now().toString(36);
  const response = await registerVerified(request, apiBase, {
    data: {
      email: `remote-server-${suffix}@solo.local`,
      password: 'SoloRemote-2026!',
      display_name: 'Remote Server E2E',
    },
  });
  if (!response.ok()) throw new Error(`register: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function api<T>(request: APIRequestContext, token: string, method: 'get' | 'post' | 'delete' | 'patch', path: string, data?: unknown): Promise<T> {
  const options = {
    headers: { authorization: `Bearer ${token}` },
    ...(data === undefined ? {} : { data }),
  };
  const response = method === 'get'
    ? await request.get(`${apiBase}${path}`, options)
    : method === 'post'
      ? await request.post(`${apiBase}${path}`, options)
      : method === 'patch'
        ? await request.patch(`${apiBase}${path}`, options)
        : await request.delete(`${apiBase}${path}`, options);
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

function runState(triggerMessageID: string): RunState {
  return databaseJSON<RunState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'status', r.status,
        'computer_id', COALESCE(r.computer_id::text, ''),
        'attempt_id', COALESCE(r.execution_attempt_id::text, ''),
        'delivery_count', r.delivery_count,
        'payload_saved', r.dispatch_payload <> '{}'::jsonb,
        'events', (SELECT COUNT(*) FROM agent_run_delivery_events e WHERE e.run_id = r.id),
        'unique_events', (SELECT COUNT(DISTINCT (e.attempt_id, e.source_seq)) FROM agent_run_delivery_events e WHERE e.run_id = r.id),
        'message_id', COALESCE(m.id::text, ''),
        'message_content', COALESCE(m.content, '')
      )::text
        FROM agent_runs r
        LEFT JOIN LATERAL (
          SELECT id, content FROM messages
           WHERE metadata->>'agent_run_id' = r.id::text
           ORDER BY created_at LIMIT 1
        ) m ON true
       WHERE r.trigger_message_id = '${triggerMessageID}'
       ORDER BY r.started_at DESC LIMIT 1
    ), '{"run_id":"","status":"","computer_id":"","attempt_id":"","delivery_count":0,"payload_saved":false,"events":0,"unique_events":0,"message_id":"","message_content":""}')
  `);
}

test.describe('complete remote Server path', () => {
  test.skip(process.env.SOLO_E2E_REMOTE_SERVER !== '1', 'requires the make-managed stack, PostgreSQL, and a real local Agent runtime');
  test.setTimeout(600000);

  test('pairs a Computer and recovers one offline Run through the real UI and database', async ({ page, request }) => {
    const auth = await authenticate(request);
    const suffix = Date.now().toString(36);
    const computerName = `Remote Computer ${suffix}`;
    const onlineAck = `REMOTE_ONLINE_${suffix.toUpperCase()}`;
    const queuedAck = `REMOTE_RECOVERED_${suffix.toUpperCase()}`;
    const attachmentMarker = `ATTACHMENT_${suffix.toUpperCase()}`;
    let computer: Computer | null = null;
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    let outsider: AuthResponse | null = null;
    const defaultComputerID = databaseJSON<string>(`SELECT to_json(COALESCE((SELECT id::text FROM computers WHERE daemon_id='daemon-01'),''))::text`);
    expect(defaultComputerID).not.toBe('');

    try {
      computer = await api<Computer>(request, auth.access_token, 'post', '/api/v1/computers', { name: computerName });
      expect(computer.enrollment_token).toBeTruthy();
      rebuildPaired(computer.id, computer.enrollment_token!);

      await expect.poll(async () => {
        const computers = await api<Computer[]>(request, auth.access_token, 'get', '/api/v1/computers');
        return computers.find((item) => item.id === computer!.id);
      }, { intervals: [500, 1000, 2000] }).toMatchObject({ status: 'online', pairing_status: 'paired' });

      const runtimes = await api<Runtime[]>(request, auth.access_token, 'get', `/api/v1/agent-backends/detect?computer_id=${computer.id}`);
      const runtime = ['claude', 'codex', 'opencode', 'hermes', 'openclaw']
        .map((type) => runtimes.find((item) => item.type === type && item.available))
        .find(Boolean);
      expect(runtime, `No authenticated local Agent runtime is available: ${JSON.stringify(runtimes)}`).toBeTruthy();

      await authenticatePage(page, auth);
      await page.goto('/computers');
      await expect(page.getByText(computerName, { exact: true }).first()).toBeVisible();
      await expect(page.getByText('paired', { exact: true })).toBeVisible();
      await expect(page.getByText('Online', { exact: true }).first()).toBeVisible();

      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `remote-server-e2e-${suffix}`,
        description: 'Complete reverse-runtime delivery E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `RemoteAgent${suffix}`,
        computer_id: computer.id,
        model_provider: runtime!.type,
        model_name: runtime!.type === 'claude' ? 'sonnet' : '',
        system_prompt: [
          `When introducing yourself, use solo message send exactly once with ${onlineAck}.`,
          `When a human message starts with REMOTE_QUEUE_, read its attached text file. If it contains ${attachmentMarker}, use solo message send exactly once with ${queuedAck}.`,
          'Never merely print the requested result; deliver it with solo message send.',
        ].join(' '),
      });

      await expect.poll(() => databaseJSON<{ status: string; message_id: string }>(`
        WITH latest AS (
          SELECT id, status FROM agent_runs WHERE agent_id = '${agent!.id}' ORDER BY started_at DESC LIMIT 1
        )
        SELECT json_build_object(
          'status', COALESCE((SELECT status FROM latest), ''),
          'message_id', COALESCE((SELECT id::text FROM messages WHERE metadata->>'agent_run_id' = (SELECT id::text FROM latest) LIMIT 1), '')
        )::text
      `), { timeout: 180000, intervals: [500, 1000, 2000] }).toMatchObject({
        status: 'completed',
        message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
      });

	  const onlineRun = databaseJSON<{ run_id: string; session_id: string }>(`
	    SELECT json_build_object('run_id', id::text, 'session_id', COALESCE(session_id::text, ''))::text
	      FROM agent_runs WHERE agent_id = '${agent.id}' ORDER BY started_at DESC LIMIT 1
	  `);
	  expect(onlineRun.session_id).toMatch(/^[0-9a-f-]{36}$/);
	  const transcript = await api<TranscriptEntry[]>(request, auth.access_token, 'get', `/api/v1/agent-runs/${onlineRun.run_id}/transcript`);
	  expect(transcript.length).toBeGreaterThan(0);
	  const timeline = await api<SessionTimeline>(request, auth.access_token, 'get', `/api/v1/agent-sessions/${onlineRun.session_id}/timeline`);
	  expect(timeline.entries.length).toBeGreaterThan(0);
	  outsider = await authenticate(request);
	  // New registrations auto-join the public Workspace and its Channels, so
	  // another public member can inspect the shared Agent's observable history.
	  expect((await request.get(`${apiBase}/api/v1/agent-runs/${onlineRun.run_id}/transcript`, {
	    headers: { authorization: `Bearer ${outsider.access_token}` },
	  })).status()).toBe(200);
	  expect((await request.get(`${apiBase}/api/v1/agent-sessions/${onlineRun.session_id}/timeline`, {
	    headers: { authorization: `Bearer ${outsider.access_token}` },
	  })).status()).toBe(200);

      const workspace = await request.get(`${apiBase}/api/v1/agents/${agent.id}/workspace`, { headers: { authorization: `Bearer ${auth.access_token}` } });
      expect(workspace.ok()).toBe(true);
      const skills = await request.get(`${apiBase}/api/v1/agents/${agent.id}/skills`, { headers: { authorization: `Bearer ${auth.access_token}` } });
      expect(skills.ok()).toBe(true);

      const upload = await request.post(`${apiBase}/api/v1/attachments/upload`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        multipart: { file: { name: 'remote-marker.txt', mimeType: 'text/plain', buffer: Buffer.from(attachmentMarker) } },
      });
      expect(upload.ok()).toBe(true);
      const attachment = await upload.json() as Attachment;
      expect((await request.get(`${apiBase}${attachment.url}`)).status()).toBe(401);
      expect((await request.get(`${apiBase}${attachment.url}?access_token=${encodeURIComponent(auth.access_token)}`)).status()).toBe(401);
      const protectedDownload = await request.get(`${apiBase}${attachment.url}`, { headers: { authorization: `Bearer ${auth.access_token}` } });
      expect(await protectedDownload.text()).toBe(attachmentMarker);

      await api(request, auth.access_token, 'post', `/api/v1/computers/${computer.id}/credential/revoke`);
      await expect.poll(() => databaseJSON<{ status: string; revoked: boolean }>(`
        SELECT json_build_object('status', status, 'revoked', credential_revoked_at IS NOT NULL)::text
          FROM computers WHERE id = '${computer!.id}'
      `)).toEqual({ status: 'offline', revoked: true });
	  expect((await request.get(`${apiBase}/api/v1/agents/${agent.id}/workspace`, {
	    headers: { authorization: `Bearer ${auth.access_token}` },
	  })).status()).toBe(503);
	  expect((await request.get(`${apiBase}/api/v1/agents/${agent.id}/skills`, {
	    headers: { authorization: `Bearer ${auth.access_token}` },
	  })).status()).toBe(503);

      const enrollment = await api<Computer>(request, auth.access_token, 'post', `/api/v1/computers/${computer.id}/enrollment`);
      const trigger = await api<{ id: string }>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/messages`, {
        content: `REMOTE_QUEUE_${suffix}`,
        attachment_ids: [attachment.id],
      });
      await expect.poll(() => runState(trigger.id), { intervals: [250, 500, 1000] }).toMatchObject({
        status: 'queued',
        computer_id: computer.id,
        attempt_id: '',
        delivery_count: 0,
        payload_saved: true,
      });

      rebuildPaired(computer.id, enrollment.enrollment_token!);

      await expect.poll(() => runState(trigger.id), { timeout: 180000, intervals: [500, 1000, 2000] }).toMatchObject({
        status: 'completed',
        computer_id: computer.id,
        attempt_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        delivery_count: 1,
        payload_saved: true,
        message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
      });
      const recovered = runState(trigger.id);
	  expect(recovered.message_content).toContain(queuedAck);
      expect(recovered.events).toBeGreaterThan(0);
      expect(recovered.unique_events).toBe(recovered.events);

      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.locator(`[data-message-id="${recovered.message_id}"]`)).toContainText(queuedAck);
      await expect(page.getByText(`REMOTE_QUEUE_${suffix}`, { exact: true })).toBeVisible();
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
      rmSync(credentialFile, { force: true });
      rebuildDefault();
      if (computer) await api(request, auth.access_token, 'delete', `/api/v1/computers/${computer.id}`);
      expect(databaseJSON<string>(`SELECT to_json(COALESCE((SELECT id::text FROM computers WHERE daemon_id='daemon-01'),''))::text`)).toBe(defaultComputerID);
      if (computer) {
        expect(databaseJSON<number>(`SELECT to_json(count(*))::text FROM computers WHERE id='${computer.id}'`)).toBe(0);
      }
      deactivateTestUsers(auth, ...(outsider ? [outsider] : []));
    }
  });
});
