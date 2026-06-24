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
assert(hook.includes('isGeneratingRef') && hook.includes('isGeneratingRef.current'), 'useTaskArtifact should synchronously gate concurrent generation');
assert(taskCard.includes('onGenerateArtifact?: (task: Task) => void') && taskCard.includes('FileText'), 'TaskCard should expose an artifact action');
assert(taskBoard.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskBoard should accept artifact action');
assert(taskBoard.includes('isArtifactGenerating?: boolean'), 'TaskBoard should accept artifact pending state');
assert(taskColumn.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskColumn should pass artifact action');
assert(taskColumn.includes('isArtifactGenerating?: boolean'), 'TaskColumn should pass artifact pending state');
assert(threadPanel.includes('onGenerateArtifact?: () => void') && threadPanel.includes('Generate Artifact'), 'ThreadPanel should expose artifact generation');
assert(threadPanel.includes('isArtifactGenerating?: boolean'), 'ThreadPanel should accept artifact pending state');
assert(channelView.includes('useTaskArtifact') && channelView.includes('handleGenerateArtifact') && channelView.includes('<iframe'), 'Channel view should wire artifact generation into an iframe viewer');
assert(channelView.includes('showToast') && channelView.includes('catch'), 'Channel view should surface artifact generation errors');
assert(channelView.includes('isGenerating') && channelView.includes('isArtifactGenerating={isGenerating}'), 'Channel view should disable artifact actions while generating');
assert(channelView.includes('role="dialog"') && channelView.includes('aria-modal="true"') && channelView.includes("event.key === 'Escape'"), 'Channel artifact viewer should use dialog semantics and Escape close');
assert(channelView.includes('artifactCloseButtonRef') && channelView.includes('artifactReturnFocusRef'), 'Channel artifact viewer should handle focus on open and close');
assert(dmView.includes('useTaskArtifact') && dmView.includes('handleGenerateArtifact') && dmView.includes('<iframe'), 'DM view should wire artifact generation into an iframe viewer');
assert(dmView.includes('showToast') && dmView.includes('catch'), 'DM view should surface artifact generation errors');
assert(dmView.includes('isGenerating') && dmView.includes('isArtifactGenerating={isGenerating}'), 'DM view should disable artifact actions while generating');
assert(dmView.includes('role="dialog"') && dmView.includes('aria-modal="true"') && dmView.includes("event.key === 'Escape'"), 'DM artifact viewer should use dialog semantics and Escape close');
assert(dmView.includes('artifactCloseButtonRef') && dmView.includes('artifactReturnFocusRef'), 'DM artifact viewer should handle focus on open and close');

console.log('task artifact entrypoint source checks passed');
