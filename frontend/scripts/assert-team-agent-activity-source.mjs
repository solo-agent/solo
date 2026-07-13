import { existsSync, readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const workspace = read('components/relationships/relationship-workspace.tsx');
const node = read('components/relationships/relationship-node.tsx');
const edge = read('components/relationships/relationship-edge.tsx');
const hook = read('lib/hooks/use-team-agent-activity.ts');
const cards = read('components/relationships/relationship-activity-card.tsx');

assert(exists('lib/hooks/use-team-agent-activity.ts'), 'team activity hook should exist');
assert(exists('components/relationships/relationship-activity-card.tsx'), 'relationship activity cards should exist');
assert(workspace.includes('useTeamAgentActivity'), 'relationship workspace should subscribe to team agent activity');
assert(workspace.includes('liveByAgent.get(node.id)'), 'relationship workspace should pass per-agent live activity into node data without rebuilding layout');
assert(workspace.includes('getLatestRunId(agentId)') && workspace.includes('onOpenRun: handleOpenLatestRun'), 'relationship nodes should get Live navigation at creation time for every channel');
assert(workspace.includes('agentTasks?: Record<string, AgentNodeTask | undefined>') && workspace.includes('task: agentTasks?.[a.id]'), 'relationship workspace should pass one latest task card into each agent node');
assert(workspace.includes('onOpenTask?: (taskId: string) => void') && workspace.includes('onOpenTaskArtifact?: (taskId: string) => void'), 'relationship workspace should pass task and artifact open actions into node data');
assert(workspace.includes('taskLayoutKey') && workspace.includes('taskLayoutChanged') && workspace.includes('hasTaskCards'), 'relationship workspace should re-run auto layout when task cards appear or change');
assert(workspace.includes("sourceHandle: 'bottom'") && workspace.includes("targetHandle: 'top'"), 'assigns_to edges should leave from the vertical handles so task cards do not cover the line');
assert(node.includes('liveActivity') && node.includes('RelationshipActivityCard'), 'relationship node should render live activity cards');
assert(node.includes('AgentTaskMiniCard') && node.includes('TASK_STATUS_BORDER') && node.includes('mt-3 flex flex-col items-center') && !node.includes('left-full top-1/2'), 'relationship node should render a connected compact task card below the agent without horizontal overlap');
assert(node.includes('role="button"') && node.includes('onOpenTask?.(task.id)') && node.includes('onOpenTaskArtifact?.(task.id)'), 'relationship task card and artifact badge should be clickable');
assert(node.includes('const hasTask = !!agentData.task') && node.includes('{!hasTask && (') && node.includes('{hasTask && ('), 'relationship node should move the bottom source handle below mounted task cards');
assert(node.includes('artifactClassName') && node.includes('animate-bounce-slow'), 'relationship task card should draw attention to available artifacts');
assert(node.includes('hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal'), 'relationship artifact button should use board-like hover feedback');
assert(node.includes('ACTIVE_DOT_STATUSES') && node.includes("'thinking', 'running', 'streaming'"), 'only thinking/running/streaming should show running dots');
assert(node.includes('RunningDots') && node.includes('animationDelay'), 'relationship node should show staggered running dots for active work');
assert(node.includes('--team-agent-status-color') && node.includes('team-agent-active-halo'), 'relationship node should use the current status color for its active halo');
assert(node.includes('Activity') && node.includes('absolute -right-3 -top-3'), 'relationship node should expose Live as an explicit icon button');
assert(node.includes('<PixelAvatar agentId={agentData.agentId} size="sm" className="flex-shrink-0" />'), 'relationship node avatar should not hide the Live navigation action');
assert(hook.includes("event.type === 'message.new'"), 'team activity hook should cache message.new events');
assert(hook.includes("event.event_type === 'user_message_received'"), 'human message should be driven by user_message_received run events');
assert(hook.includes("payloadText(event.payload, 'message_id')"), 'human message should be linked by message_id, not mention text');
assert(hook.includes('/api/v1/agent-runs/active'), 'team activity should reuse active agent runs for cold start');
assert(hook.includes('if (!next.has(run.agent_id)) {') && hook.includes('latestRunByAgentRef.current.set(run.agent_id, run.id);\n            next.set(run.agent_id, runToLive(run));'), 'cold start should keep the newest active run in both state and event filter');
assert(hook.includes("event.type !== 'agent.run.started' && latestRunId && latestRunId !== event.run_id"), 'old run status updates should not override the current team agent run');
assert(hook.includes("event.type === 'agent.run.finished' && !latestRunId"), 'unknown finished runs should not flash stale team status');
assert(cards.includes('HumanMsgCard') && cards.includes('ToolCard') && cards.includes('ActivityCard'), 'relationship activity cards should cover human, tool, and activity states');
assert(cards.includes('bottom-full left-1/2') && cards.indexOf('activity.currentHumanMsg') < cards.indexOf('activity.currentActivity') && cards.indexOf('activity.currentActivity') < cards.indexOf('activity.currentTool'), 'relationship activity cards should stack above nodes without overlap');
assert(!cards.includes('SubCallCard') && !node.includes('SubCallCard'), 'SubCallCard should stay out of MVP until real runtime subcall events exist');
assert(!edge.includes('pulseUntil') && !edge.includes('pulseEdges'), 'relationship edge should not fake dynamic delegation pulses');

console.log('team agent activity source checks passed');
