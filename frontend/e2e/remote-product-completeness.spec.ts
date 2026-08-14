import { expect, test, type APIRequestContext, type APIResponse, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const publicWorkspaceID = '00000000-0000-0000-0000-000000000001';

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: { id: string; email: string; display_name: string };
}
interface Workspace { id: string; name: string; role: 'owner' | 'admin' | 'member' }
interface Channel { id: string; name: string }
interface Message { id: string; content: string; thread_id?: string }
interface Attachment { id: string; filename: string; url: string; thumbnail_url?: string }
interface ChannelMember { member_id: string; workspace_role?: 'owner' | 'admin' | 'member' }

async function register(request: APIRequestContext, name: string): Promise<AuthResponse> {
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
  const response = await registerVerified(request, apiBase, {
    data: { email: `${name}-${suffix}@remote-product.invalid`, password: 'SoloRemote-2026!', display_name: `${name} ${suffix.slice(-5)}` },
  });
  if (!response.ok()) throw new Error(`register ${name}: ${response.status()} ${await response.text()}`);
  return response.json();
}

function headers(auth: AuthResponse, workspaceID = publicWorkspaceID) {
  return { authorization: `Bearer ${auth.access_token}`, 'X-Workspace-ID': workspaceID };
}

async function call<T>(
  request: APIRequestContext,
  auth: AuthResponse,
  method: 'get' | 'post' | 'put' | 'patch' | 'delete',
  path: string,
  workspaceID?: string,
  data?: unknown,
): Promise<T> {
  const options = { headers: headers(auth, workspaceID), ...(data === undefined ? {} : { data }) };
  const response = await request[method](`${apiBase}${path}`, options);
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function raw(
  request: APIRequestContext,
  auth: AuthResponse,
  method: 'post' | 'put' | 'patch' | 'delete',
  path: string,
  workspaceID?: string,
  data?: unknown,
): Promise<APIResponse> {
  return request[method](`${apiBase}${path}`, { headers: headers(auth, workspaceID), ...(data === undefined ? {} : { data }) });
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function authenticatePage(page: Page, auth: AuthResponse, workspaceID: string) {
  await page.addInitScript(({ accessToken, refreshToken, activeWorkspace }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo_active_workspace_id', activeWorkspace);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token, activeWorkspace: workspaceID });
}

async function sendBlockedWebSocketCommand(
  page: Page,
  auth: AuthResponse,
  workspaceID: string,
  command: { type: 'message.send' | 'thread.reply'; payload: Record<string, string> },
): Promise<Record<string, string>> {
  const wsBase = apiBase.replace(/^http/, 'ws');
  return page.evaluate(
    ({ url, accessToken, activeWorkspace, outbound }) => new Promise<Record<string, string>>((resolve, reject) => {
      const socket = new WebSocket(
        `${url}/api/v1/ws?token=${encodeURIComponent(accessToken)}&workspace_id=${encodeURIComponent(activeWorkspace)}`,
      );
      const timeout = window.setTimeout(() => {
        socket.close();
        reject(new Error('timed out waiting for WebSocket moderation error'));
      }, 15_000);
      socket.onerror = () => {
        window.clearTimeout(timeout);
        reject(new Error('WebSocket connection failed'));
      };
      socket.onopen = () => socket.send(JSON.stringify(outbound));
      socket.onmessage = (event) => {
        const message = JSON.parse(String(event.data)) as { type: string; payload: Record<string, string> };
        if (message.type !== 'error') return;
        window.clearTimeout(timeout);
        socket.close();
        resolve(message.payload);
      };
    }),
    { url: wsBase, accessToken: auth.access_token, activeWorkspace: workspaceID, outbound: command },
  );
}

test.describe('remote product completeness', () => {
  test('uploads real files and enforces private/Public Workspace governance through UI, API, WebSocket, and PostgreSQL', async ({ browser, page, request }) => {
    const owner = await register(request, 'Owner');
    const admin = await register(request, 'Admin');
    const member = await register(request, 'Member');
    const suffix = Date.now().toString(36);
    let workspace: Workspace | undefined;
    try {
      workspace = await call<Workspace>(request, owner, 'post', '/api/v1/workspaces', undefined, { name: `Remote Product ${suffix}`, icon: 'RP' });
      await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, workspace.id, { user_id: admin.user.id, role: 'admin' });
      await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, workspace.id, { user_id: member.user.id, role: 'member' });
      const channel = await call<Channel>(request, owner, 'post', '/api/v1/channels', workspace.id, { name: `governance-${suffix}` });
      const channelMembers = await call<ChannelMember[]>(request, owner, 'get', `/api/v1/channels/${channel.id}/members`, workspace.id);
      expect(channelMembers.find((item) => item.member_id === owner.user.id)?.workspace_role).toBe('owner');
      expect(channelMembers.find((item) => item.member_id === admin.user.id)?.workspace_role).toBe('admin');
      expect(channelMembers.find((item) => item.member_id === member.user.id)?.workspace_role).toBe('member');

      const publicRemoval = await raw(request, owner, 'delete', `/api/v1/workspaces/${publicWorkspaceID}/members/${member.user.id}`, publicWorkspaceID);
      expect(publicRemoval.status()).toBe(400);
      expect(await publicRemoval.text()).toContain('public Workspace members cannot be removed');

      const adminPromotion = await raw(request, admin, 'patch', `/api/v1/workspaces/${workspace.id}/members/${member.user.id}`, workspace.id, { role: 'admin' });
      expect(adminPromotion.status()).toBe(403);

      const root = await call<Message>(request, member, 'post', `/api/v1/channels/${channel.id}/messages`, workspace.id, { content: `PIN_ME_${suffix}` });
      const threadSeed = await call<Message>(request, member, 'post', `/api/v1/channels/${channel.id}/messages/${root.id}/thread`, workspace.id, { content: `THREAD_SEED_${suffix}` });
      expect((await raw(request, member, 'put', `/api/v1/channels/${channel.id}/messages/${root.id}/pin`, workspace.id, {})).status()).toBe(403);
      expect((await raw(request, member, 'patch', `/api/v1/channels/${channel.id}/moderation`, workspace.id, { posting_policy: 'admins_only' })).status()).toBe(403);
      expect((await raw(request, admin, 'put', `/api/v1/channels/${channel.id}/moderation/mutes/${owner.user.id}`, workspace.id, { reason: 'forbidden owner mute' })).status()).toBe(403);
      expect((await raw(request, admin, 'put', `/api/v1/channels/${channel.id}/moderation/mutes/${admin.user.id}`, workspace.id, { reason: 'forbidden admin mute' })).status()).toBe(403);

      await authenticatePage(page, owner, workspace.id);
      await page.goto(`/dashboard?channel=${channel.id}`);
      await expect(page.getByText('Pinned messages', { exact: true })).toHaveCount(0);
      const rootMessage = page.locator(`[data-message-id="${root.id}"]`);
      await rootMessage.hover();
      await expect(rootMessage.getByRole('button', { name: 'Pin message' })).toBeVisible();
      await rootMessage.getByRole('button', { name: 'Pin message' }).click();
      await expect(page.getByText('Message pinned.', { exact: true })).toBeVisible();
      await expect(rootMessage.getByRole('button', { name: 'Unpin message' })).toBeVisible();
      await expect(page.getByText('Pinned messages', { exact: true })).toBeVisible();
      await expect(page.getByText(`PIN_ME_${suffix}`, { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Channel management' }).click();
      const moderationDialog = page.getByRole('dialog');
      await expect(moderationDialog.getByText('Who can post')).toBeVisible();

      const memberContext = await browser.newContext();
      const memberPage = await memberContext.newPage();
      await authenticatePage(memberPage, member, workspace.id);
      await memberPage.goto(`/dashboard?channel=${channel.id}`);
      const memberComposer = memberPage.getByPlaceholder('Type a message...');
      await expect(memberComposer).toBeEnabled();

      const muteMember = moderationDialog.getByRole('button', { name: `Mute ${member.user.display_name}` });
      await expect(muteMember).toHaveAttribute('data-mute-state', 'unmuted');
      await muteMember.click();
      await expect(page.getByText(`${member.user.display_name} is now muted.`, { exact: true })).toBeVisible();
      await expect(moderationDialog.getByRole('button', { name: `Unmute ${member.user.display_name}` })).toHaveAttribute('data-mute-state', 'muted');
      await expect(memberComposer).toBeDisabled();

      expect((await raw(request, member, 'post', `/api/v1/channels/${channel.id}/messages`, workspace.id, { content: 'blocked message' })).status()).toBe(403);
      expect((await raw(request, member, 'post', `/api/v1/channels/${channel.id}/messages/${root.id}/thread`, workspace.id, { content: 'blocked reply' })).status()).toBe(403);
      expect((await raw(request, member, 'post', `/api/v1/channels/${channel.id}/tasks`, workspace.id, { title: 'blocked task' })).status()).toBe(403);
      expect((await sendBlockedWebSocketCommand(memberPage, member, workspace.id, {
        type: 'message.send', payload: { channel_id: channel.id, content: 'blocked WebSocket message' },
      })).code).toBe('FORBIDDEN');
      expect((await sendBlockedWebSocketCommand(memberPage, member, workspace.id, {
        type: 'thread.reply',
        payload: { channel_id: channel.id, thread_id: threadSeed.thread_id!, content: 'blocked WebSocket reply' },
      })).code).toBe('FORBIDDEN');

      await moderationDialog.getByRole('button', { name: `Unmute ${member.user.display_name}` }).click();
      await expect(page.getByText(`${member.user.display_name} can post again.`, { exact: true })).toBeVisible();
      await moderationDialog.getByRole('button', { name: 'Close', exact: true }).click();
      await call(request, admin, 'patch', `/api/v1/channels/${channel.id}/moderation`, workspace.id, { posting_policy: 'admins_only' });
      expect((await raw(request, member, 'post', `/api/v1/channels/${channel.id}/messages`, workspace.id, { content: 'still blocked' })).status()).toBe(403);
      await call(request, owner, 'patch', `/api/v1/channels/${channel.id}/moderation`, workspace.id, { posting_policy: 'everyone' });
      await expect(memberComposer).toBeEnabled();
      await call(request, member, 'delete', `/api/v1/channels/${channel.id}/messages/${root.id}`, workspace.id);
      await expect(page.getByText('Pinned messages', { exact: true })).toHaveCount(0);

      const textMarker = `REMOTE_ATTACHMENT_${suffix.toUpperCase()}`;
      const upload = await request.post(`${apiBase}/api/v1/attachments/upload`, {
        headers: headers(member, workspace.id),
        multipart: { file: { name: 'remote-marker.txt', mimeType: 'text/plain', buffer: Buffer.from(textMarker) } },
      });
      expect(upload.ok()).toBe(true);
      const attachment = await upload.json() as Attachment;
      const attached = await call<Message>(request, member, 'post', `/api/v1/channels/${channel.id}/messages`, workspace.id, {
        content: `Attachment ${suffix}`, attachment_ids: [attachment.id],
      });
      expect(attached.id).toMatch(/^[0-9a-f-]{36}$/);
      const download = await request.get(`${apiBase}${attachment.url}`, { headers: headers(member, workspace.id) });
      expect(await download.text()).toBe(textMarker);

      const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64');
      const avatarUpload = await request.post(`${apiBase}/api/v1/attachments/upload`, {
        headers: headers(member, workspace.id),
        multipart: { file: { name: 'avatar.png', mimeType: 'image/png', buffer: png } },
      });
      expect(avatarUpload.ok()).toBe(true);
      const avatar = await avatarUpload.json() as Attachment;
      const avatarURL = avatar.thumbnail_url || avatar.url;
      await call(request, member, 'patch', '/api/v1/users/me', workspace.id, { avatar_url: avatarURL });

      const avatarDownload = await request.get(`${apiBase}${avatarURL}`, { headers: headers(owner, workspace.id) });
      expect(avatarDownload.ok()).toBe(true);
      expect(avatarDownload.headers()['content-type']).toContain('image/png');
      const avatarMessage = await call<Message>(request, member, 'post', `/api/v1/channels/${channel.id}/messages`, workspace.id, {
        content: `AVATAR_VISIBLE_${suffix}`,
      });
      await page.reload();
      const sharedAvatar = page.locator(`[data-message-id="${avatarMessage.id}"]`).getByLabel(member.user.display_name).locator('img');
      await expect(sharedAvatar).toBeVisible();
      await expect.poll(() => sharedAvatar.evaluate((image) => image.complete && image.naturalWidth > 0)).toBe(true);

      const persisted = databaseJSON<{ pin_count: number; mute_count: number; attachment_count: number; avatar_url: string; policy: string }>(`
        SELECT json_build_object(
          'pin_count',(SELECT count(*) FROM channel_message_pins WHERE channel_id='${channel.id}'),
          'mute_count',(SELECT count(*) FROM channel_member_mutes WHERE channel_id='${channel.id}'),
          'attachment_count',(SELECT count(*) FROM attachments WHERE id IN ('${attachment.id}','${avatar.id}')),
          'avatar_url',(SELECT avatar_url FROM users WHERE id='${member.user.id}'),
          'policy',(SELECT posting_policy FROM channels WHERE id='${channel.id}')
        )::text
      `);
      expect(persisted).toEqual({ pin_count: 0, mute_count: 0, attachment_count: 2, avatar_url: avatarURL, policy: 'everyone' });

      await call(request, admin, 'put', `/api/v1/channels/${channel.id}/moderation/mutes/${member.user.id}`, workspace.id, { reason: 'remove cleanup' });
      await call(request, admin, 'delete', `/api/v1/workspaces/${workspace.id}/members/${member.user.id}`, workspace.id);
      expect(databaseJSON<{ members: number; mutes: number }>(`
        SELECT json_build_object(
          'members',(SELECT count(*) FROM workspace_members WHERE workspace_id='${workspace.id}' AND user_id='${member.user.id}'),
          'mutes',(SELECT count(*) FROM channel_member_mutes WHERE channel_id='${channel.id}' AND user_id='${member.user.id}')
        )::text
      `)).toEqual({ members: 0, mutes: 0 });
      await memberContext.close();
    } finally {
      if (workspace) await raw(request, owner, 'delete', `/api/v1/workspaces/${workspace.id}`, workspace.id).catch(() => undefined);
    }
  });
});
