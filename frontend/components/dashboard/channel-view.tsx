// ============================================================================
// ChannelView — main message area + right-side member list with Agent support
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback, lazy, Suspense } from 'react';
import { Users, Loader2, ClipboardList, MessageSquare, Eye, Plus, BookOpen } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMessages } from '@/lib/hooks/use-messages';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
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
import { WizardCard } from '@/components/onboarding/wizard-card';
import { t } from '@/lib/i18n';
import type { Channel, Message, Task, TaskStatus } from '@/lib/types';

// SOLO-63-F: Lazy-load ThreadPanel (only rendered when a thread is open)
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

import { AgentViewPanel } from './agent-view-panel';
import { KnowledgePanel } from '@/components/knowledge/knowledge-panel';

interface ChannelViewProps {
  channel: Channel;
  /** Show the onboarding wizard card above the message list */
  showOnboardingWizard?: boolean;
  /** Optional message ID to open ThreadPanel for on mount */
  initialThreadMessageId?: string;
  /** Optional message ID to scroll to on mount */
  initialScrollToMessageId?: string;
  /** v1.5: Called when thread opens/closes so the parent can sync to URL */
  onThreadChange?: (threadId: string | null) => void;
  /** SOLO-island PR3: whether the right-side AgentViewPanel is visible. */
  agentViewVisible?: boolean;
  /** SOLO-island PR3: toggle the AgentViewPanel. Called with `true` to
   * open, `false` to close. */
  onAgentViewVisibleChange?: (visible: boolean) => void;
  /** SOLO-island PR3: width of the AgentViewPanel (controlled by parent
   * so it can outlive unmounts). */
  agentViewWidth?: number;
  /** SOLO-island PR3: called when the user drags the panel's resize
   * handle. Parent should persist the new width. */
  onAgentViewWidthChange?: (width: number) => void;
  /** SOLO-island PR3: when set, the panel scrolls/highlights this agent
   * (driven by AgentIsland's "查看完整 trace" action). */
  agentViewFocusedAgentId?: string | null;
}

export function ChannelView({
  channel,
  showOnboardingWizard,
  initialThreadMessageId,
  initialScrollToMessageId,
  onThreadChange,
  agentViewVisible,
  onAgentViewVisibleChange,
  agentViewWidth,
  onAgentViewWidthChange,
  agentViewFocusedAgentId,
}: ChannelViewProps) {
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
    removeMember,
    updateMemberStatus,
  } = useChannelMembers(channel.id);

  // Keep a ref to the latest agents list for use in WS event handler closures
  const agentsRef = useRef(agents);
  agentsRef.current = agents;

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [isAddAgentModalOpen, setIsAddAgentModalOpen] = useState(false);

  // ---- Thread panel width ----
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  // ---- Agent View panel state (SOLO-island PR3) ----
  // Controlled by the parent (dashboard) so the AgentIsland can summon
  // the panel from outside this component. The Eye button in the channel
  // header toggles via the same callback; the parent owns the actual
  // state. This component is treated as "always rendered with a parent
  // controller" — if you need to mount it standalone, wrap a small
  // component that owns the boolean state.
  const showAgentView = !!agentViewVisible;
  const effectiveAgentViewWidth = agentViewWidth ?? 320;
  const toggleAgentView = () => {
    onAgentViewVisibleChange?.(!showAgentView);
  };

  // ---- Tasks tab state (SOLO-128-F) ----
  const [channelViewTab, setChannelViewTab] = useState<'messages' | 'tasks'>('messages');

  // ---- Channel search state (SOLO-237-F) ----
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);

  // ---- Member popover state ----
  const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);

  // ---- Knowledge panel state (Step 4) ----
  const [isKnowledgeOpen, setIsKnowledgeOpen] = useState(false);

  const { showToast } = useToast();

  const {
    tasks: channelTasks,
    isLoading: tasksLoading,
    error: tasksError,
    updateTask,
    claimTask,
    unclaimTask,
    convertMessageToTask,
    refetch: refetchTasks,
  } = useTasks({ channel_id: channel.id });

  // Refetch tasks when switching to tasks tab
  useEffect(() => {
    if (channelViewTab === 'tasks') refetchTasks();
  }, [channelViewTab]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- Agent member status tracking (SOLO-47-F) ----
  // SOLO-island PR2: removed thinkingAgentNames/typingAgentNames state and
  // the TypingIndicator badge. AgentIsland (mounted at the dashboard root)
  // is now the single source of truth for "agent is working". The member
  // list (right column) still updates its dots to reflect thinking/typing
  // /online so the avatar stays in sync, but with a much simpler model.
  //
  // PR-fix: the previous 5s inactivity heuristic was the same class of
  // bug as the 3s heuristic in useAgentChunks (fixed in PR0). It would
  // prematurely mark an agent as online during long-running tool calls.
  // Now we rely on agent.done as the authoritative terminal signal and
  // treat thinking/typing as transient — cleared the moment we see
  // a done event (or message.new) for the same agent.
  const memberStatusTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(
    new Map(),
  );

  /** Cancel any pending revert timer for an agent. */
  const cancelMemberStatusRevert = (agentId: string) => {
    const t = memberStatusTimersRef.current.get(agentId);
    if (t) {
      clearTimeout(t);
      memberStatusTimersRef.current.delete(agentId);
    }
  };

  /**
   * Schedule a fallback revert to online after `ms` ms. This is a safety
   * net for paths that don't emit agent.done (e.g. the legacy LLM
   * provider fallback). Primary terminal signal remains agent.done.
   */
  const scheduleMemberStatusRevert = (agentId: string, ms: number) => {
    cancelMemberStatusRevert(agentId);
    const timer = setTimeout(() => {
      updateMemberStatus(agentId, 'online');
      memberStatusTimersRef.current.delete(agentId);
    }, ms);
    memberStatusTimersRef.current.set(agentId, timer);
  };

  const { onEvent } = useWebSocket();

  // Listen for agent status events from WS (works with both real and mock WS)
  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'agent.thinking' || event.type === 'agent.typing') {
        if (event.channel_id !== channel.id) return;

        const isThinking = event.type === 'agent.thinking';
        updateMemberStatus(event.agent_id, isThinking ? 'thinking' : 'typing');
        // Fallback only — agent.done will cancel and immediately revert.
        scheduleMemberStatusRevert(event.agent_id, 30_000);
      }

      // agent.done is the authoritative terminal signal. Clear any
      // fallback timer and snap status back to online immediately.
      if (event.type === 'agent.done' && event.channel_id === channel.id && event.agent_id) {
        cancelMemberStatusRevert(event.agent_id);
        updateMemberStatus(event.agent_id, 'online');
      }

      // When an agent sends a message, revert their member status to online.
      if (
        event.type === 'message.new' &&
        event.channel_id === channel.id &&
        event.sender_type === 'agent' &&
        event.sender_id
      ) {
        cancelMemberStatusRevert(event.sender_id);
        updateMemberStatus(event.sender_id, 'online');
      }
    });

    return () => {
      unsub();
      for (const timer of memberStatusTimersRef.current.values()) {
        clearTimeout(timer);
      }
      memberStatusTimersRef.current.clear();
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

  // Handle initialScrollToMessageId: scroll to a specific message on mount or URL change.
  // Waits for isLoading to become false so the message DOM exists.
  const lastScrollTarget = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!initialScrollToMessageId || !channel || isLoading) return;
    if (lastScrollTarget.current === initialScrollToMessageId) return;
    lastScrollTarget.current = initialScrollToMessageId;
    setScrollToMessageId(initialScrollToMessageId);
    setScrollMsgKey((k) => k + 1);
  }, [initialScrollToMessageId, channel, isLoading]);

  // Sync threadTask when channelTasks change (e.g. after WS task.updated)
  useEffect(() => {
    if (!threadMessage) return;
    const task = channelTasks.find((t) => t.message_id === threadMessage.id);
    if (task) {
      setThreadTask((prev) => {
        // Only update if actually changed to avoid re-render loops
        if (!prev || prev.status !== task.status || prev.claimer_id !== task.claimer_id) {
          return task;
        }
        return prev;
      });
    }
  }, [channelTasks, threadMessage]);

  // ---- Task click in tasks tab: open ThreadPanel with the parent message ----
  const handleTaskClickInTab = useCallback(
    (task: Task) => {
      if (!task.message_id) return;

      // Find message in the already-loaded channel messages
      const existingMsg = messages.find((m) => m.id === task.message_id);
      if (existingMsg) {
        setThreadMessage({
          ...existingMsg,
          display_name: task.creator_name || existingMsg.display_name,
        });
        setThreadTask(task);
        onThreadChange?.(task.message_id);
        return;
      }

      // Message not in current loaded set — ThreadPanel loads its own
      // data via useThread so a synthetic parent message is sufficient
      setThreadTask(task);
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
      onThreadChange?.(task.message_id);
    },
    [channel.id, messages, onThreadChange],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
    onThreadChange?.(null);
  }, [onThreadChange]);

  // v1.5: Wrap onReply to also sync thread state to URL + pull latest task data
  const handleReply = useCallback(
    (message: Message) => {
      refetchTasks();
      setThreadMessage(message);
      onThreadChange?.(message.id);
    },
    [refetchTasks, onThreadChange],
  );

  // P25-08-F: Called by ThreadPanel after successfully marking thread as read
  const handleThreadMarkRead = useCallback(() => {
    if (threadMessage) {
      markMessageThreadRead(threadMessage.id);
    }
  }, [threadMessage, markMessageThreadRead]);

  const existingAgentIds = agents.map((a) => a.member_id);

  // ---- Task quick-create handler (SOLO-128-F) ----

  // ---- Claim / Unclaim handlers ----

  const handleClaim = async (task: Task) => {
    try {
      const updated = await claimTask(task.channel_id, task.id);
      setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
      showToast(t('taskClaimed', { n: task.task_number ?? '?' }), 'success');
    } catch {
      // 409: silent — per spec, no error toast
    }
  };

  const handleUnclaim = async (task: Task) => {
    try {
      const updated = await unclaimTask(task.channel_id, task.id);
      setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
      showToast(t('taskReleased', { n: task.task_number ?? '?' }), 'info');
    } catch {
      // Errors handled silently for claim/unclaim
    }
  };

  // ---- AsTask handler: convert directly, no dialog ----
  const handleAsTaskOpen = useCallback(
    async (message: Message) => {
      const title = message.content.slice(0, 200);
      try {
        const task = await convertMessageToTask(
          message.channel_id,
          message.id,
          title,
        );
        showToast(t('taskConverted', { n: task.task_number ?? '?' }), 'success');
      } catch {
        showToast(t('taskConvertError'), 'error');
      }
    },
    [convertMessageToTask, showToast],
  );

  const handleStatusChange = async (task: Task, newStatus: TaskStatus) => {
    try {
      const updated = await updateTask(task.channel_id, task.id, { status: newStatus });
      setThreadTask((prev) => (prev?.id === task.id ? updated : prev));
    } catch {
      // handled by hook
    }
  };

  // SOLO-island PR2: removed agentActivities aggregation — the
  // TypingIndicator it fed is now replaced by AgentIsland, which
  // subscribes to agent.activity events directly.

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Left: message area */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Channel header */}
        <div className="flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <span className="font-mono text-base font-bold text-black flex-shrink-0">#</span>
            <h2 className="font-bold text-foreground truncate">{channel.name}</h2>
            <div className="mx-2 h-4 w-px bg-border flex-shrink-0" />
            {/* Channel tab bar (SOLO-128-F) */}
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setChannelViewTab('messages')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold border-2 transition-all',
                  'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
                  channelViewTab === 'messages'
                    ? 'bg-brutal-primary text-black border-black shadow-brutal-sm -translate-y-px'
                    : 'text-muted-foreground hover:text-foreground border-transparent hover:border-black hover:bg-white hover:shadow-brutal-sm hover:-translate-y-px',
                )}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                {t('messages')}
              </button>
              <button
                type="button"
                onClick={() => setChannelViewTab('tasks')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold border-2 transition-all',
                  'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
                  channelViewTab === 'tasks'
                    ? 'bg-brutal-primary text-black border-black shadow-brutal-sm -translate-y-px'
                    : 'text-muted-foreground hover:text-foreground border-transparent hover:border-black hover:bg-white hover:shadow-brutal-sm hover:-translate-y-px',
                )}
              >
                <ClipboardList className="h-3.5 w-3.5" />
                {t('tasks')}
              </button>
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
            <button
              type="button"
              onClick={toggleAgentView}
              className={cn(
                'flex h-8 w-8 items-center justify-center border-2 border-black shadow-brutal-sm transition-colors',
                showAgentView
                  ? 'bg-brutal-primary text-black'
                  : 'bg-white hover:bg-brutal-cream',
              )}
              aria-label="Agent View"
              title="Agent View"
            >
              <Eye className="h-4 w-4" />
            </button>
            {/* Knowledge panel button (Step 4) */}
            <button
              type="button"
              onClick={() => setIsKnowledgeOpen(true)}
              className={cn(
                'flex h-8 w-8 items-center justify-center border-2 border-black shadow-brutal-sm transition-colors',
                'bg-white hover:bg-brutal-cream',
              )}
              aria-label={t('knowledgePanelButton')}
              title={t('knowledgePanelButton')}
            >
              <BookOpen className="h-4 w-4" />
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
              aria-label={t('channelMembers')}
              title={t('channelMembers')}
            >
              <Users className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Messages tab (SOLO-128-F) */}
        {channelViewTab === 'messages' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            {showOnboardingWizard && (
              <div className="px-4 pt-4">
                <WizardCard channelId={channel.id} />
              </div>
            )}
            <MessageList
              messages={messages}
              isLoading={isLoading}
              error={error}
              onRetry={(id, content) => retryMessage(id, content)}
              onCancel={(id) => cancelMessage(id)}
              onReply={handleReply}
              onEdit={(id, content) => editMessage(id, content)}
              onDelete={(id) => deleteMessage(id)}
              onAsTask={handleAsTaskOpen}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              loadMoreError={loadMoreError}
              onLoadMore={loadMore}
              scrollToMessageId={scrollToMessageId}
              scrollKey={scrollMsgKey}
              members={members}
            />
            <MessageInput
              onSend={async (content, _mentionedAgentIds, asTask, taskTitle, attachmentIds) => {
                if (asTask) {
                  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
                  }
                } else {
                  const result = await sendMessage(content, _mentionedAgentIds, undefined, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
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
              <h3 className="mb-4 font-heading text-sm font-bold text-foreground">
                  {t('channelTasks', { channel: channel.name })}
                </h3>
              <TaskBoard
                tasks={channelTasks}
                isLoading={tasksLoading}
                error={tasksError}
                onTaskClick={handleTaskClickInTab}
                onStatusChange={handleStatusChange}
                onRefetch={refetchTasks}
              />
            </div>
          </div>
        )}
      </div>

      {/* Agent View panel — SOLO-island PR3: controlled by parent, so the
          AgentIsland (mounted at the dashboard root) can summon it. */}
      {showAgentView && (
        <AgentViewPanel
          channelId={channel.id}
          width={effectiveAgentViewWidth}
          onWidthChange={onAgentViewWidthChange ?? (() => {})}
          focusedAgentId={agentViewFocusedAgentId}
          onClose={() => onAgentViewVisibleChange?.(false)}
        />
      )}

      {/* Thread panel (lazy-loaded, SOLO-63-F) — always mounted for smooth width transition */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-[width] duration-100 ease-linear border-l-2 border-transparent"
        style={{ width: threadMessage ? threadPanelWidth : 0, borderLeftColor: threadMessage ? 'var(--color-border, #000)' : 'transparent' }}
      >
        {/* Resize handle — only interactive when panel is open */}
        {threadMessage && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
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
        )}
      </div>

      {/* Add Agent to Channel modal */}
      <AddAgentModal
        open={isAddAgentModalOpen}
        onOpenChange={setIsAddAgentModalOpen}
        existingAgentIds={existingAgentIds}
        onAdd={addAgentToChannel}
      />


      {/* Member popover */}
      <Dialog open={isMemberPopoverOpen} onOpenChange={setIsMemberPopoverOpen}>
        <DialogHeader>
          <DialogTitle>
            <div className="flex items-center gap-2">
              <Users className="h-4 w-4" />
              {t('channelMembers')}
              <span className="font-mono text-sm font-normal text-muted-foreground">
                ({users.length + agents.length})
              </span>
            </div>
          </DialogTitle>
          <div className="flex items-center gap-1">
            <button
              onClick={() => {
                setIsMemberPopoverOpen(false);
                setIsAddAgentModalOpen(true);
              }}
              className="flex h-7 w-7 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-primary-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
              aria-label={t('addAgentToChannel')}
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
            <DialogCloseButton onClick={() => setIsMemberPopoverOpen(false)} />
          </div>
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
            onRemoveAgent={(memberId) => removeMember('agent', memberId)}
            showHeader={false}
          />
        </div>
      </Dialog>

      {/* Knowledge Panel Dialog (Step 4) */}
      <Dialog open={isKnowledgeOpen} onOpenChange={setIsKnowledgeOpen} width="md">
        <DialogHeader>
          <DialogTitle>
            <BookOpen className="inline h-4 w-4 mr-1.5 -mt-0.5" />
            {t('knowledgeChannelPanelTitle')}
            <span className="ml-2 font-mono text-sm font-normal text-muted-foreground">
              #{channel.name}
            </span>
          </DialogTitle>
          <DialogCloseButton onClick={() => setIsKnowledgeOpen(false)} />
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          <KnowledgePanel
            channelId={channel.id}
            compact
          />
        </div>
      </Dialog>

      {/* Mobile: member button */}
      <div className="lg:hidden">
        <button
          type="button"
          onClick={() => setIsMemberPopoverOpen(true)}
          className="btn-brutal fixed bottom-4 right-4 z-40 flex h-10 w-10 items-center justify-center shadow-brutal"
          aria-label={t('members')}
        >
          <Users className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
