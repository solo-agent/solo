// ============================================================================
// Tasks Kanban Board Page — 5-column board: TODO | IN PROGRESS | IN REVIEW | DONE | CLOSED
// - Route: /tasks
// - Board: horizontal scroll on desktop, stack on mobile
// - Create task: modal with title-only field
// - Task click: opens ThreadPanel as right-side panel (remains in kanban view)
// - Neubrutalist styling: card-brutal, btn-brutal, shadow-brutal
// ============================================================================

'use client';

import { useEffect, useState, useCallback, lazy, Suspense } from 'react';
import { useRouter } from 'next/navigation';
import { Plus, Loader2 } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { useChannels } from '@/lib/hooks/use-channels';
import { useToast } from '@/components/ui/toast';
import { AppFrame } from '@/components/layout/app-frame';
import { TaskBoard } from '@/components/tasks/task-board';
import { CreateTaskModal } from '@/components/tasks/create-task-modal';
import type { Task, TaskStatus, CreateTaskInput, Message } from '@/lib/types';

// SOLO-63-F: Lazy-load ThreadPanel (only rendered when a thread is open)
const ThreadPanel = lazy(() =>
  import('@/components/dashboard/thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

export default function TasksPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { channels } = useChannels();

  // Task data
  const {
    tasks,
    isLoading: tasksLoading,
    error: tasksError,
    createTask,
    updateTask,
    claimTask,
    unclaimTask,
    refetch: refetchTasks,
  } = useTasks();

  const { showToast } = useToast();

  // Modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);

  // Thread panel state
  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // ---- Task click: open ThreadPanel inline (not navigate away) ----
  // Per v1.2 task-system-analysis: click task -> open ThreadPanel
  const handleTaskClick = useCallback(
    (task: Task) => {
      // Construct a minimal Message for ThreadPanel from task data
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
    },
    [],
  );

  // Sync threadTask when the tasks list changes (e.g. after WS task.updated)
  useEffect(() => {
    if (!threadTask) return;
    const updated = tasks.find((t) => t.id === threadTask.id);
    if (updated && (updated.status !== threadTask.status || updated.claimer_id !== threadTask.claimer_id)) {
      setThreadTask(updated);
    }
  }, [tasks, threadTask]);

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
      } catch {
        // Error handled by hook
      }
    },
    [updateTask],
  );

  // ---- Claim / Unclaim from board ----
  const handleClaim = useCallback(
    async (task: Task) => {
      try {
        const updated = await claimTask(task.channel_id, task.id);
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
        showToast(`已认领任务 #${task.task_number ?? '?'}`, 'success');
      } catch {
        // 409: silent — per spec
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
        // handled silently
      }
    },
    [unclaimTask, showToast],
  );

  // ---- Create task ----
  const handleCreateTask = useCallback(
    async (input: CreateTaskInput) => {
      setIsCreating(true);
      try {
        // If no channel_id provided, use first available channel
        if (!input.channel_id && channels.length > 0) {
          input.channel_id = channels[0].id;
        }
        await createTask(input);
      } finally {
        setIsCreating(false);
      }
    },
    [createTask, channels],
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

  return (
    <AppFrame>
      <div className="flex flex-1 overflow-hidden">
        {/* Main content area */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Page header */}
          <div className="flex h-14 flex-shrink-0 items-center justify-between border-b-2 border-black px-6">
            <div>
              <h1 className="font-heading text-lg font-bold text-foreground">
                任务看板
              </h1>
            </div>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setIsCreateModalOpen(true)}
                className="btn-brutal btn-brutal-sm btn-brutal-pink flex items-center gap-1.5"
              >
                <Plus className="h-4 w-4" />
                创建任务
              </button>
            </div>
          </div>

          {/* Board content — scrollable */}
          <div className="flex-1 overflow-y-auto overflow-x-hidden px-6 py-6">
            <TaskBoard
              tasks={tasks}
              isLoading={tasksLoading}
              error={tasksError}
              onTaskClick={handleTaskClick}
              onStatusChange={handleBoardStatusChange}
              onRefetch={refetchTasks}
              onClaim={handleClaim}
              onUnclaim={handleUnclaim}
            />
          </div>
        </div>

        {/* Thread panel (lazy-loaded, SOLO-63-F) */}
        {threadMessage && (
          <div className="w-[400px] flex-shrink-0 bg-brutal-cream overflow-hidden border-l-2 border-black">
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
          </div>
        )}
      </div>

      {/* Create task modal */}
      <CreateTaskModal
        open={isCreateModalOpen}
        onOpenChange={setIsCreateModalOpen}
        channelId={channels.length > 0 ? channels[0].id : undefined}
        onSubmit={handleCreateTask}
        isSubmitting={isCreating}
      />
    </AppFrame>
  );
}
