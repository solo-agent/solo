import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified } from './support/auth';

const codexArgs = process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [];
const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
function sql(query: string) { return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim(); }
test.use({ actionTimeout: 30000 });

test('one Agent claims Tasks, mentions, reviews and subscriptions from the same durable Inbox', async ({ page, request }, testInfo) => {
  test.setTimeout(900000);
  const suffix = Date.now().toString(36);
  const registration = await registerVerified(request, base, { data: { email: `inbox-${suffix}@solo.local`, password: 'SoloE2E-2026!', display_name: 'Inbox 验收员' } });
  expect(registration.ok()).toBeTruthy(); const auth = await registration.json();
  sql(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}'`);
  const headers: Record<string, string> = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'patch' | 'delete', path: string, data?: unknown): Promise<T> => { const r = await request[method](`${base}${path}`, { headers, data }); if (!r.ok()) throw new Error(`${path}: ${r.status()} ${await r.text()}`); return r.status() === 204 ? undefined as T : r.json(); };
  const workspace = await api<{ id: string }>('post', '/api/v1/workspaces', { name: `Inbox ${suffix}` }); headers['X-Workspace-ID'] = workspace.id;
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const main = await api<{ id: string }>('post', '/api/v1/channels', { name: `inbox-main-${suffix}` });
  const ordinary = await api<{ id: string }>('post', '/api/v1/channels', { name: `inbox-updates-${suffix}` });
  const directory = mkdtempSync(join(tmpdir(), 'solo-inbox-')); const script = join(directory, 'execute.py');
  writeFileSync(script, `import json,os,pathlib,subprocess,sys,time
root=pathlib.Path(${JSON.stringify(directory)})
mode,channel,n=sys.argv[1:4]
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=40)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
task=json.loads(cli('task','get','-n',n,'-c',channel))
if mode=='review':
 subs=json.loads(cli('task','submissions','-n',n,'-c',channel))
 sub=next(s for s in subs if s['id']==task['current_submission_id'])
 evidence=next(e for e in sub['evidence'] if e['id']=='E1')
 values=json.loads(evidence['content']);assert sum(values['input'])==values['result']==42
 body={'submission_id':sub['id'],'artifact_version':sub['artifact_version'],'decision':'accepted','reason':'Independently executed the exact submitted input and checked its result','checks':[{'requirement_id':'R1','passed':True,'evidence_ids':['E1'],'reason':'Python recomputation matched 42'}],'idempotency_key':'inbox-review-'+n}
else:
 if 'INBOX_BLOCK' in task['title']:
  (root/'blocked').write_text('actual Python tool is running')
  deadline=time.monotonic()+360
  while not (root/'release').exists():
   assert time.monotonic()<deadline,'owner did not release the running check'
   time.sleep(0.2)
 values={'input':[21,21],'result':sum([21,21])};assert values['result']==42
 task=json.loads(cli('task','get','-n',n,'-c',channel))
 body={'expected_task_version':task['version'],'artifact_version':'inbox-42','handoff':{'summary':'INBOX_ACTUAL_42','changes':'Executed Python sum assertion','risks':'One small sample','next_steps':'Inspect exact result'},'evidence':[{'id':'E1','description':'Actual arithmetic input and output','content':json.dumps(values)}],'idempotency_key':'inbox-submit-'+n}
path=root/(mode+'-'+channel+'-'+n+'.json');path.write_text(json.dumps(body))
cli('task','review' if mode=='review' else 'submit','-n',n,'-c',channel,'--file',str(path))
if mode=='review':
 cli('work','list')
 cli('team','agreements','-c',channel)
 proposal={'from_agent_id':os.environ['SOLO_AGENT_ID'],'to_agent_id':sub['submitted_by'],'rel_type':'collaborates_with','weight':1,'instruction':'INBOX_HANDOFF: provide exact input/result JSON; receiver independently recomputes before accepting; return incompatible evidence to the original Task.'}
 agreement=root/'agreement.json';agreement.write_text(json.dumps(proposal))
 cli('team','propose-agreement','-c',channel,'--file',str(agreement))
cli('message','send','--target',channel+':'+task['message_id'][:8],'-c','INBOX_REVIEWED' if mode=='review' else 'INBOX_ACTUAL_42')
print('ACTUAL_INBOX_EXECUTION',mode,n)
`);
  const agents: string[] = [];
  try {
    const worker = await api<{ id: string; name: string }>('post', `/api/v1/channels/${main.id}/agents`, { name: `InboxWorker${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', model_name: process.env.SOLO_E2E_MODEL ?? 'haiku', custom_args: codexArgs, system_prompt: `For introduction only send INBOX_READY via solo message send. For an assigned Task execute python3 ${script} work <channel UUID> <Task number> once, in the foreground, with shell timeout 480000 ms; the script performs the actual task, may wait for the owner, submits and sends its own result. For a dispatched independent submission review execute the same script with mode review. Do not repeat a successful script or send an extra result. For a message containing INBOX_URGENT reply exactly INBOX_URGENT_DONE using solo message send. For INBOX_ORDINARY reply exactly INBOX_ORDINARY_DONE. Do not claim unrelated tasks or expand the requested work.` }); agents.push(worker.id);
    const waitGreeting = async (id:string, count:number) => {
      await expect.poll(() => Number(sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${id}' AND finished_at IS NOT NULL`)), {timeout:180000}).toBe(count);
      expect(Number(sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${id}' AND status='completed'`))).toBe(count);
    };
    await waitGreeting(worker.id,1);
    await api('post', `/api/v1/channels/${ordinary.id}/members`, { member_type: 'agent', member_id: worker.id });
    await waitGreeting(worker.id,2);
    await api('post', `/api/v1/agents/${worker.id}/attention`, { channel_id: main.id, policy: 'mentions' });
    const author = await api<{ id: string }>('post', `/api/v1/channels/${main.id}/agents`, { name: `InboxAuthor${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', model_name: process.env.SOLO_E2E_MODEL ?? 'haiku', custom_args: codexArgs, system_prompt: `On introduction only send AUTHOR_READY using solo message send. For assigned Tasks run exactly python3 ${script} work <channel UUID> <Task number> once; it submits real evidence and sends its own result. Do not repeat or review.` }); agents.push(author.id);
    await waitGreeting(author.id,1);
    await api('post', `/api/v1/agents/${author.id}/attention`, { policy: 'nothing' });
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${worker.id}' AND finished_at IS NULL`), { timeout: 180000 }).toBe('0');
    const contract = (reviewer: string, kind = 'human') => ({ requirements: [{ id: 'R1', text: `Execute the prescribed Python client ${script} using mode work for delivery and mode review for independent review. It must retain the exact [21,21] input and numeric result 42; INBOX_BLOCK must wait for the owner release in that client.` }], gate: { kind, reviewer_id: reviewer, max_revisions: 3 } });
    const block = await api<{ id: string }>('post', `/api/v1/channels/${main.id}/tasks`, { title: `INBOX_BLOCK ${suffix}`, description: `Run exactly python3 ${script} work ${main.id} <Task number> in the foreground with a 480000 ms timeout. The real client waits for my release, then submits and sends its own result.`, assignee: worker.id, contract: contract(auth.user.id) });
    await expect.poll(() => existsSync(join(directory, 'blocked')), { timeout: 180000 }).toBeTruthy();
    const blockRun = sql(`SELECT r.id::text FROM agent_runs r JOIN agent_run_task_links l ON l.run_id=r.id WHERE l.task_id='${block.id}' AND r.finished_at IS NULL`); expect(blockRun).toMatch(/^[0-9a-f-]{36}$/);
    const next = await api<{ id: string }>('post', `/api/v1/channels/${main.id}/tasks`, { title: `INBOX_NEXT ${suffix}`, description: `Run exactly python3 ${script} work ${main.id} <Task number>. The client performs the check and submission.`, assignee: worker.id, contract: contract(auth.user.id) });
    await expect.poll(() => sql(`SELECT count(*) FROM agent_pending_work WHERE task_id='${next.id}' AND status='pending'`)).toBe('1');
    const urgent = await api<{ id: string }>('post', `/api/v1/channels/${main.id}/messages`, { content: `@${worker.name} INBOX_URGENT`, client_msg_id: crypto.randomUUID() });
    await expect.poll(() => sql(`SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id='${worker.id}' AND channel_id='${main.id}' AND requires_visible_result`)).toBe('1');
    const review = await api<{ id: string }>('post', `/api/v1/channels/${main.id}/tasks`, { title: `INBOX_REVIEW ${suffix}`, description: `Deliver with python3 ${script} work ${main.id} <Task number>. The reviewer independently runs python3 ${script} review ${main.id} <Task number> on the exact submitted evidence.`, assignee: author.id, contract: contract(worker.id, 'agent') });
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${review.id}'`), { timeout: 180000 }).toBe('in_review');
    await api('post', `/api/v1/channels/${ordinary.id}/messages`, { content: 'INBOX_ORDINARY', client_msg_id: crypto.randomUUID() });
    await expect.poll(() => sql(`SELECT count(*) FROM agent_inbox WHERE agent_id='${worker.id}'`)).toBe('4');
    await page.addInitScript(({ access, refresh, workspaceID, userID }) => { localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo_active_workspace_id', workspaceID); localStorage.setItem(`solo_active_workspace_id:${userID}`, workspaceID); localStorage.setItem('solo.locale', 'zh-CN'); }, { access: auth.access_token, refresh: auth.refresh_token, workspaceID: workspace.id, userID: auth.user.id });
    await page.goto(`/dashboard?channel=${main.id}&panel=agent&agent=${worker.id}`);
    await page.getByText('工作记录', { exact: true }).click();
    const queue = page.getByRole('list', { name: 'Agent 工作队列' }); await expect(queue.getByRole('listitem')).toHaveCount(4);
    await expect(queue.getByRole('listitem').nth(0)).toContainText('INBOX_NEXT'); await expect(queue.getByRole('listitem').nth(1)).toContainText('会话'); await expect(queue.getByRole('listitem').nth(2)).toContainText('INBOX_REVIEW'); await expect(queue.getByRole('listitem').nth(3)).toContainText('订阅与问候');
    expect(sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${worker.id}' AND finished_at IS NULL`)).toBe('1');
    await page.screenshot({ path: testInfo.outputPath('work-queue-readonly.png') });
    writeFileSync(join(directory, 'release'), 'continue the actual Python check');
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${review.id}'`), { timeout: 300000 }).toBe('done');
    await expect.poll(() => sql(`SELECT count(*) FROM messages WHERE channel_id='${ordinary.id}' AND sender_id='${worker.id}' AND content='INBOX_ORDINARY_DONE'`), { timeout: 180000 }).toBe('1');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${worker.id}' AND finished_at IS NULL`), { timeout: 90000 }).toBe('0');
    expect(sql(`SELECT count(*) FROM agent_inbox WHERE agent_id='${worker.id}'`)).toBe('0');
    expect(sql(`SELECT count(*) FROM agent_runs a JOIN agent_runs b ON a.agent_id=b.agent_id AND a.id<b.id WHERE a.agent_id='${worker.id}' AND a.backend_started_at IS NOT NULL AND b.backend_started_at IS NOT NULL AND a.backend_started_at<b.finished_at AND b.backend_started_at<a.finished_at`)).toBe('0');
    expect(sql(`SELECT count(*) FROM task_reviews r JOIN task_submissions sub ON sub.id=r.submission_id JOIN agent_run_task_links link ON link.task_id=sub.task_id AND link.role='related' JOIN agent_runs run ON run.id=link.run_id AND run.agent_id=r.reviewer_id JOIN agent_run_token_usage u ON u.run_id=run.id WHERE sub.task_id='${review.id}' AND r.decision='accepted' AND run.backend_started_at IS NOT NULL AND u.actual_tokens>0`)).toBe('1');
    expect(sql(`SELECT count(*) FROM agent_runs WHERE trigger_message_id='${urgent.id}' AND status='completed'`)).toBe('1');
    await page.goto(`/dashboard?channel=${ordinary.id}`); await expect(page.getByText('INBOX_ORDINARY_DONE', { exact: true }).first()).toBeVisible();
    await page.goto(`/dashboard?channel=${main.id}&view=task`);
    for (const title of [`INBOX_BLOCK ${suffix}`, `INBOX_NEXT ${suffix}`]) {
      const card = page.locator('[data-task-id]').filter({ hasText: title });
      await card.getByRole('button', { name: '查看成果', exact: true }).click(); const dialog = page.getByRole('dialog');
      await expect(dialog).toContainText('INBOX_ACTUAL_42'); await dialog.getByRole('checkbox').check(); await dialog.getByPlaceholder('说明检查方法与证据').fill('Inspected exact input [21,21] and real result 42.'); await dialog.getByLabel('审核结论与原因', { exact: true }).fill('Actual execution and serialized responsibility verified.'); await dialog.getByRole('button', { name: '通过验收', exact: true }).click();
    }
    await expect.poll(() => sql(`SELECT count(*) FROM tasks WHERE id IN ('${block.id}','${next.id}','${review.id}') AND status='done'`)).toBe('3');
    const proposal = sql(`SELECT id::text FROM agent_relationship_proposals WHERE channel_id='${main.id}' AND proposed_by_agent_id='${worker.id}' AND instruction LIKE 'INBOX_HANDOFF:%'`); expect(proposal).toMatch(/^[0-9a-f-]{36}$/);
    expect(sql(`SELECT status FROM agent_relationship_proposals WHERE id='${proposal}'`)).toBe('pending');
    await page.goto(`/dashboard?channel=${main.id}#team-records`);
    const agreement = page.locator(`[data-agreement-id="${proposal}"]`); await expect(agreement).toContainText('INBOX_HANDOFF'); await agreement.getByRole('button', { name: '同意我的参与范围', exact: true }).click(); await expect(agreement).toContainText('已生效');
    expect(sql(`SELECT count(*) FROM agent_relationships r JOIN agent_relationship_proposals p ON p.relationship_id=r.id WHERE p.id='${proposal}' AND r.channel_id='${main.id}'`)).toBe('1');

  } finally {
    writeFileSync(join(directory, 'release'), 'cleanup'); await page.close(); for (const id of agents) await api('delete', `/api/v1/agents/${id}`).catch(() => undefined);
    await api('delete', `/api/v1/workspaces/${workspace.id}`).catch(() => undefined); await computer.release(request); rmSync(directory, { recursive: true, force: true });
  }
});
