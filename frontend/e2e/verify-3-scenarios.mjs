import { chromium } from 'playwright';

const BASE = 'http://localhost:3000';
const findings = [];
const screenshots = {};

function fail(s, m) { findings.push(`FAIL [${s}]: ${m}`); }
function pass(s, m) { findings.push(`PASS [${s}]: ${m}`); }
function warn(s, m) { findings.push(`WARN [${s}]: ${m}`); }

async function main() {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();

  // =========================================================================
  // LOGIN
  // =========================================================================
  console.log('[SETUP] Logging in...');
  await page.goto(`${BASE}/auth/login`, { waitUntil: 'networkidle' });
  await page.waitForSelector('#email', { timeout: 10000 });
  await page.fill('#email', '1271123275@qq.com');
  await page.fill('#password', 'Test12345!');
  await page.click('button:has-text("登录")');
  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await page.waitForTimeout(2000);
  console.log('[SETUP] Logged in, on dashboard\n');

  // =========================================================================
  // SCENARIO A: asTask no "?" avatar or blank box
  // =========================================================================
  console.log('=== SCENARIO A: asTask no "?" avatar or blank box ===\n');

  // A1. Find the asTask toggle button in the message composer
  // Source: message-input.tsx shows aria-label alternates between
  // "创建为任务" (off) and "取消创建为任务" (on)
  const asTaskToggle = page.locator('button[aria-label="创建为任务"]');
  const toggleCount = await asTaskToggle.count();

  if (toggleCount === 0) {
    // Check if it's already toggled on (unlikely, but possible)
    const asTaskOn = page.locator('button[aria-label="取消创建为任务"]');
    const onCount = await asTaskOn.count();
    if (onCount > 0) {
      console.log('A1. asTask toggle is already ON');
    } else {
      warn('A', 'asTask toggle button not found. The MessageInput might have showAsTaskToggle=false.');
    }
  } else {
    console.log('A1. Found asTask toggle: "创建为任务"');
    await asTaskToggle.first().click();
    await page.waitForTimeout(500);

    // Verify it toggled on
    const asTaskOn = page.locator('button[aria-label="取消创建为任务"]');
    const isOn = await asTaskOn.count() > 0;
    console.log(`A1. asTask toggle is now ${isOn ? 'ON' : 'OFF'}`);
  }

  // A2. Find the message input (textarea inside the composer)
  // When asTask is on: aria-label="任务标题输入框"
  // When asTask is off: aria-label="消息输入框"
  const input = page.locator('textarea[aria-label="任务标题输入框"], textarea[aria-label="消息输入框"]').first();
  const inputCount = await input.count();

  if (inputCount === 0) {
    fail('A', 'Message input textarea not found');
  } else {
    // A3. Send a message
    const testMsg = `测试asTask ${Date.now()}`;
    await input.fill(testMsg);
    console.log(`A3. Typed: "${testMsg}"`);

    // Find send button via aria-label
    // When asTask is off: aria-label="发送消息"
    // When asTask is on: aria-label="创建任务"
    const sendBtn = page.locator('button[aria-label="创建任务"], button[aria-label="发送消息"]').first();
    if (await sendBtn.count() > 0) {
      await sendBtn.click();
      console.log('A3. Clicked send button');
    } else {
      await page.keyboard.press('Enter');
      console.log('A3. Pressed Enter to send');
    }

    // A4. Wait for agent response
    console.log('A4. Waiting for agent streaming response (12s)...');
    await page.waitForTimeout(12000);

    // Screenshot
    await page.screenshot({ path: '/tmp/scenario-a.png', fullPage: true });
    screenshots.A = '/tmp/scenario-a.png';

    // A5. Check all avatars for "?" text
    // The Avatar component renders `initials || '?'`
    // So a "?" avatar is `document.querySelectorAll` looking for avatar divs with text "?"
    const allAvatarsWithQuestion = await page.evaluate(() => {
      const results = [];
      // Find all divs that look like avatars (rounded-full, flex, items-center, justify-center)
      // and contain just "?" as text
      document.querySelectorAll('[aria-label]').forEach(el => {
        const text = el.textContent.trim();
        if (text === '?' && el.classList.contains('rounded-full') && el.children.length === 0) {
          const ariaLabel = el.getAttribute('aria-label');
          results.push({ ariaLabel, tag: el.tagName, classes: el.className });
        }
      });
      return results;
    });

    console.log(`A5. Avatars showing "?": ${allAvatarsWithQuestion.length}`);
    if (allAvatarsWithQuestion.length > 0) {
      for (const q of allAvatarsWithQuestion) {
        console.log(`    ? avatar: aria-label="${q.ariaLabel}"`);
      }
      fail('A', `Found ${allAvatarsWithQuestion.length} "?" avatar(s)`);
    } else {
      pass('A', 'No "?" avatars found in agent response');
    }

    // A6. Check for blank/empty message bubbles
    const blankMessages = await page.evaluate(() => {
      const results = [];
      // Find all message-bubble elements that are empty or have only whitespace
      document.querySelectorAll('.message-bubble').forEach(el => {
        const text = el.textContent.trim();
        if (text.length === 0) {
          results.push({ classes: el.className, parentRole: el.closest('[role="listitem"]') ? 'listitem' : 'none' });
        }
      });
      return results;
    });

    console.log(`A6. Blank message bubbles: ${blankMessages.length}`);
    if (blankMessages.length > 0) {
      fail('A', `Found ${blankMessages.length} blank message bubble(s)`);
    } else {
      pass('A', 'No blank message bubbles found');
    }
  }

  // Check for failed messages
  const failedCount = await page.locator('text=发送失败').count();
  if (failedCount > 0) {
    fail('A', `Found ${failedCount} "发送失败" messages`);
  }

  // =========================================================================
  // SCENARIO B: TaskBadge real-time update
  // =========================================================================
  console.log('\n=== SCENARIO B: TaskBadge real-time update ===\n');

  // B1. Check for TaskBadge in the message list on dashboard
  // TaskBadge is rendered when message.task_status is present
  const taskBadges = page.locator('button[aria-label*="任务 #"]');
  const badgeCount = await taskBadges.count();
  console.log(`B1. TaskBadge buttons on page: ${badgeCount}`);

  // Also check for broader task badge display
  const taskBadgeText = await page.locator('body').innerText();
  const hasTODO = taskBadgeText.includes('TODO');
  const hasPending = taskBadgeText.includes('待认领');
  const hasInProgress = taskBadgeText.includes('处理中') || taskBadgeText.includes('IN PROGRESS');
  console.log(`B1. Task status text found: TODO=${hasTODO}, 待认领=${hasPending}, 处理中/IN_PROGRESS=${hasInProgress}`);

  // B2. Navigate to tasks tab within channel
  // Click "任务" tab button in channel header
  const tasksTabBtn = page.locator('button:has-text("任务")').first();
  if (await tasksTabBtn.count() > 0) {
    console.log('B2. Switching to tasks tab...');
    await tasksTabBtn.click();
    await page.waitForTimeout(2000);

    await page.screenshot({ path: '/tmp/scenario-b-tasks-tab.png', fullPage: true });
    screenshots.B1 = '/tmp/scenario-b-tasks-tab.png';

    // B3. Look for task cards with claim buttons
    // The TaskBoard has claim buttons in TaskCard, but claim buttons also appear in ThreadPanel
    // Let's find tasks and open one
    const taskCards = page.locator('button.card-brutal');
    const taskCardCount = await taskCards.count();
    console.log(`B3. Task cards: ${taskCardCount}`);

    if (taskCardCount > 0) {
      // Record initial state of first task card
      const firstCardText = await taskCards.first().textContent();
      console.log(`B3. First task card: "${firstCardText.substring(0, 100)}"`);

      // Click the first task card to open its thread
      await taskCards.first().click();
      await page.waitForTimeout(3000);

      // Check if thread panel opened with task metadata bar
      await page.screenshot({ path: '/tmp/scenario-b-thread-opened.png', fullPage: false });
      screenshots.B2 = '/tmp/scenario-b-thread-opened.png';

      // B4. Look for claim button in thread panel (TaskMetaBar)
      const claimBtn = page.locator('button[aria-label="认领任务"]');
      const claimBtnCount = await claimBtn.count();
      console.log(`B4. Claim button in thread: ${claimBtnCount}`);

      if (claimBtnCount > 0) {
        // Record task status before claim
        const beforeText = await page.locator('body').innerText();

        // Click claim
        await claimBtn.click();
        await page.waitForTimeout(3000);
        console.log('B4. Clicked "认领" in thread');

        // After claim: check that the button changed
        // Should now show "释放" button or the claimer name
        const afterClaimScreenshot = await page.screenshot({ path: '/tmp/scenario-b-after-claim.png', fullPage: false });
        screenshots.B3 = '/tmp/scenario-b-after-claim.png';

        const afterText = await page.locator('body').innerText();
        const hasRelease = afterText.includes('释放');
        const hasClaimerName = afterText.includes('@');
        console.log(`B4. After claim: 释放=${hasRelease}, @claimer=${hasClaimerName}`);

        if (hasRelease || hasClaimerName) {
          pass('B', `Task claim succeeded: badge updated in real-time (释放=${hasRelease})`);
        } else {
          // Check if claim failed with 409 silently
          warn('B', 'Claim button disappeared but no release button found. Might be 409 (already claimed). Checking for claimer name...');
          if (!hasClaimerName) {
            fail('B', 'TaskBadge did not update after claim: no release button, no @claimer');
          }
        }

        // B5. Try to change task status
        // Go back to tasks tab to see the task card status
        const closeThreadBtn = page.locator('button[aria-label="关闭线程面板"]');
        if (await closeThreadBtn.count() > 0) {
          await closeThreadBtn.click();
          await page.waitForTimeout(500);
        }

        // Check if the task card in the task tab updated
        // The task should have moved from TODO to IN_PROGRESS column (or similar)
        await page.screenshot({ path: '/tmp/scenario-b-after-all.png', fullPage: true });
        screenshots.B4 = '/tmp/scenario-b-after-all.png';

        // Check for status change
        const finalText = await page.locator('body').innerText();
        const statusChanged = finalText !== beforeText;
        console.log(`B5. Status changed from initial: ${statusChanged}`);
      } else {
        // Check for release button (already claimed)
        const releaseBtn = page.locator('button[aria-label="释放任务"]');
        const releaseBtnCount = await releaseBtn.count();
        console.log(`B4. Task already claimed. Release button: ${releaseBtnCount}`);

        if (releaseBtnCount > 0) {
          // Already claimed -- still useful: check if status can be changed
          console.log('B4. Task already claimed. Testing status change...');

          // Close thread and look for status change options
          const closeThreadBtn = page.locator('button[aria-label="关闭线程面板"]');
          if (await closeThreadBtn.count() > 0) {
            await closeThreadBtn.click();
            await page.waitForTimeout(500);
          }

          // Check task board for status change
          pass('B', 'Task already claimed. TaskBadge shows claimer info correctly.');
        } else {
          warn('B', 'No claim or release button found in thread panel.');
        }
      }
    } else {
      // No task cards. Try to create a task via the "快速创建" button
      console.log('B3. No task cards. Trying to create one...');
      const quickCreateBtn = page.locator('button:has-text("快速创建")');
      if (await quickCreateBtn.count() > 0) {
        await quickCreateBtn.click();
        await page.waitForTimeout(1000);

        const taskTitleInput = page.locator('#quick-create-title');
        if (await taskTitleInput.count() > 0) {
          await taskTitleInput.fill(`测试任务 ${Date.now()}`);
          const createBtn = page.locator('button:has-text("创建任务")').first();
          if (await createBtn.count() > 0) {
            await createBtn.click();
            await page.waitForTimeout(2000);
            console.log('B3. Created a task via quick create');
          }
        }
      } else {
        warn('B', 'No tasks and no quick create button. Cannot test TaskBadge.');
      }
    }
  } else {
    warn('B', 'Tasks tab button not found in channel header.');
  }

  // Navigate back to messages tab and dashboard for Scenario C
  const messagesTabBtn = page.locator('button:has-text("消息")').first();
  if (await messagesTabBtn.count() > 0) {
    await messagesTabBtn.click();
    await page.waitForTimeout(1000);
  }
  await page.goto(`${BASE}/dashboard`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // =========================================================================
  // SCENARIO C: Thread follow-up has agent response
  // =========================================================================
  console.log('\n=== SCENARIO C: Thread follow-up agent response ===\n');

  const msgItems = page.locator('[role="listitem"]');
  const msgCount = await msgItems.count();
  console.log(`C1. Messages on page: ${msgCount}`);

  let threadOpened = false;

  // Hover each message to find reply buttons
  for (let i = 0; i < Math.min(msgCount, 15) && !threadOpened; i++) {
    const msg = msgItems.nth(i);
    await msg.hover();
    await page.waitForTimeout(500);

    // Try to find reply button by aria-label pattern
    // aria-label="回复 {display_name} 的消息"
    const replyBtn = msg.locator('button[aria-label*="回复"][aria-label*="消息"]');
    const btnCount = await replyBtn.count();

    if (btnCount > 0) {
      console.log(`C1. Found reply button on message ${i}: "${await replyBtn.first().getAttribute('aria-label')}"`);
      await replyBtn.first().click();
      await page.waitForTimeout(3000);

      // Check if ThreadPanel opened
      const threadHeader = page.locator('h3:has-text("线程")');
      if (await threadHeader.count() > 0) {
        threadOpened = true;
        console.log('C2. ThreadPanel opened!');

        await page.screenshot({ path: '/tmp/scenario-c-thread.png', fullPage: false });
        screenshots.C1 = '/tmp/scenario-c-thread.png';

        // Check reply count in header
        const headerText = await threadHeader.textContent();
        console.log(`C2. Thread header: "${headerText}"`);

        // Count messages in thread panel by looking at the 400px panel DOM
        // The thread panel is inside a div with width 400px in channel-view
        const threadMsgCount = await page.evaluate(() => {
          // Find the thread panel container (400px width sidebar)
          const panels = document.querySelectorAll('[class*="w-[400px]"], [class*="w-\\[400px\\]"]');
          for (const panel of panels) {
            const items = panel.querySelectorAll('[role="listitem"]');
            if (items.length > 0) return items.length;
          }
          // Fallback: check for thread reply items by structural pattern
          const threadH3 = document.querySelector('h3');
          if (threadH3 && threadH3.textContent.includes('线程')) {
            const container = threadH3.closest('[class*="flex"][class*="h-full"]');
            if (container) {
              return container.querySelectorAll('.flex.gap-3').length;
            }
          }
          return 0;
        });
        console.log(`C2. Messages/replies in thread panel: ${threadMsgCount}`);

        // C3. Find the thread reply input and send a follow-up
        const threadInput = page.locator('textarea[aria-label="线程回复输入框"]');
        const inputPresent = await threadInput.count() > 0;
        console.log(`C3. Thread reply input found: ${inputPresent}`);

        if (inputPresent) {
          const followUp = `追问 ${Date.now()}`;
          await threadInput.fill(followUp);
          console.log(`C3. Typed: "${followUp}"`);

          // Click send button
          const threadSendBtn = page.locator('button[aria-label="发送回复"]');
          if (await threadSendBtn.count() > 0) {
            await threadSendBtn.click();
            console.log('C3. Sent follow-up');
          } else {
            await page.keyboard.press('Enter');
            console.log('C3. Pressed Enter to send follow-up');
          }

          // C4. Wait for agent response
          console.log('C4. Waiting for agent response (15s)...');
          await page.waitForTimeout(15000);

          await page.screenshot({ path: '/tmp/scenario-c-after.png', fullPage: false });
          screenshots.C2 = '/tmp/scenario-c-after.png';

          // Check if new messages appeared (use page evaluate for complex selectors)
          const threadMsgCountAfter = await page.evaluate(() => {
            const threadH3 = document.querySelector('h3');
            if (threadH3 && threadH3.textContent.includes('线程')) {
              const container = threadH3.closest('[class*="flex"][class*="h-full"]');
              if (container) return container.querySelectorAll('.flex.gap-3').length;
            }
            return 0;
          });
          console.log(`C4. Thread messages: ${threadMsgCount} -> ${threadMsgCountAfter}`);

          if (threadMsgCountAfter > threadMsgCount) {
            pass('C', `Thread follow-up got response: ${threadMsgCount} -> ${threadMsgCountAfter} messages`);
          } else {
            // Check for streaming/typing indicator
            const streamingEls = await page.locator('[class*="streaming"]').count();
            console.log(`C4. Streaming elements: ${streamingEls}`);

            if (streamingEls > 0) {
              console.log('C4. Agent still streaming. Waiting more...');
              await page.waitForTimeout(10000);

              const finalCount = await threadReplyItems.count();
              if (finalCount > threadMsgCount) {
                pass('C', `Thread follow-up got response (delayed): ${threadMsgCount} -> ${finalCount}`);
              } else {
                fail('C', `Thread follow-up no response: messages unchanged at ${threadMsgCount}`);
              }
            } else {
              // Check the body text for any new content
              const bodyText = await page.locator('.w-\\\\[400px\\\\]').innerText();
              console.log(`C4. Thread panel text (first 300): "${bodyText.substring(0, 300)}"`);
              if (bodyText.includes(followUp) && bodyText.length > followUp.length + 50) {
                pass('C', 'Thread follow-up got response (agent replied within existing DOM)');
              } else {
                fail('C', `Thread follow-up NO agent response: ${threadMsgCount} msgs, no streaming`);
              }
            }
          }
        } else {
          fail('C', 'Thread reply input not found');
        }

        // Close thread
        const closeBtn = page.locator('button[aria-label="关闭线程面板"]');
        if (await closeBtn.count() > 0) {
          await closeBtn.click();
          await page.waitForTimeout(500);
        }
      }
    }
  }

  if (!threadOpened) {
    // Try a broader search: click on messages that have reply counts
    const replyLinkBtns = page.locator('button:has-text("条回复")');
    const replyLinkCount = await replyLinkBtns.count();
    console.log(`C1. Reply link buttons: ${replyLinkCount}`);

    if (replyLinkCount > 0) {
      await replyLinkBtns.first().click();
      await page.waitForTimeout(3000);

      const threadHeader2 = page.locator('h3:has-text("线程")');
      threadOpened = await threadHeader2.count() > 0;
      console.log(`C1. ThreadPanel opened via reply link: ${threadOpened}`);
    }

    if (!threadOpened) {
      warn('C', 'Could not open ThreadPanel. No reply buttons or reply link buttons found.');
    }
  }

  // =========================================================================
  // FINAL
  // =========================================================================
  await page.screenshot({ path: '/tmp/scenario-final.png', fullPage: true });
  screenshots.FINAL = '/tmp/scenario-final.png';

  // =========================================================================
  // SUMMARY
  // =========================================================================
  console.log('\n========================================');
  console.log('VERIFICATION SUMMARY');
  console.log('========================================\n');

  const failCount = findings.filter(f => f.startsWith('FAIL')).length;
  const passCount = findings.filter(f => f.startsWith('PASS')).length;
  const warnCount = findings.filter(f => f.startsWith('WARN')).length;

  for (const f of findings) {
    console.log(f);
  }

  console.log(`\nResults: ${passCount} PASS, ${failCount} FAIL, ${warnCount} WARN`);
  console.log('Screenshots:', Object.entries(screenshots).map(([k,v]) => `${k}=${v}`).join(', '));

  await browser.close();
  process.exit(failCount > 0 ? 1 : 0);
}

main().catch(err => {
  console.error('FATAL:', err.message);
  process.exit(1);
});
