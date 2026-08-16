import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `thread-preview-e2e-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse { access_token: string; refresh_token: string }
interface Entity { id: string }

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Thread Preview E2E' },
  });
  if (!response.ok()) throw new Error(`E2E registration failed: ${response.status()} ${await response.text()}`);
  return response.json();
}

test('channel messages show the latest three persisted thread replies with a hierarchy line', async ({ page, request }) => {
  const auth = await authenticate(request);
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-v', 'ON_ERROR_STOP=1', '-c', `UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`,
  ], { stdio: 'ignore' });
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `thread-preview-${suffix}`, description: 'Thread preview E2E' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as Entity;

  try {
    const createdRoot = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
      headers,
      data: { content: `THREAD_PREVIEW_ROOT_${suffix}` },
    });
    expect(createdRoot.ok()).toBe(true);
    const root = await createdRoot.json() as Entity;

    for (let index = 1; index <= 4; index += 1) {
      const reply = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages/${root.id}/thread`, {
        headers,
        data: { content: `THREAD_PREVIEW_REPLY_${index}_${suffix}` },
      });
      expect(reply.ok()).toBe(true);
    }

    const persistedCount = execFileSync('docker', [
      'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
      'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
      '-tA', '-c', `SELECT count(*) FROM messages WHERE thread_id=(SELECT id FROM threads WHERE root_message_id='${root.id}')`,
    ], { encoding: 'utf8' }).trim();
    expect(persistedCount).toBe('4');

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'zh-CN');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);

    const rootRow = page.locator(`[data-message-id="${root.id}"]`);
    const preview = rootRow.locator('[data-thread-preview]');
    await expect(preview.locator('[data-thread-preview-reply]')).toHaveCount(3);
    await expect(preview).not.toContainText(`THREAD_PREVIEW_REPLY_1_${suffix}`);
    for (let index = 2; index <= 4; index += 1) {
      await expect(preview).toContainText(`THREAD_PREVIEW_REPLY_${index}_${suffix}`);
    }
    await expect(preview).toHaveCSS('border-left-style', 'solid');
    await expect(preview).toHaveCSS('border-left-width', '2px');

    const replyButton = preview.getByRole('button', { name: '4 条回复' });
    expect(await replyButton.evaluate((element) => parseFloat(getComputedStyle(element).borderRadius))).toBeGreaterThan(0);
    expect(await replyButton.evaluate((element) => getComputedStyle(element).borderColor))
      .toBe(await preview.evaluate((element) => getComputedStyle(element).borderLeftColor));
    await replyButton.click();
    await expect(page.getByRole('button', { name: '关闭讨论串面板' })).toBeVisible();
    const threadPanel = page.locator('[data-thread-panel]');
    const replies = threadPanel.locator('[data-thread-reply]');
    await expect(replies).toHaveCount(4);
    await expect(replies.first()).toHaveAttribute('data-grouped', 'false');
    await expect(replies.nth(1)).toHaveAttribute('data-grouped', 'true');
    await expect(threadPanel.locator('[data-message-date-separator]')).toHaveCount(1);
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers });
  }
});
