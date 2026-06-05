// ============================================================================
// TaskColumn — single kanban column for a given status
// - Column header with status label + task count
// - Task cards: #number + title + status badge + reply count + last activity + claim/unclaim
// - Empty state: "暂无任务" hint
// - Neubrutalist styling: card-brutal, border-2
// ============================================================================

'use client';

import { useState, useRef, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { Clock, ChevronRight, ChevronDown } from 'lucide-react';
import type { Task, TaskStatus } from '@/lib/types';

// ---- Valid status transitions ----

const VALID_TRANSITIONS: Record<TaskStatus, TaskStatus[]> = {
  todo: ['in_progress', 'closed'],
  in_progress: ['in_review', 'closed'],
  in_review: ['done', 'in_progress', 'closed'],
  done: ['closed'],
  closed: ['todo'],
};

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

// ---- Status badge with dropdown ----

const STATUS_LABELS: Record<TaskStatus, string> = {
  todo: 'TODO',
  in_progress: 'IN PROGRESS',
  in_review: 'IN REVIEW',
  done: 'DONE',
  closed: 'CLOSED',
};

function StatusBadge({
  status,
  onChange,
}: {
  status: TaskStatus;
  onChange: (newStatus: TaskStatus) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);
  const transitions = VALID_TRANSITIONS[status] ?? [];
  const config = STATUS_COLUMN_CONFIG[status];

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  return (
    <span ref={ref} className="relative inline-flex">
      <span
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className={cn(
          'badge-brutal cursor-pointer hover:opacity-80 transition-opacity inline-flex items-center gap-0.5',
          config.bgClass,
          config.textClass,
        )}
        aria-label={`状态: ${config.label}`}
        tabIndex={0}
      >
        {config.label}
        <ChevronDown className="h-3 w-3" />
      </span>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-30 min-w-[140px] border-2 border-black bg-white shadow-brutal py-1">
          {transitions.map((s) => (
            <button
              key={s}
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onChange(s);
                setOpen(false);
              }}
              className="block w-full text-left px-3 py-1.5 font-heading text-xs font-bold hover:bg-brutal-cream transition-colors"
            >
              {STATUS_LABELS[s]}
            </button>
          ))}
        </div>
      )}
    </span>
  );
}

// ---- Task card ----

interface TaskCardProps {
  task: Task;
  onClick: (task: Task) => void;
  onStatusChange: (task: Task, newStatus: TaskStatus) => void;
  parentTaskNumber?: number;
  onParentClick?: (taskId: string) => void;
}

function TaskCard({
  task,
  onClick,
  onStatusChange,
  parentTaskNumber,
  onParentClick,
}: TaskCardProps) {
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

  const lastActivity = task.updated_at || task.created_at;

  return (
    <div
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
        'card-brutal w-full cursor-pointer text-left',
        'hover:shadow-brutal-lg',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-offset-2',
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

        {/* Status badge — with dropdown for non-terminal */}
        <StatusBadge
          status={task.status}
          onChange={(newStatus) => onStatusChange(task, newStatus)}
        />

        {/* Terminal state marker */}
        {isTerminal ? (
          <div className="mt-2">
            <span className="font-mono text-[11px] font-bold text-muted-foreground">
              {task.status === 'done' ? '✓ 已完成' : '✕ 已关闭'}
            </span>
          </div>
        ) : (
          /* Claimer info — display only, no claim/unclaim buttons */
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
              </>
            ) : (
              <span className="font-body text-[11px] text-muted-foreground">
                待认领
              </span>
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

        {/* Footer: last activity */}
        <div className="mt-2 flex items-center text-[11px] text-muted-foreground">
          <Clock className="mr-1 h-3 w-3" />
          {formatRelativeTime(lastActivity)}
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
  isLoading: boolean;
  onTaskClick: (task: Task) => void;
  onStatusChange: (task: Task, newStatus: TaskStatus) => void;
  parentTaskMap?: Map<string, number>;
  onParentClick?: (taskId: string) => void;
}

// ---- Component ----

export function TaskColumn({
  status,
  tasks,
  isLoading,
  onTaskClick,
  onStatusChange,
  parentTaskMap,
  onParentClick,
}: TaskColumnProps) {
  const label = COLUMN_HEADERS[status];
  const count = tasks.length;

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

      {/* Card list */}
      <div className="flex-1 space-y-3 min-h-[100px]">
        {tasks.map((task) => (
          <TaskCard
            key={task.id}
            task={task}
            onClick={onTaskClick}
            onStatusChange={onStatusChange}
            parentTaskNumber={task.parent_task_id ? parentTaskMap?.get(task.parent_task_id) : undefined}
            onParentClick={onParentClick}
          />
        ))}

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
