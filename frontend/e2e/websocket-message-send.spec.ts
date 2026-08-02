import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const wsBase = apiBase.replace(/^http/, 'ws');
const credentials = { email: 'websocket-message-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface MessageResponse {
  id: string;
  sender_type: string;
  sender_id: string;
  content: string;
  thread_id?: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();

  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'WebSocket E2E' },
  });
  if (!register.ok()) {
    throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  }
  return register.json();
}

async function sendWebSocketEvent(
  page: Page,
  token: string,
  channelID: string,
  command: { type: 'message.send' | 'thread.reply'; payload: Record<string, string> },
  expectedType: 'message.new' | 'thread.message.new',
  expectedContent: string,
  threadID?: string,
): Promise<Record<string, unknown>> {
  return page.evaluate(
    ({ url, accessToken, channel, outbound, expected, content, thread }) =>
      new Promise<Record<string, unknown>>((resolve, reject) => {
        const socket = new WebSocket(`${url}/api/v1/ws?token=${encodeURIComponent(accessToken)}`);
        const timeout = window.setTimeout(() => {
          socket.close();
          reject(new Error(`timed out waiting for ${expected}`));
        }, 15_000);

        const finish = (result: Record<string, unknown>) => {
          window.clearTimeout(timeout);
          socket.close();
          resolve(result);
        };

        socket.onerror = () => {
          window.clearTimeout(timeout);
          reject(new Error('WebSocket connection failed'));
        };
        socket.onopen = () => {
          socket.send(JSON.stringify({ type: 'subscribe', payload: { channel_id: channel } }));
          if (thread) {
            socket.send(JSON.stringify({
              type: 'thread.subscribe',
              payload: { channel_id: channel, thread_id: thread },
            }));
          }
          socket.send(JSON.stringify(outbound));
        };
        socket.onmessage = (event) => {
          const message = JSON.parse(String(event.data)) as {
            type: string;
            payload: Record<string, unknown>;
          };
          if (message.type === 'error') {
            window.clearTimeout(timeout);
            socket.close();
            reject(new Error(JSON.stringify(message.payload)));
            return;
          }
          const actualContent = expected === 'message.new'
            ? message.payload.content
            : (message.payload.message as Record<string, unknown> | undefined)?.content;
          if (message.type === expected && actualContent === content) finish(message.payload);
        };
      }),
    {
      url: wsBase,
      accessToken: token,
      channel: channelID,
      outbound: command,
      expected: expectedType,
      content: expectedContent,
      thread: threadID,
    },
  );
}

test('WebSocket message.send and thread.reply persist and render', async ({ page, request }) => {
  const auth = await authenticate(request);
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers: { authorization: `Bearer ${auth.access_token}` },
    data: { name: `ws-message-e2e-${Date.now()}`, description: 'Issue #69 regression' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as { id: string };

  try {
    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);
    await expect(page.locator('#channel-conversation-panel')).toBeVisible();

    const messageContent = `WS_MESSAGE_${Date.now()}`;
    const messageEvent = await sendWebSocketEvent(
      page,
      auth.access_token,
      channel.id,
      { type: 'message.send', payload: { channel_id: channel.id, content: messageContent } },
      'message.new',
      messageContent,
    );
    expect(messageEvent.sender_type).toBe('user');
    await expect(page.getByText(messageContent, { exact: true })).toBeVisible();

    const messageID = String(messageEvent.id);
    const seedReply = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${messageID}/thread`,
      {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { content: `WS_THREAD_SEED_${Date.now()}` },
      },
    );
    expect(seedReply.ok()).toBe(true);
    const seed = await seedReply.json() as MessageResponse;

    const messageRow = page.locator(`[data-message-id="${messageID}"]`);
    await messageRow.hover();
    await messageRow.getByRole('button', { name: /^Reply to / }).click();
    await expect(page.getByText(seed.content, { exact: true })).toBeVisible();

    const replyContent = `WS_THREAD_REPLY_${Date.now()}`;
    const threadEvent = await sendWebSocketEvent(
      page,
      auth.access_token,
      channel.id,
      {
        type: 'thread.reply',
        payload: {
          channel_id: channel.id,
          thread_id: seed.thread_id!,
          content: replyContent,
        },
      },
      'thread.message.new',
      replyContent,
      seed.thread_id,
    );
    expect((threadEvent.message as Record<string, unknown>).sender_type).toBe('user');
    await expect(page.getByText(replyContent, { exact: true })).toBeVisible();

    const listedMessages = await request.get(
      `${apiBase}/api/v1/channels/${channel.id}/messages?limit=100`,
      { headers: { authorization: `Bearer ${auth.access_token}` } },
    );
    expect(listedMessages.ok()).toBe(true);
    const persistedMessage = (await listedMessages.json() as { messages: MessageResponse[] })
      .messages.find((message) => message.id === messageID);
    expect(persistedMessage).toMatchObject({
      sender_type: 'user',
      content: messageContent,
    });

    const listedReplies = await request.get(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${messageID}/thread`,
      { headers: { authorization: `Bearer ${auth.access_token}` } },
    );
    expect(listedReplies.ok()).toBe(true);
    const persistedReply = (await listedReplies.json() as { messages: MessageResponse[] })
      .messages.find((message) => message.content === replyContent);
    expect(persistedReply).toMatchObject({
      sender_type: 'user',
      sender_id: persistedMessage?.sender_id,
      thread_id: seed.thread_id,
    });
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
  }
});
