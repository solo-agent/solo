import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { homedir, tmpdir } from 'node:os';
import { join } from 'node:path';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const repoRoot = join(process.cwd(), '..');
const soloBinary = process.env.SOLO_E2E_SOLO_BINARY ?? join(repoRoot, '.pids', 'solo');
const daemonBinary = process.env.SOLO_E2E_DAEMON_BINARY ?? join(repoRoot, '.pids', 'daemon');
const publicWorkspaceID = '00000000-0000-0000-0000-000000000001';

interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: { id: string; email: string; display_name: string };
  workspace_id?: string;
}

interface Workspace {
  id: string;
  name: string;
  role: 'owner' | 'admin' | 'member';
  is_default: boolean;
  is_personal: boolean;
}

interface Channel { id: string; name: string; workspace_id: string }
interface Computer { id: string; name: string; status: string; enrollment_token?: string }
interface Runtime { type: string; available: boolean }
interface Agent { id: string; name: string; owner_id: string; home_channel_id: string; system_prompt: string; computer_id?: string }
interface ChannelMember { member_id: string; agent_owner_id?: string; agent_home_channel_id?: string }
interface ChannelProject {
  baseline_version: string;
  mappings: Array<{ user_id: string; computer_id?: string; local_path?: string; available: boolean }>;
}
interface GuestToken { id: string; token: string; url: string; expires_at: string }
interface GuestInfo { workspace_id: string; channels: Channel[] }
interface LucyResponse { agent_id: string; channel_id: string }
interface Message { sender_id: string; content: string }
interface MessageList { messages: Message[]; has_more: boolean }

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

function deactivateTestUsers(...auths: AuthResponse[]) {
  const ids = auths.map((auth) => `'${auth.user.id}'`).join(',');
  if (!ids) return;
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c', `
      BEGIN;
      DELETE FROM channel_members cm USING channels c
       WHERE cm.channel_id=c.id
         AND c.workspace_id='${publicWorkspaceID}'
         AND cm.member_type='user' AND cm.member_id IN (${ids});
      DELETE FROM workspace_members
       WHERE workspace_id='${publicWorkspaceID}' AND user_id IN (${ids});
      DELETE FROM sessions WHERE user_id IN (${ids});
      UPDATE agent_sessions SET status='closed',last_active_at=now()
       WHERE agent_id IN (SELECT id FROM agents WHERE owner_id IN (${ids}));
      UPDATE agents SET is_active=false,updated_at=now()
       WHERE owner_id IN (${ids});
      DELETE FROM computers WHERE owner_id IN (${ids});
      UPDATE channels SET is_archived=true,updated_at=now()
       WHERE workspace_id IN (SELECT id FROM workspaces WHERE created_by IN (${ids}))
         AND workspace_id<>'${publicWorkspaceID}';
      UPDATE workspaces SET deleted_at=COALESCE(deleted_at,now()),updated_at=now()
       WHERE created_by IN (${ids}) AND id<>'${publicWorkspaceID}';
      UPDATE users SET is_active=false,updated_at=now() WHERE id IN (${ids});
      COMMIT;
    `,
  ], { encoding: 'utf8' });
}

function finishTestRuns(...auths: AuthResponse[]) {
  const ids = auths.map((auth) => `'${auth.user.id}'`).join(',');
  if (!ids) return;
  execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo', '-c', `
      UPDATE agent_runs SET status='failed',updated_at=now(),finished_at=now()
       WHERE finished_at IS NULL
         AND agent_id IN (SELECT id FROM agents WHERE owner_id IN (${ids}));
    `,
  ], { encoding: 'utf8' });
}

async function register(request: APIRequestContext, prefix: string): Promise<AuthResponse> {
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
  const response = await registerVerified(request, apiBase, {
    data: {
      email: `${prefix}-${suffix}@workspace-e2e.invalid`,
      password: 'SoloWorkspace-2026!',
      display_name: `${prefix} ${suffix.slice(-5)}`,
    },
  });
  if (!response.ok()) throw new Error(`register ${prefix}: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function call<T>(
  request: APIRequestContext,
  auth: AuthResponse,
  method: 'get' | 'post' | 'put' | 'patch' | 'delete',
  path: string,
  workspaceID?: string,
  data?: unknown,
): Promise<T> {
  const options = {
    headers: {
      authorization: `Bearer ${auth.access_token}`,
      ...(workspaceID ? { 'X-Workspace-ID': workspaceID } : {}),
    },
    ...(data === undefined ? {} : { data }),
  };
  const response = method === 'get'
    ? await request.get(`${apiBase}${path}`, options)
    : method === 'post'
      ? await request.post(`${apiBase}${path}`, options)
      : method === 'put'
        ? await request.put(`${apiBase}${path}`, options)
      : method === 'patch'
        ? await request.patch(`${apiBase}${path}`, options)
        : await request.delete(`${apiBase}${path}`, options);
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

function daemonProfileDir(profile: string): string {
  return join(homedir(), '.solo', 'daemons', profile);
}

function daemon(profile: string, args: string[]): string {
  return execFileSync(soloBinary, ['daemon', ...args, '--profile', profile], {
    cwd: repoRoot,
    env: { ...process.env, SOLO_DAEMON_BINARY: daemonBinary },
    encoding: 'utf8',
    timeout: 60000,
  });
}

function connectDaemon(profile: string, computer: Computer) {
  if (!computer.enrollment_token) throw new Error(`Computer ${computer.id} has no enrollment token`);
  daemon(profile, [
    'connect', '--server', apiBase, '--computer-id', computer.id,
    '--token', computer.enrollment_token,
  ]);
}

function automaticDaemon(home: string, args: string[]): string {
  return execFileSync(soloBinary, ['daemon', ...args], {
    cwd: repoRoot,
    env: { ...process.env, HOME: home, SOLO_DAEMON_BINARY: daemonBinary },
    encoding: 'utf8',
    timeout: 60000,
  });
}

async function availableRuntime(request: APIRequestContext, auth: AuthResponse, workspaceID: string, computerID: string): Promise<Runtime> {
  const runtimes = await call<Runtime[]>(request, auth, 'get', `/api/v1/agent-backends/detect?computer_id=${computerID}`, workspaceID);
  const runtime = ['claude', 'codex', 'opencode', 'hermes', 'openclaw']
    .map((type) => runtimes.find((item) => item.type === type && item.available))
    .find(Boolean);
  if (!runtime) throw new Error(`No real Agent runtime is available on ${computerID}: ${JSON.stringify(runtimes)}`);
  return runtime;
}

test.describe('Workspace and multi-Daemon product flow', () => {
  test.skip(process.env.SOLO_E2E_WORKSPACES !== '1', 'requires the make-managed frontend, API, PostgreSQL, and real local runtime');
  test.setTimeout(600000);

  test('keeps two page pairing commands isolated by computer ID', async ({ page, request }) => {
    const auth = await register(request, 'AutomaticDaemon');
    const suffix = Date.now().toString(36);
    const home = mkdtempSync(join(tmpdir(), 'solo-auto-daemon-'));
    const computers = await Promise.all([
      call<Computer>(request, auth, 'post', '/api/v1/computers', undefined, { name: `Automatic A ${suffix}` }),
      call<Computer>(request, auth, 'post', '/api/v1/computers', undefined, { name: `Automatic B ${suffix}` }),
    ]);

    try {
      for (const computer of computers) {
        if (!computer.enrollment_token) throw new Error(`Computer ${computer.id} has no enrollment token`);
        automaticDaemon(home, [
          'connect', '--server', apiBase, '--computer-id', computer.id,
          '--token', computer.enrollment_token, '--profile', computer.id,
        ]);
      }

      await expect.poll(() => databaseJSON<number>(`
        SELECT to_json(count(*))::text FROM computers
         WHERE id IN ('${computers[0].id}','${computers[1].id}') AND status='online'
      `), { intervals: [250, 500, 1000] }).toBe(2);

      const directories = computers.map((computer) => join(home, '.solo', 'daemons', computer.id));
      const ports = directories.map((directory) => Number(readFileSync(join(directory, 'port'), 'utf8').trim()));
      const pids = directories.map((directory) => Number(readFileSync(join(directory, 'daemon.pid'), 'utf8').trim()));
      expect(ports[0]).not.toBe(ports[1]);
      expect(pids[0]).not.toBe(pids[1]);

      databaseJSON(`WITH updated AS (UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}' RETURNING 1) SELECT to_json(EXISTS(SELECT 1 FROM updated))::text`);
      await authenticatePage(page, auth);
      await page.goto('/computers');
      for (const computer of computers) {
        await expect(page.getByText(computer.name, { exact: true })).toBeVisible();
      }
    } finally {
      for (const computer of computers) {
        try { automaticDaemon(home, ['stop', '--profile', computer.id]); } catch { /* primary assertion reports failure */ }
      }
      rmSync(home, { recursive: true, force: true });
      deactivateTestUsers(auth);
    }
  });

  test('switches isolated Workspaces and collaborates through two users and two Daemons', async ({ page, request, browser }) => {
    const userA = await register(request, 'WorkspaceA');
    const userB = await register(request, 'WorkspaceB');
    const outsider = await register(request, 'WorkspaceOutsider');
    const suffix = Date.now().toString(36);
    const alphaName = `Alpha ${suffix}`;
    const betaName = `Beta ${suffix}`;
    const alphaChannelName = `alpha-room-${suffix}`;
    const betaChannelName = `beta-room-${suffix}`;
    const profileA = `e2e-a-${suffix}`;
    const profileB = `e2e-b-${suffix}`;
    const markerA = `PROFILE_A_${suffix.toUpperCase()}`;
    const markerB = `PROFILE_B_${suffix.toUpperCase()}`;
    const projectMarkerA = `PROJECT_A_${suffix.toUpperCase()}`;
    const projectMarkerB = `PROJECT_B_${suffix.toUpperCase()}`;
    const projectDirA = mkdtempSync(join(tmpdir(), 'solo-project-a-'));
    const projectDirB = mkdtempSync(join(tmpdir(), 'solo-project-b-'));
    let alpha: Workspace | undefined;
    let beta: Workspace | undefined;
    let alphaProject: Channel | undefined;
    let agentA: Agent | undefined;
    let agentB: Agent | undefined;
    let computerA!: Computer;
    let runtimeA!: Runtime;
    let alphaLucy!: LucyResponse;

    try {
      for (const auth of [userA, userB, outsider]) {
        const workspaces = await call<Workspace[]>(request, auth, 'get', '/api/v1/workspaces');
        expect(workspaces).toContainEqual(expect.objectContaining({ id: publicWorkspaceID, is_default: true }));
        expect(workspaces.filter((workspace) => workspace.is_personal)).toEqual([]);
      }

      computerA = await call<Computer>(request, userA, 'post', '/api/v1/computers', undefined, { name: `A Computer ${suffix}` });
      connectDaemon(profileA, computerA);
      await expect.poll(() => databaseJSON<string>(`SELECT to_json(COALESCE((SELECT status FROM computers WHERE id='${computerA.id}'),''))::text`), { intervals: [250, 500, 1000] }).toBe('online');
      await authenticatePage(page, userA);
      await page.goto('/dashboard');
      await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
      await page.getByLabel('Workspace name').fill(alphaName);
      await page.getByRole('button', { name: 'Create', exact: true }).click();
      await expect(page.getByText(alphaName, { exact: true }).first()).toBeVisible();
      alpha = (await call<Workspace[]>(request, userA, 'get', '/api/v1/workspaces')).find((item) => item.name === alphaName);
      expect(alpha).toBeTruthy();
      runtimeA = await availableRuntime(request, userA, alpha!.id, computerA.id);
      const alphaLucyChannel = await call<Channel>(request, userA, 'get', '/api/v1/channels/lucy', alpha!.id);
      alphaLucy = await call<LucyResponse>(request, userA, 'post', '/api/v1/onboarding/create-lucy', alpha!.id, {
        runtime_type: runtimeA.type,
        computer_id: computerA.id,
        channel_id: alphaLucyChannel.id,
      });
      databaseJSON(`WITH updated AS (UPDATE users SET onboarding_completed_at=now() WHERE id='${userA.user.id}' RETURNING 1) SELECT to_json(EXISTS(SELECT 1 FROM updated))::text`);
      await page.reload();

      const ownerReinvite = await request.post(`${apiBase}/api/v1/workspaces/${alpha!.id}/members`, {
        headers: { authorization: `Bearer ${userA.access_token}` },
        data: { email: userA.user.email, role: 'admin' },
      });
      expect(ownerReinvite.status()).toBe(409);
      expect(await ownerReinvite.text()).toContain('already a Workspace member');
      expect((await call<Workspace[]>(request, userA, 'get', '/api/v1/workspaces')).find((item) => item.id === alpha!.id)?.role).toBe('owner');

      await call(request, userA, 'post', `/api/v1/workspaces/${alpha!.id}/members`, undefined, { email: userB.user.email, role: 'admin' });
      expect((await call<Workspace[]>(request, userB, 'get', '/api/v1/workspaces')).find((item) => item.id === alpha!.id)?.role).toBe('admin');
      const ownerDemotion = await request.patch(`${apiBase}/api/v1/workspaces/${alpha!.id}/members/${userA.user.id}`, {
        headers: { authorization: `Bearer ${userB.access_token}` },
        data: { role: 'member' },
      });
      expect(ownerDemotion.status()).toBe(403);
      const ownerRemoval = await request.delete(`${apiBase}/api/v1/workspaces/${alpha!.id}/members/${userA.user.id}`, {
        headers: { authorization: `Bearer ${userB.access_token}` },
      });
      expect(ownerRemoval.status()).toBe(400);
      expect((await call<Workspace[]>(request, userA, 'get', '/api/v1/workspaces')).find((item) => item.id === alpha!.id)?.role).toBe('owner');
      const alphaChannels = await call<Channel[]>(request, userA, 'get', '/api/v1/channels', alpha!.id);
      const alphaGeneral = alphaChannels.find((item) => item.name === 'general');
      expect(alphaGeneral).toBeTruthy();
      expect((await call<Channel>(request, userA, 'get', '/api/v1/channels/lucy', alpha!.id)).workspace_id).toBe(alpha!.id);
      alphaProject = await call<Channel>(request, userA, 'post', '/api/v1/channels', alpha!.id, { name: alphaChannelName });
      expect((await call<Channel[]>(request, userB, 'get', '/api/v1/channels', alpha!.id)).map((channel) => channel.name)).toContain(alphaChannelName);

      await page.getByRole('button', { name: 'New workspace' }).click();
      await page.getByLabel('Workspace name').fill(betaName);
      await page.getByRole('button', { name: 'Create', exact: true }).click();
      await expect(page.getByText(betaName, { exact: true }).first()).toBeVisible();
      beta = (await call<Workspace[]>(request, userA, 'get', '/api/v1/workspaces')).find((item) => item.name === betaName);
      expect(beta).toBeTruthy();
      expect((await call<Channel>(request, userA, 'get', '/api/v1/channels/lucy', beta!.id)).workspace_id).toBe(beta!.id);
      await call<Channel>(request, userA, 'post', '/api/v1/channels', beta!.id, { name: betaChannelName });

      const lucyIsolation = databaseJSON<{ alpha_lucy: string; beta_lucy: string; shared_computer: boolean; cross_session: number }>(`
        SELECT json_build_object(
          'alpha_lucy', COALESCE((SELECT a.id::text FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id='${alpha!.id}' AND a.kind='lucy' AND a.is_active LIMIT 1), ''),
          'beta_lucy', COALESCE((SELECT a.id::text FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id='${beta!.id}' AND a.kind='lucy' AND a.is_active LIMIT 1), ''),
          'shared_computer', COALESCE((SELECT count(DISTINCT a.runtime_id)=1 FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id IN ('${alpha!.id}','${beta!.id}') AND a.kind='lucy' AND a.is_active AND a.runtime_id IS NOT NULL), false),
          'cross_session', (SELECT count(*) FROM agent_sessions s JOIN agents a ON a.id=s.agent_id JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id NOT IN ('${alpha!.id}','${beta!.id}') AND a.id IN (SELECT a2.id FROM agents a2 JOIN channels c2 ON c2.id=a2.home_channel_id WHERE c2.workspace_id IN ('${alpha!.id}','${beta!.id}') AND a2.kind='lucy'))
        )::text
      `);
      expect(lucyIsolation.alpha_lucy).not.toBe('');
      expect(lucyIsolation.beta_lucy).not.toBe('');
      expect(lucyIsolation.alpha_lucy).toBe(alphaLucy.agent_id);
      expect(lucyIsolation.alpha_lucy).not.toBe(lucyIsolation.beta_lucy);
      expect(lucyIsolation.shared_computer).toBe(true);
      expect(lucyIsolation.cross_session).toBe(0);
      await expect.poll(() => [lucyIsolation.alpha_lucy, lucyIsolation.beta_lucy].every((id) => existsSync(join(homedir(), '.solo', 'agents', id, 'workspace', 'MEMORY.md')))).toBe(true);

      await page.getByRole('button', { name: `Switch to ${alphaName}` }).click();
      await expect(page.getByText(alphaChannelName, { exact: true })).toBeVisible();
      await expect(page.getByText(betaChannelName, { exact: true })).toHaveCount(0);
      await expect(page.getByText('People', { exact: true })).toBeVisible();
      await expect(page.getByText(userB.user.display_name, { exact: true })).toBeVisible();

      await page.getByRole('button', { name: `Switch to ${betaName}` }).click();
      await expect(page.getByText(betaChannelName, { exact: true })).toBeVisible();
      await expect(page.getByText(alphaChannelName, { exact: true })).toHaveCount(0);
      await page.getByRole('button', { name: 'Switch to Solo Public' }).click();
      await expect(page.getByText(alphaChannelName, { exact: true })).toHaveCount(0);
      await expect(page.getByText(betaChannelName, { exact: true })).toHaveCount(0);

      expect((await call<Channel[]>(request, userB, 'get', '/api/v1/channels', alpha!.id)).some((item) => item.id === alphaGeneral!.id)).toBe(true);
      const outsiderWorkspaceAccess = await request.get(`${apiBase}/api/v1/channels`, {
        headers: { authorization: `Bearer ${outsider.access_token}`, 'X-Workspace-ID': alpha!.id },
      });
      expect(outsiderWorkspaceAccess.status()).toBe(403);
      const outsiderResourceAccess = await request.get(`${apiBase}/api/v1/channels/${alphaGeneral!.id}`, {
        headers: { authorization: `Bearer ${outsider.access_token}` },
      });
      expect(outsiderResourceAccess.status()).toBe(404);

      const computerB = await call<Computer>(request, userB, 'post', '/api/v1/computers', undefined, { name: `B Computer ${suffix}` });
      connectDaemon(profileB, computerB);

      await expect.poll(() => databaseJSON<{ a: string; b: string }>(`
        SELECT json_build_object(
          'a', COALESCE((SELECT status FROM computers WHERE id='${computerA.id}'), ''),
          'b', COALESCE((SELECT status FROM computers WHERE id='${computerB.id}'), '')
        )::text
      `), { intervals: [250, 500, 1000] }).toEqual({ a: 'online', b: 'online' });

      const portA = Number(readFileSync(join(daemonProfileDir(profileA), 'port'), 'utf8').trim());
      const portB = Number(readFileSync(join(daemonProfileDir(profileB), 'port'), 'utf8').trim());
      const pidA = Number(readFileSync(join(daemonProfileDir(profileA), 'daemon.pid'), 'utf8').trim());
      const pidB = Number(readFileSync(join(daemonProfileDir(profileB), 'daemon.pid'), 'utf8').trim());
      expect(portA).not.toBe(portB);
      expect(pidA).not.toBe(pidB);
      expect(daemon(profileA, ['status'])).toContain(computerA.id);
      expect(daemon(profileB, ['status'])).toContain(computerB.id);

      const runtimeB = await availableRuntime(request, userB, alpha!.id, computerB.id);
      await call<ChannelProject>(request, userA, 'patch', `/api/v1/channels/${alphaGeneral!.id}/project`, alpha!.id, {
        source: 'https://example.invalid/shared-project.git',
        baseline_version: 'main',
      });
      await call<ChannelProject>(request, userA, 'put', `/api/v1/channels/${alphaGeneral!.id}/project/mappings/${computerA.id}`, alpha!.id, {
        local_path: projectDirA,
        version: 'main',
        access_mode: 'read_write',
      });
      await call<ChannelProject>(request, userB, 'put', `/api/v1/channels/${alphaGeneral!.id}/project/mappings/${computerB.id}`, alpha!.id, {
        local_path: projectDirB,
        version: 'main',
        access_mode: 'read_write',
      });
      const projectAsA = await call<ChannelProject>(request, userA, 'get', `/api/v1/channels/${alphaGeneral!.id}/project`, alpha!.id);
      const projectAsB = await call<ChannelProject>(request, userB, 'get', `/api/v1/channels/${alphaGeneral!.id}/project`, alpha!.id);
      expect(projectAsA.baseline_version).toBe('main');
      expect(projectAsA.mappings.find((mapping) => mapping.user_id === userA.user.id)).toMatchObject({ computer_id: computerA.id, local_path: projectDirA, available: true });
      expect(projectAsA.mappings.find((mapping) => mapping.user_id === userB.user.id)?.local_path).toBeUndefined();
      expect(projectAsB.mappings.find((mapping) => mapping.user_id === userB.user.id)).toMatchObject({ computer_id: computerB.id, local_path: projectDirB, available: true });
      expect(projectAsB.mappings.find((mapping) => mapping.user_id === userA.user.id)?.local_path).toBeUndefined();

      agentA = await call<Agent>(request, userA, 'post', `/api/v1/channels/${alphaGeneral!.id}/agents`, alpha!.id, {
        name: `ProfileA${suffix}`,
        computer_id: computerA.id,
        model_provider: runtimeA.type,
        model_name: runtimeA.type === 'claude' ? 'sonnet' : '',
        system_prompt: `When you introduce yourself, use solo message send exactly once with ${markerA}. When a message contains PROJECT_MAPPING_PROOF, create agent-a-proof.txt with exactly ${projectMarkerA}, then send a visible reply containing ${projectMarkerA}.`,
      });
      await expect.poll(() => databaseJSON<{ completed: number; messages: number }>(`
        SELECT json_build_object(
          'completed', (SELECT COUNT(*) FROM agent_runs WHERE agent_id='${agentA!.id}' AND status='completed'),
          'messages', (SELECT COUNT(*) FROM messages WHERE channel_id='${alphaGeneral!.id}' AND sender_id='${agentA!.id}')
        )::text
      `), { timeout: 240000, intervals: [1000, 2000, 5000] }).toEqual({ completed: 1, messages: 1 });

      agentB = await call<Agent>(request, userB, 'post', `/api/v1/channels/${alphaGeneral!.id}/agents`, alpha!.id, {
        name: `ProfileB${suffix}`,
        computer_id: computerB.id,
        model_provider: runtimeB.type,
        model_name: runtimeB.type === 'claude' ? 'sonnet' : '',
        system_prompt: `When you introduce yourself, use solo message send exactly once with ${markerB}. When a message contains PROJECT_MAPPING_PROOF, create agent-b-proof.txt with exactly ${projectMarkerB}, then send a visible reply containing ${projectMarkerB}.`,
      });

      const directoryAsA = await call<Agent[]>(request, userA, 'get', '/api/v1/agents', alpha!.id);
      const foreignForA = directoryAsA.find((item) => item.id === agentB!.id);
      expect(foreignForA).toMatchObject({ owner_id: userB.user.id, system_prompt: '' });
      expect(foreignForA?.computer_id).toBeUndefined();
      const directoryAsB = await call<Agent[]>(request, userB, 'get', '/api/v1/agents', alpha!.id);
      const foreignForB = directoryAsB.find((item) => item.id === agentA!.id);
      expect(foreignForB).toMatchObject({ system_prompt: '' });
      expect(foreignForB?.computer_id).toBeUndefined();

      await expect.poll(() => databaseJSON<{ completed: number; messages: number }>(`
        SELECT json_build_object(
          'completed', (SELECT COUNT(*) FROM agent_runs WHERE agent_id IN ('${agentA.id}','${agentB.id}') AND status='completed'),
          'messages', (SELECT COUNT(*) FROM messages WHERE channel_id='${alphaGeneral.id}' AND sender_id IN ('${agentA.id}','${agentB.id}'))
        )::text
      `), { timeout: 240000, intervals: [1000, 2000, 5000] }).toEqual({ completed: 2, messages: 2 });
      await expect.poll(() => databaseJSON<number>(`
        SELECT count(*)::int FROM agent_runs
        WHERE agent_id IN ('${agentA.id}','${agentB.id}') AND finished_at IS NULL
      `), { timeout: 240000, intervals: [1000, 2000, 5000] }).toBe(0);

      const foreignConnect = await request.post(`${apiBase}/api/v1/channels/${alphaProject!.id}/members`, {
        headers: { authorization: `Bearer ${userB.access_token}`, 'X-Workspace-ID': alpha!.id },
        data: { member_type: 'agent', member_id: agentA.id },
      });
      expect(foreignConnect.status()).toBe(403);
      await call(request, userA, 'post', `/api/v1/channels/${alphaProject!.id}/members`, alpha!.id, { member_type: 'agent', member_id: agentA.id });
      expect((await call<ChannelMember[]>(request, userA, 'get', `/api/v1/channels/${alphaProject!.id}/members`, alpha!.id)).find((member) => member.member_id === agentA!.id)).toMatchObject({
        agent_owner_id: userA.user.id,
        agent_home_channel_id: alphaGeneral!.id,
      });
      const foreignHomeDelete = await request.delete(`${apiBase}/api/v1/channels/${alphaGeneral!.id}/members/${agentA.id}`, {
        headers: { authorization: `Bearer ${userB.access_token}`, 'X-Workspace-ID': alpha!.id },
      });
      expect(foreignHomeDelete.status()).toBe(403);
      await call(request, userB, 'delete', `/api/v1/channels/${alphaProject!.id}/members/${agentA.id}`, alpha!.id);
      expect(databaseJSON<{ active: boolean; home_member: number }>(`
        SELECT json_build_object(
          'active', is_active,
          'home_member', (SELECT count(*) FROM channel_members WHERE channel_id='${alphaGeneral!.id}' AND member_type='agent' AND member_id='${agentA.id}')
        )::text FROM agents WHERE id='${agentA.id}'
      `)).toEqual({ active: true, home_member: 1 });

      await page.evaluate(({ userID, workspaceID }) => localStorage.setItem(`solo_active_workspace_id:${userID}`, workspaceID), { userID: userA.user.id, workspaceID: alpha!.id });
      await page.goto(`/dashboard?channel=${alphaProject!.id}`);
      const createFirstAgent = page.getByRole('button', { name: 'Create first Agent' });
      await expect(createFirstAgent).toBeVisible({ timeout: 15000 });
      await createFirstAgent.click();
      const connectOwnAgent = page.getByRole('dialog').last();
      await expect(connectOwnAgent.getByText('Connect one of my Agents')).toBeVisible();
      await expect(connectOwnAgent.getByText(agentA.name, { exact: true })).toBeVisible();
      await expect(connectOwnAgent.getByText(agentB.name, { exact: true })).toHaveCount(0);
      await page.keyboard.press('Escape');

      const projectTrigger = await call<{ id: string }>(request, userA, 'post', `/api/v1/channels/${alphaGeneral!.id}/messages`, alpha!.id, {
        content: 'PROJECT_MAPPING_PROOF',
      });
      await expect.poll(() => databaseJSON<{ completed: number; mapped: number }>(`
        SELECT json_build_object(
          'completed', count(*) FILTER (WHERE status='completed'),
          'mapped', count(*) FILTER (WHERE
            (agent_id='${agentA.id}' AND project_computer_id='${computerA.id}' AND project_path='${projectDirA}' AND project_version='main') OR
            (agent_id='${agentB.id}' AND project_computer_id='${computerB.id}' AND project_path='${projectDirB}' AND project_version='main')
          )
        )::text
        FROM agent_runs
        WHERE agent_id IN ('${agentA.id}','${agentB.id}') AND trigger_message_id='${projectTrigger.id}'
      `), { timeout: 240000, intervals: [1000, 2000, 5000] }).toEqual({ completed: 2, mapped: 2 });
      await expect.poll(() => existsSync(join(projectDirA, 'agent-a-proof.txt')) && existsSync(join(projectDirB, 'agent-b-proof.txt')), {
        timeout: 15000,
      }).toBe(true);
      expect(readFileSync(join(projectDirA, 'agent-a-proof.txt'), 'utf8').trim()).toBe(projectMarkerA);
      expect(readFileSync(join(projectDirB, 'agent-b-proof.txt'), 'utf8').trim()).toBe(projectMarkerB);

      const agentMessages = await call<MessageList>(request, userA, 'get', `/api/v1/channels/${alphaGeneral.id}/messages?limit=100`, alpha.id);
      const agentAContent = agentMessages.messages.find((message) => message.sender_id === agentA!.id)?.content;
      const agentBContent = agentMessages.messages.find((message) => message.sender_id === agentB!.id)?.content;
      expect(agentAContent).toBeTruthy();
      expect(agentBContent).toBeTruthy();

      const alphaRecent = await call<{ id: string }[]>(request, userA, 'get', '/api/v1/agent-runs', alpha!.id);
      expect(alphaRecent.some((run) => run.id)).toBe(true);
      expect(await call<{ id: string }[]>(request, userA, 'get', '/api/v1/agent-runs', beta!.id)).toEqual([]);
      const alphaInsight = await call<{ agent_runs: number; messages: number }>(request, userA, 'get', '/api/v1/dashboard/insight?window_days=1', alpha!.id);
      const betaInsight = await call<{ agent_runs: number; messages: number }>(request, userA, 'get', '/api/v1/dashboard/insight?window_days=1', beta!.id);
      expect(alphaInsight.agent_runs).toBeGreaterThanOrEqual(1);
      expect(alphaInsight.messages).toBeGreaterThanOrEqual(2);
      expect(betaInsight.agent_runs).toBe(0);
      expect(betaInsight.messages).toBe(0);

      await call(request, userA, 'put', `/api/v1/workspaces/${alpha!.id}/embed`, undefined, {
        enabled: true,
        channel_ids: [alphaGeneral!.id],
      });
      const guestToken = await call<GuestToken>(request, userA, 'post', `/api/v1/workspaces/${alpha!.id}/embed/tokens`, undefined, {
        label: `E2E ${suffix}`,
        expires_in_days: 1,
      });
      const guestInfoResponse = await request.get(`${apiBase}/api/v1/guest/embed`, { headers: { authorization: `Guest ${guestToken.token}` } });
      expect(guestInfoResponse.ok()).toBe(true);
      const guestInfo = await guestInfoResponse.json() as GuestInfo;
      expect(guestInfo.workspace_id).toBe(alpha!.id);
      expect(guestInfo.channels.map((channel) => channel.id)).toEqual([alphaGeneral!.id]);
      const guestMessagesResponse = await request.get(`${apiBase}/api/v1/guest/channels/${alphaGeneral!.id}/messages`, { headers: { authorization: `Guest ${guestToken.token}` } });
      expect(guestMessagesResponse.ok()).toBe(true);
      const guestMessages = await guestMessagesResponse.json() as MessageList;
      expect(guestMessages.messages).toEqual(expect.arrayContaining([
        expect.objectContaining({ content: agentAContent }),
        expect.objectContaining({ content: agentBContent }),
      ]));
      const betaGuestAccess = await request.get(`${apiBase}/api/v1/guest/channels/${(await call<Channel[]>(request, userA, 'get', '/api/v1/channels', beta!.id))[0].id}/messages`, { headers: { authorization: `Guest ${guestToken.token}` } });
      expect(betaGuestAccess.status()).toBe(404);

      await page.evaluate(({ userID, workspaceID }) => localStorage.setItem(`solo_active_workspace_id:${userID}`, workspaceID), { userID: userA.user.id, workspaceID: alpha!.id });
      await page.goto(`/dashboard?channel=${alphaGeneral!.id}`);
      await expect(page.getByText(agentAContent!, { exact: false })).toBeVisible({ timeout: 180000 });
      await expect(page.getByText(agentBContent!, { exact: false })).toBeVisible({ timeout: 180000 });
      const screenshotDir = join(repoRoot, 'frontend', '.audit-shots');
      mkdirSync(screenshotDir, { recursive: true });
      await page.screenshot({ path: join(screenshotDir, `workspace-${suffix}.png`), fullPage: true });

      const guestContext = await browser.newContext();
      const guestPage = await guestContext.newPage();
      const guestErrors: string[] = [];
      guestPage.on('pageerror', (error) => guestErrors.push(error.message));
      guestPage.on('console', (message) => { if (message.type() === 'error') guestErrors.push(message.text()); });
      await guestPage.goto(guestToken.url);
      await expect(guestPage.getByText('Read-only Guest view')).toBeVisible();
      await expect(guestPage.getByText(agentAContent!, { exact: false })).toBeVisible();
      await expect(guestPage.getByText(agentBContent!, { exact: false })).toBeVisible();
      await expect(guestPage.getByText('Guests cannot post messages or invoke Agents')).toBeVisible();
      expect(guestErrors).toEqual([]);
      await guestPage.screenshot({ path: join(screenshotDir, `guest-${suffix}.png`), fullPage: true });
      await guestContext.close();

      await call(request, userA, 'delete', `/api/v1/workspaces/${alpha!.id}/embed/tokens/${guestToken.id}`);
      expect((await request.get(`${apiBase}/api/v1/guest/embed`, { headers: { authorization: `Guest ${guestToken.token}` } })).status()).toBe(401);

      const persisted = databaseJSON<{
        workspace_members: number;
        alpha_channels: number;
        beta_channels: number;
        computers: number;
        daemon_bound_agents: number;
      }>(`
        SELECT json_build_object(
          'workspace_members', (SELECT COUNT(*) FROM workspace_members WHERE workspace_id='${alpha.id}' AND user_id IN ('${userA.user.id}','${userB.user.id}')),
          'alpha_channels', (SELECT COUNT(*) FROM channels WHERE workspace_id='${alpha.id}'),
          'beta_channels', (SELECT COUNT(*) FROM channels WHERE workspace_id='${beta!.id}'),
          'computers', (SELECT COUNT(*) FROM computers WHERE id IN ('${computerA.id}','${computerB.id}') AND status='online'),
          'daemon_bound_agents', (SELECT COUNT(*) FROM agents WHERE id IN ('${agentA.id}','${agentB.id}') AND runtime_id IN ('${computerA.id}','${computerB.id}'))
        )::text
      `);
      expect(persisted).toEqual({ workspace_members: 2, alpha_channels: 3, beta_channels: 3, computers: 2, daemon_bound_agents: 2 });

      await call(request, userA, 'delete', `/api/v1/workspaces/${beta!.id}`);
      expect((await call<Workspace[]>(request, userA, 'get', '/api/v1/workspaces')).some((item) => item.id === beta!.id)).toBe(false);
      expect(databaseJSON<{ deleted: boolean; archived: number }>(`
        SELECT json_build_object(
          'deleted', (SELECT deleted_at IS NOT NULL FROM workspaces WHERE id='${beta.id}'),
          'archived', (SELECT COUNT(*) FROM channels WHERE workspace_id='${beta.id}' AND is_archived=true)
        )::text
      `)).toEqual({ deleted: true, archived: 3 });
    } finally {
      for (const [profile, directory] of [[profileA, daemonProfileDir(profileA)], [profileB, daemonProfileDir(profileB)]] as const) {
        try { daemon(profile, ['stop']); } catch { /* test assertion reports the primary failure */ }
        rmSync(directory, { recursive: true, force: true });
      }
      rmSync(projectDirA, { recursive: true, force: true });
      rmSync(projectDirB, { recursive: true, force: true });
      finishTestRuns(userA, userB, outsider);
      if (alpha) await call(request, userA, 'delete', `/api/v1/workspaces/${alpha.id}`).catch(() => undefined);
      deactivateTestUsers(userA, userB, outsider);
    }
  });

  test('accepts an Admin pre-invitation and a matching domain allowlist without replacing the owner', async ({ page, request }) => {
    const owner = await register(request, 'WorkspaceInviteOwner');
    const suffix = Date.now().toString(36);
    const workspace = await call<Workspace>(request, owner, 'post', '/api/v1/workspaces', undefined, {
      name: `Invite ${suffix}`,
    });
    const domain = `${suffix}.workspace-e2e.invalid`;
    const invitedEmail = `invited@${domain}`;
    const allowlistedEmail = `allowlisted@${domain}`;
    const inheritedChannelName = `everyone-${suffix}`;

    const cleanupUsers: AuthResponse[] = [owner];
    try {
      await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, undefined, {
        email: invitedEmail,
        role: 'admin',
      });
      await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/join-rules`, undefined, {
        rule_type: 'domain',
        value: domain,
      });

      const invitedRegistration = await registerVerified(request, apiBase, {
        data: {
          email: invitedEmail,
          password: 'SoloWorkspace-2026!',
          display_name: `Invited Admin ${suffix}`,
        },
      });
      if (!invitedRegistration.ok()) throw new Error(`invited registration: ${invitedRegistration.status()} ${await invitedRegistration.text()}`);
      const invited = await invitedRegistration.json() as AuthResponse;
      cleanupUsers.push(invited);
      await call(request, invited, 'patch', '/api/v1/users/me', undefined, {
        avatar_url: 'dicebear:pixel-art:3',
      });

      const allowlistedRegistration = await registerVerified(request, apiBase, {
        data: {
          email: allowlistedEmail,
          password: 'SoloWorkspace-2026!',
          display_name: `Allowlisted Member ${suffix}`,
        },
      });
      if (!allowlistedRegistration.ok()) throw new Error(`allowlisted registration: ${allowlistedRegistration.status()} ${await allowlistedRegistration.text()}`);
      const allowlisted = await allowlistedRegistration.json() as AuthResponse;
      cleanupUsers.push(allowlisted);

      for (let index = 0; index < 2; index += 1) {
        const extra = await register(request, `WorkspaceExtra${index}`);
        cleanupUsers.push(extra);
        await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, undefined, {
          email: extra.user.email,
          role: 'member',
        });
      }

      expect((await call<Workspace[]>(request, owner, 'get', '/api/v1/workspaces')).find((item) => item.id === workspace.id)?.role).toBe('owner');
      expect((await call<Workspace[]>(request, invited, 'get', '/api/v1/workspaces')).find((item) => item.id === workspace.id)?.role).toBe('admin');
      expect((await call<Workspace[]>(request, allowlisted, 'get', '/api/v1/workspaces')).find((item) => item.id === workspace.id)?.role).toBe('member');

      const inheritedChannel = await call<Channel>(request, owner, 'post', '/api/v1/channels', workspace.id, {
        name: inheritedChannelName,
      });
      expect((await call<Channel[]>(request, invited, 'get', '/api/v1/channels', workspace.id)).map((channel) => channel.id)).toContain(inheritedChannel.id);
      expect((await call<Channel[]>(request, allowlisted, 'get', '/api/v1/channels', workspace.id)).map((channel) => channel.id)).toContain(inheritedChannel.id);

      const lateMember = await register(request, 'WorkspaceExtra2');
      cleanupUsers.push(lateMember);
      await call(request, owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, undefined, {
        email: lateMember.user.email,
        role: 'member',
      });
      expect((await call<Channel[]>(request, lateMember, 'get', '/api/v1/channels', workspace.id)).map((channel) => channel.id)).toContain(inheritedChannel.id);
      expect(databaseJSON<number>(`SELECT to_json(count(*))::text FROM channel_members WHERE channel_id='${inheritedChannel.id}' AND member_type='user'`)).toBe(6);

      const persisted = databaseJSON<{
        owner_role: string;
        invited_role: string;
        allowlisted_role: string;
        invitation_accepted: boolean;
        general_members: number;
        inherited_members: number;
      }>(`
        SELECT json_build_object(
          'owner_role', (SELECT role FROM workspace_members WHERE workspace_id='${workspace.id}' AND user_id='${owner.user.id}'),
          'invited_role', (SELECT role FROM workspace_members WHERE workspace_id='${workspace.id}' AND user_id='${invited.user.id}'),
          'allowlisted_role', (SELECT role FROM workspace_members WHERE workspace_id='${workspace.id}' AND user_id='${allowlisted.user.id}'),
          'invitation_accepted', EXISTS(SELECT 1 FROM workspace_invitations WHERE workspace_id='${workspace.id}' AND lower(email)=lower('${invitedEmail}') AND accepted_by='${invited.user.id}' AND accepted_at IS NOT NULL),
          'general_members', (SELECT count(*) FROM channel_members cm JOIN channels c ON c.id=cm.channel_id WHERE c.workspace_id='${workspace.id}' AND c.name='general' AND cm.member_type='user' AND cm.member_id IN ('${owner.user.id}','${invited.user.id}','${allowlisted.user.id}')),
          'inherited_members', (SELECT count(*) FROM channel_members cm WHERE cm.channel_id='${inheritedChannel.id}' AND cm.member_type='user')
        )::text
      `);
      expect(persisted).toEqual({
        owner_role: 'owner',
        invited_role: 'admin',
        allowlisted_role: 'member',
        invitation_accepted: true,
        general_members: 3,
        inherited_members: 6,
      });

      await authenticatePage(page, invited);
      await page.addInitScript(({ userID, workspaceID }) => localStorage.setItem(`solo_active_workspace_id:${userID}`, workspaceID), { userID: invited.user.id, workspaceID: workspace.id });
      await page.goto('/dashboard');
      await expect(page.getByRole('button', { name: 'Workspace menu' })).toContainText(workspace.name);
      await expect(page.getByRole('button', { name: 'People 6' })).toBeVisible();
      await expect(page.getByTestId('workspace-person')).toHaveCount(5);
      await expect(page.getByTestId('workspace-person').filter({ hasText: invited.user.display_name })).toHaveCount(1);

      const sidebar = page.locator('aside');
      const channelsToggle = sidebar.getByRole('button', { name: 'Channels 2' });
      await expect(channelsToggle).toHaveAttribute('aria-expanded', 'true');
      await expect(sidebar.getByText('general', { exact: true })).toBeVisible();
      await expect(sidebar.getByText(inheritedChannelName, { exact: true })).toBeVisible();
      await channelsToggle.click();
      await expect(channelsToggle).toHaveAttribute('aria-expanded', 'false');
      await expect(sidebar.getByText('general', { exact: true })).toHaveCount(0);
      await expect(sidebar.getByText(inheritedChannelName, { exact: true })).toHaveCount(0);
      await channelsToggle.click();
      await expect(sidebar.getByText('general', { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Invite a person' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByRole('heading', { name: `Invite to ${workspace.name}` })).toBeVisible();
      await expect(dialog.getByLabel('Invite email')).toBeFocused();
      await expect(dialog.getByRole('button', { name: 'Member' })).toHaveAttribute('aria-pressed', 'true');
      await expect(page.locator('html')).toHaveAttribute('data-skin', 'archive');
      await expect.poll(() => dialog.evaluate((element) => getComputedStyle(element).borderTopWidth)).toBe('1px');
      const screenshotDir = join(repoRoot, 'frontend', '.audit-shots');
      mkdirSync(screenshotDir, { recursive: true });
      await page.screenshot({ path: join(screenshotDir, `workspace-invite-${suffix}.png`), fullPage: true });
      await page.evaluate(() => { document.documentElement.dataset.skin = 'classic'; });
      await expect.poll(() => dialog.evaluate((element) => getComputedStyle(element).borderTopWidth)).toBe('4px');
      await page.evaluate(() => { document.documentElement.dataset.skin = 'archive'; });
      await dialog.getByRole('button', { name: 'Close' }).click();

      await page.getByRole('link', { name: 'Manage all 6 in Settings' }).click();
      await expect(page.getByRole('heading', { name: `Workspace · ${workspace.name}` })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'People (6)' })).toBeVisible();
      const settingsPeople = page.getByRole('heading', { name: 'People (6)' }).locator('..');
      await expect(settingsPeople.getByLabel(invited.user.display_name).locator('img')).toHaveAttribute('src', /^data:image\/svg\+xml/);
      expect((await call<Array<{ user_id: string; avatar_url: string }>>(
        request,
        invited,
        'get',
        `/api/v1/workspaces/${workspace.id}/members`,
      )).find((member) => member.user_id === invited.user.id)?.avatar_url).toBe('dicebear:pixel-art:3');
      const budgetHeader = page.getByTestId('budget-card-header');
      const workspaceHeader = page.getByTestId('workspace-card-header');
      await expect(budgetHeader).toBeVisible();
      await expect(workspaceHeader).toBeVisible();
      await expect.poll(async () => {
        const [budgetStyle, workspaceStyle] = await Promise.all([
          budgetHeader.evaluate((element) => {
            const style = getComputedStyle(element);
            return [style.backgroundColor, style.paddingLeft, style.paddingTop, style.borderBottomWidth];
          }),
          workspaceHeader.evaluate((element) => {
            const style = getComputedStyle(element);
            return [style.backgroundColor, style.paddingLeft, style.paddingTop, style.borderBottomWidth];
          }),
        ]);
        return JSON.stringify({
          equal: JSON.stringify(budgetStyle) === JSON.stringify(workspaceStyle),
          padding: workspaceStyle.slice(1, 3),
          border: workspaceStyle[3],
        });
      }).toBe(JSON.stringify({
        equal: true,
        padding: ['16px', '12px'],
        border: '1px',
      }));
      await expect.poll(() => page.getByTestId('settings-page-icon').evaluate((element) => {
        const icon = element.querySelector('svg');
        return icon ? getComputedStyle(icon).color !== getComputedStyle(element).backgroundColor : false;
      })).toBe(true);
      await page.screenshot({ path: join(screenshotDir, `settings-workspace-${suffix}.png`), fullPage: true });
    } finally {
      await call(request, owner, 'delete', `/api/v1/workspaces/${workspace.id}`).catch(() => undefined);
      deactivateTestUsers(...cleanupUsers);
    }
  });

  test('deletes an offline empty Computer through the themed confirmation dialog', async ({ page, request }) => {
    const owner = await register(request, 'ComputerDeleteOwner');
    const suffix = Date.now().toString(36);
    const computer = await call<Computer>(request, owner, 'post', '/api/v1/computers', undefined, {
      name: `Disposable ${suffix}`,
    });

    try {
      await authenticatePage(page, owner);
      await page.goto('/computers');
      const computerButton = page.getByRole('button').filter({ hasText: computer.name }).first();
      await expect(computerButton).toBeVisible();
      await computerButton.click();
      await expect(computerButton).toHaveAttribute('aria-expanded', 'true');
      const deleteButton = page.getByRole('button', { name: 'Delete Computer', exact: true });
      await expect(deleteButton).toBeVisible({ timeout: 15_000 });
      await deleteButton.click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Delete Computer' })).toBeVisible();
      await expect(dialog).toContainText('This only removes the Solo record');
      await expect(page.locator('html')).toHaveAttribute('data-skin', 'archive');
      await expect.poll(() => dialog.evaluate((element) => getComputedStyle(element).borderTopWidth)).toBe('1px');
      await dialog.getByRole('button', { name: 'Delete Computer', exact: true }).click();
      await expect(dialog).toHaveCount(0);
      await expect(page.getByText(computer.name, { exact: true })).toHaveCount(0);
      expect(databaseJSON<number>(`SELECT to_json(count(*))::text FROM computers WHERE id='${computer.id}'`)).toBe(0);
    } finally {
      await call(request, owner, 'delete', `/api/v1/computers/${computer.id}`).catch(() => undefined);
      deactivateTestUsers(owner);
    }
  });

  test('copies the pairing command next to each equal-sized Copy button', async ({ page, request }) => {
    const owner = await register(request, 'ComputerPairCopyOwner');
    let computer: Computer | undefined;

    try {
      await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'http://localhost:3000' });
      await authenticatePage(page, owner);
      await page.goto('/computers');
      await page.getByRole('button', { name: 'Add Computer', exact: true }).click();
      const dialog = page.getByRole('dialog');

      const copyButtons = dialog.getByRole('button', { name: 'Copy', exact: true });
      const commands = dialog.locator('pre');
      await expect(copyButtons).toHaveCount(2);
      await expect(commands).toHaveCount(2);

      const firstBox = await copyButtons.nth(0).boundingBox();
      const secondBox = await copyButtons.nth(1).boundingBox();
      expect(firstBox).not.toBeNull();
      expect(secondBox).not.toBeNull();
      expect({ width: firstBox!.width, height: firstBox!.height }).toEqual({ width: secondBox!.width, height: secondBox!.height });

      const firstCommand = (await commands.nth(0).textContent()) ?? '';
      const secondCommand = (await commands.nth(1).textContent()) ?? '';
      expect(firstCommand).toContain('curl -fsSL');
      expect(secondCommand).toContain('solo daemon connect');
      await copyButtons.nth(0).click();
      expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(firstCommand);
      await copyButtons.nth(1).click();
      expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(secondCommand);

      computer = (await call<Computer[]>(request, owner, 'get', '/api/v1/computers'))[0];
      expect(computer).toBeTruthy();
    } finally {
      if (computer) await call(request, owner, 'delete', `/api/v1/computers/${computer.id}`).catch(() => undefined);
      deactivateTestUsers(owner);
    }
  });
});
