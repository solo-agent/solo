// spec: e2e/specs/student-first-use.plan.md
// seed: e2e/student-first-use-seed.spec.ts
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

function deactivate(email: string) {
  const escaped = email.replaceAll("'", "''");
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c',
    `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email='${escaped}'); UPDATE users SET is_active=false WHERE email='${escaped}';`,
  ], { stdio: 'ignore' });
}

test.describe('First useful screen', () => {
  test.use({ locale: 'zh-CN' });

  test('chinese-registration-opens-lucy', async ({ page }) => {
    const suffix = `${Date.now()}-${process.pid}`;
    const email = `student-first-use-${suffix}@solo.local`;
    const password = 'SoloStudent-2026!';
    const goal = `做一个记录喝水的简单网页 ${suffix}`;

    try {
      // 1. Open Solo in a fresh browser whose preferred language is Chinese.
      await page.goto('/');
      await expect(page.getByRole('combobox', { name: '语言' })).toHaveValue('zh-CN');
      await expect(page.getByRole('link', { name: '免费开始' })).toBeVisible();
      await page.getByRole('link', { name: '免费开始' }).click();

      // 2. Register a new test account through the real registration form.
      await page.getByLabel('显示名称').fill('理工科新生');
      await page.getByLabel('邮箱').fill(email);
      await page.getByLabel('密码', { exact: true }).fill(password);
      await page.getByLabel('确认密码').fill(password);
      await page.getByRole('button', { name: '创建账号' }).click();
      await expect(page).toHaveURL(/\/dashboard\?(?:channel=[^&]+&onboarding=1|lucy=1)/);
      await expect(page.getByRole('heading', { name: '开始你的第一个项目' })).toBeVisible();

      const onboarding = databaseJSON<{ personal_workspace: string; channel_id: string; public_workspace: string }>(`
        SELECT json_build_object(
          'personal_workspace', w.id::text,
          'channel_id', c.id::text,
          'public_workspace', '00000000-0000-0000-0000-000000000001'
        )::text
          FROM users u
          JOIN workspaces w ON w.created_by=u.id AND w.is_personal=true
          JOIN channels c ON c.created_by=u.id AND c.type='lucy' AND c.workspace_id=w.id
         WHERE u.email='${email}'
         ORDER BY c.created_at DESC LIMIT 1
      `);
      expect(onboarding.personal_workspace).not.toBe(onboarding.public_workspace);
      await expect.poll(() => page.evaluate(() => localStorage.getItem('solo_active_workspace_id'))).toBe(onboarding.personal_workspace);

      // 3. Enter a small goal and request a recommendation.
      await page.getByPlaceholder('例如：做一个记录每天喝水的小网页').fill(goal);
      await page.getByRole('button', { name: '看看怎么开始' }).click();
      await expect(page.getByText('做一个简单网页', { exact: false }).first()).toBeVisible();
      const hiddenMessages = databaseJSON<{ count: number }>(`
        SELECT json_build_object('count', COUNT(*))::text
          FROM messages
         WHERE channel_id='${onboarding.channel_id}' AND content='${goal}'
      `);
      expect(hiddenMessages.count).toBe(0);

      // 4. Open the template library and verify the beginner-first default.
      await page.getByRole('link', { name: '模板库' }).click();
      await expect(page.getByRole('heading', { name: '先从小事开始' })).toBeVisible();
      await expect(page.getByRole('heading', { name: '做一个简单网页' })).toBeVisible();
      await expect(page.getByRole('heading', { name: '分析一份数据' })).toBeVisible();
      await expect(page.getByRole('heading', { name: '整理学习资料' })).toBeVisible();
      await expect(page.getByRole('button', { name: '查看更多专业团队' })).toBeVisible();
      await expect(page.getByRole('heading', { name: '技术方案评审' })).toHaveCount(0);
    } finally {
      deactivate(email);
    }
  });
});
