import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: { id: string; email: string; display_name: string };
  workspace_id?: string;
}

interface InviteLink {
  id: string;
  url: string;
}

interface Workspace {
  id: string;
  name: string;
}

async function register(request: APIRequestContext, prefix: string): Promise<AuthResponse> {
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
  const email = `${prefix}-${suffix}@workspace-invite-e2e.invalid`;
  const response = await registerVerified(request, apiBase, {
    data: {
      email,
      password: 'SoloWorkspace-2026!',
      display_name: `${prefix} ${suffix.slice(-5)}`,
    },
  });
  if (!response.ok()) throw new Error(`register ${prefix}: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function call<T>(
  request: APIRequestContext,
  auth: AuthResponse,
  method: 'get' | 'post',
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
    : await request.post(`${apiBase}${path}`, options);
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

function deactivateUsers(...auths: AuthResponse[]) {
  const ids = auths.map((auth) => `'${auth.user.id}'`).join(',');
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c', `
      UPDATE users SET is_active=false,updated_at=now() WHERE id IN (${ids});
      UPDATE workspaces SET deleted_at=COALESCE(deleted_at,now()),updated_at=now()
       WHERE created_by IN (${ids});
    `,
  ], { encoding: 'utf8' });
}

test.describe('Workspace member invite links', () => {
  test.skip(process.env.SOLO_E2E_WORKSPACES !== '1', 'requires the make-managed frontend, API, and PostgreSQL');
  test.setTimeout(180000);

  test('redirects visitors, then lets a signed-in user join and see the workspace', async ({ page, browser, request }) => {
    const owner = await register(request, 'InviteOwner');
    const member = await register(request, 'InviteMember');
    const workspaceID = owner.workspace_id!;
    let invitePath = '';

    try {
      const link = await call<InviteLink>(request, owner, 'post', `/api/v1/workspaces/${workspaceID}/invite-links`, workspaceID, { expires_in_days: 7 });
      invitePath = new URL(link.url, 'http://localhost:3000').pathname;

      const visitorContext = await browser.newContext();
      const visitor = await visitorContext.newPage();
      await visitor.goto(invitePath);
      await expect(visitor).toHaveURL(/\/auth\/login\?return_to=.*invite/);
      await visitorContext.close();

      await authenticatePage(page, member);
      await page.goto(invitePath);
      await expect(page.getByRole('heading', { name: /join/i })).toBeVisible();
      await page.getByRole('button', { name: /join/i }).click();
      await expect(page).toHaveURL(/\/dashboard/);

      const workspaces = await call<Workspace[]>(request, member, 'get', '/api/v1/workspaces');
      expect(workspaces).toEqual(expect.arrayContaining([
        expect.objectContaining({ id: workspaceID }),
      ]));
    } finally {
      deactivateUsers(owner, member);
    }
  });
});
