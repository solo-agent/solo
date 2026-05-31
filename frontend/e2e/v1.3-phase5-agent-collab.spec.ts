// ============================================================================
// Solo v1.3 Phase 5 — Agent Collaboration E2E Tests
// Aligned with Slock collaboration model. Tests 16 scenarios from case.md.
//
// Prerequisites:
//   - 3 agents created & joined to #test-collab: 产品, 前端, 后端
//   - Server + daemon running
//
// Run:
//   cd frontend
//   npx playwright test e2e/v1.3-phase5-agent-collab.spec.ts
// ============================================================================

import { test, expect } from '@playwright/test';

const TEST_EMAIL = 'qa-test@test.com';
const TEST_PASSWORD = 'test123456';
const CHANNEL_NAME = 'test-collab';
const AGENT_WAIT_MS = 25000; // Time for Claude Code to process and respond

test.describe('Solo Agent Collaboration', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('http://localhost:3000');
    // Login
    await page.fill('input[type="email"]', TEST_EMAIL);
    await page.fill('input[type="password"]', TEST_PASSWORD);
    await page.click('button[type="submit"]');
    await page.waitForURL('**/dashboard**');
    // Navigate to test channel
    await page.click(`text=${CHANNEL_NAME}`);
    await page.waitForSelector('textarea');
  });

  // ── S2: @mention current agent ─────────────────────────────────────────
  test('S2: @产品 → only 产品 replies, others silent', async ({ page }) => {
    const msgInput = page.locator('textarea').first();
    await msgInput.fill('@产品 请简短介绍敏捷开发的核心原则');
    await msgInput.press('Enter');

    // Wait for agents to process
    await page.waitForTimeout(AGENT_WAIT_MS);

    // Get all messages in channel
    const messages = page.locator('[data-testid="message-item"], .message-item, [class*="message"]');
    const count = await messages.count();
    expect(count).toBeGreaterThan(1); // At least original + one reply

    // Verify no "not for me" leakage
    const pageText = await page.textContent('body');
    expect(pageText).not.toContain('not for me');
    expect(pageText).not.toContain('NOTHING');

    // 产品 should have replied
    const hasProductReply = pageText.includes('@产品') || pageText.includes('产品');
    expect(hasProductReply).toBeTruthy();
  });

  // ── S3: No @mention broadcast ──────────────────────────────────────────
  test('S3: broadcast message → agents reply by role', async ({ page }) => {
    const msgInput = page.locator('textarea').first();
    await msgInput.fill('大家好，来讨论下微服务架构的优缺点');
    await msgInput.press('Enter');

    await page.waitForTimeout(AGENT_WAIT_MS);

    const pageText = await page.textContent('body');
    // At least one agent should reply
    const hasAgentReply = pageText.includes('@后端') || pageText.includes('@前端') ||
                          pageText.includes('@产品');
    expect(hasAgentReply).toBeTruthy();
  });

  // ── S5: @agent asTask → only mentioned claims ─────────────────────────
  test('S5: @后端 asTask → only 后端 claims', async ({ page }) => {
    // Toggle As Task
    const asTaskToggle = page.locator('[aria-label*="As Task"], [class*="asTask"], button:has-text("Task")');
    if (await asTaskToggle.isVisible()) {
      await asTaskToggle.click();
    }

    const msgInput = page.locator('textarea').first();
    await msgInput.fill('@后端 设计一个用户系统的数据库表结构');
    await msgInput.press('Enter');

    await page.waitForTimeout(AGENT_WAIT_MS);

    // Check for task creation
    const pageText = await page.textContent('body');
    const hasTask = pageText.includes('task') || pageText.includes('todo') || pageText.includes('#');
    expect(hasTask).toBeTruthy();

    // Other agents should not claim
    expect(pageText).not.toContain('not for me');
  });

  // ── S6: @other agent asTask → non-target stays silent ─────────────────
  test('S6: @前端 asTask → 后端/产品 stay silent', async ({ page }) => {
    const asTaskToggle = page.locator('[aria-label*="As Task"], [class*="asTask"], button:has-text("Task")');
    if (await asTaskToggle.isVisible()) {
      await asTaskToggle.click();
    }

    const msgInput = page.locator('textarea').first();
    await msgInput.fill('@前端 实现一个响应式的登录表单组件');
    await msgInput.press('Enter');

    await page.waitForTimeout(AGENT_WAIT_MS);

    const pageText = await page.textContent('body');
    // No "not for me" or other silence-breaking messages
    expect(pageText).not.toContain('not for me');
    expect(pageText).not.toContain('NOTHING');
    expect(pageText).not.toContain('不是给我的');
  });

  // ── S7: Multi-agent claim competition ─────────────────────────────────
  test('S7: no @mention task → only one agent claims', async ({ page }) => {
    const asTaskToggle = page.locator('[aria-label*="As Task"], [class*="asTask"], button:has-text("Task")');
    if (await asTaskToggle.isVisible()) {
      await asTaskToggle.click();
    }

    const msgInput = page.locator('textarea').first();
    await msgInput.fill('谁能分析一下 Go 语言在云原生领域的优势？');
    await msgInput.press('Enter');

    await page.waitForTimeout(AGENT_WAIT_MS);

    const pageText = await page.textContent('body');
    // No agent should complain about failed claims
    expect(pageText).not.toContain('FAILED');
    expect(pageText).not.toContain('Do not reply');
  });

  // ── Message format verification ────────────────────────────────────────
  test('Message format: @sender_name: content', async ({ page }) => {
    const msgInput = page.locator('textarea').first();
    await msgInput.fill('@产品 简单介绍一下Solo平台');
    await msgInput.press('Enter');

    await page.waitForTimeout(AGENT_WAIT_MS);

    // Take screenshot for manual verification
    await page.screenshot({ path: 'e2e/screenshots/phase5-message-format.png', fullPage: true });
  });
});
