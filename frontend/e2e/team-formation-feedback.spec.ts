import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { join } from 'node:path';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const runtimeTimeout = 300_000;
function sql(query: string) { return execFileSync('docker', ['exec', 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim(); }
test.use({ actionTimeout: 30000 });
type Formed = { channel_id: string; members: { id: string; name: string; ref: string; reused?: boolean; evidence?: { qualified: number; delegations: number } }[] };

test('Lucy forms teams through real CLI feedback and reuses the delivering member with its Memory', async ({ page, request }) => {
  test.setTimeout(900000);
  const suffix = Date.now().toString(36);
  const registered = await registerVerified(request, base, { data: { email: `formation-${suffix}@solo.local`, password: 'SoloE2E-2026!', display_name: '组队验收员' } });
  expect(registered.ok()).toBeTruthy(); const auth = await registered.json();
  sql(`UPDATE users SET onboarding_completed_at=now() WHERE id='${auth.user.id}'`);
  const headers: Record<string, string> = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'patch' | 'delete', path: string, data?: unknown): Promise<T> => { const response = await requestAuthenticated(request, base, auth, method, path, { headers, data }); if (!response.ok()) throw new Error(`${path}: ${response.status()} ${await response.text()}`); return response.status() === 204 ? undefined as T : response.json(); };
  const workspace = await api<{ id: string }>('post', '/api/v1/workspaces', { name: `Formation ${suffix}` }); headers['X-Workspace-ID'] = workspace.id;
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const lucyChannel = await api<{ id: string }>('get', '/api/v1/channels/lucy');
  const members = await api<{ member_type: string; member_id: string }[]>('get', `/api/v1/channels/${lucyChannel.id}/members`);
  let lucy = members.find((m) => m.member_type === 'agent')?.member_id;
  const agents = new Set<string>(); const channels: string[] = [];
  const fixture = mkdtempSync(join(tmpdir(), 'solo-formation-'));
  const scriptPath = join(fixture, 'form.py');
  const script = `import json,pathlib,subprocess,sys
channel,name=sys.argv[1:3]
root=pathlib.Path(__file__).resolve().parent
def cli(*args):
 p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=60)
 assert p.returncode==0,p.stdout+p.stderr
 return p.stdout
messages=json.loads(cli('message','read','--target',channel,'--limit','100'))['messages']
source=next(m['id'] for m in messages if m['sender_type']=='user' and 'FORM_RUN:'+name in m['content'])
cli('template','list','--json')
candidates=json.loads(cli('team','candidates','-c',channel,'-m',source,'--template','agency-dev-api-doc-gen'))
(root/('candidates-'+name+'.json')).write_text(json.dumps(candidates))
plan={'intent_summary':'Deliver a tiny verified arithmetic task with an accountable team','channel':{'name':name},'template_id':'agency-dev-api-doc-gen'}
path=root/('plan-'+name+'.json');path.write_text(json.dumps(plan))
result=json.loads(cli('team','form','-c',channel,'-m',source,'--plan',str(path),'--output','json'))
(root/('formation-'+name+'.json')).write_text(json.dumps(result))
cli('message','send','--target',channel,'-c','FORM_DONE:'+name)
print('FORM_CLI_EXECUTED',result['channel_id'])
`;
  writeFileSync(scriptPath, script);
  try {
    if (!lucy) { lucy = (await api<{ agent_id: string }>('post', '/api/v1/onboarding/create-lucy', { runtime_type: process.env.SOLO_E2E_PROVIDER ?? 'claude', computer_id: computer.id, channel_id: lucyChannel.id })).agent_id; }
    await api('patch', `/api/v1/agents/${lucy}`, { computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'haiku', system_prompt: `For FORM_RUN:<name>, run the existing script in the foreground with the incoming channel UUID and the name as arguments: python3 ${scriptPath} <channel UUID> <name>. This script performs real template discovery, candidate inspection and team formation, writes results and sends its own confirmation. Execute exactly once and finish the Run regardless of exit status. A failure is test evidence: do not repair, retry, inspect environment variables, search credentials or other Agent directories, or start/stop any service. After successful exit do not repeat any CLI or send another message. For a greeting only say LUCY_READY using solo message send.` });
    await page.addInitScript(({ access, refresh, workspaceID, userID }) => { localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo_active_workspace_id', workspaceID); localStorage.setItem(`solo_active_workspace_id:${userID}`, workspaceID); localStorage.setItem('solo.locale', 'zh-CN'); }, { access: auth.access_token, refresh: auth.refresh_token, workspaceID: workspace.id, userID: auth.user.id });
    const form = async (name: string): Promise<Formed> => {
      const source = await api<{ id: string }>('post', `/api/v1/channels/${lucyChannel.id}/messages`, { content: `FORM_RUN:${name} Use the authorized script to form this team for me.`, client_msg_id: crypto.randomUUID() });
      await expect.poll(() => sql(`SELECT count(*) FROM team_formations WHERE source_message_id='${source.id}' AND status='completed'`), { timeout: 240000 }).toBe('1');
      const result = JSON.parse(sql(`SELECT result::text FROM team_formations WHERE source_message_id='${source.id}'`)) as Formed;
      channels.push(result.channel_id); for (const member of result.members) agents.add(member.id);
      await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${lucy}' AND finished_at IS NULL`), { timeout: runtimeTimeout }).toBe('0');
      await page.goto(`/dashboard?channel=${lucyChannel.id}`);
      const card = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: `# ${name}`, exact: true }) });
      await expect(card).toBeVisible(); await card.getByText('成员选择依据', { exact: true }).click();
      await expect(card).toContainText(result.members[0].reused ? '复用已满足权限与工具要求' : '职责暂无合适复用成员');
      const actual = JSON.parse(readFileSync(join(fixture, `formation-${name}.json`), 'utf8')) as Formed;
      expect(actual.channel_id).toBe(result.channel_id);
      return result;
    };
    const first = await form(`formation-first-${suffix}`);
    expect(first.members).toHaveLength(2); expect(first.members.every((member) => !member.reused)).toBeTruthy();
    const worker = first.members[0]; const privateNote = `MEMORY_FORMATION_${suffix}`;
    await api('patch', `/api/v1/agents/${worker.id}`, { system_prompt: `For an assigned Task execute real Python asserting sum([21,21])==42. On the first Task create MEMORY.md with exact text ${privateNote}; on later Tasks read it and assert it still contains ${privateNote} before computing. Use solo task get to obtain version, then submit once using solo task submit --file with artifact_version arithmetic-42, handoff summary ACTUAL_42_MEMORY_OK, changes mentioning actual execution, risks and next steps. Include inline E1 JSON containing result 42 and the exact MEMORY.md content. Send ACTUAL_42_MEMORY_OK to the Task thread using solo message send, then finish the Run. Never review or repeat a successful submission.` });
    const deliver = async (channelID: string, name: string) => {
      const task = await api<{ id: string }>('post', `/api/v1/channels/${channelID}/tasks`, { title: name, assignee: worker.id, contract: { requirements: [{ id: 'R1', text: 'Execute the arithmetic assertion and preserve the exact private Memory sentinel.' }], gate: { kind: 'human', reviewer_id: auth.user.id, max_revisions: 3 } } });
      await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`), { timeout: 240000 }).toBe('in_review');
      await expect.poll(() => sql(`SELECT count(*) FROM agent_runs r JOIN agent_run_task_links l ON l.run_id=r.id WHERE l.task_id='${task.id}' AND r.finished_at IS NULL`), { timeout: runtimeTimeout }).toBe('0');
      await page.goto(`/dashboard?channel=${channelID}&view=task`); await page.getByRole('button', { name: '交付与验收', exact: true }).first().click();
      const dialog = page.getByRole('dialog'); await expect(dialog).toContainText('ACTUAL_42_MEMORY_OK'); await expect(dialog).toContainText(privateNote);
      await dialog.getByRole('checkbox').check(); await dialog.getByPlaceholder('说明检查方法与证据').fill('Inspected real Python output and unchanged Memory sentinel.'); await dialog.getByLabel('审核结论与原因', { exact: true }).fill('Actual execution and continuity verified.');
      await dialog.getByRole('button', { name: '通过验收', exact: true }).click(); await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`)).toBe('done');
      expect(sql(`SELECT count(*) FROM task_submissions sub JOIN agent_runs r ON r.id=sub.run_id JOIN agent_run_token_usage u ON u.run_id=r.id WHERE sub.task_id='${task.id}' AND r.backend_started_at IS NOT NULL AND u.actual_tokens>0`)).toBe('1');
    };
    await deliver(first.channel_id, `First actual delivery ${suffix}`);
    const second = await form(`formation-second-${suffix}`);
    expect(second.members.map((member) => member.id)).toEqual(first.members.map((member) => member.id)); expect(second.members.every((member) => member.reused)).toBeTruthy();
    expect(second.members[0].evidence?.qualified).toBe(1); expect(second.members[0].evidence?.delegations).toBeGreaterThan(0);
    expect(sql(`SELECT count(*) FROM agent_relationships r JOIN agent_relationship_proposals p ON p.relationship_id=r.id WHERE r.channel_id='${second.channel_id}' AND p.status='accepted'`)).toBe('1');
    await deliver(second.channel_id, `Reused actual delivery ${suffix}`);
    expect(readFileSync(join(homedir(), '.solo', 'agents', worker.id, 'workspace', 'MEMORY.md'), 'utf8')).toContain(privateNote);
    expect(sql(`SELECT count(DISTINCT r.session_id) FROM agent_runs r JOIN agent_run_task_links l ON l.run_id=r.id JOIN tasks t ON t.id=l.task_id WHERE r.agent_id='${worker.id}' AND t.channel_id IN ('${first.channel_id}','${second.channel_id}')`)).toBe('2');
  } finally {
    rmSync(fixture, { recursive: true, force: true });
    await page.close(); for (const id of agents) await api('delete', `/api/v1/agents/${id}`).catch(() => undefined);
    for (const id of channels) await api('delete', `/api/v1/channels/${id}`).catch(() => undefined);
    // Lucy belongs to this disposable Workspace; ending the workspace preserves
    // her history while preventing a later local test from waking her.
    await api('delete', `/api/v1/workspaces/${workspace.id}`).catch(() => undefined); await computer.release(request);
  }
});
