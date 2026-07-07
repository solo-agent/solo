export type DashboardView = 'overview' | 'team' | 'task.board' | 'task.graph' | 'thought.map' | 'thought.board';
export type DashboardPanel = 'conversation' | 'summary' | 'insight' | 'artifact' | 'thread' | 'agent' | 'relationship' | 'thought' | 'task';

export interface DashboardUrlState {
  channelId: string | null;
  view: DashboardView;
  panel: DashboardPanel;
  taskId: string | null;
  threadId: string | null;
  messageId: string | null;
  nodeId: string | null;
  agentId: string | null;
  relationshipId: string | null;
}

const views = new Set<DashboardView>(['overview', 'team', 'task.board', 'task.graph', 'thought.map', 'thought.board']);
const panels = new Set<DashboardPanel>(['conversation', 'summary', 'insight', 'artifact', 'thread', 'agent', 'relationship', 'thought', 'task']);
type SearchParamsLike = Pick<URLSearchParams, 'get'>;

export function parseDashboardParams(params: SearchParamsLike): DashboardUrlState {
  const view = params.get('view');
  const panel = params.get('panel');
  const parsedView = views.has(view as DashboardView) ? view as DashboardView : 'overview';
  const parsedPanel = panels.has(panel as DashboardPanel) ? panel as DashboardPanel : 'conversation';
  return {
    channelId: params.get('channel'),
    view: parsedView,
    panel: parsedPanel,
    taskId: parsedView.startsWith('task.') ? params.get('task') : null,
    threadId: parsedPanel === 'thread' ? params.get('thread') : null,
    messageId: params.get('message'),
    nodeId: params.get('node'),
    agentId: params.get('agent'),
    relationshipId: params.get('relationship'),
  };
}

export function buildDashboardHref(
  channelId: string,
  patch: Partial<Omit<DashboardUrlState, 'channelId'>>,
): string {
  const view = patch.view ?? 'overview';
  const panel = patch.panel ?? 'conversation';
  const params = new URLSearchParams();
  params.set('channel', channelId);
  if (view !== 'overview') params.set('view', view);
  if (panel !== 'conversation') params.set('panel', panel);
  if (view.startsWith('task.') && patch.taskId) params.set('task', patch.taskId);
  if (panel === 'thread' && patch.threadId) params.set('thread', patch.threadId);
  if (panel === 'conversation' && patch.messageId) params.set('message', patch.messageId);
  if ((panel === 'thought' || view.startsWith('thought.')) && patch.nodeId) params.set('node', patch.nodeId);
  if (panel === 'agent' && patch.agentId) params.set('agent', patch.agentId);
  if (panel === 'relationship' && patch.relationshipId) params.set('relationship', patch.relationshipId);
  return `/dashboard?${params.toString()}`;
}
