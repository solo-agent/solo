// ============================================================================
// Tasks Kanban Board Page — 5-column board with filters + DnD (v1.5)
// - Route: /tasks
// - Filter bar: Assignee / Creator / Channel (global only)
// - Filter state synced to URL query params
// - Board: DnD between columns with status transition validation
// - Neubrutalist styling
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useMemo, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Plus, Loader2, Filter, X } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { useChannels } from '@/lib/hooks/use-channels';
import { useToast } from '@/components/ui/toast';
import { AppFrame } from '@/components/layout/app-frame';
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
  const { channels } = useChannels();
  const { showToast } = useToast();

  // ---- Filter state from URL ----
  const urlChannelId = searchParams.get('channel') || '';
  const urlAssignee = searchParams.get('assignee') || '';
  const urlCreator = searchParams.get('creator') || '';

  const [filterChannel, setFilterChannel] = useState(urlChannelId);
  const [filterAssignee, setFilterAssignee] = useState(urlAssignee);
  const [filterCreator, setFilterCreator] = useState(urlCreator);

  // Sync URL -> state (for browser back/forward)
  useEffect(() => {
    setFilterChannel(urlChannelId);
    setFilterAssignee(urlAssignee);
    setFilterCreator(urlCreator);
  }, [urlChannelId, urlAssignee, urlCreator]);

  const hasFilters = !!(filterChannel || filterAssignee || filterCreator);
  const isGlobalView = !urlChannelId;

  // ---- Task data ----
  const {
    tasks: allTasks,
    isLoading: tasksLoading,
    error: tasksError,
    updateTask,
    claimTask,
    unclaimTask,
    refetch: refetchTasks,
  } = useTasks();

  // ---- Client-side filtering ----
  const tasks = useMemo(() => {
    return allTasks.filter((t) => {
      if (filterChannel && t.channel_id !== filterChannel) return false;
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
  }, [allTasks, filterChannel, filterAssignee, filterCreator]);

  // ---- Unique filter options derived from allTasks ----
  const assigneeOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const t of allTasks) {
      const id = t.claimer_id || t.assignee_id;
      const name = t.claimer_name || t.assignee_name || (id ? id.slice(0, 8) : '');
      if (id && !seen.has(id)) seen.set(id, { id, name });
    }
    return Array.from(seen.values());
  }, [allTasks]);

  const creatorOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const t of allTasks) {
      const id = t.creator_id;
      const name = t.creator_name || (id ? id.slice(0, 8) : '');
      if (id && !seen.has(id)) seen.set(id, { id, name });
    }
    return Array.from(seen.values());
  }, [allTasks]);

  // ---- Update URL when filters change ----
  const updateUrlFilter = useCallback(
    (channel: string, assignee: string, creator: string) => {
      const params = new URLSearchParams();
      if (channel) params.set('channel', channel);
      if (assignee) params.set('assignee', assignee);
      if (creator) params.set('creator', creator);
      const qs = params.toString();
      router.push(qs ? `/tasks?${qs}` : '/tasks');
    },
    [router],
  );

  const handleFilterChange = useCallback(
    (field: 'channel' | 'assignee' | 'creator', value: string) => {
      const newChannel = field === 'channel' ? value : filterChannel;
      const newAssignee = field === 'assignee' ? value : filterAssignee;
      const newCreator = field === 'creator' ? value : filterCreator;
      setFilterChannel(newChannel);
      setFilterAssignee(newAssignee);
      setFilterCreator(newCreator);
      updateUrlFilter(newChannel, newAssignee, newCreator);
    },
    [filterChannel, filterAssignee, filterCreator, updateUrlFilter],
  );

  const handleClearFilters = useCallback(() => {
    setFilterChannel('');
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
    const updated = allTasks.find((t) => t.id === threadTask.id);
    if (updated && (updated.status !== threadTask.status || updated.claimer_id !== threadTask.claimer_id)) {
      setThreadTask(updated);
    }
  }, [allTasks, threadTask]);

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
  }, []);

  // ---- Status change from board ----
  const handleBoardStatusChange = useCallback(
    async (task: Task, newStatus: TaskStatus) => {
      try {
        const updated = await updateTask(task.channel_id, task.id, { status: newStatus });
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`任务状态已更新: ${newStatus}`, 'success');
      } catch {
        // Error handled by hook
      }
    },
    [updateTask, showToast],
  );

  // ---- Claim / Unclaim ----
  const handleClaim = useCallback(
    async (task: Task) => {
      try {
        const updated = await claimTask(task.channel_id, task.id);
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`已认领任务 #${task.task_number ?? '?'}`, 'success');
      } catch {
        // 409: silent
      }
    },
    [claimTask, showToast],
  );

  const handleUnclaim = useCallback(
    async (task: Task) => {
      try {
        const updated = await unclaimTask(task.channel_id, task.id);
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`已释放任务 #${task.task_number ?? '?'}`, 'info');
      } catch {
        // silent
      }
    },
    [unclaimTask, showToast],
  );

  // ---- Auth loading ----
  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  // Determine empty state message
  const emptyMessage = hasFilters
    ? '没有符合筛选条件的任务'
    : '该频道没有任务';

  return (
    <AppFrame>
      <div className="relative flex flex-1 overflow-hidden">
        {/* Main content area — unaffected by ThreadPanel */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Page header */}
          <div className="flex h-14 flex-shrink-0 items-center border-b-2 border-black px-6">
            <h1 className="font-heading text-lg font-bold text-foreground">
              任务看板
            </h1>
          </div>

          {/* Filter bar */}
          <div className="flex flex-shrink-0 items-center gap-3 border-b-2 border-black px-6 py-2.5">
            <Filter className="h-4 w-4 text-muted-foreground flex-shrink-0" />

            {/* Assignee dropdown */}
            <select
              value={filterAssignee}
              onChange={(e) => handleFilterChange('assignee', e.target.value)}
              className="input-brutal h-8 py-0 text-xs min-w-[120px]"
              aria-label="按认领人筛选"
            >
              <option value="">认领人: 全部</option>
              {assigneeOptions.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>

            {/* Creator dropdown */}
            <select
              value={filterCreator}
              onChange={(e) => handleFilterChange('creator', e.target.value)}
              className="input-brutal h-8 py-0 text-xs min-w-[120px]"
              aria-label="按创建者筛选"
            >
              <option value="">创建者: 全部</option>
              {creatorOptions.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>

            {/* Channel dropdown — only visible in global view */}
            {isGlobalView && (
              <select
                value={filterChannel}
                onChange={(e) => handleFilterChange('channel', e.target.value)}
                className="input-brutal h-8 py-0 text-xs min-w-[120px]"
                aria-label="按频道筛选"
              >
                <option value="">频道: 全部</option>
                {channels.map((ch) => (
                  <option key={ch.id} value={ch.id}>
                    #{ch.name}
                  </option>
                ))}
              </select>
            )}

            {/* Clear filters button */}
            {hasFilters && (
              <button
                type="button"
                onClick={handleClearFilters}
                className="btn-brutal btn-brutal-sm flex items-center gap-1 text-xs"
              >
                <X className="h-3 w-3" />
                清除筛选
              </button>
            )}
          </div>

          {/* Board content — scrollable */}
          <div className="flex-1 overflow-y-auto overflow-x-hidden px-6 py-6">
            {!tasksLoading && !tasksError && tasks.length === 0 && allTasks.length > 0 ? (
              // Filtered empty state
              <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-cream">
                  <Filter className="h-6 w-6 text-muted-foreground" />
                </div>
                <p className="font-body text-sm text-muted-foreground">{emptyMessage}</p>
                <button
                  type="button"
                  onClick={handleClearFilters}
                  className="btn-brutal btn-brutal-sm mt-4"
                >
                  清除筛选
                </button>
              </div>
            ) : !tasksLoading && !tasksError && tasks.length === 0 && allTasks.length === 0 ? (
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
                isLoading={tasksLoading}
                error={tasksError}
                onTaskClick={handleTaskClick}
                onStatusChange={handleBoardStatusChange}
                onRefetch={refetchTasks}
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
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
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

    </AppFrame>
  );
}
