import type { Task } from '@/lib/types';
import { t } from '@/lib/i18n';

export type TaskArtifactAction = 'hidden' | 'generate' | 'pending' | 'read';

export function getTaskArtifactAction(task: Task | null | undefined, isGenerating = false): TaskArtifactAction {
  if (!task || task.parent_task_id) return 'hidden';
  const status = isGenerating ? 'pending' : (task.artifact_status ?? 'none');
  if (status === 'available') return 'read';
  if (task.status === 'in_review') {
    if (status === 'pending') return 'pending';
    return 'generate';
  }
  return 'hidden';
}

export function taskArtifactActionLabel(action: TaskArtifactAction): string {
  if (action === 'generate') return t('taskArtifactGenerate');
  if (action === 'pending') return t('taskArtifactGenerating');
  return t('taskArtifactRead');
}
