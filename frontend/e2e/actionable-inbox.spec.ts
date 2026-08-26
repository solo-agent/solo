import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `actionable-inbox-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse { access_token: string; refresh_token: string }
interface Entity { id: string }

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Inbox Reviewer' },
  });
  if (!response.ok()) throw new Error(`E2E registration failed: ${response.status()} ${await response.text()}`);
  return response.json();
}

function psql(sql: string) {
  return execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-v', 'ON_ERROR_STOP=1', '-c', sql,
  ], { encoding: 'utf8' }).trim();
}

test('review action can be returned with a persisted reason and appears in handled', async ({ page, request }) => {
  const auth = await authenticate(request);
  psql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `inbox-${suffix}`, description: 'Actionable inbox E2E' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as Entity;

  try {
    const createdTask = await request.post(`${apiBase}/api/v1/tasks`, {
      headers,
      data: { channel_id: channel.id, title: `INBOX_REVIEW_${suffix}`, description: 'Check the real result' },
    });
    expect(createdTask.ok()).toBe(true);
    const task = await createdTask.json() as Entity;
    psql(`UPDATE tasks SET status='in_review', updated_at=now() WHERE id='${task.id}'`);

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'zh-CN');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);
    await page.getByRole('button', { name: /收件箱/ }).click();
    await expect(page).toHaveURL(/\?inbox/);

    const taskHeading = page.getByRole('heading', { name: `INBOX_REVIEW_${suffix}` });
    await expect(taskHeading).toBeVisible();
    await expect(page.getByText('Check the real result')).toBeVisible();
    await expect(page.getByText('agent.activity.completed')).toHaveCount(0);
    await page.getByRole('button', { name: '查看任务详情' }).click();
    await expect(page).toHaveURL(new RegExp(`channel=${channel.id}.*view=task.*task=${task.id}`));
    await page.goBack();
    await expect(taskHeading).toBeVisible();
    await page.getByRole('button', { name: '退回', exact: true }).click();
    await page.getByPlaceholder('告诉 Agent 需要修改什么').fill('请补充真实验证结果');
    await page.getByRole('button', { name: '确认退回' }).click();
    await expect(taskHeading).toHaveCount(0);

    await page.getByRole('tab', { name: '已处理' }).click();
    await expect(page.getByRole('heading', { name: `INBOX_REVIEW_${suffix}` })).toBeVisible();
    await expect(page.getByText('请补充真实验证结果')).toBeVisible();
    expect(psql(`SELECT status FROM tasks WHERE id='${task.id}'`)).toBe('in_progress');
    expect(psql(`SELECT decision || ':' || reason FROM task_reviews WHERE task_id='${task.id}'`))
      .toBe('rejected:请补充真实验证结果');
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers });
  }
});
