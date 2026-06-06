// ============================================================================
// Teams page redesign — e2e regression spec
// Covers: default load, Graph header, agent select, DM jump
// Requires: dev server on :3000, test user (test@test.com / test12345) with
// at least one agent created. Same setup as full-regression.spec.ts.
// ============================================================================

import { test, expect, type Page } from '@playwright/test';

const BASE = 'http://localhost:3000';
const TEST_EMAIL = 'test@test.com';
const TEST_PASSWORD = 'test12345';

async function login(page: Page) {
  await page.goto(`${BASE}/auth/login`);
  await page.locator('#email').fill(TEST_EMAIL);
  await page.locator('#password').fill(TEST_PASSWORD);
  await page.getByRole('button', { name: '登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(1000); // Allow React to hydrate
}

test.describe('Teams page redesign', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('1. default load shows left column + first agent selected', async ({ page }) => {
    await page.goto(`${BASE}/teams`);

    // First agent is auto-selected -> Profile h2 is rendered
    await expect(page.getByRole('heading', { name: /Profile/i }).first()).toBeVisible();

    // Graph header is always visible (collapsed by default; no body items)
    await expect(page.getByRole('button', { name: /进入 Graph 视图/i })).toBeVisible();

    // Agents and Humans sections are expanded by default
    await expect(page.getByRole('button', { name: /展开或折叠 Agents/i })).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByRole('button', { name: /展开或折叠 Humans/i })).toHaveAttribute('aria-expanded', 'true');
  });

  test('2. clicking Graph header switches right panel to structure view', async ({ page }) => {
    await page.goto(`${BASE}/teams`);
    await page.getByRole('button', { name: /进入 Graph 视图/i }).click();

    // The structure view footer text appears when at least one group has agents
    await expect(page.getByText('横向滚动查看更多')).toBeVisible();
  });

  test('3. clicking an agent row selects it', async ({ page }) => {
    await page.goto(`${BASE}/teams`);

    // Read agent name from aria-label (format: "查看 <name> 详情") to avoid
    // picking up extra text like the trailing "DM" button label.
    const firstAgentButton = page.locator('[aria-label^="查看"][aria-label$="详情"]').first();
    const ariaLabel = (await firstAgentButton.getAttribute('aria-label')) ?? '';
    const agentName = ariaLabel.replace(/^查看 (.+) 详情$/, '$1');

    await firstAgentButton.click();

    // The header h1 should now show the selected agent's name
    await expect(page.locator('h1').filter({ hasText: agentName })).toBeVisible();
  });

  test('4. clicking the Message button jumps to /dashboard?dm=', async ({ page }) => {
    await page.goto(`${BASE}/teams`);
    await page.getByRole('button', { name: /^Message/i }).first().click();
    await page.waitForURL(/\/dashboard\?dm=/, { timeout: 10000 });
    expect(page.url()).toMatch(/\/dashboard\?dm=/);
  });
});
