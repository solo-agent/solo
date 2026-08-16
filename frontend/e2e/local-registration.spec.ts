import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function deactivateTestUser(email: string): void {
  const escapedEmail = email.replaceAll("'", "''");
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-v', 'ON_ERROR_STOP=1', '-c', `
      BEGIN;
      DELETE FROM channel_members
       WHERE member_type='user'
         AND member_id IN (SELECT id FROM users WHERE email='${escapedEmail}')
         AND channel_id IN (SELECT id FROM channels WHERE workspace_id='00000000-0000-0000-0000-000000000001');
      DELETE FROM workspace_members
       WHERE user_id IN (SELECT id FROM users WHERE email='${escapedEmail}')
         AND workspace_id='00000000-0000-0000-0000-000000000001';
      DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email='${escapedEmail}');
      UPDATE users SET is_active=false WHERE email='${escapedEmail}';
      COMMIT;
    `,
  ], { stdio: 'ignore' });
}

test.describe('localhost registration', () => {
  test.skip(process.env.SOLO_E2E_LOCAL_REGISTRATION !== '1', 'requires the make-managed localhost stack');

  test('creates a verified account without showing the email-code step', async ({ page }) => {
    const suffix = Date.now().toString(36);
    const email = `local-register-${suffix}@workspace-e2e.invalid`;
    const password = 'SoloLocal-2026!';

    await page.addInitScript(() => {
      localStorage.setItem('solo.locale', 'en');
      (window as Window & { __soloSawEmailVerification?: boolean }).__soloSawEmailVerification = false;
      new MutationObserver(() => {
        if (document.body?.innerText.includes('Verify your email')) {
          (window as Window & { __soloSawEmailVerification?: boolean }).__soloSawEmailVerification = true;
        }
      }).observe(document.documentElement, { childList: true, subtree: true });
    });

    try {
      await page.goto('/auth/register');
      await page.getByLabel('Display name').fill('Local Registration E2E');
      await page.getByLabel('Email').fill(email);
      await page.getByLabel('Password', { exact: true }).fill(password);
      await page.getByLabel('Confirm password').fill(password);

      const registerResponsePromise = page.waitForResponse((response) =>
        response.url().endsWith('/api/v1/auth/register') && response.request().method() === 'POST');
      await page.getByRole('button', { name: 'Create account' }).click();
      const registerResponse = await registerResponsePromise;
      expect(registerResponse.status()).toBe(202);
      expect(await registerResponse.json()).toMatchObject({ email_verification: false });

      await expect(page).toHaveURL(/\/home\?onboarding=1/);
      await expect(page.getByRole('heading', { name: 'Start from Computers' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Verify your email' })).toHaveCount(0);
      expect(await page.evaluate(() =>
        (window as Window & { __soloSawEmailVerification?: boolean }).__soloSawEmailVerification)).toBe(false);

      const persisted = databaseJSON<{
        verified: boolean;
        sessions: number;
        personal_workspaces: number;
        onboarding_required: boolean;
        public_membership: number;
        missing_public_channels: number;
      }>(`
        SELECT json_build_object(
          'verified', u.email_verified_at IS NOT NULL,
          'sessions', (SELECT COUNT(*) FROM sessions s WHERE s.user_id=u.id),
          'personal_workspaces', (SELECT COUNT(*) FROM workspaces w WHERE w.created_by=u.id AND w.is_personal=true AND w.deleted_at IS NULL),
          'onboarding_required', u.onboarding_completed_at IS NULL,
          'public_membership', (SELECT COUNT(*) FROM workspace_members wm WHERE wm.user_id=u.id AND wm.workspace_id='00000000-0000-0000-0000-000000000001'),
          'missing_public_channels', (SELECT COUNT(*) FROM channels c WHERE c.workspace_id='00000000-0000-0000-0000-000000000001' AND c.type='channel' AND c.is_archived=false AND NOT EXISTS (SELECT 1 FROM channel_members cm WHERE cm.channel_id=c.id AND cm.member_type='user' AND cm.member_id=u.id))
        )::text FROM users u WHERE u.email='${email}'
      `);
      expect(persisted).toEqual({
        verified: true,
        sessions: 1,
        personal_workspaces: 0,
        onboarding_required: true,
        public_membership: 1,
        missing_public_channels: 0,
      });
    } finally {
      deactivateTestUser(email);
    }
  });
});
