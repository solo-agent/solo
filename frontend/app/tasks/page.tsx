// Tasks Kanban Board Page — DnD board, filters, thread panel (v2).
// Layout: NavBar + TasksLeftColumn (220px) + main (no AppFrame).
// ?channel= and ?dm= are mutually exclusive URL params (source-of-truth).

'use client';

import { useEffect, useState, useCallback, useMemo, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Plus, Filter, X } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { useTasks, useDMTasks } from '@/lib/hooks/use-tasks';
import { useChannels } from '@/lib/hooks/use-channels';
import { useDM } from '@/lib/hooks/use-dm';
import { NavBar } from '@/components/ui/navbar';
import { Spinner } from '@/components/ui/spinner';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { EmptyState } from '@/components/ui/empty-state';
import { TasksLeftColumn } from '@/components/tasks/tasks-left-column';
import { TaskBoard } from '@/components/tasks/task-board';
import type { Task, Message } from '@/lib/types';

// SOLO-63-F: Lazy-load ThreadPanel
const ThreadPanel = lazy(() =>
  import('@/components/dashboard/thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

export default function TasksPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-brutal-cream">
          <Spinner size="md" />
        </div>
      }
    >
      <TasksPageContent />
    </Suspense>
  );
}

function TasksPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { isAuthenticated, isLoading: authLoading } = useAuth();

  // ---- Left column data ----
  const { channels, isLoading: channelsLoading, error: channelsError, refetch: refetchChannels } = useChannels();
  const { dmChannels, isLoadingDMs, dmError, refetchDMs } = useDM();

  // ---- URL filter state ----
  const filterChannelId = searchParams.get('channel');
  const filterDmId = searchParams.get('dm');
  const urlAssignee = searchParams.get('assignee') || '';
  const urlCreator = searchParams.get('creator') || '';

  const [filterAssignee, setFilterAssignee] = useState(urlAssignee);
  const [filterCreator, setFilterCreator] = useState(urlCreator);

  // Sync URL -> state (for browser back/forward)
  useEffect(() => {
    setFilterAssignee(urlAssignee);
    setFilterCreator(urlCreator);
  }, [urlAssignee, urlCreator]);

  const hasFilters = !!(filterChannelId || filterDmId || filterAssignee || filterCreator);

  // ---- Task data sources (hooks unconditional; pick source after) ----
  const {
    tasks: allTasks,
    isLoading: tasksLoading,
    error: tasksError,
    refetch: refetchTasks,
  } = useTasks();
  const {
    tasks: dmTasks,
    isLoading: dmTasksLoading,
    error: dmTasksError,
    refetch: refetchDMTasks,
  } = useDMTasks(filterDmId);

  const sourceTasks = filterDmId ? dmTasks : allTasks;
  const sourceIsLoading = filterDmId ? dmTasksLoading : tasksLoading;
  const sourceError = filterDmId ? dmTasksError : tasksError;
  const sourceRefetch = filterDmId ? refetchDMTasks : refetchTasks;

  // ---- Client-side filtering (assignee/creator always; channel only on non-DM path) ----
  const tasks = useMemo(() => {
    return sourceTasks.filter((t) => {
      if (!filterDmId && filterChannelId && t.channel_id !== filterChannelId) return false;
      if (filterAssignee) {
        const claimerVal = t.claimer_id || t.assignee_id || '';
        const claimerName = (t.claimer_name || t.assignee_name || '').toLowerCase();
        const filterVal = filterAssignee.toLowerCase();
        if (claimerVal !== filterAssignee && !claimerName.includes(filterVal)) return false;
      }
      if (filterCreator) {
        const creatorName = (t.creator_name || t.creator_id || '').toLowerCase();
        const filterVal = filterCreator.toLowerCase();
        if (t.creator_id !== filterCreator && !creatorName.includes(filterVal)) return false;
      }
      return true;
    });
  }, [sourceTasks, filterDmId, filterChannelId, filterAssignee, filterCreator]);

  // ---- Unique filter options derived from the active source ----
  const assigneeOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const t of sourceTasks) {
      const id = t.claimer_id || t.assignee_id;
      const baseName = t.claimer_name || t.assignee_name || (id ? id.slice(0, 8) : '');
      const name = baseName + (t.claimer_deleted ? ' (Deleted)' : '');
      if (id && !seen.has(id)) seen.set(id, { id, name });
    }
    return Array.from(seen.values());
  }, [sourceTasks]);

  const creatorOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const t of sourceTasks) {
      const id = t.creator_id;
      const name = t.creator_name || (id ? id.slice(0, 8) : '');
      if (id && !seen.has(id)) seen.set(id, { id, name });
    }
    return Array.from(seen.values());
  }, [sourceTasks]);

  // ---- Left column click handlers (re-click clears filter) ----
  const handleChannelClick = useCallback(
    (channelId: string) => {
      router.push(filterChannelId === channelId ? '/tasks' : `/tasks?channel=${channelId}`);
    },
    [router, filterChannelId],
  );

  const handleDmClick = useCallback(
    (dmId: string) => {
      router.push(filterDmId === dmId ? '/tasks' : `/tasks?dm=${dmId}`);
    },
    [router, filterDmId],
  );

  // ---- Update URL when filter bar (assignee/creator) changes; preserves ?channel=?dm= ----
  const updateUrlFilter = useCallback(
    (assignee: string, creator: string) => {
      const params = new URLSearchParams();
      if (filterChannelId) params.set('channel', filterChannelId);
      if (filterDmId) params.set('dm', filterDmId);
      if (assignee) params.set('assignee', assignee);
      if (creator) params.set('creator', creator);
      const qs = params.toString();
      router.push(qs ? `/tasks?${qs}` : '/tasks');
    },
    [router, filterChannelId, filterDmId],
  );

  const handleFilterChange = useCallback(
    (field: 'assignee' | 'creator', value: string) => {
      const newAssignee = field === 'assignee' ? value : filterAssignee;
      const newCreator = field === 'creator' ? value : filterCreator;
      setFilterAssignee(newAssignee);
      setFilterCreator(newCreator);
      updateUrlFilter(newAssignee, newCreator);
    },
    [filterAssignee, filterCreator, updateUrlFilter],
  );

  const handleClearFilters = useCallback(() => {
    setFilterAssignee('');
    setFilterCreator('');
    router.push('/tasks');
  }, [router]);

  // Thread panel state
  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // Task click: open ThreadPanel inline
  const handleTaskClick = useCallback((task: Task) => {
    setThreadMessage({
      id: task.message_id || task.id,
      channel_id: task.channel_id,
      user_id: task.creator_id,
      display_name: task.creator_name || task.creator_id.slice(0, 8),
      content: task.description || task.title,
      created_at: task.created_at,
      status: 'sent',
      sender_type: 'user',
      task_number: task.task_number,
      task_status: task.status,
      task_claimer_name: task.claimer_name || task.assignee_name,
    });
    setThreadTask(task);
  }, []);

  useEffect(() => {
    if (!threadTask) return;
    const updated = sourceTasks.find((t) => t.id === threadTask.id);
    if (updated && (updated.status !== threadTask.status || updated.claimer_id !== threadTask.claimer_id)) {
      setThreadTask(updated);
    }
  }, [sourceTasks, threadTask]);

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
  }, []);

  const handleBoardActionComplete = useCallback(
    (updated: Task) => {
      setThreadTask((prev) => (prev?.id === updated.id ? updated : prev));
      sourceRefetch();
    },
    [sourceRefetch],
  );

  // Resolve selected channel/DM name for the filter bar header.
  // Must run before any early return so hook order is stable across renders.
  const selectedSourceName = useMemo(() => {
    if (filterDmId) {
      const dm = dmChannels.find((d) => d.id === filterDmId);
      if (dm?.other_user) return dm.other_user.display_name;
      if (dm?.other_agent) return dm.other_agent.name;
      return null;
    }
    if (filterChannelId) {
      const ch = channels.find((c) => c.id === filterChannelId);
      return ch?.name ?? null;
    }
    return null;
  }, [filterDmId, filterChannelId, dmChannels, channels]);

  // ---- Auth loading ----
  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="font-mono text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    );
  }

  // Per-source empty-state message for "selected source has 0 tasks"
  const selectedSourceEmptyMessage = filterDmId ? t('noTasksInDM') : t('noTasksInChannel');

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <NavBar />
      <div className="w-[220px] flex-shrink-0">
        <TasksLeftColumn
          channels={channels}
          channelsLoading={channelsLoading}
          channelsError={channelsError}
          onRetryChannels={refetchChannels}
          selectedChannelId={filterChannelId}
          onChannelClick={handleChannelClick}
          dms={dmChannels}
          dmsLoading={isLoadingDMs}
          dmsError={dmError}
          onRetryDMs={refetchDMs}
          selectedDmId={filterDmId}
          onDmClick={handleDmClick}
        />
      </div>

      <main className="flex flex-1 flex-col overflow-hidden">
        <div className="relative flex flex-1 overflow-hidden">
          {/* Main content area — unaffected by ThreadPanel */}
          <div className="flex flex-1 flex-col overflow-hidden">
            {/* Filter bar */}
            <div className="flex flex-shrink-0 items-center gap-3 border-b-2 border-black px-4 h-14">
              {selectedSourceName ? (
                <>
                  <span className="flex items-center gap-1.5 truncate">
                    <span className="font-mono text-base font-bold text-black flex-shrink-0">#</span>
                    <span className="font-heading text-sm font-bold text-foreground truncate">
                      {selectedSourceName}
                    </span>
                  </span>
                  <div className="mx-1 h-4 w-px bg-border" />
                </>
              ) : (
                <Filter className="h-4 w-4 text-muted-foreground flex-shrink-0" />
              )}

              {/* Assignee dropdown */}
              <Select
                value={filterAssignee}
                onChange={(v) => handleFilterChange('assignee', v)}
                options={[
                  { value: '', label: t('allAssignees') },
                  ...assigneeOptions.map((a) => ({ value: a.id, label: a.name })),
                ]}
                size="sm"
                className="min-w-[120px]"
                aria-label={t('filterByClaimer')}
              />

              {/* Creator dropdown */}
              <Select
                value={filterCreator}
                onChange={(v) => handleFilterChange('creator', v)}
                options={[
                  { value: '', label: t('allCreators') },
                  ...creatorOptions.map((c) => ({ value: c.id, label: c.name })),
                ]}
                size="sm"
                className="min-w-[120px]"
                aria-label={t('filterByCreator')}
              />

              {/* Clear filters button */}
              {hasFilters && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleClearFilters}
                  className="flex items-center gap-1"
                >
                  <X className="h-3 w-3" />
                  {t('clearFilter')}
                </Button>
              )}
            </div>

            {/* Board content — scrollable */}
            <div className="flex-1 overflow-y-auto overflow-x-hidden px-6 py-6">
              {!sourceIsLoading && !sourceError && tasks.length === 0 && sourceTasks.length > 0 ? (
                // Filtered empty — tasks exist but our filters excluded them all
                <EmptyState
                  variant="dashed"
                  icon={<Filter className="h-6 w-6 text-muted-foreground" />}
                  title={t('noTasksMatchingFilter')}
                  actionLabel={t('clearFilter')}
                  onAction={handleClearFilters}
                />
              ) : !sourceIsLoading && !sourceError && tasks.length === 0 && hasFilters ? (
                // Selected channel/DM has no tasks
                <EmptyState
                  variant="dashed"
                  icon={<Filter className="h-6 w-6 text-muted-foreground" />}
                  title={selectedSourceEmptyMessage}
                  actionLabel={t('clearFilter')}
                  onAction={handleClearFilters}
                />
              ) : !sourceIsLoading && !sourceError && tasks.length === 0 ? (
                // No tasks at all
                <EmptyState
                  variant="dashed"
                  icon={<Plus className="h-6 w-6 text-muted-foreground" />}
                  title={t('noTasks')}
                />
              ) : (
                <TaskBoard
                  tasks={tasks}
                  isLoading={sourceIsLoading}
                  error={sourceError}
                  onTaskClick={handleTaskClick}
                  onRefetch={sourceRefetch}
                  onActionComplete={handleBoardActionComplete}
                />
              )}
            </div>
          </div>

          {/* Thread panel — absolute overlay, doesn't shift main content */}
          <div
            className="absolute right-0 top-0 bottom-0 z-20 bg-brutal-cream overflow-hidden transition-[width,opacity] duration-100 ease-linear border-l-2 border-black shadow-brutal-lg"
            style={{ width: threadMessage ? 400 : 0, opacity: threadMessage ? 1 : 0 }}
          >
            {threadMessage && (
              <Suspense
                fallback={
                  <div className="flex h-full items-center justify-center">
                    <Spinner size="sm" />
                  </div>
                }
              >
                <ThreadPanel
                  parentMessage={threadMessage}
                  onClose={handleThreadClose}
                  task={threadTask ?? undefined}
                />
              </Suspense>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
