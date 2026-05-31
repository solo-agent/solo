import { chromium } from 'playwright';

const BASE = 'http://localhost:3000';

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  const findings = [];

  console.log('=== Phase 2.5 UI Verification ===\n');

  // 1. Login
  console.log('1. Login...');
  await page.goto(`${BASE}/auth/login`, { waitUntil: 'networkidle' });
  await page.waitForSelector('#email', { timeout: 10000 });
  await page.fill('#email', '1271123275@qq.com');
  await page.fill('#password', 'Test12345!');
  await page.click('button:has-text("登录")');
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(3000);
  console.log('   OK - Logged in, on dashboard\n');

  // 2. Message list
  console.log('2. Message list...');
  const listItems = await page.locator('[role="listitem"]').count();
  console.log(`   ${listItems} message items found`);

  // 3. Check for reply_count display
  console.log('\n3. Checking reply_count...');
  const replyCountEls = page.locator('text=/\\d+\\s*条回复/');
  const replyCount = await replyCountEls.count();
  console.log(`   '条回复' elements: ${replyCount}`);
  if (replyCount === 0) {
    findings.push('FAIL: reply_count not displayed on message cards ("条回复" not found in DOM)');
  } else {
    const texts = [];
    for (let i = 0; i < Math.min(replyCount, 3); i++) {
      texts.push(await replyCountEls.nth(i).textContent());
    }
    console.log(`   Reply count texts: ${JSON.stringify(texts)}`);
  }

  // 4. Check for TaskBadge
  console.log('\n4. Checking TaskBadge...');
  // Look for any element that contains task status text
  const pageText = await page.locator('body').innerText();

  for (const status of ['in_review', 'todo', 'in_progress', 'done']) {
    const count = (pageText.match(new RegExp(status, 'g')) || []).length;
    console.log(`   '${status}' occurrences: ${count}`);
  }

  // Look for task-related badges by searching for "Agent" and task status patterns
  const taskIndicators = await page.locator('[class*="task"], [class*="Task"], [class*="badge"], [class*="Badge"]').count();
  console.log(`   Task/badge-related elements: ${taskIndicators}`);

  // Look specifically for the task claimer name display
  for (const name of ['二狗', '三毛', '麻子']) {
    const els = await page.locator(`text=${name}`).count();
    console.log(`   '${name}' on page: ${els}`);
  }

  // 5. Look for unread indicators
  console.log('\n5. Checking unread indicators...');
  const allElements = await page.locator('*').all();
  let unreadDots = 0;
  for (const el of allElements) {
    const cls = await el.getAttribute('class');
    if (cls && (cls.includes('pink') || cls.includes('red-') || cls.includes('bg-red') || cls.includes('bg-pink'))) {
      unreadDots++;
    }
  }
  console.log(`   Pink/red elements (potential unread dots): ${unreadDots}`);

  // Check for any circle/dot elements specifically
  const dotElements = await page.locator('[class*="dot"], [class*="Dot"], [class*="indicator"]').count();
  console.log(`   Dot/Indicator elements: ${dotElements}`);

  // Look for has_unread_thread visual indicators
  const unreadBadges = await page.locator('[class*="unread"]').count();
  console.log(`   Elements with 'unread' class: ${unreadBadges}`);

  // 6. Open thread panel
  console.log('\n6. Testing ThreadPanel...');
  // Find a message with has_unread_thread (needs thread reply button)
  // Hover over message list items and look for reply buttons
  const messages = page.locator('[role="listitem"]');
  const msgCount = await messages.count();
  console.log(`   Total messages: ${msgCount}`);

  let threadOpened = false;
  for (let i = 0; i < Math.min(msgCount, 5); i++) {
    const msg = messages.nth(i);
    await msg.hover();
    await page.waitForTimeout(300);
    const replyBtn = msg.locator('button[aria-label*="回复"]');
    if (await replyBtn.count() > 0) {
      console.log(`   Hovering message ${i}...`);
      await replyBtn.first().click();
      await page.waitForTimeout(2000);

      // Check if ThreadPanel appeared
      const threadHeader = page.locator('h3:has-text("线程"), h2:has-text("线程"), [class*="thread"]');
      if (await threadHeader.count() > 0) {
        console.log('   ThreadPanel opened!');
        threadOpened = true;

        // Check for reply count in header
        const headerText = await threadHeader.first().textContent();
        console.log(`   Thread header: "${headerText}"`);

        await page.screenshot({ path: '/tmp/phase25-thread.png' });
        console.log('   Thread screenshot saved.');

        // Close thread
        const closeBtn = page.locator('button[aria-label="关闭线程面板"]');
        if (await closeBtn.count() > 0) {
          await closeBtn.click();
          await page.waitForTimeout(1000);
        }
        break;
      }
    }
  }
  if (!threadOpened) {
    console.log('   No reply button found on first 5 messages');
    findings.push('WARN: Could not open ThreadPanel to verify reply count in header');
  }

  // 7. Dashboard screenshot
  await page.screenshot({ path: '/tmp/phase25-dashboard.png', fullPage: true });
  console.log('\n7. Dashboard screenshot saved to /tmp/phase25-dashboard.png');

  // 8. Look for error states
  console.log('\n8. Error checks...');
  const errors = await page.locator('text=发送失败').count();
  console.log(`   '发送失败' messages: ${errors}`);

  // Check for loading skeletons
  const skeletons = await page.locator('[class*="skeleton"], [class*="Skeleton"]').count();
  console.log(`   Skeleton loaders: ${skeletons}`);

  // Check WebSocket connectivity
  const reconnect = await page.locator('text=正在重新连接').count();
  console.log(`   Reconnection banners: ${reconnect}`);
  if (reconnect > 0) {
    findings.push('WARN: WebSocket reconnection banner visible');
  }

  // Summary
  console.log('\n=== Findings Summary ===');
  if (findings.length === 0) {
    console.log('All checks passed!');
  } else {
    for (const f of findings) {
      console.log(`  ${f}`);
    }
  }

  await browser.close();
}

main().catch(err => {
  console.error('FATAL:', err.message);
  process.exit(1);
});
