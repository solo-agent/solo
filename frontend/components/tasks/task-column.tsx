// ============================================================================
// TaskColumn — single kanban column for a given status (v1.5: DnD support)
// - Column header with status label + task count
// - Task cards: #number + title + status badge + reply count + last activity + claim/unclaim
// - Empty state: "暂无任务" hint
// - Neubrutalist styling: card-brutal, border-2
// - Droppable area for drag-and-drop + Sortable cards
// ============================================================================

'use client';

import { useDroppable } from '@dnd-kit/core';
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { cn } from '@/lib/utils';
import { MessageSquare, Clock, ChevronRight } from 'lucide-react';
import type { Task, TaskStatus } from '@/lib/types';

// ---- Status display config ----

export const STATUS_COLUMN_CONFIG: Record<
  TaskStatus,
  { label: string; bgClass: string; textClass: string }
> = {
  todo: {
    label: 'TODO',
    bgClass: 'bg-brutal-orange',
    textClass: 'text-black',
  },
  in_progress: {
    label: 'IN PROGRESS',
    bgClass: 'bg-brutal-cyan',
    textClass: 'text-black',
  },
  in_review: {
    label: 'IN REVIEW',
    bgClass: 'bg-brutal-lavender',
    textClass: 'text-black',
  },
  done: {
    label: 'DONE',
    bgClass: 'bg-brutal-lime',
    textClass: 'text-black',
  },
  closed: {
    label: 'CLOSED',
    bgClass: 'bg-brutal-stone',
    textClass: 'text-black',
  },
};

const COLUMN_HEADERS: Record<TaskStatus, string> = {
  todo: 'TODO',
  in_progress: 'IN PROGRESS',
  in_review: 'IN REVIEW',
  done: 'DONE',
  closed: 'CLOSED',
};

// ---- Helpers ----

function formatRelativeTime(iso?: string): string {
  if (!iso) return '';
  try {
    const now = Date.now();
    const d = new Date(iso).getTime();
    const diffMs = now - d;
    if (diffMs < 0) return '刚刚';
    const secs = Math.floor(diffMs / 1000);
    if (secs < 60) return '刚刚';
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}分钟前`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}小时前`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}天前`;
    const pad = (n: number) => String(n).padStart(2, '0');
    const date = new Date(iso);
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
  } catch {
    return '';
  }
}

// ---- Sortable mini card for column (v1.5: DnD) ----

interface SortableTaskCardProps {
  task: Task;
  onClick: (task: Task) => void;
  onStatusClick: (task: Task) => void;
  onClaim?: (task: Task) => void;
  onUnclaim?: (task: Task) => void;
  parentTaskNumber?: number;
  onParentClick?: (taskId: string) => void;
}

function SortableTaskCard({
  task,
  onClick,
  onStatusClick,
  onClaim,
  onUnclaim,
  parentTaskNumber,
  onParentClick,
}: SortableTaskCardProps) {
  const {
    setNodeRef,
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: task.id,
    data: { status: task.status },
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
  };

  const statusConf = STATUS_COLUMN_CONFIG[task.status];
  const taskNum = task.task_number ? `#${task.task_number}` : null;
  const isClaimed = !!task.claimer_id;
  const isTerminal = task.status === 'done' || task.status === 'closed';
  const hasSubtasks = (task.subtask_count ?? 0) > 0;
  const isChild = !!task.parent_task_id;

  const claimerDisplay =
    task.claimer_name ||
    task.assignee_name ||
    (task.claimer_id ? task.claimer_id.slice(0, 8) : null);

  const replyCount = task.reply_count ?? 0;
  const lastActivity = task.updated_at || task.created_at;

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      role="button"
      tabIndex={0}
      onClick={() => onClick(task)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick(task);
        }
      }}
      className={cn(
        'card-brutal w-full cursor-pointer text-left touch-none',
        'hover:-translate-y-[1px] hover:shadow-brutal-lg',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-offset-2',
        isDragging && 'opacity-40',
      )}
    >
      <div className="p-3">
        {/* Task number */}
        {taskNum && (
          <span className="mb-1 block font-mono text-[11px] font-medium text-muted-foreground">
            {taskNum}
          </span>
        )}

        {/* Title */}
        <h4 className="mb-2 font-heading text-sm font-bold leading-snug text-foreground">
          {task.title}
        </h4>

        {/* Parent badge (child task) */}
        {isChild && (
          <div className="mb-1.5">
            {onParentClick ? (
              <span
                tabIndex={0}
                onClick={(e) => {
                  e.stopPropagation();
                  onParentClick(task.parent_task_id!);
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    e.stopPropagation();
                    onParentClick(task.parent_task_id!);
                  }
                }}
                className="text-[10px] text-muted-foreground hover:text-foreground underline decoration-dotted underline-offset-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink"
              >
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}子任务 of {parentTaskNumber ? `#${parentTaskNumber}` : '父任务'}
              </span>
            ) : (
              <span className="text-[10px] text-muted-foreground">
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}子任务{parentTaskNumber ? ` of #${parentTaskNumber}` : ''}
              </span>
            )}
          </div>
        )}

        {/* Status badge — clickable for non-terminal only */}
        <span
          onClick={(e) => {
            if (isTerminal) return;
            e.stopPropagation();
            onStatusClick(task);
          }}
          onKeyDown={(e) => {
            if (isTerminal) return;
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              e.stopPropagation();
              onStatusClick(task);
            }
          }}
          className={cn(
            'badge-brutal',
            isTerminal ? '' : 'cursor-pointer hover:opacity-80 transition-opacity focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink',
            statusConf.bgClass,
            statusConf.textClass,
          )}
          aria-label={`状态: ${statusConf?.label || task.status}`}
          tabIndex={isTerminal ? undefined : 0}
        >
          {statusConf.label}
        </span>

        {/* Terminal state marker */}
        {isTerminal ? (
          <div className="mt-2">
            <span className="font-mono text-[11px] font-bold text-muted-foreground">
              {task.status === 'done' ? '✓ 已完成' : '✕ 已关闭'}
            </span>
          </div>
        ) : (
          /* Claimer info + claim/unclaim buttons */
          <div className="mt-2 flex items-center gap-2">
            {isClaimed ? (
              <>
                <span className="flex h-5 w-5 items-center justify-center border-2 border-black bg-brutal-lime font-heading text-[10px] font-bold text-black">
                  {(claimerDisplay || '?').charAt(0).toUpperCase()}
                </span>
                <span className="flex-1 truncate font-body text-[11px] text-foreground font-medium">
                  {claimerDisplay}
                </span>
                <span className="flex-shrink-0 badge-brutal bg-brutal-lime text-black text-[10px]">
                  已认领
                </span>
                {onUnclaim && (
                  <span
                    tabIndex={0}
                    onClick={(e) => {
                      e.stopPropagation();
                      onUnclaim(task);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        e.stopPropagation();
                        onUnclaim(task);
                      }
                    }}
                    className="btn-brutal btn-brutal-sm flex-shrink-0 text-[11px] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink"
                    aria-label="释放任务"
                  >
                    释放
                  </span>
                )}
              </>
            ) : (
              onClaim && (
                <span
                  tabIndex={0}
                  onClick={(e) => {
                    e.stopPropagation();
                    onClaim(task);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      e.stopPropagation();
                      onClaim(task);
                    }
                  }}
                  className="btn-brutal btn-brutal-sm flex-shrink-0 text-[11px] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink"
                  aria-label="认领任务"
                >
                  认领
                </span>
              )
            )}
          </div>
        )}

        {/* Subtask progress bar (parent task) */}
        {hasSubtasks && (
          <div className="mt-2 pt-2 border-t-2 border-black/10">
            <div className="flex items-center gap-1.5 text-[10px]">
              <span className="text-muted-foreground">子任务:</span>
              <span className="font-bold">{task.done_subtask_count ?? 0}/{task.subtask_count}</span>
              <div className="flex-1 h-1 border border-black/20 bg-muted">
                <div
                  className="h-full bg-brutal-lime"
                  style={{ width: `${Math.min(((task.done_subtask_count ?? 0) / (task.subtask_count ?? 1)) * 100, 100)}%` }}
                />
              </div>
            </div>
          </div>
        )}

        {/* Footer: reply count + last activity */}
        <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
          <span className="flex items-center gap-1">
            <MessageSquare className="h-3 w-3" />
            {replyCount > 0 ? `${replyCount}` : '0'}
          </span>
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {formatRelativeTime(lastActivity)}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---- Column skeleton ----

function ColumnSkeleton({ status }: { status: TaskStatus }) {
  const label = COLUMN_HEADERS[status];
  return (
    <div className="flex w-[280px] flex-shrink-0 flex-col">
      <div className="mb-3 flex items-center gap-2 px-1">
        <div className="h-5 w-24 animate-pulse bg-muted" />
        <div className="h-5 w-8 animate-pulse bg-muted" />
      </div>
      <div className="space-y-3">
        {[1, 2, 3].map((i) => (
          <div key={i} className="card-brutal p-3">
            <div className="mb-1 h-3 w-12 animate-pulse bg-muted" />
            <div className="mb-2 h-4 w-3/4 animate-pulse bg-muted" />
            <div className="h-5 w-24 animate-pulse bg-muted" />
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- Props ----

interface TaskColumnProps {
  status: TaskStatus;
  tasks: Task[];
  taskIds: string[];
  isLoading: boolean;
  onTaskClick: (task: Task) => void;
  onStatusClick: (task: Task) => void;
  onClaim?: (task: Task) => void;
  onUnclaim?: (task: Task) => void;
  parentTaskMap?: Map<string, number>;
  onParentClick?: (taskId: string) => void;
}

// ---- Component ----

export function TaskColumn({
  status,
  tasks,
  taskIds,
  isLoading,
  onTaskClick,
  onStatusClick,
  onClaim,
  onUnclaim,
  parentTaskMap,
  onParentClick,
}: TaskColumnProps) {
  const label = COLUMN_HEADERS[status];
  const count = tasks.length;

  // Make the column body a drop target
  const { setNodeRef: setDropRef, isOver } = useDroppable({
    id: status,
    data: { status },
  });

  if (isLoading) {
    return <ColumnSkeleton status={status} />;
  }

  return (
    <div className="flex w-[280px] flex-shrink-0 flex-col">
      {/* Column header — full-saturation color bar */}
      <div
        className={cn(
          'mb-3 flex items-center gap-2 border-2 border-black px-3 py-2 shadow-brutal-sm',
          STATUS_COLUMN_CONFIG[status].bgClass,
          STATUS_COLUMN_CONFIG[status].textClass,
        )}
      >
        <h3 className="font-heading text-sm font-black tracking-tight">
          {label}
        </h3>
        <span className="flex h-5 min-w-[20px] items-center justify-center border-2 border-black bg-white px-1.5 font-mono text-[11px] font-bold text-black">
          {count}
        </span>
      </div>

      {/* Card list — droppable area */}
      <div
        ref={setDropRef}
        className={cn(
          'flex-1 space-y-3 min-h-[100px] p-1 transition-colors',
          isOver && 'bg-brutal-cyan-light/30',
        )}
      >
        <SortableContext items={taskIds} strategy={verticalListSortingStrategy}>
          {tasks.map((task) => (
            <SortableTaskCard
              key={task.id}
              task={task}
              onClick={onTaskClick}
              onStatusClick={onStatusClick}
              onClaim={onClaim}
              onUnclaim={onUnclaim}
              parentTaskNumber={task.parent_task_id ? parentTaskMap?.get(task.parent_task_id) : undefined}
              onParentClick={onParentClick}
            />
          ))}
        </SortableContext>

        {/* Empty state */}
        {count === 0 && (
          <div className="flex items-center justify-center rounded-none border-2 border-dashed border-black py-12 px-4">
            <p className="text-center font-body text-xs text-muted-foreground">
              暂无{label}任务
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
