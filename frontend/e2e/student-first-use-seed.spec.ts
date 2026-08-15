import { test } from '@playwright/test';

test('student first-use seed', async ({ page }) => {
  await page.goto('/');
  if (process.env.PWDEBUG) await page.pause();
});
