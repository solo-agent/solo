// ============================================================================
// ChannelView — main message area + right-side member list with Agent support
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback, useMemo, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Users, Loader2, SquareCheckBig, MessageSquare, Plus, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMessages } from '@/lib/hooks/use-messages';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
import { useWebSocket } from '@/lib/ws-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { TaskArtifactGenerationInProgressError, TaskArtifactStillPendingError, useTaskArtifact } from '@/lib/hooks/use-task-artifact';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { MemberList } from './member-list';
import { AddAgentModal } from './add-agent-modal';
import { TaskBoard } from '@/components/tasks/task-board';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { tabButtonClass } from '@/components/ui/tab-bar';
import { filterTaskTree } from '@/lib/task-filters';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import { WizardCard } from '@/components/onboarding/wizard-card';
import { t } from '@/lib/i18n';
import type { AgentDetailTarget, Channel, Message, Task, TaskArtifact } from '@/lib/types';

type ArtifactPreview = TaskArtifact & { previewUrl: string };

// SOLO-63-F: Lazy-load ThreadPanel (only rendered when a thread is open)
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);


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
  const router = useRouter();
  const searchParams = useSearchParams();
  const {
    messages,
    isLoading,
    error,
    sendMessage,
    retryMessage,
    cancelMessage,
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
  const [selectedAgentDetail, setSelectedAgentDetail] = useState<AgentDetailTarget | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | null>(null);
  const [artifactHistory, setArtifactHistory] = useState<TaskArtifact[]>([]);
  const [activeRightPanel, setActiveRightPanel] = useState<'thread' | 'agent' | null>(null);
  const rightPanelOpen = activeRightPanel !== null;

  // ---- Thread panel width ----
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  // ---- Tasks tab state (SOLO-128-F) ----
  const [channelViewTab, setChannelViewTab] = useState<'messages' | 'tasks'>(
    searchParams.get('tab') === 'tasks' ? 'tasks' : 'messages',
  );

  // ---- Channel search state (SOLO-237-F) ----
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);

  // ---- Member popover state ----
  const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);

  const { showToast } = useToast();
  const { generateArtifact, finalizeArtifact, fetchArtifactHTML, listArtifacts, isGenerating } = useTaskArtifact();
  const artifactOpenLinkRef = useRef<HTMLAnchorElement>(null);
  const artifactFinalizeButtonRef = useRef<HTMLButtonElement>(null);
  const artifactFrameRef = useRef<HTMLIFrameElement>(null);
  const artifactCloseButtonRef = useRef<HTMLButtonElement>(null);
  const artifactReturnFocusRef = useRef<HTMLElement | null>(null);
  const artifactPreviewUrlRef = useRef<string | null>(null);

  const closeArtifactPreview = useCallback(() => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
      artifactPreviewUrlRef.current = null;
    }
    setArtifactPreview(null);
  }, []);

  useEffect(() => () => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
      artifactPreviewUrlRef.current = null;
    }
  }, []);

  const showArtifactPreview = useCallback(async (artifact: TaskArtifact) => {
    const html = await fetchArtifactHTML(artifact);
    const previewUrl = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
    const previousPreviewUrl = artifactPreviewUrlRef.current;
    artifactPreviewUrlRef.current = previewUrl;
    setArtifactPreview({ ...artifact, previewUrl });
    if (previousPreviewUrl) {
      URL.revokeObjectURL(previousPreviewUrl);
    }
  }, [fetchArtifactHTML]);

  const refreshArtifactHistory = useCallback(async (taskId: string) => {
    const artifacts = await listArtifacts(taskId);
    setArtifactHistory(artifacts);
    return artifacts;
  }, [listArtifacts]);

  const showLatestPublishedArtifact = useCallback(async (taskId: string) => {
    const artifacts = await refreshArtifactHistory(taskId);
    const published = artifacts.find((artifact) => artifact.summary !== 'pending');
    if (published) {
      await showArtifactPreview(published);
      return true;
    }
    return false;
  }, [refreshArtifactHistory, showArtifactPreview]);

  useEffect(() => {
    if (!artifactPreview) return;

    artifactCloseButtonRef.current?.focus();
    const handleArtifactPreviewKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeArtifactPreview();
        return;
      }

      if (event.key === 'Tab') {
        const controls = ([artifactOpenLinkRef.current, artifactFinalizeButtonRef.current, artifactFrameRef.current, artifactCloseButtonRef.current] as Array<HTMLElement | null>).filter(
          (control): control is HTMLElement => Boolean(control),
        );
        if (controls.length === 0) return;

        const firstControl = controls[0];
        const lastControl = controls[controls.length - 1];
        const activeElement = document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;

        if (event.shiftKey && activeElement === firstControl) {
          event.preventDefault();
          lastControl.focus();
        } else if (!event.shiftKey && activeElement === lastControl) {
          event.preventDefault();
          firstControl.focus();
        } else if (!activeElement || !controls.includes(activeElement)) {
          event.preventDefault();
          firstControl.focus();
        }
      }
    };

    document.addEventListener('keydown', handleArtifactPreviewKeyDown);
    return () => {
      document.removeEventListener('keydown', handleArtifactPreviewKeyDown);
      artifactReturnFocusRef.current?.focus();
      artifactReturnFocusRef.current = null;
    };
  }, [artifactPreview, closeArtifactPreview]);

  const openAgentDetail = useCallback((agent: AgentDetailTarget) => {
    setSelectedAgentDetail(agent);
    setActiveRightPanel('agent');
  }, []);

  const {
    tasks: channelTasks,
    isLoading: tasksLoading,
    error: tasksError,
    convertMessageToTask,
    refetch: refetchTasks,
  } = useTasks({ channel_id: channel.id });

  const [taskFilterAssignee, setTaskFilterAssignee] = useState('');
  const [taskFilterCreator, setTaskFilterCreator] = useState('');
  const [taskFilterNumber, setTaskFilterNumber] = useState('');

  useEffect(() => {
    if (searchParams.get('channel') !== channel.id) return;
    setChannelViewTab(searchParams.get('tab') === 'tasks' ? 'tasks' : 'messages');
    setTaskFilterAssignee(searchParams.get('assignee') || '');
    setTaskFilterCreator(searchParams.get('creator') || '');
    setTaskFilterNumber(searchParams.get('task') || '');
  }, [channel.id, searchParams]);

  const pushDashboardTaskUrl = useCallback(
    (next: { tab?: 'messages' | 'tasks'; assignee?: string; creator?: string; task?: string }) => {
      const params = new URLSearchParams(searchParams.toString());
      params.set('channel', channel.id);
      const tab = next.tab ?? channelViewTab;
      if (tab === 'tasks') params.set('tab', 'tasks');
      else params.delete('tab');
      const assignee = next.assignee ?? taskFilterAssignee;
      const creator = next.creator ?? taskFilterCreator;
      const task = next.task ?? taskFilterNumber;
      if (assignee) params.set('assignee', assignee);
      else params.delete('assignee');
      if (creator) params.set('creator', creator);
      else params.delete('creator');
      if (task) params.set('task', task);
      else params.delete('task');
      router.push(`/dashboard?${params.toString()}`);
    },
    [channel.id, channelViewTab, router, searchParams, taskFilterAssignee, taskFilterCreator, taskFilterNumber],
  );

  const filteredChannelTasks = useMemo(
    () => filterTaskTree(channelTasks, {
      assignee: taskFilterAssignee,
      creator: taskFilterCreator,
      taskNumber: taskFilterNumber,
    }),
    [channelTasks, taskFilterAssignee, taskFilterCreator, taskFilterNumber],
  );

  const taskAssigneeOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const task of channelTasks) {
      const id = task.claimer_id || task.assignee_id;
      const name = task.claimer_name || task.assignee_name || (id ? id.slice(0, 8) : '');
      if (id && !seen.has(id)) seen.set(id, { id, name });
    }
    return Array.from(seen.values());
  }, [channelTasks]);

  const taskCreatorOptions = useMemo(() => {
    const seen = new Map<string, { id: string; name: string }>();
    for (const task of channelTasks) {
      const name = task.creator_name || task.creator_id.slice(0, 8);
      if (!seen.has(task.creator_id)) seen.set(task.creator_id, { id: task.creator_id, name });
    }
    return Array.from(seen.values());
  }, [channelTasks]);

  const taskNumberOptions = useMemo(() => {
    return channelTasks
      .filter((task) => task.task_number != null)
      .sort((a, b) => (a.task_number ?? 0) - (b.task_number ?? 0))
      .map((task) => ({
        value: String(task.task_number),
        label: `#${task.task_number} ${task.title}`,
      }));
  }, [channelTasks]);

  const hasChannelTaskFilters = !!(taskFilterAssignee || taskFilterCreator || taskFilterNumber);
  const clearChannelTaskFilters = useCallback(() => {
    setTaskFilterAssignee('');
    setTaskFilterCreator('');
    setTaskFilterNumber('');
    pushDashboardTaskUrl({ tab: 'tasks', assignee: '', creator: '', task: '' });
  }, [pushDashboardTaskUrl]);

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

  // ---- Handle initialThreadMessageId: watch messages list for the target ----
  useEffect(() => {
    if (!initialThreadMessageId || !channel) return;

    // Check if the message is already in the loaded list
    const found = messages.find((m) => m.id === initialThreadMessageId);
    if (found) {
      setThreadMessage(found);
      setActiveRightPanel('thread');
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
        setActiveRightPanel('thread');
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
      setActiveRightPanel('thread');
      onThreadChange?.(task.message_id);
    },
    [channel.id, messages, onThreadChange],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
    onThreadChange?.(null);
    setActiveRightPanel(selectedAgentDetail ? 'agent' : null);
  }, [onThreadChange, selectedAgentDetail]);

  const handleAgentDetailClose = useCallback(() => {
    setSelectedAgentDetail(null);
    setActiveRightPanel(threadMessage ? 'thread' : null);
  }, [threadMessage]);

  // v1.5: Wrap onReply to also sync thread state to URL + pull latest task data
  const handleReply = useCallback(
    (message: Message) => {
      refetchTasks();
      setThreadTask(null);
      setThreadMessage(message);
      setActiveRightPanel('thread');
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

  const handleViewThreadInChannel = useCallback(() => {
    if (!threadMessage) return;
    setChannelViewTab('messages');
    setScrollToMessageId(threadMessage.id);
    setScrollMsgKey((k) => k + 1);
    router.push(`/dashboard?channel=${channel.id}&message=${threadMessage.id}`);
  }, [channel.id, router, threadMessage]);

  const handleViewThreadTask = useCallback(() => {
    const taskNumber = threadTask?.task_number ?? threadMessage?.task_number;
    if (!threadMessage || taskNumber == null) return;
    setChannelViewTab('tasks');
    setTaskFilterNumber(String(taskNumber));
    router.push(`/dashboard?channel=${channel.id}&tab=tasks&task=${taskNumber}&thread=${threadMessage.id}`);
  }, [channel.id, router, threadMessage, threadTask]);

  const existingAgentIds = agents.map((a) => a.member_id);
  const canAddAgents = !channel.name.startsWith('all-');

  // ---- Task quick-create handler (SOLO-128-F) ----

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

  const handleTaskActionComplete = useCallback((updated: Task) => {
    setThreadTask((prev) => (prev?.id === updated.id ? updated : prev));
    refetchTasks();
  }, [refetchTasks]);

  const handleGenerateArtifact = useCallback(async (task: Task) => {
    if (isGenerating) return;
    artifactReturnFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;

    try {
      const artifact = await generateArtifact(task.id);
      await refreshArtifactHistory(task.id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactGenerationInProgressError) return;
      if (error instanceof TaskArtifactStillPendingError) {
        const showedExisting = await showLatestPublishedArtifact(task.id);
        if (!showedExisting) {
          artifactReturnFocusRef.current = null;
          showToast('Artifact is still generating. Try again in a moment.', 'error');
        }
        return;
      }
      artifactReturnFocusRef.current = null;
      showToast('Could not generate artifact. Please try again.', 'error');
    }
  }, [generateArtifact, isGenerating, refreshArtifactHistory, showArtifactPreview, showLatestPublishedArtifact, showToast]);

  const handleFinalizeArtifact = useCallback(async () => {
    if (!artifactPreview || isGenerating) return;

    try {
      const artifact = await finalizeArtifact(artifactPreview.task_id);
      await refreshArtifactHistory(artifactPreview.task_id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactGenerationInProgressError) return;
      if (error instanceof TaskArtifactStillPendingError) {
        showToast('Final artifact is still generating. Try again in a moment.', 'error');
        return;
      }
      showToast('Could not finalize artifact. Please try again.', 'error');
    }
  }, [artifactPreview, finalizeArtifact, isGenerating, refreshArtifactHistory, showArtifactPreview, showToast]);

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
                onClick={() => {
                  setChannelViewTab('messages');
                  pushDashboardTaskUrl({ tab: 'messages', assignee: '', creator: '', task: '' });
                }}
                className={tabButtonClass(channelViewTab === 'messages')}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                {t('messages')}
              </button>
              <button
                type="button"
                onClick={() => {
                  setChannelViewTab('tasks');
                  pushDashboardTaskUrl({ tab: 'tasks' });
                }}
                className={tabButtonClass(channelViewTab === 'tasks')}
              >
                <SquareCheckBig className="h-3.5 w-3.5" />
                {t('tasks')}
              </button>
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
            {canAddAgents && (
              <Button
                type="button"
                onClick={() => setIsAddAgentModalOpen(true)}
                variant="outline"
                size="sm"
                className="h-8 w-8 p-0"
                aria-label={t('addAgentToChannel')}
                title={t('addAgentToChannel')}
              >
                <Plus className="h-4 w-4" />
              </Button>
            )}
            <Button
              type="button"
              onClick={() => setIsMemberPopoverOpen(true)}
              variant="outline"
              size="sm"
              className="h-8 w-8 p-0"
              aria-label={t('channelMembers')}
              title={t('channelMembers')}
            >
              <Users className="h-4 w-4" />
            </Button>
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
              onAsTask={handleAsTaskOpen}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              loadMoreError={loadMoreError}
              onLoadMore={loadMore}
              scrollToMessageId={scrollToMessageId}
              scrollKey={scrollMsgKey}
              members={members}
              onAgentClick={openAgentDetail}
            />
            <MessageInput
              onSend={async (content, _mentionedAgentIds, asTask, taskTitle, attachmentIds) => {
                if (asTask) {
                  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
                    router.push(`/dashboard?channel=${channel.id}&tab=tasks&task=${result.task_number}&thread=${result.id}`);
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
            <div className="border-b-2 border-black bg-brutal-cream px-4 py-3">
              <div className="flex items-center gap-2">
                <Select
                  value={taskFilterAssignee}
                  onChange={(value) => {
                    setTaskFilterAssignee(value);
                    pushDashboardTaskUrl({ tab: 'tasks', assignee: value });
                  }}
                  options={[
                    { value: '', label: t('allAssignees') },
                    ...taskAssigneeOptions.map((a) => ({ value: a.id, label: a.name })),
                  ]}
                  size="sm"
                  className="w-36"
                  aria-label={t('filterByClaimer')}
                />
                <Select
                  value={taskFilterCreator}
                  onChange={(value) => {
                    setTaskFilterCreator(value);
                    pushDashboardTaskUrl({ tab: 'tasks', creator: value });
                  }}
                  options={[
                    { value: '', label: t('allCreators') },
                    ...taskCreatorOptions.map((c) => ({ value: c.id, label: c.name })),
                  ]}
                  size="sm"
                  className="w-36"
                  aria-label={t('filterByCreator')}
                />
                <Select
                  value={taskFilterNumber}
                  onChange={(value) => {
                    setTaskFilterNumber(value);
                    pushDashboardTaskUrl({ tab: 'tasks', task: value });
                  }}
                  options={[
                    { value: '', label: 'All tasks' },
                    ...taskNumberOptions,
                  ]}
                  size="sm"
                  className="w-40"
                  aria-label="Filter by task number"
                />
                {hasChannelTaskFilters && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={clearChannelTaskFilters}
                    className="flex items-center gap-1"
                  >
                    <X className="h-3 w-3" />
                    {t('clearFilter')}
                  </Button>
                )}
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-4">
              <TaskBoard
                tasks={filteredChannelTasks}
                isLoading={tasksLoading}
                error={tasksError}
                onTaskClick={handleTaskClickInTab}
                onRefetch={refetchTasks}
                onActionComplete={handleTaskActionComplete}
                onGenerateArtifact={handleGenerateArtifact}
                isArtifactGenerating={isGenerating}
              />
            </div>
          </div>
        )}
      </div>

      {/* Thread panel (lazy-loaded, SOLO-63-F) — always mounted for smooth width transition */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-[width] duration-100 ease-linear border-l-2 border-transparent"
        style={{ width: rightPanelOpen ? threadPanelWidth : 0, borderLeftColor: rightPanelOpen ? 'var(--color-border, #000)' : 'transparent' }}
      >
        {/* Resize handle — only interactive when panel is open */}
        {rightPanelOpen && (
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
        {activeRightPanel === 'thread' && threadMessage && (
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
              onMarkRead={handleThreadMarkRead}
              onViewInChannel={handleViewThreadInChannel}
              onViewTask={handleViewThreadTask}
              onGenerateArtifact={threadTask ? () => handleGenerateArtifact(threadTask) : undefined}
              isArtifactGenerating={isGenerating}
              onAgentClick={openAgentDetail}
            />
          </Suspense>
        )}
        {activeRightPanel === 'agent' && selectedAgentDetail && (
          <RelationshipDetailPanel
            relationship={null}
            agent={selectedAgentDetail}
            onClose={handleAgentDetailClose}
            onUpdate={() => {}}
            onDelete={() => {}}
            onAgentDeleted={handleAgentDetailClose}
            embedded
          />
        )}
      </div>

      {artifactPreview && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="channel-artifact-preview-title"
          className="fixed inset-4 z-50 flex flex-col border-4 border-black bg-white shadow-brutal-xl"
        >
          <div className="flex items-center justify-between border-b-4 border-black px-4 py-2">
            <div id="channel-artifact-preview-title" className="font-heading text-sm font-black uppercase">{artifactPreview.title}</div>
            <div className="flex items-center gap-2">
              {artifactHistory
                .filter((artifact) => artifact.summary !== 'pending')
                .map((artifact) => (
                  <button
                    key={artifact.id}
                    type="button"
                    onClick={() => showArtifactPreview(artifact)}
                    className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
                  >
                    {artifact.html_path.endsWith('final.html') ? 'Final' : 'Latest'}
                  </button>
                ))}
              <a
                ref={artifactOpenLinkRef}
                href={artifactPreview.previewUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
              >
                Open
              </a>
              <button
                ref={artifactFinalizeButtonRef}
                type="button"
                onClick={handleFinalizeArtifact}
                disabled={isGenerating}
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm disabled:opacity-50"
              >
                Finalize
              </button>
              <button
                ref={artifactCloseButtonRef}
                type="button"
                onClick={closeArtifactPreview}
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
                aria-label="Close artifact preview"
              >
                Close
              </button>
            </div>
          </div>
          <iframe ref={artifactFrameRef} title={artifactPreview.title} src={artifactPreview.previewUrl} tabIndex={0} className="min-h-0 flex-1 bg-white" />
        </div>
      )}

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
            {canAddAgents && (
              <Button
                type="button"
                onClick={() => {
                  setIsMemberPopoverOpen(false);
                  setIsAddAgentModalOpen(true);
                }}
                variant="success"
                size="icon"
                className="h-7 w-7"
                aria-label={t('addAgentToChannel')}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
            )}
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
            onAgentClick={openAgentDetail}
            showHeader={false}
            canAddAgent={canAddAgents}
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
