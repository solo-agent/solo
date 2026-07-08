export type DashboardView = 'team' | 'task';
export type DashboardPanel = 'conversation' | 'thread' | 'agent' | 'relationship';

export interface DashboardUrlState {
  channelId: string | null;
  view: DashboardView;
  panel: DashboardPanel;
  taskId: string | null;
  threadId: string | null;
  messageId: string | null;
  agentId: string | null;
  relationshipId: string | null;
}

const views = new Set<DashboardView>(['team', 'task']);
const panels = new Set<DashboardPanel>(['conversation', 'thread', 'agent', 'relationship']);
type SearchParamsLike = Pick<URLSearchParams, 'get'>;

export function parseDashboardParams(params: SearchParamsLike): DashboardUrlState {
  const view = params.get('view');
  const panel = params.get('panel');
  const parsedView = views.has(view as DashboardView) ? view as DashboardView : 'team';
  const parsedPanel = panels.has(panel as DashboardPanel)
    ? panel as DashboardPanel
    : params.get('thread')
      ? 'thread'
      : params.get('agent')
        ? 'agent'
        : params.get('relationship')
          ? 'relationship'
          : 'conversation';
  return {
    channelId: params.get('channel'),
    view: parsedView,
    panel: parsedPanel,
    taskId: parsedView === 'task' && parsedPanel === 'thread' ? params.get('task') : null,
    threadId: parsedPanel === 'thread' ? params.get('thread') : null,
    messageId: params.get('message'),
    agentId: parsedPanel === 'agent' ? params.get('agent') : null,
    relationshipId: parsedPanel === 'relationship' ? params.get('relationship') : null,
  };
}

export function buildDashboardHref(
  channelId: string,
  patch: Partial<Omit<DashboardUrlState, 'channelId'>>,
): string {
  const view = patch.view ?? 'team';
  const panel = patch.panel ?? 'conversation';
  const params = new URLSearchParams();
  params.set('channel', channelId);
  if (view !== 'team') params.set('view', view);
  if (panel !== 'conversation') params.set('panel', panel);
  if (view === 'task' && panel === 'thread' && patch.taskId) params.set('task', patch.taskId);
  if (panel === 'thread' && patch.threadId) params.set('thread', patch.threadId);
  if (panel === 'conversation' && patch.messageId) params.set('message', patch.messageId);
  if (panel === 'agent' && patch.agentId) params.set('agent', patch.agentId);
  if (panel === 'relationship' && patch.relationshipId) params.set('relationship', patch.relationshipId);
  return `/dashboard?${params.toString()}`;
}
