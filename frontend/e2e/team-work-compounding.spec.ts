import { selectValue } from './support/select';
import { codexE2EArgs } from './support/runtime';
import { expect, test, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';
const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
test.use({ actionTimeout: 30_000 });
const runtimeTimeout = 300_000;
const publicWorkspace = '00000000-0000-0000-0000-000000000001';
function sql(query: string) { return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim(); }

test('joint owners authorize existing Agents; real work consumes seven corrections and retains its obligation', async ({ page, request, browser }, testInfo) => {
 test.setTimeout(900_000);
 const suffix = Date.now().toString(36);
 type Auth = { access_token: string; refresh_token: string; id: string };
 const accounts: Auth[] = [];
 for (const name of ['owner', 'peer']) {
  const email = `${name}-work-${suffix}@solo.local`;
  const response = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: `${name} ${suffix}` } }); expect(response.ok()).toBeTruthy();
  accounts.push({ ...await response.json(), id: sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}' RETURNING id`).split('\n')[0] });
 }
 const [owner, peer] = accounts;
 async function api<T>(actor: Auth, method: 'get' | 'post' | 'delete', path: string, data?: unknown, workspace = publicWorkspace): Promise<T> {
  const response = await requestAuthenticated(request, base, actor, method, path, { headers: { authorization: `Bearer ${actor.access_token}`, 'X-Workspace-ID': workspace }, data });
  if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
  return response.status() === 204 ? undefined as T : response.json();
 }
 async function loginPage(target: Page, actor: Auth, workspace: string) {
  await target.addInitScript(({ auth, workspaceID }) => { localStorage.setItem('access_token', auth.access_token); localStorage.setItem('refresh_token', auth.refresh_token); localStorage.setItem('solo.locale', 'zh-CN'); localStorage.setItem('solo_active_workspace_id', workspaceID); localStorage.setItem(`solo_active_workspace_id:${auth.id}`, workspaceID); }, { auth: actor, workspaceID: workspace });
 }
 const computer = await acquireLocalComputer(request, base, owner.access_token);
 // Real persisted Runtime access fixture; all work and collaboration actions use product APIs/UI.
 sql(`INSERT INTO computer_members(computer_id,user_id,role) VALUES('${computer.id}','${peer.id}','member') ON CONFLICT DO NOTHING`);
 const home = await api<{ id: string }>(owner, 'post', '/api/v1/channels', { name: `work-home-${suffix}` });
 const peerHome = await api<{ id: string }>(peer, 'post', '/api/v1/channels', { name: `peer-home-${suffix}` });
 const agents: { id: string; actor: Auth }[] = [];
 let workspaceID = '';
 const peerContext = await browser.newContext(); const peerPage = await peerContext.newPage();
 try {
  const a = await api<{ id: string; name: string }>(owner, 'post', `/api/v1/channels/${home.id}/agents`, { name: `Worker${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet', system_prompt: `On introduction send WORKER_READY using solo message send and stop. On WORK_SEQUENCE, create a persistent work mark via solo work mark --file: channel_id is current channel, description="Follow up regression ${suffix}", next_action="Owner will verify and resolve", idempotency_key="followup-${suffix}". Then execute python3 -c 'import time; time.sleep(25)' once. Then try sending INITIAL_DRAFT via solo message send to the assigned Task thread (read task get for its message_id). If held, read every returned newest_message and retry sending to consume all pages until none remain. Revise the final message to CORRECTION_APPLIED green followed by every CORRECTION_N label you received. Leave the work mark open for the owner, then stop. On SECOND_REQUEST send SECOND_DONE and stop. On PROPOSE_AGREEMENT, create the exact scoped collaboration draft requested using solo team propose-agreement --file, then send AGREEMENT_PROPOSED; never approve for either owner. On RESOLVE_WORK, read solo work list, resolve only the named mark with solo work mark --file (id,status=resolved,resolution describing the checked corrections), then send OBLIGATION_RESOLVED. Do not mention another Agent.` }); agents.push({ id: a.id, actor: owner });
  await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${a.id}' AND status='completed'`), { timeout: 240_000 }).toBe('1');
  const b = await api<{ id: string; name: string }>(peer, 'post', `/api/v1/channels/${peerHome.id}/agents`, { name: `Partner${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet', system_prompt: 'On introduction send PARTNER_READY using solo message send and stop. Only respond when explicitly mentioned. Do not start unrelated work.' }); agents.push({ id: b.id, actor: peer });
  await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id IN ('${a.id}','${b.id}') AND status='completed'`), { timeout: 240_000 }).toBe('2');
  const workspace = await api<{ id: string }>(owner, 'post', '/api/v1/workspaces', { name: `Joint work ${suffix}` }); workspaceID = workspace.id;
  await api(owner, 'post', `/api/v1/workspaces/${workspace.id}/members`, { user_id: peer.id });
  const channel = await api<{ id: string }>(owner, 'post', '/api/v1/channels', { name: `joint-${suffix}` }, workspace.id);
  const headers = { authorization: `Bearer ${owner.access_token}`, 'X-Workspace-ID': workspace.id };
  const denied = await requestAuthenticated(request, base, owner, 'post', `/api/v1/channels/${channel.id}/members`, { headers, data: { member_type: 'agent', member_id: b.id } }); expect(denied.status()).toBe(403);
  expect((await api<{ id: string }[]>(owner, 'get', '/api/v1/agents?owned=true', undefined, workspace.id)).some((agent) => agent.id === a.id)).toBeTruthy();
  await api(owner, 'post', `/api/v1/channels/${channel.id}/members`, { member_type: 'agent', member_id: a.id }, workspace.id);
  await api(peer, 'post', `/api/v1/channels/${channel.id}/members`, { member_type: 'agent', member_id: b.id }, workspace.id);
  expect(sql(`SELECT home_channel_id::text FROM agents WHERE id='${a.id}'`)).toBe(home.id);
  await loginPage(page, owner, workspace.id); await page.goto(`/dashboard?channel=${channel.id}&view=task`);
  await api(owner, 'post', `/api/v1/channels/${channel.id}/messages`, { content: `@${a.name} PROPOSE_AGREEMENT：请起草当前频道内的合作约定，from_agent_id=${a.id}，to_agent_id=${b.id}，rel_type=assigns_to，instruction="Only share verified task evidence; ask the owner before expanding scope."。只起草，等待双方各自授权。`, client_msg_id: crypto.randomUUID() }, workspace.id);
  await expect.poll(() => sql(`SELECT count(*) FROM agent_relationship_proposals WHERE channel_id='${channel.id}' AND proposed_by_agent_id='${a.id}'`), { timeout: runtimeTimeout }).toBe('1');
  const agreementID = sql(`SELECT id::text FROM agent_relationship_proposals WHERE channel_id='${channel.id}'`);
  await page.goto(`/dashboard?channel=${channel.id}#team-records`);
  await page.locator(`[data-agreement-id="${agreementID}"]`).getByRole('button', { name: '同意我的参与范围', exact: true }).click();
  expect(sql(`SELECT count(*) FROM agent_relationships WHERE channel_id='${channel.id}'`)).toBe('0');
  await loginPage(peerPage, peer, workspace.id); await peerPage.goto(`/dashboard?channel=${channel.id}#team-records`);
  await peerPage.locator(`[data-agreement-id="${agreementID}"]`).getByRole('button', { name: '同意我的参与范围', exact: true }).click();
  await expect.poll(() => sql(`SELECT count(*) FROM agent_relationships WHERE channel_id='${channel.id}'`)).toBe('1');
  await expect(peerPage.locator(`[data-agreement-id="${agreementID}"]`)).toContainText('已生效');
  await peerPage.screenshot({ path: testInfo.outputPath('agreement-both-owners.png') });
  // Preserve the legacy explicit snapshot API, while the daily UI has no manual pin control.
  await expect(page.getByRole('button', { name: '固定当前团队', exact: true })).toHaveCount(0);
  await api(owner, 'post', `/api/v1/channels/${channel.id}/team-versions`, { reason: 'Owners agree to this exact collaboration snapshot' }, workspace.id);
  expect(sql(`SELECT team_version_id IS NULL FROM channels WHERE id='${channel.id}'`)).toBe('t');
  await peerPage.reload();
  await peerPage.getByRole('button', { name: '同意我的成员使用此版本', exact: true }).click();
  await expect.poll(() => sql(`SELECT team_version_id IS NOT NULL FROM channels WHERE id='${channel.id}'`)).toBe('t');
  const version = sql(`SELECT team_version_id::text FROM channels WHERE id='${channel.id}'`);
  const privateRead = await requestAuthenticated(request, base, peer, 'get', `/api/v1/agents/${a.id}`, { headers: { 'X-Workspace-ID': workspace.id } }); expect(privateRead.status()).toBe(404);
  await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${a.id}`);
  const work = page.getByRole('region', { name: '成员工作状态' });
  await page.getByText('消息接收', { exact: true }).click();
  await expect(work.getByLabel('自动接收消息')).toBeVisible(); await selectValue(work, '自动接收消息', 'nothing');
  await expect.poll(() => sql(`SELECT attention_policy FROM agents WHERE id='${a.id}'`)).toBe('nothing');
  const muted = await api<{ id: string }>(owner, 'post', `/api/v1/channels/${channel.id}/messages`, { content: `@${a.name} MUTED_REQUEST`, client_msg_id: crypto.randomUUID() }, workspace.id);
  await page.waitForTimeout(2500); // Let the asynchronous wake path settle before asserting silence.
  expect(sql(`SELECT count(*) FROM agent_runs WHERE trigger_message_id='${muted.id}'`)).toBe('0');
  await selectValue(work, '自动接收消息', 'mentions'); await expect.poll(() => sql(`SELECT attention_policy FROM agents WHERE id='${a.id}'`)).toBe('mentions');
  const workTask = await api<{ id: string; message_id: string }>(owner, 'post', `/api/v1/channels/${channel.id}/tasks`, { title: `WORK_SEQUENCE ${suffix}`, description: `Create the follow-up mark for this regression, wait 25 seconds, then apply every CORRECTION_N message from this Task thread. Send the final result in the Task thread. Keep the obligation open for the subsequent explicit follow-up.`, assignee: a.id }, workspace.id);
  await expect.poll(() => sql(`SELECT count(*) FROM agent_work_marks WHERE agent_id='${a.id}' AND status='open'`), { timeout: runtimeTimeout }).toBe('1');
  const runID = sql(`SELECT r.id::text FROM agent_runs r JOIN agent_run_task_links link ON link.run_id=r.id WHERE link.task_id='${workTask.id}' AND r.finished_at IS NULL ORDER BY r.started_at DESC LIMIT 1`);
  const taskThread = sql(`SELECT id::text FROM threads WHERE root_message_id='${workTask.message_id}'`);
  for (let i = 1; i <= 7; i++) await api(owner, 'post', `/api/v1/channels/${channel.id}/messages`, { content: `CORRECTION_${i}: use green and include this label in the final response.`, thread_id: taskThread, client_msg_id: crypto.randomUUID() }, workspace.id);
  expect(sql(`SELECT count(*) FROM messages WHERE thread_id='${taskThread}' AND metadata->>'correction_of_run_id'='${runID}'`)).toBe('7');
  const second = await api<{ id: string }>(owner, 'post', `/api/v1/channels/${home.id}/messages`, { content: `@${a.name} SECOND_REQUEST: acknowledge this independent request by sending exactly SECOND_DONE in this channel.`, client_msg_id: crypto.randomUUID() });
  await expect.poll(() => sql(`SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id='${a.id}' AND channel_id='${home.id}'`)).toBe('1');
  await expect.poll(() => sql(`SELECT count(*) FROM messages WHERE metadata->>'agent_run_id'='${runID}' AND content LIKE '%CORRECTION_APPLIED%' AND content LIKE '%green%' AND content LIKE '%CORRECTION_7%'`), { timeout: 240_000 }).toBe('1');
  expect(sql(`SELECT count(*) FROM agent_runs r JOIN messages m ON m.id=r.trigger_message_id WHERE m.metadata->>'correction_of_run_id'='${runID}' AND r.agent_id<>'${a.id}'`)).toBe('0');
  expect(sql(`SELECT count(*) FROM agent_message_consumptions WHERE agent_id='${a.id}' AND run_id='${runID}'`)).toBe('7');
  expect(Number(sql(`SELECT count(*) FROM agent_run_events WHERE run_id='${runID}' AND type='visible_message_held'`))).toBeGreaterThanOrEqual(2);
  expect(sql(`SELECT count(*) FROM messages WHERE metadata->>'agent_run_id'='${runID}' AND content='INITIAL_DRAFT'`)).toBe('0');
  await expect.poll(() => sql(`SELECT count(*) FROM messages m JOIN agent_runs r ON r.id=(m.metadata->>'agent_run_id')::uuid WHERE r.trigger_message_id='${second.id}' AND m.content LIKE '%SECOND_DONE%'`), { timeout: runtimeTimeout }).toBe('1');
  await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${a.id}' AND finished_at IS NULL`)).toBe('0');
  expect(sql(`SELECT (later.backend_started_at>=prior.finished_at)::text FROM agent_runs prior,agent_runs later WHERE prior.id='${runID}' AND later.trigger_message_id='${second.id}'`)).toBe('true');
  await page.reload(); await page.getByText('工作记录', { exact: true }).click(); await expect(work.getByText(`Follow up regression ${suffix}`, { exact: true })).toBeVisible();
  const markID = sql(`SELECT id::text FROM agent_work_marks WHERE agent_id='${a.id}' AND status='open'`);
  await expect(work.getByRole('button', { name: '标记已处理' })).toHaveCount(0);
  await api(owner, 'post', `/api/v1/channels/${channel.id}/messages`, { content: `@${a.name} RESOLVE_WORK：七条补充已核对，独立请求也已完成，请处理你留下的 Follow up regression ${suffix}，并说明依据。`, client_msg_id: crypto.randomUUID() }, workspace.id);
  await expect.poll(() => sql(`SELECT status FROM agent_work_marks WHERE id='${markID}'`), { timeout: runtimeTimeout }).toBe('resolved');
  await expect.poll(() => sql(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}' AND sender_id='${a.id}' AND content LIKE '%OBLIGATION_RESOLVED%'`), { timeout: runtimeTimeout }).toBe('1');
  await page.reload(); await page.getByText('工作记录', { exact: true }).click(); await expect(work.getByText(`Follow up regression ${suffix}`, { exact: true })).toHaveCount(0);
  await page.getByText('高级', { exact: true }).click(); await page.getByText('手动改进', { exact: true }).click(); await expect(page.getByRole('button', { name: '刷新版本' })).toBeVisible();
  await peerPage.goto(`/dashboard?channel=${channel.id}#team-records`);
  await peerPage.locator(`[data-agreement-id="${agreementID}"]`).getByRole('button', { name: '撤回我的参与', exact: true }).click();
  await expect(peerPage.locator(`[data-agreement-id="${agreementID}"]`)).toContainText('已撤回');
  expect(sql(`SELECT count(*) FROM agent_relationships WHERE channel_id='${channel.id}'`)).toBe('0');
  expect(sql(`SELECT status FROM agent_relationship_proposals WHERE id='${agreementID}'`)).toBe('withdrawn');
  await peerPage.screenshot({ path: testInfo.outputPath('agreement-withdrawn.png') });
  await api(owner, 'delete', `/api/v1/channels/${channel.id}/members/${a.id}`, undefined, workspace.id); expect(sql(`SELECT team_version_id IS NULL FROM channels WHERE id='${channel.id}'`)).toBe('t');
  const restore = await requestAuthenticated(request, base, owner, 'post', `/api/v1/channels/${channel.id}/team-versions`, { headers: { ...headers }, data: { version_id: version, reason: 'must not restore revoked membership' } }); expect(restore.status()).toBe(400);
 } finally {
  await peerContext.close(); for (const agent of agents.reverse()) await api(agent.actor, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
  if (workspaceID) await api(owner, 'delete', `/api/v1/workspaces/${workspaceID}`).catch(() => undefined);
  await api(owner, 'delete', `/api/v1/channels/${home.id}`).catch(() => undefined); await api(peer, 'delete', `/api/v1/channels/${peerHome.id}`).catch(() => undefined); await computer.release(request);
 }
});
