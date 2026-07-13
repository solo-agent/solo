import { readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const pixelAvatar = read('components/ui/pixel-avatar.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const dmView = read('components/dashboard/dm-view.tsx');
const relationshipWorkspace = read('components/relationships/relationship-workspace.tsx');
const relationshipDetailPanel = read('components/relationships/relationship-detail-panel.tsx');
const messageList = read('components/dashboard/message-list.tsx');
const agentMessage = read('components/dashboard/agent-message.tsx');
const streamingMessage = read('components/dashboard/streaming-message.tsx');
const memberList = read('components/dashboard/member-list.tsx');
const threadPanel = read('components/dashboard/thread-panel.tsx');

assert(
  pixelAvatar.includes('onClick?: () => void') && pixelAvatar.includes('<button'),
  'PixelAvatar should become a button when an onClick handler is provided',
);
assert(
  channelView.includes('workspaceDetail') &&
    channelView.includes('<RelationshipDetailPanel') &&
    channelView.includes("mainPanel === 'thread'") &&
    channelView.includes("mainPanel === 'agent' || mainPanel === 'relationship'") &&
    channelView.includes("panel: 'agent'") &&
    channelView.includes("panel: 'relationship'") &&
    channelView.includes('embedded') &&
    channelView.includes('onAgentClick={openAgentDetail}'),
  'channel view should preserve thread and agent detail state through URL-driven main panels',
);
assert(
  dmView.includes('selectedAgentDetail') &&
    dmView.includes('<RelationshipDetailPanel') &&
    dmView.includes("useState<'thread' | 'agent' | null>(null)") &&
    dmView.includes("setActiveRightPanel('agent')") &&
    dmView.includes("setActiveRightPanel('thread')") &&
    dmView.includes('style={{ width: rightPanelOpen ? threadPanelWidth : 0') &&
    dmView.includes('embedded') &&
    dmView.includes('onAgentClick={openAgentDetail}'),
  'DM view should preserve thread and agent detail state while showing only the active right panel',
);
assert(
  relationshipDetailPanel.includes("'flex h-full flex-col border-l-2") &&
    relationshipDetailPanel.includes("panelHeaderClass(embedded ? 'sidebar-collapse-offset h-14 flex-shrink-0' : undefined)") &&
    relationshipDetailPanel.includes('style={embedded ? undefined : { width: panelWidth }}'),
  'embedded relationship detail should align with dashboard panel height and let the parent own width',
);
assert(
  relationshipWorkspace.includes('const detailPanelOpen = !!detailRel || !!detailAgent') &&
    relationshipWorkspace.includes('style={{ width: detailPanelOpen ? detailPanelWidth : 0') &&
    relationshipWorkspace.includes('embedded') &&
    !relationshipWorkspace.includes('fixed right-0 top-14'),
  'Teams relationship workspace should render detail in a right-side slot instead of a fixed overlay',
);
assert(
  messageList.includes('onAgentClick?: (agent: AgentDetailTarget) => void') &&
    messageList.includes('<AgentMessage') &&
    messageList.includes('onAgentClick={onAgentClick}') &&
    messageList.includes('<StreamingMessage') &&
    messageList.includes('onAgentClick={onAgentClick}'),
  'message list should pass agent avatar clicks to agent message renderers',
);
assert(
  agentMessage.includes('onAgentClick?.({') &&
    agentMessage.includes("ariaLabel={t('viewAgentDetail', { name: message.display_name })}"),
  'agent message avatar should open agent detail',
);
assert(
  streamingMessage.includes('onAgentClick?.({') &&
    streamingMessage.includes("ariaLabel={t('viewAgentDetail', { name: message.display_name })}"),
  'streaming message avatar should open agent detail',
);
assert(
  memberList.includes('onAgentClick?: (agent: AgentDetailTarget) => void') &&
    memberList.includes('onAgentClick?.({') &&
    memberList.includes("ariaLabel={t('viewAgentDetail', { name: member.display_name })}"),
  'channel member agent avatar should open agent detail',
);
assert(
  threadPanel.includes('onAgentClick?: (agent: AgentDetailTarget) => void') &&
    threadPanel.includes('onAgentClick={onAgentClick}') &&
    threadPanel.includes("ariaLabel={t('viewAgentDetail', { name: displayName })}"),
  'thread panel agent avatars should open agent detail',
);

console.log('agent detail avatar entrypoint source checks passed');
