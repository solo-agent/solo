import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: `message-time-e2e-${Date.now()}@solo.local`, password: 'SoloE2E-2026!' };

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
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Message Time E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

function bypassUnrelatedFirstRunWizard() {
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-v', 'ON_ERROR_STOP=1', '-c',
    `UPDATE users SET onboarding_completed_at=now() WHERE email='${credentials.email}'`,
  ], { stdio: 'ignore' });
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
  bypassUnrelatedFirstRunWizard();
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

    const createdFollowUp = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
      data: { content: `GROUPED_TIMESTAMP_E2E_${Date.now()}` },
    });
    expect(createdFollowUp.ok()).toBe(true);
    const followUp = await createdFollowUp.json() as MessageResponse;

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      if (!localStorage.getItem('solo.locale')) localStorage.setItem('solo.locale', 'zh-CN');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.clock.setFixedTime(shiftedDay(message.created_at, 0));
    await page.goto(`/dashboard?channel=${channel.id}`);

    const timestamp = page.locator(`[data-message-id="${message.id}"] time`);
    await expect(timestamp).toHaveText(expectedTime(message.created_at, 'zh-CN'));
    await expect(page.locator('[data-message-date-separator]').first()).toContainText('今天');
    await expect(page.locator(`[data-message-id="${followUp.id}"]`)).toHaveAttribute('data-message-grouped', 'true');

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
