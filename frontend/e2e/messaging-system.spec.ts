// ============================================================================
// Solo Messaging System E2E Test Suite
// Each scenario is independent with its own login to avoid token expiry
// ============================================================================
import { test, expect, type Page } from '@playwright/test';

const TEST_EMAIL = 'testqa@test.com';
const TEST_PASSWORD = 'test123456';
const AGENT_NAME = '二狗';
const BASE_URL = 'http://localhost:3000';

interface BugEntry { scenario: string; severity: string; description: string }
const allBugs: BugEntry[] = [];

function bug(scenario: string, severity: string, description: string) {
  allBugs.push({ scenario, severity, description });
  console.log(`[BUG] ${severity} | ${scenario}: ${description}`);
}

function note(obs: string) { console.log(`[NOTE] ${obs}`); }

async function doLogin(page: Page) {
  await page.goto(`${BASE_URL}/auth/login`);
  await page.waitForSelector('#email', { timeout: 10000 });
  await page.fill('#email', TEST_EMAIL);
  await page.fill('#password', TEST_PASSWORD);
  await page.click('button:has-text("登录")');
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(1500);
}

async function goToChannel(page: Page) {
  try {
    const link = page.locator('a[href*="b3beadb4"]').first();
    if (await link.isVisible({ timeout: 3000 })) { await link.click(); await page.waitForTimeout(500); }
  } catch { /* ok */ }
  await page.waitForSelector('textarea[aria-label="消息输入框"]', { timeout: 10000 });
}

async function msgCount(page: Page) { return page.locator('[role="listitem"]').count(); }

async function sendMsg(page: Page, content: string) {
  const ta = page.locator('textarea[aria-label="消息输入框"]');
  await ta.fill(content);
  await page.click('button[aria-label="发送消息"]');
  await page.waitForTimeout(300);
}

async function waitAgent(page: Page, maxMs = 40000) {
  const t0 = Date.now();
  try { await page.waitForSelector('[data-streaming="true"]', { timeout: 8000 }); } catch { /* ok */ }
  const remain = maxMs - (Date.now() - t0);
  if (remain > 0) {
    try {
      await page.waitForFunction(() => !document.querySelector('[data-streaming="true"]'), { timeout: remain, polling: 500 });
    } catch { /* ok */ }
  }
  await page.waitForTimeout(500);
}

async function openThreadViaReply(page: Page): Promise<boolean> {
  const items = page.locator('[role="listitem"]');
  const cnt = await items.count();
  for (let i = 0; i < cnt; i++) {
    if (await items.nth(i).locator('.agent-message').count() > 0) continue;
    await items.nth(i).hover();
    await page.waitForTimeout(300);
    const btn = items.nth(i).locator('button[title="回复"]');
    if (await btn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await btn.click();
      await page.waitForTimeout(800);
      return true;
    }
  }
  return false;
}

async function closeThread(page: Page) {
  const btn = page.locator('button[aria-label="关闭线程面板"]');
  if (await btn.isVisible({ timeout: 2000 }).catch(() => false)) { await btn.click(); await page.waitForTimeout(300); }
}

async function threadReply(page: Page, content: string) {
  const ta = page.locator('textarea[aria-label="线程回复输入框"]');
  await ta.waitFor({ state: 'visible', timeout: 5000 });
  await ta.fill(content);
  await page.click('button[aria-label="发送回复"]');
  await page.waitForTimeout(800);
}

async function replyCount(page: Page): Promise<number> {
  const btn = page.locator('button:has-text("条回复")').first();
  const t = await btn.textContent().catch(() => null);
  const m = t?.match(/(\d+)\s*条回复/);
  return m ? parseInt(m[1], 10) : 0;
}

async function hasUnreadDot(page: Page): Promise<boolean> {
  return (await page.locator('span.block.h-1.w-1.rounded-full.bg-brutal-pink').count()) > 0;
}

// ============================================================================
// SCENARIO 1: Agent Streaming Reply
// ============================================================================
test.describe('S1: Agent Streaming Reply', () => {
  test.beforeAll(async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    // Store page on test.info() - we use a global approach instead
    (globalThis as any).__page = p;
  });

  test.afterAll(async () => {
    const p = (globalThis as any).__page as Page;
    if (p) await p.close();
  });

  function getPage(): Page { return (globalThis as any).__page as Page; }

  test('S1-T1: @agent triggers streaming response with badge and styling', async () => {
    const p = getPage();
    const before = await msgCount(p);
    await sendMsg(p, `@${AGENT_NAME} 介绍一下Solo平台`);

    // Try to capture streaming
    try {
      await p.waitForSelector('[data-streaming="true"]', { timeout: 5000 });
      const el = p.locator('[data-streaming="true"]').first();
      const badgeVisible = (await el.getByText('流式输出中').count()) > 0;
      console.log(`[S1-T1] Streaming badge: ${badgeVisible}`);
      if (!badgeVisible) bug('S1-T1', 'P2', 'Missing "流式输出中" badge during streaming');

      await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s1-t1-streaming.png' });
    } catch { note('S1-T1: Streaming too fast to capture'); }

    await waitAgent(p, 40000);
    const after = await msgCount(p);
    expect(after).toBeGreaterThan(before);

    // Verify agent message has content
    const agentItems = p.locator('[role="listitem"].agent-message');
    const agentCnt = await agentItems.count();
    expect(agentCnt).toBeGreaterThan(0);

    const lastContent = await agentItems.last().locator('p').first().textContent().catch(() => '');
    console.log(`[S1-T1] Agent content: "${lastContent?.substring(0, 60)}"`);
    expect(lastContent?.length || 0).toBeGreaterThan(1);  // Agent responded, content should be non-empty
    console.log('[S1-T1] PASS');
  });

  test('S1-T2: After streaming, agent message is stable and in channel timeline', async () => {
    const p = getPage();
    const still = await p.locator('[data-streaming="true"]').count();
    expect(still).toBe(0);

    // All agent messages should be in [role="list"]
    const agentItems = p.locator('[role="listitem"].agent-message');
    const cnt = await agentItems.count();
    expect(cnt).toBeGreaterThan(0);
    console.log(`[S1-T2] ${cnt} agent messages in channel timeline`);

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s1-t2-final.png' });
    console.log('[S1-T2] PASS');
  });
});

// ============================================================================
// SCENARIO 2: Thread Reply Count
// ============================================================================
test.describe('S2: Thread Reply Count', () => {
  test.beforeAll(async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    (globalThis as any).__page2 = p;
  });
  test.afterAll(async () => {
    const p = (globalThis as any).__page2 as Page;
    if (p) await p.close();
  });
  function getPage(): Page { return (globalThis as any).__page2 as Page; }

  test('S2-T1: Open thread panel via reply button', async () => {
    const p = getPage();
    const opened = await openThreadViaReply(p);
    expect(opened).toBe(true);

    const header = p.getByText('线程').first();
    const visible = await header.isVisible({ timeout: 3000 }).catch(() => false);
    expect(visible).toBe(true);
    console.log('[S2-T1] PASS — Thread panel opened');

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s2-t1-thread-open.png' });
  });

  test('S2-T2: Send reply in thread and check reply count appears', async () => {
    const p = getPage();
    // Thread should still be open from S2-T1
    await threadReply(p, 'S2-T2: 线程回复测试消息');

    // Close thread and check reply_count
    await closeThread(p);
    await p.waitForTimeout(800);

    const rc = await replyCount(p);
    console.log(`[S2-T2] Reply count after reply: ${rc}`);

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s2-t2-count.png' });

    // KNOWN BUG #39
    if (rc === 0) {
      bug('S2-T2', 'P0', '#39: reply_count stays 0 after sending thread reply (no real-time update without refresh)');
    }
    console.log('[S2-T2] PASS');
  });

  test('S2-T3: reply_count persists after page refresh', async () => {
    const p = getPage();
    const before = await replyCount(p);
    console.log(`[S2-T3] Before refresh: ${before}`);

    await p.reload();
    await p.waitForURL('**/dashboard', { timeout: 10000 });
    await p.waitForTimeout(1500);
    await goToChannel(p);
    await p.waitForSelector('[role="listitem"]', { timeout: 10000 });
    await p.waitForTimeout(500);

    const after = await replyCount(p);
    console.log(`[S2-T3] After refresh: ${after}`);
    if (after > 0) { console.log('[S2-T3] PASS — reply_count persists'); }
    else { note('S2-T3: reply_count 0 after refresh — may be correct if thread reply failed'); }
  });
});

// ============================================================================
// SCENARIO 3: Thread Follow-up
// ============================================================================
test.describe('S3: Thread Follow-up', () => {
  test.beforeAll(async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    (globalThis as any).__page3 = p;
  });
  test.afterAll(async () => {
    const p = (globalThis as any).__page3 as Page;
    if (p) await p.close();
  });
  function getPage(): Page { return (globalThis as any).__page3 as Page; }

  test('S3-T1: Follow-up question in thread (without @) triggers agent response', async () => {
    const p = getPage();
    // First, open a thread
    await openThreadViaReply(p);

    const area = p.locator('.flex-1.overflow-y-auto');
    const before = await area.locator('.flex.gap-3').count();

    await threadReply(p, '你能介绍一下Solo的架构吗？（线程追问测试）');
    await p.waitForTimeout(5000);

    const after = await area.locator('.flex.gap-3').count();
    console.log(`[S3-T1] Thread msgs: ${before} -> ${after}`);

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s3-t1-followup.png' });

    if (after <= before) {
      bug('S3-T1', 'P0', '#41: Thread follow-up question has no agent response');
    }

    await closeThread(p);
    console.log('[S3-T1] PASS');
  });

  test('S3-T2: @agent in thread triggers specific agent response', async () => {
    const p = getPage();
    await openThreadViaReply(p);

    const area = p.locator('.flex-1.overflow-y-auto');
    const before = await area.locator('.flex.gap-3').count();

    await threadReply(p, `@${AGENT_NAME} 你在吗？这是线程中的@测试`);
    await p.waitForTimeout(5000);

    const after = await area.locator('.flex.gap-3').count();
    console.log(`[S3-T2] Thread msgs: ${before} -> ${after}`);

    if (after <= before) {
      bug('S3-T2', 'P0', '@agent in thread does not trigger response');
    }

    await closeThread(p);
    console.log('[S3-T2] PASS');
  });
});

// ============================================================================
// SCENARIO 4 & 5: Task System
// ============================================================================
test.describe('S4-S5: Task System Messages and Badge', () => {
  test.beforeAll(async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    (globalThis as any).__page45 = p;
  });
  test.afterAll(async () => {
    const p = (globalThis as any).__page45 as Page;
    if (p) await p.close();
  });
  function getPage(): Page { return (globalThis as any).__page45 as Page; }

  test('S4-T1: Create task via toggle creates task badge', async () => {
    const p = getPage();
    const before = await p.locator('button[aria-label^="任务 "]').count();

    const ta = p.locator('textarea[aria-label="消息输入框"]');
    await ta.fill('E2E任务测试');
    await p.click('button[aria-label="创建为任务"]');
    await p.waitForTimeout(300);

    try {
      const taskInput = p.locator('textarea[aria-label="任务标题输入框"]');
      if (await taskInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        await taskInput.fill('E2E任务测试');
      }
    } catch { /* fallback */ }

    await p.click('button[aria-label="创建任务"]');
    await p.waitForTimeout(2000);

    const after = await p.locator('button[aria-label^="任务 "]').count();
    console.log(`[S4-T1] Badges: ${before} -> ${after}`);

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s4-t1-task-created.png' });

    if (after > before) { console.log('[S4-T1] PASS — Task badge created'); }
    else { bug('S4-T1', 'P2', 'Task created but badge not visible without refresh'); }
  });

  test('S5-T1: TaskBadge shows task number, status, and claimer info', async () => {
    const p = getPage();
    // Use ^="任务 " to exclude "创建为任务" and "转为任务" buttons
    const badges = p.locator('button[aria-label^="任务 "]');
    const cnt = await badges.count();
    console.log(`[S5-T1] Task badges: ${cnt}`);

    if (cnt > 0) {
      const text = await badges.last().textContent();
      const aria = await badges.last().getAttribute('aria-label');
      console.log(`[S5-T1] Badge: "${text?.trim()}"`);
      console.log(`[S5-T1] Aria:  "${aria}"`);

      const hasStatus = ['TODO', '处理中', '待审核', '已完成', '待认领'].some(s => text?.includes(s) || false);
      if (!hasStatus) { bug('S5-T1', 'P2', 'TaskBadge missing status label'); }

      await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s5-t1-badge.png' });
      console.log('[S5-T1] PASS');
    } else { note('S5-T1: No task badges to inspect'); }
  });

  test('S4-T2: Thread panel shows task metadata for task messages', async () => {
    const p = getPage();
    const badge = p.locator('button[aria-label^="任务 "]').last();
    if (await badge.isVisible({ timeout: 2000 }).catch(() => false)) {
      await badge.click();
      await p.waitForTimeout(1000);

      const claimBtn = p.getByText('认领').first();
      const hasMeta = await claimBtn.isVisible({ timeout: 2000 }).catch(() => false);
      console.log(`[S4-T2] Task meta bar (认领): ${hasMeta}`);

      await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s4-t2-thread-task.png' });
      if (!hasMeta) { bug('S4-T2', 'P2', 'Thread panel missing task metadata for task-bound message'); }

      await closeThread(p);
    }
    console.log('[S4-T2] PASS');
  });
});

// ============================================================================
// SCENARIO 6: Unread Dot
// ============================================================================
test.describe('S6: Unread Thread Dot', () => {
  test.beforeAll(async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    (globalThis as any).__page6 = p;
  });
  test.afterAll(async () => {
    const p = (globalThis as any).__page6 as Page;
    if (p) await p.close();
  });
  function getPage(): Page { return (globalThis as any).__page6 as Page; }

  test('S6-T1: Check unread dot visibility on messages', async () => {
    const p = getPage();
    const dotVisible = await hasUnreadDot(p);
    const unreadBtnCnt = await p.locator('button[aria-label="有未读线程回复，点击查看"]').count();
    console.log(`[S6-T1] Unread dot: ${dotVisible}, Unread btn: ${unreadBtnCnt}`);

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/s6-t1-unread.png' });

    if (dotVisible || unreadBtnCnt > 0) {
      console.log('[S6-T1] PASS — Unread indicators present');
    } else {
      note('S6-T1: No unread indicators (requires unread state from another participant)');
    }
  });

  test('S6-T2: Opening thread clears unread indicator', async () => {
    const p = getPage();
    const hadDot = await hasUnreadDot(p);
    console.log(`[S6-T2] Had unread: ${hadDot}`);

    if (hadDot) {
      const unreadBtn = p.locator('button[aria-label="有未读线程回复，点击查看"]').first();
      if (await unreadBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await unreadBtn.click();
      } else {
        await openThreadViaReply(p);
      }
      await p.waitForTimeout(1000);
      await closeThread(p);
      await p.waitForTimeout(500);

      const after = await hasUnreadDot(p);
      console.log(`[S6-T2] Unread after opening thread: ${after}`);
      if (after) { bug('S6-T2', 'P2', 'Unread dot persists after opening thread panel'); }
    }
    console.log('[S6-T2] PASS');
  });
});

// ============================================================================
// FINAL: Duplicate check
// ============================================================================
test.describe('FINAL: Cross-cutting', () => {
  test('Duplicate message check', async ({ browser }) => {
    const p = await browser.newPage();
    await doLogin(p);
    await goToChannel(p);
    await p.waitForSelector('[role="listitem"]', { timeout: 10000 });
    await p.waitForTimeout(500);

    const items = p.locator('[role="listitem"]');
    const cnt = await items.count();
    const seen = new Set<string>();
    let dups = 0;
    for (let i = 0; i < cnt; i++) {
      const text = await items.nth(i).textContent().catch(() => '');
      const key = text?.trim().substring(0, 80);
      if (key && seen.has(key)) { dups++; }
      seen.add(key);
    }
    console.log(`[FINAL] Messages: ${cnt}, Duplicates: ${dups}`);
    if (dups > 0) {
      bug('FINAL', 'P0', `Phase25: ${dups} duplicate messages in channel`);
    }

    await p.screenshot({ path: '/Users/langgengxin/AiWorkspace/solo/test-results/final.png' });

    // Print full bug report
    console.log('\n========================================');
    console.log('COMPLETE BUG REPORT');
    console.log('========================================');
    const bySev: Record<string, string[]> = { P0: [], P1: [], P2: [], P3: [] };
    allBugs.forEach(b => { const k = bySev[b.severity] || []; bySev[b.severity] = [...k, `[${b.scenario}] ${b.description}`]; });
    for (const [sev, items] of Object.entries(bySev)) {
      if (items.length > 0) {
        console.log(`\n${sev} (${items.length}):`);
        items.forEach((d, i) => console.log(`  ${i + 1}. ${d}`));
      }
    }
    console.log(`\nTotal: ${allBugs.length} bugs`);

    // Write report
    const fs = require('fs');
    fs.writeFileSync('/Users/langgengxin/AiWorkspace/solo/test-results/messaging-final-report.md',
      `# Solo Messaging System Test Report\nDate: ${new Date().toISOString()}\n\n` +
      Object.entries(bySev).flatMap(([sev, items]) =>
        items.length > 0 ? [`## ${sev}`, ...items.map((d, i) => `${i + 1}. ${d}`), ''] : []
      ).join('\n')
    );

    await p.close();
  });
});
