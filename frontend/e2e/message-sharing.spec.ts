import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `message-sharing-e2e-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

async function authenticate(request: APIRequestContext) {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Message Sharing E2E' },
  });
  if (!response.ok()) throw new Error(`E2E registration failed: ${response.status()} ${await response.text()}`);
  return response.json() as Promise<{ access_token: string; refresh_token: string }>;
}

function queryDatabase(sql: string) {
  return execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-v', 'ON_ERROR_STOP=1', '-c', sql,
  ], { encoding: 'utf8' }).trim();
}

test('copies messages and exports selected channel and thread messages as PNG without changing persisted data', async ({ page, request, context }) => {
  const auth = await authenticate(request);
  queryDatabase(`UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `message-sharing-${suffix}`, description: 'Message sharing E2E' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as { id: string; name: string };

  try {
    const roots = [];
    for (const content of [`SHARE_FIRST_${suffix}`, `分享第二条_${suffix}`]) {
      const response = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, { headers, data: { content } });
      expect(response.ok()).toBe(true);
      roots.push(await response.json() as { id: string; content: string });
    }
    const reply = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages/${roots[0].id}/thread`, {
      headers,
      data: { content: `THREAD_SHARE_${suffix}` },
    });
    expect(reply.ok()).toBe(true);
    const threadReply = await reply.json() as { id: string };
    expect(queryDatabase(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}'`)).toBe('3');

    await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'http://localhost:3000' });
    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'zh-CN');
      localStorage.setItem('solo.message-shortcuts-seen', '1');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);

    const first = page.locator(`[data-message-id="${roots[0].id}"]`);
    const second = page.locator(`[data-message-id="${roots[1].id}"]`);
    await first.hover();
    await first.locator('[data-message-copy]').click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(roots[0].content);

    await first.hover();
    await first.locator('[data-message-select]').click();
    await expect(page.locator('[data-message-selection-toolbar]')).toContainText('已选择 1 条消息');
    await second.click();
    await expect(page.locator('[data-message-selection-toolbar]')).toContainText('已选择 2 条消息');
    await page.getByRole('button', { name: '生成分享图' }).click();
    await expect(page.locator('[data-share-image-preview]')).toBeVisible();
    await page.getByRole('button', { name: '复制图片' }).click();
    await expect(page.getByText('图片已复制')).toBeVisible();
    await expect.poll(() => page.evaluate(async () => (await navigator.clipboard.read())[0]?.types ?? [])).toContain('image/png');
    const channelDownloadPromise = page.waitForEvent('download');
    await page.locator('[data-share-image-download]').click();
    const channelDownload = await channelDownloadPromise;
    const channelPath = await channelDownload.path();
    expect(readFileSync(channelPath!).subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a');
    await page.getByRole('button', { name: '关闭', exact: true }).click();
    await page.getByRole('button', { name: '取消选择' }).click();

    await first.getByRole('button', { name: '1 条回复' }).click();
    const thread = page.locator('[data-thread-panel]');
    const parent = thread.locator(`[data-message-id="${roots[0].id}"]`);
    const replyRow = thread.locator(`[data-message-id="${threadReply.id}"]`);
    await parent.hover();
    await parent.locator('[data-message-select]').click();
    await replyRow.click();
    await expect(thread.locator('[data-message-selection-toolbar]')).toContainText('已选择 2 条消息');
    await thread.getByRole('button', { name: '生成分享图' }).click();
    await expect(page.locator('[data-share-image-preview]')).toBeVisible();
    const threadDownloadPromise = page.waitForEvent('download');
    await page.locator('[data-share-image-download]').click();
    const threadDownload = await threadDownloadPromise;
    const threadPath = await threadDownload.path();
    expect(readFileSync(threadPath!).subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a');

    expect(queryDatabase(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}'`)).toBe('3');
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers });
  }
});
