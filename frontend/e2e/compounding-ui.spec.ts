import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { registerVerified } from './support/auth';
import { selectValue } from './support/select';

test.use({ actionTimeout: 15_000 });

const base = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
function sql(query: string) {
  return execFileSync('docker', ['exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres', 'psql', '-U', 'solo', '-d', process.env.POSTGRES_DB ?? 'solo', '-tA', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8' }).trim();
}

test('converged entries keep keyboard access, narrow layout and readonly persisted history', async ({ page, request }, testInfo) => {
  test.setTimeout(180_000);
  const suffix = Date.now().toString(36);
  const email = `ui-contract-${suffix}@solo.local`;
  const registered = await registerVerified(request, base, { data: { email, password: 'SoloE2E-2026!', display_name: '界面回归检查' } });
  expect(registered.ok()).toBeTruthy();
  const auth = await registered.json();
  sql(`UPDATE users SET onboarding_completed_at=now() WHERE email='${email}'`);
  const headers: Record<string, string> = { authorization: `Bearer ${auth.access_token}` };
  const workspaceResponse = await request.post(`${base}/api/v1/workspaces`, { headers, data: { name: `UI ${suffix}` } });
  expect(workspaceResponse.ok()).toBeTruthy();
  const workspace = await workspaceResponse.json();
  headers['X-Workspace-ID'] = workspace.id;
  const channelResponse = await request.post(`${base}/api/v1/channels`, { headers, data: { name: `ui-${suffix}` } });
  expect(channelResponse.ok()).toBeTruthy();
  const channel = await channelResponse.json();
  try {
    await page.addInitScript(({ tokens, workspaceID }) => {
      localStorage.setItem('access_token', tokens.access_token);
      localStorage.setItem('refresh_token', tokens.refresh_token);
      if (!localStorage.getItem('solo.locale')) localStorage.setItem('solo.locale', 'zh-CN');
      localStorage.setItem('solo_active_workspace_id', workspaceID);
      localStorage.setItem(`solo_active_workspace_id:${tokens.user.id}`, workspaceID);
    }, { tokens: auth, workspaceID: workspace.id });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await expect(page.getByRole('button', { name: '固定当前团队', exact: true })).toHaveCount(0);
    await page.getByLabel('频道更多', { exact: true }).click();
    await page.getByRole('button', { name: '团队记录', exact: true }).click();
    await expect(page.getByRole('dialog')).toContainText('版本历史');
    await expect(page.getByLabel('合作约定内容')).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath('team-records.png') });
    await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
    await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
    await page.reload();
    const workspacePanel = page.locator('#channel-workspace-panel');
    for (const width of [1280, 1024]) {
      await page.setViewportSize({ width, height: 900 });
      await workspacePanel.getByRole('button', { name: 'Automations', exact: true }).click();
      await expect(page).toHaveURL(/view=automation/);
      await workspacePanel.getByRole('button', { name: 'Thinking', exact: true }).click();
      await expect(page).toHaveURL(/view=thinking/);
      if (width === 1280) await page.screenshot({ path: testInfo.outputPath('workspace-tabs-english.png') });
    }
    await page.evaluate(() => localStorage.setItem('solo.locale', 'zh-CN'));
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`/dashboard?channel=${channel.id}&view=task`);
    await page.getByRole('button', { name: '创建任务', exact: true }).first().click();
    const dialog = page.getByRole('dialog');
    const requirements = dialog.getByLabel('验收要求', { exact: true });
    const text = `统一检查 ${'长内容需要在当前面板内自然换行。'.repeat(12)}`;
    await dialog.locator('#task-create-title').fill(`界面验证 ${suffix}`);
    await expect(requirements).not.toBeVisible();
    await dialog.getByText('自定义验收', { exact: true }).click();
    await requirements.fill(text);
    expect((await requirements.boundingBox())!.height).toBeGreaterThanOrEqual(80);
    const gate = dialog.getByRole('button', { name: '验收方式', exact: true });
    await gate.click();
    await expect(page.getByRole('listbox')).toBeVisible();
    await gate.press('Escape');
    await expect(page.getByRole('listbox')).toHaveCount(0);
    await expect(dialog).toBeVisible();
    await expect(gate).toBeFocused();
    await gate.press('ArrowDown');
    await gate.press('ArrowDown');
    await gate.press('Enter');
    await expect(page.getByRole('listbox')).toHaveCount(0);
    await selectValue(dialog, '验收方式', 'code');
    await expect(dialog.getByLabel('验收命令 JSON')).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath('code-contract-desktop.png') });
    await page.setViewportSize({ width: 390, height: 844 });
    await dialog.getByLabel('验收命令 JSON').scrollIntoViewIfNeeded();
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth + 1)).toBeTruthy();
    await page.screenshot({ path: testInfo.outputPath('code-contract-mobile.png') });
    await selectValue(dialog, '验收方式', 'human');
    await dialog.getByRole('button', { name: '创建任务', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    const persisted = JSON.parse(sql(`SELECT json_build_object('id',id,'contract',contract,'claimer_id',claimer_id) FROM tasks WHERE channel_id='${channel.id}' AND title='界面验证 ${suffix}'`));
    expect(persisted.contract.requirements).toEqual([{ id: 'R1', text }]);
    expect(persisted.contract.gate.kind).toBe('human');
    expect(persisted.contract.gate.reviewer_id).toBe(auth.user.id);
    expect(persisted.claimer_id).toBeNull();
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.locator(`[data-task-id="${persisted.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await dialog.getByText('验证记录与交付成本', { exact: true }).click();
    await expect(dialog.getByLabel('观察说明', { exact: true })).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: '记录观察', exact: true })).toHaveCount(0);
    const recorded = await request.post(`${base}/api/v1/tasks/${persisted.id}/observations`, { headers, data: { kind: 'comparison', category: 'UI／人工验收／长文本', note: '真实表单的键盘与窄屏检查已完成。', idempotency_key: `ui-${suffix}` } });
    expect(recorded.ok()).toBeTruthy();
    expect(sql(`SELECT category FROM task_observations WHERE task_id='${persisted.id}' AND kind='comparison'`)).toBe('UI／人工验收／长文本');
    await dialog.getByRole('button', { name: '重新读取', exact: true }).click();
    await expect(dialog.getByRole('region', { name: '交付成本', exact: true })).toContainText('对照组：UI／人工验收／长文本');
    await expect(dialog.getByRole('region', { name: '交付成本', exact: true })).toContainText('已记录人工投入 未记录');
    await page.screenshot({ path: testInfo.outputPath('delivery-desktop.png') });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth + 1)).toBeTruthy();
    await dialog.getByRole('button', { name: '重新读取' }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath('delivery-mobile.png') });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.reload();
    await page.locator(`[data-task-id="${persisted.id}"]`).getByRole('button', { name: '查看成果', exact: true }).click();
    await dialog.getByText('验证记录与交付成本', { exact: true }).click();
    await expect(dialog.getByText(/对照组：UI／人工验收／长文本/)).toBeVisible();
    expect(sql(`SELECT count(*) FROM agent_runs WHERE channel_id='${channel.id}'`)).toBe('0');
  } finally {
    const cleanup = await request.delete(`${base}/api/v1/channels/${channel.id}`, { headers }).catch(() => null);
    if (cleanup) expect(cleanup.ok()).toBeTruthy();
  }
});
