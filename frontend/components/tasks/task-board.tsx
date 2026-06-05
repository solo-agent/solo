// ============================================================================
// TaskBoard — 5-column kanban board for tasks (v1.5: DnD support)
// - Columns: TODO | IN PROGRESS | IN REVIEW | DONE | CLOSED
// - Horizontal scroll on desktop, stack on mobile
// - Loading: 5 column skeletons
// - Error: error banner with retry
// - Groups tasks by status into columns
// - DndContext wrapping for drag-and-drop between columns
// ============================================================================

'use client';

import { useCallback, useMemo, useState } from 'react';
import {
  DndContext,
  DragOverlay,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { TaskColumn } from './task-column';
import type { Task, TaskStatus } from '@/lib/types';

// ---- Constants ----

const ALL_STATUSES: TaskStatus[] = [
  'todo',
  'in_progress',
  'in_review',
  'done',
  'closed',
];

// Valid state transitions
const VALID_TRANSITIONS: Record<TaskStatus, TaskStatus[]> = {
  todo: ['in_progress', 'closed'],
  in_progress: ['in_review', 'closed'],
  in_review: ['done', 'in_progress', 'closed'],
  done: ['closed'],
  closed: ['todo'],
};

function isValidTransition(from: TaskStatus, to: TaskStatus): boolean {
  return VALID_TRANSITIONS[from]?.includes(to) ?? false;
}

// ---- Props ----

interface TaskBoardProps {
  tasks: Task[];
  isLoading: boolean;
  error: string | null;
  onTaskClick: (task: Task) => void;
  onStatusChange: (task: Task, newStatus: TaskStatus) => void;
  onRefetch: () => void;
  onClaim?: (task: Task) => void;
  onUnclaim?: (task: Task) => void;
  /** Called when drag results in an illegal transition */
  onIllegalTransition?: (from: TaskStatus, to: TaskStatus) => void;
}

// ---- Dragging card preview (used in DragOverlay) ----

function DragCardPreview({ task }: { task: Task }) {
  return (
    <div className="card-brutal w-[260px] bg-white opacity-80 p-3 shadow-brutal-lg">
      {task.task_number && (
        <span className="mb-1 block font-mono text-[11px] font-medium text-muted-foreground">
          #{task.task_number}
        </span>
      )}
      <h4 className="mb-1 font-heading text-sm font-bold leading-snug text-foreground">
        {task.title}
      </h4>
    </div>
  );
}

// ---- Component ----

export function TaskBoard({
  tasks,
  isLoading,
  error,
  onTaskClick,
  onStatusChange,
  onRefetch,
  onClaim,
  onUnclaim,
  onIllegalTransition,
}: TaskBoardProps) {
  const [activeDragTask, setActiveDragTask] = useState<Task | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8, // 8px before drag starts, prevents accidental drags on click
      },
    }),
  );

  // Group tasks by status
  const tasksByStatus = useMemo(() => {
    const map: Record<TaskStatus, Task[]> = {
      todo: [],
      in_progress: [],
      in_review: [],
      done: [],
      closed: [],
    };
    for (const task of tasks) {
      const status = map[task.status] ? task.status : 'todo';
      map[status].push(task);
    }
    return map;
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

  // Collect all task IDs per status for SortableContext
  const taskIdsByStatus = useMemo(() => {
    const map: Record<TaskStatus, string[]> = {
      todo: [],
      in_progress: [],
      in_review: [],
      done: [],
      closed: [],
    };
    for (const status of ALL_STATUSES) {
      map[status] = tasksByStatus[status].map((t) => t.id);
    }
    return map;
  }, [tasksByStatus]);

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

  const handleStatusClick = useCallback(
    (task: Task) => {
      if (task.status === 'done' || task.status === 'closed') return;
      const currentIdx = ALL_STATUSES.indexOf(task.status);
      const nextIdx = (currentIdx + 1) % ALL_STATUSES.length;
      onStatusChange(task, ALL_STATUSES[nextIdx]);
    },
    [onStatusChange],
  );

  // ---- Drag and drop handlers ----

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const taskId = event.active.id as string;
      const task = taskByIdMap.get(taskId);
      if (task) {
        setActiveDragTask(task);
      }
    },
    [taskByIdMap],
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActiveDragTask(null);

      const { active, over } = event;
      if (!over) return;

      const taskId = active.id as string;
      const task = taskByIdMap.get(taskId);
      if (!task) return;

      // Determine target status
      let newStatus: TaskStatus | null = null;

      // over.id could be a task ID (dropped on another card) or a droppable column ID
      if (ALL_STATUSES.includes(over.id as TaskStatus)) {
        // Dropped directly on a column's droppable area
        newStatus = over.id as TaskStatus;
      } else {
        // Dropped on another task card — find that task's status
        const overTask = taskByIdMap.get(over.id as string);
        if (overTask) {
          newStatus = overTask.status;
        }
      }

      if (!newStatus) return;

      // Same column: no-op
      if (task.status === newStatus) return;

      // Validate transition
      if (!isValidTransition(task.status, newStatus)) {
        onIllegalTransition?.(task.status, newStatus);
        return;
      }

      // Legal: trigger status change
      onStatusChange(task, newStatus);
    },
    [taskByIdMap, onStatusChange, onIllegalTransition],
  );

  // ---- Error state ----
  if (error) {
    return (
      <div className="flex items-center gap-3 border-2 border-brutal-red bg-brutal-red-light p-4 shadow-brutal-sm">
        <AlertCircle className="h-5 w-5 flex-shrink-0 text-brutal-red" />
        <span className="flex-1 font-body text-sm text-foreground">{error}</span>
        <button
          type="button"
          onClick={onRefetch}
          className="btn-brutal btn-brutal-sm flex items-center gap-1.5"
        >
          <RefreshCw className="h-3.5 w-3.5" />
          重试
        </button>
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      {/* Desktop: horizontal scroll container */}
      <div className="hidden md:flex gap-4 overflow-x-auto pb-4 -mx-1 px-1">
        {ALL_STATUSES.map((status) => (
          <TaskColumn
            key={status}
            status={status}
            tasks={tasksByStatus[status]}
            taskIds={taskIdsByStatus[status]}
            isLoading={isLoading}
            onTaskClick={onTaskClick}
            onStatusClick={handleStatusClick}
            onClaim={onClaim}
            onUnclaim={onUnclaim}
            parentTaskMap={parentTaskMap}
            onParentClick={handleParentClick}
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
            taskIds={taskIdsByStatus[status]}
            isLoading={isLoading}
            onTaskClick={onTaskClick}
            onStatusClick={handleStatusClick}
            onClaim={onClaim}
            onUnclaim={onUnclaim}
            parentTaskMap={parentTaskMap}
            onParentClick={handleParentClick}
          />
        ))}
      </div>

      {/* Drag overlay — semi-transparent card preview */}
      <DragOverlay dropAnimation={null}>
        {activeDragTask ? <DragCardPreview task={activeDragTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
