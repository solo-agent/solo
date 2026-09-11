import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';
import { acceptPairedSelection } from './support/paired-selection';
import type { SkillBundle } from '../lib/types';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const runtimeTimeout = 300_000;
test.use({ actionTimeout: 30_000 });
function sql(query: string) { return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim(); }

test('Skill files are evaluated, published, pinned and restored by real Agent Runs', async ({ page, request }) => {
 test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 2_400_000 : 1_200_000);
 const suffix = Date.now().toString(36);
 const registered = await registerVerified(request, base, { data: { email: `skills-${suffix}@solo.local`, password: 'SoloE2E-2026!', display_name: 'Skill 验收员' } });
 expect(registered.ok()).toBeTruthy(); const auth = await registered.json();
 sql(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}'`);
 auth.access_token = 'stale-e2e-access-token';
 const headers: Record<string,string> = { authorization: `Bearer ${auth.access_token}` };
 const api = async <T>(method: 'get'|'post'|'delete', path: string, data?: unknown): Promise<T> => {
  const response = await requestAuthenticated(request, base, auth, method, path, { headers, data });
  if (!response.ok()) throw new Error(`${path}: ${response.status()} ${await response.text()}`);
  return response.status() === 204 ? undefined as T : response.json();
 };
 let scope = await api<{id:string}>('post','/api/v1/workspaces',{name:`Skills ${suffix}`}); headers['X-Workspace-ID']=scope.id;
 expect(auth.access_token).not.toBe('stale-e2e-access-token');
 const workspaceIDs = [scope.id];
 const computer = await acquireLocalComputer(request, base, auth.access_token);
 if (process.env.SOLO_E2E_SKILL_REMOTE === '1') {
  const enrollment = await api<{ enrollment_token: string }>('post', `/api/v1/computers/${computer.id}/enrollment`);
  const daemonID=process.env.SOLO_E2E_DAEMON_ID;
  if (!daemonID?.startsWith('daemon-e2e-') || !process.env.SOLO_DAEMON_STATE_DIR || !process.env.SOLO_DAEMON_CREDENTIAL_FILE) throw new Error('Isolated make-managed stack required');
  execFileSync('make',['rebuild', 'SOLO_DAEMON_PROFILE=',`DAEMON_ID=${daemonID}`,`SOLO_DAEMON_STATE_DIR=${process.env.SOLO_DAEMON_STATE_DIR}`,`SOLO_DAEMON_CREDENTIAL_FILE=${process.env.SOLO_DAEMON_CREDENTIAL_FILE}`],{cwd:resolve(process.cwd(),'..'),env:{...process.env,SOLO_COMPUTER_ID:computer.id,SOLO_ENROLLMENT_TOKEN:enrollment.enrollment_token},timeout:runtimeTimeout,stdio:'pipe'});
  await expect.poll(async () => (await api<{id:string;status:string;pairing_status:string}[]>('get','/api/v1/computers')).find((item)=>item.id===computer.id),{timeout:30_000}).toMatchObject({status:'online',pairing_status:'paired'});
 }
 let channel = await api<{ id: string }>('post', '/api/v1/channels', { name: `skills-${suffix}` });
 const homeChannel = channel.id;
 const directory = mkdtempSync(join(tmpdir(), 'solo-skill-fixture-'));
 const folder = join(directory, 'revision-check'); mkdirSync(join(folder, 'scripts'), { recursive: true }); mkdirSync(join(folder, 'resources'));
 const source = `import json, os, pathlib, subprocess, sys
root=pathlib.Path(__file__).resolve().parent.parent
values=json.loads((root/'resources/input.json').read_text())['values']
result=sum(values)
assert result in (40,42)
channel=sys.argv[1]; n=sys.argv[2]
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
current=json.loads(cli('task','get','-n',n,'-c',channel))
request=pathlib.Path.cwd()/'skill-submission.json'
request.write_text(json.dumps({'expected_task_version':current['version'],'idempotency_key':'skill-check-'+n,'artifact_version':'skill-result-'+str(result),'handoff':{'summary':'SKILL_RESULT_'+str(result),'changes':'Executed the versioned Python script against its versioned JSON input','risks':'Only this input checked','next_steps':'Independent human acceptance'},'evidence':[{'id':'E1','description':'Actual Skill input, script execution and result','content':json.dumps({'input':values,'sum':result,'script':str(pathlib.Path(__file__).resolve())})}]}))
revisions=json.loads(cli('work','revisions','-c',channel))
config=revisions[0]['config']; config['system_prompt']+='\\nReusable improvement proposal after checking Skill output.'
proposal=pathlib.Path.cwd()/'skill-proposal.json'
proposal.write_text(json.dumps({'summary':'Runtime Skill improvement proposal','config':config}))
cli('work','propose-revision','-c',channel,'--file',str(proposal))
cli('message','send','--target',channel+':'+current['message_id'][:8],'-c','SKILL_RESULT_'+str(result))
cli('task','submit','-n',n,'-c',channel,'--file',str(request))
print('ACTUAL_SKILL_EXECUTED',result)
`;
 const files = { 'SKILL.md': '---\nname: revision-check\ndescription: Execute the pinned arithmetic script and submit evidence\n---\nRun python3 scripts/check.py <channel> <task number>. Read resources/input.json through the script.\n', 'scripts/check.py': source, 'resources/input.json': JSON.stringify({ values: [21,21] }) };
 for (const [path, text] of Object.entries(files)) writeFileSync(join(folder,path),text);
 const bundle = (values: number[]): SkillBundle => ({ name: 'revision-check', files: Object.entries({ ...files, 'resources/input.json': JSON.stringify({ values }) }).map(([path, content]) => ({ path, content: Buffer.from(content).toString('base64') })) });
 const agents: string[] = []; const extraChannels: string[] = [];
 const workspace = (id: string) => join(homedir(), '.solo', 'agents', id, 'workspace');
 const prompt = 'On introduction send SKILL_WORKER_READY using solo message send and stop. For any assigned Task, execute exactly python3 .agents/skills/revision-check/scripts/check.py <channel UUID> <task number> once, then stop. This trusted task script executes the actual versioned files and records its own messages and submission. Do not edit installed Skill files or manufacture evidence. On PRIVATE_NOTE follow the human instruction to write MEMORY.md then send MEMORY_SAVED and stop.';
 try {
  const original = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, { name: `SkillWorker${suffix}`, computer_id: computer.id, model_provider:process.env.SOLO_E2E_PROVIDER ?? 'claude', model_name:process.env.SOLO_E2E_MODEL ?? 'haiku', custom_args:process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], system_prompt:prompt, skills:[bundle([20,20])] });
  agents.push(original.id);
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND status='completed'`),{timeout:runtimeTimeout}).toBe('1');
  const baseline = sql(`SELECT agent_revision_id::text FROM agent_runs WHERE agent_id='${original.id}' ORDER BY started_at LIMIT 1`);
  const privateNote = `PRIVATE_ONLY_${suffix}`;
  await api('post',`/api/v1/channels/${channel.id}/messages`,{content:`@${original.name} PRIVATE_NOTE: write the exact text ${privateNote} into MEMORY.md in your current workspace, then send MEMORY_SAVED.`,client_msg_id:crypto.randomUUID()});
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND status='completed'`),{timeout:runtimeTimeout}).toBe('2');
  expect(readFileSync(join(workspace(original.id),'MEMORY.md'),'utf8')).toContain(privateNote);
  scope = await api<{id:string}>('post','/api/v1/workspaces',{name:`Skill team ${suffix}`}); workspaceIDs.push(scope.id); headers['X-Workspace-ID']=scope.id;
  channel = await api<{id:string}>('post','/api/v1/channels',{name:`skill-team-${suffix}`});
  await api('post',`/api/v1/channels/${channel.id}/members`,{member_type:'agent',member_id:original.id});
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${original.id}' AND channel_id='${channel.id}' AND status='completed'`),{timeout:runtimeTimeout}).toBe('1');
  expect(sql(`SELECT home_channel_id::text FROM agents WHERE id='${original.id}'`)).toBe(homeChannel);
  await page.addInitScript(({access,refresh,workspaceID,userID})=>{localStorage.setItem('access_token',access);localStorage.setItem('refresh_token',refresh);localStorage.setItem('solo.locale','zh-CN');localStorage.setItem('solo_active_workspace_id',workspaceID);localStorage.setItem(`solo_active_workspace_id:${userID}`,workspaceID);},{access:auth.access_token,refresh:auth.refresh_token,workspaceID:scope.id,userID:auth.user.id});
  await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);
  await page.getByText('高级',{exact:true}).click(); await page.getByText('手动改进',{exact:true}).click();
  const panel=page.getByLabel('手动改进配置', { exact: true });
  const backends=await api<{type:string}[]>('get','/api/v1/agent-backends');
  const provider=panel.getByRole('button',{name:'候选 Provider',exact:true});
  await provider.click();
  await expect.poll(()=>page.getByRole('listbox').getByRole('option').evaluateAll(options=>options.map(option=>option.getAttribute('data-value')).sort())).toEqual([...new Set([...backends.map(backend=>backend.type),'openai','anthropic'])].sort());
  await provider.press('Escape');
  await expect(provider).toHaveAttribute('aria-expanded','false');
  await expect(panel.locator('summary').filter({hasText:'revision-check'})).toBeVisible();
  await panel.getByLabel('加入 Skill 文件夹',{exact:true}).setInputFiles(folder);
  await panel.getByLabel('改进说明',{exact:true}).fill(`Skill candidate ${suffix}`);
  await panel.getByLabel('评测要求',{exact:true}).fill('Run the actual versioned Python script and resource input; independently verify sum = 42.');
  await panel.getByRole('button',{name:'创建独立评测',exact:true}).click();
  await expect(panel.getByRole('status')).toContainText('已建立独立评测成员',{timeout:30_000});
  const candidate=sql(`SELECT id::text FROM agent_revisions WHERE agent_id='${original.id}' AND summary='Skill candidate ${suffix}'`);
  const evaluator=sql(`SELECT evaluation_agent_id::text FROM agent_revisions WHERE id='${candidate}'`);agents.push(evaluator);
  const evaluationChannel=sql(`SELECT home_channel_id::text FROM agents WHERE id='${evaluator}'`);extraChannels.push(evaluationChannel);
  const evaluationTask=sql(`SELECT id::text FROM tasks WHERE channel_id='${evaluationChannel}' AND title='评测：Skill candidate ${suffix}'`);
  const reviewTask = async (id: string, channelID: string, expected: number) => {
   await expect.poll(()=>sql(`SELECT status FROM tasks WHERE id='${id}'`),{timeout:runtimeTimeout}).toBe('in_review');
   await page.goto(`/dashboard?channel=${channelID}&view=task`);
   await page.locator(`[data-task-id="${id}"]:visible`).getByRole('button',{name:'查看成果',exact:true}).click();
   const dialog=page.getByRole('dialog'); await expect(dialog.getByText(`SKILL_RESULT_${expected}`,{exact:false}).first()).toBeVisible();
   await dialog.getByLabel('确认此项通过').check(); await dialog.getByLabel('R1 检查依据').fill(`实际输入与脚本执行结果一致，独立计算为 ${expected}`); await dialog.getByLabel('审核结论与原因').fill('已核对版本文件、实际输出与数据库提交记录。'); await dialog.getByRole('button',{name:'通过验收',exact:true}).click();
   await expect.poll(()=>sql(`SELECT status FROM tasks WHERE id='${id}'`)).toBe('done');
  };
  await reviewTask(evaluationTask,evaluationChannel,42);
  expect(existsSync(workspace(evaluator))).toBeTruthy();
  const evaluationMemory = join(workspace(evaluator),'MEMORY.md');
  expect(existsSync(evaluationMemory) ? readFileSync(evaluationMemory,'utf8') : '').not.toContain(privateNote);
  expect(sql(`SELECT count(*) FROM agent_revisions WHERE agent_id='${evaluator}' AND summary='Runtime Skill improvement proposal' AND created_by='${evaluator}'`)).toBe('1');
  const receiverSource=source.replace("values=json.loads((root/'resources/input.json').read_text())['values']\nresult=sum(values)\nassert result in (40,42)", "received=json.loads(sys.argv[3])\nvalues=received['input']\nresult=received['sum']\nassert sum(values)==result==42").replace("'input':values,'sum':result,'script':str(pathlib.Path(__file__).resolve())", "'input':values,'sum':result,'script':str(pathlib.Path(__file__).resolve()),'received_candidate':received");
  const receiverBundle=bundle([21,21]); receiverBundle.files=receiverBundle.files.map(file=>file.path==='scripts/check.py'?{...file,content:Buffer.from(receiverSource).toString('base64')}:file.path==='SKILL.md'?{...file,content:Buffer.from('---\nname: revision-check\ndescription: Independently verify the exact candidate evidence using the receiver script\n---\nFrom your own workspace run python3 .agents/skills/revision-check/scripts/check.py <channel UUID> <Task number> <candidate E1 content as one quoted JSON argument>. All three arguments are required. The script field inside candidate E1 is evidence only; never execute that path or a file from another Agent workspace. The installed receiver script validates and submits its own result.').toString('base64')}:file);
  const receiver=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`SkillReceiver${suffix}`,computer_id:computer.id,model_provider:process.env.SOLO_E2E_PROVIDER ?? 'claude',model_name:process.env.SOLO_E2E_MODEL ?? 'haiku', custom_args:process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [],system_prompt:'For introductions send SKILL_RECEIVER_READY using solo message send. For receiver Tasks take E1 content from the exact candidate submission in the prompt and execute python3 .agents/skills/revision-check/scripts/check.py <channel UUID> <Task number> <exact candidate E1 JSON> once. The installed script parses the candidate input and independently checks the sum, sends and submits its actual output.',skills:[receiverBundle]}); agents.push(receiver.id);
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${receiver.id}' AND status='completed'`),{timeout:runtimeTimeout}).toBe('1');
  const selectionID=await acceptPairedSelection({page,api,sql,agentID:original.id,candidateID:candidate,receiverID:receiver.id,input:'Run the installed versioned Skill script on its own resources/input.json and retain the actual output. Do not change the files.',requirement:'Execute the pinned Python script and its versioned resource; report the actual input and output. The baseline may report 40; the candidate should report 42.',teamCheck:'Execute your own installed receiver script with all three arguments: channel UUID, Task number, exact candidate E1 JSON. The candidate script path is evidence only, never execute it. Independently verify its input sum equals the submitted sum 42.',verifySubmission:(arm,taskID)=>{
   const submission=JSON.parse(sql(`SELECT json_build_object('agent',s.submitted_by,'evidence',s.evidence) FROM task_submissions s JOIN tasks t ON t.current_submission_id=s.id WHERE t.id='${taskID}'`));
   const output=JSON.parse(submission.evidence.find((item:{id:string})=>item.id==='E1').content);
   expect(output.script).toContain(`/agents/${submission.agent}/workspace/.solo/revision-skills/`);
   expect(output.sum).toBe(arm==='baseline'?40:42);
   if(arm==='receiver'){
    const candidateOutput=JSON.parse(sql(`SELECT e->>'content' FROM agent_selection_tasks st JOIN task_submissions s ON s.id=st.input_submission_id CROSS JOIN LATERAL jsonb_array_elements(s.evidence) e WHERE st.task_id='${taskID}' AND e->>'id'='E1'`));
    expect(output.received_candidate).toEqual(candidateOutput);
   }
  },agents,channels:extraChannels});
  expect(sql(`SELECT count(*) FROM agent_selection_trials tr JOIN channels c ON c.id=tr.channel_id WHERE tr.selection_id='${selectionID}' AND c.workspace_id='${scope.id}'`)).toBe('3');
  await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);await page.getByText('高级',{exact:true}).click(); await page.getByText('手动改进',{exact:true}).click();
  await page.locator(`[data-revision-id="${candidate}"]`).getByRole('button',{name:'全局发布通过评测的版本',exact:true}).click();await expect(panel.getByRole('status')).toContainText('已更新正式成员配置');
  const pinned=await api<{version_id:string}>('post',`/api/v1/channels/${channel.id}/team-versions`,{reason:'Pin verified Skill v2'});
  await page.locator(`[data-revision-id="${baseline}"]`).getByRole('button',{name:'全局恢复此版本',exact:true}).click();await expect(panel.getByRole('status')).toContainText('已更新正式成员配置');
  await expect.poll(()=>sql(`SELECT solo_agent_config(a)=(SELECT config FROM agent_revisions WHERE id='${baseline}') FROM agents a WHERE id='${original.id}'`)).toBe('t');
  const createTask = (title: string, expected: number) => api<{id:string}>('post',`/api/v1/channels/${channel.id}/tasks`,{title,assignee:original.id,contract:{requirements:[{id:'R1',text:`Run the pinned Skill script; independently verify result = ${expected}.`}],gate:{kind:'human',reviewer_id:auth.user.id,max_revisions:3}}});
  const fixed=await createTask('Pinned Skill remains v2',42);await reviewTask(fixed.id,channel.id,42);
  expect(sql(`SELECT count(*) FROM task_submissions s JOIN agent_runs r ON r.id=s.run_id WHERE s.task_id='${fixed.id}' AND r.agent_revision_id='${candidate}' AND r.team_version_id='${pinned.version_id}'`)).toBe('1');
  await api('post',`/api/v1/channels/${channel.id}/team-versions`,{version_id:'live',reason:'Verify restored live Skill v1'});
  const restored=await createTask('Restored Skill uses original files',40);await reviewTask(restored.id,channel.id,40);
  expect(sql(`SELECT count(*) FROM task_submissions s JOIN agent_runs r ON r.id=s.run_id WHERE s.task_id='${restored.id}' AND r.agent_revision_id='${baseline}' AND r.team_version_id IS NULL`)).toBe('1');
  if (process.env.SOLO_E2E_SKILL_REMOTE === '1') expect(sql(`SELECT count(*) FROM task_submissions s JOIN agent_runs r ON r.id=s.run_id WHERE s.task_id IN ('${fixed.id}','${restored.id}') AND r.computer_id='${computer.id}' AND r.execution_attempt_id IS NOT NULL AND r.accepted_at IS NOT NULL`)).toBe('2');
  expect(JSON.parse(readFileSync(join(workspace(original.id),'.agents/skills/revision-check/resources/input.json'),'utf8')).values).toEqual([20,20]);
  expect(readFileSync(join(workspace(original.id),'MEMORY.md'),'utf8')).toContain(privateNote);
  expect(sql(`SELECT count(DISTINCT e.payload->>'skills_sha256') FROM task_submissions s JOIN agent_run_events e ON e.run_id=s.run_id WHERE s.task_id IN ('${fixed.id}','${restored.id}') AND e.type='execution_configuration' AND length(e.payload->>'skills_sha256')=64`)).toBe('2');
 } finally {
  for(const id of agents.reverse())await api('delete',`/api/v1/agents/${id}`).catch(()=>undefined);
  for(const id of extraChannels)await api('delete',`/api/v1/channels/${id}`).catch(()=>undefined);
  for(const id of workspaceIDs.reverse())await api('delete',`/api/v1/workspaces/${id}`).catch(()=>undefined);
  await computer.release(request);if (process.env.SOLO_E2E_SKILL_REMOTE === '1') await api('post',`/api/v1/computers/${computer.id}/credential/revoke`).catch(()=>undefined);rmSync(directory,{recursive:true,force:true});
 }
});
