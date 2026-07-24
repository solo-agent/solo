import { execFileSync } from 'node:child_process';
import { expect, test, type APIRequestContext } from '@playwright/test';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const credentials = { email: 'agent-channel-template-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface Agent {
  id: string;
  name: string;
  home_channel_id: string;
  kind: string;
  avatar_url: string;
}

interface PersistedScope {
  source_template_id: string;
  agents: number;
  wrong_homes: number;
  visible_memberships: number;
  relationships: number;
  cross_channel_relationships: number;
  avatar_mismatches: number;
}

interface RemovedAgentState {
  active_agents: number;
  removed_active: boolean;
  removed_visible_memberships: number;
  historical_relationships: number;
  released_tasks: number;
  cancelled_runs: number;
  closed_sessions: number;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await request.post(`${apiBase}/api/v1/auth/register`, {
    data: { ...credentials, display_name: 'Agent Channel Template E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
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

test('large template relationship controls select the exact edge', async ({ page, request }) => {
  const auth = await authenticate(request);
  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });

  await page.goto('/templates');
  await page.locator('a[href="/templates/agency-solo-company-all-hands"]').click();
  await expect(page).toHaveURL(/\/templates\/agency-solo-company-all-hands$/);
  await expect(page.locator('.react-flow__node')).toHaveCount(8);
  const edgeButtons = page.locator('button[data-relationship-edge-id]');
  await expect(edgeButtons).toHaveCount(9);
  await expect(page.locator('[data-id="content-creator"]')).toContainText('Content Creator');
  expect((await page.locator('[data-id="content-creator"]').innerText()).match(/Content Creator/g)).toHaveLength(1);

  const assignmentButton = page.locator(
    'button[data-relationship-edge-id="nexus-strategy-trend-researcher-0"]',
  );
  const assignmentBox = await assignmentButton.boundingBox();
  const assignmentTargetBox = await page.locator('[data-id="trend-researcher"]').boundingBox();
  expect(assignmentBox).not.toBeNull();
  expect(assignmentTargetBox).not.toBeNull();
  expect(Math.abs(
    assignmentBox!.x + assignmentBox!.width / 2
    - (assignmentTargetBox!.x + assignmentTargetBox!.width / 2),
  )).toBeLessThan(2);
  expect(assignmentTargetBox!.y - (assignmentBox!.y + assignmentBox!.height / 2)).toBeGreaterThan(8);

  for (const [from, to] of [
    ['backend-architect', 'financial-forecaster'],
    ['manager', 'content-creator'],
  ]) {
    const fromBox = await page.locator(`[data-id="${from}"]`).boundingBox();
    const toBox = await page.locator(`[data-id="${to}"]`).boundingBox();
    expect(fromBox).not.toBeNull();
    expect(toBox).not.toBeNull();
    const gap = Math.max(toBox!.x - (fromBox!.x + fromBox!.width), fromBox!.x - (toBox!.x + toBox!.width));
    expect(gap).toBeLessThan(60);
  }

  const edgeIDs = await edgeButtons.evaluateAll((buttons) => (
    buttons.map((button) => button.getAttribute('data-relationship-edge-id')!)
  ));
  for (const edgeID of edgeIDs) {
    const button = page.locator(`button[data-relationship-edge-id="${edgeID}"]`);
    await button.click();
    await expect(button).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator(`[data-testid="rf__edge-${edgeID}"]`)).toHaveClass(/selected/);
    await expect(page.locator('.relationship-edge-label-selected')).toHaveCount(1);
  }
});

test('official template creates a fresh Channel-scoped team through the real UI and database', async ({ page, request }) => {
  const auth = await authenticate(request);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const existingChannelsResponse = await request.get(`${apiBase}/api/v1/channels`, { headers });
  expect(existingChannelsResponse.ok()).toBeTruthy();
  const existingChannels = await existingChannelsResponse.json() as Array<{ id: string; name: string }>;
  for (const channel of existingChannels) {
    if (/^(blank|template|lucy)-e2e-/.test(channel.name)) {
      const cleanupResponse = await request.delete(
        `${apiBase}/api/v1/channels/${channel.id}`,
        { headers },
      );
      expect(cleanupResponse.ok()).toBeTruthy();
    }
  }
  const lucyChannelResponse = await request.get(`${apiBase}/api/v1/channels/lucy`, { headers });
  expect(lucyChannelResponse.ok()).toBeTruthy();
  const lucyChannel = await lucyChannelResponse.json() as { id: string };
  const lucyMembersResponse = await request.get(
    `${apiBase}/api/v1/channels/${lucyChannel.id}/members`,
    { headers },
  );
  expect(lucyMembersResponse.ok()).toBeTruthy();
  const lucyMembers = await lucyMembersResponse.json() as Array<{
    member_type: string;
    display_name: string;
  }>;
  if (!lucyMembers.some((member) => member.member_type === 'agent' && member.display_name === 'Lucy')) {
    const createLucyResponse = await request.post(`${apiBase}/api/v1/onboarding/create-lucy`, {
      headers,
      data: { runtime_type: 'codex', channel_id: lucyChannel.id },
    });
    expect(createLucyResponse.ok()).toBeTruthy();
  }
  const channelName = `template-e2e-${Date.now().toString(36)}`;
  const blankChannelName = `blank-e2e-${Date.now().toString(36)}`;
  const appliedChannelName = `apply-e2e-${Date.now().toString(36)}`;
  const blankAgentName = `First Agent ${Date.now().toString(36)}`;
  const renamedBlankAgent = `${blankAgentName} Updated`;
  let channelID = '';
  let blankChannelID = '';
  let appliedChannelID = '';

  await page.addInitScript(({ accessToken, refreshToken }) => {
    localStorage.setItem('access_token', accessToken);
    localStorage.setItem('refresh_token', refreshToken);
    localStorage.setItem('solo.locale', 'en');
  }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });

  try {
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Lucy' }).click();
    await expect(page).toHaveURL(/\/dashboard\?channel=/);
    await expect(page.getByRole('heading', { name: 'Lucy' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Teams', exact: true })).toBeVisible();
    await expect(page.getByText('Quick start with Lucy')).toBeVisible();
    await page.getByRole('button', { name: 'Choose a template for a product launch' }).click();
    await expect(page.getByLabel('Message input')).toHaveValue('Choose a template for a product launch');
    await page.getByLabel('Message input').fill('');

    const createChannelButton = page.getByRole('button', { name: 'Create Channel' });
    await createChannelButton.click();
    await expect(page.getByRole('heading', { name: 'Start a Channel' })).toBeVisible();
    const createChannelDialog = page.getByRole('dialog');
    const dialogButtons = createChannelDialog.getByRole('button');
    await dialogButtons.last().focus();
    await page.keyboard.press('Tab');
    await expect(createChannelDialog.getByRole('button', { name: 'Close' })).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(createChannelDialog).toBeHidden();
    await expect(createChannelButton).toBeFocused();

    await createChannelButton.click();
    await page.getByRole('button', { name: /Blank Channel/ }).click();
    await expect(page.getByRole('heading', { name: 'Create a blank Channel' })).toBeVisible();
    await page.getByRole('dialog').getByRole('textbox', { name: 'Name', exact: true }).fill(blankChannelName);
    await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
    await expect.poll(
      () => new URL(page.url()).searchParams.get('channel'),
    ).not.toBe(lucyChannel.id);
    blankChannelID = new URL(page.url()).searchParams.get('channel') ?? '';
    expect(blankChannelID).not.toBe('');

    await page.getByRole('button', { name: 'Teams', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Build this Channel’s team' })).toBeVisible();
    await page.getByRole('button', { name: 'Create first Agent' }).click();
    const addAgentDialog = page.getByRole('dialog');
    await expect(addAgentDialog.getByRole('heading', { name: 'Create Agent' })).toBeVisible();
    await addAgentDialog.getByLabel('Name *').fill(blankAgentName);
    await addAgentDialog.getByLabel('Description').fill('Owns the first task in this Channel.');
    await addAgentDialog.getByRole('button', { name: 'Select Runtime...' }).click();
    await page.locator('[role="option"]:not([aria-disabled="true"])').first().click();
    await addAgentDialog.getByLabel('System Prompt').fill('Work only inside this Channel and report progress clearly.');
    await addAgentDialog.getByRole('button', { name: 'Create Agent' }).click();
    await expect(addAgentDialog).toBeHidden();
    await expect(page.locator('.react-flow__node')).toHaveCount(1);
    await expect(page.getByText(blankAgentName, { exact: true })).toBeVisible();

    const blankAgentsResponse = await request.get(`${apiBase}/api/v1/channels/${blankChannelID}/agents`, { headers });
    expect(blankAgentsResponse.ok()).toBeTruthy();
    const blankAgents = await blankAgentsResponse.json() as Agent[];
    expect(blankAgents).toHaveLength(1);
    expect(blankAgents[0].home_channel_id).toBe(blankChannelID);
    expect(blankAgents[0].avatar_url).toBe(`dicebear:pixel-art:agent-${blankAgents[0].id}`);

    await page.locator('.react-flow__node').click();
    await expect(page.getByRole('heading', { name: 'Agent Detail' })).toBeVisible();
    await page.getByRole('button', { name: 'Edit Name *' }).click();
    await page.getByLabel('Name *').fill(renamedBlankAgent);
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText(renamedBlankAgent, { exact: true }).first()).toBeVisible();
    await expect.poll(async () => {
      const response = await request.get(`${apiBase}/api/v1/channels/${blankChannelID}/agents`, { headers });
      const scopedAgents = await response.json() as Agent[];
      return scopedAgents[0]?.name;
    }).toBe(renamedBlankAgent);

    const blankAgentState = databaseJSON<{
      name: string;
      home_channel_id: string;
      avatar_url: string;
      visible_memberships: number;
    }>(`
      SELECT json_build_object(
        'name', agent.name,
        'home_channel_id', agent.home_channel_id,
        'avatar_url', agent.avatar_url,
        'visible_memberships', (
          SELECT COUNT(*)
            FROM channel_members member
            JOIN channels membership_channel ON membership_channel.id = member.channel_id
           WHERE member.member_type = 'agent'
             AND member.member_id = agent.id
             AND membership_channel.type <> 'dm'
        )
      )::text
        FROM agents agent
       WHERE agent.id = '${blankAgents[0].id}'
    `);
    expect(blankAgentState).toEqual({
      name: renamedBlankAgent,
      home_channel_id: blankChannelID,
      avatar_url: `dicebear:pixel-art:agent-${blankAgents[0].id}`,
      visible_memberships: 1,
    });

    await createChannelButton.click();
    await expect(page.getByRole('heading', { name: 'Start a Channel' })).toBeVisible();
    await page.getByRole('button', { name: /Blank Channel/ }).click();
    await page.getByRole('dialog').getByRole('textbox', { name: 'Name', exact: true }).fill(blankChannelName);
    await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
    await expect(page.getByRole('dialog').getByRole('alert')).toContainText('a channel with this name already exists');
    await expect(page).toHaveURL(new RegExp(`/dashboard\\?channel=${blankChannelID}`));
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

    await createChannelButton.click();
    await page.getByRole('button', { name: /Blank Channel/ }).click();
    await page.getByRole('dialog').getByRole('textbox', { name: 'Name', exact: true }).fill(appliedChannelName);
    await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
    await expect.poll(
      () => new URL(page.url()).searchParams.get('channel'),
    ).not.toBe(blankChannelID);
    appliedChannelID = new URL(page.url()).searchParams.get('channel') ?? '';
    expect(appliedChannelID).not.toBe('');

    await page.getByRole('button', { name: 'Teams', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Create first Agent' })).toBeVisible();
    await page.getByRole('button', { name: 'Choose Team template' }).click();
    await expect(page).toHaveURL(new RegExp(`/templates\\?channel=${appliedChannelID}`));
    await expect(page.getByText('Choose the whole team')).toBeVisible();
    await expect(page.locator('a[href^="/templates/"] img').first()).toBeVisible();
    await expect(page.getByRole('link', { name: /Let Lucy choose the team/ })).toHaveCount(0);
    await page.locator('a[href^="/templates/agency-dev-tech-design-review"]').click();
    await expect(page.locator('.react-flow__node')).toHaveCount(4);
    await expect(page.locator('.template-flow .relationship-edge-label svg')).toHaveCount(4);
    await page.getByLabel(/assigns to/i).first().click();
    await expect(page.getByText('Delegation Criteria')).toBeVisible();
    await expect(page.getByText(/Delegation contract:/)).toBeVisible();
    const templateAvatarSources = await page.locator('.react-flow__node img').evaluateAll(
      (images) => images.map((image) => (image as HTMLImageElement).src).sort(),
    );
    expect(new Set(templateAvatarSources).size).toBe(4);
    await page.getByRole('button', { name: 'Add team to Channel' }).click();
    await expect(page).toHaveURL(new RegExp(`/dashboard\\?channel=${appliedChannelID}`));
    await page.getByRole('button', { name: 'Teams', exact: true }).click();
    await expect(page.locator('.react-flow__node')).toHaveCount(4);
    await expect(page.locator('.relationship-flow .relationship-edge-label svg')).toHaveCount(4);
    await expect(page.locator('.relationship-flow .react-flow__edge path[marker-end]')).toHaveCount(3);
    const teamAgentNode = page.locator('.relationship-flow .react-flow__node').first();
    await teamAgentNode.click();
    await page.waitForTimeout(500);
    await expect(teamAgentNode.locator('.relationship-agent-node-selected')).toBeVisible();
    await expect(page.locator('.relationship-flow .relationship-agent-node-selected')).toHaveCount(1);
    const teamEdgeButton = page.locator('.relationship-flow button[data-relationship-edge-id]').first();
    const teamEdgeID = await teamEdgeButton.getAttribute('data-relationship-edge-id');
    await teamEdgeButton.click();
    await page.waitForTimeout(500);
    await expect(page.locator('.relationship-flow .relationship-agent-node-selected')).toHaveCount(0);
    await expect(teamEdgeButton).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator(`[data-testid="rf__edge-${teamEdgeID}"]`)).toHaveClass(/selected/);
    await expect(page.locator('.relationship-flow .relationship-edge-label-selected')).toHaveCount(1);
    const agentAvatarSources = await page.locator('.react-flow__node img').evaluateAll(
      (images) => images.map((image) => (image as HTMLImageElement).src).sort(),
    );
    expect(agentAvatarSources).toEqual(templateAvatarSources);
    await page.getByRole('button', { name: 'Channel members' }).click();
    await expect(page.getByRole('dialog').getByRole('button', { name: 'Add agent to channel' })).toHaveCount(0);
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

    const appliedPersisted = databaseJSON<PersistedScope>(`
      SELECT json_build_object(
        'source_template_id', channel.source_template_id,
        'agents', COUNT(DISTINCT agent.id),
        'wrong_homes', COUNT(DISTINCT agent.id) FILTER (WHERE agent.home_channel_id <> channel.id),
        'visible_memberships', (
          SELECT COUNT(*)
            FROM channel_members member
           WHERE member.channel_id = channel.id
             AND member.member_type = 'agent'
        ),
        'relationships', (
          SELECT COUNT(*)
            FROM agent_relationships relationship
           WHERE relationship.from_agent_id IN (SELECT id FROM agents WHERE home_channel_id = channel.id)
        ),
        'cross_channel_relationships', (
          SELECT COUNT(*)
            FROM agent_relationships relationship
            JOIN agents source ON source.id = relationship.from_agent_id
            JOIN agents target ON target.id = relationship.to_agent_id
           WHERE source.home_channel_id = channel.id
             AND source.home_channel_id <> target.home_channel_id
        ),
        'avatar_mismatches', COUNT(DISTINCT agent.id) FILTER (
          WHERE agent.avatar_url IS NULL
             OR agent.avatar_url NOT LIKE 'dicebear:pixel-art:template-agency-dev-tech-design-review-%'
        )
      )::text
        FROM channels channel
        JOIN agents agent ON agent.home_channel_id = channel.id AND agent.is_active = true
       WHERE channel.id = '${appliedChannelID}'
       GROUP BY channel.id
    `);
    expect(appliedPersisted).toEqual({
      source_template_id: 'agency-dev-tech-design-review',
      agents: 4,
      wrong_homes: 0,
      visible_memberships: 4,
      relationships: 4,
      cross_channel_relationships: 0,
      avatar_mismatches: 0,
    });
    const secondApplyResponse = await request.post(
      `${apiBase}/api/v1/channels/${appliedChannelID}/template`,
      {
        headers,
        data: { template_id: 'agency-dev-tech-design-review' },
      },
    );
    expect(secondApplyResponse.status()).toBe(409);

    await createChannelButton.click();
    await page.getByRole('button', { name: /From template/ }).click();

    await expect(page.getByRole('heading', { name: 'Agent team templates' })).toBeVisible();
    await expect(page.getByRole('link', { name: /Let Lucy choose the team/ })).toBeVisible();
    await page.locator('a[href^="/templates/agency-dev-tech-design-review"]').click();

    await expect(page.getByRole('heading', { name: 'Team relationship preview' })).toBeVisible();
    await expect(page.locator('.react-flow__node')).toHaveCount(4);

    await page.getByLabel('Channel name').fill(channelName);
    await page.getByRole('button', { name: 'Create Channel + team' }).click();
    await expect.poll(
      () => new URL(page.url()).searchParams.get('channel'),
    ).not.toBeNull();
    channelID = new URL(page.url()).searchParams.get('channel') ?? '';
    expect(channelID).not.toBe('');

    const agentsResponse = await request.get(`${apiBase}/api/v1/channels/${channelID}/agents`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
    expect(agentsResponse.ok()).toBe(true);
    const agents = await agentsResponse.json() as Agent[];
    expect(agents).toHaveLength(4);
    expect(agents.every((agent) => agent.home_channel_id === channelID && agent.kind === 'agent')).toBe(true);
    expect(agents.every((agent) => agent.avatar_url.startsWith('dicebear:pixel-art:template-agency-dev-tech-design-review-'))).toBe(true);

    const removedGlobalCreate = await request.post(`${apiBase}/api/v1/agents`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
      data: { name: 'should-not-exist' },
    });
    expect(removedGlobalCreate.status()).toBe(404);

    const persisted = databaseJSON<PersistedScope>(`
      SELECT json_build_object(
        'source_template_id', channel.source_template_id,
        'agents', COUNT(DISTINCT agent.id),
        'wrong_homes', COUNT(DISTINCT agent.id) FILTER (WHERE agent.home_channel_id <> channel.id),
        'visible_memberships', (
          SELECT COUNT(*)
            FROM channel_members member
            JOIN channels membership_channel ON membership_channel.id = member.channel_id
           WHERE member.member_type = 'agent'
             AND member.member_id IN (SELECT id FROM agents WHERE home_channel_id = channel.id)
             AND membership_channel.type <> 'dm'
        ),
        'relationships', (
          SELECT COUNT(*)
            FROM agent_relationships relationship
           WHERE relationship.from_agent_id IN (SELECT id FROM agents WHERE home_channel_id = channel.id)
        ),
        'cross_channel_relationships', (
          SELECT COUNT(*)
            FROM agent_relationships relationship
            JOIN agents source ON source.id = relationship.from_agent_id
            JOIN agents target ON target.id = relationship.to_agent_id
           WHERE source.home_channel_id = channel.id
             AND source.home_channel_id <> target.home_channel_id
        ),
        'avatar_mismatches', COUNT(DISTINCT agent.id) FILTER (
          WHERE agent.avatar_url IS NULL
             OR agent.avatar_url NOT LIKE 'dicebear:pixel-art:template-agency-dev-tech-design-review-%'
        )
      )::text
        FROM channels channel
        JOIN agents agent ON agent.home_channel_id = channel.id AND agent.is_active = true
       WHERE channel.id = '${channelID}'
       GROUP BY channel.id
    `);
    expect(persisted).toEqual({
      source_template_id: 'agency-dev-tech-design-review',
      agents: 4,
      wrong_homes: 0,
      visible_memberships: 4,
      relationships: 4,
      cross_channel_relationships: 0,
      avatar_mismatches: 0,
    });

    const seededLifecycle = databaseJSON<{ sessions: number; runs: number; tasks: number }>(`
      WITH scoped_agents AS MATERIALIZED (
        SELECT id, row_number() OVER (ORDER BY created_at, id) AS position
          FROM agents
         WHERE home_channel_id = '${channelID}' AND is_active = true
      ),
      inserted_sessions AS (
        INSERT INTO agent_sessions (agent_id, provider, external_session_id, status)
        SELECT id, 'e2e', 'agent-channel-scope-' || id::text, 'active'
          FROM scoped_agents
        RETURNING id, agent_id
      ),
      inserted_runs AS (
        INSERT INTO agent_runs (agent_id, session_id, trigger_type, channel_id, status)
        SELECT session.agent_id, session.id, 'e2e', '${channelID}', 'running'
          FROM inserted_sessions session
        RETURNING id
      ),
      inserted_tasks AS (
        INSERT INTO tasks (
          channel_id, creator_id, claimer_id, title, status, priority, task_number
        )
        SELECT '${channelID}', channel.created_by, agent.id,
               'agent-scope-removal-' || agent.id::text,
               'in_progress', 'normal', (1000 + agent.position)::int
          FROM scoped_agents agent
          JOIN channels channel ON channel.id = '${channelID}'
        RETURNING id
      )
      SELECT json_build_object(
        'sessions', (SELECT COUNT(*) FROM inserted_sessions),
        'runs', (SELECT COUNT(*) FROM inserted_runs),
        'tasks', (SELECT COUNT(*) FROM inserted_tasks)
      )::text
    `);
    expect(seededLifecycle).toEqual({ sessions: 4, runs: 4, tasks: 4 });

    await page.getByRole('button', { name: 'Teams', exact: true }).click();
    await page.getByRole('button', { name: 'Channel members' }).click();
    await expect(page.getByRole('heading', { name: /Channel Members/ })).toBeVisible();
    await page.getByRole('button', { name: /^Delete Agent: / }).first().click();
    await page.getByRole('button', { name: 'Delete Agent', exact: true }).click();
    await expect.poll(async () => {
      const response = await request.get(`${apiBase}/api/v1/channels/${channelID}/agents`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
      });
      return (await response.json() as Agent[]).length;
    }).toBe(3);

    const activeAgentsResponse = await request.get(`${apiBase}/api/v1/channels/${channelID}/agents`, {
      headers: { authorization: `Bearer ${auth.access_token}` },
    });
    const activeAgentIDs = new Set(
      (await activeAgentsResponse.json() as Agent[]).map((agent) => agent.id),
    );
    const removedAgent = agents.find((agent) => !activeAgentIDs.has(agent.id));
    expect(removedAgent).toBeDefined();
    const removedState = databaseJSON<RemovedAgentState>(`
      SELECT json_build_object(
        'active_agents', (
          SELECT COUNT(*) FROM agents
           WHERE home_channel_id = '${channelID}' AND is_active = true
        ),
        'removed_active', (
          SELECT is_active FROM agents WHERE id = '${removedAgent!.id}'
        ),
        'removed_visible_memberships', (
          SELECT COUNT(*)
            FROM channel_members member
            JOIN channels membership_channel ON membership_channel.id = member.channel_id
           WHERE member.member_id = '${removedAgent!.id}'
             AND membership_channel.type <> 'dm'
        ),
        'historical_relationships', (
          SELECT COUNT(*)
            FROM agent_relationships
           WHERE from_agent_id = '${removedAgent!.id}' OR to_agent_id = '${removedAgent!.id}'
        ),
        'released_tasks', (
          SELECT COUNT(*)
            FROM tasks
           WHERE channel_id = '${channelID}'
             AND title = 'agent-scope-removal-${removedAgent!.id}'
             AND status = 'todo'
             AND claimer_id IS NULL
        ),
        'cancelled_runs', (
          SELECT COUNT(*)
            FROM agent_runs
           WHERE channel_id = '${channelID}'
             AND agent_id = '${removedAgent!.id}'
             AND status = 'cancelled'
        ),
        'closed_sessions', (
          SELECT COUNT(*)
            FROM agent_sessions
           WHERE agent_id = '${removedAgent!.id}'
             AND status = 'closed'
        )
      )::text
    `);
    expect(removedState.active_agents).toBe(3);
    expect(removedState.removed_active).toBe(false);
    expect(removedState.removed_visible_memberships).toBe(0);
    expect(removedState.historical_relationships).toBeGreaterThan(0);
    expect(removedState.released_tasks).toBe(1);
    expect(removedState.cancelled_runs).toBe(1);
    expect(removedState.closed_sessions).toBe(1);

    const retiredTeamsPage = await page.request.get('/teams', { maxRedirects: 0 });
    expect(retiredTeamsPage.status()).toBe(404);
  } finally {
    if (blankChannelID) {
      await request.delete(`${apiBase}/api/v1/channels/${blankChannelID}`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
      });
    }
    if (channelID) {
      await request.delete(`${apiBase}/api/v1/channels/${channelID}`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
      });
    }
    if (appliedChannelID) {
      await request.delete(`${apiBase}/api/v1/channels/${appliedChannelID}`, {
        headers: { authorization: `Bearer ${auth.access_token}` },
      });
    }
  }
});
