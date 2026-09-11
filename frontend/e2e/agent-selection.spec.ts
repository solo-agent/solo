import { selectValue } from './support/select';
import { codexE2EArgs } from './support/runtime';
import { expect,test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base=process.env.SOLO_E2E_API_URL??'http://127.0.0.1:8080';
const runtimeTimeout = 300_000;
function sql(query:string){return execFileSync('docker',['exec','solo-postgres','psql','-U','solo','-d',process.env.POSTGRES_DB??'solo','-tA','-v','ON_ERROR_STOP=1','-c',query],{encoding:'utf8'}).trim();}
test.use({actionTimeout:30000});

test('paired Selection executes fixed cases and receiver checks before owner publication',async({page,request})=>{
 test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 1_500_000 : 600_000);
 const suffix=Date.now().toString(36);
 const registered=await registerVerified(request,base,{data:{email:`selection-${suffix}@solo.local`,password:'SoloE2E-2026!',display_name:'Selection 验收员'}});
 expect(registered.ok()).toBeTruthy();const auth=await registered.json();
 sql(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}'`);
 const headers:Record<string,string>={authorization:`Bearer ${auth.access_token}`};
 const api=async<T>(method:'get'|'post'|'delete',path:string,data?:unknown):Promise<T>=>{const response=await requestAuthenticated(request,base,auth,method,path,{headers,data});if(!response.ok())throw new Error(`${path}: ${response.status()} ${await response.text()}`);return response.status()===204?undefined as T:response.json();};
 const workspace=await api<{id:string}>('post','/api/v1/workspaces',{name:`Selection ${suffix}`});headers['X-Workspace-ID']=workspace.id;
 const computer=await acquireLocalComputer(request,base,auth.access_token);
 const channel=await api<{id:string}>('post','/api/v1/channels',{name:`selection-source-${suffix}`});
 const finish=`
request={'expected_task_version':task['version'],'idempotency_key':'result-'+n,'artifact_version':'result-'+str(result),'handoff':{'summary':'ACTUAL_RESULT_'+str(result),'changes':'Executed the installed Python script','risks':'One fixed sample','next_steps':'Independent acceptance'},'evidence':[{'id':'E1','description':'Actual input and output','content':json.dumps({'input':values,'result':result,'script':str(pathlib.Path(__file__).resolve()),'received_candidate':globals().get('received')})}]}
path=pathlib.Path.cwd()/'selection-submission.json';path.write_text(json.dumps(request))
cli('message','send','--target',channel+':'+task['message_id'][:8],'-c','ACTUAL_RESULT_'+str(result))
cli('task','submit','-n',n,'-c',channel,'--file',str(path))
print('EXECUTED_SELECTION_SCRIPT',result)
`;
 const prelude=`import json,pathlib,subprocess,sys
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
channel=sys.argv[1];n=sys.argv[2]
task=json.loads(cli('task','get','-n',n,'-c',channel))
`;
 const bundle=(body:string)=>({name:'selection-check',files:[{path:'SKILL.md',content:Buffer.from('---\nname: selection-check\ndescription: Execute a real fixed comparison and submit its evidence\n---\nRun scripts/check.py with channel and Task number. Receiver also passes exact candidate evidence JSON.').toString('base64')},{path:'scripts/check.py',content:Buffer.from(prelude+body+finish).toString('base64')}]});
 const workerPrompt='For introductions send SELECTION_READY using solo message send and stop. For assigned Tasks immediately run python3 .agents/skills/selection-check/scripts/check.py <channel UUID> <Task number>. The installed script reads the fixed input, executes the actual calculation and submits. The script itself sends and submits; after its successful execution finish the turn without submitting again. Never edit the script, substitute tasks or invent results.';
 const agents:string[]=[];const trialChannels:string[]=[];
 try{
  const original=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`SelectionWorker${suffix}`,computer_id:computer.id,model_provider:process.env.SOLO_E2E_PROVIDER??'claude',custom_args:process.env.SOLO_E2E_PROVIDER==='codex'?codexE2EArgs:[],model_name:process.env.SOLO_E2E_MODEL??'haiku',system_prompt:workerPrompt,skills:[bundle("values=json.loads(task['description'])\nresult=sum(values)-2\n")]});agents.push(original.id);
  const receiver=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`SelectionReceiver${suffix}`,computer_id:computer.id,model_provider:process.env.SOLO_E2E_PROVIDER??'claude',custom_args:process.env.SOLO_E2E_PROVIDER==='codex'?codexE2EArgs:[],model_name:process.env.SOLO_E2E_MODEL??'haiku',system_prompt:'For introductions send RECEIVER_READY using solo message send and stop. For assigned receiver Tasks find the exact candidate submission in the incoming prompt, copy E1 content as the third argument, and run python3 .agents/skills/selection-check/scripts/check.py <channel UUID> <Task number> <exact candidate E1 JSON>. The installed script parses and verifies it then submits. Do not replace it with a claimed check.',skills:[bundle("received=json.loads(sys.argv[3])\nvalues=received['input']\nresult=received['result']\nassert sum(values)==result==42\n")]});agents.push(receiver.id);
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id IN ('${original.id}','${receiver.id}') AND status='completed'`),{timeout: runtimeTimeout}).toBe('2');
  await page.addInitScript(({access,refresh,workspaceID,userID})=>{localStorage.setItem('access_token',access);localStorage.setItem('refresh_token',refresh);localStorage.setItem('solo_active_workspace_id',workspaceID);localStorage.setItem(`solo_active_workspace_id:${userID}`,workspaceID);localStorage.setItem('solo.locale','zh-CN');},{access:auth.access_token,refresh:auth.refresh_token,workspaceID:workspace.id,userID:auth.user.id});
  const candidate=await api<{id:string}>('post',`/api/v1/agents/${original.id}/revisions`,{summary:`Fix sum ${suffix}`,config:{system_prompt:workerPrompt,model_provider:process.env.SOLO_E2E_PROVIDER??'claude',custom_args:process.env.SOLO_E2E_PROVIDER==='codex'?codexE2EArgs:[],model_name:process.env.SOLO_E2E_MODEL??'haiku',skills:[bundle("values=json.loads(task['description'])\nresult=sum(values)\n")]}});
  // Load this exact candidate into the real form using native directory upload.
  const {mkdtempSync,mkdirSync,writeFileSync,rmSync}=await import('node:fs');const {tmpdir}=await import('node:os');const {join}=await import('node:path');
  const folder=mkdtempSync(join(tmpdir(),'solo-selection-skill-'));mkdirSync(join(folder,'scripts'));
  for(const file of bundle("values=json.loads(task['description'])\nresult=sum(values)\n").files)writeFileSync(join(folder,file.path),Buffer.from(file.content,'base64'));
  try{
   await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);
   await page.getByText('高级',{exact:true}).click(); await page.getByText('手动改进',{exact:true}).click();
   const versions=page.getByLabel('手动改进配置',{exact:true});
   await versions.getByLabel('改进说明',{exact:true}).fill(`Fix sum ${suffix}`);
   await versions.getByLabel('加入 Skill 文件夹',{exact:true}).setInputFiles(folder);
   await versions.getByText('新旧版本对照',{exact:true}).click();
   const panel=page.locator('details[aria-label="新旧版本对照"]');
   await panel.getByLabel('问题与工作引用',{exact:true}).fill('Original arithmetic Task returned 40 for [21,21]; compare the actual outputs.');
   await selectValue(panel, '接收成员', receiver.id);
   await panel.getByLabel('每侧 Token 额度',{exact:true}).fill('2000000');
   await panel.getByLabel('接收检查',{exact:true}).fill('Execute the receiver script on the exact candidate E1 JSON. Parsing must succeed and input sum must equal reported result 42.');
   await panel.getByLabel('样例 1 title',{exact:true}).fill(`Actual arithmetic ${suffix}`);
   await panel.getByLabel('样例 1 input',{exact:true}).fill('[21,21]');
   await panel.getByLabel('样例 1 requirements',{exact:true}).fill('Execute the installed Python script on the fixed input and retain its actual numeric output without altering the implementation.');
   await panel.getByRole('button',{name:'开始固定条件对照',exact:true}).click();
   await expect(panel.getByRole('status')).toContainText('已固定对照计划');
   const selection=sql(`SELECT id::text FROM agent_selections WHERE agent_id='${original.id}' AND candidate_revision_id='${candidate.id}'`);expect(selection).toMatch(/^[0-9a-f-]{36}$/);
   const trials=JSON.parse(sql(`SELECT json_agg(json_build_object('agent_id',agent_id,'channel_id',channel_id)) FROM agent_selection_trials WHERE selection_id='${selection}'`)) as {agent_id:string;channel_id:string}[];
   agents.push(...trials.map((item)=>item.agent_id));trialChannels.push(...trials.map((item)=>item.channel_id));
   const item=panel.locator(`[data-selection-id="${selection}"]`);
   await panel.getByLabel('选择依据',{exact:true}).fill('Observe before all cases finish.');
   await item.getByRole('button',{name:'继续观察',exact:true}).click();await expect(item).toContainText('继续观察：Observe before all cases finish.');
   const blocked=await requestAuthenticated(request,base,auth,'post',`/api/v1/agents/${original.id}/revisions/${candidate.id}/publish`,{headers,data:{reason:'must reject premature publication'}});expect(blocked.status()).toBe(400);
   for(const [arm,result] of [['baseline',40],['candidate',42],['receiver',42]] as const){
    const taskID=sql(`SELECT task_id::text FROM agent_selection_tasks WHERE selection_id='${selection}' AND arm='${arm}'`);
    await expect.poll(()=>sql(`SELECT status FROM tasks WHERE id='${taskID}'`),{timeout: runtimeTimeout}).toBe('in_review');
    await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs r JOIN agent_run_task_links l ON l.run_id=r.id WHERE l.task_id='${taskID}' AND r.finished_at IS NULL`),{timeout: runtimeTimeout}).toBe('0');
    const delivered=JSON.parse(sql(`SELECT e->>'content' FROM tasks t JOIN task_submissions s ON s.id=t.current_submission_id CROSS JOIN LATERAL jsonb_array_elements(s.evidence) e WHERE t.id='${taskID}' AND e->>'id'='E1'`));
    const trialAgent=sql(`SELECT agent_id FROM agent_selection_trials WHERE selection_id='${selection}' AND arm='${arm}'`);
    expect(delivered.script).toContain(`/agents/${trialAgent}/workspace/.solo/revision-skills/`);
    expect(delivered.result).toBe(result);
    if(arm==='receiver'){
     const candidateEvidence=JSON.parse(sql(`SELECT e->>'content' FROM agent_selection_tasks st JOIN task_submissions s ON s.id=st.input_submission_id CROSS JOIN LATERAL jsonb_array_elements(s.evidence) e WHERE st.task_id='${taskID}' AND e->>'id'='E1'`));
     expect(delivered.received_candidate).toEqual(candidateEvidence);
    }
    await panel.getByRole('button',{name:'刷新对照结果',exact:true}).click();
    const row=item.locator('div').filter({has:page.locator(`a[href*="task=${taskID}"]`)}).first();
    await row.getByRole('button',{name:'查看交付与验收',exact:true}).click();
    const dialog=page.getByRole('dialog');await expect(dialog).toContainText(`ACTUAL_RESULT_${result}`);
    if(arm==='candidate'){
     await api('post',`/api/v1/tasks/${taskID}/observations`,{kind:'intervention',minutes:2.5,note:'E2E fixture: manual intervention record retained for compatibility',idempotency_key:'intervention-'+suffix});
     await api('post',`/api/v1/tasks/${taskID}/observations`,{kind:'comparison',category:`arithmetic-simple-${suffix}`,note:'Same fixed arithmetic input, model and lifetime budget.',idempotency_key:'cohort-'+suffix});
     await dialog.getByRole('button',{name:'重新读取',exact:true}).click();
     await dialog.getByText('验证记录与交付成本',{exact:true}).click();
     await expect(dialog.getByRole('region',{name:'交付成本',exact:true})).toContainText(`对照组：arithmetic-simple-${suffix}`);
     await expect(dialog.getByLabel('实际投入分钟',{exact:true})).toHaveCount(0);
     expect(sql(`SELECT sum(minutes)::text FROM task_observations WHERE task_id='${taskID}' AND kind='intervention'`)).toBe('2.5');
    }
    await dialog.getByRole('checkbox').check();
    await dialog.getByPlaceholder('说明检查方法与证据').fill(`Inspected actual script output ${result}; ${arm==='receiver'?'candidate JSON parsed and summed':'input and output retained'}.`);
    await dialog.getByLabel('审核结论与原因',{exact:true}).fill(`Actual ${arm} output verified.`);
    await dialog.getByRole('button',{name:'通过验收',exact:true}).click();
    await expect.poll(()=>sql(`SELECT status FROM tasks WHERE id='${taskID}'`)).toBe('done');
    await dialog.getByRole('button',{name:'关闭',exact:true}).click();
    await expect(dialog).not.toBeVisible();
    if(arm==='baseline'){
     await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs r JOIN agent_selection_trials tr ON tr.agent_id=r.agent_id WHERE tr.selection_id='${selection}' AND r.finished_at IS NULL`),{timeout: runtimeTimeout}).toBe('0');
     const daemonID=process.env.SOLO_E2E_DAEMON_ID;
     if(!daemonID?.startsWith('daemon-e2e-')||!process.env.SOLO_DAEMON_STATE_DIR||!process.env.SOLO_DAEMON_CREDENTIAL_FILE)throw new Error('Isolated make-managed stack required');
     await page.goto('about:blank');
     execFileSync('make',['rebuild','SOLO_DAEMON_PROFILE=',`DAEMON_ID=${daemonID}`,`SOLO_DAEMON_STATE_DIR=${process.env.SOLO_DAEMON_STATE_DIR}`,`SOLO_DAEMON_CREDENTIAL_FILE=${process.env.SOLO_DAEMON_CREDENTIAL_FILE}`],{cwd:join(process.cwd(),'..'),env:process.env,timeout:180000,stdio:'pipe'});
     await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${original.id}`);
     await page.getByText('高级',{exact:true}).click(); await page.getByText('手动改进',{exact:true}).click();
     await page.getByText('新旧版本对照',{exact:true}).click();
     await expect(item).toContainText(`Fix sum ${suffix}`);
    }
   }
   await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs r JOIN agent_selection_trials tr ON tr.agent_id=r.agent_id WHERE tr.selection_id='${selection}' AND r.finished_at IS NULL`),{timeout: runtimeTimeout}).toBe('0');
   await panel.getByRole('button',{name:'刷新对照结果',exact:true}).click();
   await panel.getByLabel('选择依据',{exact:true}).fill('Identical input and model: baseline 40, candidate 42; receiver independently parsed candidate evidence and verified 42.');
   const acceptedResponse=page.waitForResponse((response)=>response.url().endsWith(`/selections/${selection}/decide`)&&response.request().method()==='POST');
   await item.getByRole('button',{name:'接受',exact:true}).click();
   const accepted=await acceptedResponse;expect(accepted.ok(),await accepted.text()).toBeTruthy();
   await expect(item.getByRole('button',{name:'全局发布此候选',exact:true})).toBeEnabled();
   await item.getByRole('button',{name:'全局发布此候选',exact:true}).click();await expect(panel.getByRole('status')).toContainText('已发布');
   expect(sql(`SELECT (solo_agent_config(a)=v.config)::text FROM agents a JOIN agent_revisions v ON v.id='${candidate.id}' WHERE a.id='${original.id}'`)).toBe('true');
   expect(sql(`SELECT count(*) FROM agent_selection_decisions WHERE selection_id='${selection}' AND decision='accepted' AND jsonb_array_length(evidence->'trials')=3`)).toBe('1');
   expect(sql(`SELECT count(*) FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id JOIN task_submissions sub ON sub.id=t.current_submission_id JOIN agent_runs r ON r.id=sub.run_id JOIN agent_run_token_usage u ON u.run_id=r.id WHERE st.selection_id='${selection}' AND t.status='done' AND r.backend_started_at IS NOT NULL AND r.finished_at IS NOT NULL AND u.actual_tokens>0`)).toBe('3');
   expect(sql(`SELECT (receiver.input_submission_id=candidate.current_submission_id)::text FROM agent_selection_tasks receiver JOIN agent_selection_tasks ct ON ct.selection_id=receiver.selection_id AND ct.arm='candidate' JOIN tasks candidate ON candidate.id=ct.task_id WHERE receiver.selection_id='${selection}' AND receiver.arm='receiver'`)).toBe('true');
   await page.goto('/observability/insight');
   const cohort=page.getByTestId('delivery-cohorts').getByRole('row').filter({hasText:`arithmetic-simple-${suffix}`});
   await expect(cohort).toContainText('合格 1 / 1 个任务');
   await expect(cohort).toContainText('已记录人工 2.5 分钟');
   await expect(cohort).toContainText('可比样本 1');
   const delivery=await api<{delivery:{cohort:string;actual_tokens:number;human_minutes:number;qualified:number}[]}>('get','/api/v1/dashboard/insight');
   const measured=delivery.delivery.find((entry)=>entry.cohort===`arithmetic-simple-${suffix}`);expect(measured?.human_minutes).toBe(2.5);expect(measured?.qualified).toBe(1);expect(measured?.actual_tokens).toBeGreaterThan(0);
  }finally{rmSync(folder,{recursive:true,force:true});}
 }finally{await page.close();for(const id of agents.reverse())await api('delete',`/api/v1/agents/${id}`).catch(()=>undefined);for(const id of [...trialChannels,channel.id])await api('delete',`/api/v1/channels/${id}`).catch(()=>undefined);await computer.release(request);}
});
