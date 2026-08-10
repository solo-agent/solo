import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const suffix = Date.now().toString(36);
const credentials = { email: `budget-gate-${suffix}@solo.local`, password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface RunTokenState {
  run_id: string;
  status: string;
  budget_state: string;
  actual_tokens: number;
  reserved_tokens: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  reservation_count: number;
  active_reservations: number;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec',
    process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql',
    '-U', process.env.POSTGRES_USER ?? 'solo',
    '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA',
    '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const response = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Budget Gate E2E' },
  });
  if (!response.ok()) throw new Error(`register: ${response.status()} ${await response.text()}`);
  return response.json();
}

async function authenticatePage(page: Page, auth: AuthResponse) {
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    if (!localStorage.getItem('solo.locale')) localStorage.setItem('solo.locale', 'zh-CN');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
}

async function api<T>(request: APIRequestContext, token: string, method: 'post' | 'put' | 'delete', path: string, data?: unknown): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: { authorization: `Bearer ${token}` },
    data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

function latestRunState(agentID: string, messageContent?: string): RunTokenState {
  const triggerFilter = messageContent
    ? `AND r.trigger_message_id = (SELECT id FROM messages WHERE content = '${messageContent}' ORDER BY created_at DESC LIMIT 1)`
    : '';
  return databaseJSON<RunTokenState>(`
    SELECT COALESCE((
      SELECT json_build_object(
        'run_id', r.id::text,
        'status', r.status,
        'budget_state', COALESCE(c.state, ''),
        'actual_tokens', COALESCE(c.actual_tokens, 0),
        'reserved_tokens', COALESCE(c.reserved_tokens, 0),
        'input_tokens', COALESCE(c.input_tokens, 0),
        'output_tokens', COALESCE(c.output_tokens, 0),
        'cache_read_tokens', COALESCE(c.cache_read_tokens, 0),
        'cache_write_tokens', COALESCE(c.cache_write_tokens, 0),
        'reservation_count', (SELECT COUNT(*) FROM budget_reservations b WHERE b.run_id = r.id),
        'active_reservations', (SELECT COUNT(*) FROM budget_reservations b WHERE b.run_id = r.id AND b.state = 'active')
      )::text
        FROM agent_runs r
        LEFT JOIN agent_run_token_usage c ON c.run_id = r.id
       WHERE r.agent_id = '${agentID}'
         ${triggerFilter}
       ORDER BY r.started_at DESC
       LIMIT 1
    ), '{"run_id":"","status":"","budget_state":"","actual_tokens":0,"reserved_tokens":0,"input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":0,"reservation_count":0,"active_reservations":0}')
  `);
}

function usedAgentTokens(agentID: string): number {
  return databaseJSON<number>(`
    SELECT COALESCE(SUM(actual_tokens), 0)::text
      FROM agent_run_token_usage
     WHERE agent_id = '${agentID}' AND state = 'settled'
  `);
}

function blockedMessageState(agentID: string, content: string): { messages: number; runs: number } {
  return databaseJSON<{ messages: number; runs: number }>(`
    SELECT json_build_object('messages', COUNT(m.id), 'runs', COUNT(r.id))::text
      FROM messages m
      LEFT JOIN agent_runs r ON r.trigger_message_id = m.id AND r.agent_id = '${agentID}'
     WHERE m.content = '${content}'
  `);
}

test.describe('real usage budget gate', () => {
  test.skip(process.env.SOLO_E2E_REAL_BUDGET_GATE !== '1', 'requires the make-managed stack and authenticated local Agent runtime');
  test.setTimeout(300000);
  let localComputer: LocalComputerLease;

  test.beforeAll(async ({ request }) => {
    const auth = await authenticate(request);
    localComputer = await acquireLocalComputer(request, apiBase, auth.access_token);
  });

  test.afterAll(async ({ request }) => {
    await localComputer?.release(request);
  });

  test('saves Token limits, settles a real run, renders its Tokens, and blocks the next run', async ({ page, request }, testInfo) => {
    const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
    if (!login.ok()) throw new Error(`login: ${login.status()} ${await login.text()}`);
    const auth = await login.json() as AuthResponse;
    await authenticatePage(page, auth);

    let channel: Entity | null = null;
    let agent: Entity | null = null;
    try {
      channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
        name: `budget-e2e-${suffix}`,
        description: 'Real usage budget gate E2E',
      });
      agent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `BudgetE2E${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: 'Always answer through solo message send. When introducing yourself, send exactly READY. For any later human message, send exactly BUDGET_OK.',
      });

      await expect.poll(() => latestRunState(agent!.id).status, {
        timeout: 180000, intervals: [500, 1000, 2000],
      }).toMatch(/completed|failed/);

      const usedBeforeBudget = usedAgentTokens(agent.id);
      await api(request, auth.access_token, 'put', `/api/v1/agents/${agent.id}/budget`, {
        enabled: true,
        monthly_limit_tokens: usedBeforeBudget + 100_000,
      });

      await page.goto('/settings');
      const userBudget = page.getByTestId('user-budget-card');
      await expect(page.getByRole('heading', { name: 'Token 额度' })).toBeVisible();
      const budgetHeader = userBudget.getByTestId('budget-card-header');
      const themeColors = await budgetHeader.evaluate((element) => {
        const probe = document.createElement('div');
        probe.style.backgroundColor = 'var(--color-brutal-primary)';
        probe.style.color = 'var(--color-foreground)';
        document.body.appendChild(probe);
        const style = getComputedStyle(element);
        const colors = {
          skin: document.documentElement.dataset.skin,
          background: style.backgroundColor,
          foreground: style.color,
          expectedBackground: getComputedStyle(probe).backgroundColor,
          expectedForeground: getComputedStyle(probe).color,
        };
        probe.remove();
        return colors;
      });
      expect(themeColors.skin).toBe('archive');
      expect(themeColors.background).toBe(themeColors.expectedBackground);
      expect(themeColors.foreground).toBe(themeColors.expectedForeground);
      const contrast = await budgetHeader.evaluate((element) => {
        const pixel = (color: string) => {
          const canvas = document.createElement('canvas');
          canvas.width = 1;
          canvas.height = 1;
          const context = canvas.getContext('2d', { willReadFrequently: true });
          if (!context) return [0, 0, 0];
          context.fillStyle = color;
          context.fillRect(0, 0, 1, 1);
          return Array.from(context.getImageData(0, 0, 1, 1).data.slice(0, 3));
        };
        const luminance = (rgb: number[]) => {
          const channels = rgb.map((value) => {
            const normalized = value / 255;
            return normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
          });
          return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
        };
        const style = getComputedStyle(element);
        const foreground = luminance(pixel(style.color));
        const background = luminance(pixel(style.backgroundColor));
        return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
      });
      expect(contrast).toBeGreaterThanOrEqual(4.5);
      const budgetScreenshot = await userBudget.screenshot({ path: process.env.SOLO_E2E_BUDGET_SCREENSHOT });
      await testInfo.attach('token-budget-card', { body: budgetScreenshot, contentType: 'image/png' });
      await expect(userBudget).not.toContainText('$');
      await expect(userBudget).not.toContainText('美元');
      await userBudget.getByRole('switch').click();
      await userBudget.getByLabel('每月 Token 上限').fill('200');
      await expect(userBudget).toContainText('万 Token/月');
      await expect(userBudget).not.toContainText('每日 Token 上限');
      await expect(userBudget).not.toContainText('每次运行预留 Token');
      await userBudget.getByRole('button', { name: '保存' }).click();
      await expect(page.getByText('Token 额度已保存')).toBeVisible();

      const savedUserLimit = databaseJSON<number>(`
        SELECT monthly_limit_tokens::text
          FROM budget_policies
         WHERE owner_id = (SELECT id FROM users WHERE email = '${credentials.email}')
           AND scope_type = 'user'
      `);
      expect(savedUserLimit).toBe(2_000_000);

      await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
      await page.reload();
      await expect(page.getByRole('heading', { name: 'Token budget' })).toBeVisible();
      await expect(page.getByLabel('Monthly Token limit')).toHaveValue('200');
      await expect(page.getByTestId('user-budget-card')).toContainText('10K Token / month');
      await page.evaluate(() => localStorage.setItem('solo.locale', 'zh-CN'));
      await page.reload();

      await page.goto(`/dashboard?channel=${channel.id}`);
      await page.getByRole('button', { name: '频道成员' }).click();
      await page.getByRole('dialog').getByRole('button', { name: `查看 ${agent.name} 详情` }).click();
      const agentBudget = page.getByTestId('agent-budget-card');
      await expect(agentBudget.getByRole('heading', { name: 'Agent Token 额度' })).toBeVisible();
      await expect(agentBudget.getByLabel('每月 Token 上限')).toHaveValue(String((usedBeforeBudget + 100_000) / 10_000));
      await expect(agentBudget).not.toContainText('每日 Token 上限');
      await expect(agentBudget).not.toContainText('每次运行预留 Token');
      const agentBudgetScreenshot = await agentBudget.screenshot();
      await testInfo.attach('agent-token-budget-card', { body: agentBudgetScreenshot, contentType: 'image/png' });

      const firstMessage = `@${agent.name} BUDGET_E2E_FIRST_${suffix}`;
      await page.goto(`/dashboard?channel=${channel.id}`);
      const composer = page.getByPlaceholder('输入消息...');
      await composer.fill(firstMessage);
      await composer.press('Enter');

      await expect.poll(() => latestRunState(agent!.id, firstMessage), {
        timeout: 180000, intervals: [500, 1000, 2000],
      }).toMatchObject({
        run_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
        status: 'completed',
        budget_state: 'settled',
        reserved_tokens: 100_000,
        reservation_count: 2,
        active_reservations: 0,
        actual_tokens: expect.any(Number),
      });
      const settled = latestRunState(agent.id, firstMessage);
      expect(settled.actual_tokens).toBeGreaterThan(0);
      expect(settled.actual_tokens).toBe(
        settled.input_tokens + settled.output_tokens + settled.cache_read_tokens + settled.cache_write_tokens,
      );

      await page.goto(`/observability/live?run_id=${settled.run_id}`);
      await expect(page.getByText('本次 Token', { exact: true })).toBeVisible();
      await expect(page.getByText(/实际:/)).toBeVisible();

      const secondMessage = `@${agent.name} BUDGET_E2E_BLOCK_${suffix}`;
      await page.goto(`/dashboard?channel=${channel.id}`);
      await composer.fill(secondMessage);
      await composer.press('Enter');
      await expect(page.getByText(/剩余 Token 不足以开始下一次运行/)).toBeVisible({ timeout: 10000 });
      await expect.poll(() => blockedMessageState(agent!.id, secondMessage), {
        timeout: 10000, intervals: [500, 1000],
      }).toEqual({ messages: 1, runs: 0 });

      await page.evaluate(() => localStorage.setItem('solo.locale', 'en'));
      await page.reload();
      const thirdMessage = `@${agent.name} BUDGET_E2E_BLOCK_EN_${suffix}`;
      await page.getByPlaceholder('Type a message...').fill(thirdMessage);
      await page.getByPlaceholder('Type a message...').press('Enter');
      await expect(page.getByText(/monthly Token budget cannot cover another run/)).toBeVisible({ timeout: 10000 });
      await expect.poll(() => blockedMessageState(agent!.id, thirdMessage), {
        timeout: 10000, intervals: [500, 1000],
      }).toEqual({ messages: 1, runs: 0 });

      const policyState = databaseJSON<{ enabled: boolean; reservations: number }>(`
        SELECT json_build_object(
          'enabled', p.enabled,
          'reservations', (SELECT COUNT(*) FROM budget_reservations b WHERE b.policy_id = p.id)
        )::text
          FROM budget_policies p
         WHERE p.agent_id = '${agent.id}'
      `);
      expect(policyState).toEqual({ enabled: true, reservations: 1 });
    } finally {
      if (agent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${agent.id}`).catch(() => undefined);
      if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    }
  });
});
