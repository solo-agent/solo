import { existsSync, readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const serviceAgent = read('../internal/server/service/agent.go');
const activity = read('lib/agent-activity.ts');
const i18n = read('lib/i18n.ts');
const observability = read('components/agents/agent-observability-tab.tsx');
const liveMonitor = read('components/dashboard/live-monitor.tsx');
const appFrame = read('components/layout/app-frame.tsx');
const dashboardPage = read('app/dashboard/page.tsx');
const relationshipNode = read('components/relationships/relationship-node.tsx');
const teamActivity = read('lib/hooks/use-team-agent-activity.ts');

for (const text of ['已接收，正在处理', '仍在运行，暂无可见回复', '仍在运行，暂无新的进度', 'No available daemon to run this agent.']) {
  assert(!serviceAgent.includes(text), `backend should not emit hardcoded product copy: ${text}`);
}

for (const key of ['agentActivityAccepted', 'agentActivityNoVisibleReply', 'agentActivityNoProgress']) {
  assert(i18n.includes(key), `${key} should be translated`);
}

assert(
  activity.includes('ACTIVITY_TEXT_KEYS') &&
    activity.includes('agent.activity.accepted') &&
    activity.includes('agentActivityAccepted'),
  'displayAgentActivity should translate stable activity codes',
);
assert(
  !appFrame.includes('AgentIsland') &&
    !dashboardPage.includes('AgentIsland'),
  'AgentIsland should no longer be mounted in app shell or dashboard',
);
assert(exists('lib/agent-run-types.ts'), 'AgentRunStatus should live in a shared agent-run-types module');
assert(!exists('components/agents/agent-island.tsx'), 'unused AgentIsland component should be removed');
assert(!exists('lib/hooks/use-agent-island.ts'), 'unused AgentIsland hook should be removed');
for (const [name, source] of [
  ['agent activity helper', activity],
  ['relationship node', relationshipNode],
  ['team activity hook', teamActivity],
  ['observability tab', observability],
  ['live monitor', liveMonitor],
]) {
  assert(source.includes("from '@/lib/agent-run-types'"), `${name} should import AgentRunStatus from shared agent-run-types`);
}
assert(observability.includes('displayAgentActivity('), 'observability run lists should display translated activity text');
assert(
  liveMonitor.includes('displayAgentActivity(run.status, run.activity_text, undefined'),
  'timeline titles should translate activity text',
);
assert(
  liveMonitor.includes('const activity = displayAgentActivity(agent.status, agent.activity_text, agent.tool_input_summary)') &&
    liveMonitor.includes('title={activity}') &&
    liveMonitor.includes('group-hover:line-clamp-none') &&
    liveMonitor.includes('group-focus:line-clamp-none'),
  'live monitor agent cards should expose full truncated activity text on hover/focus',
);
assert(
  liveMonitor.includes('latestQuestionSeq') &&
    liveMonitor.includes('transcript.scrollTop = transcript.scrollHeight') &&
    liveMonitor.includes('timeline-question-${latestQuestionSeq}'),
  'live monitor transcript should open at the latest timeline entry',
);

console.log('agent activity i18n source checks passed');
