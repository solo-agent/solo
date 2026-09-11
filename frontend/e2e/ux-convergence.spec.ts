import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified, requestAuthenticated } from './support/auth';
import { acquireLocalComputer } from './support/local-computer';
import { codexE2EArgs } from './support/runtime';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const sql = (query: string) => execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
test.use({ actionTimeout: 30_000 });

test('Agent prepares and reviews an improvement, owner applies it to one team and confirms the real result', async ({ page, request }, testInfo) => {
  test.setTimeout(1_500_000);
  const suffix = Date.now().toString(36);
  const registered = await registerVerified(request, base, { data: { email: `ux-convergence-${suffix}@solo.local`, password: 'SoloE2E-2026!', display_name: '入口收敛验收' } });
  expect(registered.ok()).toBeTruthy();
  const auth = await registered.json();
  sql(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}'`);
  const headers: Record<string, string> = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'delete', path: string, data?: unknown): Promise<T> => {
    const response = await requestAuthenticated(request, base, auth, method, path, { headers, data });
    if (!response.ok()) throw new Error(`${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined as T : response.json();
  };
  const workspace = await api<{ id: string }>('post', '/api/v1/workspaces', { name: `UX 验收 ${suffix}` });
  headers['X-Workspace-ID'] = workspace.id;
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const channel = await api<{ id: string; name: string }>('post', '/api/v1/channels', { name: `ux-team-${suffix}` });
  const provider = process.env.SOLO_E2E_PROVIDER ?? 'claude';
  const model = process.env.SOLO_E2E_MODEL ?? 'haiku';
  const customArgs = provider === 'codex' ? codexE2EArgs : [];
  const agents: string[] = [];
  const channels: string[] = [channel.id];
  const util = `import json,pathlib,subprocess,sys,hashlib,re
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=40)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
`;
  const submit = `
proof={'input':values,'result':result,'mode':mode,'script':str(pathlib.Path(__file__).resolve()),'received_candidate':globals().get('received')}
body={'expected_task_version':task['version'],'idempotency_key':'result-'+str(task['version']),'artifact_version':hashlib.sha256(json.dumps(proof,sort_keys=True).encode()).hexdigest(),'handoff':{'summary':'ACTUAL_RESULT_'+str(result),'changes':'Executed the installed Python program','risks':'One fixed arithmetic sample','next_steps':'Review the actual result'},'evidence':[{'id':'E1','description':'Actual arithmetic output','content':json.dumps(proof)}]}
path=pathlib.Path.cwd()/'ux-submission.json';path.write_text(json.dumps(body))
cli('message','send','--target',channel+':'+task['message_id'][:8],'-c','ACTUAL_RESULT_'+str(result))
cli('task','submit','-n',n,'-c',channel,'--file',str(path))
print('EXECUTED_ACTUAL_PROGRAM',result)
`;
  const workerScript = (mode: string) => util + `channel=sys.argv[1];n=sys.argv[2]
task=json.loads(cli('task','get','-n',n,'-c',channel))
values=json.loads(re.search(r'\\[[0-9, ]+\\]',task['description']).group());mode=${JSON.stringify(mode)}
result=sum(values)${mode === 'baseline' ? '-2' : ''}
` + submit;
  const receiverScript = util + `channel=sys.argv[1];n=sys.argv[2]
task=json.loads(cli('task','get','-n',n,'-c',channel))
received=json.loads(sys.argv[3]);values=received['input'];result=received['result'];mode='receiver'
assert sum(values)==result==42
` + submit;
  const reviewScript = util + `channel=sys.argv[1];n=sys.argv[2]
task=json.loads(cli('task','get','-n',n,'-c',channel))
history=json.loads(cli('task','submissions','-n',n,'-c',channel))
sub=next(item for item in history if item['id']==task['current_submission_id'])
proof={};actual=None;passed=False
try:
 proof=json.loads(next(item for item in sub['evidence'] if item['id']=='E1')['content'])
 actual=sum(proof['input'])
 passed=actual==proof['result']==42 and proof['script'].endswith('/scripts/check.py') and proof['mode'] in ('baseline','candidate','receiver')
 if proof['mode']=='receiver': passed=passed and proof['received_candidate']['result']==42 and proof['received_candidate']['input']==proof['input']
 reason='Independent Python recomputation: expected '+str(actual)+', received '+str(proof.get('result'))
except (ValueError,TypeError,KeyError,StopIteration):
 reason='Rejected: missing or malformed immutable program output; follow the frozen revision procedure and submit its exact E1 JSON.'
body={'submission_id':sub['id'],'artifact_version':sub['artifact_version'],'idempotency_key':'review-'+sub['id'],'decision':'accepted' if passed else 'rejected','reason':reason,'checks':[{'requirement_id':item['id'],'passed':passed,'reason':reason,'evidence_ids':['E1','independent-check']} for item in sub['contract']['requirements']],'evidence':[{'id':'independent-check','description':'Independent calculation against immutable submitted output','content':json.dumps({'expected':actual,'received':proof.get('result'),'passed':passed,'script':str(pathlib.Path(__file__).resolve())})}]}
path=pathlib.Path.cwd()/'ux-review.json';path.write_text(json.dumps(body))
cli('task','review','-n',n,'-c',channel,'--file',str(path))
cli('message','send','--target',channel+':'+task['message_id'][:8],'-c',reason)
print('EXECUTED_INDEPENDENT_REVIEW',body['decision'])
`;
  const bundle = (files: Record<string, string>) => ({ name: 'ux-check', files: Object.entries({ 'SKILL.md': '---\nname: ux-check\ndescription: Execute the actual arithmetic program and immutable delivery checks\n---\nUse the supplied scripts. Never manufacture evidence or edit the program during an evaluation.', ...files }).map(([path, content]) => ({ path, content: Buffer.from(content).toString('base64') })) });
  const workerPrompt = 'For a greeting, send UX_READY using solo message send and stop. For every assigned execution or fixed evaluation Task, execute the installed program even if its output is wrong; this measures the frozen version, so never calculate a replacement answer, modify the program, or repair its result. Immediately run python3 .agents/skills/ux-check/scripts/check.py <channel UUID> <Task number>. The script actually calculates, sends and submits. After it succeeds, stop; do not submit again. Never edit or replace the installed program. For a Selection outcome notification, read solo work selections once, report its actual status using solo message send and include SELECTION_OUTCOME, then stop; do not start another comparison.';
  try {
    const receiver = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/agents`, { name: `UxReceiver${suffix}`, computer_id: computer.id, model_provider: provider, model_name: model, custom_args: customArgs, system_prompt: 'For greetings, send RECEIVER_READY and stop. For receiver tasks, copy the exact candidate E1 content from the supplied immutable submission into the third argument and execute python3 .agents/skills/ux-check/scripts/check.py <channel UUID> <Task number> <candidate E1 JSON>. The script sends and submits itself. Do not edit it or substitute claimed checks.', skills: [bundle({ 'scripts/check.py': receiverScript })] });
    agents.push(receiver.id);
    const reviewer = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/agents`, { name: `UxReviewer${suffix}`, computer_id: computer.id, model_provider: provider, model_name: model, custom_args: customArgs, system_prompt: 'For greetings, send REVIEWER_READY using solo message send and stop. For designated reviews, never claim or implement the task. Run python3 .agents/skills/ux-check/scripts/review.py <channel UUID> <Task number>. It reads the current immutable submission, independently calculates, records acceptance or rejection with evidence, and sends the explanation. Malformed or missing E1 JSON is a rejection, never permission to approve another way. Stop after successful execution; never approve separately or edit the script.', skills: [bundle({ 'scripts/review.py': reviewScript })] });
    agents.push(reviewer.id);
    const candidateConfig = { system_prompt: workerPrompt, model_provider: provider, model_name: model, custom_args: customArgs, skills: [bundle({ 'scripts/check.py': workerScript('candidate') })] };
    const plan = { channel_id: channel.id, receiver_agent_id: receiver.id, reviewer_agent_id: reviewer.id, problem: 'The actual arithmetic program returned 40 for [20,22].', change: `修正加法结果 ${suffix}`, token_budget: 2_000_000, idempotency_key: `ux-comparison-${suffix}`, cases: [{ title: 'Actual arithmetic regression', input: '[20,22]', requirements: [{ id: 'R1', text: 'Execute the frozen revision with python3 .agents/skills/ux-check/scripts/check.py <channel UUID> <Task number>, without editing or replacing it. Its actual numeric output for [20,22] must equal 42. Submit the script-produced E1 JSON unchanged; any missing program output or substitute calculation fails this requirement.' }] }], team_check: 'Parse the exact candidate E1 output; independently verify the supplied input sums to the reported result 42.' };
    const prepareScript = util + `channel=sys.argv[1]
revision=${JSON.stringify(JSON.stringify({ summary: plan.change, config: candidateConfig }))}
path=pathlib.Path.cwd()/'ux-revision.json';path.write_text(revision)
created=json.loads(cli('work','propose-revision','-c',channel,'--file',str(path)))
plan=json.loads(${JSON.stringify(JSON.stringify(plan))});plan['candidate_revision_id']=created['id']
path=pathlib.Path.cwd()/'ux-plan.json';path.write_text(json.dumps(plan))
selection=json.loads(cli('work','start-selection','-c',channel,'--file',str(path)))
cli('message','send','--target',channel,'-c','已开始固定条件评测：'+selection['id'])
print('AGENT_STARTED_SELECTION',selection['id'])
`;
    const original = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/agents`, { name: `UxWorker${suffix}`, computer_id: computer.id, model_provider: provider, model_name: model, custom_args: customArgs, system_prompt: `For UX_START, preserve the original user message: write its concrete success requirement to a contract file with human result confirmation (kind=human, human_review_mode=decision, reviewer_id=${auth.user.id}, max_revisions=5), then use solo task claim -m <that source message ID> -c <channel> --contract-file <file>. Execute check.py for the returned task number. Do not create a duplicate task or calculate a replacement answer. ` + 'If the user requests PREPARE_IMPROVEMENT, run python3 .agents/skills/ux-check/scripts/prepare.py <channel UUID> exactly once. This prepares the candidate and evaluation through your own Solo credentials. Then report that evaluation has started and stop. ' + workerPrompt, skills: [bundle({ 'scripts/check.py': workerScript('baseline'), 'scripts/prepare.py': prepareScript })] });
    agents.push(original.id);
    await expect.poll(() => sql(`SELECT count(DISTINCT agent_id) FROM agent_runs WHERE agent_id IN ('${original.id}','${receiver.id}','${reviewer.id}') AND status='completed'`), { timeout: 300_000 }).toBe('3');
    await page.addInitScript(({ tokens, workspaceID }) => { localStorage.setItem('access_token', tokens.access_token); localStorage.setItem('refresh_token', tokens.refresh_token); localStorage.setItem('solo_active_workspace_id', workspaceID); localStorage.setItem(`solo_active_workspace_id:${tokens.user.id}`, workspaceID); localStorage.setItem('solo.locale', 'zh-CN'); }, { tokens: auth, workspaceID: workspace.id });
    const source = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/messages`, { content: `@UxWorker${suffix} UX_START：请用已安装的加法程序计算 [20,22]，结果应为 42。保留程序原样并提交它的实际结果，完成后由我确认。` });
    await expect.poll(() => sql(`SELECT count(*) FROM tasks WHERE message_id='${source.id}'`), { timeout: 300_000 }).toBe('1');
    const task = JSON.parse(sql(`SELECT json_build_object('id',id,'creator',creator_id,'contract',contract) FROM tasks WHERE message_id='${source.id}'`));
    expect(task.creator).toBe(auth.user.id);
    expect(task.contract.gate).toMatchObject({ kind: 'human', human_review_mode: 'decision', reviewer_id: auth.user.id });
    expect(task.contract.requirements.length).toBeGreaterThan(0);
    await expect.poll(() => sql(`SELECT sub.handoff->>'summary' FROM tasks t JOIN task_submissions sub ON sub.id=t.current_submission_id WHERE t.id='${task.id}'`), { timeout: 300_000 }).toBe('ACTUAL_RESULT_40');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND finished_at IS NULL`), { timeout: 300_000 }).toBe('0');
    await api('post', `/api/v1/channels/${channel.id}/messages`, { content: `@UxWorker${suffix} 原任务实际得到 40，预期是 42。请 PREPARE_IMPROVEMENT，准备候选并完成独立评测，再让我决定是否在本团队使用。` });
    await expect.poll(() => sql(`SELECT count(*) FROM agent_selections WHERE agent_id='${original.id}'`), { timeout: 300_000 }).toBe('1');
    const selection = JSON.parse(sql(`SELECT json_build_object('id',id,'candidate',candidate_revision_id,'baseline',baseline_revision_id) FROM agent_selections WHERE agent_id='${original.id}'`));
    const trials = JSON.parse(sql(`SELECT json_agg(json_build_object('agent',agent_id,'channel',channel_id)) FROM agent_selection_trials WHERE selection_id='${selection.id}'`)) as { agent: string; channel: string }[];
    expect(trials).toHaveLength(4); agents.push(...trials.map((trial) => trial.agent)); channels.push(...trials.map((trial) => trial.channel));
    const listedChannels = await api<{ id: string }[]>('get', '/api/v1/channels');
    expect(listedChannels.some((item) => trials.some((trial) => trial.channel === item.id))).toBeFalsy();
    const listedAgents = await api<{ id: string }[]>('get', '/api/v1/agents');
    expect(listedAgents.some((item) => trials.some((trial) => trial.agent === item.id))).toBeFalsy();
    const listedTasks = await api<{ channel_id: string }[]>('get', '/api/v1/tasks');
    expect(listedTasks.some((item) => trials.some((trial) => trial.channel === item.channel_id))).toBeFalsy();
    await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);
    await expect(page.getByLabel('改进说明', { exact: true })).toHaveCount(0);
    const proposal = page.locator(`[data-improvement-id="${selection.id}"]`);
    await expect(proposal).toContainText(channel.name);
    await expect.poll(() => sql(`SELECT count(*) FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id='${selection.id}' AND t.status IN ('done','closed')`), { timeout: 900_000 }).toBe('3');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs r JOIN agent_selection_trials tr ON tr.agent_id=r.agent_id WHERE tr.selection_id='${selection.id}' AND r.finished_at IS NULL`), { timeout: 300_000 }).toBe('0');
    await expect.poll(() => sql(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}' AND sender_id='${original.id}' AND content LIKE '%SELECTION_OUTCOME%'`), { timeout: 300_000 }).not.toBe('0');
    expect(sql(`SELECT count(*) FROM task_reviews review JOIN agent_selection_tasks st ON st.task_id=review.task_id WHERE st.selection_id='${selection.id}' AND review.reviewer_id='${auth.user.id}'`)).toBe('0');
    expect(Number(sql(`SELECT count(*) FROM task_reviews review JOIN agent_selection_tasks st ON st.task_id=review.task_id WHERE st.selection_id='${selection.id}' AND st.arm='baseline' AND review.decision='rejected'`))).toBeGreaterThanOrEqual(1);
    expect(sql(`SELECT t.status||':'||st.attempts FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id='${selection.id}' AND st.arm='baseline'`)).toBe('closed:3');
    await page.getByRole('button', { name: '刷新改进状态', exact: true }).click();
    await expect(proposal.getByRole('button', { name: '采用到这个团队', exact: true })).toBeEnabled();
    await testInfo.attach('proposal-ready', { body: await page.screenshot(), contentType: 'image/png' });
    await proposal.getByRole('button', { name: '采用到这个团队', exact: true }).click();
    await expect(proposal).toContainText('此团队正在使用');
    const appliedVersion = sql(`SELECT team_version_id::text FROM channels WHERE id='${channel.id}'`);
    expect(appliedVersion).toMatch(/^[0-9a-f-]{36}$/);
    expect(sql(`SELECT (solo_agent_config(a)=v.config)::text FROM agents a JOIN agent_revisions v ON v.id='${selection.baseline}' WHERE a.id='${original.id}'`)).toBe('true');
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await page.locator(`[data-task-id="${task.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('region', { name: '当前交付' })).toContainText('ACTUAL_RESULT_40');
    await expect(dialog.getByRole('checkbox')).toHaveCount(0);
    await dialog.getByLabel('审核结论与原因', { exact: true }).fill('应为 42，请使用已采用的团队版本重新处理。');
    await dialog.getByRole('button', { name: '退回修改', exact: true }).click();
    await expect.poll(() => sql(`SELECT sub.handoff->>'summary' FROM tasks t JOIN task_submissions sub ON sub.id=t.current_submission_id WHERE t.id='${task.id}'`), { timeout: 300_000 }).toBe('ACTUAL_RESULT_42');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND finished_at IS NULL`), { timeout: 300_000 }).toBe('0');
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await page.locator(`[data-task-id="${task.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await expect(dialog.getByRole('region', { name: '当前交付' })).toContainText('ACTUAL_RESULT_42');
    await expect(dialog.getByRole('button', { name: '接受交付', exact: true })).toBeEnabled();
    await dialog.getByRole('button', { name: '接受交付', exact: true }).click();
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`)).toBe('done');
    expect(sql(`SELECT jsonb_array_length(checks) FROM task_reviews WHERE task_id='${task.id}' AND decision='accepted'`)).toBe('0');
    expect(sql(`SELECT (r.agent_revision_id='${selection.candidate}' AND r.team_version_id='${appliedVersion}')::text FROM tasks t JOIN task_submissions sub ON sub.id=t.current_submission_id JOIN agent_runs r ON r.id=sub.run_id WHERE t.id='${task.id}'`)).toBe('true');
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await page.locator(`[data-task-id="${task.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await dialog.getByText('验证记录与交付成本', { exact: true }).click();
    await expect(dialog.getByLabel('实际投入分钟', { exact: true })).toHaveCount(0);
    await testInfo.attach('result-accepted', { body: await page.screenshot(), contentType: 'image/png' });
    await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);
    await proposal.getByRole('button', { name: '恢复此前版本', exact: true }).click();
    await expect.poll(() => sql(`SELECT member->>'revision_id' FROM channels c JOIN channel_team_versions tv ON tv.id=c.team_version_id CROSS JOIN LATERAL jsonb_array_elements(tv.lockfile->'members') member WHERE c.id='${channel.id}' AND member->>'agent_id'='${original.id}'`)).toBe(selection.baseline);
    const restoredTask = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/tasks`, { title: '恢复版本执行核对', description: '[20,22]', assignee: original.id, contract: { requirements: [{ id: 'R1', text: '执行已恢复程序并如实记录输出，不替换算法。' }], gate: { kind: 'human', human_review_mode: 'decision', reviewer_id: auth.user.id, max_revisions: 3 } } });
    await expect.poll(() => sql(`SELECT sub.handoff->>'summary' FROM tasks t JOIN task_submissions sub ON sub.id=t.current_submission_id WHERE t.id='${restoredTask.id}'`), { timeout: 300_000 }).toBe('ACTUAL_RESULT_40');
    expect(sql(`SELECT (r.agent_revision_id='${selection.baseline}')::text FROM tasks t JOIN task_submissions sub ON sub.id=t.current_submission_id JOIN agent_runs r ON r.id=sub.run_id WHERE t.id='${restoredTask.id}'`)).toBe('true');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND finished_at IS NULL`), { timeout: 300_000 }).toBe('0');
    await page.goto(`/dashboard?channel=${channel.id}#team-records`);
    await expect(page.getByRole('dialog')).toContainText(`团队记录 · ${channel.name}`);
    await testInfo.attach('team-records', { body: await page.screenshot(), contentType: 'image/png' });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.getByRole('dialog').evaluate((node) => node.scrollWidth <= node.clientWidth + 1)).toBeTruthy();
    await testInfo.attach('team-records-mobile', { body: await page.screenshot(), contentType: 'image/png' });
    const receipt = JSON.parse(sql(`SELECT json_build_object('selection_id',sel.id,'plan',sel.plan,'decisions',(SELECT json_agg(json_build_object('decision',d.decision,'reason',d.reason,'evidence',d.evidence)) FROM agent_selection_decisions d WHERE d.selection_id=sel.id),'delivery_task','${task.id}','applied_version','${appliedVersion}','current_team_version',(SELECT team_version_id FROM channels WHERE id='${channel.id}')) FROM agent_selections sel WHERE sel.id='${selection.id}'`));
    await testInfo.attach('real-runtime-and-database-receipt', { body: JSON.stringify(receipt, null, 2), contentType: 'application/json' });
  } finally {
    if (agents.length) {
      const ids = agents.map((id) => `'${id}'`).join(',');
      const state = sql(`SELECT json_build_object('agents',(SELECT json_agg(json_build_object('id',id,'name',name)) FROM agents WHERE id IN (${ids})),'tasks',(SELECT json_agg(json_build_object('id',t.id,'channel_id',t.channel_id,'status',t.status,'version',t.version,'current_submission_id',t.current_submission_id,'handoff',sub.handoff)) FROM tasks t LEFT JOIN task_submissions sub ON sub.id=t.current_submission_id WHERE t.claimer_id IN (${ids})),'runs',(SELECT json_agg(json_build_object('id',id,'agent',agent_id,'channel',channel_id,'thread',thread_id,'status',status,'revision',agent_revision_id,'team_version',team_version_id,'started_at',started_at,'finished_at',finished_at)) FROM agent_runs WHERE agent_id IN (${ids})))`);
      await testInfo.attach('persisted-state-before-cleanup', { body: state, contentType: 'application/json' });
    }
    await page.close();
    for (const id of agents.reverse()) await api('delete', `/api/v1/agents/${id}`).catch(() => undefined);
    for (const id of channels.reverse()) await api('delete', `/api/v1/channels/${id}`).catch(() => undefined);
    await computer.release(request);
  }
});
