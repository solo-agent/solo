import assert from 'node:assert/strict';
import { buildDashboardHref, parseDashboardParams } from '../lib/dashboard-url.ts';

const state = parseDashboardParams(new URLSearchParams('channel=c1&view=task&task=t1&panel=thread&thread=m1'));
assert.deepEqual(state, {
  channelId: 'c1',
  view: 'task',
  panel: 'thread',
  taskId: 't1',
  threadId: 'm1',
  messageId: null,
  agentId: null,
  relationshipId: null,
});

assert.equal(
  buildDashboardHref('c1', { view: 'task', panel: 'conversation', taskId: null, threadId: null }),
  '/dashboard?channel=c1&view=task',
);

assert.equal(
  buildDashboardHref('c1', { view: 'task', panel: 'conversation', taskId: 'stale-task', threadId: 'stale-thread' }),
  '/dashboard?channel=c1&view=task',
);

assert.deepEqual(parseDashboardParams(new URLSearchParams('channel=c1')), {
  channelId: 'c1',
  view: 'team',
  panel: 'conversation',
  taskId: null,
  threadId: null,
  messageId: null,
  agentId: null,
  relationshipId: null,
});

assert.deepEqual(parseDashboardParams(new URLSearchParams('channel=c1&thread=m1')), {
  channelId: 'c1',
  view: 'team',
  panel: 'thread',
  taskId: null,
  threadId: 'm1',
  messageId: null,
  agentId: null,
  relationshipId: null,
});

assert.equal(
  buildDashboardHref('c1', { view: 'team', panel: 'agent', agentId: 'a1' }),
  '/dashboard?channel=c1&panel=agent&agent=a1',
);
