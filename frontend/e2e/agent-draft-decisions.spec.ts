import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
test.use({ actionTimeout: 30_000 });
const runtimeTimeout = 300_000;
function sql(query: string) {
  return execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('real Runtime explicitly retains and discards held drafts without losing obligations', async ({ page, request }, testInfo) => {
  test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 1_500_000 : 600_000);
  const suffix = Date.now().toString(36);
  const email = `draft-${suffix}@solo.local`;
  const registration = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: '草稿验收员' } });
  const auth = await registration.json();
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'delete', path: string, data?: unknown): Promise<T> => {
    const response = await requestAuthenticated(request, base, auth, method, path, { headers, data });
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined as T : response.json();
  };
  sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}'`);
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const channel = await api<{ id: string }>('post', '/api/v1/channels', { name: `draft-${suffix}` });
  const directory = mkdtempSync(join(tmpdir(), 'solo-draft-decisions-'));
  const script = join(directory, 'check.py');
  // The real Runtime executes this client script; every command uses the actual
  // CLI, current-turn daemon credentials, API and PostgreSQL, without stubs.
  writeFileSync(script, `import json, os, pathlib, re, subprocess, time
root=pathlib.Path(${JSON.stringify(directory)})
channel=${JSON.stringify(channel.id)}
def stage(name):
 (root/name).write_text('ready')
 deadline=time.monotonic()+120
 while not (root/('continue-'+name)).exists():
  assert time.monotonic()<deadline, 'owner did not continue '+name
  time.sleep(0.2)
def cli(*args, ok=True):
 result=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
 assert (result.returncode==0)==ok, result.stdout+result.stderr
 return result.stdout
run=next(r['id'] for r in json.loads(cli('work','list'))['runs'] if r['channel_id']==channel)
req=root/'request.json'
req.write_text(json.dumps({'channel_id':channel,'description':'Follow up draft decision','next_action':'Owner will resolve separately','idempotency_key':'draft-'+run}))
cli('work','mark','--file',str(req))
stage('first')
held=cli('message','send','--target',channel,'-c','KEEP_THIS_DRAFT')
assert 'HELD:' in held, held
first=re.search(r'Seen up to sequence: (\\d+)',held).group(1)
stage('retain')
held=cli('message','send','--target',channel,'-c','KEEP_THIS_DRAFT','--keep-after-seq',first,'--freshness-reason','First correction permits the same answer')
assert 'HELD:' in held, held
latest=re.search(r'Seen up to sequence: (\\d+)',held).group(1)
assert latest!=first
cli('message','send','--target',channel,'-c','KEEP_THIS_DRAFT','--keep-after-seq',first,'--freshness-reason','stale decision',ok=False)
cli('message','send','--target',channel,'-c','KEEP_THIS_DRAFT','--keep-after-seq',latest,'--freshness-reason','Both corrections explicitly permit this unchanged answer')
stage('discard')
assert 'HELD:' in cli('message','send','--target',channel,'-c','DISCARD_CLI_DRAFT')
work=json.loads(cli('work','list'))
draft=next(d for d in work['drafts'] if d['run_id']==run)
req.write_text(json.dumps({'run_id':run,'sha256':draft['sha256'],'reason':'CLI explicitly abandons this superseded reply'}))
cli('work','discard','--file',str(req))
cli('work','discard','--file',str(req))
stage('ui')
assert 'HELD:' in cli('message','send','--target',channel,'-c','DISCARD_UI_DRAFT')
stage('ui-ready')
work=json.loads(cli('work','list'));draft=next(d for d in work['drafts'] if d['run_id']==run)
req.write_text(json.dumps({'run_id':run,'sha256':draft['sha256'],'reason':'Agent abandons the superseded draft after the original user correction'}))
cli('work','discard','--file',str(req))
assert any(m['description']=='Follow up draft decision' for m in json.loads(cli('work','list'))['marks'])
assert not any(d['run_id']==run for d in json.loads(cli('work','list'))['drafts'])
cli('message','send','--target',channel,'-c','DRAFT_DECISIONS_VERIFIED')
print('DRAFT_DECISIONS_VERIFIED')
`);
  let agentID = '';
  try {
    const agent = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, {
      name: `DraftWorker${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet',
      system_prompt: 'On introduction send DRAFT_WORKER_READY with solo message send and stop. When asked for the protocol check, execute the supplied Python file once with the real shell. It uses the current Run CLI credentials. Do not rewrite it, spawn background work, inspect unrelated files, claim a task, or fabricate its result. Let the file wait for the owner checkpoints; then stop after it completes.',
    });
    agentID = agent.id;
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${agent.id}' AND status='completed'`), { timeout: runtimeTimeout }).toBe('1');
    await api('post', `/api/v1/channels/${channel.id}/messages`, { content: `@${agent.name} Execute this protocol check now: python3 ${script}. This is an interactive API verification, not a Task to claim. The script waits for my corrections and readonly inspection, then sends its result.`, client_msg_id: crypto.randomUUID() });
    let runID = '';
    for (const [stage, content] of [['first', 'The original text KEEP_THIS_DRAFT is correct; explicitly confirm after reading.'], ['retain', 'I still want exactly KEEP_THIS_DRAFT; explicitly review this newer correction too.'], ['discard', 'Do not send DISCARD_CLI_DRAFT. Explicitly discard that draft.'], ['ui', 'Do not send DISCARD_UI_DRAFT. Review and discard it yourself; retain the separate obligation.']]) {
      await expect.poll(() => existsSync(join(directory, stage)), { timeout: runtimeTimeout }).toBeTruthy();
      runID = sql(`SELECT id::text FROM agent_runs WHERE agent_id='${agent.id}' AND finished_at IS NULL ORDER BY started_at DESC LIMIT 1`);
      expect(runID).toMatch(/^[0-9a-f-]{36}$/);
      await api('post', `/api/v1/channels/${channel.id}/messages`, { content, correction_of_run_id: runID, client_msg_id: crypto.randomUUID() });
      writeFileSync(join(directory, `continue-${stage}`), 'continue');
    }
    await expect.poll(() => existsSync(join(directory, 'ui-ready')), { timeout: 90_000 }).toBeTruthy();
    expect(sql(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}' AND content='KEEP_THIS_DRAFT' AND metadata->'freshness_resolution'->>'action'='retain' AND metadata->'freshness_resolution'->>'reason'='Both corrections explicitly permit this unchanged answer'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM messages WHERE channel_id='${channel.id}' AND content IN ('DISCARD_CLI_DRAFT','DISCARD_UI_DRAFT')`)).toBe('0');
    expect(sql(`SELECT count(*) FROM agent_run_events WHERE run_id='${runID}' AND type='visible_message_draft_discarded'`)).toBe('1');
    await page.addInitScript(({ access, refresh }) => {
      localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo.locale', 'zh-CN');
    }, { access: auth.access_token, refresh: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}&panel=agent&agent=${agent.id}`);
    await page.getByText('工作记录', { exact: true }).click();
    const draft = page.locator(`[data-draft-run-id="${runID}"]`);
    await draft.getByText('发送前暂存的草稿', { exact: true }).click();
    await expect(draft.getByText('DISCARD_UI_DRAFT', { exact: true })).toBeVisible();
    await expect(draft.getByRole('button', { name: '放弃这份草稿', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: '标记已处理', exact: true })).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath('draft-readonly.png') });
    writeFileSync(join(directory, 'continue-ui-ready'), 'continue');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_held_drafts WHERE run_id='${runID}'`)).toBe('0');
    expect(sql(`SELECT count(*) FROM agent_run_events WHERE run_id='${runID}' AND type='visible_message_draft_discarded'`)).toBe('2');
    expect(sql(`SELECT count(*) FROM agent_work_marks WHERE agent_id='${agent.id}' AND status='open'`)).toBe('1');
    await expect.poll(() => sql(`SELECT status FROM agent_runs WHERE id='${runID}'`), { timeout: 90_000 }).toBe('completed');
    await page.goto(`/dashboard?channel=${channel.id}`);
    await expect(page.getByText('DRAFT_DECISIONS_VERIFIED', { exact: true }).first()).toBeVisible();
    expect(sql(`SELECT count(*) FROM agent_run_events WHERE run_id='${runID}' AND type='visible_message_held'`)).toBe('4');
    expect(sql(`SELECT count(*) FROM agent_message_consumptions WHERE run_id='${runID}'`)).toBe('4');
  } finally {
    if (agentID) await api('delete', `/api/v1/agents/${agentID}`).catch(() => undefined);
    await api('delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    await computer.release(request);
    rmSync(directory, { recursive: true, force: true });
  }
});
