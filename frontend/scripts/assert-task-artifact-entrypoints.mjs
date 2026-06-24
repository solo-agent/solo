import { readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const types = read('lib/types.ts');
const apiClient = read('lib/api-client.ts');
const hook = read('lib/hooks/use-task-artifact.ts');
const taskCard = read('components/tasks/task-card.tsx');
const taskBoard = read('components/tasks/task-board.tsx');
const taskColumn = read('components/tasks/task-column.tsx');
const threadPanel = read('components/dashboard/thread-panel.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const dmView = read('components/dashboard/dm-view.tsx');

assert(types.includes('export interface TaskArtifact'), 'TaskArtifact type should exist');
assert(apiClient.includes('getText') && apiClient.includes('processTextResponse'), 'ApiClient should fetch protected artifact HTML as text');
assert(hook.includes('generateArtifact') && hook.includes('/api/v1/tasks/${taskId}/artifact'), 'useTaskArtifact should call the generate endpoint');
assert(hook.includes('finalizeArtifact') && hook.includes('/api/v1/tasks/${taskId}/artifact/finalize'), 'useTaskArtifact should call the finalize endpoint');
assert(hook.includes('waitForPublishedArtifact') && hook.includes('/api/v1/tasks/${taskId}/artifact/latest?mode=${mode}'), 'useTaskArtifact should wait for the published artifact after generation');
assert(hook.includes('fetchArtifactHTML') && hook.includes('apiClient.getText(artifact.url)'), 'useTaskArtifact should fetch artifact HTML with bearer auth');
assert(hook.includes('isGeneratingRef') && hook.includes('isGeneratingRef.current'), 'useTaskArtifact should synchronously gate concurrent generation');
assert(hook.includes('inFlightPromiseRef') && hook.includes('return inFlightPromiseRef.current'), 'useTaskArtifact should return in-flight generation instead of throwing');
assert(hook.includes('inFlightTaskIdRef') && hook.includes('inFlightTaskIdRef.current !== taskId'), 'useTaskArtifact should only reuse in-flight generation for the same task');
assert(hook.includes('TaskArtifactGenerationInProgressError'), 'useTaskArtifact should expose a known different-task concurrency error');
assert(taskCard.includes('onGenerateArtifact?: (task: Task) => void') && taskCard.includes('FileText'), 'TaskCard should expose an artifact action');
assert(taskCard.includes('e.target !== e.currentTarget') && taskCard.includes('e.stopPropagation()'), 'TaskCard should not let nested artifact keydown trigger parent navigation');
assert(taskBoard.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskBoard should accept artifact action');
assert(taskBoard.includes('isArtifactGenerating?: boolean'), 'TaskBoard should accept artifact pending state');
assert(taskColumn.includes('onGenerateArtifact?: (task: Task) => void'), 'TaskColumn should pass artifact action');
assert(taskColumn.includes('isArtifactGenerating?: boolean'), 'TaskColumn should pass artifact pending state');
assert(threadPanel.includes('onGenerateArtifact?: () => void') && threadPanel.includes('Generate Artifact'), 'ThreadPanel should expose artifact generation');
assert(threadPanel.includes('isArtifactGenerating?: boolean'), 'ThreadPanel should accept artifact pending state');
assert(channelView.includes('useTaskArtifact') && channelView.includes('handleGenerateArtifact') && channelView.includes('<iframe'), 'Channel view should wire artifact generation into an iframe viewer');
assert(channelView.includes('showToast') && channelView.includes('catch'), 'Channel view should surface artifact generation errors');
assert(channelView.includes('URL.createObjectURL') && channelView.includes('URL.revokeObjectURL') && channelView.includes('previewUrl'), 'Channel viewer should use revokable blob URLs for protected artifact HTML');
assert(channelView.includes('handleFinalizeArtifact') && channelView.includes('Finalize'), 'Channel viewer should expose final artifact generation');
assert(channelView.includes('isGenerating') && channelView.includes('isArtifactGenerating={isGenerating}'), 'Channel view should disable artifact actions while generating');
assert(channelView.includes('role="dialog"') && channelView.includes('aria-modal="true"') && channelView.includes("event.key === 'Escape'"), 'Channel artifact viewer should use dialog semantics and Escape close');
assert(channelView.includes('artifactCloseButtonRef') && channelView.includes('artifactReturnFocusRef'), 'Channel artifact viewer should handle focus on open and close');
assert(channelView.includes("event.key === 'Tab'") && channelView.includes('artifactOpenLinkRef'), 'Channel artifact viewer should trap Tab focus across viewer controls');
assert(channelView.includes('artifactFinalizeButtonRef'), 'Channel artifact viewer should include finalize in focus handling');
assert(channelView.includes('artifactFrameRef') && channelView.includes('tabIndex={0}'), 'Channel artifact viewer should include iframe in the focus trap');
assert(dmView.includes('useTaskArtifact') && dmView.includes('handleGenerateArtifact') && dmView.includes('<iframe'), 'DM view should wire artifact generation into an iframe viewer');
assert(dmView.includes('showToast') && dmView.includes('catch'), 'DM view should surface artifact generation errors');
assert(dmView.includes('URL.createObjectURL') && dmView.includes('URL.revokeObjectURL') && dmView.includes('previewUrl'), 'DM viewer should use revokable blob URLs for protected artifact HTML');
assert(dmView.includes('handleFinalizeArtifact') && dmView.includes('Finalize'), 'DM viewer should expose final artifact generation');
assert(dmView.includes('isGenerating') && dmView.includes('isArtifactGenerating={isGenerating}'), 'DM view should disable artifact actions while generating');
assert(dmView.includes('role="dialog"') && dmView.includes('aria-modal="true"') && dmView.includes("event.key === 'Escape'"), 'DM artifact viewer should use dialog semantics and Escape close');
assert(dmView.includes('artifactCloseButtonRef') && dmView.includes('artifactReturnFocusRef'), 'DM artifact viewer should handle focus on open and close');
assert(dmView.includes("event.key === 'Tab'") && dmView.includes('artifactOpenLinkRef'), 'DM artifact viewer should trap Tab focus across viewer controls');
assert(dmView.includes('artifactFinalizeButtonRef'), 'DM artifact viewer should include finalize in focus handling');
assert(dmView.includes('artifactFrameRef') && dmView.includes('tabIndex={0}'), 'DM artifact viewer should include iframe in the focus trap');

console.log('task artifact entrypoint source checks passed');
