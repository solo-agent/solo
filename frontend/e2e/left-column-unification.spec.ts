// ============================================================================
// Left column unification — cross-page e2e regression spec
// Verifies the four pages that share a 220px-wide left column:
//   /dashboard  -> label "Chat",        collapsible Channels / Direct Messages
//   /teams      -> label "Teams",       Graph (no chevron), Agents / Humans (chevron)
//   /tasks      -> label "Tasks",       collapsible Channels / Direct Messages, URL syncs
//   /computers  -> label "Computers",   collapsible All Computers, click toggles detail
// Where a section has no children the disclosure chevron is absent; where
// children exist it toggles aria-expanded. Tests are defensive — channel /
// DM / computer / agent data is not guaranteed to exist for this user.
// Requires: dev server on :3000, teams-e2e@test.com / TeamsE2E12345!.
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
  await page.waitForTimeout(1000); // Allow React to hydrate
}

test.describe('Left column unification', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('/dashboard shows "Chat" label + collapsible Channels/DMs', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await expect(page.getByText('Chat', { exact: true })).toBeVisible();

    // Channels toggle: visible only if ChannelList rendered. Default expanded.
    const channelsHeader = page.getByRole('button', { name: '展开或折叠 频道' });
    if (await channelsHeader.isVisible()) {
      await expect(channelsHeader).toHaveAttribute('aria-expanded', 'true');
      await channelsHeader.click();
      await expect(channelsHeader).toHaveAttribute('aria-expanded', 'false');
      await channelsHeader.click();
      await expect(channelsHeader).toHaveAttribute('aria-expanded', 'true');
    }

    // DMs toggle: same shape as Channels.
    const dmsHeader = page.getByRole('button', { name: '展开或折叠 直接消息' });
    if (await dmsHeader.isVisible()) {
      await expect(dmsHeader).toHaveAttribute('aria-expanded', 'true');
      await dmsHeader.click();
      await expect(dmsHeader).toHaveAttribute('aria-expanded', 'false');
    }
  });

  test('/teams shows "Teams" label; Graph has no chevron; Agents/Humans do', async ({ page }) => {
    await page.goto(`${BASE}/teams`);
    await expect(page.getByText('Teams', { exact: true })).toBeVisible();

    // Graph is a single-select: no children, no disclosure chevron.
    const graphButton = page.getByRole('button', { name: '进入 Graph 视图' });
    await expect(graphButton).toBeVisible();
    await expect(graphButton).not.toHaveAttribute('aria-expanded');

    // Agents and Humans default expanded; only assert if the header is rendered.
    const agentsHeader = page.getByRole('button', { name: '展开或折叠 Agents' });
    if (await agentsHeader.isVisible()) {
      await expect(agentsHeader).toHaveAttribute('aria-expanded', 'true');
    }

    const humansHeader = page.getByRole('button', { name: '展开或折叠 Humans' });
    if (await humansHeader.isVisible()) {
      await expect(humansHeader).toHaveAttribute('aria-expanded', 'true');
    }
  });

  test('/tasks shows "Tasks" label + URL syncs when channel clicked', async ({ page }) => {
    await page.goto(`${BASE}/tasks`);
    await expect(page.getByText('Tasks', { exact: true })).toBeVisible();

    // Channel rows render with a leading "#" prefix; pick the first one if any.
    const firstChannel = page.locator('button:has-text("#")').first();
    if (await firstChannel.isVisible()) {
      await firstChannel.click();
      await expect(page).toHaveURL(/\?channel=/);

      // Re-click the same channel clears the URL back to /tasks.
      await firstChannel.click();
      await expect(page).toHaveURL(/\/tasks$/);
    }
  });

  test('/computers shows "Computers" label + select toggles detail', async ({ page }) => {
    await page.goto(`${BASE}/computers`);
    await expect(page.getByText('Computers', { exact: true })).toBeVisible();

    // Computer rows are <button> with a status dot (green online / gray offline).
    // Skip the click flow entirely if no computers exist for this user.
    const firstComputer = page
      .locator('button:has(span.bg-green-500), button:has(span.bg-gray-400)')
      .first();
    if (await firstComputer.isVisible()) {
      await firstComputer.click();
      // Detail card replaces the "pick a computer" empty state.
      await expect(page.getByText('请从左侧选择一台电脑')).not.toBeVisible();

      await firstComputer.click();
      await expect(page.getByText('请从左侧选择一台电脑')).toBeVisible();
    }
  });
});
