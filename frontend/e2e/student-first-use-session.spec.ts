// spec: e2e/specs/student-first-use.plan.md
// seed: e2e/student-first-use-seed.spec.ts
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';

import { registerVerified } from './support/auth';

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function deactivate(email: string) {
  const escaped = email.replaceAll("'", "''");
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c',
    `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email='${escaped}'); UPDATE users SET is_active=false WHERE email='${escaped}';`,
  ], { stdio: 'ignore' });
}

test.describe('Stable login and personal Workspace', () => {
  test.use({ locale: 'en-US' });

  test('concurrent-refresh-and-returning-login-stay-valid', async ({ page, request }) => {
    const suffix = `${Date.now()}-${process.pid}`;
    const email = `student-session-${suffix}@solo.local`;
    const password = 'SoloStudent-2026!';
    const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';

    try {
      const verified = await registerVerified(request, apiBase, {
        data: { email, password, display_name: 'Returning Student' },
      });
      expect(verified.status()).toBe(201);
      const auth = await verified.json() as {
        refresh_token: string;
        user: { id: string };
        workspace_id: string;
      };

      const [firstRefresh, secondRefresh] = await Promise.all([
        request.post(`${apiBase}/api/v1/auth/refresh`, { data: { refresh_token: auth.refresh_token } }),
        request.post(`${apiBase}/api/v1/auth/refresh`, { data: { refresh_token: auth.refresh_token } }),
      ]);
      expect(firstRefresh.status()).toBe(200);
      expect(secondRefresh.status()).toBe(200);
      expect((await firstRefresh.json()).refresh_token).toBe(auth.refresh_token);
      expect((await secondRefresh.json()).refresh_token).toBe(auth.refresh_token);
      expect(databaseJSON<number>(`SELECT to_json(COUNT(*))::text FROM sessions WHERE user_id='${auth.user.id}'`)).toBe(1);

      await page.addInitScript(() => {
        localStorage.setItem('solo.locale', 'en');
        localStorage.setItem('solo_active_workspace_id', '00000000-0000-0000-0000-000000000001');
        localStorage.removeItem('solo_authenticated_user_id');
      });
      await page.goto('/auth/login');
      await page.getByLabel('Email').fill(email);
      await page.getByLabel('Password').fill(password);
      await page.getByRole('button', { name: 'Log in' }).click();

      await expect(page).toHaveURL(/\/dashboard/);
      await expect(page.getByRole('button', { name: 'Lucy' })).toBeVisible();
      await expect.poll(() => page.evaluate((userID) => ({
        current: localStorage.getItem(`solo_active_workspace_id:${userID}`),
        legacy: localStorage.getItem('solo_active_workspace_id'),
      }), auth.user.id)).toEqual({ current: auth.workspace_id, legacy: auth.workspace_id });
      await expect(page.getByRole('button', { name: 'Workspace menu' })).toContainText("Returning Student's Workspace");
    } finally {
      deactivate(email);
    }
  });
});
