import { existsSync, readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));
const assert = (condition, message) => {
  if (!condition) {
    throw new Error(message);
  }
};

const teamsPage = read('app/teams/page.tsx');
const relationshipWorkspace = read('components/relationships/relationship-workspace.tsx');
const detailPanel = read('components/relationships/relationship-detail-panel.tsx');
const teamsAgentProfile = read('components/teams/teams-agent-profile.tsx');
const agentProfileTab = read('components/agents/agent-profile-tab.tsx');
const relationshipEdge = read('components/relationships/relationship-edge.tsx');
const relationshipNode = read('components/relationships/relationship-node.tsx');
const teamsAgentWorkspace = read('components/teams/teams-agent-workspace.tsx');
const navbar = read('components/ui/navbar.tsx');

assert(
  !exists('app/relationships/page.tsx'),
  'standalone /relationships route should be removed',
);
assert(
  !exists('app/workspace/page.tsx'),
  'standalone /workspace route should be removed',
);
assert(
  relationshipWorkspace.includes('export function RelationshipWorkspace'),
  'relationship workspace should live as a reusable Teams component',
);
assert(
  teamsPage.includes('<RelationshipWorkspace') && !teamsPage.includes('TeamsLeftColumn'),
  'teams page should render the relationship workspace directly instead of the old TeamsLeftColumn layout',
);
assert(
  detailPanel.includes('TeamsAgentProfile') && detailPanel.includes('TeamsAgentWorkspace'),
  'agent node detail should reuse the existing Teams profile/workspace components',
);
assert(
  relationshipWorkspace.includes('AgentForm') && relationshipWorkspace.includes('Create from Template'),
  'relationship workspace should preserve single-agent and template creation',
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
  detailPanel.includes('showProfileHeader={false}'),
  'embedded agent profile should hide its duplicate avatar header',
);
assert(
  !detailPanel.includes('<div className="tab">Runtime</div>'),
  'agent node detail should not add a standalone Runtime tab',
);
assert(
  detailPanel.includes('redirectAfterDelete={false}') && teamsAgentProfile.includes('redirectAfterDelete = true'),
  'embedded agent profile should delete in-place without redirecting away from the relationship graph',
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
  teamsAgentWorkspace.includes('PanelLeftClose') && teamsAgentWorkspace.includes('isFilePaneCollapsed'),
  'workspace file pane should be collapsible',
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
  teamsAgentWorkspace.includes('Workspace') && teamsAgentWorkspace.includes('Readonly') && teamsAgentWorkspace.includes('border-b-4 border-black'),
  'workspace preview should keep a brutal header aligned with the file pane',
);
assert(
  !teamsAgentWorkspace.includes('href={`/workspace?agent=${agentId}`}') && !navbar.includes("href: '/workspace'"),
  'workspace should not be exposed as a separate left-nav tab from Teams',
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
