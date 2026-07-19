// ============================================================================
// TaskBoard — 5-column kanban board for tasks
// - Columns: TODO | IN PROGRESS | IN REVIEW | DONE | CLOSED
// - Horizontal scroll on desktop, stack on mobile
// - Loading: 5 column skeletons
// - Error: error banner with retry
// - Groups tasks by status into columns
// ============================================================================

'use client';

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { TaskColumn } from './task-column';
import type { Task, TaskStatus } from '@/lib/types';
import { t } from '@/lib/i18n';
import { motionScrollBehavior } from '@/lib/motion';

// ---- Constants ----

const ALL_STATUSES: TaskStatus[] = [
  'todo',
  'in_progress',
  'in_review',
  'done',
  'closed',
];

// ---- Props ----

interface TaskBoardProps {
  tasks: Task[];
  isLoading: boolean;
  error: string | null;
  onTaskClick: (task: Task) => void;
  onRefetch: () => void;
  onActionComplete?: (task: Task) => void;
  onGenerateArtifact?: (task: Task) => void;
  isArtifactGenerating?: (task: Task) => boolean;
  selectedTaskId?: string | null;
}

// ---- Component ----

export function TaskBoard({
  tasks,
  isLoading,
  error,
  onTaskClick,
  onRefetch,
  onActionComplete,
  onGenerateArtifact,
  isArtifactGenerating,
  selectedTaskId,
}: TaskBoardProps) {
  const boardRef = useRef<HTMLDivElement>(null);
  // Group tasks by status
  const { tasksByStatus, childrenByParent } = useMemo(() => {
    const taskById = new Map(tasks.map((task) => [task.id, task]));
    const children = new Map<string, Task[]>();
    const map: Record<TaskStatus, Task[]> = {
      todo: [],
      in_progress: [],
      in_review: [],
      done: [],
      closed: [],
    };
    for (const task of tasks) {
      if (task.parent_task_id && taskById.has(task.parent_task_id)) {
        const siblings = children.get(task.parent_task_id) ?? [];
        siblings.push(task);
        children.set(task.parent_task_id, siblings);
        continue;
      }
      const status = map[task.status] ? task.status : 'todo';
      map[status].push(task);
    }
    return { tasksByStatus: map, childrenByParent: children };
  }, [tasks]);

  // Build parent task number lookup map (id -> task_number)
  const parentTaskMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const task of tasks) {
      if (task.task_number != null) {
        map.set(task.id, task.task_number);
      }
    }
    return map;
  }, [tasks]);

  // Full task lookup map for navigation (id -> task)
  const taskByIdMap = useMemo(() => {
    const map = new Map<string, Task>();
    for (const task of tasks) {
      map.set(task.id, task);
    }
    return map;
  }, [tasks]);

  // Handle parent badge click: find parent task and navigate to it
  const handleParentClick = useCallback(
    (parentTaskId: string) => {
      const parentTask = taskByIdMap.get(parentTaskId);
      if (parentTask) {
        onTaskClick(parentTask);
      }
    },
    [onTaskClick, taskByIdMap],
  );

  useEffect(() => {
    if (!selectedTaskId || isLoading) return;
    const el = boardRef.current?.querySelector<HTMLElement>(`[data-task-id="${selectedTaskId}"]`);
    el?.scrollIntoView({ behavior: motionScrollBehavior(), block: 'center', inline: 'center' });
  }, [isLoading, selectedTaskId, tasks.length]);

  // ---- Error state ----
  if (error) {
    return (
      <div className="flex items-center gap-3 border-2 border-brutal-danger bg-brutal-danger-light p-4 shadow-brutal-sm">
        <AlertCircle className="h-5 w-5 flex-shrink-0 text-brutal-danger" />
        <span className="flex-1 font-body text-sm text-foreground">{error}</span>
        <button
          type="button"
          onClick={onRefetch}
          className="btn-brutal btn-brutal-sm flex items-center gap-1.5"
        >
          <RefreshCw className="h-3.5 w-3.5" />
          {t('retry')}
        </button>
      </div>
    );
  }

  return (
    <div ref={boardRef} className="min-w-0">
      {/* Desktop: horizontal scroll container */}
      <div className="hidden md:flex gap-4 overflow-x-auto pb-4 -mx-1 px-1">
        {ALL_STATUSES.map((status) => (
          <TaskColumn
            key={status}
            status={status}
            tasks={tasksByStatus[status]}
            isLoading={isLoading}
            onTaskClick={onTaskClick}
            parentTaskMap={parentTaskMap}
            onParentClick={handleParentClick}
            childrenByParent={childrenByParent}
            onActionComplete={onActionComplete}
            onGenerateArtifact={onGenerateArtifact}
            isArtifactGenerating={isArtifactGenerating}
            selectedTaskId={selectedTaskId}
          />
        ))}
      </div>

      {/* Mobile: vertical stack */}
      <div className="flex flex-col gap-6 md:hidden">
        {ALL_STATUSES.map((status) => (
          <TaskColumn
            key={status}
            status={status}
            tasks={tasksByStatus[status]}
            isLoading={isLoading}
            onTaskClick={onTaskClick}
            parentTaskMap={parentTaskMap}
            onParentClick={handleParentClick}
            childrenByParent={childrenByParent}
            onActionComplete={onActionComplete}
            onGenerateArtifact={onGenerateArtifact}
            isArtifactGenerating={isArtifactGenerating}
            selectedTaskId={selectedTaskId}
          />
        ))}
      </div>
    </div>
  );
}
