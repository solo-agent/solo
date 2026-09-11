import { codexE2EArgs } from './support/runtime';
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer } from './support/local-computer';
import { registerVerified, requestAuthenticated } from './support/auth';

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
test.use({ actionTimeout: 30_000 });
const runtimeTimeout = 300_000;
function sql(query: string) {
  return execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('real Runtime claims an ordinary message by short ID; legacy human review remains usable', async ({ page, request }) => {
  test.setTimeout(process.env.SOLO_E2E_PROVIDER === 'codex' ? 1_500_000 : 600_000);
  const suffix = Date.now().toString(36);
  const email = `claim-${suffix}@solo.local`;
  const registration = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: '认领验收员' } });
  const auth = await registration.json();
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const api = async <T>(method: 'get' | 'post' | 'delete', path: string, data?: unknown): Promise<T> => {
    const response = await requestAuthenticated(request, base, auth, method, path, { headers, data });
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined as T : response.json();
  };
  const ownerID = sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}' RETURNING id`).split('\n')[0];
  const computer = await acquireLocalComputer(request, base, auth.access_token);
  const channel = await api<{ id: string }>('post', '/api/v1/channels', { name: `claim-${suffix}` });
  let agentID = '';
  try {
    const agent = await api<{ id: string; name: string }>('post', `/api/v1/channels/${channel.id}/agents`, {
      name: `Claimer${suffix}`, computer_id: computer.id, model_provider: process.env.SOLO_E2E_PROVIDER ?? 'claude', custom_args: process.env.SOLO_E2E_PROVIDER === 'codex' ? codexE2EArgs : [], model_name: process.env.SOLO_E2E_MODEL ?? 'sonnet',
      system_prompt: 'On introduction send CLAIM_WORKER_READY with solo message send and stop. On a user request marked CLAIM_CHECK, use the received msg= short ID with solo task claim -m <short ID> -c <channel ID>. Repeat the same claim once to verify idempotence. Run python3 -c "assert 2+2==4; print(4)". Send CLAIM_CHECK_PASSED with solo message send to the task thread target returned by claim. Submit the same Task with solo task submit -n <number> -c <channel ID> (it is a legacy task without contract), then stop. Never create a second task, accept it yourself, or edit the test.',
    });
    agentID = agent.id;
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs WHERE agent_id='${agent.id}' AND status='completed'`), { timeout: runtimeTimeout }).toBe('1');
    await page.addInitScript(({ access, refresh }) => {
      localStorage.setItem('access_token', access); localStorage.setItem('refresh_token', refresh); localStorage.setItem('solo.locale', 'zh-CN');
    }, { access: auth.access_token, refresh: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    const source = await api<{ id: string }>('post', `/api/v1/channels/${channel.id}/messages`, { content: `@${agent.name} CLAIM_CHECK: Claim this ordinary message using its msg= short ID, repeat the same claim, actually verify 2+2 with Python, send CLAIM_CHECK_PASSED to the claim's Thread, and submit that Task for my review.`, client_msg_id: crypto.randomUUID() });
    const state = () => JSON.parse(sql(`SELECT COALESCE((SELECT json_build_object('id',id,'status',status,'claimer',claimer_id,'creator',creator_id)::text FROM tasks WHERE message_id='${source.id}'),'{}')`));
    await expect.poll(() => state().status, { timeout: 240_000 }).toBe('in_review');
    const task = state();
    expect(task.claimer).toBe(agent.id);
    expect(task.creator).toBe(ownerID);
    expect(sql(`SELECT count(*) FROM tasks WHERE message_id='${source.id}'`)).toBe('1');
    expect(sql(`SELECT count(*) FROM messages m JOIN threads th ON th.id=m.thread_id WHERE th.root_message_id='${source.id}' AND m.sender_id='${agent.id}' AND m.content LIKE '%CLAIM_CHECK_PASSED%'`)).toBe('1');
    // The card must arrive over the existing WS path, without a page reload.
    const card = page.locator(`[data-task-id="${task.id}"]:visible`);
    await expect(card).toBeVisible();
    await card.getByRole('button', { name: '通过', exact: true }).click();
    await expect.poll(() => state().status).toBe('done');
    await page.reload();
    await expect(page.locator(`[data-task-id="${task.id}"]:visible`).getByRole('button', { name: '重新打开', exact: true })).toBeVisible();
    expect(sql(`SELECT count(*) FROM task_reviews WHERE task_id='${task.id}' AND decision='accepted' AND reviewer_id='${ownerID}'`)).toBe('1');
  } finally {
    if (agentID) await api('delete', `/api/v1/agents/${agentID}`).catch(() => undefined);
    await api('delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    await computer.release(request);
  }
});
