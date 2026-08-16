import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `emoji-e2e-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse { access_token: string; refresh_token: string; workspace_id: string }
interface Entity { id: string; name: string }

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Emoji E2E' },
  });
  if (!register.ok()) throw new Error(`E2E registration failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

async function api<T>(request: APIRequestContext, token: string, workspaceID: string | undefined, method: 'post' | 'delete', path: string, data?: unknown): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: { authorization: `Bearer ${token}`, ...(workspaceID ? { 'X-Workspace-ID': workspaceID } : {}) },
    data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse, workspaceID: string) {
  await page.addInitScript(({ accessToken, refreshToken, workspaceID }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo_active_workspace_id', workspaceID);
    localStorage.setItem('solo.locale', 'zh-CN');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token, workspaceID });
}

test('emoji selected in the composer reaches PostgreSQL and the real Agent runtime', async ({ page, request }) => {
  test.skip(process.env.SOLO_E2E_REAL_EMOJI !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(300_000);

  const auth = await authenticate(request);
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-v', 'ON_ERROR_STOP=1', '-c', `UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`,
  ], { stdio: 'ignore' });

  let lease: LocalComputerLease | null = null;
  let workspace: Entity | null = null;
  let channel: Entity | null = null;
  let agent: Entity | null = null;
  try {
    lease = await acquireLocalComputer(request, apiBase, auth.access_token);
    const suffix = Date.now().toString(36);
    const marker = `EMOJI_RUNTIME_${suffix}`;
    const content = `${marker} \u{1F44D}`;
    const reply = `EMOJI_SEEN_${suffix.toUpperCase()}`;

    workspace = await api<Entity>(request, auth.access_token, undefined, 'post', '/api/v1/workspaces', {
      name: `Emoji E2E ${suffix}`,
    });
    channel = await api<Entity>(request, auth.access_token, workspace.id, 'post', '/api/v1/channels', {
      name: `emoji-e2e-${suffix}`,
      description: 'Emoji real runtime E2E',
    });
    agent = await api<Entity>(request, auth.access_token, workspace.id, 'post', `/api/v1/channels/${channel.id}/agents`, {
      name: `EmojiAgent${suffix}`,
      computer_id: lease.id,
      model_provider: 'claude',
      model_name: 'sonnet',
      system_prompt: [
        'Always use solo message send with the exact target from the latest incoming message.',
        'When introducing yourself, send exactly EMOJI_E2E_READY.',
        `For a message containing both ${marker} and the thumbs-up emoji, send exactly ${reply}.`,
        'Send one visible message and no explanation.',
      ].join(' '),
    });

    await expect.poll(() => databaseJSON<boolean>(`
      SELECT EXISTS(SELECT 1 FROM agent_runs WHERE agent_id='${agent!.id}' AND trigger_message_id IS NULL AND status='completed')::text
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toBe(true);

    await authenticatePage(page, auth, workspace.id);
    await page.goto(`/dashboard?channel=${channel.id}`);
    const composer = page.getByLabel('消息输入框');
    await composer.fill(`${marker} `);
    await page.getByRole('button', { name: '表情' }).click();
    await page.getByRole('button', { name: /插入.*👍/ }).click();
    await expect(composer).toHaveValue(content);
    await composer.press('Enter');
    await expect(page.getByText(content, { exact: true })).toHaveCount(1);

    await expect.poll(() => databaseJSON<string>(`
      SELECT to_json(COALESCE((SELECT content FROM messages WHERE channel_id='${channel!.id}' AND content LIKE '${marker}%' ORDER BY seq DESC LIMIT 1), ''))::text
    `), { timeout: 30_000, intervals: [250, 500, 1000] }).toBe(content);

    await expect.poll(() => databaseJSON<number>(`
      SELECT COUNT(*)::text FROM messages m
       WHERE m.channel_id='${channel!.id}' AND m.sender_id='${agent!.id}' AND m.content='${reply}'
         AND EXISTS (
           SELECT 1 FROM agent_runs r
            WHERE r.id::text=m.metadata->>'agent_run_id' AND r.status='completed'
              AND r.trigger_message_id=(SELECT id FROM messages WHERE channel_id='${channel!.id}' AND content='${content}' ORDER BY seq DESC LIMIT 1)
         )
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toBe(1);
    await expect(page.getByText(reply, { exact: true })).toHaveCount(1);
  } finally {
    if (agent) await api(request, auth.access_token, workspace?.id, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
    if (channel) await api(request, auth.access_token, workspace?.id, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    if (workspace) await api(request, auth.access_token, workspace.id, 'delete', `/api/v1/workspaces/${workspace.id}`).catch(() => undefined);
    if (lease) await lease.release(request).catch(() => undefined);
  }
});
