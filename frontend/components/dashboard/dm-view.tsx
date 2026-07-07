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

import { useState, useMemo, useCallback, useEffect, useRef, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, RefreshCw, MessageSquare, Circle, SquareCheckBig } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useStreamingMessages } from '@/lib/hooks/use-streaming-messages';
import { TaskArtifactStillPendingError, useTaskArtifact } from '@/lib/hooks/use-task-artifact';
import { getTaskArtifactAction } from '@/lib/utils/task-artifact';
import { apiClient } from '@/lib/api-client';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { TaskBoard } from '@/components/tasks/task-board';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { Skeleton } from '@/components/ui/skeleton';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { tabButtonClass } from '@/components/ui/tab-bar';
import { useToast } from '@/components/ui/toast';
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);
import { t } from '@/lib/i18n';
import type { AgentDetailTarget, DMChannel, ChannelMember, Message, Task, TaskArtifact } from '@/lib/types';

type ArtifactPreview = TaskArtifact & { previewUrl: string };

interface DMViewProps {
  dm: DMChannel;
  messages: Message[];
  isLoading: boolean;
  error: string | null;
  sendMessage: (content: string, mentionedAgentIds?: string[], asTask?: boolean, attachmentIds?: string[]) => Promise<{ id: string; task_number?: number } | null>;
  retryMessage: (messageId: string, content: string) => Promise<void>;
  cancelMessage?: (messageId: string) => void;
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
  onConvertToTask?: (channelId: string, messageId: string, title?: string) => Promise<Task>;
  onTaskCreated?: () => void;
  /** v1.5: Called when thread opens/closes so the parent can sync to URL */
  onThreadChange?: (threadId: string | null) => void;
  /** v1.5: Initial thread message ID from URL — opens ThreadPanel on mount or on change */
  initialThreadMessageId?: string;
  /** Optional message ID to scroll to on mount or URL change */
  initialScrollToMessageId?: string;
}

// ---- Helpers ----

function getDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return t('unknown');
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
  onConvertToTask,
  onTaskCreated,
  onThreadChange,
  initialThreadMessageId,
  initialScrollToMessageId,
}: DMViewProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const name = getDisplayName(dm);
  const isAgent = isAgentDM(dm);
  const deleted = isAgentDeleted(dm);
  const [viewTab, setViewTab] = useState<'messages' | 'tasks'>(
    searchParams.get('tab') === 'tasks' ? 'tasks' : 'messages',
  );
  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | null>(null);
  const [artifactHistory, setArtifactHistory] = useState<TaskArtifact[]>([]);
  const [artifactReviewBusy, setArtifactReviewBusy] = useState(false);
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);
  const [selectedAgentDetail, setSelectedAgentDetail] = useState<AgentDetailTarget | null>(null);
  const [activeRightPanel, setActiveRightPanel] = useState<'thread' | 'agent' | null>(null);
  const rightPanelOpen = activeRightPanel !== null;
  const { showToast } = useToast();
  const { generateArtifact, regenerateArtifact, fetchArtifactHTML, listArtifacts, isGeneratingTask } = useTaskArtifact();
  const artifactOpenLinkRef = useRef<HTMLAnchorElement>(null);
  const artifactRegenerateButtonRef = useRef<HTMLButtonElement>(null);
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

  const showExistingArtifact = useCallback(async (taskId: string) => {
    const artifacts = await refreshArtifactHistory(taskId);
    const published = artifacts.find((artifact) => artifact.summary !== 'pending');
    if (published) {
      await showArtifactPreview(published);
      return true;
    }
    return false;
  }, [refreshArtifactHistory, showArtifactPreview]);

  const handleOpenArtifactReference = useCallback(async (ref: string) => {
    artifactReturnFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    try {
      const url = new URL(ref, window.location.origin);
      if (url.pathname.startsWith('/api/v1/artifacts/')) {
        const artifact = await apiClient.get<TaskArtifact>(`${url.pathname.replace(/\/meta$/, '')}/meta`);
        await showArtifactPreview(artifact);
        return;
      }

      const fileMatch = ref.match(/\/\.solo\/artifacts\/([^/\s]+)\/([^/\s]+\.html)/);
      if (fileMatch) {
        const [, taskId, filename] = fileMatch;
        const artifacts = await refreshArtifactHistory(taskId);
        const artifact = artifacts.find((item) => item.summary !== 'pending' && item.html_path.endsWith(`/${filename}`))
          ?? artifacts.find((item) => item.summary !== 'pending');
        if (artifact) {
          await showArtifactPreview(artifact);
          return;
        }
      }
    } catch {
      // Fall through to toast below.
    }
    artifactReturnFocusRef.current = null;
    showToast('Could not open artifact link.', 'error');
  }, [refreshArtifactHistory, showArtifactPreview, showToast]);

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
        const controls = ([artifactOpenLinkRef.current, artifactRegenerateButtonRef.current, artifactFrameRef.current, artifactCloseButtonRef.current] as Array<HTMLElement | null>).filter(
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

  useEffect(() => {
    if (searchParams.get('dm') !== dm.id) return;
    setViewTab(searchParams.get('tab') === 'tasks' ? 'tasks' : 'messages');
  }, [dm.id, searchParams]);

  const selectTab = useCallback(
    (tab: 'messages' | 'tasks') => {
      setViewTab(tab);
      const params = new URLSearchParams(searchParams.toString());
      params.set('dm', dm.id);
      if (tab === 'tasks') params.set('tab', 'tasks');
      else {
        params.delete('tab');
        params.delete('task');
      }
      router.push(`/dashboard?${params.toString()}`);
    },
    [dm.id, router, searchParams],
  );

  const openAgentDetail = useCallback((agent: AgentDetailTarget) => {
    setSelectedAgentDetail(agent);
    setActiveRightPanel('agent');
  }, []);

  // Handle initialScrollToMessageId: scroll to a specific message on mount or URL change.
  // Waits for isLoading to become false so the message DOM exists.
  const lastScrollTarget = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!initialScrollToMessageId || !dm || isLoading) return;
    if (lastScrollTarget.current === initialScrollToMessageId) return;
    lastScrollTarget.current = initialScrollToMessageId;
    setScrollToMessageId(initialScrollToMessageId);
    setScrollMsgKey((k) => k + 1);
  }, [initialScrollToMessageId, dm, isLoading]);

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
      setActiveRightPanel('thread');
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
        display_name: t('selfRef'),
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

  const handleTaskActionComplete = useCallback((updated: Task) => {
    setThreadTask((prev) => (prev?.id === updated.id ? updated : prev));
    refetchTasks?.();
  }, [refetchTasks]);

  useEffect(() => {
    if (!artifactPreview) return;

    const handleArtifactMessage = async (event: MessageEvent) => {
      if (event.source !== artifactFrameRef.current?.contentWindow) return;
      const data = event.data;
      if (!data || typeof data !== 'object' || data.type !== 'artifact.reviewAction') return;
      const taskId = typeof data.taskId === 'string' && data.taskId.trim() !== ''
        ? data.taskId.trim()
        : artifactPreview.task_id;
      if (artifactPreview.task_id && taskId && taskId !== artifactPreview.task_id) return;
      if (data.action !== 'accept' && data.action !== 'reject') return;
      if (artifactReviewBusy) return;

      const reason = typeof data.reason === 'string' ? data.reason.trim() : '';
      if (data.action === 'reject' && reason === '') {
        showToast('Reject comment is required.', 'error');
        return;
      }

      setArtifactReviewBusy(true);
      try {
        if (!taskId) throw new Error('missing task id');
        const path = `/api/v1/tasks/${taskId}/${data.action === 'accept' ? 'accept' : 'reject'}`;
        const updated = await apiClient.post<Task>(path, data.action === 'reject' ? { reason } : undefined);
        handleTaskActionComplete(updated);
        closeArtifactPreview();
        showToast(data.action === 'accept' ? 'Task accepted.' : 'Task rejected.', 'success');
      } catch {
        showToast(data.action === 'accept' ? 'Could not accept task.' : 'Could not reject task.', 'error');
      } finally {
        setArtifactReviewBusy(false);
      }
    };

    window.addEventListener('message', handleArtifactMessage);
    return () => window.removeEventListener('message', handleArtifactMessage);
  }, [artifactPreview, artifactReviewBusy, closeArtifactPreview, handleTaskActionComplete, showToast]);

  const handleGenerateArtifact = useCallback(async (task: Task) => {
    const action = getTaskArtifactAction(task, isGeneratingTask(task.id));
    if (action === 'hidden' || action === 'pending') return;
    artifactReturnFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;

    try {
      if (action === 'read') {
        await showExistingArtifact(task.id);
        return;
      }
      if (await showExistingArtifact(task.id)) return;
      const artifact = await generateArtifact(task.id);
      await refreshArtifactHistory(task.id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactStillPendingError) {
        const showedExisting = await showExistingArtifact(task.id);
        if (!showedExisting) {
          artifactReturnFocusRef.current = null;
          showToast('Artifact is still generating. Try again in a moment.', 'error');
        }
        return;
      }
      artifactReturnFocusRef.current = null;
      showToast('Could not generate artifact. Please try again.', 'error');
    }
  }, [generateArtifact, isGeneratingTask, refreshArtifactHistory, showArtifactPreview, showExistingArtifact, showToast]);

  const handleRegenerateArtifact = useCallback(async () => {
    if (!artifactPreview || isGeneratingTask(artifactPreview.task_id)) return;

    try {
      const artifact = await regenerateArtifact(artifactPreview.task_id);
      await refreshArtifactHistory(artifactPreview.task_id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactStillPendingError) {
        showToast('Artifact is still regenerating. Try again in a moment.', 'error');
        return;
      }
      showToast('Could not regenerate artifact. Please try again.', 'error');
    }
  }, [artifactPreview, regenerateArtifact, isGeneratingTask, refreshArtifactHistory, showArtifactPreview, showToast]);

  // ---- ThreadPanel handlers ----

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

  const handleMessageClick = useCallback(
    (message: Message) => {
      refetchTasks?.();
      const task = tasks?.find((t) => t.message_id === message.id || t.id === message.id);
      setThreadMessage({
        ...message,
        display_name: task?.creator_name || message.display_name,
      });
      setThreadTask(task ?? null);
      setActiveRightPanel('thread');
      onThreadChange?.(message.id);
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
      setActiveRightPanel('thread');
      onThreadChange?.(task.message_id);
    },
    [messages, refetchTasks, onThreadChange],
  );

  const handleAsTask = useCallback(
    async (message: Message) => {
      if (onAsTask) {
        await onAsTask(message);
        return;
      }
      if (!onConvertToTask) return;
      try {
        await onConvertToTask(dm.id, message.id);
      } catch {
        // handled silently
      }
    },
    [dm.id, onAsTask, onConvertToTask],
  );

  const handleViewThreadInDM = useCallback(() => {
    if (!threadMessage) return;
    setViewTab('messages');
    setScrollToMessageId(threadMessage.id);
    setScrollMsgKey((k) => k + 1);
    router.push(`/dashboard?dm=${dm.id}&message=${threadMessage.id}`);
  }, [dm.id, router, threadMessage]);

  const handleViewThreadTask = useCallback(() => {
    const taskNumber = threadTask?.task_number ?? threadMessage?.task_number;
    if (!threadMessage || taskNumber == null) return;
    setViewTab('tasks');
    router.push(`/dashboard?dm=${dm.id}&tab=tasks&task=${taskNumber}&thread=${threadMessage.id}`);
  }, [dm.id, router, threadMessage, threadTask]);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Header — full-width attached bar, same skeleton as Channel */}
        <div className="sidebar-collapse-offset flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            {/* Avatar */}
            <PixelAvatar
              agentId={dm.other_agent?.id ?? dm.other_user?.id ?? dm.id}
              size="md"
              avatarUrl={dm.other_agent?.avatar_url ?? null}
              onClick={dm.other_agent ? () => openAgentDetail({
                id: dm.other_agent!.id,
                name: dm.other_agent!.name,
                avatar_url: dm.other_agent!.avatar_url,
                is_active: dm.other_agent!.is_active,
              }) : undefined}
              ariaLabel={dm.other_agent ? t('viewAgentDetail', { name }) : undefined}
            />
            {/* Name + status */}
            <div className="flex items-center gap-2 min-w-0">
              <h2 className="font-bold text-foreground truncate">
                {name}
              </h2>
              {/* Online status dot */}
              <Circle className="h-2 w-2 fill-brutal-success text-brutal-success flex-shrink-0" />
            </div>
            <div className="mx-2 h-4 w-px bg-border flex-shrink-0" />
            {/* Tab bar (v1.2 Phase 2+3) */}
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => selectTab('messages')}
                className={tabButtonClass(viewTab === 'messages')}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                {t('messages')}
              </button>
              <button
                type="button"
                onClick={() => selectTab('tasks')}
                className={tabButtonClass(viewTab === 'tasks')}
              >
                <SquareCheckBig className="h-3.5 w-3.5" />
                {t('tasks')}
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
                  {t('systemMessageDM', { name })}
                </span>
              </div>
            </div>

            {/* Error state */}
            {error && !isLoading && (
              <div className="flex flex-1 items-center justify-center">
                <div className="text-center space-y-3">
                  <AlertCircle className="mx-auto h-8 w-8 text-brutal-danger" />
                  <p className="font-mono text-sm text-brutal-danger">{error}</p>
                  <button
                    type="button"
                    onClick={handleRetry}
                    className="btn-brutal btn-brutal-sm gap-2"
                  >
                    <RefreshCw className="h-4 w-4" />
                    {t('retry')}
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
                onAsTask={handleAsTask}
                onReply={handleMessageClick}
                hasMore={hasMore}
                isLoadingMore={isLoadingMore}
                loadMoreError={loadMoreError}
                onLoadMore={loadMore}
                scrollToMessageId={scrollToMessageId}
                scrollKey={scrollMsgKey}
                members={members}
                onOpenArtifactReference={handleOpenArtifactReference}
                onAgentClick={openAgentDetail}
              />
            )}

            {/* Input — hidden for deleted agents */}
            {!deleted && (
              <MessageInput
                onSend={async (content, mentionedAgentIds, asTask, _taskTitle, attachmentIds) => {
                  const result = await sendMessage(content, mentionedAgentIds, asTask, attachmentIds);
                  if (asTask && result?.task_number) {
                    onTaskCreated?.();
                    router.push(`/dashboard?dm=${dm.id}&tab=tasks&task=${result.task_number}&thread=${result.id}`);
                  }
                  return result;
                }}
                members={members}
                placeholder={t('sendMessageTo', { name })}
                showAsTaskToggle
              />
            )}
            {deleted && (
              <div className="border-t-2 border-black bg-brutal-muted/20 px-4 py-3 text-center">
                <span className="badge-brutal bg-brutal-muted text-black">
                  {t('deleted')}
                </span>
                <p className="mt-2 font-body text-xs text-muted-foreground">
                  {t('agentDeletedDM')}
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
                  <SquareCheckBig className="h-4 w-4" />
                  <span>
                    {t('dmTasks', { name })}
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
                onRefetch={refetchTasks ?? (() => {})}
                onActionComplete={handleTaskActionComplete}
                onGenerateArtifact={handleGenerateArtifact}
                isArtifactGenerating={(task) => isGeneratingTask(task.id)}
              />
            </div>
          </div>
        )}
      </div>

      {/* ThreadPanel — always mounted for smooth width transition */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-[width] duration-100 ease-linear border-l-2 border-transparent"
        style={{ width: rightPanelOpen ? threadPanelWidth : 0, borderLeftColor: rightPanelOpen ? '#000' : 'transparent' }}
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
              <div className="flex h-full w-[400px] items-center justify-center">
                <Skeleton className="h-8 w-32" />
              </div>
            }
          >
            <ThreadPanel
              parentMessage={threadMessage}
              task={threadTask ?? undefined}
              onClose={handleThreadClose}
              replyCount={threadMessage.reply_count ?? 0}
              onViewInChannel={handleViewThreadInDM}
              onViewTask={handleViewThreadTask}
              onOpenArtifactReference={handleOpenArtifactReference}
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
          aria-labelledby="dm-artifact-preview-title"
          className="fixed inset-4 z-50 flex flex-col border-4 border-black bg-white shadow-brutal-xl"
        >
          <div className="flex items-center justify-between border-b-4 border-black px-4 py-2">
            <div id="dm-artifact-preview-title" className="font-heading text-sm font-black uppercase">{artifactPreview.title}</div>
            <div className="flex items-center gap-2">
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
                ref={artifactRegenerateButtonRef}
                type="button"
                onClick={handleRegenerateArtifact}
                disabled={isGeneratingTask(artifactPreview.task_id)}
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm disabled:opacity-50"
              >
                Regenerate
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
    </div>
  );
}
