import { expect, test, type APIRequestContext } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'channel-split-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Channel Split E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

test('conversation and workspace divider resizes with pointer and keyboard', async ({ page, request }) => {
  const auth = await authenticate(request);
  const created = await request.post(`${apiBase}/api/v1/channels`, {
    headers: { authorization: `Bearer ${auth.access_token}` },
    data: { name: `split-e2e-${Date.now()}`, description: 'Resizable channel split E2E' },
  });
  expect(created.ok()).toBe(true);
  const channel = await created.json() as { id: string };

  try {
    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);

    const separator = page.getByRole('separator', { name: 'Resize conversation and workspace panels' });
    const conversation = page.locator('#channel-conversation-panel');
    const workspace = page.locator('#channel-workspace-panel');
    await expect(separator).toBeVisible();

    const initialConversation = await conversation.boundingBox();
    const initialWorkspace = await workspace.boundingBox();
    const separatorBox = await separator.boundingBox();
    expect(initialConversation && initialWorkspace && separatorBox).toBeTruthy();

    await page.mouse.move(separatorBox!.x + separatorBox!.width / 2, separatorBox!.y + separatorBox!.height / 2);
    await page.mouse.down();
    await page.mouse.move(separatorBox!.x - 100, separatorBox!.y + separatorBox!.height / 2, { steps: 50 });
    await page.mouse.up();

    await expect.poll(async () => (await conversation.boundingBox())?.width ?? 0).toBeLessThan(initialConversation!.width - 60);
    await expect.poll(async () => (await workspace.boundingBox())?.width ?? 0).toBeGreaterThan(initialWorkspace!.width + 60);

    const pointerWidth = (await conversation.boundingBox())!.width;
    await separator.focus();
    await page.keyboard.press('ArrowRight');
    await expect.poll(async () => (await conversation.boundingBox())?.width ?? 0).toBeGreaterThan(pointerWidth + 10);
  } finally {
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
  }
});
