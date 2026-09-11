import { selectValue } from './support/select';
import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
test.use({ actionTimeout: 30_000 });
const runtimeTimeout = 300_000;
function sql(query: string) { return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim(); }

test('real Agent edits a managed worktree; Runtime verifies the exact commit without a reviewer model', async ({ page, request }, testInfo) => {
 test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 1_500_000 : 600_000);
 const suffix=Date.now().toString(36); const repository=mkdtempSync(join(tmpdir(),'solo-code-gate-'));
 const git=(...args: string[])=>execFileSync('git',args,{cwd:repository,encoding:'utf8'}).trim();
 git('init');writeFileSync(join(repository,'double.py'),'def double(n):\n    return n\n');git('add','.');git('-c','user.name=Solo E2E','-c','user.email=e2e@solo.local','commit','-m','base');const commit=git('rev-parse','HEAD');
 const email=`code-${suffix}@solo.local`;const registered=await registerVerified(request,base,{data:{email,password:'SoloE2E-2026!',display_name:'代码验收员'}});expect(registered.ok()).toBeTruthy();const auth=await registered.json();
 sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}'`);
 const headers={authorization:`Bearer ${auth.access_token}`};
 async function api<T>(method:'get'|'post'|'delete',path:string,data?:unknown):Promise<T>{const response=await requestAuthenticated(request,base,auth,method,path,{headers,data});if(!response.ok())throw new Error(`${path}: ${response.status()} ${await response.text()}`);return response.status()===204?undefined as T:response.json();}
 const computer=await acquireLocalComputer(request,base,auth.access_token);const channel=await api<{id:string}>('post','/api/v1/channels',{name:`code-${suffix}`});const agents:string[]=[];
 try{
  const author=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`Coder${suffix}`,computer_id:computer.id,model_provider:process.env.SOLO_E2E_PROVIDER??'claude',custom_args:process.env.SOLO_E2E_PROVIDER==='codex'?codexE2EArgs:[],model_name:process.env.SOLO_E2E_MODEL??'sonnet',system_prompt:'On introduction send CODE_AUTHOR_READY via solo message send then stop. For an assigned code Task read solo task get, call solo task worktree -n <number> -c <channel>. Work only in the returned path. Fix double.py so double(n) returns n * 2. Run actual assertions for -2, 0, 3, commit the change using git -c user.name=Solo -c user.email=solo@solo.local commit. Submit using solo task submit --file with expected_task_version, artifact_version equal to full git rev-parse HEAD, handoff summary/changes/risks/next_steps, inline evidence with source and actual command/output, unique idempotency_key. Do not review the task. Send CODE_SUBMITTED to its thread.'});agents.push(author.id);
  await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${author.id}' AND status='completed'`), { timeout: runtimeTimeout }).toBe('1');
  const reviewer=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`Gate${suffix}`,computer_id:computer.id,model_provider:process.env.SOLO_E2E_PROVIDER??'claude',custom_args:process.env.SOLO_E2E_PROVIDER==='codex'?codexE2EArgs:[],model_name:process.env.SOLO_E2E_MODEL??'sonnet',system_prompt:'On introduction send CODE_REVIEWER_READY via solo message send then stop. Do not claim or implement tasks.'});agents.push(reviewer.id);
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id IN ('${author.id}','${reviewer.id}') AND status='completed'`),{timeout: runtimeTimeout}).toBe('2');
  await page.addInitScript(({access,refresh})=>{localStorage.setItem('access_token',access);localStorage.setItem('refresh_token',refresh);localStorage.setItem('solo.locale','zh-CN');},{access:auth.access_token,refresh:auth.refresh_token});
  await page.goto(`/dashboard?channel=${channel.id}&view=task`);await page.getByRole('button',{name:/创建任务/}).first().click();const dialog=page.getByRole('dialog');
  await dialog.locator('#task-create-title').fill(`Code verified ${suffix}`);await selectValue(dialog, '负责人', author.id);await dialog.getByText('自定义验收',{exact:true}).click();await dialog.getByLabel('验收要求',{exact:true}).fill('double(-2)、double(0)、double(3) 返回两倍值。');await selectValue(dialog, '验收方式', 'code');await selectValue(dialog, '审核者', reviewer.id);await dialog.getByLabel('仓库绝对路径').fill(repository);await dialog.getByLabel('基准 Git commit').fill(commit);await dialog.getByLabel('验收命令 JSON').fill(JSON.stringify([{requirement_id:'R1',command:['python3','-c','from double import double; assert [double(n) for n in [-2,0,3]] == [-4,0,6]; print("CODE_GATE_PASSED")']}]));await dialog.getByRole('button',{name:'创建任务',exact:true}).click();await expect(dialog).toHaveCount(0);
  await expect.poll(()=>sql(`SELECT COALESCE((SELECT status FROM tasks WHERE channel_id='${channel.id}' AND title='Code verified ${suffix}'),'missing')`),{timeout:300_000}).toBe('done');
  const taskID=sql(`SELECT id::text FROM tasks WHERE channel_id='${channel.id}' AND title='Code verified ${suffix}'`);
  await expect.poll(()=>sql(`SELECT count(*) FROM agent_run_task_links l JOIN agent_runs r ON r.id=l.run_id WHERE l.task_id='${taskID}' AND l.role='related' AND r.source='code_gate' AND r.status='completed' AND r.session_id IS NULL`)).toBe('1');
  expect(sql(`SELECT count(*) FROM task_reviews r JOIN task_submissions s ON s.id=r.submission_id WHERE s.task_id='${taskID}' AND r.decision='accepted' AND r.evidence::text LIKE '%CODE_GATE_PASSED%'`)).toBe('1');
  expect(git('rev-parse','HEAD')).toBe(commit);expect(git('status','--porcelain')).toBe('');
  const artifact=sql(`SELECT artifact_version FROM task_submissions WHERE task_id='${taskID}'`);expect(artifact).toMatch(/^[0-9a-f]{40}$/);expect(artifact).not.toBe(commit);const submissionID=sql(`SELECT current_submission_id::text FROM tasks WHERE id='${taskID}'`);expect(git('rev-parse',`refs/solo/submissions/${submissionID}`)).toBe(artifact);
  await page.reload();await page.locator(`[data-task-id="${taskID}"]`).getByRole('button',{name:'查看成果',exact:true}).click();await page.getByRole('dialog').getByText('验证记录与交付成本',{exact:true}).click();await page.getByRole('dialog').locator('summary').filter({hasText:/^提交版本/}).first().click();await expect(page.getByRole('dialog').getByText('固定 Git 版本运行检查',{exact:false})).toBeVisible();await expect(page.getByRole('dialog').getByText(/accepted ·/)).toBeVisible();await expect(page.getByRole('dialog').getByText(/CODE_GATE_PASSED/).first()).toBeVisible();await testInfo.attach('code-gate-verified',{body:await page.screenshot(),contentType:'image/png'});
 }finally{for(const id of agents.reverse())await api('delete',`/api/v1/agents/${id}`).catch(()=>undefined);await api('delete',`/api/v1/channels/${channel.id}`).catch(()=>undefined);await computer.release(request);rmSync(repository,{recursive:true,force:true});}
});
