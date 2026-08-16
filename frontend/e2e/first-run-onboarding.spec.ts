import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';

const repoRoot = resolve(process.cwd(), '..');

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function rebuild(extra: Record<string, string> = {}) {
  execFileSync('make', ['rebuild', ...Object.entries(extra).map(([key, value]) => `${key}=${value}`)], {
    cwd: repoRoot,
    env: { ...process.env, ...extra },
    stdio: 'ignore',
    timeout: 120_000,
  });
}

test.describe('mandatory first-run onboarding', () => {
  test.skip(process.env.SOLO_E2E_FIRST_RUN !== '1', 'requires the make-managed localhost stack and a real local Agent runtime');
  test.use({ locale: 'zh-CN', viewport: { width: 1440, height: 960 } });

  test('registers, connects a real computer, creates a workspace and waits for Lucy', async ({ page }) => {
    const suffix = `${Date.now()}-${process.pid}`;
    const email = `first-run-${suffix}@solo.local`;
    const password = 'SoloFirstRun-2026!';
    const workspaceName = `第一次项目-${suffix}`;
    const daemonState = mkdtempSync(`${tmpdir()}/solo-first-run-daemon.`);

    await page.addInitScript(() => localStorage.setItem('solo.locale', 'zh-CN'));

    try {
      await page.goto('/auth/register');
      await page.getByLabel('显示名称').fill('第一次使用 Solo');
      await page.getByLabel('邮箱').fill(email);
      await page.getByLabel('密码', { exact: true }).fill(password);
      await page.getByLabel('确认密码').fill(password);
      await page.getByRole('button', { name: '创建账号' }).click();

      await expect(page).toHaveURL(/\/home\?onboarding=1/);
      await expect(page.getByRole('heading', { name: '从“电脑”开始' })).toBeVisible();
      expect(databaseJSON<{ personal: number; required: boolean }>(`
        SELECT json_build_object(
          'personal',(SELECT count(*) FROM workspaces w WHERE w.created_by=u.id AND w.is_personal=true AND w.deleted_at IS NULL),
          'required',u.onboarding_completed_at IS NULL
        )::text FROM users u WHERE u.email='${email}'
      `)).toEqual({ personal: 0, required: true });

      await page.getByRole('button', { name: '查看电脑' }).click();
      await expect(page).toHaveURL(/\/computers/);
      await page.getByRole('button', { name: '连接这台电脑' }).click();
      const command = await page.locator('pre').last().textContent();
      const match = command?.match(/--server '([^']+)' --computer-id '([^']+)' --token '([^']+)'/);
      expect(match).toBeTruthy();

      rebuild({
        DAEMON_SERVER_URL: match![1],
        SOLO_COMPUTER_ID: match![2],
        SOLO_ENROLLMENT_TOKEN: match![3],
        SOLO_DAEMON_CREDENTIAL_FILE: `${daemonState}/credentials.json`,
        SOLO_DAEMON_STATE_DIR: daemonState,
      });
      await page.goto('/home?onboarding=1');

      await expect(page.getByRole('heading', { name: '创建你的第一个工作区' })).toBeVisible();
      await page.getByRole('button', { name: '创建工作区', exact: true }).click();
      await page.getByLabel('工作区名称').fill(workspaceName);
      await page.getByRole('button', { name: '创建', exact: true }).click();

      await expect(page).toHaveURL(/\/dashboard\?channel=[0-9a-f-]{36}&onboarding=1/);
      await expect(page.getByText('先告诉 Solo 你想完成什么')).toHaveCount(0);
      await expect(page.getByRole('heading', { name: '创建 Lucy', exact: true })).toBeVisible();
      await page.getByRole('button', { name: '创建 Lucy', exact: true }).click();
      const codex = page.getByLabel(/codex/i);
      if (await codex.count()) await codex.check();
      await page.getByRole('button', { name: '创建 Lucy', exact: true }).click();

      await expect(page.getByRole('heading', { name: 'Lucy 正在向你打招呼' })).toBeVisible();
      await expect(page).toHaveURL(/\/dashboard\?channel=[0-9a-f-]{36}/, { timeout: 240_000 });
      await expect(page.getByText('Lucy', { exact: true }).first()).toBeVisible();

      const persisted = databaseJSON<{ personal: number; lucy: number; greetings: number; completed: boolean }>(`
        SELECT json_build_object(
          'personal',(SELECT count(*) FROM workspaces w WHERE w.created_by=u.id AND w.is_personal=true AND w.deleted_at IS NULL),
          'lucy',(SELECT count(*) FROM agents a WHERE a.owner_id=u.id AND a.kind='lucy' AND a.is_active=true),
          'greetings',(SELECT count(*) FROM messages m JOIN agents a ON a.id=m.sender_id WHERE a.owner_id=u.id AND a.kind='lucy' AND m.sender_type='agent'),
          'completed',u.onboarding_completed_at IS NOT NULL
        )::text FROM users u WHERE u.email='${email}'
      `);
      expect(persisted).toMatchObject({ personal: 1, lucy: 1, completed: true });
      expect(persisted.greetings).toBeGreaterThanOrEqual(1);
    } finally {
      const escaped = email.replaceAll("'", "''");
      execFileSync('docker', [
        'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
        'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
        '-d', process.env.POSTGRES_DB ?? 'solo', '-c',
        `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email='${escaped}'); UPDATE users SET is_active=false WHERE email='${escaped}';`,
      ], { stdio: 'ignore' });
      rebuild();
      rmSync(daemonState, { recursive: true, force: true });
    }
  });
});
