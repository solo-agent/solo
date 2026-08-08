import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: { id: string };
}

interface MessageResponse {
  id: string;
  content: string;
  thread_id?: string;
}

interface MessageListResponse {
  messages: MessageResponse[];
  has_more: boolean;
}

async function register(request: APIRequestContext, label: string): Promise<AuthResponse> {
  const suffix = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const response = await registerVerified(request, apiBase, {
    data: {
      email: `ws-recovery-${label}-${suffix}@solo.local`,
      password: 'SoloE2E-2026!',
      display_name: `WS Recovery ${label}`,
    },
  });
  if (!response.ok()) {
    throw new Error(`E2E registration failed: ${response.status()} ${await response.text()}`);
  }
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse): Promise<void> {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

function authorization(auth: AuthResponse): { authorization: string } {
  return { authorization: `Bearer ${auth.access_token}` };
}

test('foreground and network recovery restore Channel and Thread messages', async ({ page, request, context }) => {
  const auth = await register(request, 'channel');
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers: authorization(auth),
    data: { name: `ws-recovery-${Date.now()}`, description: 'WebSocket recovery E2E' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as { id: string };

  try {
    const rootContent = `WS_RECOVERY_ROOT_${Date.now()}`;
    const rootResponse = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
      headers: authorization(auth),
      data: { content: rootContent },
    });
    expect(rootResponse.ok()).toBe(true);
    const root = await rootResponse.json() as MessageResponse;

    const seedContent = `WS_RECOVERY_THREAD_SEED_${Date.now()}`;
    const seedResponse = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${root.id}/thread`,
      { headers: authorization(auth), data: { content: seedContent } },
    );
    expect(seedResponse.ok()).toBe(true);
    const seed = await seedResponse.json() as MessageResponse;

    let socketCount = 0;
    let pingCount = 0;
    page.on('websocket', (socket) => {
      if (!socket.url().includes('/api/v1/ws')) return;
      socketCount++;
      socket.on('framereceived', ({ payload }) => {
        try {
          if (JSON.parse(String(payload)).type === 'ping') pingCount++;
        } catch {
          // Ignore non-JSON frames.
        }
      });
    });

    await authenticatePage(page, auth);
    await page.goto(`/dashboard?channel=${channel.id}`);
    await expect(page.locator('#channel-conversation-panel')).toBeVisible();
    await expect(page.getByText(rootContent, { exact: true })).toBeVisible();

    const rootRow = page.locator(`[data-message-id="${root.id}"]`);
    await rootRow.hover();
    await rootRow.getByRole('button', { name: /^Reply to / }).click();
    await expect(page.getByText(seedContent, { exact: true })).toBeVisible();

    await expect.poll(() => pingCount, { timeout: 35_000 }).toBeGreaterThan(0);
    const socketsBeforeForeground = socketCount;
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    await expect.poll(() => socketCount).toBeGreaterThan(socketsBeforeForeground);

    await context.setOffline(true);
    await expect(
      page.getByRole('alert').filter({ hasText: /Connection lost|Reconnecting/ }).first(),
    ).toBeVisible();

    const channelMissedContent = `WS_RECOVERY_CHANNEL_MISSED_${Date.now()}`;
    const channelMissedResponse = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages`,
      { headers: authorization(auth), data: { content: channelMissedContent } },
    );
    expect(channelMissedResponse.ok()).toBe(true);
    const channelMissed = await channelMissedResponse.json() as MessageResponse;

    const threadMissedContent = `WS_RECOVERY_THREAD_MISSED_${Date.now()}`;
    const threadMissedResponse = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${root.id}/thread`,
      { headers: authorization(auth), data: { content: threadMissedContent } },
    );
    expect(threadMissedResponse.ok()).toBe(true);
    const threadMissed = await threadMissedResponse.json() as MessageResponse;

    await context.setOffline(false);
    await expect(page.getByText(threadMissedContent, { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Close thread panel' }).click();
    await expect(page.getByText(channelMissedContent, { exact: true })).toBeVisible();

    const listedChannel = await request.get(
      `${apiBase}/api/v1/channels/${channel.id}/messages?after=${root.id}&limit=50`,
      { headers: authorization(auth) },
    );
    expect(listedChannel.ok()).toBe(true);
    expect(await listedChannel.json() as MessageListResponse).toMatchObject({
      messages: [{ id: channelMissed.id, content: channelMissedContent }],
      has_more: false,
    });

    const listedThread = await request.get(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${root.id}/thread?after=${seed.id}&limit=50`,
      { headers: authorization(auth) },
    );
    expect(listedThread.ok()).toBe(true);
    expect(await listedThread.json() as MessageListResponse).toMatchObject({
      messages: [{ id: threadMissed.id, content: threadMissedContent }],
      has_more: false,
    });
  } finally {
    await context.setOffline(false);
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, {
      headers: authorization(auth),
    });
  }
});

test('network recovery restores missed DM messages', async ({ page, request, context }) => {
  const auth = await register(request, 'dm-owner');
  const peer = await register(request, 'dm-peer');
  const createdDM = await request.post(`${apiBase}/api/v1/dm`, {
    headers: authorization(auth),
    data: { member_type: 'user', member_id: peer.user.id },
  });
  expect(createdDM.ok()).toBe(true);
  const dm = await createdDM.json() as { id: string };

  const seedContent = `WS_RECOVERY_DM_SEED_${Date.now()}`;
  const seedResponse = await request.post(`${apiBase}/api/v1/dm/${dm.id}/messages`, {
    headers: authorization(peer),
    data: { content: seedContent },
  });
  expect(seedResponse.ok()).toBe(true);
  const seed = await seedResponse.json() as MessageResponse;

  await authenticatePage(page, auth);
  await page.goto(`/dashboard?dm=${dm.id}`);
  await expect(page.getByText(seedContent, { exact: true })).toBeVisible();

  await context.setOffline(true);
  await expect(
    page.getByRole('alert').filter({ hasText: /Connection lost|Reconnecting/ }).first(),
  ).toBeVisible();

  const missedContent = `WS_RECOVERY_DM_MISSED_${Date.now()}`;
  const missedResponse = await request.post(`${apiBase}/api/v1/dm/${dm.id}/messages`, {
    headers: authorization(peer),
    data: { content: missedContent },
  });
  expect(missedResponse.ok()).toBe(true);
  const missed = await missedResponse.json() as MessageResponse;

  await context.setOffline(false);
  await expect(page.getByText(missedContent, { exact: true })).toBeVisible();

  const listed = await request.get(
    `${apiBase}/api/v1/dm/${dm.id}/messages?after=${seed.id}&limit=50`,
    { headers: authorization(auth) },
  );
  expect(listed.ok()).toBe(true);
  expect(await listed.json() as MessageListResponse).toMatchObject({
    messages: [{ id: missed.id, content: missedContent }],
    has_more: false,
  });
});
