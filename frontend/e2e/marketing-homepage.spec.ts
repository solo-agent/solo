import { expect, test } from '@playwright/test';
import { registerVerified } from './support/auth';

test('marketing homepage presents Solo and links into the real product', async ({ page, request }) => {
  const api = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
  await expect((await request.get(`${api}/readyz`)).ok()).toBeTruthy();

  await page.goto('/');
  await expect(page).toHaveTitle(/Multiplayer Workspace for AI Agents/);
  await expect(page.getByRole('heading', { name: 'Give your ideas a team.' })).toBeVisible();
  await expect(page.getByAltText('Solo workspace showing a shared Channel conversation and a team of Agents collaborating on tasks')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Solo' })).toHaveAttribute('href', '/auth/login');

  const start = page.getByRole('link', { name: 'Start building' });
  await expect(start).toHaveAttribute('href', '/auth/register');
  await start.click();
  await expect(page).toHaveURL(/\/auth\/register$/);
  await expect(page.getByRole('heading', { name: 'Create account' })).toBeVisible();

  const registration = await registerVerified(request, api, {
    data: {
      email: `marketing-home-${Date.now()}@workspace-e2e.invalid`,
      password: 'SoloMarketing-2026!',
      display_name: 'Marketing Homepage E2E',
    },
  });
  expect(registration.ok()).toBeTruthy();
  const auth = await registration.json() as { access_token: string; refresh_token: string };
  await page.evaluate(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });

  const currentUser = page.waitForResponse((response) => response.url().endsWith('/api/v1/users/me'));
  await page.goto('/');
  await currentUser;
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { name: 'Give your ideas a team.' })).toBeVisible();
});
