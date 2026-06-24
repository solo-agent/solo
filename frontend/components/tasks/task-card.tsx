// ============================================================================
// TaskCard — individual task card with status badge, priority, assignee
// - card-brutal with hover lift effect
// - Badge colors for status and priority
// - Click handler for navigation to detail page
// ============================================================================

'use client';

import { cn } from '@/lib/utils';
import { Calendar, User, ChevronRight, FileText } from 'lucide-react';
import type { Task, TaskStatus, TaskPriority } from '@/lib/types';
import { t } from '@/lib/i18n';

// ---- Status display config ----
// v3.3: shadowClass powers hover color-coded shadow (status as visual info).
// Static card keeps the neutral 12px black shadow; hover swaps to a tinted
// 12px shadow in the status color so the list reads like a temperature gauge.

const STATUS_CONFIG: Record<TaskStatus, { label: string; bgClass: string; shadowClass: string }> = {
  todo: { label: 'TODO', bgClass: 'bg-brutal-warning text-black', shadowClass: 'hover:shadow-brutal-warning' },
  in_progress: { label: 'IN PROGRESS', bgClass: 'bg-brutal-info text-black', shadowClass: 'hover:shadow-brutal-info' },
  in_review: { label: 'IN REVIEW', bgClass: 'bg-brutal-violet text-black', shadowClass: 'hover:shadow-brutal-violet' },
  done: { label: 'DONE', bgClass: 'bg-brutal-success text-black', shadowClass: 'hover:shadow-brutal-success' },
  closed: { label: 'CLOSED', bgClass: 'bg-brutal-muted text-black', shadowClass: 'hover:shadow-brutal-accent' },
};

const PRIORITY_CONFIG: Record<TaskPriority, { label: string; bgClass: string }> = {
  urgent: { label: t('priorityUrgent'), bgClass: 'bg-brutal-danger text-white' },
  high: { label: t('priorityHigh'), bgClass: 'bg-brutal-warning text-black' },
  normal: { label: t('priorityNormal'), bgClass: 'bg-brutal-cream text-foreground border-2 border-black' },
  low: { label: t('priorityLow'), bgClass: 'bg-brutal-muted text-black' },
};

// ---- Helpers ----

function formatDate(iso?: string): string {
  if (!iso) return '';
  try {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  } catch {
    return iso;
  }
}

// ---- Props ----

interface TaskCardProps {
  task: Task;
  onClick?: (task: Task) => void;
  /** Show channel_name (default true, set false when used inside a channel view) */
  showChannel?: boolean;
  /** Parent task number lookup (key = task id, value = task_number) */
  parentTaskNumber?: number;
  /** Called when the parent badge is clicked */
  onParentClick?: (taskId: string) => void;
  onGenerateArtifact?: (task: Task) => void;
  isArtifactGenerating?: boolean;
}

// ---- Component ----

export function TaskCard({ task, onClick, showChannel = true, parentTaskNumber, onParentClick, onGenerateArtifact, isArtifactGenerating }: TaskCardProps) {
  const statusConf = STATUS_CONFIG[task.status] || STATUS_CONFIG.todo;
  const priorityConf = PRIORITY_CONFIG[task.priority] || PRIORITY_CONFIG.normal;
  const hasSubtasks = (task.subtask_count ?? 0) > 0;
  const isChild = !!task.parent_task_id;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onClick?.(task)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick?.(task);
        }
      }}
      // v3.2 (Phase 2): now uses the .card-brutal-heavy class. Same
      // 4px border + 12px shadow + 16px hover lift as the inline version
      // it replaces, but now reusable for other hero-tier cards.
      // v3.3: statusClass swaps the static black shadow for a tinted one
      // on hover — color reinforces status without a second visual signal.
      className={cn(
        'card-brutal-heavy w-full cursor-pointer text-left',
        statusConf.shadowClass,
      )}
    >
      <div className="p-4">
        {/* Top row: status + priority badges */}
        <div className="mb-2 flex flex-wrap items-center gap-2">
          <span className={cn('badge-brutal', statusConf.bgClass)}>
            {statusConf.label}
          </span>
          <span className={cn('badge-brutal', priorityConf.bgClass)}>
            {priorityConf.label}
          </span>
        </div>

        {/* Title */}
        <h3 className="font-heading text-base font-bold text-foreground leading-snug">
          {task.title}
        </h3>

        {/* Description preview */}
        {task.description && (
          <p className="mt-1 line-clamp-2 font-body text-sm text-muted-foreground">
            {task.description}
          </p>
        )}

        {/* Parent badge (child task) */}
        {isChild && (
          <div className="mt-1">
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
                className="text-xs text-muted-foreground hover:text-foreground underline decoration-dotted underline-offset-2 transition-colors"
              >
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}{t('subTask')} of {parentTaskNumber ? `#${parentTaskNumber}` : t('parentTask')}
              </span>
            ) : (
              <span className="text-xs text-muted-foreground">
                <ChevronRight className="inline h-3 w-3 -mt-px" />
                {' '}{t('subTask')}{parentTaskNumber ? ` of #${parentTaskNumber}` : ''}
              </span>
            )}
          </div>
        )}

        {/* Subtask progress bar (parent task) */}
        {hasSubtasks && (
          <div className="mt-2 pt-2 border-t-2 border-brutal-muted">
            <div className="flex items-center gap-2 text-xs">
              <span className="text-muted-foreground">{t('subTaskLabel')}</span>
              <span className="font-bold">{task.done_subtask_count ?? 0}/{task.subtask_count}</span>
              <div className="flex-1 h-1.5 border-2 border-black bg-brutal-cream">
                <div
                  className="h-full bg-brutal-success"
                  style={{ width: `${Math.min(((task.done_subtask_count ?? 0) / (task.subtask_count ?? 1)) * 100, 100)}%` }}
                />
              </div>
            </div>
          </div>
        )}

        {/* Bottom row: assignee + channel + due date */}
        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {onGenerateArtifact && (
            <button
              type="button"
              disabled={isArtifactGenerating}
              onClick={(e) => {
                e.stopPropagation();
                if (isArtifactGenerating) return;
                onGenerateArtifact(task);
              }}
              className="inline-flex items-center gap-1 border-2 border-black bg-white px-2 py-1 font-mono text-[10px] font-bold uppercase shadow-brutal-sm hover:bg-brutal-info hover:text-black disabled:pointer-events-none disabled:opacity-50"
              aria-label={`Generate artifact for ${task.title}`}
            >
              <FileText className="h-3 w-3" />
              Artifact
            </button>
          )}

          {task.assignee_name && (
            <span className="flex items-center gap-1">
              <User className="h-3 w-3" />
              {task.assignee_name}{task.claimer_deleted ? ' (Deleted)' : ''}
            </span>
          )}

          {showChannel && task.channel_name && (
            <span className="flex items-center gap-1 font-mono">
              <span className="font-bold text-sm text-black">#</span>
              {task.channel_name}
            </span>
          )}

          {task.due_date && (
            <span className="flex items-center gap-1">
              <Calendar className="h-3 w-3" />
              {formatDate(task.due_date)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
