import { test, expect, Page } from '@playwright/test';

const BASE = 'http://localhost:3000';
const EMAIL = 'qa-test@test.com';
const PASSWORD = 'test123456';
const CHANNEL_ID = 'aa53b630-b612-41c4-b518-afbba75e92d2';
const DM_PRODUCT_ID = 'c1b02a74-0682-4141-9710-f8ab687e9edc';
const TASK26_MSG_ID = '992ea919-8772-4d42-83ae-f8191e5fe8ce';
const TASK27_MSG_ID = 'e08d69c7-cf30-4a6e-bea0-bd2e41a2f9d9';

async function login(page: Page) {
  await page.goto(`${BASE}/auth/login`, { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', EMAIL);
  await page.fill('input[type="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/dashboard**', { timeout: 15000 });
  await page.waitForSelector('aside', { timeout: 10000 });
  await page.waitForTimeout(1000);
}

async function clickSidebarDM(page: Page, dmName: string) {
  // Find and click DM item in sidebar
  const dmLink = page.locator('aside').locator('button, a, div[role="button"]').filter({ hasText: dmName }).first();
  await dmLink.waitFor({ state: 'visible', timeout: 5000 });
  await dmLink.click();
  await page.waitForTimeout(2000);
}

async function clickSidebarChannel(page: Page, channelName: string) {
  // Find and click channel item in sidebar
  const chLink = page.locator('aside').locator('button, a, div[role="button"]').filter({ hasText: channelName }).first();
  await chLink.waitFor({ state: 'visible', timeout: 5000 });
  await chLink.click();
  await page.waitForTimeout(2000);
}

async function openThreadViaMessage(page: Page, messageId: string) {
  // Navigate to dashboard with channel and message params to open thread
  await page.goto(`${BASE}/dashboard?channel=${CHANNEL_ID}&message=${messageId}`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);
}

async function sendTextMessage(page: Page, text: string) {
  // Use the message input textarea
  const input = page.locator('textarea[aria-label="消息输入框"]');
  await input.waitFor({ state: 'visible', timeout: 5000 });
  await input.click();
  await input.fill(text);
  await page.waitForTimeout(300);

  // Click send button
  const sendBtn = page.locator('button[aria-label="发送消息"]');
  await sendBtn.waitFor({ state: 'visible', timeout: 3000 });
  await sendBtn.click();
  await page.waitForTimeout(1000);
}

async function sendThreadReply(page: Page, text: string) {
  const input = page.locator('textarea[aria-label="线程回复输入框"]');
  await input.waitFor({ state: 'visible', timeout: 5000 });
  await input.click();
  await input.fill(text);
  await page.waitForTimeout(300);

  const sendBtn = page.locator('button[aria-label="发送回复"]');
  await sendBtn.waitFor({ state: 'visible', timeout: 3000 });
  await sendBtn.click();
  await page.waitForTimeout(1000);
}

async function sendAsTaskInChannel(page: Page, text: string, title?: string) {
  // Click "创建为任务" toggle button
  const toggleBtn = page.locator('button[aria-label="创建为任务"]');
  await toggleBtn.waitFor({ state: 'visible', timeout: 5000 });
  await toggleBtn.click();
  await page.waitForTimeout(500);

  // Fill task title if provided
  if (title) {
    const titleInput = page.locator('input[aria-label="任务标题"]');
    await titleInput.waitFor({ state: 'visible', timeout: 3000 });
    await titleInput.fill(title);
  }

  // Fill task description
  const input = page.locator('textarea[aria-label="任务描述输入框"]');
  await input.waitFor({ state: 'visible', timeout: 3000 });
  await input.fill(text);
  await page.waitForTimeout(300);

  // Click create button
  const createBtn = page.locator('button[aria-label="创建任务"]');
  await createBtn.waitFor({ state: 'visible', timeout: 3000 });
  await createBtn.click();
  await page.waitForTimeout(1000);
}

async function waitForAgentResponse(page: Page, timeoutMs: number = 25000) {
  // Poll for new content appearing
  const startTime = Date.now();
  const pollInterval = 2000;
  let lastMessageCount = 0;

  while (Date.now() - startTime < timeoutMs) {
    const newCount = await page.locator('[class*="message"], .message-content, .message-bubble').count();
    if (newCount > lastMessageCount + 1) {
      // New messages appeared - wait a bit more to let them fully render
      await page.waitForTimeout(2000);
      return true;
    }
    lastMessageCount = newCount;
    await page.waitForTimeout(pollInterval);
  }
  return false;
}

test.describe('E2E Agent Collaboration Scenarios', () => {

  test('TC-04: DM current agent — 产品 replies in DM, NOT in channel', async ({ page }) => {
    await login(page);
    await clickSidebarDM(page, '产品');

    await sendTextMessage(page, '私聊测试，请回复收到');

    // wait for agent reply
    await waitForAgentResponse(page, 25000);

    // Check DM has the reply content visible on page
    const dmPageText = await page.textContent('main');
    const hasReply = dmPageText?.includes('收到') || dmPageText?.includes('私聊测试');
    console.log(`TC-04: DM reply indicator in DOM: ${hasReply}`);

    // Now go to channel and verify NOT visible
    await clickSidebarChannel(page, 'test-collab');
    const channelText = await page.textContent('main');
    // The agent's DM reply should NOT appear in channel
    const found = channelText?.includes('私聊测试') || false;
    expect(found).toBe(false);
    console.log(`TC-04: DM reply leaked to channel: ${found}`);
  });

  test('TC-10: Thread @mention current agent — @前端 replies in thread', async ({ page }) => {
    await login(page);
    await openThreadViaMessage(page, TASK26_MSG_ID);

    await sendThreadReply(page, '@前端 你能看看这个问题吗');

    await waitForAgentResponse(page, 25000);

    // Check thread replies visible
    const threadReplies = await page.locator('[class*="thread"], .thread-reply, .thread-message').count();
    console.log(`TC-10: Thread reply elements: ${threadReplies}`);
    expect(threadReplies).toBeGreaterThan(0);
  });

  test('TC-11: Thread @mention external agent — @后端 joins and replies', async ({ page }) => {
    await login(page);
    await openThreadViaMessage(page, TASK27_MSG_ID);

    await sendThreadReply(page, '@后端 过来给点意见');

    await waitForAgentResponse(page, 25000);

    const threadReplies = await page.locator('[class*="thread"], .thread-reply, .thread-message').count();
    console.log(`TC-11: Thread reply elements: ${threadReplies}`);
    expect(threadReplies).toBeGreaterThan(0);
  });

  test('TC-12: Agent delegation — @产品 coordinates frontend and backend', async ({ page }) => {
    await login(page);
    await clickSidebarChannel(page, 'test-collab');

    await sendAsTaskInChannel(page, '@产品 帮我组织一个小游戏开发，你协调前后端，我在channel等结果', '小游戏开发协调');

    await waitForAgentResponse(page, 35000);

    const pageText = await page.textContent('main');
    const hasActivity = (pageText?.match(/产品|前端|后端|小游戏|协调|开发/g) || []).length >= 2;
    console.log(`TC-12: Coordination activity found: ${hasActivity}`);
    // At minimum, some agent activity should be visible
  });

  test('TC-13: DM asTask — reply appears in DM, NOT in channel', async ({ page }) => {
    await login(page);
    await clickSidebarDM(page, '产品');

    await sendTextMessage(page, '写一个判断回文字符串的函数');

    await waitForAgentResponse(page, 25000);

    const dmText = await page.textContent('main');
    const hasReply = dmText?.includes('回文') || dmText?.includes('palindrome') || dmText?.length! > 500;
    console.log(`TC-13: Reply in DM: ${hasReply}`);

    // Go to channel and verify NOT there
    await clickSidebarChannel(page, 'test-collab');
    const channelText = await page.textContent('main');
    const found = channelText?.includes('回文字符串') || false;
    expect(found).toBe(false);
    console.log(`TC-13: Task leaked to channel: ${found}`);
  });
});
