import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { acquireLocalComputer, type LocalComputerLease } from './support/local-computer';
import { registerVerified } from './support/auth';

const apiBase = process.env.SOLO_E2E_API_URL ?? 'http://127.0.0.1:8080';
const wsBase = apiBase.replace(/^http/, 'ws');
const credentials = { email: 'message-reliability-e2e@solo.local', password: 'SoloE2E-2026!' };

interface AuthResponse {
  access_token: string;
  refresh_token: string;
}

interface SendResponse {
  id: string;
  message_id?: string;
  thread_id?: string;
  client_msg_id: string;
  deduplicated?: boolean;
}

function databaseJSON<T>(query: string): T {
  const output = execFileSync('docker', [
    'exec', process.env.SOLO_POSTGRES_CONTAINER ?? 'solo-postgres',
    'psql', '-U', process.env.POSTGRES_USER ?? 'solo', '-d', process.env.POSTGRES_DB ?? 'solo',
    '-tA', '-c', query,
  ], { encoding: 'utf8' }).trim();
  return JSON.parse(output) as T;
}

async function authenticate(request: APIRequestContext): Promise<AuthResponse> {
  const login = await request.post(`${apiBase}/api/v1/auth/login`, { data: credentials });
  if (login.ok()) return login.json();
  const register = await registerVerified(request, apiBase, {
    data: { ...credentials, display_name: 'Message Reliability E2E' },
  });
  if (!register.ok()) throw new Error(`E2E authentication failed: ${register.status()} ${await register.text()}`);
  return register.json();
}

async function openEventProbe(page: Page, token: string, channelID: string): Promise<void> {
  await page.evaluate(
    ({ url, accessToken, channel }) => new Promise<void>((resolve, reject) => {
      const state = window as typeof window & { reliabilityEvents?: Array<Record<string, unknown>>; reliabilitySocket?: WebSocket };
      state.reliabilityEvents = [];
      const socket = new WebSocket(`${url}/api/v1/ws?token=${encodeURIComponent(accessToken)}`);
      state.reliabilitySocket = socket;
      socket.onerror = () => reject(new Error('reliability probe failed to connect'));
      socket.onmessage = (event) => {
        const envelope = JSON.parse(String(event.data)) as { type: string; payload: Record<string, unknown> };
        if (envelope.type === 'message.new' || envelope.type === 'thread.message.new') {
          state.reliabilityEvents!.push({ type: envelope.type, ...envelope.payload });
        }
      };
      socket.onopen = () => {
        socket.send(JSON.stringify({ type: 'subscribe', payload: { channel_id: channel } }));
        resolve();
      };
    }),
    { url: wsBase, accessToken: token, channel: channelID },
  );
}

test('client message ID deduplicates message, task, and first thread reply', async ({ page, request }) => {
  const auth = await authenticate(request);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  const created = await request.post(`${apiBase}/api/v1/channels`, {
    headers,
    data: { name: `reliability-e2e-${suffix}`, description: 'Message reliability E2E' },
  });
  expect(created.ok()).toBe(true);
  const channel = await created.json() as { id: string };
  let dmID = '';
  let dmMessageID = '';

  try {
    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channel.id}`);
    await expect(page.locator('#channel-conversation-panel')).toBeVisible();
    await openEventProbe(page, auth.access_token, channel.id);

    const messageContent = `RELIABLE_MESSAGE_${suffix}`;
    const messageClientID = crypto.randomUUID();
    const messageRequests = [1, 2].map(() => request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages`,
      { headers, data: { content: messageContent, client_msg_id: messageClientID } },
    ));
    const messageResponses = await Promise.all(messageRequests);
    expect(messageResponses.every((response) => response.status() === 201)).toBe(true);
    const messageBodies = await Promise.all(messageResponses.map((response) => response.json() as Promise<SendResponse>));
    expect(new Set(messageBodies.map((body) => body.id)).size).toBe(1);
    expect(messageBodies.some((body) => body.deduplicated)).toBe(true);
    expect(messageBodies.every((body) => body.client_msg_id === messageClientID)).toBe(true);
    await expect(page.getByLabel('Message list').getByText(messageContent, { exact: true })).toHaveCount(1);

    const taskContent = `RELIABLE_TASK_${suffix}`;
    const taskClientID = crypto.randomUUID();
    const taskResponses = await Promise.all([1, 2].map(() => request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages`,
      { headers, data: { content: taskContent, as_task: true, client_msg_id: taskClientID } },
    )));
    const taskBodies = await Promise.all(taskResponses.map((response) => response.json() as Promise<SendResponse>));
    expect(new Set(taskBodies.map((body) => body.id)).size).toBe(1);
    expect(taskBodies.some((body) => body.deduplicated)).toBe(true);

    const threadContent = `RELIABLE_THREAD_${suffix}`;
    const threadClientID = crypto.randomUUID();
    const messageID = messageBodies[0].id;
    const threadResponses = await Promise.all([1, 2].map(() => request.post(
      `${apiBase}/api/v1/channels/${channel.id}/messages/${messageID}/thread`,
      { headers, data: { content: threadContent, client_msg_id: threadClientID } },
    )));
    const threadBodies = await Promise.all(threadResponses.map((response) => response.json() as Promise<SendResponse>));
    expect(new Set(threadBodies.map((body) => body.id)).size).toBe(1);
    expect(threadBodies.some((body) => body.deduplicated)).toBe(true);

    await expect.poll(() => page.evaluate(({ messageID, threadID }) => {
      const events = (window as typeof window & { reliabilityEvents?: Array<Record<string, unknown>> }).reliabilityEvents ?? [];
      return {
        messages: events.filter((event) => event.client_msg_id === messageID).length,
        threads: events.filter((event) => {
          const message = event.message as Record<string, unknown> | undefined;
          return message?.client_msg_id === threadID;
        }).length,
      };
    }, { messageID: messageClientID, threadID: threadClientID })).toEqual({ messages: 1, threads: 1 });

    const state = databaseJSON<{ messages: number; tasks: number; replies: number; reply_count: number }>(`
      SELECT json_build_object(
        'messages', COUNT(*) FILTER (WHERE m.content = '${messageContent}'),
        'tasks', (SELECT COUNT(*) FROM tasks WHERE channel_id = '${channel.id}' AND title = '${taskContent}'),
        'replies', COUNT(*) FILTER (WHERE m.content = '${threadContent}'),
        'reply_count', COALESCE((SELECT reply_count FROM threads WHERE root_message_id = '${messageID}'), 0)
      )::text
      FROM messages m
      WHERE m.channel_id = '${channel.id}'
    `);
    expect(state).toEqual({ messages: 1, tasks: 1, replies: 1, reply_count: 1 });

    const secondary = await registerVerified(request, apiBase, {
      data: {
        email: `message-reliability-peer-${suffix}@solo.local`,
        password: credentials.password,
        display_name: 'Reliability Peer',
      },
    });
    expect(secondary.ok()).toBe(true);
    const secondaryAuth = await secondary.json() as AuthResponse;
    const secondaryUserResponse = await request.get(`${apiBase}/api/v1/users/me`, {
      headers: { authorization: `Bearer ${secondaryAuth.access_token}` },
    });
    const secondaryUser = await secondaryUserResponse.json() as { id: string };
    const dmResponse = await request.post(`${apiBase}/api/v1/dm`, {
      headers,
      data: { member_type: 'user', member_id: secondaryUser.id },
    });
    expect(dmResponse.ok()).toBe(true);
    dmID = (await dmResponse.json() as { id: string }).id;
    const dmContent = `RELIABLE_DM_${suffix}`;
    const dmClientID = crypto.randomUUID();
    const dmResponses = await Promise.all([1, 2].map(() => request.post(
      `${apiBase}/api/v1/dm/${dmID}/messages`,
      { headers, data: { content: dmContent, client_msg_id: dmClientID } },
    )));
    const dmBodies = await Promise.all(dmResponses.map((response) => response.json() as Promise<SendResponse>));
    dmMessageID = dmBodies[0].id;
    expect(new Set(dmBodies.map((body) => body.id)).size).toBe(1);
    expect(dmBodies.some((body) => body.deduplicated)).toBe(true);
    expect(databaseJSON<number>(`
      SELECT COUNT(*)::int::text FROM messages WHERE channel_id = '${dmID}' AND content = '${dmContent}'
    `)).toBe(1);

    const uiContent = `RELIABLE_UI_${suffix}`;
    const composer = page.getByPlaceholder('Type a message...');
    await composer.fill(uiContent);
    await composer.press('Enter');
    await expect(composer).toHaveValue('');
    await expect(page.getByLabel('Message list').getByText(uiContent, { exact: true })).toHaveCount(1);
    await expect.poll(() => databaseJSON<number>(`
      SELECT COUNT(*)::int::text FROM messages WHERE channel_id = '${channel.id}' AND content = '${uiContent}'
    `)).toBe(1);
    await expect.poll(() => page.evaluate((content) => {
      const events = (window as typeof window & { reliabilityEvents?: Array<Record<string, unknown>> }).reliabilityEvents ?? [];
      return events.find((event) => event.content === content)?.client_msg_id ?? '';
    }, uiContent)).not.toBe('');
  } finally {
    await page.evaluate(() => (window as typeof window & { reliabilitySocket?: WebSocket }).reliabilitySocket?.close()).catch(() => undefined);
    if (dmID && dmMessageID) {
      await request.delete(`${apiBase}/api/v1/dm/${dmID}/messages/${dmMessageID}`, { headers }).catch(() => undefined);
    }
    await request.delete(`${apiBase}/api/v1/channels/${channel.id}`, { headers });
  }
});

test('duplicate delivery triggers one real Agent run and one visible CLI reply', async ({ page, request }) => {
  test.skip(process.env.SOLO_E2E_REAL_AGENT_DELIVERY !== '1', 'requires the make-managed stack and authenticated local Claude runtime');
  test.setTimeout(240_000);

  const auth = await authenticate(request);
  const headers = { authorization: `Bearer ${auth.access_token}` };
  const suffix = Date.now().toString(36);
  let channelID = '';
  let agentID = '';
  let localComputer: LocalComputerLease | null = null;

  try {
    localComputer = await acquireLocalComputer(request, apiBase, auth.access_token);
    const channelResponse = await request.post(`${apiBase}/api/v1/channels`, {
      headers,
      data: { name: `reliability-agent-${suffix}`, description: 'Real Agent reliability E2E' },
    });
    channelID = (await channelResponse.json() as { id: string }).id;
    const acknowledgement = `RELIABLE_AGENT_ACK_${suffix.toUpperCase()}`;
    const agentResponse = await request.post(`${apiBase}/api/v1/channels/${channelID}/agents`, {
      headers,
      data: {
        name: `Reliability Agent ${suffix}`,
        computer_id: localComputer.id,
        model_provider: 'claude',
        model_name: 'sonnet',
        system_prompt: `For every human message, use solo message send to reply with exactly ${acknowledgement}. Do not merely print the reply.`,
      },
    });
    expect(agentResponse.ok()).toBe(true);
    agentID = (await agentResponse.json() as { id: string }).id;

    await expect.poll(() => databaseJSON<boolean>(`
      SELECT (COUNT(*) > 0 AND COUNT(*) FILTER (WHERE status IN ('queued','thinking','running','streaming','waiting_input','waiting_approval')) = 0)::text
      FROM agent_runs WHERE agent_id = '${agentID}'
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toBe(true);

    const content = `HUMAN_RELIABLE_TRIGGER_${suffix}`;
    const clientMsgID = crypto.randomUUID();
    const duplicateResponses = await Promise.all([1, 2].map(() => request.post(
      `${apiBase}/api/v1/channels/${channelID}/messages`,
      { headers, data: { content, client_msg_id: clientMsgID } },
    )));
    const duplicateBodies = await Promise.all(duplicateResponses.map((response) => response.json() as Promise<SendResponse>));
    const triggerMessageID = duplicateBodies[0].id;
    expect(new Set(duplicateBodies.map((body) => body.id)).size).toBe(1);

    await expect.poll(() => databaseJSON<{ runs: number; visible_replies: number }>(`
      SELECT json_build_object(
        'runs', (SELECT COUNT(*) FROM agent_runs WHERE trigger_message_id = '${triggerMessageID}'),
        'visible_replies', (SELECT COUNT(*) FROM messages WHERE channel_id = '${channelID}' AND sender_id = '${agentID}' AND content = '${acknowledgement}')
      )::text
    `), { timeout: 180_000, intervals: [500, 1000, 2000] }).toEqual({ runs: 1, visible_replies: 1 });

    await page.addInitScript(({ accessToken, refreshToken }) => {
      localStorage.setItem('access_token', accessToken);
      localStorage.setItem('refresh_token', refreshToken);
      localStorage.setItem('solo.locale', 'en');
    }, { accessToken: auth.access_token, refreshToken: auth.refresh_token });
    await page.goto(`/dashboard?channel=${channelID}`);
    await expect(page.getByLabel('Message list').getByText(acknowledgement, { exact: true })).toHaveCount(1);
  } finally {
    if (agentID) await request.delete(`${apiBase}/api/v1/agents/${agentID}`, { headers }).catch(() => undefined);
    if (channelID) await request.delete(`${apiBase}/api/v1/channels/${channelID}`, { headers }).catch(() => undefined);
    await localComputer?.release(request);
  }
});
