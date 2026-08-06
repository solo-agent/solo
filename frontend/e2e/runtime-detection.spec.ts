import { execFileSync } from 'node:child_process';
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const daemonBase = process.env.SOLO_E2E_DAEMON_URL ?? 'http://127.0.0.1:8081';
const credentials = { email: 'runtime-detection-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface BackendMeta {
  type: string;
  display_name: string;
  requires_binary: string;
  protocols: string[];
}

interface BackendStatus {
  type: string;
  display_name: string;
  binary: string;
  available: boolean;
  version?: string;
  error?: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Runtime Detection E2E' },
  });
  if (!register.ok()) {
    throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  }
  return register.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

function onlineComputerState(): { count: number; daemon_id: string } {
  const output = execFileSync('docker', [
    'exec',
    process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA',
    '-c', `SELECT json_build_object(
      'count', COUNT(*),
      'daemon_id', COALESCE(MAX(daemon_id), '')
    )::text FROM computers WHERE status = 'online' AND daemon_id IS NOT NULL`,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output);
}

test.describe('M9 runtime boundaries', () => {
  test.skip(process.env.SOLO_E2E_RUNTIME_DETECTION !== '1', 'requires the make-managed stack');

  test('uses registry metadata and the real daemon probe in API and UI', async ({ page, request }) => {
    const auth = await authenticate(request);
    const headers = { authorization: `Bearer ${auth.access_token}` };
    const suffix = Date.now().toString(36);

    const metadataResponse = await request.get(`${apiBase}/api/v1/agent-backends`, { headers });
    expect(metadataResponse.ok()).toBeTruthy();
    const metadata = await metadataResponse.json() as BackendMeta[];
    expect(metadata.find((item) => item.type === 'opencode')?.protocols).toEqual(['acp']);
    expect(metadata.find((item) => item.type === 'openclaw')?.protocols).toEqual(['acp']);
    expect(metadata.every((item) => item.type && item.display_name && item.requires_binary)).toBeTruthy();

    const daemonResponse = await request.get(`${daemonBase}/internal/daemon/backends/detect`);
    expect(daemonResponse.ok()).toBeTruthy();
    const daemonResults = await daemonResponse.json() as BackendStatus[];

    const serverResponse = await request.get(`${apiBase}/api/v1/agent-backends/detect`, { headers });
    expect(serverResponse.ok()).toBeTruthy();
    const serverResults = await serverResponse.json() as BackendStatus[];
    const stableStatus = (items: BackendStatus[]) => items
      .map(({ type, display_name, binary, available }) => ({ type, display_name, binary, available }))
      .sort((a, b) => a.type.localeCompare(b.type));
    expect(stableStatus(serverResults)).toEqual(stableStatus(daemonResults));

    const channelResponse = await request.post(`${apiBase}/api/v1/channels`, {
      headers,
      data: { name: `runtime-detection-e2e-${suffix}` },
    });
    expect(channelResponse.ok()).toBeTruthy();
    const channel = await channelResponse.json() as { id: string };

    try {
      await authenticatePage(page, auth);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await page.getByRole('button', { name: 'Teams', exact: true }).click();
      await page.getByRole('button', { name: 'Create first Agent' }).click();

      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Select Runtime...' }).click();
      const visibleRuntime = serverResults.find((item) =>
        ['openclaw', 'hermes', 'claude', 'opencode', 'codex'].includes(item.type),
      );
      expect(visibleRuntime).toBeDefined();
      await expect(page.locator('[role="option"]').filter({ hasText: visibleRuntime!.display_name })).toBeVisible();

      const persisted = onlineComputerState();
      expect(persisted.count).toBeGreaterThanOrEqual(1);
      expect(persisted.daemon_id).not.toBe('');
    } finally {
      const cleanup = await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers });
      expect(cleanup.ok()).toBeTruthy();
    }
  });
});
