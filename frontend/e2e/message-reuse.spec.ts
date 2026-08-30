import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `message-reuse-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse { access_token: string; refresh_token: string }
interface Entity { id: string; name?: string }

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Message Reuse E2E' },
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

test('keeps Agent DMs, channel messages, and thread replies consistent', async ({ page, request }) => {
  const auth = await authenticate(request);
  psql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  const sourceResponse = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `reuse-source-${suffix}`, description: 'Message reuse source' },
  });
  const targetResponse = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `reuse-target-${suffix}`, description: 'Message reuse target' },
  });
  expect(sourceResponse.ok()).toBe(true);
  expect(targetResponse.ok()).toBe(true);
  const source = await sourceResponse.json() as Entity;
  const target = await targetResponse.json() as Entity;
  const content = `REUSE_MESSAGE_${suffix}`;
  const branchTitle = `复用分支 ${suffix}`;
  const agentName = `reuse-agent-${suffix}`;
  const userID = psql(`SELECT id FROM users WHERE email='${credentials.email}'`);
  const agentID = psql('SELECT gen_random_uuid()');
  psql(`INSERT INTO agents (id, name, owner_id, model_name, home_channel_id) VALUES ('${agentID}', '${agentName}', '${userID}', 'e2e', '${source.id}')`);
  psql(`INSERT INTO channel_members (channel_id, member_type, member_id, role) VALUES ('${source.id}', 'agent', '${agentID}', 'member')`);
  let dmID = '';

  try {
    const messageResponse = await request.post(`${apiBase}/api/v1/channels/${source.id}/messages`, {
      headers,
      data: { content },
    });
    expect(messageResponse.ok()).toBe(true);
    const message = await messageResponse.json() as Entity;

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'zh-CN');
      localStorage.setItem('solo.message-shortcuts-seen', '1');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${source.id}`);

    await page.getByRole('button', { name: agentName, exact: true }).click();
    await expect(page).toHaveURL(/\?dm=[0-9a-f-]+/);
    await expect(page.getByRole('button', { name: agentName, exact: true })).toHaveAttribute('aria-current', 'true');
    dmID = new URL(page.url()).searchParams.get('dm') ?? '';
    expect(dmID).toBeTruthy();
    expect(psql(`SELECT count(*) FROM dm_members WHERE channel_id='${dmID}'`)).toBe('2');

    const dmContent = `REUSE_DM_${suffix}`;
    const dmMessageID = psql('SELECT gen_random_uuid()');
    psql(`INSERT INTO messages (id, channel_id, sender_type, sender_id, content) VALUES ('${dmMessageID}', '${dmID}', 'user', '${userID}', '${dmContent}')`);
    await page.goto(`/dashboard?dm=${dmID}`);
    const dmMessage = page.locator(`[data-message-id="${dmMessageID}"]`);
    await expect(dmMessage).toContainText(dmContent);
    await dmMessage.hover();
    await dmMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '收藏消息' }).click();
    await expect(page.getByText('消息已收藏')).toBeVisible();
    expect(psql(`SELECT count(*) FROM message_favorites WHERE message_id='${dmMessageID}'`)).toBe('1');

    await dmMessage.hover();
    await dmMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '转发消息' }).click();
    await page.getByLabel('转发到频道').selectOption(target.id);
    await page.getByRole('button', { name: '转发消息', exact: true }).last().click();
    await expect(page.getByText('消息已转发')).toBeVisible();
    expect(psql(`SELECT concat_ws(':', metadata->>'forwarded_message_id', metadata->>'forwarded_channel_type') FROM messages WHERE channel_id='${target.id}' AND content='${dmContent}'`))
      .toBe(`${dmMessageID}:dm`);

    await page.goto(`/dashboard?channel=${source.id}`);

    const sourceMessage = page.locator(`[data-message-id="${message.id}"]`);
    await expect(sourceMessage).toContainText(content);
    await sourceMessage.hover();
    await sourceMessage.getByLabel('更多消息操作').click();
    const menu = page.getByRole('menu');
    const menuBox = await menu.boundingBox();
    const viewport = page.viewportSize();
    expect(menuBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(menuBox!.y).toBeGreaterThanOrEqual(0);
    expect(menuBox!.y + menuBox!.height).toBeLessThanOrEqual(viewport!.height);
    await menu.getByRole('menuitem', { name: '收藏消息' }).click();
    await expect(page.getByText('消息已收藏')).toBeVisible();
    expect(psql(`SELECT count(*) FROM message_favorites WHERE message_id='${message.id}'`)).toBe('1');

    await page.goto('/home');
    await page.getByRole('link', { name: '收藏消息' }).click();
    await expect(page).toHaveURL('/favorites');
    await expect(page.getByText(content)).toBeVisible();
    await page.locator('article').filter({ hasText: content }).getByRole('button', { name: '打开原消息' }).click();
    await expect(page).toHaveURL(new RegExp(`channel=${source.id}.*message=${message.id}`));

    await sourceMessage.hover();
    await sourceMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '转发消息' }).click();
    await page.getByLabel('转发到频道').selectOption(target.id);
    await page.getByRole('button', { name: '转发消息', exact: true }).last().click();
    await expect(page.getByText('消息已转发')).toBeVisible();
    expect(psql(`SELECT concat_ws(':', content_type, jsonb_extract_path_text(metadata, 'forwarded_message_id'), cardinality(mentioned_agent_ids)) FROM messages WHERE channel_id='${target.id}' AND content='${content}'`))
      .toBe(`forwarded:${message.id}:0`);

    await page.goto(`/dashboard?channel=${target.id}`);
    await expect(page.getByText(content)).toBeVisible();
    await expect(page.getByText(new RegExp(`来自 #${source.name}`))).toBeVisible();

    await page.goto(`/dashboard?channel=${source.id}`);
    await sourceMessage.hover();
    await sourceMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '从消息创建分支' }).click();
    await page.getByLabel('分支标题').fill(branchTitle);
    await page.getByRole('button', { name: '创建分支', exact: true }).last().click();
    await expect(page.getByText('分支已创建')).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`channel=${source.id}.*view=thinking.*node=`));
    const nodeID = new URL(page.url()).searchParams.get('node');
    expect(nodeID).toBeTruthy();
    expect(psql(`SELECT source_message_id::text || ':' || source || ':' || fork_handoff_pending FROM thinking_nodes WHERE id='${nodeID}'`))
      .toBe(`${message.id}:manual:false`);
    await page.getByRole('button', { name: /节点上下文/ }).click();
    await page.getByRole('link', { name: '来源消息' }).click();
    await expect(page).toHaveURL(new RegExp(`channel=${source.id}.*message=${message.id}`));
    await expect(page.locator(`[data-message-id="${message.id}"]`).getByRole('link', { name: '1 个分支' })).toBeVisible();

    const threadRootResponse = await request.post(`${apiBase}/api/v1/channels/${source.id}/messages`, {
      headers,
      data: { content: `THREAD_ROOT_${suffix}` },
    });
    expect(threadRootResponse.ok()).toBe(true);
    const threadRoot = await threadRootResponse.json() as Entity;
    const threadContent = `THREAD_REPLY_${suffix}`;
    const threadReplyResponse = await request.post(`${apiBase}/api/v1/channels/${source.id}/messages/${threadRoot.id}/thread`, {
      headers,
      data: { content: threadContent },
    });
    expect(threadReplyResponse.ok()).toBe(true);
    const threadReply = await threadReplyResponse.json() as Entity;

    await page.goto(`/dashboard?channel=${source.id}&panel=thread&thread=${threadRoot.id}`);
    const replyMessage = page.locator(`[data-message-id="${threadReply.id}"]`);
    await expect(replyMessage).toContainText(threadContent);
    await replyMessage.hover();
    await replyMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '收藏消息' }).click();
    await expect(page.getByText('消息已收藏')).toBeVisible();
    expect(psql(`SELECT count(*) FROM message_favorites WHERE message_id='${threadReply.id}'`)).toBe('1');

    await replyMessage.hover();
    await replyMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '转发消息' }).click();
    await page.getByLabel('转发到频道').selectOption(target.id);
    await page.getByRole('button', { name: '转发消息', exact: true }).last().click();
    await expect(page.getByText('消息已转发')).toBeVisible();
    expect(psql(`SELECT concat_ws(':', metadata->>'forwarded_message_id', metadata->>'forwarded_thread_root_message_id') FROM messages WHERE channel_id='${target.id}' AND content='${threadContent}'`))
      .toBe(`${threadReply.id}:${threadRoot.id}`);

    await replyMessage.hover();
    await replyMessage.getByLabel('更多消息操作').click();
    await page.getByRole('menu').getByRole('menuitem', { name: '从消息创建分支' }).click();
    await page.getByLabel('分支标题').fill(`讨论串分支 ${suffix}`);
    await page.getByRole('button', { name: '创建分支', exact: true }).last().click();
    await expect(page.getByText('分支已创建')).toBeVisible();
    expect(psql(`SELECT count(*) FROM thinking_nodes WHERE source_message_id='${threadReply.id}'`)).toBe('1');
  } finally {
    if (dmID) psql(`DELETE FROM channels WHERE id='${dmID}'`);
    psql(`DELETE FROM agents WHERE id='${agentID}'`);
    await request.delete(`${apiBase}/api/v1/channels/${target.id}`, { headers });
    await request.delete(`${apiBase}/api/v1/channels/${source.id}`, { headers });
  }
});
