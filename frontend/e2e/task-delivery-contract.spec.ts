import { selectValue } from './support/select';
import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';
import { acceptPairedSelection } from './support/paired-selection';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const runtimeTimeout = 300_000;
test.use({ actionTimeout: 30_000 });
function sql(query: string) {
  return execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('real author submits evidence; human and independent Agent accept exact submissions', async ({ page, request }, testInfo) => {
  test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 2_400_000 : 1_500_000);
  const suffix = Date.now().toString(36);
  const email = `delivery-${suffix}@solo.local`;
  const registered = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: '交付验收员' } });
  expect(registered.ok()).toBeTruthy();
  const auth = await registered.json();
  const headers: Record<string,string> = { authorization: `Bearer ${auth.access_token}` };
  async function api<T>(method: 'get' | 'post' | 'patch' | 'delete', path: string, data?: unknown): Promise<T> {
    const result = await requestAuthenticated(request, base, auth, method, path, { headers, data });
    if (!result.ok()) throw new Error(`${method} ${path}: ${result.status()} ${await result.text()}`);
    return result.status() === 204 ? undefined as T : result.json();
  }
  const userId = sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}' RETURNING id` ).split('\n')[0];
  const scope = await api<{id:string}>('post','/api/v1/workspaces',{name:`Delivery ${suffix}`}); headers['X-Workspace-ID']=scope.id;
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const channel = await api<{ id: string }>('post', '/api/v1/channels', { name: `delivery-${suffix}` });
  const agents: string[] = [];
  const evaluationChannels: string[] = [];
  try {
    const author = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, {
      name: `Author${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet', custom_args:process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [],
      system_prompt: `You implement small tasks and submit versioned deliveries. On introduction only send AUTHOR_READY with solo message send and stop. For any task assigned to you, create double.py containing def double(n): return n * 2, execute Python assertions for -2, 0, 3, and report the actual command/output. Read the Task via solo task get, then submit with solo task submit --file. Include inline evidence named source containing the complete Python source and verification containing the actual executed command and output. artifact_version must identify this source (use its SHA256). Handoff must explain changes, risks and next steps. Use expected_task_version from GET and a unique idempotency_key. Never review your own task. After submission send DELIVERY_SUBMITTED in the task thread using solo message send.`,
    });
    agents.push(author.id);
  await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${author.id}' AND status='completed'`), { timeout: 240_000 }).toBe('1');
    const reviewer = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, {
      name: `Reviewer${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet', custom_args:process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [],
      system_prompt: 'On introduction only send REVIEWER_READY using solo message send. When dispatched a submission for review, independently execute the supplied Python source with assertions double(-2)==-4, double(0)==0, double(3)==6. Never implement or claim tasks. Record actual results via solo task review --file with accepted only if all requirements pass and each check references the submitted evidence. Send REVIEW_RECORDED after the review to the task thread.',
    });
    agents.push(reviewer.id);
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id IN ('${author.id}','${reviewer.id}') AND status='completed'`), { timeout: 240_000 }).toBe('2');
    await page.addInitScript(({ access, refresh, workspaceID, userID }) => {
      localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo.locale', 'zh-CN'); localStorage.setItem('solo_active_workspace_id',workspaceID); localStorage.setItem(`solo_active_workspace_id:${userID}`,workspaceID);
    }, { access: auth.access_token, refresh: auth.refresh_token, workspaceID:scope.id, userID:auth.user.id });
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await page.getByRole('button', { name: /创建任务/ }).first().click();
    const create = page.getByRole('dialog');
    await create.locator('#task-create-title').fill(`Double human ${suffix}`);
    await selectValue(create, '负责人', author.id);
    await create.getByText('自定义验收', { exact: true }).click();
    await create.getByLabel('验收要求', { exact: true }).fill('提供 double(n) 函数，负数、零、正数均返回两倍值，并附实际测试结果。');
    await selectValue(create, '验收方式', 'human');
    await create.getByRole('button', { name: '创建任务', exact: true }).click();
    await expect(create).toHaveCount(0);
    const readTask = () => sql(`SELECT COALESCE((SELECT json_build_object('id',id,'status',status,'version',version,'submission',current_submission_id)::text FROM tasks WHERE channel_id='${channel.id}' AND title='Double human ${suffix}'),'{}')`);
    await expect.poll(() => JSON.parse(readTask()).status, { timeout: 300_000, intervals: [1000, 2000, 5000] }).toBe('in_review');
    const humanTask = JSON.parse(readTask());
    expect(sql(`SELECT contract->'gate'->>'reviewer_id' FROM tasks WHERE id='${humanTask.id}'`)).toBe(userId);
    await page.reload();
    await page.getByRole('button', { name: '查看成果', exact: true }).first().click();
    const review = page.getByRole('dialog');
    await review.getByText('验证记录与交付成本', { exact: true }).click();
    await review.locator('summary').filter({ hasText: /^提交版本/ }).first().click();
    await expect(review.getByText('source ·', { exact: false })).toBeVisible();
    await expect(review.getByText('verification ·', { exact: false })).toBeVisible();
    await review.getByLabel('确认此项通过').check();
    await review.getByLabel('R1 检查依据').fill('已核对源代码与三个输入的实际执行输出。');
    await review.getByLabel('审核结论与原因').fill('要求和当前提交一致，边界输入测试通过。');
    await review.getByRole('button', { name: '通过验收', exact: true }).click();
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${humanTask.id}'`)).toBe('done');
    await page.reload();
    await page.locator(`[data-task-id="${humanTask.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await review.getByText('验证记录与交付成本', { exact: true }).click();
    await review.locator('summary').filter({ hasText: /^提交版本/ }).first().click();
    await expect(page.getByRole('dialog').getByText(/accepted ·/)).toBeVisible();
    expect(sql(`SELECT status FROM tasks WHERE id='${humanTask.id}'`)).toBe('done');
    expect(sql(`SELECT count(*) FROM task_reviews WHERE submission_id='${humanTask.submission}' AND decision='accepted' AND reviewer_id='${userId}'`)).toBe('1');
    await review.getByRole('button', { name: /关闭|Close/ }).first().click();

    const agentTask = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/tasks`, {
      title: `Double independent ${suffix}`, assignee: author.id,
      contract: { requirements: [{ id: 'R1', text: 'double(n) must return twice n for -2, 0 and 3, with independently executed checks.' }], gate: { kind: 'agent', reviewer_id: reviewer.id, max_revisions: 3 } },
    });
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${agentTask.id}'`), { timeout: 300_000, intervals: [1000, 2000, 5000] }).toBe('done');
    expect(sql(`SELECT count(*) FROM task_reviews r JOIN task_submissions s ON s.id=r.submission_id WHERE s.task_id='${agentTask.id}' AND r.reviewer_id='${reviewer.id}' AND r.decision='accepted'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM agent_run_task_links l JOIN agent_runs r ON r.id=l.run_id JOIN agent_sessions s ON s.id=r.session_id WHERE l.task_id='${agentTask.id}' AND l.role='related' AND r.agent_id='${reviewer.id}' AND s.external_session_id<>''`)).toBe('1');
    await page.reload();
    // The completed result and immutable history must remain visible after a reload.
    await expect(page.getByText(`Double independent ${suffix}`, { exact: true }).first()).toBeVisible();
    await page.locator(`[data-task-id="${agentTask.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await review.getByText('验证记录与交付成本', { exact: true }).click();
    await review.locator('summary').filter({ hasText: /^提交版本/ }).first().click();
    await expect(page.getByRole('dialog').getByText(/accepted ·/)).toBeVisible();

    await page.getByRole('dialog').getByRole('button', { name: /关闭|Close/ }).first().click();
    // A real automation inherits the contract and remains active after the author Run finishes.
    const loop = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/automations`, { name: `Contract loop ${suffix}`, task_title: 'Double automation', target_agent_id: author.id, schedule_type: 'daily', schedule_hour: 9, schedule_minute: 0, timezone: 'UTC', enabled: false, contract: { requirements: [{ id: 'R1', text: 'double(n) returns twice n with actual executed tests.' }], gate: { kind: 'human', reviewer_id: userId, max_revisions: 3 } } });
    const loopRun = await api<{ task_id: string }>('post', `/api/v1/channels/${channel.id}/automations/${loop.id}/run`);
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${loopRun.task_id}'`), { timeout: 300_000 }).toBe('in_review');
    expect(sql(`SELECT status FROM automation_runs WHERE task_id='${loopRun.task_id}'`)).toBe('running');
    const overlap = await requestAuthenticated(request, base, auth, 'post', `/api/v1/channels/${channel.id}/automations/${loop.id}/run`, { headers });
    expect(overlap.status()).toBe(409);
    await page.reload();
    await page.locator(`[data-task-id="${loopRun.task_id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await page.getByRole('dialog').getByLabel('确认此项通过').check();
    await page.getByRole('dialog').getByLabel('R1 检查依据').fill('实际执行的 Python 测试和源代码一致。');
    await page.getByRole('dialog').getByLabel('审核结论与原因').fill('本轮通过验收。');
    await page.getByRole('dialog').getByRole('button', { name: '通过验收', exact: true }).click();
    await expect.poll(() => sql(`SELECT status FROM automation_runs WHERE task_id='${loopRun.task_id}'`), { timeout: 60_000 }).toBe('completed');
    await api('delete', `/api/v1/channels/${channel.id}/automations/${loop.id}`);

    // Test a candidate on a separate real Agent, then publish only the exact accepted configuration.
    const original = await api<{ system_prompt: string; model_provider: string; model_name: string; custom_args?: string[] }>('get', `/api/v1/agents/${author.id}`);
    const config = { system_prompt: original.system_prompt + '\nEvery handoff summary must include REVISION_VERIFIED.', model_provider: original.model_provider, model_name: original.model_name, custom_args: original.custom_args ?? [] };
    const proposalSummary = `Verification marker ${suffix}`;
    await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${author.id}`);
    await page.getByText('高级', { exact: true }).click(); await page.getByText('手动改进', { exact: true }).click();
    const proposal = page.getByLabel('手动改进配置', { exact: true });
    await proposal.getByLabel('改进说明').fill(proposalSummary);
    await proposal.getByLabel('候选工作说明').fill(config.system_prompt);
    await proposal.getByLabel('评测要求').fill('Execute double(-2), double(0), double(3) checks; include REVISION_VERIFIED in the handoff summary.');
    await proposal.getByRole('button', { name: '创建独立评测', exact: true }).click();
    await expect(proposal.getByRole('status')).toContainText('已建立独立评测成员', { timeout: 30_000 });
    const candidate = { id: sql(`SELECT id::text FROM agent_revisions WHERE agent_id='${author.id}' AND summary='${proposalSummary}'`) };
    const evaluation = { id: sql(`SELECT evaluation_agent_id::text FROM agent_revisions WHERE id='${candidate.id}'`) }; agents.push(evaluation.id);
    const evaluationChannel = sql(`SELECT home_channel_id::text FROM agents WHERE id='${evaluation.id}'`); evaluationChannels.push(evaluationChannel);
    expect(evaluationChannel).not.toBe(channel.id);
    const evaluationTask = { id: sql(`SELECT id::text FROM tasks WHERE channel_id='${evaluationChannel}' AND title='评测：${proposalSummary}'`) };
    const unevaluated = await requestAuthenticated(request, base, auth, 'post', `/api/v1/agents/${author.id}/revisions/${candidate.id}/publish`, { headers, data: { reason: 'must fail before evaluation' } }); expect(unevaluated.status()).toBe(400);
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${evaluationTask.id}'`), { timeout: 300_000 }).toBe('in_review');
    expect(sql(`SELECT handoff->>'summary' LIKE '%REVISION_VERIFIED%' FROM task_submissions WHERE task_id='${evaluationTask.id}'`)).toBe('t');
    await page.goto(`/dashboard?channel=${evaluationChannel}&view=task`);
    await page.locator(`[data-task-id="${evaluationTask.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await page.getByRole('dialog').getByLabel('确认此项通过').check();
    await page.getByRole('dialog').getByLabel('R1 检查依据').fill('核对独立副本执行的三个 Python 输入和 REVISION_VERIFIED 交付标识。');
    await page.getByRole('dialog').getByLabel('审核结论与原因').fill('独立频道中的实际执行与固定要求一致。');
    await page.getByRole('dialog').getByRole('button', { name: '通过验收', exact: true }).click();
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${evaluationTask.id}'`)).toBe('done');
    const receiver=await api<{id:string}>('post',`/api/v1/channels/${channel.id}/agents`,{name:`DeliveryReceiver${suffix}`,computer_id:computer.id,model_provider:original.model_provider,model_name:original.model_name,custom_args:original.custom_args ?? [],system_prompt:'On introduction only send DELIVERY_RECEIVER_READY via solo message send. On an assigned receiver Task, take the complete source evidence from the exact candidate submission included in the incoming prompt. Save that exact source to double.py; execute actual Python assertions double(-2)==-4, double(0)==0, double(3)==6. Check that the incoming candidate Handoff includes REVISION_VERIFIED. Submit your own receiver Task using solo task submit --file, expected_task_version from solo task get, unique idempotency_key and artifact_version, Handoff with summary/changes/risks/next_steps, and evidence E1 containing the actual command and output plus the consumed candidate submission ID. Send RECEIVER_VERIFIED in your Task thread. Do not implement replacement source or review another Task.'}); agents.push(receiver.id);
    await expect.poll(()=>sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${receiver.id}' AND status='completed'`),{timeout: runtimeTimeout}).toBe('1');
    await acceptPairedSelection({page,api,sql,agentID:author.id,candidateID:candidate.id,receiverID:receiver.id,input:'Implement double(n), execute assertions for -2, 0 and 3, and submit the complete source and actual verification output.',requirement:'double(n) returns twice n for -2, 0 and 3. Execute real assertions and preserve source evidence and actual command/output.',teamCheck:'Execute the exact candidate source against -2, 0 and 3; verify its Handoff includes REVISION_VERIFIED, then submit the actual receiver result and candidate submission ID.',verifySubmission:(arm,taskID)=>{
     const evidence=JSON.parse(sql(`SELECT s.evidence FROM task_submissions s JOIN tasks t ON t.current_submission_id=s.id WHERE t.id='${taskID}'`)) as {id:string;content:string}[];
     if(arm==='receiver'){
      const candidateSubmission=sql(`SELECT input_submission_id FROM agent_selection_tasks WHERE task_id='${taskID}'`);
      expect(evidence.find(item=>item.id==='E1')?.content).toContain(candidateSubmission);
     }else{
      const source=evidence.find(item=>/def\s+double\s*\(/.test(item.content));
      expect(source).toBeDefined();
      // The approving human independently executes the submitted source.
      execFileSync('python3',['-c',`${source!.content}\nassert double(-2)==-4\nassert double(0)==0\nassert double(3)==6\n`],{timeout:5000});
      expect(evidence.some(item=>item!==source && /\bpython(?:3(?:\.\d+)?)?\b/i.test(item.content))).toBeTruthy();
     }
    },agents,channels:evaluationChannels});
    await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${author.id}`); await page.getByText('高级', { exact: true }).click(); await page.getByText('手动改进', { exact: true }).click();
    await page.locator(`[data-revision-id="${candidate.id}"]`).getByRole('button', { name: '全局发布通过评测的版本', exact: true }).click();
    await expect(proposal.getByRole('status')).toContainText('已更新正式成员配置');
    expect(sql(`SELECT system_prompt LIKE '%REVISION_VERIFIED%' FROM agents WHERE id='${author.id}'`)).toBe('t');
    expect(sql(`SELECT count(*) FROM agent_revision_publications WHERE revision_id='${candidate.id}' AND evaluation_task_id='${evaluationTask.id}'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM task_submissions s JOIN agent_runs r ON r.id=s.run_id JOIN agent_revisions v ON v.id=r.agent_revision_id WHERE s.task_id='${evaluationTask.id}' AND v.config_hash=(SELECT config_hash FROM agent_revisions WHERE id='${candidate.id}')`)).toBe('1');

    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await expect(page.getByRole('button', { name: '固定当前团队', exact: true })).toHaveCount(0);
    // Legacy callers can still request an explicit snapshot through the original API.
    await api('post', `/api/v1/channels/${channel.id}/team-versions`, { reason: '真实评测通过后固定团队' });
    const versionId = sql(`SELECT team_version_id::text FROM channels WHERE id='${channel.id}'`);
    await page.goto(`/dashboard?channel=${channel.id}#team-records`);
    await page.getByRole('dialog').getByText('真实评测通过后固定团队（当前）', { exact: true }).click();
    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: '导出 Lockfile', exact: true }).click();
    const exported = await download;
    expect(exported.suggestedFilename()).toContain(versionId);
    const exportedPath = await exported.path();
    expect(exportedPath).toBeTruthy();
    const exportedLockfile = JSON.parse(readFileSync(exportedPath!, 'utf8'));
    expect(exportedLockfile).toEqual(JSON.parse(sql(`SELECT lockfile FROM channel_team_versions WHERE id='${versionId}'`)));
    await testInfo.attach('exported-team-lockfile', { body: Buffer.from(JSON.stringify(exportedLockfile, null, 2)), contentType: 'application/json' });
    await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
    await api('patch', `/api/v1/agents/${author.id}`, { system_prompt: 'Only output UNPUBLISHED_CONFIG if invoked. Do not implement any task.' });
    const pinnedTask = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/tasks`, { title: `Pinned revision ${suffix}`, assignee: author.id, contract: { requirements: [{ id: 'R1', text: 'Execute double tests and put REVISION_VERIFIED in the handoff summary.' }], gate: { kind: 'agent', reviewer_id: reviewer.id, max_revisions: 3 } } });
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${pinnedTask.id}'`), { timeout: 300_000 }).toBe('done');
    expect(sql(`SELECT count(*) FROM task_submissions s JOIN agent_runs r ON r.id=s.run_id WHERE s.task_id='${pinnedTask.id}' AND r.agent_revision_id='${candidate.id}' AND r.team_version_id='${versionId}' AND s.handoff->>'summary' LIKE '%REVISION_VERIFIED%'`)).toBe('1');
    await page.getByRole('button', { name: '任务看板', exact: true }).click();
    await page.locator(`[data-task-id="${pinnedTask.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await expect(page.getByRole('dialog').getByText(/REVISION_VERIFIED/).first()).toBeVisible();
    await testInfo.attach('pinned-delivery-verified', { body: await page.screenshot(), contentType: 'image/png' });
    await testInfo.attach('delivery-database-state', { body: Buffer.from(sql(`SELECT json_agg(json_build_object('task_id',t.id,'status',t.status,'submission_id',s.id,'artifact_version',s.artifact_version,'handoff',s.handoff,'run_id',r.id,'revision_id',r.agent_revision_id,'team_version_id',r.team_version_id,'reviews',(SELECT json_agg(json_build_object('reviewer_id',v.reviewer_id,'decision',v.decision)) FROM task_reviews v WHERE v.submission_id=s.id))) FROM tasks t JOIN task_submissions s ON s.id=t.current_submission_id JOIN agent_runs r ON r.id=s.run_id WHERE t.id IN ('${humanTask.id}','${agentTask.id}','${pinnedTask.id}')`)), contentType: 'application/json' });
    await api('post', `/api/v1/channels/${channel.id}/team-versions`, { version_id: 'live', reason: 'Release test pin after verification' });
  } finally {
    for (const id of agents.reverse()) await api('delete', `/api/v1/agents/${id}`).catch(() => undefined);
    for (const id of evaluationChannels) await api('delete', `/api/v1/channels/${id}`).catch(() => undefined);
    await api('delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    await api('delete', `/api/v1/workspaces/${scope.id}`).catch(() => undefined);
    await computer.release(request);
  }
});
