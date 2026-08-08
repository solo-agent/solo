import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const authCode = process.env.SOLO_E2E_AUTH_CODE ?? '123456';

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

test.describe('public remote first-user experience', () => {
  test.skip(process.env.SOLO_E2E_PUBLIC_REMOTE !== '1', 'requires the make-managed frontend, API, and PostgreSQL stack');
  test.setTimeout(180000);

  test('registers, recovers the password, and exposes a clean-machine pairing command', async ({ page, request, context }) => {
    const suffix = Date.now().toString(36);
    const email = `public-remote-${suffix}@solo.local`;
    const password = 'SoloPublic-2026!';
    const newPassword = 'SoloRecovered-2026!';
    let computerID = '';

    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.addInitScript(() => localStorage.setItem('solo.locale', 'en'));
    await page.goto('/auth/register');
    await page.getByLabel('Display name').fill('Public Remote E2E');
    await page.getByLabel('Email').fill(email);
    await page.getByLabel('Password', { exact: true }).fill(password);
    await page.getByLabel('Confirm password').fill(password);
    await page.getByRole('button', { name: 'Create account' }).click();

    await expect(page.getByRole('heading', { name: 'Verify your email' })).toBeVisible();
    if (process.env.SOLO_E2E_MAILPIT_URL) {
      await expect.poll(async () => {
        const response = await request.get(`${process.env.SOLO_E2E_MAILPIT_URL}/api/v1/messages`);
        return response.ok() ? JSON.stringify(await response.json()) : '';
      }, { timeout: 15000, intervals: [250, 500, 1000] }).toContain(email);
    }
    await page.getByLabel('Verification code').fill(authCode);
    await page.getByRole('button', { name: 'Verify and continue' }).click();
    await expect(page).toHaveURL(/\/dashboard/);

    const persisted = databaseJSON<{ verified: boolean; onboarding_channels: number }>(`
      SELECT json_build_object(
        'verified', email_verified_at IS NOT NULL,
        'onboarding_channels', (SELECT COUNT(*) FROM channels WHERE created_by = users.id)
      )::text FROM users WHERE email = '${email}'
    `);
    expect(persisted.verified).toBe(true);
    expect(persisted.onboarding_channels).toBeGreaterThan(0);

    const initialLogin = await request.post(`${apiBase}/api/v1/auth/login`, { data: { email, password } });
    expect(initialLogin.ok()).toBe(true);
    const initialAuth = await initialLogin.json() as { access_token: string; refresh_token: string };

    await page.evaluate(() => {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
    });
    await page.goto('/auth/forgot-password');
    await page.getByLabel('Email').fill(email);
    await page.getByRole('button', { name: 'Send reset code' }).click();
    await page.getByLabel('Verification code').fill(authCode);
    await page.getByLabel('New password').fill(newPassword);
    await page.getByLabel('Confirm password').fill(newPassword);
    await page.getByRole('button', { name: 'Reset password' }).click();
    await expect(page.getByText('Password reset. You can now log in with your new password.')).toBeVisible();

    expect((await request.post(`${apiBase}/api/v1/auth/login`, { data: { email, password } })).status()).toBe(401);
    const recoveredLogin = await request.post(`${apiBase}/api/v1/auth/login`, { data: { email, password: newPassword } });
    expect(recoveredLogin.ok()).toBe(true);
    const recoveredAuth = await recoveredLogin.json() as { access_token: string; refresh_token: string };
    expect((await request.post(`${apiBase}/api/v1/auth/refresh`, { data: { refresh_token: initialAuth.refresh_token } })).status()).toBe(401);

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
    }, { accessToken: recoveredAuth.access_token, refreshToken: recoveredAuth.refresh_token });
    await page.goto('/computers');
    await page.getByRole('button', { name: /Add Computer/i }).click();
    await page.getByLabel('Name', { exact: true }).fill(`Public Remote ${suffix}`);
    await page.getByRole('button', { name: /Create pairing/i }).click();
    const installCommand = page.locator('pre').first();
    await expect(installCommand).toContainText('curl -fsSL');
    await expect(installCommand).toContainText('scripts/install.sh');
    await expect(installCommand).toContainText('bash -s -- connect');
    await expect(page.locator('pre').nth(1)).toContainText('solo daemon connect');
    await page.getByRole('button', { name: 'Copy' }).first().click();
    await expect(page.getByText('Command copied.')).toBeVisible();

    const computers = await request.get(`${apiBase}/api/v1/computers`, { headers: { authorization: `Bearer ${recoveredAuth.access_token}` } });
    const computerList = await computers.json() as Array<{ id: string; name: string }>;
    computerID = computerList.find((computer) => computer.name === `Public Remote ${suffix}`)?.id ?? '';
    expect(computerID).not.toBe('');
    await request.delete(`${apiBase}/api/v1/computers/${computerID}`, { headers: { authorization: `Bearer ${recoveredAuth.access_token}` } });
  });
});
