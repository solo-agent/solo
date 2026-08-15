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
  method: 'get' | 'post' | 'patch' | 'delete',
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

      computerID = databaseJSON<string>(`
        SELECT to_json(COALESCE((SELECT id::text FROM computers WHERE daemon_id='daemon-01' AND status='online' ORDER BY updated_at DESC LIMIT 1),''))::text
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
      await page.getByRole('button', { name: '创建频道' }).click();
      await page.getByRole('button', { name: '空白频道' }).click();
      await page.getByLabel('名称').fill(channelName);
      await page.getByRole('button', { name: '创建', exact: true }).click();
      await expect(page).toHaveURL(/\/dashboard\?channel=([0-9a-f-]{36})/);
      const channelID = new URL(page.url()).searchParams.get('channel');
      expect(channelID).toMatch(/^[0-9a-f-]{36}$/);
      channel = { id: channelID!, name: channelName };
      await expect(page.getByRole('heading', { name: channelName })).toBeVisible();

      // Verify the simplified Agent dialog fits a laptop screen and keeps advanced fields folded.
      await page.getByRole('button', { name: '创建第一个智能体' }).click();
      const agentDialog = page.getByRole('dialog').last();
      await expect(agentDialog.getByText('高级运行时设置')).toBeVisible();
      await expect(agentDialog.getByLabel('系统提示词')).toHaveCount(0);
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

      // Bind the Channel to one existing folder through Channel management.
      await page.getByRole('button', { name: '群管理' }).click();
      await page.getByLabel('文件夹所在电脑').selectOption(computerID);
      await page.getByLabel('文件夹完整路径').fill(projectDir);
      await page.getByRole('button', { name: '保存项目文件夹' }).click();
      await expect(page.getByText('项目文件夹已保存。')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.getByTitle(projectDir)).toBeVisible();

      expect(databaseJSON<{ computer: string; path: string }>(`
        SELECT json_build_object(
          'computer', COALESCE(project_computer_id::text,''),
          'path', COALESCE(project_path,'')
        )::text FROM channels WHERE id='${channel.id}'
      `)).toEqual({ computer: computerID, path: projectDir });

      const runtimes = await call<Runtime[]>(request, auth, 'get', `/api/v1/agent-backends/detect?computer_id=${computerID}`);
      const runtime = ['codex', 'claude', 'opencode', 'hermes', 'openclaw']
        .map((type) => runtimes.find((item) => item.type === type && item.available))
        .find(Boolean);
      expect(runtime, `No real local Agent runtime is available: ${JSON.stringify(runtimes)}`).toBeTruthy();

      agent = await call<Entity>(request, auth, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `FolderAgent${suffix}`,
        description: 'Project folder E2E',
        computer_id: computerID,
        model_provider: runtime!.type,
        model_name: runtime!.type === 'claude' ? 'sonnet' : '',
        system_prompt: 'Follow the user request using the real current working directory. Always deliver a visible final result with solo message send.',
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
      const agentWorkspace = join(homedir(), '.solo', 'agents', agent.id, 'workspace');
      expect(existsSync(agentWorkspace)).toBe(true);
      expect(agentWorkspace).not.toBe(projectDir);
    } finally {
      if (auth && agent) await call(request, auth, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (auth && channel) await call(request, auth, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
      if (auth && computerID) databaseExec(`DELETE FROM computer_members WHERE computer_id='${computerID}' AND user_id='${auth.user.id}';`);
      if (auth) databaseExec(`DELETE FROM sessions WHERE user_id='${auth.user.id}'; UPDATE users SET is_active=false WHERE id='${auth.user.id}';`);
      rmSync(projectDir, { recursive: true, force: true });
    }
  });
});
