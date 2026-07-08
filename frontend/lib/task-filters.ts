import type { Task } from '@/lib/types';

export interface TaskTreeFilters {
  channelId?: string | null;
  assignee?: string;
  creator?: string;
  taskId?: string | null;
  taskNumber?: string;
}

function parseTaskNumber(value?: string): number | null | undefined {
  const trimmed = value?.trim();
  if (!trimmed) return undefined;
  const raw = trimmed.startsWith('#') ? trimmed.slice(1) : trimmed;
  if (!/^\d+$/.test(raw)) return null;
  return Number(raw);
}

function matchesTask(task: Task, filters: TaskTreeFilters): boolean {
  const taskId = filters.taskId?.trim();
  if (taskId) {
    const taskNumber = parseTaskNumber(taskId);
    if (
      task.id !== taskId &&
      task.message_id !== taskId &&
      (taskNumber == null || task.task_number !== taskNumber)
    ) return false;
  }
  if (filters.assignee) {
    const claimerVal = task.claimer_id || task.assignee_id || '';
    const claimerName = (task.claimer_name || task.assignee_name || '').toLowerCase();
    const filterVal = filters.assignee.toLowerCase();
    if (claimerVal !== filters.assignee && !claimerName.includes(filterVal)) return false;
  }
  if (filters.creator) {
    const creatorName = (task.creator_name || task.creator_id || '').toLowerCase();
    const filterVal = filters.creator.toLowerCase();
    if (task.creator_id !== filters.creator && !creatorName.includes(filterVal)) return false;
  }
  const taskNumber = parseTaskNumber(filters.taskNumber);
  if (taskNumber === null) return false;
  if (taskNumber !== undefined && task.task_number !== taskNumber) return false;
  return true;
}

export function filterTaskTree(tasks: Task[], filters: TaskTreeFilters): Task[] {
  const scoped = filters.channelId
    ? tasks.filter((task) => task.channel_id === filters.channelId)
    : tasks;
  const hasTreeFilter = !!(filters.assignee || filters.creator || filters.taskId?.trim() || filters.taskNumber?.trim());
  if (!hasTreeFilter) return scoped;

  const taskById = new Map(scoped.map((task) => [task.id, task]));
  const rootByTask = new Map<string, string>();

  for (const task of scoped) {
    rootByTask.set(
      task.id,
      task.parent_task_id && taskById.has(task.parent_task_id)
        ? task.parent_task_id
        : task.id,
    );
  }

  const matchedRoots = new Set<string>();
  for (const task of scoped) {
    if (matchesTask(task, filters)) {
      matchedRoots.add(rootByTask.get(task.id) ?? task.id);
    }
  }

  return scoped.filter((task) => matchedRoots.has(rootByTask.get(task.id) ?? task.id));
}
