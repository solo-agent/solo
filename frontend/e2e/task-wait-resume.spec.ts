import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const runtimeTimeout = 300_000;
test.use({ actionTimeout: 30_000 });
function sql(query: string) {
  return execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('real Runtime waits durably, works independently, and resumes the original Task after a confirmed condition', async ({ page, request }, testInfo) => {
  test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 1_500_000 : 600_000);
  const suffix = Date.now().toString(36);
  const email = `wait-${suffix}@solo.local`;
  const registration = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: '等待验收员' } });
  const auth = await registration.json();
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'delete', path: string, data?: unknown): Promise<T> => {
    const response = await requestAuthenticated(request, base, auth, method, path, { headers, data });
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined as T : response.json();
  };
  const owner = sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}' RETURNING id`).split('\n')[0];
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const channel = await api<{ id: string }>('post', '/api/v1/channels', { name: `wait-${suffix}` });
  const directory = mkdtempSync(join(tmpdir(), 'solo-wait-protocol-'));
  const script = join(directory, 'check.py');
  writeFileSync(script, `import json, pathlib, subprocess, sys, time
root=pathlib.Path(${JSON.stringify(directory)})
channel=${JSON.stringify(channel.id)}
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
mode=sys.argv[1]
if mode=='independent':
 assert sum(range(5))==10
 (root/'independent-running').write_text('actual separate calculation verified')
 deadline=time.monotonic()+120
 while not (root/'independent-release').exists():
  assert time.monotonic()<deadline
  time.sleep(0.2)
 cli('message','send','--target',channel,'-c','INDEPENDENT_WORK_VERIFIED')
else:
 n=sys.argv[2]
 task=json.loads(cli('task','get','-n',n,'-c',channel))
 target=channel+':'+task['message_id'][:8]
 req=root/'request.json'
 if mode=='wait':
  payload={'expected_task_version':task['version'],'idempotency_key':'wait-for-owner','condition':{'kind':'signal','description':'Owner has verified the real input file input.json and authorizes computing its sum'},'handoff':{'summary':'Original Task retained; input needs independent confirmation','changes':'No calculation submitted yet','risks':'Unconfirmed input','next_steps':'Wait for owner evidence'},'next_action':'Run python3 '+str(root/'check.py')+' finish '+n+' once, then stop.'}
  req.write_text(json.dumps(payload))
  first=json.loads(cli('task','wait','-n',n,'-c',channel,'--file',str(req)))
  again=json.loads(cli('task','wait','-n',n,'-c',channel,'--file',str(req)))
  assert first['id']==again['id']
  cli('message','send','--target',target,'-c','WAIT_REGISTERED: original Task retained until owner confirms the input.')
 elif mode=='finish':
  waits=json.loads(cli('task','waits','-n',n,'-c',channel))['waits']
  assert len(waits)==1 and waits[0]['status']=='resumed'
  data=json.loads((root/'input.json').read_text())
  result=sum(data['values'])
  assert result==42
  req.write_text(json.dumps({'expected_task_version':task['version'],'idempotency_key':'resume-delivery','artifact_version':'input-v1-result-42','handoff':{'summary':'Computed verified input after the recorded wait','changes':'sum is 42','risks':'Only this input checked','next_steps':'Human independent acceptance'},'evidence':[{'id':'E1','description':'Actual Python input and sum','content':json.dumps({'input':data,'sum':result})}]}))
  cli('message','send','--target',target,'-c','ORIGINAL_TASK_RESUMED: verified sum = 42')
  cli('task','submit','-n',n,'-c',channel,'--file',str(req))
 else: raise AssertionError('unknown mode')
print('REAL_WAIT_PROTOCOL_'+mode.upper())
`);
  let agentID = '';
  try {
    const agent = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, {
      name: `Waiter${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'haiku',
      system_prompt: 'On introduction send WAIT_WORKER_READY and stop. For a Task titled WAIT_PROTOCOL run the provided Python file with wait and its task number, then stop after it has registered the wait. Do not submit yet. On a Server resume notification execute the exact saved next_action (the same script, finish, task number), then stop. For INDEPENDENT_CHECK execute its supplied command once; its checkpoint wait is intentional. Use the real shell and current-turn solo CLI. Do not edit the script, fabricate results, make a replacement Task, or perform extra work. The Python scripts publish the necessary messages themselves.',
    });
    agentID = agent.id;
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${agent.id}' AND status='completed'`), { timeout: runtimeTimeout }).toBe('1');
    const task = await api<{ id: string; task_number: number }>('post', `/api/v1/channels/${channel.id}/tasks`, { title: 'WAIT_PROTOCOL', description: `First run python3 ${script} wait <this task number>. When the recorded condition is satisfied, run its exact saved next_action to finish the same Task.`, assignee: agent.id, contract: { requirements: [{ id: 'R1', text: 'After the owner confirms input.json, actually calculate its sum, preserve the same Task, and submit reproducible input/output evidence.' }], gate: { kind: 'human', reviewer_id: owner, max_revisions: 3 } } });
    const waitState = () => JSON.parse(sql(`SELECT COALESCE((SELECT json_build_object('id',id,'status',status,'run_id',run_id,'fulfillment',fulfillment)::text FROM task_waits WHERE task_id='${task.id}'),'{}')`));
    await expect.poll(() => waitState().status, { timeout: runtimeTimeout }).toBe('waiting');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${agent.id}' AND finished_at IS NULL`)).toBe('0');
    expect(sql(`SELECT count(*) FROM task_waits WHERE task_id='${task.id}'`)).toBe('1');
    await page.addInitScript(({ access, refresh }) => {
      localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo.locale', 'zh-CN');
    }, { access: auth.access_token, refresh: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    const card = page.locator(`[data-task-id="${task.id}"]:visible`);
    await expect(card.getByRole('button', { name: '等待条件中', exact: true })).toBeVisible();
    // The same make-managed stack is restarted; no local condition cache survives.
    await page.goto('about:blank');
    execFileSync('make', ['rebuild', 'SOLO_DAEMON_PROFILE=', `DAEMON_ID=${process.env.SOLO_E2E_DAEMON_ID}`, `SOLO_DAEMON_STATE_DIR=${process.env.SOLO_DAEMON_STATE_DIR}`, `SOLO_DAEMON_CREDENTIAL_FILE=${process.env.SOLO_DAEMON_CREDENTIAL_FILE}`], { cwd: resolve(process.cwd(), '..'), timeout: 180_000, stdio: 'pipe' });
    await expect.poll(async () => { try { return (await request.get(`${base}/readyz`)).ok(); } catch { return false; } }).toBeTruthy();
    expect(waitState().status).toBe('waiting');
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await card.getByRole('button', { name: '等待条件中', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByText('查看等待记录', { exact: true }).click();
    await expect(dialog.getByText('Original Task retained; input needs independent confirmation', { exact: false })).toBeVisible();
    await api('post', `/api/v1/channels/${channel.id}/messages`, { content: `@${agent.name} INDEPENDENT_CHECK: execute python3 ${script} independent once. Keep WAIT_PROTOCOL waiting; this is separate work and should not create a Task.`, client_msg_id: crypto.randomUUID() });
    await expect.poll(() => existsSync(join(directory, 'independent-running')), { timeout: runtimeTimeout }).toBeTruthy();
    expect(waitState().status).toBe('waiting');
    writeFileSync(join(directory, 'input.json'), JSON.stringify({ values: [20, 22] }));
    await dialog.getByLabel('条件证据或取消原因').fill('已实际核对 input.json，输入为 20 和 22；允许计算这份已确认输入。');
    await dialog.getByRole('button', { name: '确认条件已满足', exact: true }).click();
    await expect(dialog.getByText('条件已确认，等待负责人空闲', { exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath('wait-confirmed.png') });
    expect(waitState().fulfillment.confirmed_by).toBe(owner);
    expect(waitState().run_id).toBeNull();
    const confirmedAt = Date.now();
    await expect.poll(() => { expect(waitState().run_id).toBeNull(); return Date.now() - confirmedAt; }, { intervals: [1000] }).toBeGreaterThan(6000);
    writeFileSync(join(directory, 'independent-release'), 'continue');
    await expect.poll(() => waitState().status, { timeout: runtimeTimeout }).toBe('resumed');
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`), { timeout: runtimeTimeout }).toBe('in_review');
    // Moving from execution to review remounts the board card and closes its dialog.
    await expect(card.getByRole('button', { name: '等待条件中', exact: true })).toHaveCount(0);
    await card.getByRole('button', { name: '查看成果', exact: true }).click();
    await dialog.getByText('验证记录与交付成本', { exact: true }).click();
    await expect(dialog.getByRole('region', { name: '等待与恢复记录' })).toContainText('已恢复原任务');
    const delivery = page.getByRole('dialog');
    await expect(delivery.getByRole('region', { name: '当前交付' })).toContainText('Computed verified input after the recorded wait');
    await page.screenshot({ path: testInfo.outputPath('wait-resumed-same-task.png') });
    // Use the actual Gate API after independently checking the persisted evidence.
    const current = await api<{ current_submission_id: string }>('get', `/api/v1/tasks/${task.id}`);
    const evidence = JSON.parse(sql(`SELECT evidence->0->>'content' FROM task_submissions WHERE id='${current.current_submission_id}'`));
    expect(evidence).toEqual({ input: { values: [20, 22] }, sum: 42 });
    await api('post', `/api/v1/tasks/${task.id}/review`, { submission_id: current.current_submission_id, artifact_version: 'input-v1-result-42', decision: 'accepted', reason: 'Independent actual input/output check passed', idempotency_key: 'wait-e2e-review', checks: [{ requirement_id: 'R1', passed: true, reason: 'Same Task resumed and 20+22=42', evidence_ids: ['E1'] }] });
    await page.reload();
    await expect(card.getByRole('button', { name: '重新打开', exact: true })).toBeVisible();
    expect(sql(`SELECT count(*) FROM tasks WHERE channel_id='${channel.id}'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM agent_run_task_links WHERE task_id='${task.id}' AND run_id='${waitState().run_id}' AND role='primary'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM task_waits WHERE task_id='${task.id}' AND status='resumed'`)).toBe('1');
    expect(sql(`SELECT status FROM tasks WHERE id='${task.id}'`)).toBe('done');
    expect(sql(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}' AND content='INDEPENDENT_WORK_VERIFIED'`)).toBe('1');
    // Acceptance can precede the provider's final response and usage settlement.
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs r LEFT JOIN agent_run_token_usage u ON u.run_id=r.id WHERE r.agent_id='${agentID}' AND (r.status<>'completed' OR r.finished_at IS NULL OR u.actual_tokens IS NULL)`), { timeout: runtimeTimeout }).toBe('0');
    await testInfo.attach('wait-runs-state', { body: Buffer.from(sql(`SELECT json_agg(json_build_object('id',r.id,'agent_id',r.agent_id,'status',r.status,'finished_at',r.finished_at,'actual_tokens',u.actual_tokens)) FROM agent_runs r LEFT JOIN agent_run_token_usage u ON u.run_id=r.id WHERE r.agent_id='${agentID}'`)), contentType: 'application/json' });
  } finally {
    if (agentID) await api('delete', `/api/v1/agents/${agentID}`).catch(() => undefined);
    await api('delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    await computer.release(request);
    rmSync(directory, { recursive: true, force: true });
  }
});
