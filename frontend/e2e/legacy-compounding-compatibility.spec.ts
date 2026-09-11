import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer } from './support/local-computer';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8180';
const database = process.env.POSTGRES_DB ?? '';
function sql(query: string) {
  if (!database.startsWith('solo_compounding_legacy_e2e_')) throw new Error('Use the isolated database seeded by TestLegacyCompoundingMigrationPostgres');
  return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', database, '-At', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('upgraded legacy account keeps duplicate tasks and runs its original global team', async ({ page, request }) => {
  test.setTimeout(600_000);
  const channel = sql("SELECT id FROM channels WHERE name='legacy-compat-0'");
  const owner = sql("SELECT id FROM users WHERE email='legacy-compat@solo.local'");
  const leader = sql("SELECT id FROM agents WHERE name='LegacyMember0'");
  const worker = sql("SELECT id FROM agents WHERE name='LegacyMember1'");
  const tasks = JSON.parse(sql(`SELECT json_agg(json_build_object('id',id,'message_id',message_id,'number',task_number) ORDER BY task_number) FROM tasks WHERE channel_id='${channel}' AND legacy_message_source`)) as { id: string; message_id: string; number: number }[];
  expect(tasks).toHaveLength(3);
  await page.addInitScript(() => localStorage.setItem('solo.locale', 'zh-CN'));
  await page.goto(`/auth/login?return_to=${encodeURIComponent(`/dashboard?channel=${channel}&view=task`)}`);
  await page.locator('input[type=email]').fill('legacy-compat@solo.local');
  await page.locator('input[type=password]').fill('LegacyCompat-2026!');
  await page.locator('button[type=submit]').click();
  await expect(page).toHaveURL(/\/dashboard\?/);
  for (const task of tasks) await expect(page.locator(`[data-task-id="${task.id}"]:visible`)).toContainText(`历史任务 ${task.number}`);
  await expect(page.getByRole('button', { name: '创建任务', exact: true })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem('access_token'));
  expect(token).toBeTruthy();
  const workspace = sql(`SELECT workspace_id FROM channels WHERE id='${channel}'`);
  const headers = { authorization: `Bearer ${token}`, 'X-Workspace-ID': workspace };
  const ambiguous = await request.get(`${base}/api/v1/channels/${channel}/tasks/${tasks[0].message_id}`, { headers });
  expect(ambiguous.status()).toBe(409);
  for (const task of tasks) {
    const response = await request.get(`${base}/api/v1/channels/${channel}/tasks/${task.number}`, { headers });
    expect(response.ok()).toBeTruthy(); expect((await response.json()).id).toBe(task.id);
  }
  const outsideWorkspace = sql(`SELECT workspace_id FROM workspace_members WHERE user_id='${owner}' AND workspace_id<>'${workspace}' LIMIT 1`);
  expect(outsideWorkspace).toBeTruthy();
  const denied = await request.get(`${base}/api/v1/agents/${leader}`, { headers: { ...headers, 'X-Workspace-ID': outsideWorkspace } });
  expect(denied.status()).toBe(404);
  const computer = await acquireLocalComputer(request, base, token!);
  try {
    for (const id of [leader, worker]) {
      const updated = await request.patch(`${base}/api/v1/agents/${id}`, { headers, data: {
        computer_id: computer.id, model_provider: 'claude', model_name: 'sonnet',
        system_prompt: 'On LEGACY_COMPAT_CHECK, run python3 -c "assert 2+2==4; print(4)" once and send LEGACY_TEAM_RUNTIME_OK using solo message send to the current channel. Then stop. If any command fails, stop without searching files, credentials, environment, other Agent directories, or managing services. Do not delegate or send any other message.',
      } });
      expect(updated.ok(), `bind legacy Agent: ${updated.status()}`).toBeTruthy();
    }
    const source = await request.post(`${base}/api/v1/channels/${channel}/messages`, { headers, data: { content: 'LEGACY_COMPAT_CHECK: 请原团队协调者按说明执行一次 Python 检查并回复 LEGACY_TEAM_RUNTIME_OK。', client_msg_id: crypto.randomUUID() } });
    expect(source.ok()).toBeTruthy();
    await expect.poll(() => sql(`SELECT count(*) FROM messages WHERE channel_id='${channel}' AND sender_id='${leader}' AND content LIKE '%LEGACY_TEAM_RUNTIME_OK%'`), { timeout: 300_000 }).toBe('1');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${leader}' AND status='completed'`), { timeout: 60_000 }).toBe('1');
    expect(sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${worker}'`)).toBe('0');
    await page.goto(`/dashboard?channel=${channel}&view=chat`);
    await expect(page.getByText('LEGACY_TEAM_RUNTIME_OK', { exact: true })).toBeVisible();
    expect(sql(`SELECT count(*) FROM agent_relationships WHERE from_agent_id='${leader}' AND channel_id IS NULL`)).toBe('1');
    expect(sql(`SELECT count(*) FROM agents WHERE id IN ('${leader}','${worker}') AND home_channel_id IS NULL`)).toBe('2');
    expect(sql(`SELECT count(*) FROM tasks WHERE channel_id='${channel}' AND message_id='${tasks[0].message_id}' AND creator_id='${owner}'`)).toBe('3');
    await page.goto(`/dashboard?channel=${channel}&view=task`);
    for (const task of tasks) await expect(page.locator(`[data-task-id="${task.id}"]:visible`)).toContainText(`历史任务 ${task.number}`);
  } finally { await computer.release(request); }
});
