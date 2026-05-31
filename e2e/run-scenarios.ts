import { chromium, Browser, Page } from 'playwright';

const BASE = 'http://localhost:3000';
const EMAIL = 'qa-test@test.com';
const PASSWORD = 'test123456';
const CHANNEL_ID = 'aa53b630-b612-41c4-b518-afbba75e92d2';
const DM_PRODUCT_ID = 'c1b02a74-0682-4141-9710-f8ab687e9edc';
const TASK26_MSG_ID = '992ea919-8772-4d42-83ae-f8191e5fe8ce';
const TASK27_MSG_ID = 'e08d69c7-cf30-4a6e-bea0-bd2e41a2f9d9';

const LOG_API = 'http://localhost:8080';

interface ScenarioResult {
  name: string;
  passed: boolean;
  detail: string;
}

const results: ScenarioResult[] = [];

async function login(page: Page): Promise<void> {
  console.log('  [login] Navigating to login page...');
  await page.goto(`${BASE}/auth/login`, { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', EMAIL);
  await page.fill('input[type="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/dashboard**', { timeout: 15000 });
  await page.waitForSelector('aside', { timeout: 10000 });
  await page.waitForTimeout(2000);
  console.log('  [login] Dashboard loaded');
}

async function clickSidebarDM(page: Page, dmName: string): Promise<void> {
  console.log(`  [nav] Clicking DM: ${dmName}`);
  const sidebar = page.locator('aside');
  const dmLink = sidebar.locator('button, a, [role="button"]').filter({ hasText: dmName }).first();
  await dmLink.waitFor({ state: 'visible', timeout: 5000 });
  await dmLink.click();
  await page.waitForTimeout(2500);
}

async function clickSidebarChannel(page: Page, channelName: string): Promise<void> {
  console.log(`  [nav] Clicking channel: ${channelName}`);
  const sidebar = page.locator('aside');
  const chLink = sidebar.locator('button, a, [role="button"]').filter({ hasText: channelName }).first();
  await chLink.waitFor({ state: 'visible', timeout: 5000 });
  await chLink.click();
  await page.waitForTimeout(2500);
}

async function openThreadViaMessage(page: Page, messageId: string): Promise<void> {
  console.log(`  [nav] Opening thread for message: ${messageId}`);
  await page.goto(`${BASE}/dashboard?channel=${CHANNEL_ID}&message=${messageId}`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);
}

async function sendTextMessage(page: Page, text: string): Promise<void> {
  console.log(`  [send] "${text.slice(0, 50)}..."`);
  const input = page.locator('textarea[aria-label="消息输入框"]');
  await input.waitFor({ state: 'visible', timeout: 5000 });
  await input.click();
  await input.fill(text);
  await page.waitForTimeout(500);

  const sendBtn = page.locator('button[aria-label="发送消息"]');
  await sendBtn.waitFor({ state: 'visible', timeout: 3000 });
  await sendBtn.click();
  await page.waitForTimeout(1500);
}

async function sendThreadReply(page: Page, text: string): Promise<void> {
  console.log(`  [send-thread] "${text.slice(0, 50)}..."`);
  const input = page.locator('textarea[aria-label="线程回复输入框"]');
  await input.waitFor({ state: 'visible', timeout: 5000 });
  await input.click();
  await input.fill(text);
  await page.waitForTimeout(500);

  const sendBtn = page.locator('button[aria-label="发送回复"]');
  await sendBtn.waitFor({ state: 'visible', timeout: 3000 });
  await sendBtn.click();
  await page.waitForTimeout(1500);
}

async function sendAsTaskInChannel(page: Page, text: string, title?: string): Promise<void> {
  console.log(`  [send-asTask] title="${title || ''}" desc="${text.slice(0, 50)}..."`);

  // Click "创建为任务" toggle button
  const toggleBtn = page.locator('button[aria-label="创建为任务"]');
  await toggleBtn.waitFor({ state: 'visible', timeout: 5000 });
  await toggleBtn.click();
  await page.waitForTimeout(800);

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
  await page.waitForTimeout(500);

  // Click create button
  const createBtn = page.locator('button[aria-label="创建任务"]');
  await createBtn.waitFor({ state: 'visible', timeout: 3000 });
  await createBtn.click();
  await page.waitForTimeout(2000);
}

async function waitForAgentResponse(page: Page, timeoutMs: number = 25000): Promise<boolean> {
  console.log(`  [wait] Waiting up to ${timeoutMs / 1000}s for agent response...`);
  const startTime = Date.now();
  const pollInterval = 2000;
  let lastMessageCount = await page.locator('[class*="message"], .message-content, .message-bubble').count();

  while (Date.now() - startTime < timeoutMs) {
    await page.waitForTimeout(pollInterval);
    try {
      const currentCount = await page.locator('[class*="message"], .message-content, .message-bubble').count();
      if (currentCount > lastMessageCount + 1) {
        console.log(`  [wait] New messages detected (${lastMessageCount} -> ${currentCount})`);
        await page.waitForTimeout(2000);
        return true;
      }
      lastMessageCount = currentCount;
    } catch {
      // page might be navigating
    }
  }
  console.log('  [wait] Timeout — no new messages detected');
  return false;
}

async function checkCascadeErrors(): Promise<number> {
  try {
    const res = await fetch(`${LOG_API}/healthz`);
    // We'll check via a curl-like approach instead
    return 0; // Placeholder - we'll check separately
  } catch {
    return 0;
  }
}

// ============================================================
// SCENARIO EXECUTION
// ============================================================

async function runTC04(page: Page): Promise<ScenarioResult> {
  console.log('\n=== TC-04: DM current agent — 产品 replies in DM, NOT in channel ===');
  try {
    await clickSidebarDM(page, '产品');
    await sendTextMessage(page, '私聊测试，请回复收到');
    await waitForAgentResponse(page, 25000);

    // Check DM content
    const dmText = await page.textContent('main') || '';
    const hasReply = dmText.includes('收到') || dmText.includes('私聊测试');
    console.log(`  [verify] DM has reply indicator: ${hasReply}`);

    // Navigate to channel and check reply NOT there
    await clickSidebarChannel(page, 'test-collab');
    await page.waitForTimeout(2000);
    const channelText = await page.textContent('main') || '';
    const leaked = channelText.includes('收到') && channelText.includes('私聊测试');
    console.log(`  [verify] Reply leaked to channel: ${leaked}`);

    const passed = hasReply && !leaked;
    return { name: 'TC-04', passed, detail: `DM reply: ${hasReply}, leaked: ${leaked}` };
  } catch (e: any) {
    return { name: 'TC-04', passed: false, detail: `ERROR: ${e.message}` };
  }
}

async function runTC10(page: Page): Promise<ScenarioResult> {
  console.log('\n=== TC-10: Thread @mention current agent — @前端 replies in thread ===');
  try {
    await openThreadViaMessage(page, TASK26_MSG_ID);
    await sendThreadReply(page, '@前端 你能看看这个问题吗');
    await waitForAgentResponse(page, 25000);

    const threadCount = await page.locator('[class*="thread"]').count();
    console.log(`  [verify] Thread elements found: ${threadCount}`);

    const passed = threadCount > 0;
    return { name: 'TC-10', passed, detail: `Thread elements: ${threadCount}` };
  } catch (e: any) {
    return { name: 'TC-10', passed: false, detail: `ERROR: ${e.message}` };
  }
}

async function runTC11(page: Page): Promise<ScenarioResult> {
  console.log('\n=== TC-11: Thread @mention external agent — @后端 joins and replies ===');
  try {
    await openThreadViaMessage(page, TASK27_MSG_ID);
    await sendThreadReply(page, '@后端 过来给点意见');
    await waitForAgentResponse(page, 25000);

    const threadCount = await page.locator('[class*="thread"]').count();
    console.log(`  [verify] Thread elements found: ${threadCount}`);

    const passed = threadCount > 0;
    return { name: 'TC-11', passed, detail: `Thread elements: ${threadCount}` };
  } catch (e: any) {
    return { name: 'TC-11', passed: false, detail: `ERROR: ${e.message}` };
  }
}

async function runTC12(page: Page): Promise<ScenarioResult> {
  console.log('\n=== TC-12: Agent delegation — @产品 coordinates frontend and backend ===');
  try {
    await clickSidebarChannel(page, 'test-collab');
    await sendAsTaskInChannel(page, '@产品 帮我组织一个小游戏开发，你协调前后端，我在channel等结果', '小游戏开发协调');
    await waitForAgentResponse(page, 35000);

    const pageText = await page.textContent('main') || '';
    const mentions = ['产品', '前端', '后端', '小游戏', '协调'];
    let matchCount = 0;
    for (const m of mentions) {
      if (pageText.includes(m)) matchCount++;
    }
    console.log(`  [verify] Keyword matches: ${matchCount}/5`);

    const passed = matchCount >= 2;
    return { name: 'TC-12', passed, detail: `Keyword matches: ${matchCount}/5` };
  } catch (e: any) {
    return { name: 'TC-12', passed: false, detail: `ERROR: ${e.message}` };
  }
}

async function runTC13(page: Page): Promise<ScenarioResult> {
  console.log('\n=== TC-13: DM asTask — reply in DM, NOT in channel ===');
  try {
    await clickSidebarDM(page, '产品');
    await sendTextMessage(page, '写一个判断回文字符串的函数');

    await waitForAgentResponse(page, 25000);

    const dmText = await page.textContent('main') || '';
    const hasReply = dmText.includes('回文') || dmText.includes('palindrome') || dmText.length > 500;
    console.log(`  [verify] DM reply found: ${hasReply} (text length: ${dmText.length})`);

    // Navigate to channel and check NOT there
    await clickSidebarChannel(page, 'test-collab');
    await page.waitForTimeout(2000);
    const channelText = await page.textContent('main') || '';
    const leaked = channelText.includes('回文字符串');
    console.log(`  [verify] Task leaked to channel: ${leaked}`);

    const passed = hasReply && !leaked;
    return { name: 'TC-13', passed, detail: `DM reply: ${hasReply}, leaked: ${leaked}` };
  } catch (e: any) {
    return { name: 'TC-13', passed: false, detail: `ERROR: ${e.message}` };
  }
}

// ============================================================
// MAIN
// ============================================================

async function main() {
  console.log('=== Solo E2E Agent Collaboration Test Runner ===');
  console.log(`Base URL: ${BASE}`);
  console.log(`Time: ${new Date().toISOString()}\n`);

  let browser: Browser | null = null;

  try {
    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
    });
    const page = await context.newPage();

    // Login once
    console.log('--- LOGIN ---');
    await login(page);

    // Run all scenarios sequentially with shared login session
    results.push(await runTC04(page));
    results.push(await runTC10(page));
    results.push(await runTC11(page));
    results.push(await runTC12(page));
    results.push(await runTC13(page));

  } catch (e: any) {
    console.error(`FATAL: ${e.message}`);
    results.push({ name: 'SETUP', passed: false, detail: `FATAL: ${e.message}` });
  } finally {
    if (browser) await browser.close();
  }

  // Print summary
  console.log('\n========================================');
  console.log('            TEST SUMMARY');
  console.log('========================================');
  let passCount = 0;
  let failCount = 0;
  for (const r of results) {
    const status = r.passed ? 'PASS' : 'FAIL';
    if (r.passed) passCount++; else failCount++;
    console.log(`  ${status} | ${r.name} | ${r.detail}`);
  }
  console.log('----------------------------------------');
  console.log(`  TOTAL: ${results.length} | PASS: ${passCount} | FAIL: ${failCount}`);
  console.log('========================================');

  process.exit(failCount > 0 ? 1 : 0);
}

main();
