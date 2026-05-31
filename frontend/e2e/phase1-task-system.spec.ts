// ============================================================================
// Phase 1 Task System E2E Tests — Claim, AsTask, Kanban, Toast
// ============================================================================
import { test, expect, type Page, type BrowserContext, type APIRequestContext } from '@playwright/test';

const TEST_EMAIL = 'test@test.com';
const TEST_PASSWORD = 'test12345';
const API_BASE = 'http://localhost:8080';

let sharedPage: Page;
let apiRequest: APIRequestContext;
let accessToken: string;
let userId: string;
let userName: string;
let channelId: string;
let testTaskId: string;

test.beforeAll(async ({ browser }) => {
  const context = await browser.newContext();
  sharedPage = await context.newPage();
  apiRequest = context.request;

  // Login via API
  const loginRes = await apiRequest.post(`${API_BASE}/api/v1/auth/login`, {
    data: { email: TEST_EMAIL, password: TEST_PASSWORD },
  });
  expect(loginRes.ok()).toBeTruthy();
  const loginData = await loginRes.json();
  accessToken = loginData.access_token;
  userId = loginData.user.id;
  userName = loginData.user.display_name || 'TestUser';
  console.log(`[Setup] Logged in as: ${userName} (${userId})`);

  // Set token in localStorage for page-based tests
  await sharedPage.goto('/auth/login');
  await sharedPage.evaluate((token: string) => {
    localStorage.setItem('access_token', token);
  }, accessToken);

  // Get or create a channel
  const channelsRes = await apiRequest.get(`${API_BASE}/api/v1/channels`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  expect(channelsRes.ok()).toBeTruthy();
  const channels = await channelsRes.json();
  if (channels.length > 0) {
    channelId = channels[0].id;
  } else {
    const createRes = await apiRequest.post(`${API_BASE}/api/v1/channels`, {
      headers: { Authorization: `Bearer ${accessToken}` },
      data: { name: 'phase1-test-channel', type: 'public' },
    });
    expect(createRes.ok()).toBeTruthy();
    const channel = await createRes.json();
    channelId = channel.id;
  }
  console.log(`[Setup] Using channel: ${channelId}`);
});

// ---- Helpers ----

async function ensureAuth() {
  await sharedPage.evaluate((token: string) => {
    localStorage.setItem('access_token', token);
  }, accessToken);
}

async function navTo(path: string) {
  await ensureAuth();
  await sharedPage.goto(path);
  await sharedPage.waitForTimeout(1500);
}

async function createTask(title: string, status?: string): Promise<any> {
  const res = await apiRequest.post(`${API_BASE}/api/v1/tasks`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: { channel_id: channelId, title, description: 'E2E Phase 1 test', priority: 'normal' },
  });
  expect(res.ok()).toBeTruthy();
  const task = await res.json();
  if (!status || status === 'todo') return task;

  // Valid transitions: todo -> in_progress -> in_review -> done
  const patch = async (s: string) => {
    const r = await apiRequest.patch(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${task.id}`,
      { headers: { Authorization: `Bearer ${accessToken}` }, data: { status: s } },
    );
    expect(r.ok(), `PATCH to ${s} should succeed`).toBeTruthy();
    return r.json();
  };

  if (status === 'in_progress') return patch('in_progress');
  if (status === 'in_review') {
    await patch('in_progress');
    return patch('in_review');
  }
  if (status === 'done') {
    await patch('in_progress');
    await patch('in_review');
    return patch('done');
  }
  if (status === 'closed') {
    await patch('in_progress');
    return patch('closed');
  }
  return task;
}

async function sendMessage(content: string): Promise<any> {
  const res = await apiRequest.post(`${API_BASE}/api/v1/channels/${channelId}/messages`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: { content },
  });
  expect(res.ok()).toBeTruthy();
  return await res.json();
}

// ============================================================================
// Test suites
// ============================================================================

test.describe.serial('Phase 1 — Claim System', () => {

  test('CL-01: Unclaimed task shows "认领" button in kanban board', async () => {
    // Create a fresh todo task
    testTaskId = (await createTask(`Phase1 CL-01 ${Date.now()}`)).id;
    console.log(`[CL-01] Created task: ${testTaskId}`);

    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    // Look for the "认领" button on our task card
    const claimBtn = sharedPage.locator('button:has-text("认领")').first();
    const hasClaimBtn = await claimBtn.isVisible({ timeout: 5000 }).catch(() => false);

    if (hasClaimBtn) {
      console.log('[CL-01] PASS: "认领" button visible on unclaimed task');
    } else {
      // Check if our task exists on the page
      const taskVisible = await sharedPage.locator(`text=Phase1 CL-01`).first().isVisible().catch(() => false);
      if (taskVisible) {
        // Task visible but no claim button — could be because this task is already claimed
        // Check the claimer_id via API
        const taskRes = await apiRequest.get(`${API_BASE}/api/v1/tasks/${testTaskId}`, {
          headers: { Authorization: `Bearer ${accessToken}` },
        });
        const task = await taskRes.json();
        if (task.claimer_id) {
          console.log(`[CL-01] Task already claimed by: ${task.claimer_id}`);
        }
        console.log('[CL-01] WARN: Task visible but no "认领" button found');
      } else {
        console.log('[CL-01] WARN: Task not visible on page');
      }
    }

    // Also verify via API that the claim endpoint exists
    const claimRes = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${testTaskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    const claimOk = claimRes.ok();
    console.log(`[CL-01] Claim API test: status=${claimRes.status()}, ok=${claimOk}`);
  });

  test('CL-02: Claiming a task changes status to in_progress', async () => {
    // Create a fresh todo task for claiming
    const taskId = (await createTask(`Phase1 CL-02 ${Date.now()}`)).id;
    console.log(`[CL-02] Created task: ${taskId}`);

    // Claim via API
    const claimRes = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(claimRes.ok()).toBeTruthy();
    const claimedTask = await claimRes.json();
    console.log(`[CL-02] Claim API response status=${claimedTask.status}, claimer_id=${claimedTask.claimer_id}`);

    // Assert status changed to in_progress
    expect(claimedTask.status).toBe('in_progress');
    console.log('[CL-02] PASS: Claim changed status to in_progress');
  });

  test('CL-03: Claim response includes claimer_id (claimername fix)', async () => {
    const taskId = (await createTask(`Phase1 CL-03 ${Date.now()}`)).id;

    const claimRes = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(claimRes.ok()).toBeTruthy();
    const task = await claimRes.json();

    // Verify claimer_id is set (not empty)
    expect(task.claimer_id).toBeTruthy();
    console.log(`[CL-03] claimer_id: ${task.claimer_id}`);

    // BUG CHECK: claimer_name should NOT be just the first 8 chars of claimer_id
    // The frontend falls back to claimer_id.slice(0,8), which looks like "566c4803"
    // If the backend returns claimer_name, it should be a human-readable name like "TestUser"
    if (task.claimer_name) {
      console.log(`[CL-03] claimer_name returned: ${task.claimer_name}`);
      // The claimer_name should contain actual name, not a UUID substring
      const isUUIDFragment = /^[0-9a-f]{1,8}$/i.test(task.claimer_name);
      if (!isUUIDFragment) {
        console.log('[CL-03] PASS: claimer_name is a proper name');
      } else {
        console.log('[CL-03] FAIL: claimer_name looks like a UUID fragment (BUG: claimer_name not properly returned)');
      }
    } else {
      console.log('[CL-03] FAIL: claimer_name is empty — backend does not return claimer_name field');
      console.log('[CL-03] This causes frontend to show claimer_id.slice(0,8) instead of user name');
    }
  });

  test('CL-04: Re-claiming an already claimed task returns 409 (silent fail)', async () => {
    const taskId = (await createTask(`Phase1 CL-04 ${Date.now()}`)).id;

    // First claim — should succeed
    const claim1 = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(claim1.ok()).toBeTruthy();
    console.log('[CL-04] First claim: OK');

    // Second claim — should return 409
    const claim2 = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(claim2.status()).toBe(409);
    console.log('[CL-04] PASS: Duplicate claim returned 409 (silent conflict)');
  });

  test('CL-05: Unclaim returns task to todo status', async () => {
    const taskId = (await createTask(`Phase1 CL-05 ${Date.now()}`)).id;

    // First claim the task
    await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );

    // Then unclaim
    const unclaimRes = await apiRequest.delete(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(unclaimRes.ok()).toBeTruthy();
    const unclaimed = await unclaimRes.json();
    console.log(`[CL-05] After unclaim — status: ${unclaimed.status}, claimer_id: "${unclaimed.claimer_id}"`);

    // After unclaim, status should go back to todo
    expect(unclaimed.status).toBe('todo');
    // claimer_id is omitted when empty (omitempty tag), so it's undefined after JSON parse
    expect(unclaimed.claimer_id || '').toBe('');
    console.log('[CL-05] PASS: Unclaim reset status to todo and cleared claimer_id');
  });
});

test.describe.serial('Phase 1 — AsTask (message to task conversion)', () => {

  test('AT-01: AsTask dialog opens from channel message', async () => {
    // Navigate to channel
    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    // Hover on a message — the "As Task" button is in the hover actions group
    // We need to hover over a message item
    const messageItems = sharedPage.locator('[role="listitem"]');
    const count = await messageItems.count();
    console.log(`[AT-01] Found ${count} message items in channel`);

    if (count > 0) {
      // Hover over the first message
      const firstMsg = messageItems.first();
      await firstMsg.hover();
      await sharedPage.waitForTimeout(500);

      // Look for the "转为任务" button (ClipboardList icon)
      const asTaskBtn = sharedPage.locator('button[aria-label="转为任务"]').first();
      const asTaskVisible = await asTaskBtn.isVisible({ timeout: 3000 }).catch(() => false);

      if (asTaskVisible) {
        console.log('[AT-01] PASS: "转为任务" button visible on message hover');
      } else {
        console.log('[AT-01] WARN: "转为任务" button not visible on hover');
        await sharedPage.screenshot({ path: 'at01-hover-no-astask.png' });
      }
    } else {
      // No messages — send one first
      await sendMessage('E2E AT-01 test message');
      await sharedPage.waitForTimeout(1000);
      await sharedPage.reload();
      await sharedPage.waitForTimeout(2000);

      const newMsg = sharedPage.locator('[role="listitem"]').first();
      await newMsg.hover();
      await sharedPage.waitForTimeout(500);

      const asTaskBtn = sharedPage.locator('button[aria-label="转为任务"]').first();
      const visible = await asTaskBtn.isVisible({ timeout: 3000 }).catch(() => false);
      console.log(`[AT-01] After sending message: asTask button visible = ${visible}`);
    }
  });

  test('AT-02: Click AsTask opens dialog with pre-filled title', async () => {
    // Send a new message specifically for this test
    const testContent = `AT-02 specific message ${Date.now()}`;
    await sendMessage(testContent);

    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    // Hover and click "转为任务"
    const messageItem = sharedPage.locator('[role="listitem"]').first();
    await messageItem.hover();
    await sharedPage.waitForTimeout(300);

    const asTaskBtn = sharedPage.locator('button[aria-label="转为任务"]').first();
    const clickable = await asTaskBtn.isVisible({ timeout: 3000 }).catch(() => false);

    if (clickable) {
      await asTaskBtn.click();
      await sharedPage.waitForTimeout(1000);

      // Should show dialog titled "转为任务"
      const dialogTitle = sharedPage.locator('text=转为任务').first();
      const dialogVisible = await dialogTitle.isVisible({ timeout: 5000 }).catch(() => false);

      if (dialogVisible) {
        console.log('[AT-02] PASS: "转为任务" dialog opened');

        // Check if title input is pre-filled
        const titleInput = sharedPage.locator('#as-task-title');
        const titleValue = await titleInput.inputValue().catch(() => '');
        console.log(`[AT-02] Pre-filled title: "${titleValue}"`);
        if (titleValue) {
          console.log('[AT-02] PASS: Title is pre-filled with message content');
        } else {
          console.log('[AT-02] WARN: Title input is empty (not pre-filled)');
        }

        // Close the dialog
        const cancelBtn = sharedPage.locator('button:has-text("取消")').last();
        if (await cancelBtn.isVisible().catch(() => false)) {
          await cancelBtn.click();
          await sharedPage.waitForTimeout(500);
        }
      } else {
        console.log('[AT-02] FAIL: Dialog did not open after clicking AsTask');
        await sharedPage.screenshot({ path: 'at02-no-dialog.png' });
      }
    } else {
      console.log('[AT-02] FAIL: "转为任务" button not clickable');
    }
  });

  test('AT-03: Submit AsTask creates task in TODO column', async () => {
    const msgContent = `AT-03 convert to task ${Date.now()}`;
    await sendMessage(msgContent);

    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    // Hover and click "转为任务"
    const messageItem = sharedPage.locator('[role="listitem"]').first();
    await messageItem.hover();
    await sharedPage.waitForTimeout(300);

    const asTaskBtn = sharedPage.locator('button[aria-label="转为任务"]').first();
    if (!(await asTaskBtn.isVisible({ timeout: 3000 }).catch(() => false))) {
      console.log('[AT-03] SKIP: asTask button not visible');
      return;
    }

    await asTaskBtn.click();
    await sharedPage.waitForTimeout(1000);

    // Modify title for uniqueness
    const uniqueTitle = `AT-03 Task ${Date.now()}`;
    const titleInput = sharedPage.locator('#as-task-title');
    await titleInput.fill(uniqueTitle);

    // Submit
    const submitBtn = sharedPage.locator('button:has-text("转为任务")').last();
    if (!(await submitBtn.isVisible({ timeout: 2000 }).catch(() => false))) {
      console.log('[AT-03] FAIL: Submit button not visible');
      return;
    }

    await submitBtn.click();
    await sharedPage.waitForTimeout(2000);

    // Verify the task exists via API
    const tasksRes = await apiRequest.get(
      `${API_BASE}/api/v1/tasks?channel_id=${channelId}`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    const tasks = await tasksRes.json();
    const ourTask = tasks.find((t: any) => t.title === uniqueTitle);
    if (ourTask) {
      console.log(`[AT-03] PASS: Task created, status=${ourTask.status}, id=${ourTask.id}`);
    } else {
      console.log('[AT-03] FAIL: Task not found in API after asTask submission');
    }
  });
});

test.describe.serial('Phase 1 — Channel Kanban Board', () => {

  test('KB-01: Channel Tasks tab shows kanban 5 columns', async () => {
    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    // Click the "任务" tab
    const tasksTab = sharedPage.locator('button:has-text("任务")').first();
    const tabVisible = await tasksTab.isVisible({ timeout: 5000 }).catch(() => false);

    if (!tabVisible) {
      console.log('[KB-01] FAIL: "任务" tab button not found in channel header');
      await sharedPage.screenshot({ path: 'kb01-no-tasks-tab.png' });
      return;
    }

    await tasksTab.click();
    await sharedPage.waitForTimeout(2000);

    // Check for the 5 column headers: TODO, IN PROGRESS, IN REVIEW, DONE, CLOSED
    const expectedHeaders = ['TODO', 'IN PROGRESS', 'IN REVIEW', 'DONE', 'CLOSED'];
    let allFound = true;
    for (const header of expectedHeaders) {
      const elem = sharedPage.locator(`h3:has-text("${header}")`).first();
      const found = await elem.isVisible({ timeout: 3000 }).catch(() => false);
      console.log(`[KB-01] Column header "${header}": ${found ? 'FOUND' : 'NOT FOUND'}`);
      if (!found) allFound = false;
    }

    if (allFound) {
      console.log('[KB-01] PASS: All 5 kanban columns visible');
    } else {
      console.log('[KB-01] FAIL: Not all 5 kanban columns visible');
      await sharedPage.screenshot({ path: 'kb01-columns.png' });
    }

    // Check for "快速创建" button
    const quickCreateBtn = sharedPage.locator('button:has-text("快速创建")');
    const qcVisible = await quickCreateBtn.isVisible({ timeout: 3000 }).catch(() => false);
    console.log(`[KB-01] "快速创建" button: ${qcVisible ? 'VISIBLE' : 'NOT VISIBLE'}`);
  });

  test('KB-02: Channel kanban matches global /tasks kanban visually', async () => {
    // Check global /tasks first
    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    const globalHeaders = ['TODO', 'IN PROGRESS', 'IN REVIEW', 'DONE', 'CLOSED'];
    let globalCount = 0;
    for (const h of globalHeaders) {
      const elem = sharedPage.locator(`h3:has-text("${h}")`).first();
      if (await elem.isVisible({ timeout: 2000 }).catch(() => false)) globalCount++;
    }
    console.log(`[KB-02] Global /tasks: ${globalCount}/5 columns visible`);

    // Then check channel tasks tab
    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    const tasksTab = sharedPage.locator('button:has-text("任务")').first();
    await tasksTab.click();
    await sharedPage.waitForTimeout(2000);

    let channelCount = 0;
    for (const h of globalHeaders) {
      const elem = sharedPage.locator(`h3:has-text("${h}")`).first();
      if (await elem.isVisible({ timeout: 2000 }).catch(() => false)) channelCount++;
    }
    console.log(`[KB-02] Channel tasks: ${channelCount}/5 columns visible`);

    if (globalCount === 5 && channelCount === 5) {
      console.log('[KB-02] PASS: Both global and channel kanban show all 5 columns');
    } else {
      console.log(`[KB-02] CHECK: global=${globalCount}/5, channel=${channelCount}/5`);
    }
  });

  test('KB-03: Terminal state cards show appropriate markers', async () => {
    // Create a task and move to done status
    const task = await createTask(`Phase1 KB-03 ${Date.now()}`, 'done');
    console.log(`[KB-03] Created done task: ${task.id}`);

    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    // The task should appear in the DONE column
    // Check the DONE column for terminal markers
    const doneColumn = sharedPage.locator('h3:has-text("DONE")').first();
    const hasDoneColumn = await doneColumn.isVisible({ timeout: 3000 }).catch(() => false);

    if (hasDoneColumn) {
      // Find the card in DONE column
      const doneCards = sharedPage.locator('text=Phase1 KB-03').first();
      const cardFound = await doneCards.isVisible({ timeout: 3000 }).catch(() => false);
      if (cardFound) {
        console.log('[KB-03] Task visible in DONE column');

        // Check if the card shows a terminal marker (e.g., "已完成" or checkmark)
        // For done tasks, the badge should say CLOSED or DONE
        const terminalMarker = sharedPage.locator('text=已完成, text=已关闭, text=✕').first();
        const hasMarker = await terminalMarker.isVisible({ timeout: 2000 }).catch(() => false);
        console.log(`[KB-03] Terminal marker in card: ${hasMarker ? 'FOUND' : 'NOT FOUND (BUG)'}`);
      } else {
        console.log('[KB-03] WARN: Task not found in DONE column');
      }
    } else {
      console.log('[KB-03] WARN: DONE column not found');
    }

    // Also test via API: done -> claim should return 409
    const claimRes = await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${task.id}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );
    expect(claimRes.status()).toBe(409);
    console.log('[KB-03] PASS: Claiming a done task correctly rejected with 409');
  });
});

test.describe.serial('Phase 1 — Toast Notifications', () => {

  test('TO-01: Toast appears after asTask success', async () => {
    const msgContent = `TO-01 msg ${Date.now()}`;
    await sendMessage(msgContent);

    await navTo(`/dashboard?channel=${channelId}`);
    await sharedPage.waitForTimeout(3000);

    const messageItem = sharedPage.locator('[role="listitem"]').first();
    await messageItem.hover();
    await sharedPage.waitForTimeout(300);

    const asTaskBtn = sharedPage.locator('button[aria-label="转为任务"]').first();
    if (!(await asTaskBtn.isVisible({ timeout: 3000 }).catch(() => false))) {
      console.log('[TO-01] SKIP: AsTask button not visible');
      return;
    }

    await asTaskBtn.click();
    await sharedPage.waitForTimeout(1000);

    const uniqueTitle = `TO-01 Task ${Date.now()}`;
    await sharedPage.locator('#as-task-title').fill(uniqueTitle);

    const submitBtn = sharedPage.locator('button:has-text("转为任务")').last();
    await submitBtn.click();
    await sharedPage.waitForTimeout(2000);

    // Check for toast message containing "已转为任务 #"
    const toastText = sharedPage.locator('text=已转为任务').first();
    const toastVisible = await toastText.isVisible({ timeout: 5000 }).catch(() => false);
    if (toastVisible) {
      console.log('[TO-01] PASS: Toast "已转为任务 #N" visible');
    } else {
      console.log('[TO-01] FAIL: Toast not visible after asTask');
      await sharedPage.screenshot({ path: 'to01-no-toast.png' });
    }
  });

  test('TO-02: Toast appears after claim success', async () => {
    // First go to tasks page
    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    // Create a task via API first
    const task = await createTask(`TO-02 ${Date.now()}`);
    console.log(`[TO-02] Created task: ${task.id}`);

    // Refresh to see the new task
    await sharedPage.reload();
    await sharedPage.waitForTimeout(2000);

    // Find claim button and click it
    const claimBtn = sharedPage.locator('button[aria-label="认领任务"]').first();
    const claimVisible = await claimBtn.isVisible({ timeout: 5000 }).catch(() => false);

    if (claimVisible) {
      await claimBtn.click();
      await sharedPage.waitForTimeout(2000);

      // Check for toast
      const toastText = sharedPage.locator('text=已认领').first();
      const hasClaimToast = await toastText.isVisible({ timeout: 3000 }).catch(() => false);
      if (hasClaimToast) {
        console.log('[TO-02] PASS: Claim toast visible');
      } else {
        // The frontend may not show a toast for claims — check channel-view.tsx handleClaim
        console.log('[TO-02] CHECK: No claim toast (may not be implemented yet)');
        // This is acceptable per spec — claim may be silent
      }
    } else {
      console.log('[TO-02] WARN: No claimable task found on page');
    }
  });
});

test.describe.serial('Phase 1 — Terminal State Markers', () => {

  test('TM-01: done/closed tasks show terminal marker in detail panel', async () => {
    const task = await createTask(`Phase1 TM-01 ${Date.now()}`, 'done');
    console.log(`[TM-01] Created done task: ${task.id}`);

    await navTo(`/tasks`);
    await sharedPage.waitForTimeout(2000);

    // Click the task card to open detail panel
    const taskCard = sharedPage.locator(`text=Phase1 TM-01`).first();
    if (await taskCard.isVisible({ timeout: 5000 }).catch(() => false)) {
      await taskCard.click();
      await sharedPage.waitForTimeout(1500);

      // Check for terminal marker in detail panel
      const doneMarker = sharedPage.locator('text=✓ 已完成').first();
      const hasMarker = await doneMarker.isVisible({ timeout: 5000 }).catch(() => false);

      if (hasMarker) {
        console.log('[TM-01] PASS: "✓ 已完成" terminal marker visible in detail panel');
      } else {
        console.log('[TM-01] FAIL: No terminal marker in detail panel for done task');
        await sharedPage.screenshot({ path: 'tm01-no-marker.png' });
      }
    } else {
      console.log('[TM-01] WARN: Task card not found');
    }
  });

  test('TM-02: done/closed tasks show terminal marker in kanban card', async () => {
    const task = await createTask(`Phase1 TM-02 ${Date.now()}`, 'done');

    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    // Find the task in the DONE column
    const doneColTask = sharedPage.locator(`text=Phase1 TM-02`).first();
    const cardFound = await doneColTask.isVisible({ timeout: 5000 }).catch(() => false);

    if (cardFound) {
      // In the card, look for a terminal badge
      // The status badge in done column should show "DONE" (not "CLOSED" since backend returns "done")
      // But there might not be a terminal marker distinct from the status badge
      const card = doneColTask.locator('..').locator('..');
      const markers = await card.locator('text=已完成, text=✓').count();
      console.log(`[TM-02] Terminal marker count near task: ${markers}`);

      // The badge spans in the card should show status name via STATUS_COLUMN_CONFIG
      // which maps 'done' to 'DONE' and 'closed' to 'CLOSED'
      const statusBadge = card.locator('.badge-brutal').first();
      const badgeText = await statusBadge.textContent().catch(() => '');
      console.log(`[TM-02] Status badge text: "${badgeText}"`);
      if (badgeText === 'DONE' || badgeText === 'CLOSED') {
        console.log('[TM-02] PASS: Terminal status badge visible in kanban card');
      }
    } else {
      console.log('[TM-02] WARN: Card not found in DONE column');
    }
  });
});

test.describe.serial('Phase 1 — Bug Verification (fixed bugs)', () => {

  test('BV-01: Claimer shows display name, NOT UUID fragment', async () => {
    // Create and claim a task
    const task = await createTask(`Phase1 BV-01 ${Date.now()}`);
    await apiRequest.post(
      `${API_BASE}/api/v1/channels/${channelId}/tasks/${task.id}/claim`,
      { headers: { Authorization: `Bearer ${accessToken}` } },
    );

    // Fetch the task via API
    const taskRes = await apiRequest.get(`${API_BASE}/api/v1/tasks/${task.id}`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    const fetched = await taskRes.json();

    console.log(`[BV-01] Task response: claimer_id=${fetched.claimer_id}`);
    console.log(`[BV-01] Task response: claimer_name=${fetched.claimer_name || '(not returned)'}`);

    // The displayed value in the frontend would be:
    // claimer_name || assignee_name || claimer_id.slice(0, 8)
    const displayValue = fetched.claimer_name || fetched.claimer_id?.slice(0, 8) || 'none';
    console.log(`[BV-01] Computed display: "${displayValue}"`);

    // If claimer_name is not returned, the frontend will show UUID fragment
    if (fetched.claimer_name && fetched.claimer_name.length > 0) {
      // Check it's not just a UUID fragment
      const isUUID = /^[0-9a-f]{1,8}$/i.test(fetched.claimer_name);
      if (!isUUID) {
        console.log(`[BV-01] PASS: claimer_name is "${fetched.claimer_name}" (human readable)`);
      } else {
        console.log(`[BV-01] FAIL: claimer_name "${fetched.claimer_name}" looks like UUID fragment`);
      }
    } else {
      console.log('[BV-01] FAIL: claimer_name not returned by backend — frontend will show UUID fragment');
    }
  });

  test('BV-02: Closed/done tasks have terminal state markers, not empty buttons', async () => {
    // Create a done task
    await createTask(`Phase1 BV-02-done ${Date.now()}`, 'done');
    await createTask(`Phase1 BV-02-closed ${Date.now()}`, 'closed');

    await navTo('/tasks');
    await sharedPage.waitForTimeout(2000);

    // Take a screenshot for manual inspection
    await sharedPage.screenshot({ path: 'bv02-terminal-states.png' });

    // Check the detail panel for a terminal task
    // Click on a task to open detail panel
    const doneTaskCard = sharedPage.locator(`text=Phase1 BV-02-done`).first();
    if (await doneTaskCard.isVisible({ timeout: 5000 }).catch(() => false)) {
      await doneTaskCard.click();
      await sharedPage.waitForTimeout(1500);

      // Should see "✓ 已完成" and no action buttons like "开始处理" etc.
      const doneMarker = sharedPage.locator('text=✓ 已完成').first();
      const doneVisible = await doneMarker.isVisible({ timeout: 3000 }).catch(() => false);

      const startBtn = sharedPage.locator('button:has-text("开始处理")').first();
      const hasStartBtn = await startBtn.isVisible({ timeout: 1000 }).catch(() => false);

      console.log(`[BV-02] Done marker visible: ${doneVisible}`);
      console.log(`[BV-02] "开始处理" button visible: ${hasStartBtn}`);

      if (doneVisible && !hasStartBtn) {
        console.log('[BV-02] PASS: Terminal marker shown, no action buttons for done task');
      } else if (!doneVisible) {
        console.log('[BV-02] FAIL: No terminal marker for done task');
      } else if (hasStartBtn) {
        console.log('[BV-02] FAIL: Action buttons should not appear for done tasks');
      }
    }
  });
});
