import { readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const types = read('lib/types.ts');
const hook = read('lib/hooks/use-task-artifact.ts');
const taskCard = read('components/tasks/task-card.tsx');
const taskBoard = read('components/tasks/task-board.tsx');
const taskColumn = read('components/tasks/task-column.tsx');
const threadPanel = read('components/dashboard/thread-panel.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const dmView = read('components/dashboard/dm-view.tsx');

assert(types.includes('export interface TaskArtifact'), 'TaskArtifact type should exist');
assert(hook.includes('generateArtifact') && hook.includes('/api/v1/tasks/${taskId}/artifact'), 'useTaskArtifact should call the generate endpoint');
assert(taskCard.includes('onGenerateArtifact?: (task: Task) => void') && taskCard.includes('FileText'), 'TaskCard should expose an artifact action');
assert(taskBoard.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskBoard should accept artifact action');
assert(taskColumn.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskColumn should pass artifact action');
assert(threadPanel.includes('onGenerateArtifact?: () => void') && threadPanel.includes('Generate Artifact'), 'ThreadPanel should expose artifact generation');
assert(channelView.includes('useTaskArtifact') && channelView.includes('handleGenerateArtifact') && channelView.includes('<iframe'), 'Channel view should wire artifact generation into an iframe viewer');
assert(dmView.includes('useTaskArtifact') && dmView.includes('handleGenerateArtifact') && dmView.includes('<iframe'), 'DM view should wire artifact generation into an iframe viewer');

console.log('task artifact entrypoint source checks passed');
