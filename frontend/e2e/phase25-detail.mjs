import { chromium } from 'playwright';

const BASE = 'http://localhost:3000';

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  await page.goto(`${BASE}/auth/login`, { waitUntil: 'networkidle' });
  await page.waitForSelector('#email', { timeout: 10000 });
  await page.fill('#email', '1271123275@qq.com');
  await page.fill('#password', 'Test12345!');
  await page.click('button:has-text("登录")');
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(3000);

  console.log('=== Detailed Message Card Analysis ===\n');

  // Get message list items
  const listItems = page.locator('[role="listitem"]');
  const count = await listItems.count();
  console.log(`Total messages: ${count}\n`);

  // Analyze first 8 messages in detail
  for (let i = 0; i < Math.min(count, 8); i++) {
    const item = listItems.nth(i);
    const text = await item.textContent();
    const truncated = text.substring(0, 120).replace(/\n/g, ' ');
    console.log(`Message ${i}: "${truncated}..."`);

    // Check specific elements within message
    const badges = item.locator('[class*="badge"], [class*="Badge"]');
    const badgeCount = await badges.count();
    if (badgeCount > 0) {
      for (let j = 0; j < badgeCount; j++) {
        const badgeText = await badges.nth(j).textContent();
        console.log(`  -> Badge: "${badgeText}"`);
      }
    }

    // Check for reply info
    const replyEls = item.locator('text=/条回复/');
    if (await replyEls.count() > 0) {
      const rt = await replyEls.first().textContent();
      console.log(`  -> Reply text: "${rt}"`);
    }

    // Check for task status display
    const taskEls = item.locator('text=/in_review|todo|in_progress|done/');
    if (await taskEls.count() > 0) {
      const tt = await taskEls.first().textContent();
      console.log(`  -> Task status: "${tt}"`);
    }

    console.log('');
  }

  // Check the first message HTML structure
  console.log('=== First message inner HTML (first 1000 chars) ===');
  const firstMsg = listItems.first();
  const html = await firstMsg.innerHTML();
  console.log(html.substring(0, 1000));

  console.log('\n=== Global DOM search ===');
  // Search for task-related patterns
  const bodyHTML = await page.locator('body').innerHTML();
  const patterns = ['task-status', 'task-badge', 'TaskBadge', 'task_card', 'TaskCard',
                    'reply-count', 'reply_count', 'ReplyCount', 'replyCount',
                    'has_unread', 'has-unread', 'unread-thread', 'unread_thread',
                    '条回复', 'in_review', 'in_progress', 'task_status'];
  for (const p of patterns) {
    const found = bodyHTML.includes(p);
    if (found) {
      const idx = bodyHTML.indexOf(p);
      const snippet = bodyHTML.substring(Math.max(0, idx - 30), idx + p.length + 30).replace(/\n/g, '\\n');
      console.log(`  FOUND "${p}" at context: ...${snippet}...`);
    } else {
      console.log(`  MISSING "${p}"`);
    }
  }

  // Check ThreadPanel more carefully
  console.log('\n=== ThreadPanel detail ===');
  await listItems.nth(0).hover();
  await page.waitForTimeout(300);
  const replyBtn = listItems.nth(0).locator('button[aria-label*="回复"]');
  if (await replyBtn.count() > 0) {
    await replyBtn.first().click();
    await page.waitForTimeout(2000);

    // Get thread panel content
    const threadPanel = page.locator('[class*="thread"], [class*="Thread"], [class*="panel"], [class*="Panel"]').first();
    if (await threadPanel.count() > 0) {
      const panelHTML = await threadPanel.innerHTML();
      console.log('ThreadPanel HTML (first 800 chars):');
      console.log(panelHTML.substring(0, 800));
    }

    await page.screenshot({ path: '/tmp/phase25-thread-detail.png' });

    // Close
    const closeBtn = page.locator('button[aria-label="关闭线程面板"]');
    if (await closeBtn.count() > 0) {
      await closeBtn.click();
    }
  }

  await browser.close();
}

main().catch(err => {
  console.error('FATAL:', err.message);
  process.exit(1);
});
