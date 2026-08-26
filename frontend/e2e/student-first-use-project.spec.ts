// spec: e2e/specs/student-first-use.plan.md
// seed: e2e/student-first-use-seed.spec.ts
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { join } from 'node:path';

import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';

type Auth = {
  access_token: string;
  refresh_token: string;
  user: { id: string };
  workspace_id: string;
};

type Entity = { id: string; name: string };
type Runtime = { type: string; available: boolean };
type ChannelProject = { can_manage: boolean };

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function databaseExec(query: string): void {
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-v', 'ON_ERROR_STOP=1', '-c', query,
  ], { stdio: 'ignore' });
}

async function call<T>(
  request: APIRequestContext,
  auth: Auth,
  method: 'get' | 'post' | 'put' | 'patch' | 'delete',
  path: string,
  data?: unknown,
): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: {
      authorization: `Bearer ${auth.access_token}`,
      'x-workspace-id': auth.workspace_id,
    },
    data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function authenticatePage(page: Page, auth: Auth): Promise<void> {
  await page.addInitScript((value) => {
    localStorage.setItem('access_token', value.accessToken);
    localStorage.setItem('refresh_token', value.refreshToken);
    localStorage.setItem('solo.locale', 'zh-CN');
    localStorage.setItem('solo_authenticated_user_id', value.userID);
    localStorage.setItem('solo_active_workspace_id', value.workspaceID);
    localStorage.setItem(`solo_active_workspace_id:${value.userID}`, value.workspaceID);
  }, {
    accessToken: auth.access_token,
    refreshToken: auth.refresh_token,
    userID: auth.user.id,
    workspaceID: auth.workspace_id,
  });
}

test.describe('A Channel owns a real project folder', () => {
  test.use({ locale: 'zh-CN', viewport: { width: 1280, height: 720 } });

  test('chinese-channel-modal-and-real-agent-use-the-bound-folder', async ({ page, request }) => {
    const suffix = `${Date.now()}-${process.pid}`;
    const email = `student-project-${suffix}@solo.local`;
    const password = 'SoloStudent-2026!';
    const channelName = `大学课程项目-${suffix}`;
    const marker = `SOLO_PROJECT_FOLDER_${suffix}`;
    const projectDir = mkdtempSync(join(tmpdir(), 'solo-student-project-'));
    let auth: Auth | null = null;
    let channel: Entity | null = null;
    let agent: Entity | null = null;
    let computerID = '';

    try {
      const verified = await registerVerified(request, apiBase, {
        data: { email, password, display_name: '大学新生' },
      });
      expect(verified.status()).toBe(201);
      auth = await verified.json() as Auth;

      const workspaceResponse = await request.post(`${apiBase}/api/v1/workspaces`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { name: `大学课程项目实验室-${suffix}` },
      });
      expect(workspaceResponse.status()).toBe(201);
      auth.workspace_id = ((await workspaceResponse.json()) as Entity).id;
      databaseExec(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}';`);

      computerID = databaseJSON<string>(`
        SELECT to_json(COALESCE((SELECT id::text FROM computers WHERE status='online' ORDER BY updated_at DESC LIMIT 1),''))::text
      `);
      expect(computerID).not.toBe('');
      databaseExec(`
        INSERT INTO computer_members (computer_id,user_id,role)
        VALUES ('${computerID}','${auth.user.id}','member')
        ON CONFLICT (computer_id,user_id) DO UPDATE SET role='member';
      `);

      await authenticatePage(page, auth);
      await page.goto('/dashboard');

      // Create a naturally named Channel through the real UI.
      await page.locator('button[aria-label="创建频道"]').click();
      await page.getByRole('button', { name: '空白频道 先创建空白频道，之后再添加新的智能体。' }).click();
      await page.getByLabel('名称').fill(channelName);
      await page.getByRole('button', { name: '创建', exact: true }).click();
      await expect(page).toHaveURL(/\/dashboard\?channel=([0-9a-f-]{36})/);
      const channelID = new URL(page.url()).searchParams.get('channel');
      expect(channelID).toMatch(/^[0-9a-f-]{36}$/);
      channel = { id: channelID!, name: channelName };
      await expect(page.getByRole('heading', { name: channelName })).toBeVisible();
      expect((await call<ChannelProject>(request, auth, 'get', `/api/v1/channels/${channel.id}/project`)).can_manage).toBe(true);

      // Verify the Agent dialog fits a laptop screen, exposes role instructions,
      // and keeps environment variables and CLI arguments folded.
      await page.getByRole('button', { name: '创建第一个智能体' }).click();
      const agentDialog = page.getByRole('dialog').last();
      await expect(agentDialog.getByText('高级运行时设置')).toBeVisible();
      await expect(agentDialog.getByLabel('系统提示词')).toBeVisible();
      const createAgentButton = agentDialog.getByRole('button', { name: '创建智能体' });
      await createAgentButton.scrollIntoViewIfNeeded();
      await expect(createAgentButton).toBeVisible();
      const dialogBox = await agentDialog.boundingBox();
      const submitBox = await createAgentButton.boundingBox();
      expect(dialogBox).not.toBeNull();
      expect(submitBox).not.toBeNull();
      expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
      expect(submitBox!.y + submitBox!.height).toBeLessThanOrEqual(720);
      await page.keyboard.press('Escape');

      // Configure the shared project identity and this member's private folder
      // through the ordinary Channel header entry.
      const projectResponse = page.waitForResponse((response) => response.url().endsWith(`/api/v1/channels/${channel!.id}/project`));
      await page.getByRole('button', { name: '项目', exact: true }).click();
      const loadedProject = await projectResponse;
      expect(loadedProject.status()).toBe(200);
      expect(((await loadedProject.json()) as ChannelProject).can_manage).toBe(true);
      const projectDialog = page.getByRole('dialog', { name: `项目 · ${channelName}` });
      const projectSource = page.getByLabel('代码来源（可选）');
      await expect(projectSource).toBeVisible({ timeout: 10_000 });
      await projectSource.fill('https://example.invalid/course-project.git');
      await page.getByLabel('团队约定版本（可选）').fill('main');
      await page.getByRole('button', { name: '保存项目信息' }).click();
      await expect(page.getByText('项目信息已保存。')).toBeVisible();
      await page.getByLabel('文件夹所在电脑').selectOption(computerID);
      await page.getByLabel('文件夹完整路径').fill(projectDir);
      await page.getByLabel('我的版本').fill('main');
      await page.getByRole('button', { name: '保存项目文件夹' }).click();
      await expect(page.getByText('项目文件夹已保存。')).toBeVisible();
      await expect(page.getByText('已准备好')).toBeVisible();
      await projectDialog.getByRole('button', { name: '关闭' }).click();
      await expect(projectDialog).toBeHidden();

      expect(databaseJSON<{ source: string; baseline: string; computer: string; path: string; version: string }>(`
        SELECT json_build_object(
          'source', c.project_source,
          'baseline', c.project_baseline,
          'computer', m.computer_id::text,
          'path', m.local_path,
          'version', m.version
        )::text
        FROM channels c
        JOIN channel_project_mappings m ON m.channel_id=c.id AND m.user_id='${auth.user.id}'
        WHERE c.id='${channel.id}'
      `)).toEqual({
        source: 'https://example.invalid/course-project.git',
        baseline: 'main',
        computer: computerID,
        path: projectDir,
        version: 'main',
      });

      const runtimes = await call<Runtime[]>(request, auth, 'get', `/api/v1/agent-backends/detect?computer_id=${computerID}`);
      const runtime = ['claude', 'codex', 'opencode', 'hermes', 'openclaw']
        .map((type) => runtimes.find((item) => item.type === type && item.available))
        .find(Boolean);
      expect(runtime, `No real local Agent runtime is available: ${JSON.stringify(runtimes)}`).toBeTruthy();

      agent = await call<Entity>(request, auth, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `FolderAgent${suffix}`,
        description: 'Project folder E2E',
        computer_id: computerID,
        model_provider: runtime!.type,
        model_name: runtime!.type === 'claude' ? 'sonnet' : '',
        system_prompt: `Follow the user request using the real current working directory. Always deliver the final result with solo message send --target '#${channelName}'.`,
      });

      await expect.poll(() => databaseJSON<string>(`
        SELECT to_json(COALESCE((SELECT status FROM agent_runs WHERE agent_id='${agent!.id}' ORDER BY started_at DESC LIMIT 1),''))::text
      `), { timeout: 180000, intervals: [500, 1000, 2000] }).toBe('completed');

      const trigger = await call<{ id: string }>(request, auth, 'post', `/api/v1/channels/${channel.id}/messages`, {
        content: `Create project-proof.txt in the current working directory with exactly this text: ${marker}. Then send a visible reply that includes ${marker}.`,
      });

      await expect.poll(() => databaseJSON<{ status: string; reply: string }>(`
        SELECT json_build_object(
          'status', COALESCE((SELECT status FROM agent_runs WHERE agent_id='${agent!.id}' AND trigger_message_id='${trigger.id}' ORDER BY started_at DESC LIMIT 1),''),
          'reply', COALESCE((SELECT m.content FROM messages m JOIN agent_runs r ON m.metadata->>'agent_run_id'=r.id::text WHERE r.agent_id='${agent!.id}' AND r.trigger_message_id='${trigger.id}' ORDER BY m.created_at DESC LIMIT 1),'')
        )::text
      `), { timeout: 180000, intervals: [500, 1000, 2000] }).toMatchObject({
        status: 'completed',
        reply: expect.stringContaining(marker),
      });

      await expect.poll(() => existsSync(join(projectDir, 'project-proof.txt')), { timeout: 30000 }).toBe(true);
      expect(readFileSync(join(projectDir, 'project-proof.txt'), 'utf8').trim()).toBe(marker);
      expect(databaseJSON<{ computer: string; path: string; version: string }>(`
        SELECT json_build_object(
          'computer', COALESCE(project_computer_id::text,''),
          'path', COALESCE(project_path,''),
          'version', project_version
        )::text
        FROM agent_runs
        WHERE agent_id='${agent.id}' AND trigger_message_id='${trigger.id}'
        ORDER BY started_at DESC LIMIT 1
      `)).toEqual({ computer: computerID, path: projectDir, version: 'main' });
      const agentWorkspace = join(homedir(), '.solo', 'agents', agent.id, 'workspace');
      expect(existsSync(agentWorkspace)).toBe(true);
      expect(agentWorkspace).not.toBe(projectDir);

      // A running Agent protects the current project from being switched.
      // Once that work is finished, the same Channel can switch projects and
      // shows the change in the conversation.
      const completedRunID = databaseJSON<string>(`
        SELECT to_json(id::text)::text FROM agent_runs
        WHERE agent_id='${agent.id}' AND trigger_message_id='${trigger.id}'
        ORDER BY started_at DESC LIMIT 1
      `);
      databaseExec(`UPDATE agent_runs SET status='running' WHERE id='${completedRunID}';`);
      const blockedSwitch = await request.patch(`${apiBase}/api/v1/channels/${channel.id}/project`, {
        headers: {
          authorization: `Bearer ${auth.access_token}`,
          'x-workspace-id': auth.workspace_id,
        },
        data: { source: 'https://example.invalid/blocked.git', baseline_version: 'main' },
      });
      expect(blockedSwitch.status()).toBe(409);
      expect((await blockedSwitch.json()).code).toBe('CHANNEL_PROJECT_BUSY');
      expect(databaseJSON<string>(`SELECT to_json(project_source)::text FROM channels WHERE id='${channel.id}'`))
        .toBe('https://example.invalid/course-project.git');
      databaseExec(`UPDATE agent_runs SET status='completed' WHERE id='${completedRunID}';`);

      const nextProjectSource = 'https://example.invalid/next-course-project.git';
      const fallbackMarker = `SOLO_PRIVATE_WORKSPACE_${suffix}`;
      const projectReload = page.waitForResponse((response) => response.url().endsWith(`/api/v1/channels/${channel!.id}/project`));
      await page.getByRole('button', { name: '项目', exact: true }).click();
      await projectReload;
      await projectSource.fill(nextProjectSource);
      await page.getByRole('button', { name: '保存项目信息' }).click();
      await expect(page.getByText('项目信息已保存。')).toBeVisible();
      await page.getByRole('button', { name: '取消使用文件夹' }).click();
      await expect(page.getByText('已取消项目文件夹。')).toBeVisible();
      await projectDialog.getByRole('button', { name: '关闭' }).click();
      await expect(projectDialog).toBeHidden();
      await expect(page.getByText(`大学新生 已将本频道切换到 ${nextProjectSource}`, { exact: false })).toBeVisible();
      expect(databaseJSON<string>(`SELECT to_json(project_source)::text FROM channels WHERE id='${channel.id}'`))
        .toBe(nextProjectSource);

      const fallbackTrigger = await call<{ id: string }>(request, auth, 'post', `/api/v1/channels/${channel.id}/messages`, {
        content: `Create private-fallback-proof.txt in the current working directory with exactly this text: ${fallbackMarker}. Then send a visible reply that includes both ${fallbackMarker} and ${nextProjectSource}.`,
      });
      await expect.poll(() => databaseJSON<{ status: string; reply: string }>(`
        SELECT json_build_object(
          'status', COALESCE((SELECT status FROM agent_runs WHERE agent_id='${agent!.id}' AND trigger_message_id='${fallbackTrigger.id}' ORDER BY started_at DESC LIMIT 1),''),
          'reply', COALESCE((SELECT m.content FROM messages m JOIN agent_runs r ON m.metadata->>'agent_run_id'=r.id::text WHERE r.agent_id='${agent!.id}' AND r.trigger_message_id='${fallbackTrigger.id}' ORDER BY m.created_at DESC LIMIT 1),'')
        )::text
      `), { timeout: 180000, intervals: [500, 1000, 2000] }).toMatchObject({
        status: 'completed',
        reply: expect.stringContaining(fallbackMarker),
      });
      expect(readFileSync(join(agentWorkspace, 'private-fallback-proof.txt'), 'utf8').trim()).toBe(fallbackMarker);
      expect(existsSync(join(projectDir, 'private-fallback-proof.txt'))).toBe(false);
      expect(databaseJSON<{ computer: string; path: string }>(`
        SELECT json_build_object(
          'computer', COALESCE(project_computer_id::text,''),
          'path', COALESCE(project_path,'')
        )::text
        FROM agent_runs
        WHERE agent_id='${agent.id}' AND trigger_message_id='${fallbackTrigger.id}'
        ORDER BY started_at DESC LIMIT 1
      `)).toEqual({ computer: '', path: '' });
    } finally {
      if (auth && agent) await call(request, auth, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (auth && channel) await call(request, auth, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
      if (auth && computerID) databaseExec(`DELETE FROM computer_members WHERE computer_id='${computerID}' AND user_id='${auth.user.id}';`);
      if (auth) databaseExec(`
        DELETE FROM channel_members cm USING channels c
         WHERE cm.channel_id=c.id AND c.workspace_id='00000000-0000-0000-0000-000000000001' AND cm.member_id='${auth.user.id}';
        DELETE FROM workspace_members WHERE workspace_id='00000000-0000-0000-0000-000000000001' AND user_id='${auth.user.id}';
        DELETE FROM sessions WHERE user_id='${auth.user.id}';
        UPDATE workspaces SET deleted_at=COALESCE(deleted_at,now()) WHERE created_by='${auth.user.id}' AND id<>'00000000-0000-0000-0000-000000000001';
        UPDATE users SET is_active=false WHERE id='${auth.user.id}';
      `);
      rmSync(projectDir, { recursive: true, force: true });
    }
  });
});
