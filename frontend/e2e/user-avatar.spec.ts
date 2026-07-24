import { execFileSync } from 'node:child_process';
import { expect, test, type APIRequestContext } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'user-avatar-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'User Avatar E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

function persistedAvatar(): string | null {
  const output = execFileSync('docker', [
    'exec',
    process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA',
    '-c', `SELECT json_build_object('avatar_url', avatar_url)::text FROM users WHERE email = '${credentials.email}'`,
  ], { encoding: 'utf8' }).trim();
  return (JSON.parse(output) as { avatar_url: string | null }).avatar_url;
}

test('user chooses a pixel portrait, uploads a photo, and restores the default through the real profile flow', async ({ page, request }) => {
  const auth = await authenticate(request);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const reset = await request.patch(`${apiBase}/api/v1/users/me`, {
    headers,
    data: { avatar_url: '' },
  });
  expect(reset.ok()).toBeTruthy();

  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });

  await page.goto('/settings');
  await expect(page.getByText('Solo pixel portrait', { exact: true })).toBeVisible();
  const currentAvatar = page.locator('[aria-label="User Avatar E2E"]').first().locator('img');
  await expect(currentAvatar).toHaveAttribute('src', /^data:image\/svg\+xml/);

  const secondPreset = page.getByRole('button', { name: 'Profile picture 2' });
  await secondPreset.click();
  await expect(secondPreset).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(persistedAvatar).toBe('dicebear:pixel-art:1');

  const onePixelPNG = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
    'base64',
  );
  await page.locator('input[type="file"]').setInputFiles({
    name: 'profile.png',
    mimeType: 'image/png',
    buffer: onePixelPNG,
  });

  await expect(page.getByText('Custom photo', { exact: true })).toBeVisible();
  await expect(currentAvatar).toHaveAttribute('src', /^http:\/\/127\.0\.0\.1:8080\/api\/v1\/attachments\//);
  await expect.poll(persistedAvatar).toMatch(/^\/api\/v1\/attachments\//);

  await page.getByRole('button', { name: 'Restore default' }).click();
  await expect(page.getByText('Solo pixel portrait', { exact: true })).toBeVisible();
  await expect(currentAvatar).toHaveAttribute('src', /^data:image\/svg\+xml/);
  await expect.poll(persistedAvatar).toBeNull();
});
