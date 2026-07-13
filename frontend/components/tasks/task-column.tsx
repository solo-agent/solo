// ============================================================================
// TaskColumn — single kanban column for a given status
// - Column header with status label + task count
// - Task cards: #number + title + status badge + reply count + last activity + claim/unclaim
// - Empty state: "暂无任务" hint
// - Neubrutalist styling: card-brutal, border-2
// ============================================================================

'use client';

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { Clock, ChevronRight, ChevronDown, FileText } from 'lucide-react';
import type { Task, TaskStatus } from '@/lib/types';
import { t } from '@/lib/i18n';
import { getTaskArtifactAction, taskArtifactActionLabel } from '@/lib/utils/task-artifact';
import { TaskActionButtons } from './task-action-buttons';

// ---- Status display config ----
// v3.3: shadowClass drives the hover color-coded shadow on each card.
// Static card keeps the neutral .shadow-brutal-lg (7px black), hover swaps
// to a tinted 9px shadow in the status color so the kanban reads as a
// status-coded heat map without adding any new visual primitive.

export const STATUS_COLUMN_CONFIG: Record<
  TaskStatus,
  { bgClass: string; textClass: string; shadowClass: string }
> = {
  todo: {
    bgClass: 'bg-brutal-warning',
    textClass: 'text-black',
    shadowClass: 'hover:shadow-brutal-warning',
  },
  in_progress: {
    bgClass: 'bg-brutal-info',
    textClass: 'text-black',
    shadowClass: 'hover:shadow-brutal-info',
  },
  in_review: {
    bgClass: 'bg-brutal-violet',
    textClass: 'text-black',
    shadowClass: 'hover:shadow-brutal-violet',
  },
  done: {
    bgClass: 'bg-brutal-success',
    textClass: 'text-black',
    shadowClass: 'hover:shadow-brutal-success',
  },
  closed: {
    bgClass: 'bg-brutal-muted',
    textClass: 'text-black',
    shadowClass: 'hover:shadow-brutal-accent',
  },
};

function taskColumnHeader(status: TaskStatus): string {
  return {
    todo: t('statusTodo'),
    in_progress: t('statusInProgress'),
    in_review: t('statusInReview'),
    done: t('statusDone'),
    closed: t('statusClosed'),
  }[status];
}

// ---- Helpers ----

function formatRelativeTime(iso?: string): string {
  if (!iso) return '';
  try {
    const now = Date.now();
    const d = new Date(iso).getTime();
    const diffMs = now - d;
    if (diffMs < 0) return t('justNow');
    const secs = Math.floor(diffMs / 1000);
    if (secs < 60) return t('justNow');
    const mins = Math.floor(secs / 60);
    if (mins < 60) return t('minutesAgo', { n: mins });
    const hours = Math.floor(mins / 60);
    if (hours < 24) return t('hoursAgo', { n: hours });
    const days = Math.floor(hours / 24);
    if (days < 30) return t('daysAgo', { n: days });
    const pad = (n: number) => String(n).padStart(2, '0');
    const date = new Date(iso);
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
  } catch {
    return '';
  }
}

// ---- Task card ----

interface TaskCardProps {
  task: Task;
  childTasks?: Task[];
  onClick: (task: Task) => void;
  parentTaskNumber?: number;
  onParentClick?: (taskId: string) => void;
  onActionComplete?: (task: Task) => void;
  onGenerateArtifact?: (task: Task) => void;
  isArtifactGenerating?: boolean;
  selectedTaskId?: string | null;
}

function TaskCard({
  task,
  childTasks = [],
  onClick,
  parentTaskNumber,
  onParentClick,
  onActionComplete,
  onGenerateArtifact,
  isArtifactGenerating,
  selectedTaskId,
}: TaskCardProps) {
  const [subtasksOpen, setSubtasksOpen] = useState(true);
  const taskNum = task.task_number ? `#${task.task_number}` : null;
  const isClaimed = !!task.claimer_id;
  const hasSubtasks = childTasks.length > 0 || (task.subtask_count ?? 0) > 0;
  const isChild = !!task.parent_task_id;
  const isSelected = selectedTaskId === task.id;

  const claimerDisplay =
    task.claimer_name ||
    task.assignee_name ||
    (task.claimer_id ? task.claimer_id.slice(0, 8) : null);
  const claimerDeletedSuffix = task.claimer_deleted ? ' (Deleted)' : '';

  const lastActivity = task.updated_at || task.created_at;
  const artifactAction = getTaskArtifactAction(task, isArtifactGenerating);

  return (
    <div
      data-task-id={task.id}
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
        'group card-brutal w-full cursor-pointer text-left',
        isSelected && 'bg-brutal-primary-light',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-primary focus-visible:ring-offset-2',
      )}
      style={{
        ['--card-hover-shadow' as string]: task.status === 'todo' ? 'var(--color-brutal-warning)' :
          task.status === 'in_progress' ? 'var(--color-brutal-info)' :
          task.status === 'in_review' ? 'var(--color-brutal-violet)' :
          task.status === 'done' ? 'var(--color-brutal-success)' :
          'var(--color-brutal-accent)',
      }}
    >
      <div className="p-3 relative">
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
                className="text-[10px] text-muted-foreground hover:text-foreground underline decoration-dotted underline-offset-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-primary"
              >
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}{t('subTask')} of {parentTaskNumber ? `#${parentTaskNumber}` : t('parentTask')}
              </span>
            ) : (
              <span className="text-[10px] text-muted-foreground">
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}{t('subTask')}{parentTaskNumber ? ` of #${parentTaskNumber}` : ''}
              </span>
            )}
          </div>
        )}

        <div className="mt-2 flex items-center gap-2">
          {isClaimed ? (
            <>
              <span className="flex h-5 w-5 items-center justify-center border-2 border-black bg-brutal-success font-heading text-[10px] font-bold text-black">
                {(claimerDisplay || '?').charAt(0).toUpperCase()}
              </span>
              <span className="min-w-0 flex-1 truncate font-body text-[11px] font-medium text-foreground">
                {claimerDisplay}{claimerDeletedSuffix}
              </span>
            </>
          ) : (
            <span className="font-body text-[11px] text-muted-foreground">
              {t('unclaimed')}
            </span>
          )}
        </div>

        <TaskActionButtons task={task} onActionComplete={onActionComplete} />

        {/* Subtask progress bar (parent task) */}
        {hasSubtasks && (
          <div className="mt-2 pt-2 border-t-2 border-brutal-muted">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setSubtasksOpen((v) => !v);
              }}
              className="flex w-full items-center gap-1.5 text-left text-[10px] font-heading font-black uppercase tracking-wider"
            >
              {subtasksOpen ? (
                <ChevronDown className="h-3 w-3 flex-shrink-0" />
              ) : (
                <ChevronRight className="h-3 w-3 flex-shrink-0" />
              )}
              <span className="border-2 border-black bg-brutal-cream px-1.5 py-0.5 text-black">
                {t('subTaskLabel').replace(':', '')}
              </span>
              <span className="border-2 border-black bg-brutal-success px-1.5 py-0.5 text-black">
                {task.done_subtask_count ?? 0}/{task.subtask_count}
              </span>
              <div className="flex-1 h-2 border-2 border-black bg-muted">
                <div
                  className="h-full bg-brutal-success"
                  style={{ width: `${Math.min(((task.done_subtask_count ?? 0) / (task.subtask_count ?? 1)) * 100, 100)}%` }}
                />
              </div>
            </button>

            {subtasksOpen && childTasks.length > 0 && (
              <div className="mt-2 space-y-1.5">
                {childTasks.map((child) => (
                  <button
                    key={child.id}
                    data-task-id={child.id}
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onClick(child);
                    }}
                    className={cn(
                      'w-full border-2 border-black bg-white px-2 py-1.5 text-left shadow-brutal-sm transition-transform hover:-translate-y-0.5',
                      selectedTaskId === child.id && 'bg-brutal-primary-light',
                    )}
                  >
                    <div className="flex items-center gap-1.5">
                      <span className={cn('h-2 w-2 flex-shrink-0 border border-black', STATUS_COLUMN_CONFIG[child.status].bgClass)} />
                      <span className="font-mono text-[10px] text-muted-foreground">
                        {child.task_number ? `#${child.task_number}` : t('subTask')}
                      </span>
                      <span className="min-w-0 flex-1 truncate font-body text-[11px] font-bold text-foreground">
                        {child.title}
                      </span>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Footer: last activity */}
        <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
          {onGenerateArtifact && artifactAction !== 'hidden' && (
            <button
              type="button"
              disabled={artifactAction === 'pending'}
              onClick={(e) => {
                e.stopPropagation();
                if (artifactAction === 'pending') return;
                onGenerateArtifact(task);
              }}
              className={cn(
                'inline-flex items-center gap-1 border-2 border-black px-2 py-1 font-mono text-[10px] font-bold uppercase shadow-brutal-sm transition-all duration-100 hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none disabled:pointer-events-none disabled:opacity-80 disabled:hover:translate-x-0 disabled:hover:translate-y-0 disabled:hover:shadow-brutal-sm',
                artifactAction === 'generate' && 'bg-brutal-success text-black',
                artifactAction === 'pending' && 'bg-brutal-muted text-black',
                artifactAction === 'read' && 'bg-brutal-primary text-black animate-bounce-slow',
              )}
              aria-label={`Generate artifact for ${task.title}`}
            >
              <FileText className="h-3 w-3" />
              {taskArtifactActionLabel(artifactAction)}
            </button>
          )}
          <span className="flex items-center">
            <Clock className="mr-1 h-3 w-3" />
            {formatRelativeTime(lastActivity)}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---- Column skeleton ----

function ColumnSkeleton({ status }: { status: TaskStatus }) {
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
  parentTaskMap?: Map<string, number>;
  onParentClick?: (taskId: string) => void;
  childrenByParent?: Map<string, Task[]>;
  onActionComplete?: (task: Task) => void;
  onGenerateArtifact?: (task: Task) => void;
  isArtifactGenerating?: (task: Task) => boolean;
  selectedTaskId?: string | null;
}

// ---- Component ----

export function TaskColumn({
  status,
  tasks,
  isLoading,
  onTaskClick,
  parentTaskMap,
  onParentClick,
  childrenByParent,
  onActionComplete,
  onGenerateArtifact,
  isArtifactGenerating,
  selectedTaskId,
}: TaskColumnProps) {
  const label = taskColumnHeader(status);
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
            childTasks={childrenByParent?.get(task.id)}
            onClick={onTaskClick}
            parentTaskNumber={task.parent_task_id ? parentTaskMap?.get(task.parent_task_id) : undefined}
            onParentClick={onParentClick}
            onActionComplete={onActionComplete}
            onGenerateArtifact={onGenerateArtifact}
            isArtifactGenerating={isArtifactGenerating?.(task)}
            selectedTaskId={selectedTaskId}
          />
        ))}

        {/* Empty state */}
        {count === 0 && (
          <div className="flex items-center justify-center rounded-none border-2 border-dashed border-black py-12 px-4">
            <p className="text-center font-body text-xs text-muted-foreground">
              {t('noTasks')}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
