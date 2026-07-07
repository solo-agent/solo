import assert from 'node:assert/strict';
import { buildDashboardHref, parseDashboardParams } from '../lib/dashboard-url.ts';

const state = parseDashboardParams(new URLSearchParams('channel=c1&view=task.graph&task=t1&panel=thread&thread=m1'));
assert.deepEqual(state, {
  channelId: 'c1',
  view: 'task.graph',
  panel: 'thread',
  taskId: 't1',
  threadId: 'm1',
  messageId: null,
  nodeId: null,
  agentId: null,
  relationshipId: null,
});

assert.equal(
  buildDashboardHref('c1', { view: 'task.board', panel: 'conversation', taskId: null, threadId: null }),
  '/dashboard?channel=c1&view=task.board',
);

assert.equal(
  buildDashboardHref('c1', { view: 'overview', panel: 'conversation', taskId: 'stale-task', threadId: 'stale-thread' }),
  '/dashboard?channel=c1',
);

const oldState = parseDashboardParams(new URLSearchParams('channel=c1&scope=channel&right=task&mode=graph&task=t1&thread=m1'));
assert.deepEqual(oldState, {
  channelId: 'c1',
  view: 'overview',
  panel: 'conversation',
  taskId: null,
  threadId: null,
  messageId: null,
  nodeId: null,
  agentId: null,
  relationshipId: null,
});
