import { expect, type Page } from '@playwright/test';

// Use actual trial Agents and native acceptance dialogs when a publication test
// needs an accepted comparison. No rows, Runs or submissions are manufactured.
export async function acceptPairedSelection(options: {
  page: Page;
  api: <T>(method: 'get' | 'post', path: string, data?: unknown) => Promise<T>;
  sql: (query: string) => string;
  agentID: string;
  candidateID: string;
  receiverID: string;
  input: string;
  requirement: string;
  teamCheck: string;
  verifySubmission: (arm: 'baseline' | 'candidate' | 'receiver', taskID: string) => void;
  agents: string[];
  channels: string[];
}) {
  const { page, api, sql } = options;
  const selection = await api<{ id: string }>('post', `/api/v1/agents/${options.agentID}/selections`, {
    candidate_revision_id: options.candidateID, receiver_agent_id: options.receiverID,
    problem: 'Verify this candidate before publication and preserve the baseline for rollback.',
    change: options.requirement, token_budget: 2_000_000, idempotency_key: crypto.randomUUID(),
    cases: [{ title: 'Publication comparison', input: options.input, requirements: [{ id: 'R1', text: options.requirement }] }],
    team_check: options.teamCheck,
  });
  const trials = JSON.parse(sql(`SELECT json_agg(json_build_object('agent',agent_id,'channel',channel_id)) FROM agent_selection_trials WHERE selection_id='${selection.id}'`)) as { agent: string; channel: string }[];
  options.agents.push(...trials.map((trial) => trial.agent));
  options.channels.push(...trials.map((trial) => trial.channel));
  for (const arm of ['baseline', 'candidate', 'receiver'] as const) {
    const task = JSON.parse(sql(`SELECT json_build_object('id',t.id,'channel',t.channel_id)::text FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id='${selection.id}' AND st.arm='${arm}'`));
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`), { timeout: 300_000 }).toBe('in_review');
    await expect.poll(() => sql(`SELECT count(*) FROM agent_runs r JOIN agent_run_task_links l ON l.run_id=r.id WHERE l.task_id='${task.id}' AND r.finished_at IS NULL`), { timeout: 300_000 }).toBe('0');
    options.verifySubmission(arm, task.id);
    await page.goto(`/dashboard?channel=${task.channel}&view=task`);
    await page.locator(`[data-task-id="${task.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByText('验证记录与交付成本', { exact: true }).click();
    await dialog.locator('summary').filter({ hasText: /^提交版本/ }).first().click();
    await expect(dialog.getByText(/E1 ·|source ·/).first()).toBeVisible();
    await dialog.getByRole('checkbox').check();
    await dialog.getByPlaceholder('说明检查方法与证据').fill(`核对 ${arm} 的实际提交、执行结果和对应证据；接收方另行执行固定兼容性检查。`);
    await dialog.getByLabel('审核结论与原因', { exact: true }).fill('固定输入已实际执行，证据与当前提交一致。');
    await dialog.getByRole('button', { name: '通过验收', exact: true }).click();
    await expect.poll(() => sql(`SELECT status FROM tasks WHERE id='${task.id}'`)).toBe('done');
  }
  await api('post', `/api/v1/agents/${options.agentID}/selections/${selection.id}/decide`, { decision: 'accepted', reason: 'All three real trial deliveries were inspected; the receiver executed checks on the exact candidate submission.' });
  expect(sql(`SELECT count(*) FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id JOIN task_submissions s ON s.id=t.current_submission_id JOIN agent_runs r ON r.id=s.run_id JOIN agent_run_token_usage u ON u.run_id=r.id WHERE st.selection_id='${selection.id}' AND t.status='done' AND r.backend_started_at IS NOT NULL AND r.finished_at IS NOT NULL AND u.actual_tokens>0`)).toBe('3');
  expect(sql(`SELECT count(*) FROM agent_selection_decisions WHERE selection_id='${selection.id}' AND decision='accepted'`)).toBe('1');
  return selection.id;
}
