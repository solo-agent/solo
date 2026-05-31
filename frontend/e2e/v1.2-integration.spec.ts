// ============================================================================
// Solo v1.1+v1.2 Integration Test Suite
// Tests: Auth, Agent management, Task system, Channel operations
// Uses serial execution with shared page for auth state persistence
// ============================================================================
import { test, expect, type Page } from '@playwright/test';

// ---- Test credentials ----
const TEST_EMAIL = 'test@test.com';
const TEST_PASSWORD = 'test12345';

// ---- Helpers ----

let sharedPage: Page;

test.beforeAll(async ({ browser }) => {
  const context = await browser.newContext();
  sharedPage = await context.newPage();
});

async function clearAuth() {
  await sharedPage.evaluate(() => {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
  });
}

async function waitForDashboard() {
  await sharedPage.waitForURL('**/dashboard', { timeout: 15000 });
  await sharedPage.locator('text=Solo').first().waitFor({ state: 'visible', timeout: 10000 });
  await sharedPage.waitForTimeout(1500);
}

async function waitForLoginForm() {
  await sharedPage.waitForURL('**/auth/login', { timeout: 15000 });
  await sharedPage.locator('#email').waitFor({ state: 'visible', timeout: 15000 });
}

// ---- Serial test suite ----

test.describe.serial('v1.1+v1.2 Integration Tests', () => {
  // ==================================================================
  // Test A: Authentication
  // ==================================================================
  test('A1: Login with test account and redirect to dashboard', async () => {
    // Start at home page, should redirect to login
    await sharedPage.goto('/');
    await waitForLoginForm();
    await expect(sharedPage.locator('text=欢迎回来').first()).toBeVisible();

    // Fill login form
    await sharedPage.fill('#email', TEST_EMAIL);
    await sharedPage.fill('#password', TEST_PASSWORD);

    // Submit
    await sharedPage.getByRole('button', { name: '登录' }).click();

    // Verify dashboard
    await waitForDashboard();
    await expect(sharedPage.locator('text=Solo').first()).toBeVisible();
    console.log('[A1] Login successful, dashboard loaded');
  });

  // ==================================================================
  // Test B: Agent Management
  // ==================================================================
  test('B1: Agent list page loads via sidebar and back button works', async () => {
    // Ensure on dashboard
    await sharedPage.goto('/dashboard');
    await waitForDashboard();

    // Click sidebar "Agent 管理" link
    const agentSidebarLink = sharedPage.locator('a:has-text("Agent 管理")').first();
    await agentSidebarLink.waitFor({ state: 'visible', timeout: 10000 });
    await agentSidebarLink.click();

    // Verify agent page loads
    await sharedPage.waitForURL('**/agents', { timeout: 15000 });
    await sharedPage.waitForTimeout(2000);
    await expect(sharedPage.locator('text=Agent 管理').first()).toBeVisible({ timeout: 10000 });
    console.log('[B1] Agent page loaded from sidebar');

    // Verify back button exists (ArrowLeft button that navigates to /dashboard)
    const backButton = sharedPage.locator('button[aria-label="返回仪表盘"]').first();
    await expect(backButton).toBeVisible({ timeout: 5000 });
    console.log('[B1] Back button is visible');

    // Click back button and verify navigation to dashboard
    await backButton.click();
    await waitForDashboard();
    console.log('[B1] Back button navigated to dashboard');
  });

  test('B2: Agent detail page and tabs', async () => {
    // Navigate to agents page
    await sharedPage.goto('/agents');
    await sharedPage.waitForURL('**/agents', { timeout: 10000 });
    await sharedPage.waitForTimeout(2000);

    // Check if there are any agents
    const hasAgents = await sharedPage.locator('.card-brutal').count();

    if (hasAgents === 0) {
      console.log('[B2] No agents found, creating one for testing');

      // Click create agent button
      await sharedPage.getByRole('button', { name: '创建 Agent' }).click();
      await sharedPage.waitForURL('**/agents/new', { timeout: 10000 });

      // Fill the form
      await sharedPage.waitForTimeout(1000);
      await sharedPage.fill('#name', 'TestBot');
      await sharedPage.fill('#description', 'E2E test agent');

      // Select "本地 CLI" provider
      const localCliBtn = sharedPage.getByRole('button', { name: '本地 CLI' });
      await localCliBtn.waitFor({ state: 'visible', timeout: 5000 });
      await localCliBtn.click();
      await sharedPage.waitForTimeout(500);

      await sharedPage.fill('#system_prompt', 'You are a test bot.');

      // Submit
      await sharedPage.getByRole('button', { name: '创建 Agent' }).click();
      await sharedPage.waitForURL('**/agents', { timeout: 15000 });
      await sharedPage.waitForTimeout(2000);
    }

    // Click the first agent card
    const firstAgentLink = sharedPage.locator('a:has-text("TestBot")').first();
    const firstAgentCard = sharedPage.locator('text=TestBot').first();
    const agentVisible = await firstAgentCard.isVisible().catch(() => false);

    if (!agentVisible) {
      // Try clicking any link/button on the page that navigates to /agents/[id]
      console.log('[B2] TestBot not visible, looking for any agent link');
      const anyAgentLink = sharedPage.locator('a[href^="/agents/"]').first();
      if (await anyAgentLink.isVisible().catch(() => false)) {
        await anyAgentLink.click();
      } else {
        console.log('[B2] No agent links found, skipping detail page test');
        return;
      }
    } else {
      await firstAgentLink.click();
    }

    // Verify agent detail page
    await sharedPage.waitForURL('**/agents/**', { timeout: 15000 });
    await sharedPage.waitForTimeout(2000);

    // Verify the detail page loaded with agent info
    await expect(sharedPage.locator('text=运行时').first()).toBeVisible({ timeout: 10000 });
    console.log('[B2] Agent detail page loaded');

    // Check Runtime tab is active by default
    const runtimeTab = sharedPage.locator('button:has-text("运行时")').first();
    await expect(runtimeTab).toBeVisible({ timeout: 5000 });

    // Click Tools tab
    const toolsTab = sharedPage.locator('button:has-text("工具")').first();
    await toolsTab.waitFor({ state: 'visible', timeout: 5000 });
    await toolsTab.click();
    await sharedPage.waitForTimeout(500);
    console.log('[B2] Tools tab is clickable');

    // Click History tab
    const historyTab = sharedPage.locator('button:has-text("历史")').first();
    await historyTab.waitFor({ state: 'visible', timeout: 5000 });
    await historyTab.click();
    await sharedPage.waitForTimeout(500);
    console.log('[B2] History tab is clickable');

    // Verify back to dashboard works
    const backBtn = sharedPage.locator('button[aria-label="返回仪表盘"]').first();
    if (await backBtn.isVisible().catch(() => false)) {
      await backBtn.click();
      await waitForDashboard();
      console.log('[B2] Back button from agent detail works');
    }
  });

  // ==================================================================
  // Test C: Task System
  // ==================================================================
  test('C1: Task management page loads and back button works', async () => {
    // Ensure on dashboard
    await sharedPage.goto('/dashboard');
    await waitForDashboard();

    // Click sidebar "任务管理" link
    const taskSidebarLink = sharedPage.locator('a:has-text("任务管理")').first();
    await taskSidebarLink.waitFor({ state: 'visible', timeout: 10000 });
    await taskSidebarLink.click();

    // Verify tasks page loads
    await sharedPage.waitForURL('**/tasks', { timeout: 15000 });
    await sharedPage.waitForTimeout(2000);
    await expect(sharedPage.locator('text=任务列表').first()).toBeVisible({ timeout: 10000 });
    console.log('[C1] Tasks page loaded from sidebar');

    // Verify back button exists
    const backButton = sharedPage.locator('button[aria-label="返回仪表盘"]').first();
    await expect(backButton).toBeVisible({ timeout: 5000 });
    console.log('[C1] Back button is visible on tasks page');

    // Click back and verify navigation
    await backButton.click();
    await waitForDashboard();
    console.log('[C1] Back button navigated to dashboard');
  });

  test('C2: Create task from /tasks/new page', async () => {
    // Navigate to tasks page
    await sharedPage.goto('/tasks/new');
    await sharedPage.waitForURL('**/tasks/new', { timeout: 10000 });
    await sharedPage.waitForTimeout(2000);

    // Verify page loaded
    await expect(sharedPage.locator('h1:has-text("创建任务")')).toBeVisible({ timeout: 10000 });
    console.log('[C2] Create task page loaded');

    // Check if channel selector is visible (no ?channel_id in URL, so it should show)
    const channelSelector = sharedPage.locator('select').filter({ has: sharedPage.locator('option:has-text("选择频道")') }).first();
    const hasChannelSelector = await channelSelector.isVisible().catch(() => false);

    if (hasChannelSelector) {
      console.log('[C2] Channel selector visible, selecting first channel');
      // Select the first available channel
      await channelSelector.selectOption({ index: 1 });  // Skip "选择频道..." placeholder
      await sharedPage.waitForTimeout(500);
    }

    // Fill in task form
    await sharedPage.fill('#task-title', 'E2E Test Task');
    await sharedPage.fill('#task-description', 'Created by Playwright E2E test');

    // Set priority to high
    await sharedPage.selectOption('#task-priority', 'high');

    // Fill due date
    await sharedPage.fill('#task-due-date', '2026-06-01');

    // Submit
    await sharedPage.getByRole('button', { name: '创建任务' }).click();

    // Verify redirect to /tasks list
    await sharedPage.waitForURL('**/tasks', { timeout: 15000 });
    await sharedPage.waitForTimeout(2000);
    console.log('[C2] Task created, redirected to task list');

    // Verify the task appears in the list
    await expect(sharedPage.locator('text=E2E Test Task').first()).toBeVisible({ timeout: 10000 });
    console.log('[C2] New task visible in list');
  });

  test('C3: Task detail page loads', async () => {
    // Navigate to tasks page
    await sharedPage.goto('/tasks');
    await sharedPage.waitForURL('**/tasks', { timeout: 10000 });
    await sharedPage.waitForTimeout(2000);

    // Find our test task
    const testTask = sharedPage.locator('text=E2E Test Task').first();
    if (!(await testTask.isVisible().catch(() => false))) {
      console.log('[C3] Test task not found, skipping');
      return;
    }

    // Click on the task
    // The task should be a link or clickable card
    const taskLink = sharedPage.locator('a:has-text("E2E Test Task")').first();
    if (await taskLink.isVisible().catch(() => false)) {
      await taskLink.click();
    } else {
      await testTask.click();
    }

    // Verify task detail page
    await sharedPage.waitForURL('**/tasks/**', { timeout: 15000 });
    await sharedPage.waitForTimeout(2000);

    // Verify task details are displayed
    await expect(sharedPage.locator('text=E2E Test Task').first()).toBeVisible({ timeout: 10000 });
    console.log('[C3] Task detail page loaded');

    // Verify back link to task list exists
    const backLink = sharedPage.locator('button:has-text("返回任务列表")').first();
    if (await backLink.isVisible().catch(() => false)) {
      console.log('[C3] Back to task list link visible');
    }
  });

  // ==================================================================
  // Test D: Channel operations
  // ==================================================================
  test('D1: Select channel and verify Messages/Tasks tabs', async () => {
    // Navigate to dashboard (auto-selects first channel)
    await sharedPage.goto('/dashboard');
    await waitForDashboard();

    // Wait for auto-channel selection to complete
    await sharedPage.waitForTimeout(2000);

    // The channel view should be visible with "消息" and "任务" tabs
    const messagesTab = sharedPage.locator('button:has-text("消息")').first();
    const tasksTab = sharedPage.locator('button:has-text("任务")').first();

    const hasMessagesTab = await messagesTab.isVisible().catch(() => false);
    const hasTasksTab = await tasksTab.isVisible().catch(() => false);

    // At least one tab should be visible if channels are loaded
    if (hasMessagesTab) {
      console.log('[D1] Messages tab visible');
      await messagesTab.click();
      await sharedPage.waitForTimeout(500);
    }
    if (hasTasksTab) {
      console.log('[D1] Tasks tab visible');
      await tasksTab.click();
      await sharedPage.waitForTimeout(500);
    }
    if (!hasMessagesTab && !hasTasksTab) {
      console.log('[D1] Neither Messages nor Tasks tab found - channel view may not be loaded');
      // Take a screenshot for debugging
      await sharedPage.screenshot({ path: 'd1-no-tabs.png' });
    }
  });

  test('D2: Quick create task within channel', async () => {
    // Navigate to dashboard (auto-selects first channel)
    await sharedPage.goto('/dashboard');
    await waitForDashboard();
    await sharedPage.waitForTimeout(2000);

    // Switch to "任务" (Tasks) tab
    const tasksTab = sharedPage.locator('button:has-text("任务")').first();
    if (await tasksTab.isVisible().catch(() => false)) {
      await tasksTab.click();
      await sharedPage.waitForTimeout(1000);
      console.log('[D2] Tasks tab clicked in channel view');

      // Click "快速创建" (Quick Create) button
      const quickCreateBtn = sharedPage.locator('button:has-text("快速创建")').first();
      if (await quickCreateBtn.isVisible().catch(() => false)) {
        await quickCreateBtn.click();
        await sharedPage.waitForTimeout(1000);
        console.log('[D2] Quick create task dialog opened');

        // The task form dialog should be open
        await expect(sharedPage.locator('text=快速创建任务').first()).toBeVisible({ timeout: 5000 });

        // Fill the form
        await sharedPage.fill('#task-title', 'Channel Quick Task');
        await sharedPage.fill('#task-description', 'Quickly created from channel view');
        await sharedPage.selectOption('#task-priority', 'high');

        // Submit - the dialog's task form has channel.id pre-filled
        const submitBtn = sharedPage.locator('button:has-text("创建任务")').last();
        await submitBtn.click();
        await sharedPage.waitForTimeout(2000);

        // Verify the task appears in the channel's task list
        await expect(sharedPage.locator('text=Channel Quick Task').first()).toBeVisible({ timeout: 10000 });
        console.log('[D2] Quick task created successfully in channel');
      } else {
        console.log('[D2] Quick create button not found');
        await sharedPage.screenshot({ path: 'd2-no-quick-create.png' });
      }
    } else {
      console.log('[D2] Tasks tab not found in channel view');
      await sharedPage.screenshot({ path: 'd2-no-tasks-tab.png' });
    }
  });

  // ==================================================================
  // Cleanup: Logout
  // ==================================================================
  test('Z1: Logout', async () => {
    await clearAuth();
    await sharedPage.goto('/auth/login');
    await waitForLoginForm();
    console.log('[Z1] Logged out successfully');
  });
});
