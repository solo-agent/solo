// ============================================================================
// TaskDependenciesSection — shows blockers/blocked-by lists with add/remove UI
// Rendered inside ThreadPanel when viewing a task in the kanban board.
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Lock, AlertTriangle, Plus, X, Search } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { Task } from '@/lib/types';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';

// ---- Minimal task info returned by blockers/blocked endpoints ----

interface DepTask {
  id: string;
  task_number?: number;
  title: string;
  status: string;
}

// ---- Props ----

interface TaskDependenciesSectionProps {
  task: Task;
  /** All currently loaded tasks (for the search dialog) */
  allTasks: Task[];
  /** Called after any mutation so the board can refresh counts */
  onMutated: () => void;
}

// ---- Component ----

export function TaskDependenciesSection({ task, allTasks, onMutated }: TaskDependenciesSectionProps) {
  const [blockers, setBlockers] = useState<DepTask[]>([]);
  const [blocked, setBlocked] = useState<DepTask[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const fetchDeps = useCallback(async () => {
    setIsLoading(true);
    try {
      const [bRes, dRes] = await Promise.all([
        apiClient.get<DepTask[]>(`/api/v1/tasks/${task.id}/blockers`).catch(() => [] as DepTask[]),
        apiClient.get<DepTask[]>(`/api/v1/tasks/${task.id}/blocked`).catch(() => [] as DepTask[]),
      ]);
      setBlockers(Array.isArray(bRes) ? bRes : []);
      setBlocked(Array.isArray(dRes) ? dRes : []);
    } catch {
      // silent
    } finally {
      setIsLoading(false);
    }
  }, [task.id]);

  useEffect(() => {
    fetchDeps();
  }, [fetchDeps]);

  const handleRemove = useCallback(async (blockerId: string, blockedId: string) => {
    try {
      await apiClient.delete(
        `/api/v1/task-dependencies?blocker_task_id=${blockerId}&blocked_task_id=${blockedId}`,
      );
      fetchDeps();
      onMutated();
    } catch {
      // silent
    }
  }, [fetchDeps, onMutated]);

  const handleAdd = useCallback(async (otherTaskId: string, direction: 'blocks_this' | 'blocked_by_this') => {
    try {
      const body = direction === 'blocks_this'
        ? { blocker_task_id: otherTaskId, blocked_task_id: task.id }
        : { blocker_task_id: task.id, blocked_task_id: otherTaskId };
      await apiClient.post('/api/v1/task-dependencies', body);
      fetchDeps();
      onMutated();
      setIsDialogOpen(false);
    } catch {
      // silent
    }
  }, [task.id, fetchDeps, onMutated]);

  const hasDeps = blockers.length > 0 || blocked.length > 0;

  return (
    <>
      <div className="border-b-2 border-black bg-brutal-cream px-4 py-2.5">
        {/* Header row */}
        <div className="flex items-center justify-between mb-1.5">
          <span className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground">
            {t('taskDependenciesTitle')}
          </span>
          <button
            type="button"
            onClick={() => setIsDialogOpen(true)}
            className={cn(
              'inline-flex items-center gap-1 px-1.5 py-0.5',
              'font-heading text-[10px] font-bold uppercase tracking-wider',
              'border-2 border-black bg-white hover:bg-brutal-primary-light',
              'active:translate-x-0.5 active:translate-y-0.5',
              'transition-all cursor-pointer',
            )}
            aria-label={t('taskAddDependency')}
          >
            <Plus className="h-3 w-3" />
            {t('taskAddDependency')}
          </button>
        </div>

        {isLoading ? (
          <p className="font-mono text-[10px] text-muted-foreground">{t('loading')}...</p>
        ) : !hasDeps ? (
          <p className="font-mono text-[10px] text-muted-foreground">{t('taskNoDependencies')}</p>
        ) : (
          <div className="space-y-1 max-h-28 overflow-y-auto">
            {/* Blocked by (this task is waiting on...) */}
            {blockers.length > 0 && (
              <div>
                <span className="inline-flex items-center gap-1 font-heading text-[10px] font-bold uppercase tracking-wider text-brutal-danger mb-0.5">
                  <Lock className="h-2.5 w-2.5" />
                  {t('taskBlockedByLabel')}
                </span>
                <ul className="space-y-0.5">
                  {blockers.map((b) => (
                    <li key={b.id} className="flex items-center gap-1.5 text-[11px]">
                      <span className="font-mono font-bold text-muted-foreground">
                        #{b.task_number ?? '?'}
                      </span>
                      <span className="font-body text-foreground truncate flex-1">
                        {b.title}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleRemove(b.id, task.id)}
                        className="flex-shrink-0 flex h-4 w-4 items-center justify-center border border-black hover:bg-brutal-danger-light active:translate-x-px active:translate-y-px transition-all cursor-pointer"
                        aria-label={t('taskRemoveDependency', { n: b.task_number ?? '?' })}
                      >
                        <X className="h-2.5 w-2.5" />
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* Blocking (this task blocks...) */}
            {blocked.length > 0 && (
              <div>
                <span className="inline-flex items-center gap-1 font-heading text-[10px] font-bold uppercase tracking-wider text-brutal-warning mb-0.5">
                  <AlertTriangle className="h-2.5 w-2.5" />
                  {t('taskBlockingLabel')}
                </span>
                <ul className="space-y-0.5">
                  {blocked.map((b) => (
                    <li key={b.id} className="flex items-center gap-1.5 text-[11px]">
                      <span className="font-mono font-bold text-muted-foreground">
                        #{b.task_number ?? '?'}
                      </span>
                      <span className="font-body text-foreground truncate flex-1">
                        {b.title}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleRemove(task.id, b.id)}
                        className="flex-shrink-0 flex h-4 w-4 items-center justify-center border border-black hover:bg-brutal-danger-light active:translate-x-px active:translate-y-px transition-all cursor-pointer"
                        aria-label={t('taskRemoveDependency', { n: b.task_number ?? '?' })}
                      >
                        <X className="h-2.5 w-2.5" />
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Add dependency dialog */}
      <AddDependencyDialog
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        task={task}
        allTasks={allTasks}
        existingBlockerIds={new Set(blockers.map((b) => b.id))}
        existingBlockedIds={new Set(blocked.map((b) => b.id))}
        onAdd={handleAdd}
      />
    </>
  );
}

// ---- Add Dependency Dialog ----

interface AddDependencyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: Task;
  allTasks: Task[];
  existingBlockerIds: Set<string>;
  existingBlockedIds: Set<string>;
  onAdd: (otherTaskId: string, direction: 'blocks_this' | 'blocked_by_this') => void;
}

function AddDependencyDialog({
  open,
  onOpenChange,
  task,
  allTasks,
  existingBlockerIds,
  existingBlockedIds,
  onAdd,
}: AddDependencyDialogProps) {
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // Reset query when dialog opens
  useEffect(() => {
    if (open) {
      setQuery('');
      // Focus input after render
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  // Filter available tasks: not self, not already linked
  const availableTasks = useMemo(() => {
    const excludeIds = new Set([task.id, ...existingBlockerIds, ...existingBlockedIds]);
    const q = query.trim().toLowerCase();
    if (!q) {
      return allTasks
        .filter((t) => !excludeIds.has(t.id))
        .slice(0, 20);
    }
    // Search by task_number or title
    return allTasks.filter((t) => {
      if (excludeIds.has(t.id)) return false;
      const numStr = t.task_number != null ? String(t.task_number) : '';
      return numStr.includes(q) || t.title.toLowerCase().includes(q);
    }).slice(0, 20);
  }, [allTasks, task.id, existingBlockerIds, existingBlockedIds, query]);

  const currentNum = task.task_number ?? '?';

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width="sm">
      <DialogHeader>
        <DialogTitle>
          {t('taskAddDependencyTitle', { n: currentNum })}
        </DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>

      {/* Search input */}
      <div className="mb-3">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('taskSearchDependencyPlaceholder')}
            className={cn(
              'w-full h-10 pl-8 pr-3',
              'border-2 border-black bg-white',
              'font-mono text-sm',
              'placeholder:text-muted-foreground/50',
              'focus:outline-none focus:bg-brutal-secondary-light',
              'focus:shadow-[2px_2px_0px_0px_#000]',
            )}
            aria-label={t('taskSearchDependencyPlaceholder')}
          />
        </div>
      </div>

      {/* Task list */}
      {availableTasks.length === 0 ? (
        <p className="font-mono text-xs text-muted-foreground text-center py-4">
          {query ? t('taskNoMatchingTasks') : t('taskAllTasksLinked')}
        </p>
      ) : (
        <ul className="max-h-64 overflow-y-auto space-y-0.5">
          {availableTasks.map((at) => (
            <li
              key={at.id}
              className="flex items-center gap-2 border-2 border-black bg-white p-2"
            >
              <span className="font-mono text-xs font-bold text-muted-foreground flex-shrink-0">
                #{at.task_number ?? '?'}
              </span>
              <span className="font-body text-xs text-foreground truncate flex-1">
                {at.title}
              </span>
              {/* Two action buttons: "Blocks this" and "Blocked by this" */}
              <div className="flex items-center gap-1 flex-shrink-0">
                <button
                  type="button"
                  onClick={() => onAdd(at.id, 'blocks_this')}
                  className={cn(
                    'px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider',
                    'border-2 border-black bg-brutal-danger-light text-brutal-danger',
                    'hover:bg-brutal-danger hover:text-white',
                    'active:translate-x-0.5 active:translate-y-0.5',
                    'transition-all cursor-pointer',
                  )}
                >
                  {t('taskDependencyBlocksThis')}
                </button>
                <button
                  type="button"
                  onClick={() => onAdd(at.id, 'blocked_by_this')}
                  className={cn(
                    'px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider',
                    'border-2 border-black bg-brutal-warning-light text-black',
                    'hover:bg-brutal-warning',
                    'active:translate-x-0.5 active:translate-y-0.5',
                    'transition-all cursor-pointer',
                  )}
                >
                  {t('taskDependencyBlockedByThis')}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Dialog>
  );
}
