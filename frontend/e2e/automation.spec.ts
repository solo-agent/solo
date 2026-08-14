import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const suffix = Date.now().toString(36);
const credentials = { email: `automation-${suffix}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse { access_token: string; refresh_token: string }
interface Entity { id: string; name: string }
interface AutomationState {
  id: string;
  next_run_at: string;
  target_agent_id: string;
  completion_policy: string;
  run_count: number;
  task_count: number;
}
interface RunState {
  id: string;
  source: string;
  status: string;
  task_id: string;
  task_number: number;
  task_status: string;
  agent_runs: number;
  agent_run_id: string;
  agent_status: string;
  replies: number;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function databaseExec(query: string) {
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-v', 'ON_ERROR_STOP=1', '-c', query,
  ], { encoding: 'utf8' });
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Automation E2E' },
  });
  if (!response.ok()) throw new Error(`register: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    if (!localStorage.getItem('solo.locale')) localStorage.setItem('solo.locale', 'zh-CN');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

async function api<T>(request: APIRequestContext, token: string, method: 'post' | 'patch' | 'delete', path: string, data?: unknown): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: { authorization: `Bearer ${token}` }, data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

function automationState(channelID: string, name: string): AutomationState {
  return databaseJSON<AutomationState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'id', a.id::text,
        'next_run_at', COALESCE(a.next_run_at::text, ''),
        'target_agent_id', COALESCE(a.target_agent_id::text, ''),
        'completion_policy', a.completion_policy,
        'run_count', (SELECT count(*) FROM automation_runs r WHERE r.automation_id=a.id),
        'task_count', (SELECT count(*) FROM automation_runs r WHERE r.automation_id=a.id AND r.task_id IS NOT NULL)
      )::text FROM automations a WHERE a.channel_id='${channelID}' AND a.name='${name}' LIMIT 1
    ), '{"id":"","next_run_at":"","target_agent_id":"","run_count":0,"task_count":0}')
  `);
}

function latestRun(automationID: string, source?: 'manual' | 'scheduled'): RunState {
  const sourceFilter = source ? `AND r.source='${source}'` : '';
  return databaseJSON<RunState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'id', r.id::text,
        'source', r.source,
        'status', r.status,
        'task_id', COALESCE(r.task_id::text, ''),
        'task_number', COALESCE(t.task_number, 0),
        'task_status', COALESCE(t.status, ''),
        'agent_runs', (SELECT count(*) FROM agent_run_task_links l WHERE l.task_id=r.task_id),
        'agent_run_id', COALESCE((
          SELECT ar.id::text FROM agent_run_task_links l JOIN agent_runs ar ON ar.id=l.run_id
          WHERE l.task_id=r.task_id ORDER BY ar.started_at DESC LIMIT 1
        ), ''),
        'agent_status', COALESCE((
          SELECT ar.status FROM agent_run_task_links l JOIN agent_runs ar ON ar.id=l.run_id
          WHERE l.task_id=r.task_id ORDER BY ar.started_at DESC LIMIT 1
        ), ''),
        'replies', (
          SELECT count(*) FROM messages reply
          WHERE reply.thread_id=(SELECT th.id FROM threads th WHERE th.root_message_id=t.message_id LIMIT 1)
        )
      )::text
      FROM automation_runs r LEFT JOIN tasks t ON t.id=r.task_id
      WHERE r.automation_id='${automationID}' AND r.task_id IS NOT NULL ${sourceFilter}
      ORDER BY r.created_at DESC LIMIT 1
    ), '{"id":"","source":"","status":"","task_id":"","task_number":0,"task_status":"","agent_runs":0,"agent_run_id":"","agent_status":"","replies":0}')
  `);
}

function daemonReportedTokens(runID: string): number {
  const lines = readFileSync('../daemon.log', 'utf8').trim().split('\n');
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    try {
      const entry = JSON.parse(lines[index]) as Record<string, unknown>;
      if (entry.msg === 'task backend completed' && entry.task_id === runID) {
        return Number(entry.input_tokens ?? 0) + Number(entry.output_tokens ?? 0)
          + Number(entry.cache_read_tokens ?? 0) + Number(entry.cache_write_tokens ?? 0);
      }
    } catch { /* ignore non-JSON process output */ }
  }
  return 0;
}

function agentGreetingFinished(agentID: string): boolean {
  return databaseJSON<boolean>(`
    SELECT EXISTS(
      SELECT 1 FROM agent_runs WHERE agent_id='${agentID}' AND status='completed'
    )::text
  `);
}

test.describe('automation workspace navigation', () => {
  test('keeps the Channel automation page visible across navigation, reload, and locale changes', async ({ page, request }, testInfo) => {
    const navigationCredentials = {
      email: `automation-navigation-${suffix}@solo.local`,
      password: 'SoloE2E-2026!',
    };
    const registration = await registerVerified(request, apiBase, {
      data: { ...navigationCredentials, display_name: 'Automation Navigation E2E' },
    });
    if (!registration.ok()) throw new Error(`register: ${registration.status()} ${await registration.text()}`);
    const auth = await registration.json() as AuthResponse;
    await authenticatePage(page, auth);

    let channel: Entity | null = null;
    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `automation-navigation-${suffix}`, description: 'Real automation navigation E2E',
      });

      await page.goto(`/dashboard?channel=${channel.id}`);
      await page.getByRole('button', { name: '自动任务' }).click();
      await expect(page).toHaveURL(/view=automation/);
      const workspace = page.getByTestId('automation-workspace');
      await expect(workspace.getByRole('heading', { name: '自动任务' })).toBeVisible();
      await expect(workspace.getByText('还没有自动任务', { exact: true })).toBeVisible();

      await page.reload();
      await expect(page).toHaveURL(/view=automation/);
      await expect(workspace.getByRole('heading', { name: '自动任务' })).toBeVisible();
      await workspace.getByRole('button', { name: '新建自动任务' }).click();
      await expect(workspace.getByLabel('名称')).toBeVisible();
      await expect(workspace.getByLabel('任务标题')).toBeVisible();
      await workspace.getByRole('button', { name: '取消' }).click();

      await page.getByRole('button', { name: '任务看板' }).click();
      await expect(page).toHaveURL(/view=task/);
      await page.goBack();
      await expect(page).toHaveURL(/view=automation/);
      await expect(workspace.getByRole('heading', { name: '自动任务' })).toBeVisible();

      await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
      await page.reload();
      await expect(page.getByRole('button', { name: 'Automations' })).toHaveAttribute('aria-pressed', 'true');
      await expect(workspace.getByRole('heading', { name: 'Automations' })).toBeVisible();
      await expect(workspace.getByRole('button', { name: 'New automation' })).toBeVisible();

      const screenshotPath = testInfo.outputPath('automation-workspace-en.png');
      await page.locator('#channel-workspace-panel').screenshot({ path: screenshotPath });
      await testInfo.attach('automation-workspace-en', { path: screenshotPath, contentType: 'image/png' });
    } finally {
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });
});

test.describe('real Channel automations', () => {
  test.skip(process.env.SOLO_E2E_REAL_AUTOMATION !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(600_000);
  let localComputer: LocalComputerLease;
  let auth: AuthResponse;
  let createdChannel: Entity | null = null;
  let createdAgent: Entity | null = null;

  test.beforeAll(async ({ request }) => {
    auth = await authenticate(request);
    localComputer = await acquireLocalComputer(request, apiBase, auth.access_token);
  });

  test.afterAll(async ({ request }) => {
    if (createdAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${createdAgent.id}`).catch(() => undefined);
    if (createdChannel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${createdChannel.id}`).catch(() => undefined);
    await localComputer?.release(request).catch(() => undefined);
  });

  test('creates from the real UI, avoids duplicate work, runs the Agent, and resumes a missed schedule once', async ({ page, request }, testInfo) => {
    const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
    if (!login.ok()) throw new Error(`login: ${login.status()} ${await login.text()}`);
    const auth = await login.json() as AuthResponse;
    await authenticatePage(page, auth);

    const automationName = `每日检查 ${suffix}`;
    const taskTitle = `AUTO_E2E_${suffix}`;
    const reply = `AUTO_DONE_${suffix.toUpperCase()}`;
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `automation-${suffix}`, description: 'Real automation E2E',
      });
      createdChannel = channel;
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `AutoAgent${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude', model_name: 'sonnet',
        system_prompt: [
          'Always copy the latest incoming target= field verbatim into solo message send --target.',
          'When introducing yourself, send exactly AUTO_READY.',
          `For every assigned task whose title contains ${taskTitle}, send exactly ${reply}.`,
          'Send one visible message and no explanation.',
        ].join(' '),
      });
      createdAgent = agent;
      await expect.poll(() => agentGreetingFinished(agent!.id), {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toBe(true);

      await page.goto(`/dashboard?channel=${channel.id}`);
      await page.getByRole('button', { name: '自动任务' }).click();
      await expect(page).toHaveURL(/view=automation/);
      const workspace = page.getByTestId('automation-workspace');
      await expect(workspace.getByRole('heading', { name: '自动任务' })).toBeVisible();
      await workspace.getByRole('button', { name: '新建自动任务' }).click();
      await workspace.getByLabel('名称').fill(automationName);
      await workspace.getByLabel('任务标题').fill(taskTitle);
      await workspace.getByLabel('任务说明').fill('检查一次并把结果回复到任务讨论中。');
      await workspace.getByRole('button', { name: '目标智能体' }).click();
      await page.getByRole('option', { name: agent.name }).click();
      await workspace.getByRole('button', { name: '小时' }).click();
      const hour23 = page.getByRole('option', { name: '23', exact: true });
      await expect(hour23).toBeVisible();
      const timePickerPath = testInfo.outputPath('automation-time-picker-zh.png');
      await page.screenshot({ path: timePickerPath });
      await testInfo.attach('automation-time-picker-zh', { path: timePickerPath, contentType: 'image/png' });
      await hour23.click({ timeout: 10_000 });
      await workspace.getByRole('button', { name: '分钟' }).click();
      await page.getByRole('option', { name: '59', exact: true }).click({ timeout: 10_000 });
      await workspace.getByRole('button', { name: '时区' }).click();
      await page.getByRole('option', { name: 'Asia/Shanghai', exact: true }).click();
      await workspace.getByRole('button', { name: '创建自动任务' }).click();

      await expect(workspace.getByText(automationName, { exact: true })).toBeVisible();
      const stored = automationState(channel.id, automationName);
      expect(stored.id).toMatch(/^[0-9a-f-]{36}$/);
      expect(stored.target_agent_id).toBe(agent.id);
      expect(stored.next_run_at).not.toBe('');
      const screenshot = await workspace.screenshot();
      await testInfo.attach('automation-list-zh', { body: screenshot, contentType: 'image/png' });

      const card = workspace.locator(`[data-automation-id="${stored.id}"]`);
      await card.getByRole('button', { name: '立即运行' }).click();
      await expect(page.getByText('已创建新任务并交给智能体。')).toBeVisible();

      await expect.poll(() => latestRun(stored.id, 'manual'), {
        timeout: 30_000, intervals: [250, 500, 1000],
      }).toMatchObject({ source: 'manual', status: 'running', task_id: expect.stringMatching(/^[0-9a-f-]{36}$/) });
      const manual = latestRun(stored.id, 'manual');

      const duplicate = await request.post(`${apiBase}/api/v1/channels/${channel.id}/automations/${stored.id}/run`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
      });
      expect(duplicate.status()).toBe(409);
      expect(await duplicate.json()).toMatchObject({ code: 'automation_already_running' });

      await card.getByRole('button', { name: '运行历史' }).click();
      await expect(workspace.getByText('已有运行', { exact: true })).toBeVisible();
      await expect(workspace.getByText('上一轮任务还没有结束', { exact: true })).toBeVisible();
      await workspace.getByRole('button', { name: '返回' }).click();
      await page.getByRole('button', { name: '任务看板' }).click();
      await expect(page.getByText(taskTitle, { exact: true }).first()).toBeVisible();

      await expect.poll(() => {
        const state = latestRun(stored.id, 'manual');
        return {
          task_id: state.task_id,
          automation_finished: state.status === 'completed',
          agent_finished: state.agent_runs >= 1 && state.agent_status === 'completed',
          task_completed: state.task_status === 'done',
          replies: state.replies,
        };
      }, {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        task_id: manual.task_id,
        automation_finished: true,
        agent_finished: true,
        task_completed: true,
        replies: 1,
      });
      expect(daemonReportedTokens(latestRun(stored.id, 'manual').agent_run_id)).toBeGreaterThan(0);
      await page.getByRole('button', { name: new RegExp(taskTitle) }).first().click();
      await expect(page.getByText(reply, { exact: true })).toBeVisible();

      databaseExec(`UPDATE automations SET next_run_at=now()-interval '30 days' WHERE id='${stored.id}'`);
      await expect.poll(() => {
        const state = latestRun(stored.id, 'scheduled');
        return {
          source: state.source, status: state.status, task_id: state.task_id,
          agent_started: state.agent_runs >= 1,
        };
      }, {
        timeout: 60_000, intervals: [500, 1000, 2000],
      }).toMatchObject({ source: 'scheduled', status: 'running', task_id: expect.stringMatching(/^[0-9a-f-]{36}$/), agent_started: true });
      const scheduled = latestRun(stored.id, 'scheduled');
      expect(scheduled.task_id).not.toBe(manual.task_id);
      const afterSchedule = automationState(channel.id, automationName);
      expect(afterSchedule.run_count).toBe(3);
      expect(afterSchedule.task_count).toBe(2);
      expect(new Date(afterSchedule.next_run_at).getTime()).toBeGreaterThan(Date.now());

      await expect.poll(() => {
        const state = latestRun(stored.id, 'scheduled');
        return {
          automation_finished: state.status === 'completed',
          agent_finished: state.agent_runs >= 1 && state.agent_status === 'completed',
          task_completed: state.task_status === 'done',
          replies: state.replies,
        };
      }, {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({ automation_finished: true, agent_finished: true, task_completed: true, replies: 1 });
      expect(daemonReportedTokens(latestRun(stored.id, 'scheduled').agent_run_id)).toBeGreaterThan(0);

      await api(request, auth.access_token, 'patch', `/api/v1/channels/${channel.id}/automations/${stored.id}`, {
        name: automationName,
        task_title: taskTitle,
        task_description: '检查一次并把结果回复到任务讨论中。',
        target_agent_id: agent.id,
        schedule_type: 'daily',
        schedule_hour: 23,
        schedule_minute: 59,
        schedule_weekday: null,
        timezone: 'Asia/Shanghai',
        completion_policy: 'review_required',
        enabled: true,
      });
      expect(automationState(channel.id, automationName).completion_policy).toBe('review_required');
      await api(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/automations/${stored.id}/run`);
      await expect.poll(() => {
        const state = latestRun(stored.id, 'manual');
        return {
          distinct_task: state.task_id !== manual.task_id && state.task_id !== scheduled.task_id,
          automation_finished: state.status === 'completed',
          agent_finished: state.agent_runs >= 1 && state.agent_status === 'completed',
          task_waits_for_review: state.task_status === 'in_review',
          replies: state.replies,
        };
      }, {
        timeout: 180_000, intervals: [500, 1000, 2000],
      }).toMatchObject({ distinct_task: true, automation_finished: true, agent_finished: true, task_waits_for_review: true, replies: 1 });
      expect(daemonReportedTokens(latestRun(stored.id, 'manual').agent_run_id)).toBeGreaterThan(0);

      const databaseStatePath = testInfo.outputPath('automation-database-final-state.json');
      writeFileSync(databaseStatePath, JSON.stringify({
          automation: automationState(channel.id, automationName),
          manual: latestRun(stored.id, 'manual'),
          scheduled: latestRun(stored.id, 'scheduled'),
        }, null, 2));
      await testInfo.attach('automation-database-final-state', {
        path: databaseStatePath,
        contentType: 'application/json',
      });

      await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
      await page.reload();
      await page.getByRole('button', { name: 'Automations' }).click();
      const englishWorkspace = page.getByTestId('automation-workspace');
      await expect(englishWorkspace.getByText('Every day · 23:59 · Asia/Shanghai')).toBeVisible();
      await expect(englishWorkspace.getByRole('button', { name: 'Run now' })).toBeVisible();
      const finalWorkspacePath = testInfo.outputPath('automation-final-workspace-en.png');
      await englishWorkspace.screenshot({ path: finalWorkspacePath });
      await testInfo.attach('automation-final-workspace-en', {
        path: finalWorkspacePath,
        contentType: 'image/png',
      });
    } finally {
      if (agent) {
        const removed = await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).then(() => true).catch(() => false);
        if (removed) createdAgent = null;
      }
      if (channel) {
        const removed = await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).then(() => true).catch(() => false);
        if (removed) createdChannel = null;
      }
    }
  });
});
