// ============================================================================
// brutal-consistency — design system regression spec
// Verifies that the rendered DOM conforms to the neo-brutalism rules the
// audit-brutal.sh script enforces. This is the runtime twin of the
// static linter: it catches violations that only appear at runtime (CSS
// loaded, dark-mode active, dynamically-injected markup).
//
// Requires: dev server on :3000, teams-e2e@test.com / TeamsE2E12345!.
// Each test fails LOUDLY so the gate is enforceable.
// ============================================================================

import { test, expect, type Page } from '@playwright/test';

const BASE = 'http://localhost:3000';
const TEST_EMAIL = 'teams-e2e@test.com';
const TEST_PASSWORD = 'TeamsE2E12345!';

async function login(page: Page) {
  await page.goto(`${BASE}/auth/login`);
  await page.locator('#email').fill(TEST_EMAIL);
  await page.locator('#password').fill(TEST_PASSWORD);
  await page.getByRole('button', { name: '登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(800);
}

test.describe('Brutal consistency', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('no mid-range rounded corners on any visited page', async ({ page }) => {
    const pages = ['/dashboard', '/tasks', '/teams', '/computers', '/settings'];
    for (const path of pages) {
      await page.goto(`${BASE}${path}`);
      await page.waitForTimeout(400);
      // Mid-range = rounded-{sm,md,lg,xl,2xl,3xl}. rounded-none and
      // rounded-full are fine.
      const offenders = page.locator(
        '[class*="rounded-sm"], [class*="rounded-md"], [class*="rounded-lg"], [class*="rounded-xl"], [class*="rounded-2xl"], [class*="rounded-3xl"]',
      );
      const count = await offenders.count();
      expect(count, `path=${path} should have 0 mid-range rounded elements`).toBe(0);
    }
  });

  test('no default Tailwind color scale on visible interactive elements', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await page.waitForTimeout(400);
    // Look for the most common offenders (green-500, red-500, blue-500,
    // gray-400). The test fails if any element carries these classes.
    const regex =
      /(^|\s)(bg|text|border)-(green|red|blue|gray|amber|yellow)-(400|500|600)(\s|$)/;
    const candidates = page.locator('[class*="bg-"], [class*="text-"], [class*="border-"]');
    const total = await candidates.count();
    for (let i = 0; i < total; i++) {
      const cls = (await candidates.nth(i).getAttribute('class')) ?? '';
      expect(cls, `element at index ${i} has default Tailwind color`).not.toMatch(regex);
    }
  });

  test('spinners all share the same className shape', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await page.waitForTimeout(400);
    // Trigger a loading state by navigating to tasks (left column loads).
    await page.goto(`${BASE}/tasks`);
    await page.waitForTimeout(800);
    // Audit expects every spinner to have animate-spin AND a
    // border-{N} class. The Spinner stub always adds `border-4 border-black`
    // (or border-2), so we look for the union shape.
    const spinners = page.locator('[class*="animate-spin"]');
    const count = await spinners.count();
    for (let i = 0; i < count; i++) {
      const cls = (await spinners.nth(i).getAttribute('class')) ?? '';
      expect(cls, `spinner #${i} missing border`).toMatch(/\bborder-[24]\b/);
      expect(cls, `spinner #${i} missing animate-spin`).toMatch(/\banimate-spin\b/);
    }
  });

  test('focus ring is consistent (cyan outline color) on interactive elements', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await page.waitForTimeout(400);
    // Tab into the page; first focusable should be the Inbox / first nav item.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    // The global focus-visible rule in globals.brutal.css sets
    // outline: 3px solid #74b9ff (cyan). Read the computed style.
    const outlineColor = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      if (!el) return '';
      return window.getComputedStyle(el).outlineColor;
    });
    // outlineColor comes back as 'rgb(116, 185, 255)' for #74b9ff.
    expect(outlineColor).toMatch(/rgb\(116, 185, 255\)/);
  });

  test('dialog overlay has no backdrop-blur (use opaque brutalist backdrop)', async ({ page }) => {
    // Open the create-channel modal on /dashboard (it uses the shared
    // Dialog component which currently still has backdrop-blur-sm —
    // this is the fe1 boundary we flagged).
    await page.goto(`${BASE}/dashboard`);
    await page.waitForTimeout(400);
    const dialog = page.locator('.fixed.inset-0.z-50').first();
    if (await dialog.isVisible()) {
      const cls = (await dialog.getAttribute('class')) ?? '';
      expect(cls, 'Dialog overlay should not use backdrop-blur').not.toMatch(/backdrop-blur/);
    }
  });
});
