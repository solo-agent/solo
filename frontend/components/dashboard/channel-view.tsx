// ============================================================================
// ChannelView — main message area + right-side member list with Agent support
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback, lazy, Suspense } from 'react';
import { Hash, Users, Loader2, ClipboardList, MessageSquare, Plus, Eye } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMessages } from '@/lib/hooks/use-messages';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
import { useStreamingMessages } from '@/lib/hooks/use-streaming-messages';
import { useWebSocket } from '@/lib/ws-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { MemberList } from './member-list';
import { AddAgentModal } from './add-agent-modal';
import { ChannelSearch } from './channel-search';
import { TaskBoard } from '@/components/tasks/task-board';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import type { AgentActivity } from './typing-indicator';
import type { Channel, CreateTaskInput, Message, Task, TaskStatus } from '@/lib/types';

// SOLO-63-F: Lazy-load ThreadPanel (only rendered when a thread is open)
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

import { AgentViewPanel } from './agent-view-panel';

// ---- Inline quick-create task form ----

function CreateTaskInline({
  channelId,
  isSubmitting,
  onSubmit,
}: {
  channelId: string;
  isSubmitting: boolean;
  onSubmit: (input: CreateTaskInput) => Promise<void>;
}) {
  const [title, setTitle] = useState('');

  const handleSubmit = async () => {
    if (!title.trim() || isSubmitting) return;
    await onSubmit({
      channel_id: channelId,
      title: title.trim(),
    });
    setTitle('');
  };

  return (
    <div className="space-y-3">
      <label
        htmlFor="quick-create-title"
        className="block font-heading text-sm font-bold text-foreground"
      >
        任务标题
      </label>
      <input
        id="quick-create-title"
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSubmit();
          }
        }}
        placeholder="输入任务标题..."
        disabled={isSubmitting}
        className="input-brutal w-full"
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={handleSubmit}
          disabled={isSubmitting || !title.trim()}
          className="btn-brutal btn-brutal-sm btn-brutal-pink"
        >
          {isSubmitting ? '创建中...' : '创建任务'}
        </button>
      </div>
    </div>
  );
}

interface ChannelViewProps {
  channel: Channel;
  /** Optional message ID to open ThreadPanel for on mount */
  initialThreadMessageId?: string;
}

export function ChannelView({ channel, initialThreadMessageId }: ChannelViewProps) {
  const {
    messages,
    isLoading,
    error,
    sendMessage,
    retryMessage,
    cancelMessage,
    editMessage,
    deleteMessage,
    hasMore,
    isLoadingMore,
    loadMoreError,
    loadMore,
    markMessageThreadRead,
  } = useMessages(channel.id);

  const {
    members,
    users,
    agents,
    isLoading: membersLoading,
    addAgentToChannel,
    updateMemberStatus,
  } = useChannelMembers(channel.id);

  // Keep a ref to the latest agents list for use in WS event handler closures
  const agentsRef = useRef(agents);
  agentsRef.current = agents;

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [isAddAgentModalOpen, setIsAddAgentModalOpen] = useState(false);

  // ---- Thread panel width ----
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  // ---- Agent View panel state ----
  const [showAgentView, setShowAgentView] = useState(false);
  const [agentViewWidth, setAgentViewWidth] = useState(320);

  // ---- Tasks tab state (SOLO-128-F) ----
  const [channelViewTab, setChannelViewTab] = useState<'messages' | 'tasks'>('messages');
  const [isQuickCreateTaskOpen, setIsQuickCreateTaskOpen] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);

  // ---- AsTask dialog state ----
  const [asTaskTarget, setAsTaskTarget] = useState<Message | null>(null);
  const [asTaskTitle, setAsTaskTitle] = useState('');
  const [isConvertingTask, setIsConvertingTask] = useState(false);

  // ---- Channel search state (SOLO-237-F) ----
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);

  // ---- Member popover state ----
  const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);

  const { showToast } = useToast();

  const {
    tasks: channelTasks,
    isLoading: tasksLoading,
    error: tasksError,
    createTask,
    updateTask,
    claimTask,
    unclaimTask,
    convertMessageToTask,
    refetch: refetchTasks,
  } = useTasks({ channel_id: channel.id });

  // ---- Agent thinking/typing/streaming status tracking (SOLO-47-F, SOLO-52-F) ----
  const [thinkingAgentNames, setThinkingAgentNames] = useState<string[]>([]);
  const [typingAgentNames, setTypingAgentNames] = useState<string[]>([]);
  const typingTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(
    new Map(),
  );

  const { activeStreamingAgentIds } = useStreamingMessages(channel.id);

  const { onEvent } = useWebSocket();

  // Listen for agent status events from WS (works with both real and mock WS)
  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'agent.thinking' || event.type === 'agent.typing') {
        if (event.channel_id !== channel.id) return;

        const isThinking = event.type === 'agent.thinking';
        updateMemberStatus(event.agent_id, isThinking ? 'thinking' : 'typing');

        const agent = agentsRef.current.find(
          (a) => a.member_id === event.agent_id,
        );
        const agentName = agent?.display_name ?? event.agent_id;
        const setFn = isThinking ? setThinkingAgentNames : setTypingAgentNames;
        setFn((prev) => {
          if (prev.includes(agentName)) return prev;
          return [...prev, agentName];
        });

        // Auto-clear after 5s
        const existing = typingTimersRef.current.get(event.agent_id);
        if (existing) clearTimeout(existing);
        const timer = setTimeout(() => {
          setFn((prev) => prev.filter((n) => n !== agentName));
          typingTimersRef.current.delete(event.agent_id);
          updateMemberStatus(event.agent_id, 'online');
        }, 5000);
        typingTimersRef.current.set(event.agent_id, timer);
      }

      // When an agent sends a message, clear all their statuses
      if (
        event.type === 'message.new' &&
        event.channel_id === channel.id &&
        event.sender_type === 'agent' &&
        event.sender_id
      ) {
        const name = event.sender_name;
        if (name) {
          setThinkingAgentNames((prev) => prev.filter((n) => n !== name));
          setTypingAgentNames((prev) => prev.filter((n) => n !== name));
        }
        const timer = typingTimersRef.current.get(event.sender_id);
        if (timer) {
          clearTimeout(timer);
          typingTimersRef.current.delete(event.sender_id);
        }
        updateMemberStatus(event.sender_id, 'online');
      }
    });

    return () => {
      unsub();
      for (const timer of typingTimersRef.current.values()) {
        clearTimeout(timer);
      }
      typingTimersRef.current.clear();
    };
  }, [channel.id, onEvent, updateMemberStatus]);

  // ---- Task associated with the open thread (for metadata bar) ----
  const [threadTask, setThreadTask] = useState<Task | null>(null);

  // ---- Handle initialThreadMessageId: watch messages list for the target ----
  useEffect(() => {
    if (!initialThreadMessageId || !channel) return;

    // Check if the message is already in the loaded list
    const found = messages.find((m) => m.id === initialThreadMessageId);
    if (found) {
      setThreadMessage(found);
      // Try to find the associated task for the metadata bar
      const task = channelTasks.find((t) => t.message_id === initialThreadMessageId);
      if (task) setThreadTask(task);
    }
    // If not found yet, it will be caught when messages load (via the next effect)
  }, [initialThreadMessageId, channel, messages, channelTasks]);

  // Sync threadTask when channelTasks change
  useEffect(() => {
    if (threadMessage && !threadTask) {
      const task = channelTasks.find((t) => t.message_id === threadMessage.id);
      if (task) setThreadTask(task);
    }
  }, [channelTasks, threadMessage, threadTask]);

  // ---- Task click in tasks tab: open ThreadPanel with the parent message ----
  const handleTaskClickInTab = useCallback(
    (task: Task) => {
      // If task has no message_id, can't open thread
      if (!task.message_id) return;

      // Find message in the already-loaded channel messages
      const existingMsg = messages.find((m) => m.id === task.message_id);
      if (existingMsg) {
        setThreadMessage(existingMsg);
        setThreadTask(task);
        return;
      }

      // Message not in current loaded set — switch to messages tab
      // and the message may be loaded via pagination
      setChannelViewTab('messages');
      setThreadTask(task);
      // Construct a minimal message object for the ThreadPanel
      // The useThread hook will load the actual thread messages
      setThreadMessage({
        id: task.message_id,
        channel_id: channel.id,
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
    },
    [channel.id, messages],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
  }, []);

  // P25-08-F: Called by ThreadPanel after successfully marking thread as read
  const handleThreadMarkRead = useCallback(() => {
    if (threadMessage) {
      markMessageThreadRead(threadMessage.id);
    }
  }, [threadMessage, markMessageThreadRead]);

  const existingAgentIds = agents.map((a) => a.member_id);

  // ---- Task quick-create handler (SOLO-128-F) ----

  const handleQuickCreateTask = async (input: CreateTaskInput) => {
    setIsCreatingTask(true);
    try {
      await createTask({
        channel_id: channel.id,
        title: input.title,
        description: input.description,
        priority: input.priority,
        assignee_id: input.assignee_id,
        assignee_type: input.assignee_type,
        due_date: input.due_date,
      });
      setIsQuickCreateTaskOpen(false);
    } finally {
      setIsCreatingTask(false);
    }
  };

  // ---- Claim / Unclaim handlers ----

  const handleClaim = async (task: Task) => {
    try {
      await claimTask(task.channel_id, task.id);
      showToast(`已认领任务 #${task.task_number ?? '?'}`, 'success');
    } catch {
      // 409: silent — per spec, no error toast
    }
  };

  const handleUnclaim = async (task: Task) => {
    try {
      await unclaimTask(task.channel_id, task.id);
      showToast(`已释放任务 #${task.task_number ?? '?'}`, 'info');
    } catch {
      // Errors handled silently for claim/unclaim
    }
  };

  // ---- AsTask handler ----

  const handleAsTaskOpen = (message: Message) => {
    setAsTaskTarget(message);
    setAsTaskTitle(message.content.slice(0, 200));
  };

  const handleAsTaskConfirm = async () => {
    if (!asTaskTarget || !asTaskTitle.trim()) return;
    setIsConvertingTask(true);
    try {
      const task = await convertMessageToTask(
        asTaskTarget.channel_id,
        asTaskTarget.id,
        asTaskTitle.trim(),
      );
      showToast(`已转为任务 #${task.task_number ?? '?'}`, 'success');
      setAsTaskTarget(null);
      setAsTaskTitle('');
    } catch {
      showToast('转换任务失败，请稍后再试', 'error');
    } finally {
      setIsConvertingTask(false);
    }
  };

  const handleStatusChange = async (task: Task, newStatus: TaskStatus) => {
    try {
      await updateTask(task.id, { status: newStatus });
    } catch {
      // handled by hook
    }
  };

  // Compute the combined agent activity array for the typing indicator
  const agentActivities: AgentActivity[] = [
    // thinking agents
    ...thinkingAgentNames.map((name) => {
      const member = agents.find((a) => a.display_name === name);
      return { agentId: member?.member_id ?? name, name, state: 'thinking' as const };
    }),
    // typing agents (pre-streaming)
    ...typingAgentNames
      .filter((n) => !thinkingAgentNames.includes(n))
      .map((name) => {
        const member = agents.find((a) => a.display_name === name);
        return { agentId: member?.member_id ?? name, name, state: 'typing' as const };
      }),
    // streaming agents
    ...activeStreamingAgentIds.map((id) => {
      const member = agents.find((a) => a.member_id === id);
      const name = member?.display_name ?? id;
      return { agentId: id, name, state: 'streaming' as const };
    }),
  ];

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Left: message area */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Channel header */}
        <div className="flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <Hash className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
            <h2 className="font-semibold text-foreground truncate">{channel.name}</h2>
            <div className="mx-2 h-4 w-px bg-border flex-shrink-0" />
            {/* Channel tab bar (SOLO-128-F) */}
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setChannelViewTab('messages')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  channelViewTab === 'messages'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                消息
              </button>
              <button
                type="button"
                onClick={() => setChannelViewTab('tasks')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  channelViewTab === 'tasks'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <ClipboardList className="h-3.5 w-3.5" />
                任务
              </button>
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
            <button
              type="button"
              onClick={() => setShowAgentView(prev => !prev)}
              className={cn(
                'flex h-8 w-8 items-center justify-center border-2 border-black shadow-brutal-sm transition-colors',
                showAgentView
                  ? 'bg-brutal-pink text-black'
                  : 'bg-white hover:bg-brutal-cream',
              )}
              aria-label="Agent View"
              title="Agent View"
            >
              <Eye className="h-4 w-4" />
            </button>
            {/* SOLO-237-F: Channel-internal search */}
            {channelViewTab === 'messages' && (
              <ChannelSearch
                channelId={channel.id}
                channelName={channel.name}
                onResultClick={(msgId) => {
                  setScrollToMessageId(msgId);
                  setScrollMsgKey((k) => k + 1);
                }}
              />
            )}
            <button
              type="button"
              onClick={() => setIsMemberPopoverOpen(true)}
              className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-cream transition-colors"
              aria-label="频道成员"
              title="频道成员"
            >
              <Users className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Messages tab (SOLO-128-F) */}
        {channelViewTab === 'messages' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            <MessageList
              messages={messages}
              isLoading={isLoading}
              error={error}
              onRetry={(id, content) => retryMessage(id, content)}
              onCancel={(id) => cancelMessage(id)}
              onReply={setThreadMessage}
              onEdit={(id, content) => editMessage(id, content)}
              onDelete={(id) => deleteMessage(id)}
              onAsTask={handleAsTaskOpen}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              loadMoreError={loadMoreError}
              onLoadMore={loadMore}
              agentActivities={agentActivities}
              scrollToMessageId={scrollToMessageId}
              scrollKey={scrollMsgKey}
            />
            <MessageInput
              onSend={async (content, _mentionedAgentIds, asTask, taskTitle, attachmentIds) => {
                if (asTask) {
                  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(`已创建任务 #${result.task_number}`, 'success');
                  }
                } else {
                  const result = await sendMessage(content, _mentionedAgentIds, undefined, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(`已创建任务 #${result.task_number}`, 'success');
                  }
                }
              }}
              members={members}
              showAsTaskToggle
            />
          </div>
        )}

        {/* Tasks tab (SOLO-128-F) */}
        {channelViewTab === 'tasks' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            <div className="flex-1 overflow-y-auto px-4 py-4">
              <div className="mb-4 flex items-center justify-between">
                <h3 className="font-heading text-sm font-bold text-foreground">
                  #{channel.name} 的任务
                </h3>
                <button
                  type="button"
                  onClick={() => setIsQuickCreateTaskOpen(true)}
                  className="btn-brutal btn-brutal-sm"
                >
                  <Plus className="mr-1 h-3.5 w-3.5" />
                  快速创建
                </button>
              </div>
              <TaskBoard
                tasks={channelTasks}
                isLoading={tasksLoading}
                error={tasksError}
                onTaskClick={handleTaskClickInTab}
                onStatusChange={handleStatusChange}
                onRefetch={refetchTasks}
                onClaim={handleClaim}
                onUnclaim={handleUnclaim}
              />
            </div>
          </div>
        )}
      </div>

      {/* Agent View panel */}
      <AgentViewPanel
        channelId={channel.id}
        visible={showAgentView}
        width={agentViewWidth}
        onWidthChange={setAgentViewWidth}
      />

      {/* Thread panel (lazy-loaded, SOLO-63-F) */}
      {threadMessage && (
        <div
          className="flex-shrink-0 bg-brutal-cream overflow-hidden relative"
          style={{ width: threadPanelWidth }}
        >
          {/* Resize handle */}
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
              members={members}
              replyCount={threadMessage.reply_count ?? 0}
              task={threadTask ?? undefined}
              onClaimTask={handleClaim}
              onUnclaimTask={handleUnclaim}
              onMarkRead={handleThreadMarkRead}
            />
          </Suspense>
        </div>
      )}

      {/* Add Agent to Channel modal */}
      <AddAgentModal
        open={isAddAgentModalOpen}
        onOpenChange={setIsAddAgentModalOpen}
        existingAgentIds={existingAgentIds}
        onAdd={addAgentToChannel}
      />

      {/* Quick-create task dialog (SOLO-128-F) */}
      <Dialog
        open={isQuickCreateTaskOpen}
        onOpenChange={setIsQuickCreateTaskOpen}
      >
        <DialogHeader>
          <DialogTitle>快速创建任务</DialogTitle>
          <DialogCloseButton onClick={() => setIsQuickCreateTaskOpen(false)} />
        </DialogHeader>
        <div className="mt-4 px-5 pb-5">
          <CreateTaskInline
            channelId={channel.id}
            isSubmitting={isCreatingTask}
            onSubmit={handleQuickCreateTask}
          />
        </div>
      </Dialog>

      {/* AsTask confirm dialog */}
      <Dialog
        open={!!asTaskTarget}
        onOpenChange={() => setAsTaskTarget(null)}
      >
        <DialogHeader>
          <DialogTitle>转为任务</DialogTitle>
          <DialogCloseButton onClick={() => setAsTaskTarget(null)} />
        </DialogHeader>
        <div className="mt-4 px-5 pb-5">
          <label
            htmlFor="as-task-title"
            className="mb-2 block font-heading text-sm font-bold text-foreground"
          >
            任务标题
          </label>
          <input
            id="as-task-title"
            type="text"
            value={asTaskTitle}
            onChange={(e) => setAsTaskTitle(e.target.value)}
            placeholder="输入任务标题..."
            className="input-brutal w-full"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleAsTaskConfirm();
              }
            }}
          />
          {asTaskTarget && (
            <p className="mt-2 font-mono text-[11px] text-muted-foreground">
              来源: {asTaskTarget.display_name} 的消息
            </p>
          )}
          <div className="mt-4 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setAsTaskTarget(null)}
              disabled={isConvertingTask}
              className="btn-brutal btn-brutal-sm"
            >
              取消
            </button>
            <button
              type="button"
              onClick={handleAsTaskConfirm}
              disabled={isConvertingTask || !asTaskTitle.trim()}
              className="btn-brutal btn-brutal-sm btn-brutal-pink"
            >
              {isConvertingTask ? '创建中...' : '转为任务'}
            </button>
          </div>
        </div>
      </Dialog>

      {/* Member popover */}
      <Dialog open={isMemberPopoverOpen} onOpenChange={setIsMemberPopoverOpen}>
        <DialogHeader>
          <DialogTitle>
            <div className="flex items-center gap-2">
              <Users className="h-4 w-4" />
              频道成员
              <span className="font-mono text-sm font-normal text-muted-foreground">
                ({users.length + agents.length})
              </span>
            </div>
          </DialogTitle>
          <DialogCloseButton onClick={() => setIsMemberPopoverOpen(false)} />
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          <MemberList
            users={users}
            agents={agents}
            isLoading={membersLoading}
            onAddAgent={() => {
              setIsMemberPopoverOpen(false);
              setIsAddAgentModalOpen(true);
            }}
          />
        </div>
      </Dialog>

      {/* Mobile: member button */}
      <div className="lg:hidden">
        <button
          type="button"
          onClick={() => setIsMemberPopoverOpen(true)}
          className="btn-brutal fixed bottom-4 right-4 z-40 flex h-10 w-10 items-center justify-center shadow-brutal"
          aria-label="成员"
        >
          <Users className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
