import { existsSync, readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));
const assert = (condition, message) => {
  if (!condition) {
    throw new Error(message);
  }
};

const nextConfig = read('next.config.ts');
const relationshipWorkspace = read('components/relationships/relationship-workspace.tsx');
const detailPanel = read('components/relationships/relationship-detail-panel.tsx');
const teamsAgentProfile = read('components/teams/teams-agent-profile.tsx');
const agentProfileTab = read('components/agents/agent-profile-tab.tsx');
const relationshipEdge = read('components/relationships/relationship-edge.tsx');
const relationshipNode = read('components/relationships/relationship-node.tsx');
const typeSelector = read('components/relationships/type-selector.tsx');
const teamsAgentWorkspace = read('components/teams/teams-agent-workspace.tsx');
const button = read('components/ui/button.tsx');
const detailSection = read('components/ui/detail-section.tsx');
const dialog = read('components/ui/dialog.tsx');
const panelHeader = read('components/ui/panel-header.tsx');
const selectableRow = read('components/ui/selectable-row.tsx');
const channelList = read('components/dashboard/channel-list.tsx');
const dmList = read('components/dashboard/dm-list.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const channelSearch = read('components/dashboard/channel-search.tsx');
const dmView = read('components/dashboard/dm-view.tsx');
const inboxView = read('components/inbox/inbox-view.tsx');
const tasksLeftColumn = read('components/tasks/tasks-left-column.tsx');
const createTaskModal = read('components/tasks/create-task-modal.tsx');
const createRelationshipModal = read('components/relationships/create-relationship-modal.tsx');
const navbar = read('components/ui/navbar.tsx');
const computersPage = read('app/computers/page.tsx');
const tabBar = read('components/ui/tab-bar.tsx');
const brutalCss = read('app/globals.brutal.css');
const splitDragHotPath = channelView.match(/const scheduleSplitResize[\s\S]*?\n  }, \[[^\]]*\]\);/)?.[0] ?? '';

assert(
  !exists('app/relationships/page.tsx'),
  'standalone /relationships route should be removed',
);
assert(
  !exists('app/workspace/page.tsx'),
  'standalone /workspace route should be removed',
);
assert(
  !exists('app/teams/page.tsx') &&
    !nextConfig.includes("source: '/teams'"),
  'standalone /teams should be removed without a redirect',
);
assert(
  !navbar.includes("href: '/teams'") && !computersPage.includes("router.push('/teams')"),
  'retired /teams should not remain in navigation or computer agent links',
);
assert(
  relationshipWorkspace.includes('export function RelationshipWorkspace'),
  'relationship workspace should remain available inside channels',
);
assert(
  !relationshipWorkspace.includes('MiniMap'),
  'embedded relationship workspace should not render the ReactFlow minimap',
);
assert(
  detailPanel.includes('TeamsAgentProfile') && detailPanel.includes('TeamsAgentWorkspace'),
  'agent node detail should reuse the existing Teams profile/workspace components',
);
assert(
  detailPanel.includes('handleMessageAgent') &&
    detailPanel.includes('variant="outline"') &&
    !detailPanel.includes('className="inline-flex h-8 flex-shrink-0 items-center gap-1.5 border-2 border-black bg-white px-2.5 font-heading text-[10px] font-black uppercase tracking-wider hover:bg-brutal-primary-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none disabled:opacity-50 transition-all"'),
  'agent detail message action should use the shared brutal Button primitive',
);
assert(
  relationshipWorkspace.includes('AgentForm') &&
    relationshipWorkspace.includes('useAgents(activeChannelFilterId)') &&
    !relationshipWorkspace.includes('applyTemplate'),
  'relationship workspace should create only fresh channel-scoped agents',
);
assert(
  relationshipWorkspace.includes("import { Button } from '@/components/ui/button'") &&
    relationshipWorkspace.includes('variant="outline"') &&
    relationshipWorkspace.includes('variant="success"') &&
    !relationshipWorkspace.includes('className="flex items-center gap-1 h-8 px-2 border-2 border-black bg-white hover:bg-brutal-primary-light disabled:opacity-30"') &&
    !relationshipWorkspace.includes('className="flex items-center gap-1.5 h-8 px-3 border-2 border-black bg-white hover:bg-brutal-info-light font-heading text-xs font-bold uppercase tracking-wider"'),
  'relationship toolbar actions should use the shared brutal Button primitive instead of raw button classes',
);
assert(
  !relationshipWorkspace.includes('+ Agent'),
  'toolbar should not show a duplicate plus in the Agent button label',
);
assert(
  detailPanel.includes('panelWidth') && detailPanel.includes('cursor-col-resize'),
  'agent detail panel should be resizable like the channel thread panel',
);
assert(
  channelView.includes('role="separator"') &&
    channelView.includes('aria-orientation="vertical"') &&
    channelView.includes('setPointerCapture') &&
    channelView.includes('MIN_SPLIT_PANE_WIDTH') &&
    channelView.includes('handleSplitKeyDown') &&
    channelView.includes('requestAnimationFrame') &&
    channelView.includes("conversationPanelRef.current.style.flexBasis = `${percent}%`") &&
    splitDragHotPath.includes('requestAnimationFrame') &&
    !splitDragHotPath.includes('setConversationPanelPercent'),
  'channel conversation and workspace panes should share an accessible draggable divider',
);
assert(
  detailPanel.includes('showProfileHeader={false}'),
  'embedded agent profile should hide its duplicate avatar header',
);
assert(
  !detailPanel.includes('<div className="tab">Runtime</div>'),
  'agent node detail should not add a standalone Runtime tab',
);
assert(
  !detailPanel.includes('redirectAfterDelete') && !teamsAgentProfile.includes('/teams'),
  'embedded agent profile should delete in-place without referencing the retired teams route',
);
assert(
  relationshipWorkspace.includes('onAgentDeleted={handleAgentDeleted}'),
  'relationship graph should refresh agents after deleting one from the embedded profile',
);
assert(
  teamsAgentProfile.includes('flex h-full flex-col') && teamsAgentProfile.includes('border-t-2 border-black p-4 bg-brutal-cream'),
  'agent delete action should be fixed in a bottom footer instead of hidden at the end of the scroll content',
);
assert(
  !teamsAgentProfile.includes('BrutalSeparator') && !agentProfileTab.includes('<BrutalSeparator'),
  'agent detail should use boxed sections instead of long separator lines',
);
assert(
  detailSection.includes('export function detailSectionClass') &&
    detailSection.includes('export function detailSectionTitleClass') &&
    detailSection.includes('export function detailFieldLabelClass') &&
    agentProfileTab.includes('detailSectionClass') &&
    agentProfileTab.includes('detailSectionTitleClass') &&
    detailPanel.includes('detailSectionClass') &&
    detailPanel.includes('detailSectionTitleClass') &&
    teamsAgentProfile.includes('detailSectionClass') &&
    !agentProfileTab.includes("style={{ transform: 'rotate(-0.8deg)' }}"),
  'Agent and relationship detail sections should share the same Card-like brutal section primitives',
);
assert(
  button.includes('success: "btn-brutal-success"') &&
    brutalCss.includes('.btn-brutal-success') &&
    agentProfileTab.includes('variant="success"') &&
    createRelationshipModal.includes('variant="success"') &&
    detailPanel.includes('variant="success"') &&
    typeSelector.includes('variant="success"') &&
    !agentProfileTab.includes('bg-brutal-success text-black font-heading text-[10px]') &&
    !createRelationshipModal.includes('btn-brutal-sm bg-brutal-success') &&
    !typeSelector.includes('btn-brutal-xs') &&
    !detailPanel.includes('bg-brutal-success text-black font-heading text-[10px]'),
  'Agent and relationship edit save actions should use the shared brutal Button success variant',
);
assert(
  createRelationshipModal.includes("import { Button } from '@/components/ui/button'") &&
    createRelationshipModal.includes('size="icon"') &&
    !createRelationshipModal.includes('className="flex h-10 w-10 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-primary-light disabled:opacity-30"') &&
    !createRelationshipModal.includes('className="btn-brutal-sm px-4 py-1.5"'),
  'create relationship modal actions should use shared brutal Button primitives',
);
assert(
  relationshipWorkspace.includes('selected: e.id === edge.id') && relationshipEdge.includes('selected ? 4'),
  'relationship edges should show a visible selected state after click',
);
assert(
  relationshipEdge.includes("cursor: 'pointer'") && relationshipEdge.includes('hover:-translate-y-0.5'),
  'relationship edges should use the same pointer and press feedback as agent nodes',
);
assert(
  !relationshipWorkspace.includes('relationshipEditorDeleteEdge'),
  'toolbar should not show a duplicate delete relationship action',
);
assert(
  relationshipNode.includes('selected ?') && relationshipNode.includes('bg-brutal-primary'),
  'agent nodes should show a visible selected state after click',
);
assert(
  detailPanel.includes('Math.max(width, 720)') && detailPanel.includes('hasUserResizedPanel'),
  'workspace tab should expand the drawer without overriding user-resized width',
);
assert(
  teamsAgentWorkspace.includes('useState(160)') && teamsAgentWorkspace.includes('Math.max(120, Math.min(240'),
  'workspace file pane should default narrow and be resizable within drawer-friendly bounds',
);
assert(
  teamsAgentWorkspace.includes('firstFilePath') && teamsAgentWorkspace.includes('void handleSelect(path'),
  'workspace drawer should auto-select the first file',
);
assert(
  teamsAgentWorkspace.includes('Maximize2') && teamsAgentWorkspace.includes('fixed inset-0'),
  'workspace drawer should fullscreen in-place instead of linking away',
);
assert(
  teamsAgentWorkspace.includes('agents/<span') && teamsAgentWorkspace.includes('border-b-4 border-black'),
  'workspace preview should keep a brutal header aligned with the file pane',
);
assert(
  !teamsAgentWorkspace.includes('href={`/workspace?agent=${agentId}`}') && !navbar.includes("href: '/workspace'"),
  'workspace should not be exposed as a separate left-nav tab',
);
assert(
  selectableRow.includes('export function selectableRowClass') &&
    channelList.includes('selectableRowClass') &&
    dmList.includes('selectableRowClass') &&
    tasksLeftColumn.includes('selectableRowClass'),
  'Dashboard and Tasks list selections should share the same selected-row primitive',
);
assert(
  selectableRow.includes('export function selectableRowIconClass') &&
    channelList.includes('selectableRowIconClass') &&
    tasksLeftColumn.includes('selectableRowIconClass'),
  'Dashboard and Tasks channel rows should share the same selected-row icon primitive',
);
assert(
  button.includes('export function iconActionClass') &&
    channelView.includes('DialogCloseButton') &&
    dialog.includes('iconActionClass') &&
    detailPanel.includes('iconActionClass') &&
    createTaskModal.includes('DialogCloseButton'),
  'Drawer and modal close buttons should share the same icon action primitive',
);
assert(
  panelHeader.includes('export function panelHeaderClass') &&
    panelHeader.includes('export function panelTitleClass') &&
    detailPanel.includes('panelHeaderClass') &&
    detailPanel.includes('panelTitleClass'),
  'Relationship and agent drawer headers should share the same panel-header primitive',
);
assert(
  tabBar.includes('export function tabButtonClass') &&
    channelView.includes('tabButtonClass') &&
    dmView.includes('tabButtonClass'),
  'Dashboard message/task tabs should share the same tab-button primitive',
);
assert(
  !channelView.includes("hasViewParam || mainPanel !== 'thread' || workspaceView !== 'team'") &&
    !channelView.includes("router.replace(buildDashboardHref(channel.id, {\n      view: 'task'"),
  'Opening a channel thread should not automatically switch the right workspace to the task board',
);
assert(
  channelView.includes('view: workspaceView') &&
    channelView.includes('[channel.id, messages, onThreadChange, pushDashboardState, workspaceView]'),
  'Opening a task thread should preserve the current workspace tab',
);
assert(
  channelView.includes('latestTaskByAgent') &&
    channelView.includes("TEAM_TASK_VISIBLE_STATUSES = new Set<TaskStatus>(['in_progress', 'in_review'])") &&
    channelView.includes('!TEAM_TASK_VISIBLE_STATUSES.has(task.status)') &&
    channelView.includes('agentTasks={latestTaskByAgent}') &&
    channelView.includes('onOpenTask={handleTeamTaskOpen}') &&
    channelView.includes('onOpenTaskArtifact={handleTeamTaskArtifactOpen}'),
  'Team view should attach each agent latest in-progress/in-review task without switching to the task board',
);
assert(
  channelSearch.includes("import { Button } from '@/components/ui/button'") &&
    channelSearch.includes('className="h-8 w-8 p-0"') &&
    channelView.includes('className="h-8 w-8 p-0"') &&
    !channelSearch.includes('className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-cream transition-colors"') &&
    !channelView.includes('className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-cream transition-colors"'),
  'Channel header icon actions should use shared brutal Button primitives',
);
assert(
  inboxView.includes("import { Button } from '@/components/ui/button'") &&
    inboxView.includes('variant="outline"') &&
    inboxView.includes('variant="primary"') &&
    !inboxView.includes('className="border-2 border-black bg-white px-3 py-1 text-xs font-heading font-bold shadow-brutal-sm hover:bg-brutal-cream active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"') &&
    !inboxView.includes('className="border-2 border-black bg-brutal-primary px-3 py-1 text-xs font-heading font-bold text-black shadow-brutal-sm hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"'),
  'Inbox header actions should use shared brutal Button primitives',
);
assert(
  !exists('components/agents/agent-detail-panel.tsx') &&
    !exists('components/agents/agent-workspace-tab.tsx') &&
    !exists('components/workspace/agent-selector.tsx') &&
    !exists('components/workspace/breadcrumb.tsx') &&
    !exists('components/workspace/resizable-panel.tsx'),
  'legacy standalone workspace and agent detail components should be deleted',
);

console.log('team relationship-first source checks passed');
