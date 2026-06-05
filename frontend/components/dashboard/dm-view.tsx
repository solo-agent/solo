// ============================================================================
// DM View — DM conversation view with neubrutalist styling
// - Header: card-brutal style + online status dot
// - MessageList (already brutalist)
// - System message: "这是你与 {name} 的私信"
// - Messages/Tasks dual tab (v1.2 Phase 2+3)
// - Input: MessageInput (already brutalist)
// - States: loading, error with retry, normal
// ============================================================================

'use client';

import { useState, useMemo, useCallback, useEffect, lazy, Suspense } from 'react';
import { Bot, User, AlertCircle, RefreshCw, MessageSquare, Circle, ClipboardList } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useStreamingMessages } from '@/lib/hooks/use-streaming-messages';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { TaskBoard } from '@/components/tasks/task-board';
import { Skeleton } from '@/components/ui/skeleton';
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);
import type { DMChannel, ChannelMember, Message, Task, TaskStatus } from '@/lib/types';

interface DMViewProps {
  dm: DMChannel;
  messages: Message[];
  isLoading: boolean;
  error: string | null;
  sendMessage: (content: string, mentionedAgentIds?: string[], asTask?: boolean, attachmentIds?: string[]) => Promise<{ id: string; task_number?: number } | null>;
  retryMessage: (messageId: string, content: string) => Promise<void>;
  cancelMessage?: (messageId: string) => void;
  editMessage?: (messageId: string, content: string) => Promise<void>;
  deleteMessage?: (messageId: string) => Promise<void>;
  onAsTask?: (message: Message) => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMoreError: string | null;
  loadMore: () => Promise<void>;
  refetch: () => void;
  // ---- DM Tasks (v1.2 Phase 2+3) ----
  tasks?: Task[];
  tasksLoading?: boolean;
  tasksError?: string | null;
  refetchTasks?: () => void;
  onTaskStatusChange?: (task: Task, newStatus: TaskStatus) => Promise<Task | void>;
  onClaimTask?: (task: Task) => void;
  onUnclaimTask?: (task: Task) => void;
  onConvertToTask?: (channelId: string, messageId: string, title?: string) => Promise<Task>;
  onTaskCreated?: () => void;
  /** v1.5: Called when thread opens/closes so the parent can sync to URL */
  onThreadChange?: (threadId: string | null) => void;
  /** v1.5: Initial thread message ID from URL — opens ThreadPanel on mount or on change */
  initialThreadMessageId?: string;
}

// ---- Helpers ----

function getDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return '未知用户';
}

function isAgentDM(dm: DMChannel): boolean {
  return !!dm.other_agent;
}

function isAgentDeleted(dm: DMChannel): boolean {
  return isAgentDM(dm) && dm.other_agent?.is_active === false;
}

export function DMView({
  dm,
  messages,
  isLoading,
  error,
  sendMessage,
  retryMessage,
  cancelMessage,
  editMessage,
  deleteMessage,
  onAsTask,
  hasMore,
  isLoadingMore,
  loadMoreError,
  loadMore,
  refetch,
  tasks,
  tasksLoading,
  tasksError,
  refetchTasks,
  onTaskStatusChange,
  onClaimTask,
  onUnclaimTask,
  onConvertToTask,
  onTaskCreated,
  onThreadChange,
  initialThreadMessageId,
}: DMViewProps) {
  const name = getDisplayName(dm);
  const isAgent = isAgentDM(dm);
  const deleted = isAgentDeleted(dm);
  const [viewTab, setViewTab] = useState<'messages' | 'tasks'>('messages');
  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  // Refetch tasks when switching to tasks tab
  useEffect(() => {
    if (viewTab === 'tasks') refetchTasks?.();
  }, [viewTab]); // eslint-disable-line react-hooks/exhaustive-deps

  // v1.5: Handle initialThreadMessageId — open thread panel when URL param changes
  useEffect(() => {
    if (!initialThreadMessageId || !dm) return;
    const found = messages.find((m) => m.id === initialThreadMessageId);
    if (found) {
      setThreadMessage(found);
      const task = tasks?.find((t) => t.message_id === initialThreadMessageId || t.id === initialThreadMessageId);
      if (task) setThreadTask(task);
    }
  }, [initialThreadMessageId, dm, messages, tasks]);

  // Sync threadTask when tasks list changes (align with channel-view pattern)
  useEffect(() => {
    if (!threadMessage) return;
    const task = tasks?.find((t) => t.message_id === threadMessage.id || t.id === threadMessage.id);
    if (task) {
      setThreadTask((prev) => {
        if (!prev || prev.status !== task.status || prev.claimer_id !== task.claimer_id) {
          return task;
        }
        return prev;
      });
    }
  }, [tasks, threadMessage]);

  const { activeStreamingAgentIds } = useStreamingMessages(dm.id);

  const members = useMemo<ChannelMember[]>(() => {
    const result: ChannelMember[] = [
      {
        channel_id: dm.id,
        member_type: 'user',
        member_id: 'user-1',
        role: 'member',
        display_name: '我',
        status: 'online',
      },
    ];
    if (isAgent && dm.other_agent) {
      result.push({
        channel_id: dm.id,
        member_type: 'agent',
        member_id: dm.other_agent.id,
        role: 'member',
        display_name: dm.other_agent.name,
        status: 'online',
      });
    } else if (dm.other_user) {
      result.push({
        channel_id: dm.id,
        member_type: 'user',
        member_id: dm.other_user.id,
        role: 'member',
        display_name: dm.other_user.display_name,
        status: 'online',
      });
    }
    return result;
  }, [dm, isAgent]);

  const agentActivities = useMemo(() => {
    if (!isAgent) return [];
    return activeStreamingAgentIds.map((id) => {
      const agentName = dm.other_agent?.name ?? id;
      return { agentId: id, name: agentName, state: 'streaming' as const };
    });
  }, [isAgent, activeStreamingAgentIds, dm.other_agent?.name]);

  const handleRetry = useCallback(() => {
    refetch();
  }, [refetch]);

  const handleStatusChange = useCallback(
    async (task: Task, newStatus: TaskStatus) => {
      const updated = await onTaskStatusChange?.(task, newStatus);
      if (updated) {
        setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
      }
    },
    [onTaskStatusChange],
  );

  // ---- ThreadPanel handlers ----

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
    onThreadChange?.(null);
  }, [onThreadChange]);

  const handleMessageClick = useCallback(
    (message: Message) => {
      if (message.task_number != null) {
        refetchTasks?.();
        const task = tasks?.find((t) => t.message_id === message.id || t.id === message.id);
        setThreadMessage({
          ...message,
          display_name: task?.creator_name || message.display_name,
        });
        setThreadTask(task ?? null);
        onThreadChange?.(message.id);
      }
    },
    [tasks, refetchTasks, onThreadChange],
  );

  const handleTaskClickFromBoard = useCallback(
    (task: Task) => {
      if (!task.message_id) return;
      // Use task.message_id to find the original message
      const existingMsg = messages.find((m) => m.id === task.message_id);
      if (existingMsg) {
        setThreadMessage({
          ...existingMsg,
          display_name: task.creator_name || existingMsg.display_name,
        });
      } else {
        // Construct a minimal message for ThreadPanel
        setThreadMessage({
          id: task.message_id,
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
      }
      setThreadTask(task);
      onThreadChange?.(task.message_id);
    },
    [messages, refetchTasks, onThreadChange],
  );

  const handleAsTask = useCallback(
    async (message: Message) => {
      if (!onConvertToTask) return;
      try {
        const task = await onConvertToTask(dm.id, message.id);
        setThreadMessage({
          ...message,
          display_name: task?.creator_name || message.display_name,
        });
        setThreadTask(task);
        onThreadChange?.(message.id);
      } catch {
        // handled silently
      }
    },
    [dm.id, onConvertToTask, onThreadChange],
  );

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Header — card-brutal style */}
        <div className="card-brutal mx-4 mt-4 p-4">
          <div className="flex items-center gap-3">
            {/* Icon */}
            <div className={`
              flex h-10 w-10 flex-shrink-0 items-center justify-center border-2 border-black
              ${isAgent ? 'bg-brutal-lime' : 'bg-brutal-cyan'}
            `}>
              {isAgent ? (
                <Bot className="h-5 w-5 text-black" />
              ) : (
                <User className="h-5 w-5 text-black" />
              )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h2 className="font-heading font-bold text-foreground">
                  {name}
                </h2>
                {/* Online status dot */}
                <Circle className="h-2.5 w-2.5 fill-brutal-lime text-brutal-lime" />
              </div>
              <span className="font-mono text-[11px] text-muted-foreground">
                {isAgent ? 'AI Agent' : '用户'}
              </span>
            </div>
            {/* Tab bar (v1.2 Phase 2+3) */}
            <div className="flex items-center gap-1 flex-shrink-0">
              <button
                type="button"
                onClick={() => setViewTab('messages')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  viewTab === 'messages'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                消息
              </button>
              <button
                type="button"
                onClick={() => setViewTab('tasks')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  viewTab === 'tasks'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <ClipboardList className="h-3.5 w-3.5" />
                任务
              </button>
            </div>
          </div>
        </div>

        {/* Messages tab */}
        {viewTab === 'messages' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            {/* System message banner */}
            <div className="flex-shrink-0 border-b-2 border-black bg-brutal-cream px-6 py-3">
              <div className="flex items-center gap-2 font-body text-sm text-muted-foreground">
                <MessageSquare className="h-4 w-4" />
                <span>
                  这是你与 <strong className="text-foreground">{name}</strong> 的私信
                </span>
              </div>
            </div>

            {/* Error state */}
            {error && !isLoading && (
              <div className="flex flex-1 items-center justify-center">
                <div className="text-center space-y-3">
                  <AlertCircle className="mx-auto h-8 w-8 text-brutal-red" />
                  <p className="font-mono text-sm text-brutal-red">{error}</p>
                  <button
                    type="button"
                    onClick={handleRetry}
                    className="btn-brutal btn-brutal-sm gap-2"
                  >
                    <RefreshCw className="h-4 w-4" />
                    重试
                  </button>
                </div>
              </div>
            )}

            {/* Messages */}
            {(!error || isLoading) && (
              <MessageList
                messages={messages}
                isLoading={isLoading}
                error={error}
                onRetry={(id, content) => retryMessage(id, content)}
                onCancel={cancelMessage}
                onEdit={editMessage}
                onDelete={deleteMessage}
                onAsTask={handleAsTask}
                onReply={handleMessageClick}
                hasMore={hasMore}
                isLoadingMore={isLoadingMore}
                loadMoreError={loadMoreError}
                onLoadMore={loadMore}
                agentActivities={agentActivities}
              />
            )}

            {/* Input — hidden for deleted agents */}
            {!deleted && (
              <MessageInput
                onSend={async (content, mentionedAgentIds, asTask, _taskTitle, attachmentIds) => {
                  const result = await sendMessage(content, mentionedAgentIds, asTask, attachmentIds);
                  if (asTask && result?.task_number) onTaskCreated?.();
                  return result;
                }}
                members={members}
                placeholder={`发送消息给 ${name}...`}
                showAsTaskToggle
              />
            )}
            {deleted && (
              <div className="border-t-2 border-black bg-brutal-stone/20 px-4 py-3 text-center">
                <span className="badge-brutal bg-brutal-stone text-black">
                  DELETED
                </span>
                <p className="mt-2 font-body text-xs text-muted-foreground">
                  此 Agent 已被删除，无法发送消息
                </p>
              </div>
            )}
          </div>
        )}

        {/* Tasks tab (v1.2 Phase 2+3) */}
        {viewTab === 'tasks' && (
          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="flex-shrink-0 border-b-2 border-black bg-brutal-cream px-6 py-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 font-body text-sm text-muted-foreground">
                  <ClipboardList className="h-4 w-4" />
                  <span>
                    与 <strong className="text-foreground">{name}</strong> 的任务
                  </span>
                </div>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-4">
              <TaskBoard
                tasks={tasks ?? []}
                isLoading={tasksLoading ?? false}
                error={tasksError ?? null}
                onTaskClick={handleTaskClickFromBoard}
                onStatusChange={handleStatusChange}
                onRefetch={refetchTasks ?? (() => {})}
              />
            </div>
          </div>
        )}
      </div>

      {/* ThreadPanel — always mounted for smooth width transition */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] border-l-2 border-transparent"
        style={{ width: threadMessage ? threadPanelWidth : 0, borderLeftColor: threadMessage ? '#000' : 'transparent' }}
      >
        {/* Resize handle — only interactive when panel is open */}
        {threadMessage && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-pink/50 transition-colors z-10"
            onMouseDown={(e) => {
              e.preventDefault();
              const startX = e.clientX;
              const startWidth = threadPanelWidth;
              const onMove = (ev: MouseEvent) => {
                const newWidth = Math.max(280, Math.min(800, startWidth + startX - ev.clientX));
                setThreadPanelWidth(newWidth);
              };
              const onUp = () => {
                document.removeEventListener('mousemove', onMove);
                document.removeEventListener('mouseup', onUp);
              };
              document.addEventListener('mousemove', onMove);
              document.addEventListener('mouseup', onUp);
            }}
          />
        )}
        {threadMessage && (
          <Suspense
            fallback={
              <div className="flex h-full w-[400px] items-center justify-center">
                <Skeleton className="h-8 w-32" />
              </div>
            }
          >
            <ThreadPanel
              parentMessage={threadMessage}
              task={threadTask ?? undefined}
              onClose={handleThreadClose}
              onClaimTask={onClaimTask}
              onUnclaimTask={onUnclaimTask}
              replyCount={threadMessage.reply_count ?? 0}
            />
          </Suspense>
        )}
      </div>
    </div>
  );
}
