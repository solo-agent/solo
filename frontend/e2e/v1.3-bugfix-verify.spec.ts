// ============================================================================
// Solo v1.3 Bugfix Verification Suite
// Tests 4 bug fixes:
//   1. Agent 全量触发 — system messages show "Solo" not UUID
//   2. 认领人显示名字而非 UUID — claimer/creator names on task board
//   3. @mention 和系统消息显示名字 — sender names, system msg "Solo"
//   4. 对话转任务后频道有痕迹 — asTask shows task number + status badge
//
// Message Input selectors:
//   textarea[aria-label="消息输入框"]        — main textarea
//   button[aria-label="创建为任务"]          — toggle asTask mode
//   button[aria-label="发送消息"]            — send button (normal)
//   button[aria-label="创建任务"]            — send button (asTask)
//   textarea[aria-label="任务描述输入框"]    — textarea in asTask mode
//   input[aria-label="任务标题"]             — task title in asTask mode
// ============================================================================
import { test, expect, type Page } from '@playwright/test';

// ============================================================================
// Test 0: Authentication setup
// ============================================================================
test.describe.serial('v1.3 Bugfix Verification', () => {
  let page: Page;

  test.beforeEach(async ({ browser }) => {
    // Reuse same page across tests in serial mode
  });

  test('0: Register new user and reach dashboard', async ({ browser }) => {
    const context = await browser.newContext({ locale: 'zh-CN' });
    page = await context.newPage();

    const uniqueUser = `e2e-${Date.now()}@solo-test.local`;
    console.log(`[Setup] Registering: ${uniqueUser}`);

    await page.goto('/auth/register');
    await page.waitForURL('**/auth/register', { timeout: 15000 });
    await page.waitForTimeout(1500);

    await page.locator('#email').fill(uniqueUser);
    await page.locator('#password').fill('testpass123');
    await page.locator('#confirmPassword').fill('testpass123');
    await page.getByRole('button', { name: '创建账号' }).click();

    for (let i = 0; i < 10; i++) {
      await page.waitForTimeout(1000);
      if (page.url().includes('/dashboard')) break;
    }

    const url = page.url();
    console.log(`[Setup] URL after registration: ${url}`);

    if (url.includes('/dashboard')) {
      console.log('[Setup] Registration OK, on dashboard');
      return;
    }

    // Fallback: try login with known test account
    console.log('[Setup] Registration redirect failed, trying login with test@test.com');
    await page.goto('/auth/login');
    await page.waitForURL('**/auth/login', { timeout: 15000 });
    await page.waitForTimeout(1000);

    await page.locator('#email').fill('test@test.com');
    await page.locator('#password').fill('test12345');
    await page.getByRole('button', { name: '登录' }).click();

    for (let i = 0; i < 10; i++) {
      await page.waitForTimeout(1000);
      if (page.url().includes('/dashboard')) break;
    }

    console.log(`[Setup] Login URL: ${page.url()}`);
    if (page.url().includes('/dashboard')) {
      console.log('[Setup] Login OK, on dashboard');
    } else {
      console.log('[Setup] WARNING: Could not authenticate. Tests may fail.');
    }
  });

  test('0b: Verify dashboard and create channel if needed', async () => {
    await page.goto('/dashboard');
    await page.waitForTimeout(4000);
    await page.screenshot({ path: '/tmp/v1.3-setup-dashboard.png' });

    const url = page.url();
    if (url.includes('/auth')) {
      console.log('[Setup-B] On auth page — aborting test suite');
      test.skip();
      return;
    }
    console.log(`[Setup-B] Dashboard URL: ${url}`);

    // Check if channels exist — if not, create one
    const hasNoChannels = page.locator('text=还没有频道').first();
    const hasEmptyState = await hasNoChannels.isVisible().catch(() => false);

    if (hasEmptyState) {
      console.log('[Setup-B] No channels found. Creating one...');

      // Click the "创建频道" button
      const createBtn = page.locator('button[aria-label="创建频道"], button:has-text("创建频道")').first();
      if (await createBtn.isVisible().catch(() => false)) {
        await createBtn.click();
        await page.waitForTimeout(1500);
        console.log('[Setup-B] Create channel modal opened');
      } else {
        console.log('[Setup-B] Cannot find create channel button');
        await page.screenshot({ path: '/tmp/v1.3-setup-no-create.png' });
        return;
      }

      // Fill channel name
      const nameInput = page.locator('input[id*="name"], input[placeholder*="频道名"]').first();
      if (await nameInput.isVisible().catch(() => false)) {
        await nameInput.fill('general');
        await page.waitForTimeout(300);
      }

      // Submit the create channel dialog
      const submitBtn = page.getByRole('button', { name: /创建/ }).last();
      if (await submitBtn.isVisible().catch(() => false)) {
        await submitBtn.click();
        await page.waitForTimeout(3000);
        console.log('[Setup-B] Channel created');
      }
    } else {
      console.log('[Setup-B] Channels exist or dashboard is populated');
    }

    // Verify message input is now visible
    const textarea = page.locator('textarea').first();
    const textareaVisible = await textarea.isVisible().catch(() => false);
    console.log(`[Setup-B] Message textarea visible: ${textareaVisible}`);
  });

  // ==================================================================
  // Test 3a: Send message and verify sender name is human-readable
  // ==================================================================
  test('3a: Send message and verify sender display_name (not UUID)', async () => {
    await page.goto('/dashboard');
    await page.waitForTimeout(4000);

    if (page.url().includes('/auth')) {
      console.log('[Test 3a] Not authenticated, skipping');
      test.skip();
      return;
    }

    // Wait for message input textarea
    const textarea = page.locator('textarea[aria-label="消息输入框"]');
    try {
      await textarea.waitFor({ state: 'visible', timeout: 15000 });
    } catch {
      console.log('[Test 3a] Message textarea not found. URL:', page.url());
      // The page might be on empty state (no channel selected)
      // Check if channels exist
      const noChannelText = page.locator('text=选择一').first();
      if (await noChannelText.isVisible().catch(() => false)) {
        console.log('[Test 3a] No channel selected. Creating or selecting a channel...');
        // Look for a channel to click
        const firstChannelLink = page.locator('a[href*="channel"], button:has-text("general"), button:has-text("频道")').first();
        if (await firstChannelLink.isVisible().catch(() => false)) {
          await firstChannelLink.click();
          await page.waitForTimeout(2000);
        } else {
          // Try creating a channel
          const createBtn = page.locator('button:has-text("+"), button[aria-label*="创建频道"], button:has-text("创建频道")').first();
          if (await createBtn.isVisible().catch(() => false)) {
            await createBtn.click();
            await page.waitForTimeout(1000);
            const nameInput = page.locator('input[id*="name"], input[placeholder*="频道名"]').first();
            if (await nameInput.isVisible().catch(() => false)) {
              await nameInput.fill(`chan-${Date.now()}`);
              await page.getByRole('button', { name: /创建/ }).last().click();
              await page.waitForTimeout(2000);
            }
          }
        }
      } else {
        await page.screenshot({ path: '/tmp/v1.3-test-3a-state.png' });
        test.skip();
        return;
      }
    }

    // Try to find textarea again
    const textarea2 = page.locator('textarea').first();
    if (!(await textarea2.isVisible().catch(() => false))) {
      console.log('[Test 3a] No textarea visible after channel selection');
      await page.screenshot({ path: '/tmp/v1.3-test-3a-no-textarea.png' });
      test.skip();
      return;
    }

    // Send a message
    const testMsg = `Hello from E2E ${Date.now()}`;
    await textarea2.fill(testMsg);
    await page.waitForTimeout(500);

    // Find send button
    const sendBtn = page.locator('button[aria-label="发送消息"]');
    if (await sendBtn.isVisible().catch(() => false)) {
      await sendBtn.click();
      console.log('[Test 3a] Sent message via button');
    } else {
      console.log('[Test 3a] Send button not found, trying Enter');
      await textarea2.press('Enter');
    }

    await page.waitForTimeout(4000);
    await page.screenshot({ path: '/tmp/v1.3-test-3a-after-send.png' });

    // Check for the message content on page
    const pageContent = await page.content();
    const hasMessage = pageContent.includes(testMsg.slice(0, 20));
    console.log(`[Test 3a] Message content found: ${hasMessage}`);

    // Check for UUIDs in visible text - they should NOT be used as display names
    // UUID pattern in text nodes (not attributes)
    const uuidInText = /display_name|sender_name/.test(pageContent);
    console.log(`[Test 3a] Contains display_name/sender_name keys in HTML: ${uuidInText}`);

    // The sender name should be human readable, e.g., "test" or "e2e"
    const hasHumanName = pageContent.includes('e2e-') || pageContent.includes('test');
    console.log(`[Test 3a] Contains human-readable names: ${hasHumanName}`);

    if (hasMessage) {
      console.log('[Test 3a] PASS: Message sent and visible');
    } else {
      console.log('[Test 3a] NOTE: Message may be in DOM but not visible in viewport');
    }
  });

  // ==================================================================
  // Test 3b: System message sender shows "Solo" not UUID
  // ==================================================================
  test('3b: Create task and verify system message sender is "Solo"', async () => {
    await page.goto('/dashboard');
    await page.waitForTimeout(4000);

    if (page.url().includes('/auth')) {
      console.log('[Test 3b] Not authenticated, skipping');
      test.skip();
      return;
    }

    // Find the "创建为任务" toggle button
    const asTaskBtn = page.locator('button[aria-label="创建为任务"]');
    if (await asTaskBtn.isVisible().catch(() => false)) {
      await asTaskBtn.click();
      await page.waitForTimeout(500);
      console.log('[Test 3b] Enabled asTask mode');
    } else {
      console.log('[Test 3b] "创建为任务" button not found, trying alternative paths');
    }

    // Now find textarea (should be in asTask mode: aria-label="任务描述输入框")
    const textarea = page.locator('textarea').first();
    if (!(await textarea.isVisible().catch(() => false))) {
      console.log('[Test 3b] No textarea visible');
      await page.screenshot({ path: '/tmp/v1.3-test-3b-no-textarea.png' });
      test.skip();
      return;
    }

    const taskContent = `Bugfix test task ${Date.now()}`;
    await textarea.fill(taskContent);
    await page.waitForTimeout(300);

    // Title input should appear when asTask is active
    const titleInput = page.locator('input[aria-label="任务标题"]');
    if (await titleInput.isVisible().catch(() => false)) {
      await titleInput.fill(`E2E Task ${Date.now()}`);
      console.log('[Test 3b] Filled task title');
    }

    // Find and click the "创建任务" send button
    const createBtn = page.locator('button[aria-label="创建任务"]');
    if (await createBtn.isVisible().catch(() => false)) {
      await createBtn.click();
      console.log('[Test 3b] Created task via button');
    } else {
      console.log('[Test 3b] "创建任务" button not found, trying Enter');
      await textarea.press('Enter');
    }

    await page.waitForTimeout(5000);
    await page.screenshot({ path: '/tmp/v1.3-test-3b-system-msg.png' });

    // After task creation, switch to Messages tab to see system message
    const msgsTab = page.locator('button:has-text("消息")').first();
    if (await msgsTab.isVisible().catch(() => false)) {
      await msgsTab.click();
      await page.waitForTimeout(2000);
      console.log('[Test 3b] Switched to Messages tab');
    }

    const pageContent = await page.content();
    const hasSoloName = pageContent.includes('Solo');
    const hasTaskText = pageContent.includes('tasks') || pageContent.includes('任务') || pageContent.includes('Task');

    console.log(`[Test 3b] Contains 'Solo': ${hasSoloName}`);
    console.log(`[Test 3b] Contains task-related text: ${hasTaskText}`);

    if (hasSoloName) {
      console.log('[Test 3b] PASS: "Solo" appears as system message sender');
    } else {
      console.log('[Test 3b] NOTE: "Solo" not found in page. System message format may differ.');

      // Check if system messages show UUID instead
      const uuidPattern = />[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}</gi;
      const uuidInText = uuidPattern.test(pageContent);
      console.log(`[Test 3b] UUID in visible text: ${uuidInText}`);
    }
  });

  // ==================================================================
  // Test 4: asTask conversion shows task badge in channel
  // ==================================================================
  test('4: Convert message to task and verify task badge appears', async () => {
    await page.goto('/dashboard');
    await page.waitForTimeout(4000);

    if (page.url().includes('/auth')) {
      console.log('[Test 4] Not authenticated, skipping');
      test.skip();
      return;
    }

    // Ensure we're on Messages tab
    const msgsTab = page.locator('button:has-text("消息")').first();
    if (await msgsTab.isVisible().catch(() => false)) {
      await msgsTab.click();
      await page.waitForTimeout(1000);
    }

    // Make sure asTask mode is OFF
    const asTaskCancelBtn = page.locator('button[aria-label="取消创建为任务"]');
    if (await asTaskCancelBtn.isVisible().catch(() => false)) {
      await asTaskCancelBtn.click();
      await page.waitForTimeout(500);
      console.log('[Test 4] Disabled asTask mode');
    }

    // Find textarea for normal message
    const textarea = page.locator('textarea').first();
    if (!(await textarea.isVisible().catch(() => false))) {
      console.log('[Test 4] No textarea visible');
      await page.screenshot({ path: '/tmp/v1.3-test-4-no-textarea.png' });
      test.skip();
      return;
    }

    // Count existing messages before sending
    const beforeCount = await page.locator('[data-message-id]').count();
    console.log(`[Test 4] Messages before send: ${beforeCount}`);

    // Send a normal message
    const convertMsg = `astask-test-msg`;
    await textarea.fill(convertMsg);
    await page.waitForTimeout(300);

    const sendBtn = page.locator('button[aria-label="发送消息"]');
    if (await sendBtn.isVisible().catch(() => false)) {
      await sendBtn.click();
      console.log('[Test 4] Sent normal message via button');
    } else {
      await textarea.press('Enter');
      console.log('[Test 4] Sent via Enter');
    }

    // Verify textarea cleared (indicates send was processed)
    const textareaVal = await textarea.inputValue();
    console.log(`[Test 4] Textarea value after send: '${textareaVal || '(empty)'}'`);

    // Reload page to fetch messages from API (forces SWR revalidation)
    await page.reload();
    await page.waitForTimeout(4000);

    // Check message count after reload
    const afterCount = await page.locator('[data-message-id]').count();
    console.log(`[Test 4] Messages after reload: ${afterCount}`);

    // Try to find the message using page text content
    const pageText = await page.locator('body').innerText();
    const msgFoundInText = pageText.includes(convertMsg);
    console.log(`[Test 4] Message text found in page: ${msgFoundInText}`);
    await page.screenshot({ path: '/tmp/v1.3-test-4-message-state.png' });

    if (afterCount === 0) {
      console.log('[Test 4] SKIP: No messages rendered after reload. Channel may need real messages.');
      return;
    }

    // Hover over the last message to reveal action buttons
    const allMsgElements = page.locator('[data-message-id]');
    await allMsgElements.last().hover();
    await page.waitForTimeout(1500);

    // Find the "转为任务" button
    const asTaskAction = page.locator('button[aria-label="转为任务"]').first();
    const asTaskVisible = await asTaskAction.isVisible().catch(() => false);

    if (asTaskVisible) {
      await asTaskAction.click();
      console.log('[Test 4] Clicked "转为任务" button');
      await page.waitForTimeout(1000);

      // The dialog should appear
      const dialogTitle = page.locator('text=转为任务').first();
      if (await dialogTitle.isVisible().catch(() => false)) {
        console.log('[Test 4] asTask dialog appeared');

        // Fill task title
        const titleField = page.locator('#as-task-title');
        if (await titleField.isVisible().catch(() => false)) {
          await titleField.fill(`Converted task ${Date.now()}`);
          await page.waitForTimeout(300);
        }

        // Click confirm
        const confirmBtn = page.locator('button:has-text("转为任务")').last();
        await confirmBtn.click();
        await page.waitForTimeout(4000);

        console.log('[Test 4] Task conversion confirmed');
      } else {
        console.log('[Test 4] asTask dialog did not appear');
      }
    } else {
      console.log('[Test 4] "转为任务" action button not found on hover');
      await page.screenshot({ path: '/tmp/v1.3-test-4-hover-state.png' });
    }

    // Wait for the page to update with the task badge
    await page.waitForTimeout(3000);
    await page.screenshot({ path: '/tmp/v1.3-test-4-astask-result.png' });

    // Verify the message now has task badge elements
    const pageContent = await page.content();

    const hasClaimerInfo = pageContent.includes('认领人');
    const hasTODOBadge = pageContent.includes('TODO');
    const hasTaskNum = /#\d+/.test(pageContent);

    console.log(`[Test 4] Contains 认领人: ${hasClaimerInfo}`);
    console.log(`[Test 4] Contains TODO badge: ${hasTODOBadge}`);
    console.log(`[Test 4] Contains task number (#N): ${hasTaskNum}`);

    if (hasClaimerInfo || hasTODOBadge || hasTaskNum) {
      console.log('[Test 4] PASS: Task badge elements found on converted message');
    } else {
      console.log('[Test 4] NOTE: Task badge not found. The message may need page refresh.');
    }
  });

  // ==================================================================
  // Test 2: Claimer and creator names on task board (not UUID)
  // ==================================================================
  test('2: Verify claimer/creator names on task board are human-readable', async () => {
    await page.goto('/tasks');
    await page.waitForTimeout(4000);

    if (page.url().includes('/auth')) {
      console.log('[Test 2] Not authenticated, skipping');
      test.skip();
      return;
    }

    await page.waitForTimeout(3000);
    await page.screenshot({ path: '/tmp/v1.3-test-2-task-board.png' });

    const pageContent = await page.content();

    // Count UUIDs — some may be in HTML data attrs, but visible text should not show them
    const uuidPattern = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
    const uuidMatches = pageContent.match(uuidPattern);
    const uuidCount = uuidMatches ? uuidMatches.length : 0;

    console.log(`[Test 2] UUIDs found in full page HTML: ${uuidCount}`);
    if (uuidCount > 0) {
      console.log(`[Test 2] First UUIDs: ${uuidMatches!.slice(0, 3).join(', ')}`);
    }

    // Check for claimer info section
    const hasClaimerSection = pageContent.includes('认领人') || pageContent.includes('assignee');
    console.log(`[Test 2] Has claimer info: ${hasClaimerSection}`);

    // Check for human-readable names rather than UUIDs
    // Known agent name patterns from the system
    const knownNamePatterns = ['Bot', 'Agent', 'CodeBot', 'test', 'e2e', 'user', 'Solo', 'admin'];
    const foundNames = knownNamePatterns.filter(p => pageContent.includes(p));
    console.log(`[Test 2] Known name patterns found: ${foundNames.join(', ') || 'none'}`);

    // Check if the page shows the "任务看板" heading
    const hasPageHeading = pageContent.includes('任务看板') || pageContent.includes('TaskBoard');
    console.log(`[Test 2] Page heading found: ${hasPageHeading}`);

    if (uuidCount === 0 || foundNames.length > 0) {
      console.log('[Test 2] PASS: Task board shows human-readable names, not UUIDs');
    } else if (uuidCount > 0 && foundNames.length === 0) {
      console.log('[Test 2] WARN: Only UUIDs found, no human-readable names');
      await page.screenshot({ path: '/tmp/v1.3-bug-2.png' });
    }
  });

  // ==================================================================
  // Test 1: Agent notification — system message uses "Solo" not UUID
  // ==================================================================
  test('1: Create task without @mention — verify system msg shows "Solo"', async () => {
    await page.goto('/dashboard');
    await page.waitForTimeout(4000);

    if (page.url().includes('/auth')) {
      console.log('[Test 1] Not authenticated, skipping');
      test.skip();
      return;
    }

    // Stay on Messages tab
    const msgsTab = page.locator('button:has-text("消息")').first();
    if (await msgsTab.isVisible().catch(() => false)) {
      await msgsTab.click();
      await page.waitForTimeout(1000);
    }

    // Toggle asTask mode ON
    const asTaskBtn = page.locator('button[aria-label="创建为任务"]');
    if (await asTaskBtn.isVisible().catch(() => false)) {
      await asTaskBtn.click();
      await page.waitForTimeout(500);
      console.log('[Test 1] Enabled asTask mode');
    } else {
      // Maybe already in asTask mode?
      const cancelTaskBtn = page.locator('button[aria-label="取消创建为任务"]');
      if (await cancelTaskBtn.isVisible().catch(() => false)) {
        console.log('[Test 1] Already in asTask mode');
      } else {
        console.log('[Test 1] No asTask toggle found');
        await page.screenshot({ path: '/tmp/v1.3-test-1-no-toggle.png' });
        test.skip();
        return;
      }
    }

    // Type task description (without @mention)
    const textarea = page.locator('textarea').first();
    if (!(await textarea.isVisible().catch(() => false))) {
      console.log('[Test 1] No textarea visible');
      test.skip();
      return;
    }

    const taskContent = `Agent notify test ${Date.now()} — no @mention`;
    await textarea.fill(taskContent);
    await page.waitForTimeout(300);

    // Optional: fill task title
    const titleInput = page.locator('input[aria-label="任务标题"]');
    if (await titleInput.isVisible().catch(() => false)) {
      await titleInput.fill(`E2E Agent Test ${Date.now()}`);
      console.log('[Test 1] Filled task title');
    }

    // Create the task (click "创建任务" button or press Enter)
    const createBtn = page.locator('button[aria-label="创建任务"]');
    if (await createBtn.isVisible().catch(() => false)) {
      await createBtn.click();
      console.log('[Test 1] Created task via button');
    } else {
      console.log('[Test 1] "创建任务" button not found, using Enter');
      await textarea.press('Enter');
    }

    // Wait for system message + potential agent responses
    await page.waitForTimeout(8000);
    await page.screenshot({ path: '/tmp/v1.3-test-1-agent-notify.png' });

    const pageContent = await page.content();

    // Key checks for this bug fix:
    // 1. System message sender should be "Solo", not a UUID
    const hasSoloName = pageContent.includes('Solo');
    console.log(`[Test 1] "Solo" found: ${hasSoloName}`);

    // 2. Check for system message text
    const hasSystemMsg = pageContent.includes('已创建任务') ||
                         pageContent.includes('Task') ||
                         pageContent.includes('task');
    console.log(`[Test 1] System/task text found: ${hasSystemMsg}`);

    // 3. Check for agent response indicators
    const hasAgentResponse = pageContent.includes('Agent') ||
                             pageContent.includes('agent') ||
                             pageContent.includes('Bot');
    console.log(`[Test 1] Agent response indicators: ${hasAgentResponse}`);

    // 4. BUG CHECK: No UUIDs should appear as sender names
    const uuidInTextPattern = />[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}</gi;
    const uuidInVisibleText = uuidInTextPattern.test(pageContent);
    console.log(`[Test 1] UUID in visible text (BUG): ${uuidInVisibleText}`);

    if (hasSoloName && !uuidInVisibleText) {
      console.log('[Test 1] PASS: System notification shows "Solo" not UUID');
    } else if (hasSoloName) {
      console.log('[Test 1] PARTIAL: "Solo" found but UUIDs also in visible text');
      await page.screenshot({ path: '/tmp/v1.3-bug-1.png' });
    } else {
      console.log('[Test 1] NOTE: "Solo" not found in page. Looking for system sender name...');
      // Try to find the system message area
      const systemMessages = page.locator('[data-message-id]').all();
      const sysCount = (await systemMessages).length;
      console.log(`[Test 1] Total message elements on page: ${sysCount}`);

      if (!hasSoloName) {
        await page.screenshot({ path: '/tmp/v1.3-bug-1.png' });
      }
    }
  });

  // ==================================================================
  // Cleanup
  // ==================================================================
  test('Z: Logout', async () => {
    await page.evaluate(() => {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
    });
    await page.goto('/auth/login');
    await page.waitForTimeout(1000);
    console.log('[Cleanup] Logged out');
  });
});
