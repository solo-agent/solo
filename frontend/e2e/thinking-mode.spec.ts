import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const password = 'SoloE2E-2026!';
const daemonLogPath = join(process.cwd(), '..', 'daemon.log');

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

function providerSessionIDForNode(nodeID: string): string {
  return databaseJSON<{ provider_session_id: string }>(`
    SELECT json_build_object('provider_session_id', session.external_session_id)::text
      FROM thinking_nodes node
      JOIN agent_sessions session ON session.id = node.agent_session_id
     WHERE node.id = '${nodeID}'
  `).provider_session_id;
}

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Entity {
  id: string;
  name: string;
}

interface ThinkingNode {
  id: string;
  parent_id?: string;
  agent_id?: string;
  agent_session_id?: string;
  title: string;
  source: 'root' | 'team' | 'manual' | 'auto';
  checkpoint_handoff?: string;
  checkpoint_handoff_at?: string;
  checkpoint_status: 'missing' | 'fresh' | 'stale' | 'final';
  inherited_handoff?: string;
  fork_handoff_pending: boolean;
  fork_handoff_at?: string;
  returned_handoff?: string;
  returning_at?: string;
  returned_at?: string;
}

interface ThinkingSpace {
  id: string;
  channel_id: string;
  nodes: ThinkingNode[];
}

interface MessageList {
  messages: Array<{ id: string; content: string; content_type?: string; thinking_node_id?: string; sender_type: string; sender_id: string }>;
}

interface ThreadMessageList {
  messages: Array<{ content: string; sender_type: string; sender_id: string }>;
}

interface AgentRun {
  agent_id: string;
  channel_id?: string;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const credentials = { email: 'thinking-e2e@solo.local', password };
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();

  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Thinking E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

async function api<T>(
  request: APIRequestContext,
  token: string,
  method: 'get' | 'post' | 'delete',
  path: string,
  data?: unknown,
): Promise<T> {
  const response = await request[method](`${apiBase}${path}`, {
    headers: { authorization: `Bearer ${token}` },
    data,
  });
  if (!response.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${response.status()} ${await response.text()}`);
  if (response.status() === 204) return undefined as T;
  return response.json();
}

function flowNode(page: Page, title: string) {
  const exactTitle = new RegExp(`^${title.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`);
  return page.locator('.react-flow__node').filter({ has: page.locator('p').filter({ hasText: exactTitle }) });
}

async function expectReadableFlowNode(page: Page, title: string) {
  const node = flowNode(page, title);
  await expect(node).toBeVisible();
  await expect.poll(async () => {
    const box = await node.boundingBox();
    const flowBox = await page.locator('.react-flow').boundingBox();
    if (!box || !flowBox || box.width < 100) return false;
    const inside = box.x >= flowBox.x
      && box.y >= flowBox.y
      && box.x + box.width <= flowBox.x + flowBox.width
      && box.y + box.height <= flowBox.y + flowBox.height;
    if (!inside) return false;
    return node.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const hit = document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
      return hit === element || element.contains(hit);
    });
  }, { timeout: 10000, intervals: [100, 200, 500] }).toBe(true);
}

async function expectNoCrossingEdges(page: Page) {
  const crossings = await page.locator('.react-flow__edge path').evaluateAll((paths) => {
    const segments = paths.flatMap((path) => {
      const values = (path.getAttribute('d') ?? '').match(/-?\d+(?:\.\d+)?/g)?.map(Number) ?? [];
      return values.length >= 4 ? [{ a: { x: values[0], y: values[1] }, b: { x: values[2], y: values[3] } }] : [];
    });
    const side = (a: { x: number; y: number }, b: { x: number; y: number }, c: { x: number; y: number }) =>
      (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x);
    let count = 0;
    for (let i = 0; i < segments.length; i++) {
      for (let j = i + 1; j < segments.length; j++) {
        const first = segments[i];
        const second = segments[j];
        const p1 = side(first.a, first.b, second.a);
        const p2 = side(first.a, first.b, second.b);
        const p3 = side(second.a, second.b, first.a);
        const p4 = side(second.a, second.b, first.b);
        if (p1 * p2 < -0.001 && p3 * p4 < -0.001) count++;
      }
    }
    return count;
  });
  expect(crossings).toBe(0);
}

async function nodeMessages(request: APIRequestContext, token: string, channelID: string, nodeID: string) {
  return api<MessageList>(request, token, 'get',
    `/api/v1/channels/${channelID}/messages?limit=100&thinking_node_id=${nodeID}`);
}

async function expectThinkingProcessClosed(nodeID: string, force: boolean) {
  await expect.poll(() => {
    try {
      return readFileSync(daemonLogPath, 'utf8').split('\n').some((line) =>
        (line.includes('"msg":"session: closing"')
          || (force && line.includes('"msg":"session: force-closing"')))
        && line.includes(`"session_key":"thinking:${nodeID}"`)
        && (line.includes(`"force":${force}`) || force));
    } catch {
      return false;
    }
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

async function expectProviderProcessEnded(sessionID: string, force: boolean) {
  const message = force ? 'claude: persistent session ended' : 'claude: persistent session closed';
  await expect.poll(() => {
    try {
      return readFileSync(daemonLogPath, 'utf8').split('\n').some((line) =>
        line.includes(`"msg":"${message}"`)
        && line.includes(`"session_id":"${sessionID}"`));
    } catch {
      return false;
    }
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

async function expectThinkingProcessSlept(nodeID: string) {
  await expect.poll(() => {
    try {
      return readFileSync(daemonLogPath, 'utf8').split('\n').some((line) =>
        line.includes('"msg":"session: sleeping idle Thinking process"')
        && line.includes(`"session_key":"thinking:${nodeID}"`));
    } catch {
      return false;
    }
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

async function expectThinkingSessionResumed(nodeID: string, sessionID: string) {
  await expect.poll(() => {
    try {
      return readFileSync(daemonLogPath, 'utf8').split('\n').some((line) =>
        line.includes('"msg":"session: creating"')
        && line.includes(`"session_key":"thinking:${nodeID}"`)
        && line.includes(`"resume":"${sessionID}"`));
    } catch {
      return false;
    }
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(true);
}

async function waitForReadyFork(
  request: APIRequestContext,
  token: string,
  channelID: string,
  nodeID: string,
) {
  await expect.poll(async () => {
    const space = await api<ThinkingSpace>(request, token, 'get', `/api/v1/channels/${channelID}/thinking`);
    const node = space.nodes.find((candidate) => candidate.id === nodeID);
    return Boolean(node && !node.fork_handoff_pending && node.fork_handoff_at && node.inherited_handoff?.includes('# Handoff'));
  }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(true);
}

async function waitForAgentMessage(
  page: Page,
  request: APIRequestContext,
  token: string,
  channelID: string,
  nodeID: string,
  agentID: string,
  content: string,
  timeout = 60000,
) {
  await expect.poll(async () => {
    const list = await nodeMessages(request, token, channelID, nodeID);
    return list.messages.some((message) => message.sender_type === 'agent'
      && message.sender_id === agentID && message.content === content);
  }, { timeout, intervals: [1000, 2000, 3000] }).toBe(true);
  await expect(page.getByLabel('Message list').getByText(content, { exact: true })).toBeVisible();
}

async function sendNodeInstruction(page: Page, instruction: string) {
  const composer = page.getByPlaceholder('Explore this branch...');
  await composer.fill(instruction);
  await composer.press('Enter');
}

async function sendNodeInstructionAndWait(
  page: Page,
  request: APIRequestContext,
  token: string,
  channelID: string,
  nodeID: string,
  agentID: string,
  instruction: string,
  content: string,
) {
  let lastError: unknown;
  for (let attempt = 0; attempt < 3; attempt++) {
    const retry = attempt === 0
      ? instruction
      : `${instruction}\nE2E retry ${attempt}: use the solo message send command; do not answer with plain assistant text.`;
    await sendNodeInstruction(page, retry);
    try {
      await waitForAgentMessage(page, request, token, channelID, nodeID, agentID, content);
      return;
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError;
}

async function sendNodeInstructionAndObserveActivity(
  page: Page,
  request: APIRequestContext,
  token: string,
  channelID: string,
  nodeID: string,
  siblingNodeID: string,
  agentID: string,
  instruction: string,
  content: string,
) {
  const node = page.getByTestId(`rf__node-${nodeID}`);
  const sibling = page.getByTestId(`rf__node-${siblingNodeID}`);
  await expect(node.locator('[data-agent-activity-kind]')).toHaveCount(0, { timeout: 10000 });
  await expect(sibling.locator('[data-agent-activity-kind]')).toHaveCount(0, { timeout: 10000 });

  await sendNodeInstruction(page, instruction);
  await expect(node.locator('[data-agent-activity-kind="human"]')).toBeVisible({ timeout: 60000 });
  await expect(node.locator('[data-agent-activity-kind="activity"]')).toBeVisible({ timeout: 60000 });
  await expect(node.locator('.team-agent-active-halo')).toBeVisible({ timeout: 60000 });
  await expect.poll(() => node.locator('.team-agent-active-halo').evaluate((element) =>
    getComputedStyle(element, '::before').animationName), {
    timeout: 10000,
  }).toBe('team-agent-halo-pulse');
  await expect(node.locator('[data-agent-activity-kind="tool"]')).toBeVisible({ timeout: 120000 });

  // The parent and child intentionally reuse the same FE Agent. Activity must
  // remain scoped to the exact Thinking run/node, never to every node by agent_id.
  await expect(sibling.locator('[data-agent-activity-kind]')).toHaveCount(0);
  await waitForAgentMessage(page, request, token, channelID, nodeID, agentID, content, 180000);
}

async function sendAutoSplitAndWait(
  page: Page,
  request: APIRequestContext,
  token: string,
  channelID: string,
  instruction: string,
  title: string,
) {
  let lastError: unknown;
  for (let attempt = 0; attempt < 3; attempt++) {
    const retry = attempt === 0
      ? instruction
      : `${instruction}\nE2E retry ${attempt}: use the solo message send command; do not answer with plain assistant text.`;
    await sendNodeInstruction(page, retry);
    try {
      await expect.poll(async () => {
        const space = await api<ThinkingSpace>(request, token, 'get', `/api/v1/channels/${channelID}/thinking`);
        return space.nodes.some((node) => node.title === title && node.source === 'auto');
      }, { timeout: 60000, intervals: [1000, 2000, 3000] }).toBe(true);
      return;
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError;
}

test('Thinking mode uses real local Agent sessions end to end', async ({ page, request }) => {
  const auth = await authenticate(request);
  const suffix = Date.now().toString(36);
  const channelName = `thinking-e2e-${suffix}`;
  const manualTitle = `Manual-${suffix}`;
  const emptyTitle = `Empty-${suffix}`;
  const autoTitle = `Auto-${suffix}`;
  const rootAck = `ROOT_ACK_${suffix}`;
  const legacyRootAck = `LEGACY_ROOT_ACK_${suffix}`;
  const feFirst = `FE_FIRST_${suffix}`;
  const feContinued = `${feFirst}_CONTINUED`;
  const childAck = `CHILD_ACK_${suffix}`;
  const childLater = `CHILD_LATER_${suffix}`;
  const autoAck = `AUTO_ACK_${suffix}`;
  const siblingAck = `SIBLING_SEEN_${autoAck}`;
  const returnedAck = `RETURNED_SEEN_${childAck}`;
  const normalAck = `NORMAL_AGENT_ACK_${suffix}`;
  const threadAck = `THREAD_AGENT_ACK_${suffix}`;
  const agents: Entity[] = [];
  let channel: Entity | null = null;

  try {
    channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
      name: channelName,
      description: 'Real Thinking mode E2E data',
    });

    for (const role of ['Lead', 'FE', 'BE', 'QA']) {
      agents.push(await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
        name: `${role}-${suffix}`,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: `You are the ${role} in a real Solo end-to-end test. When a user message starts with E2E:, follow it literally and immediately, including any explicitly requested Bash tool step. Never skip, simulate, background, or replace that tool step. Send the requested visible payload using solo message send to the incoming message's target. If the instruction explicitly requests a second hidden Handoff protocol message, send it separately after the visible payload. Do not add explanations, acknowledgements, punctuation, or any other visible message. When Solo asks for a final Thinking handoff, first run the foreground Bash command sleep 8, wait for it to finish, then follow the requested handoff format exactly and include the branch's concrete payloads.`,
      }));
    }
    const [lead, fe, be, qa] = agents;
    for (const child of [fe, be, qa]) {
      await api(request, auth.access_token, 'post', '/api/v1/agent-relationships', {
        from_agent_id: lead.id,
        to_agent_id: child.id,
        rel_type: 'assigns_to',
      });
    }

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });

    await page.goto(`/dashboard?channel=${channel.id}`);
    await page.getByRole('button', { name: 'Thinking', exact: true }).last().click();
    await expect(page).toHaveURL(/view=thinking/);
    await expect(page.getByRole('button', { name: 'Thinking', exact: true, pressed: true })).toHaveCount(2);
    await expect(flowNode(page, channelName)).toBeVisible();
    await expect(flowNode(page, fe.name)).toBeVisible();
    await expect(flowNode(page, be.name)).toBeVisible();
    await expect(flowNode(page, qa.name)).toBeVisible();
    await expectNoCrossingEdges(page);

    let space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const root = space.nodes.find((node) => !node.parent_id);
    const feNode = space.nodes.find((node) => node.agent_id === fe.id);
    const beNode = space.nodes.find((node) => node.agent_id === be.id);
    expect(root).toMatchObject({ title: channelName, source: 'root', agent_id: lead.id });
    expect(space.nodes.filter((node) => node.parent_id === root?.id && node.source === 'team')).toHaveLength(3);
    expect(feNode).toBeTruthy();
    expect(beNode).toBeTruthy();

    await expect.poll(async () => {
      const activeRuns = await api<AgentRun[]>(request, auth.access_token, 'get', '/api/v1/agent-runs/active');
      return (activeRuns ?? []).some((run) => agents.some((agent) => agent.id === run.agent_id)
        && run.channel_id === channel!.id);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(false);

    // Splitting an untouched parent must not wake its Agent or invent a
    // Handoff from unrelated workspace memory.
    await page.getByTestId(`rf__node-${beNode!.id}`).click();
    await page.getByRole('button', { name: 'Split', exact: true }).click();
    await page.getByPlaceholder('Name this branch').fill(emptyTitle);
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    await expectReadableFlowNode(page, emptyTitle);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const emptyNode = space.nodes.find((node) => node.title === emptyTitle)!;
    expect(emptyNode).toMatchObject({ parent_id: beNode!.id, agent_id: be.id, fork_handoff_pending: false, checkpoint_status: 'missing' });
    expect(emptyNode.inherited_handoff).toBeFalsy();
    expect(emptyNode.agent_session_id).toBeFalsy();
    await expect(page.getByPlaceholder('Explore this branch...')).toBeEnabled();
    const emptyContext = page.locator(`[data-thinking-node-context="${emptyNode.id}"]`);
    await emptyContext.getByRole('button').first().click();
    await expect(emptyContext).toContainText('No parent conversation context was available');
    const emptyPersisted = databaseJSON<{ parent_run_count: number; child_message_count: number }>(`
      SELECT json_build_object(
        'parent_run_count', (SELECT COUNT(*) FROM agent_runs run WHERE run.thinking_node_id = child.parent_id),
        'child_message_count', (SELECT COUNT(*) FROM messages message WHERE message.thinking_node_id = child.id)
      )::text
        FROM thinking_nodes child
       WHERE child.id = '${emptyNode.id}'
    `);
    expect(emptyPersisted).toEqual({ parent_run_count: 0, child_message_count: 0 });
    await page.getByTestId(`rf__node-${root!.id}`).click();

    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, root!.id, lead.id,
      `E2E: send exactly ${rootAck}`, rootAck);

    // Reproduce a long-lived Agent invoking an older solo binary that does
    // not forward SOLO_NODE_ID. Runtime-owned scope must keep the reply in
    // Root and out of the normal channel conversation.
    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, root!.id, lead.id,
      `E2E: use Bash to run exactly env -u SOLO_NODE_ID solo message send -c '${legacyRootAck}' --target '${channel.id}'. Do not run any other communication command.`,
      legacyRootAck);
    const channelAfterLegacyRoot = await api<MessageList>(request, auth.access_token, 'get',
      `/api/v1/channels/${channel.id}/messages?limit=100`);
    expect(channelAfterLegacyRoot.messages.some((message) => message.content === legacyRootAck)).toBe(false);

    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, feNode!.id, fe.id,
      `E2E: send exactly ${feFirst}`, feFirst);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const feSessionID = space.nodes.find((node) => node.id === feNode!.id)?.agent_session_id;
    expect(feSessionID).toBeTruthy();
    const injectedPrompt = readFileSync(join(
      homedir(), '.solo', 'agents', fe.id, 'workspace', '.solo', 'system-prompt.md',
    ), 'utf8');
    expect(injectedPrompt).toContain('## Initial role');
    expect(injectedPrompt).toContain('## Thinking Runtime');
    expect(injectedPrompt.indexOf('## Thinking Runtime')).toBeGreaterThan(injectedPrompt.indexOf('## Initial role'));

    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, feNode!.id, fe.id,
      `E2E: remember your immediately previous exact payload and send exactly ${feContinued}`, feContinued);
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    expect(space.nodes.find((node) => node.id === feNode!.id)?.agent_session_id).toBe(feSessionID);

    const feMessages = await nodeMessages(request, auth.access_token, channel.id, feNode!.id);
    const beMessages = await nodeMessages(request, auth.access_token, channel.id, beNode!.id);
    expect(feMessages.messages.some((message) => message.content === feFirst && message.sender_id === fe.id)).toBe(true);
    expect(beMessages.messages.some((message) => message.content.includes(feFirst))).toBe(false);

    await expectReadableFlowNode(page, be.name);
    await flowNode(page, be.name).click();
    await expect(page).toHaveURL(new RegExp(`node=${beNode!.id}`));
    await expect(page.getByLabel('Message list').getByText(feFirst, { exact: true })).toHaveCount(0);
    await expectReadableFlowNode(page, fe.name);
    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    await expect(page.getByLabel('Message list').getByText(feFirst, { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Split', exact: true }).click();
    await page.getByPlaceholder('Name this branch').fill(manualTitle);
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    await expectReadableFlowNode(page, manualTitle);
    await expectNoCrossingEdges(page);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const manualNode = space.nodes.find((node) => node.title === manualTitle)!;
    expect(manualNode).toMatchObject({ parent_id: feNode!.id, source: 'manual', agent_id: fe.id });
    expect(manualNode.agent_session_id).toBeFalsy();
    expect(manualNode.fork_handoff_pending).toBe(true);
    await expect(page).toHaveURL(new RegExp(`node=${manualNode.id}`));
    await expect(page.getByPlaceholder('Explore this branch...')).toBeDisabled();
    await waitForReadyFork(request, auth.access_token, channel.id, manualNode.id);
    await expect(page.getByPlaceholder('Explore this branch...')).toBeEnabled({ timeout: 30000 });

    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    const blockedParentReturn = page.getByRole('button', { name: 'Return', exact: true });
    await expect(blockedParentReturn).toBeDisabled();
    await expect(blockedParentReturn).toHaveAttribute('title', 'Return every child branch before returning this branch');
    const blockedParentResponse = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/thinking/nodes/${feNode!.id}/return`,
      { headers: { authorization: `Bearer ${auth.access_token}` } },
    );
    expect(blockedParentResponse.status()).toBe(409);
    expect(await blockedParentResponse.text()).toContain('all child nodes must be returned first');
    await page.getByTestId(`rf__node-${manualNode.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${manualNode.id}`));

    await sendNodeInstructionAndObserveActivity(
      page,
      request,
      auth.access_token,
      channel.id,
      manualNode.id,
      feNode!.id,
      fe.id,
      `E2E: first use the Bash tool to run the foreground command sleep 8. Wait for it to finish. Then send visible payload exactly ${childAck}. After that send a second hidden protocol message beginning exactly [[handoff:checkpoint]], followed by Markdown # Handoff headings that include ${childAck} as a confirmed conclusion.`,
      childAck,
    );
    await expect.poll(async () => {
      const current = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
      return current.nodes.find((node) => node.id === manualNode.id)?.checkpoint_handoff;
    }, { timeout: 60000, intervals: [1000, 2000, 3000] }).toContain(childAck);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const checkpointedChild = space.nodes.find((node) => node.id === manualNode.id)!;
    expect(checkpointedChild.checkpoint_status).toBe('fresh');
    const checkpointBeforeRefresh = checkpointedChild.checkpoint_handoff_at;
    const childSessionID = checkpointedChild.agent_session_id;
    expect(childSessionID).toBeTruthy();
    expect(childSessionID).not.toBe(feSessionID);
    const childProviderSessionID = providerSessionIDForNode(manualNode.id);
    expect(childProviderSessionID).toBeTruthy();

    const childContext = page.locator(`[data-thinking-node-context="${manualNode.id}"]`);
    await childContext.getByRole('button').first().click();
    await expect(childContext.locator('[data-handoff-kind="inherited"]')).toBeVisible();
    await expect(childContext.locator('[data-handoff-kind="active"]')).toContainText(childAck);
    await expect(childContext).toContainText('Current');

    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, manualNode.id, fe.id,
      `E2E: send exactly ${childLater}. Do not send any hidden Handoff protocol message.`, childLater);
    await expect.poll(async () => {
      const current = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
      return current.nodes.find((node) => node.id === manualNode.id)?.checkpoint_status;
    }, { timeout: 30000, intervals: [500, 1000, 2000] }).toBe('stale');
    await expect(childContext).toContainText('Needs refresh');
    await childContext.getByRole('button', { name: 'Refresh Current State' }).click();
    await expect(childContext).toContainText('Refreshing…');
    await expect.poll(async () => {
      const current = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
      const refreshed = current.nodes.find((node) => node.id === manualNode.id);
      return refreshed?.checkpoint_status === 'fresh'
        && refreshed.checkpoint_handoff_at !== checkpointBeforeRefresh
        && refreshed.checkpoint_handoff?.includes(childLater);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(true);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    expect(space.nodes.find((node) => node.id === manualNode.id)?.agent_session_id).toBe(childSessionID);
    await expect(childContext).toContainText('Current');

    await expect(page.getByRole('button', { name: 'Return', exact: true })).toBeEnabled({ timeout: 30000 });
    await page.getByRole('button', { name: 'Return', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Returning…', exact: true })).toBeVisible();
    await expect(page.getByPlaceholder('Explore this branch...')).toBeDisabled();
    await expect.poll(async () => {
      const current = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
      return current.nodes.find((node) => node.id === manualNode.id)?.returned_at;
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBeTruthy();
    await expectThinkingProcessClosed(manualNode.id, false);
    await expectProviderProcessEnded(childProviderSessionID, false);
    await expect(page).toHaveURL(new RegExp(`node=${manualNode.id}`));
    await expect(page.getByLabel('Message list').getByText(childAck, { exact: true }).first()).toBeVisible();
    await expect(page.getByPlaceholder('Explore this branch...')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Returned', exact: true })).toBeDisabled();
    const rejectedSplit = await request.post(
      `${apiBase}/api/v1/channels/${channel.id}/thinking/nodes/${manualNode.id}/children`,
      {
        headers: { authorization: `Bearer ${auth.access_token}` },
        data: { title: `Rejected-${suffix}` },
      },
    );
    expect(rejectedSplit.status()).toBe(409);
    expect(await rejectedSplit.text()).toContain('returned thinking nodes are closed');
    const rejectedMessage = await request.post(`${apiBase}/api/v1/channels/${channel.id}/messages`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
      data: { content: `REJECTED_${suffix}`, thinking_node_id: manualNode.id },
    });
    expect(rejectedMessage.status()).toBe(409);
    expect(await rejectedMessage.text()).toContain('returned thinking nodes are closed');
    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    await page.getByTestId(`rf__node-${manualNode.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${manualNode.id}`));
    await expect(page.getByLabel('Message list').getByText(childAck, { exact: true }).first()).toBeVisible();
    await expect(page.getByPlaceholder('Explore this branch...')).toBeDisabled();
    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    const handoffMessage = page.getByRole('listitem').filter({ hasText: `Handoff returned from ${manualTitle}:` });
    await expect(handoffMessage).toContainText(childAck);
    await expect(handoffMessage.getByRole('heading', { name: 'Handoff', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Return', exact: true })).toBeEnabled();
    const parentMessages = await nodeMessages(request, auth.access_token, channel.id, feNode!.id);
    const persistedHandoff = parentMessages.messages.find((message) =>
      message.content.startsWith(`Handoff returned from ${manualTitle}:`));
    expect(persistedHandoff?.content_type).toBe('thinking_handoff');
    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, feNode!.id, fe.id,
      'E2E: inspect the returned child Handoffs. Find the full value beginning CHILD_ACK_. Send one payload formed by concatenating the literal prefix RETURNED_SEEN_ and that full value.', returnedAck);

    await sendAutoSplitAndWait(page, request, auth.access_token, channel.id,
      `E2E: first send exactly two visible lines using one solo message send command. First line: ${autoAck}. Second line: [[split: ${autoTitle}]]. After that send a second hidden protocol message beginning exactly [[handoff:checkpoint]], followed by Markdown # Handoff headings that include ${autoAck} as a confirmed conclusion.`, autoTitle);
    await expectReadableFlowNode(page, autoTitle);
    await expectNoCrossingEdges(page);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const autoNode = space.nodes.find((node) => node.title === autoTitle)!;
    await waitForReadyFork(request, auth.access_token, channel.id, autoNode.id);
    await expect.poll(async () => {
      const current = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
      return current.nodes.find((node) => node.id === feNode!.id)?.checkpoint_handoff;
    }, { timeout: 60000, intervals: [1000, 2000, 3000] }).toContain(autoAck);

    await expectReadableFlowNode(page, be.name);
    await flowNode(page, be.name).click();
    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, beNode!.id, be.id,
      'E2E: inspect the sibling Handoffs. Find the full value beginning AUTO_ACK_. Send one payload formed by concatenating the literal prefix SIBLING_SEEN_ and that full value.', siblingAck);

    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    expect(space.nodes.find((node) => node.title === autoTitle)).toMatchObject({ parent_id: feNode!.id, source: 'auto', agent_id: fe.id });
    const returnedManual = space.nodes.find((node) => node.id === manualNode.id)!;
    expect(returnedManual.returning_at).toBeFalsy();
    expect(returnedManual.returned_at).toBeTruthy();
    expect(returnedManual.returned_handoff).toContain('# Handoff');
    expect(returnedManual.returned_handoff).toContain(childAck);
    expect(returnedManual.checkpoint_status).toBe('final');
    const persistedReturn = databaseJSON<{
      checkpoint: string;
      handoff: string;
      handoff_count: number;
      session_isolated: boolean;
      fork_ready: boolean;
      protocol_message_count: number;
    }>(`
      SELECT json_build_object(
        'checkpoint', child.checkpoint_handoff,
        'handoff', child.returned_handoff,
        'handoff_count', (
          SELECT COUNT(*) FROM messages message
           WHERE message.thinking_node_id = child.parent_id
             AND message.content_type = 'thinking_handoff'
             AND message.content LIKE 'Handoff returned from ${manualTitle}:%'
        ),
        'session_isolated', child.agent_session_id IS DISTINCT FROM parent.agent_session_id,
        'fork_ready', child.fork_handoff_pending = false
          AND child.fork_handoff_at IS NOT NULL
          AND child.inherited_handoff LIKE '# Handoff%',
        'protocol_message_count', (
          SELECT COUNT(*) FROM messages message
           WHERE message.channel_id = child_space.channel_id
             AND message.content LIKE '[[handoff:%'
        )
      )::text
        FROM thinking_nodes child
        JOIN thinking_nodes parent ON parent.id = child.parent_id
        JOIN thinking_spaces child_space ON child_space.id = child.space_id
       WHERE child.id = '${manualNode.id}'
         AND child.returned_at IS NOT NULL
    `);
    expect(persistedReturn.checkpoint).toContain(childLater);
    expect(persistedReturn.handoff).toContain(childAck);
    expect(persistedReturn.handoff_count).toBe(1);
    expect(persistedReturn.session_isolated).toBe(true);
    expect(persistedReturn.fork_ready).toBe(true);
    expect(persistedReturn.protocol_message_count).toBe(0);

    await page.getByTestId(`rf__node-${feNode!.id}`).click();
    await expect(page).toHaveURL(new RegExp(`node=${feNode!.id}`));
    await page.reload();
    await expect(flowNode(page, manualTitle)).toBeVisible();
    await expect(flowNode(page, autoTitle)).toBeVisible();
    await expect(page.getByLabel('Message list').getByText(returnedAck, { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Teams', exact: true }).click();
    await expect(page).not.toHaveURL(/view=thinking/);
    const channelComposer = page.getByPlaceholder('Type a message...');
    await expect(channelComposer).toBeVisible();
    const normalMessage = `E2E: send exactly ${normalAck}`;
    await channelComposer.fill(normalMessage);
    await channelComposer.press('Enter');
    await expect(page.getByLabel('Message list').getByText(normalMessage, { exact: true })).toBeVisible();
    await expect.poll(async () => {
      const list = await api<MessageList>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/messages?limit=100`);
      return list.messages.some((message) => message.sender_type === 'agent'
        && message.sender_id === lead.id && !message.thinking_node_id && message.content === normalAck);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(true);
    await expect(page.getByLabel('Message list').getByText(normalAck, { exact: true })).toBeVisible();
    await expect.poll(async () => {
      const activeRuns = await api<AgentRun[]>(request, auth.access_token, 'get', '/api/v1/agent-runs/active');
      return (activeRuns ?? []).some((run) => run.agent_id === lead.id && run.channel_id === channel.id);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(false);

    const normalMessages = await api<MessageList>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/messages?limit=100`);
    const normalRoot = normalMessages.messages.find((message) => message.sender_type === 'user' && message.content === normalMessage);
    expect(normalRoot).toBeTruthy();
    const normalRootItem = page.getByRole('listitem').filter({ hasText: normalMessage }).first();
    await normalRootItem.hover();
    await normalRootItem.locator('button[title^="Reply to"]').click();
    const threadComposer = page.getByLabel('Thread reply input');
    await threadComposer.fill(`E2E: send exactly ${threadAck}`);
    await threadComposer.press('Enter');
    await expect.poll(async () => {
      const list = await api<ThreadMessageList>(request, auth.access_token, 'get',
        `/api/v1/channels/${channel.id}/messages/${normalRoot!.id}/thread`);
      return list.messages.some((message) => message.sender_type === 'agent'
        && message.sender_id === lead.id && message.content === threadAck);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(true);
    await expect(page.getByText(threadAck, { exact: true })).toBeVisible();
    await expect.poll(async () => {
      const activeRuns = await api<AgentRun[]>(request, auth.access_token, 'get', '/api/v1/agent-runs/active');
      return (activeRuns ?? []).some((run) => run.agent_id === lead.id && run.channel_id === channel.id);
    }, { timeout: 180000, intervals: [1000, 2000, 3000] }).toBe(false);

    const cleanupSpace = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const cleanupNodes = [root!.id, feNode!.id, beNode!.id].map((nodeID) => {
      const node = cleanupSpace.nodes.find((candidate) => candidate.id === nodeID)!;
      expect(node.agent_session_id).toBeTruthy();
      return { node, providerSessionID: providerSessionIDForNode(node.id) };
    });
    const archivedChannelID = channel.id;
    await api(request, auth.access_token, 'delete', `/api/v1/channels/${archivedChannelID}`);
    channel = null;
    const archivedThinking = await request.get(`${apiBase}/api/v1/channels/${archivedChannelID}/thinking`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
    expect(archivedThinking.status()).toBe(404);
    for (const cleanup of cleanupNodes) {
      await expectThinkingProcessClosed(cleanup.node.id, true);
      await expectProviderProcessEnded(cleanup.providerSessionID, true);
    }
    const persistedArchive = databaseJSON<{ archived: boolean; node_count: number }>(`
      SELECT json_build_object(
        'archived', channel.is_archived,
        'node_count', COUNT(node.id)
      )::text
        FROM channels channel
        JOIN thinking_spaces space ON space.channel_id = channel.id
        JOIN thinking_nodes node ON node.space_id = space.id
       WHERE channel.id = '${archivedChannelID}'
       GROUP BY channel.is_archived
    `);
    expect(persistedArchive.archived).toBe(true);
    expect(persistedArchive.node_count).toBeGreaterThanOrEqual(6);
  } finally {
    await page.close();
    if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    for (const member of agents.reverse()) {
      await api(request, auth.access_token, 'delete', `/api/v1/agents/${member.id}`).catch(() => undefined);
    }
  }
});

test('Thinking idle runtime sleeps and resumes the real provider session', async ({ page, request }) => {
  test.skip(process.env.SOLO_E2E_EXPECT_IDLE_REAPER !== '1', 'requires a short-TTL daemon started through make rebuild');

  const auth = await authenticate(request);
  const suffix = `idle-${Date.now().toString(36)}`;
  const firstAck = `IDLE_FIRST_${suffix}`;
  const resumedAck = `IDLE_RESUMED_${suffix}`;
  let channel: Entity | null = null;
  let runtimeAgent: Entity | null = null;

  try {
    channel = await api<Entity>(request, auth.access_token, 'post', '/api/v1/channels', {
      name: `thinking-${suffix}`,
      description: 'Real short-TTL Thinking runtime E2E data',
    });
    runtimeAgent = await api<Entity>(request, auth.access_token, 'post', `/api/v1/channels/${channel.id}/agents`, {
      name: `Runtime-${suffix}`,
      model_provider: 'claude',
      model_name: 'sonnet',
      system_prompt: 'You are a real Solo idle lifecycle test Agent. When a message starts with E2E:, immediately send exactly the requested payload using solo message send to the incoming target. Do not send any other visible text.',
    });

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);
    await page.getByRole('button', { name: 'Thinking', exact: true }).last().click();
    await expect(flowNode(page, channel.name)).toBeVisible({ timeout: 30000 });

    let space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const root = space.nodes.find((node) => !node.parent_id)!;
    expect(root.agent_id).toBe(runtimeAgent.id);
    await flowNode(page, channel.name).click();
    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, root.id, runtimeAgent.id,
      `E2E: send exactly ${firstAck}`, firstAck);
    await expect.poll(async () => {
      const activeRuns = await api<AgentRun[]>(request, auth.access_token, 'get', '/api/v1/agent-runs/active');
      return (activeRuns ?? []).some((run) => run.agent_id === runtimeAgent!.id && run.channel_id === channel!.id);
    }, { timeout: 180000, intervals: [500, 1000, 2000] }).toBe(false);

    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    const nodeSessionID = space.nodes.find((node) => node.id === root.id)?.agent_session_id;
    expect(nodeSessionID).toBeTruthy();
    const providerSessionID = providerSessionIDForNode(root.id);
    expect(providerSessionID).toBeTruthy();
    await expectThinkingProcessSlept(root.id);
    await expectProviderProcessEnded(providerSessionID, false);

    const sleepingState = databaseJSON<{ session_id: string; returned: boolean; first_message_count: number }>(`
      SELECT json_build_object(
        'session_id', node.agent_session_id,
        'returned', node.returned_at IS NOT NULL,
        'first_message_count', (
          SELECT COUNT(*) FROM messages message
           WHERE message.thinking_node_id = node.id
             AND message.content = '${firstAck}'
        )
      )::text
        FROM thinking_nodes node
       WHERE node.id = '${root.id}'
    `);
    expect(sleepingState.session_id).toBe(nodeSessionID);
    expect(sleepingState.returned).toBe(false);
    expect(sleepingState.first_message_count).toBe(1);

    await sendNodeInstructionAndWait(page, request, auth.access_token, channel.id, root.id, runtimeAgent.id,
      `E2E: send exactly ${resumedAck}`, resumedAck);
    await expectThinkingSessionResumed(root.id, providerSessionID);
    space = await api<ThinkingSpace>(request, auth.access_token, 'get', `/api/v1/channels/${channel.id}/thinking`);
    expect(space.nodes.find((node) => node.id === root.id)?.agent_session_id).toBe(nodeSessionID);
    expect(providerSessionIDForNode(root.id)).toBe(providerSessionID);
    const resumedState = databaseJSON<{ message_count: number }>(`
      SELECT json_build_object('message_count', COUNT(*))::text
        FROM messages
       WHERE thinking_node_id = '${root.id}'
         AND content IN ('${firstAck}', '${resumedAck}')
    `);
    expect(resumedState.message_count).toBe(2);
  } finally {
    await page.close();
    if (channel) await api(request, auth.access_token, 'delete', `/api/v1/channels/${channel.id}`).catch(() => undefined);
    if (runtimeAgent) await api(request, auth.access_token, 'delete', `/api/v1/agents/${runtimeAgent.id}`).catch(() => undefined);
  }
});
