import { expect, test, type APIRequestContext } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'message-time-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface MessageResponse {
  id: string;
  content: string;
  created_at: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Message Time E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

function shiftedDay(value: string, days: number) {
  const date = new Date(value);
  date.setDate(date.getDate() + days);
  date.setHours(12, 0, 0, 0);
  return date;
}

function expectedTime(value: string, locale: 'en' | 'zh-CN') {
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(new Date(value));
}

test('channel message timestamps use localized calendar labels and a 24-hour clock', async ({ page, request }) => {
  const auth = await authenticate(request);
  const createdChannel = await request.post(`${apiBase}/api/v1/channels`, {
    headers: { authorization: `Bearer ${auth.access_token}` },
    data: { name: `message-time-e2e-${Date.now()}`, description: 'Message timestamp E2E' },
  });
  expect(createdChannel.ok()).toBe(true);
  const channel = await createdChannel.json() as { id: string };

  try {
    const createdMessage = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
      data: { content: `TIMESTAMP_E2E_${Date.now()}` },
    });
    expect(createdMessage.ok()).toBe(true);
    const message = await createdMessage.json() as MessageResponse;

    const listed = await request.get(`${apiBase}/api/v1/channels/${channel.id}/messages?limit=100`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
    expect(listed.ok()).toBe(true);
    const persisted = (await listed.json() as { messages: MessageResponse[] }).messages.find((item) => item.id === message.id);
    expect(persisted?.content).toBe(message.content);
    expect(new Date(persisted!.created_at).getTime()).toBe(new Date(message.created_at).getTime());

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      if (!localStorage.getItem('solo.locale')) localStorage.setItem('solo.locale', 'zh-CN');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.clock.setFixedTime(shiftedDay(message.created_at, 0));
    await page.goto(`/dashboard?channel=${channel.id}`);

    const timestamp = page.locator(`[data-message-id="${message.id}"] time`);
    await expect(timestamp).toHaveText(expectedTime(message.created_at, 'zh-CN'));

    await page.clock.setFixedTime(shiftedDay(message.created_at, 1));
    await page.reload();
    await expect(timestamp).toHaveText(`昨天 ${expectedTime(message.created_at, 'zh-CN')}`);

    await page.clock.setFixedTime(shiftedDay(message.created_at, 3));
    await page.reload();
    const zhDate = new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric' }).format(new Date(message.created_at));
    await expect(timestamp).toHaveText(`${zhDate} ${expectedTime(message.created_at, 'zh-CN')}`);

    await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
    await page.reload();
    const enDate = new Intl.DateTimeFormat('en', { month: 'short', day: 'numeric' }).format(new Date(message.created_at));
    await expect(timestamp).toHaveText(`${enDate} ${expectedTime(message.created_at, 'en')}`);
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
  }
});
