// Tasks Kanban Board Page — DnD board, filters, thread panel (v2).
// Layout: NavBar + TasksLeftColumn (220px) + main (no AppFrame).
// ?channel= and ?dm= are mutually exclusive URL params (source-of-truth).

'use client';

import { useEffect, useState, useCallback, useMemo, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Plus, Filter, X } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useTasks, useDMTasks } from '@/lib/hooks/use-tasks';
import { useChannels } from '@/lib/hooks/use-channels';
import { useDM } from '@/lib/hooks/use-dm';
import { useToast } from '@/components/ui/toast';
import { NavBar } from '@/components/ui/navbar';
import { Spinner } from '@/components/ui/spinner';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { TasksLeftColumn } from '@/components/tasks/tasks-left-column';
import { TaskBoard } from '@/components/tasks/task-board';
import type { Task, TaskStatus, Message } from '@/lib/types';

// SOLO-63-F: Lazy-load ThreadPanel
const ThreadPanel = lazy(() =>
  import('@/components/dashboard/thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

export default function TasksPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { showToast } = useToast();

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
    updateTask,
    claimTask,
    unclaimTask,
    refetch: refetchTasks,
  } = useTasks();
  const {
    tasks: dmTasks,
    isLoading: dmTasksLoading,
    error: dmTasksError,
    updateTask: dmUpdateTask,
    claimTask: dmClaimTask,
    unclaimTask: dmUnclaimTask,
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
      const name = t.claimer_name || t.assignee_name || (id ? id.slice(0, 8) : '');
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

  // ---- Status change from board ----
  // In DM mode, the mutation must go through the DM-specific hook (the
  // channel-scoped endpoint would 404 because the task's channel_id is
  // actually the dm_id).
  const handleBoardStatusChange = useCallback(
    async (task: Task, newStatus: TaskStatus) => {
      try {
        const updated = filterDmId
          ? await dmUpdateTask(task.id, { status: newStatus })
          : await updateTask(task.channel_id, task.id, { status: newStatus });
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`任务状态已更新: ${newStatus}`, 'success');
      } catch {
        // Error handled by hook
      }
    },
    [filterDmId, updateTask, dmUpdateTask, showToast],
  );

  // ---- Claim / Unclaim ----
  const handleClaim = useCallback(
    async (task: Task) => {
      try {
        const updated = filterDmId
          ? await dmClaimTask(task.channel_id, task.id)
          : await claimTask(task.channel_id, task.id);
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`已认领任务 #${task.task_number ?? '?'}`, 'success');
      } catch {
        // 409: silent
      }
    },
    [filterDmId, claimTask, dmClaimTask, showToast],
  );

  const handleUnclaim = useCallback(
    async (task: Task) => {
      try {
        const updated = filterDmId
          ? await dmUnclaimTask(task.channel_id, task.id)
          : await unclaimTask(task.channel_id, task.id);
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`已释放任务 #${task.task_number ?? '?'}`, 'info');
      } catch {
        // silent
      }
    },
    [filterDmId, unclaimTask, dmUnclaimTask, showToast],
  );

  // ---- Auth loading ----
  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="lg" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  // Per-source empty-state message for "selected source has 0 tasks"
  const selectedSourceEmptyMessage = filterDmId ? '该 DM 没有任务' : '该频道没有任务';

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
            <div className="flex flex-shrink-0 items-center gap-3 border-b-2 border-black px-6 py-2.5">
              <Filter className="h-4 w-4 text-muted-foreground flex-shrink-0" />

              {/* Assignee dropdown */}
              <Select
                value={filterAssignee}
                onChange={(e) => handleFilterChange('assignee', e.target.value)}
                className="h-8 py-0 text-xs min-w-[120px]"
                aria-label="按认领人筛选"
              >
                <option value="">认领人: 全部</option>
                {assigneeOptions.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </Select>

              {/* Creator dropdown */}
              <Select
                value={filterCreator}
                onChange={(e) => handleFilterChange('creator', e.target.value)}
                className="h-8 py-0 text-xs min-w-[120px]"
                aria-label="按创建者筛选"
              >
                <option value="">创建者: 全部</option>
                {creatorOptions.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </Select>

              {/* Clear filters button */}
              {hasFilters && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleClearFilters}
                  className="flex items-center gap-1"
                >
                  <X className="h-3 w-3" />
                  清除筛选
                </Button>
              )}
            </div>

            {/* Board content — scrollable */}
            <div className="flex-1 overflow-y-auto overflow-x-hidden px-6 py-6">
              {!sourceIsLoading && !sourceError && tasks.length === 0 && sourceTasks.length > 0 ? (
                // Filtered empty — tasks exist but our filters excluded them all
                <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                  <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-cream">
                    <Filter className="h-6 w-6 text-muted-foreground" />
                  </div>
                  <p className="font-body text-sm text-muted-foreground">没有符合筛选条件的任务</p>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClearFilters}
                    className="mt-4"
                  >
                    清除筛选
                  </Button>
                </div>
              ) : !sourceIsLoading && !sourceError && tasks.length === 0 && hasFilters ? (
                // Selected channel/DM has no tasks
                <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                  <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-cream">
                    <Filter className="h-6 w-6 text-muted-foreground" />
                  </div>
                  <p className="font-body text-sm text-muted-foreground">{selectedSourceEmptyMessage}</p>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClearFilters}
                    className="mt-4"
                  >
                    清除筛选
                  </Button>
                </div>
              ) : !sourceIsLoading && !sourceError && tasks.length === 0 ? (
                // No tasks at all
                <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                  <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-cream">
                    <Plus className="h-6 w-6 text-muted-foreground" />
                  </div>
                  <p className="font-body text-sm text-muted-foreground">还没有任务</p>
                </div>
              ) : (
                <TaskBoard
                  tasks={tasks}
                  isLoading={sourceIsLoading}
                  error={sourceError}
                  onTaskClick={handleTaskClick}
                  onStatusChange={handleBoardStatusChange}
                  onRefetch={sourceRefetch}
                />
              )}
            </div>
          </div>

          {/* Thread panel — absolute overlay, doesn't shift main content */}
          <div
            className="absolute right-0 top-0 bottom-0 z-20 bg-brutal-cream overflow-hidden transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] border-l-2 border-black shadow-brutal-lg"
            style={{ width: threadMessage ? 400 : 0, opacity: threadMessage ? 1 : 0 }}
          >
            {threadMessage && (
              <Suspense
                fallback={
                  <div className="flex h-full items-center justify-center">
                    <Spinner size="sm" square={false} />
                  </div>
                }
              >
                <ThreadPanel
                  parentMessage={threadMessage}
                  onClose={handleThreadClose}
                  task={threadTask ?? undefined}
                  onClaimTask={handleClaim}
                  onUnclaimTask={handleUnclaim}
                />
              </Suspense>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
