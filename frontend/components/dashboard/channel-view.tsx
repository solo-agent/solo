// ============================================================================
// ChannelView — main message area + right-side member list with Agent support
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback, useMemo, lazy, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Users, Loader2, SquareCheckBig, Plus, Network, Maximize2, Minimize2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMessages } from '@/lib/hooks/use-messages';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
import { useWebSocket } from '@/lib/ws-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { TaskArtifactStillPendingError, useTaskArtifact } from '@/lib/hooks/use-task-artifact';
import { getTaskArtifactAction } from '@/lib/utils/task-artifact';
import { apiClient } from '@/lib/api-client';
import { displayAgentErrorReason } from '@/lib/agent-activity';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { MemberList } from './member-list';
import { AddAgentModal } from './add-agent-modal';
import { TaskBoard } from '@/components/tasks/task-board';
import { RelationshipWorkspace } from '@/components/relationships/relationship-workspace';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { Button, PanelToggleIcon, panelToggleButtonClass } from '@/components/ui/button';
import { tabButtonClass } from '@/components/ui/tab-bar';
import { buildDashboardHref, parseDashboardParams, type DashboardPanel, type DashboardView } from '@/lib/dashboard-url';
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
import type { AgentDetailTarget, AgentRelationship, Channel, Message, Task, TaskArtifact, TaskStatus } from '@/lib/types';

type ArtifactPreview = TaskArtifact & { previewUrl: string };
type WorkspaceDetail = { relationship: AgentRelationship | null; agent: AgentDetailTarget | null };
const TEAM_TASK_VISIBLE_STATUSES = new Set<TaskStatus>(['in_progress', 'in_review']);

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
  onChannelCreated?: () => void;
  /** Whether the right-side AgentViewPanel is visible. */
  agentViewVisible?: boolean;
  /** Toggle the AgentViewPanel. Called with `true` to open, `false` to close. */
  onAgentViewVisibleChange?: (visible: boolean) => void;
  /** Width of the AgentViewPanel, controlled by parent so it can outlive unmounts. */
  agentViewWidth?: number;
  /** Called when the user drags the panel's resize handle. Parent should persist the new width. */
  onAgentViewWidthChange?: (width: number) => void;
  /** When set, the panel scrolls/highlights this agent. */
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
    refetch: refetchMembers,
  } = useChannelMembers(channel.id);

  // Keep a ref to the latest agents list for use in WS event handler closures
  const agentsRef = useRef(agents);
  agentsRef.current = agents;

  const dashboardState = useMemo(() => parseDashboardParams(searchParams), [searchParams]);
  const workspaceView = dashboardState.view;
  const mainPanel = dashboardState.panel;
  const pushDashboardState = useCallback(
    (patch: Partial<{
      view: DashboardView;
      panel: DashboardPanel;
      taskId: string | null;
      threadId: string | null;
      messageId: string | null;
      agentId: string | null;
      relationshipId: string | null;
    }>) => {
      router.push(buildDashboardHref(channel.id, { ...dashboardState, ...patch }));
    },
    [channel.id, dashboardState, router],
  );

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [isAddAgentModalOpen, setIsAddAgentModalOpen] = useState(false);
  const [workspaceDetail, setWorkspaceDetail] = useState<WorkspaceDetail | null>(null);
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | null>(null);
  const [artifactHistory, setArtifactHistory] = useState<TaskArtifact[]>([]);
  const [artifactReviewBusy, setArtifactReviewBusy] = useState(false);
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);

  // ---- Channel search state (SOLO-237-F) ----
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);

  // ---- Member popover state ----
  const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);
  const [isWorkspaceCollapsed, setIsWorkspaceCollapsed] = useState(false);
  const [isWorkspaceFullscreen, setIsWorkspaceFullscreen] = useState(false);

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

  const openWorkspaceDetail = useCallback(
    (detail: WorkspaceDetail) => {
      setWorkspaceDetail(detail);
      if (detail.agent) {
        pushDashboardState({
          panel: 'agent',
          agentId: detail.agent.id,
          relationshipId: null,
          threadId: null,
          messageId: null,
        });
      } else if (detail.relationship) {
        pushDashboardState({
          panel: 'relationship',
          relationshipId: detail.relationship.id,
          agentId: null,
          threadId: null,
          messageId: null,
        });
      }
    },
    [pushDashboardState],
  );

  const openAgentDetail = useCallback((agent: AgentDetailTarget) => {
    openWorkspaceDetail({ relationship: null, agent });
  }, [openWorkspaceDetail]);

  const {
    tasks: channelTasks,
    isLoading: tasksLoading,
    error: tasksError,
    convertMessageToTask,
    refetch: refetchTasks,
  } = useTasks({ channel_id: channel.id });

  const channelTeam = useMemo(() => ({
    agents: agents.map((agent) => ({
      id: agent.member_id,
      name: agent.display_name || agent.member_id,
      status: agent.status,
    })),
  }), [agents]);

  const channelAgentMap = useMemo(() => {
    return new Map(channelTeam.agents.map((agent) => [agent.id, agent]));
  }, [channelTeam]);

  const latestTaskByAgent = useMemo(() => {
    const result: Record<string, {
      id: string;
      taskNumber?: number;
      title: string;
      status: TaskStatus;
      artifactStatus?: 'none' | 'pending' | 'available';
      createdAt?: string;
      updatedAt?: string;
    }> = {};

    for (const task of channelTasks) {
      if (!TEAM_TASK_VISIBLE_STATUSES.has(task.status)) continue;
      const agentId = task.claimer_id || task.assignee_id;
      if (!agentId || !channelAgentMap.has(agentId)) continue;

      const taskCreatedAt = Date.parse(task.created_at || task.updated_at || '');
      const current = result[agentId];
      const currentCreatedAt = current ? Date.parse(current.createdAt || current.updatedAt || '') : Number.NEGATIVE_INFINITY;
      if (current && taskCreatedAt < currentCreatedAt) continue;

      result[agentId] = {
        id: task.id,
        taskNumber: task.task_number,
        title: task.title,
        status: task.status,
        artifactStatus: task.artifact_status,
        createdAt: task.created_at,
        updatedAt: task.updated_at,
      };
    }

    return result;
  }, [channelAgentMap, channelTasks]);

  const taskBoardTasks = useMemo(
    () => filterTaskTree(channelTasks, {
      taskId: workspaceView === 'task' && mainPanel === 'thread'
        ? dashboardState.taskId ?? threadTask?.id ?? null
        : null,
    }),
    [channelTasks, dashboardState.taskId, mainPanel, threadTask?.id, workspaceView],
  );

  const loadRelationships = useCallback(async () => {
    try {
      const rels = await apiClient.get<AgentRelationship[]>('/api/v1/agent-relationships');
      setRelationships(rels);
    } catch {
      setRelationships([]);
    }
  }, []);

  useEffect(() => {
    if (workspaceView === 'team' || mainPanel === 'relationship') {
      void loadRelationships();
    }
  }, [loadRelationships, mainPanel, workspaceView]);

  useEffect(() => {
    if (mainPanel === 'agent' && dashboardState.agentId) {
      const agent = channelAgentMap.get(dashboardState.agentId);
      setWorkspaceDetail({
        relationship: null,
        agent: {
          id: dashboardState.agentId,
          name: agent?.name ?? dashboardState.agentId.slice(0, 8),
          is_active: agent?.status === 'online' || agent?.status === 'thinking' || agent?.status === 'typing',
        },
      });
      return;
    }

    if (mainPanel === 'relationship' && dashboardState.relationshipId) {
      const rel = relationships.find((item) => item.id === dashboardState.relationshipId);
      if (!rel) return;
      const from = channelAgentMap.get(rel.from_agent_id);
      const to = channelAgentMap.get(rel.to_agent_id);
      setWorkspaceDetail({
        relationship: {
          ...rel,
          from_agent_name: from?.name ?? rel.from_agent_name,
          from_agent_active: from ? from.status !== 'offline' : rel.from_agent_active,
          to_agent_name: to?.name ?? rel.to_agent_name,
          to_agent_active: to ? to.status !== 'offline' : rel.to_agent_active,
        },
        agent: null,
      });
      return;
    }

    if (mainPanel !== 'agent' && mainPanel !== 'relationship') {
      setWorkspaceDetail(null);
    }
  }, [channelAgentMap, dashboardState.agentId, dashboardState.relationshipId, mainPanel, relationships]);

  // Refetch tasks when switching to tasks tab
  useEffect(() => {
    if (workspaceView === 'task') refetchTasks();
  }, [workspaceView]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- Agent member status tracking (SOLO-47-F) ----
  // The member list still updates dots to reflect thinking/typing/online so
  // the avatar stays in sync, but with a simple channel-local model.
  //
  // PR-fix: the previous 5s inactivity heuristic was the same class of
  // bug as the 3s heuristic in useAgentChunks (fixed in PR0). It would
  // prematurely mark an agent as online during long-running tool calls.
  // Now we rely on agent.run.finished as the authoritative terminal signal and
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
   * net for paths that don't emit agent.run.finished (e.g. the legacy LLM
   * provider fallback). Primary terminal signal remains agent.run.finished.
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
        // Fallback only — agent.run.finished will cancel and immediately revert.
        scheduleMemberStatusRevert(event.agent_id, 30_000);
      }

      if (event.type === 'agent.error' && event.channel_id === channel.id && !event.thread_id) {
        showToast(t('agentRunFailedToast', {
          name: event.agent_name ?? t('agent'),
          reason: displayAgentErrorReason(event.error, event.detail),
        }), 'error');
      }

      // agent.run.finished is the authoritative terminal signal. Clear any
      // fallback timer and snap status back to online immediately.
      if (event.type === 'agent.run.finished' && event.channel_id === channel.id && event.agent_id) {
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
  }, [channel.id, onEvent, showToast, updateMemberStatus]);

  // ---- Handle thread deep links: watch messages list for the target ----
  useEffect(() => {
    const targetThreadId = dashboardState.threadId || initialThreadMessageId;
    if (!targetThreadId || mainPanel !== 'thread') return;

    // Check if the message is already in the loaded list
    const found = messages.find((m) => m.id === targetThreadId);
    if (found) {
      setThreadMessage(found);
      // Try to find the associated task for the metadata bar
      const task = channelTasks.find((t) => t.message_id === targetThreadId);
      setThreadTask(task ?? null);
      return;
    }
    setThreadTask(null);
    setThreadMessage({
      id: targetThreadId,
      channel_id: channel.id,
      user_id: '',
      display_name: channel.name,
      content: '',
      created_at: new Date().toISOString(),
      status: 'sent',
      sender_type: 'user',
    });
    // If not found yet, it will be caught when messages load (via the next effect)
  }, [channel.id, channel.name, dashboardState.threadId, initialThreadMessageId, mainPanel, messages, channelTasks]);

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
    if (!task) {
      setThreadTask(null);
      return;
    }
    setThreadTask((prev) => {
      // Only update if actually changed to avoid re-render loops
      if (!prev || prev.status !== task.status || prev.claimer_id !== task.claimer_id) {
        return task;
      }
      return prev;
    });
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
        pushDashboardState({
          view: workspaceView,
          panel: 'thread',
          taskId: task.id,
          threadId: task.message_id,
          agentId: null,
          relationshipId: null,
          messageId: null,
        });
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
      pushDashboardState({
        view: workspaceView,
        panel: 'thread',
        taskId: task.id,
        threadId: task.message_id,
        agentId: null,
        relationshipId: null,
        messageId: null,
      });
      onThreadChange?.(task.message_id);
    },
    [channel.id, messages, onThreadChange, pushDashboardState, workspaceView],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
    onThreadChange?.(null);
    pushDashboardState({
      panel: 'conversation',
      threadId: null,
      agentId: null,
      relationshipId: null,
      messageId: null,
    });
  }, [onThreadChange, pushDashboardState]);

  const handleAgentDetailClose = useCallback(() => {
    setWorkspaceDetail(null);
    pushDashboardState({
      panel: 'conversation',
      agentId: null,
      relationshipId: null,
      threadId: null,
      messageId: null,
    });
  }, [pushDashboardState]);

  // v1.5: Wrap onReply to also sync thread state to URL + pull latest task data
  const handleReply = useCallback(
    (message: Message) => {
      refetchTasks();
      setThreadTask(null);
      setThreadMessage(message);
      pushDashboardState({
        panel: 'thread',
        taskId: null,
        threadId: message.id,
        agentId: null,
        relationshipId: null,
        messageId: null,
      });
      onThreadChange?.(message.id);
    },
    [refetchTasks, pushDashboardState, onThreadChange],
  );

  // P25-08-F: Called by ThreadPanel after successfully marking thread as read
  const handleThreadMarkRead = useCallback(() => {
    if (threadMessage) {
      markMessageThreadRead(threadMessage.id);
    }
  }, [threadMessage, markMessageThreadRead]);

  const handleViewThreadInChannel = useCallback(() => {
    if (!threadMessage) return;
    setScrollToMessageId(threadMessage.id);
    setScrollMsgKey((k) => k + 1);
    pushDashboardState({
      panel: 'conversation',
      threadId: null,
      agentId: null,
      relationshipId: null,
      messageId: threadMessage.id,
    });
  }, [pushDashboardState, threadMessage]);

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

  const handleTeamTaskOpen = useCallback((taskId: string) => {
    const task = channelTasks.find((item) => item.id === taskId);
    if (task) handleTaskClickInTab(task);
  }, [channelTasks, handleTaskClickInTab]);

  const handleTeamTaskArtifactOpen = useCallback((taskId: string) => {
    const task = channelTasks.find((item) => item.id === taskId);
    if (task) void handleGenerateArtifact(task);
  }, [channelTasks, handleGenerateArtifact]);

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

  return (
    <div className={cn(
      'relative flex flex-1 overflow-hidden',
      isWorkspaceCollapsed && '[&_.sidebar-collapse-offset]:pr-14',
    )}>
      {/* Left: conversation/thread/detail */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden border-r-2 border-black">
        {mainPanel === 'thread' && threadMessage ? (
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
              onOpenArtifactReference={handleOpenArtifactReference}
              onAgentClick={openAgentDetail}
            />
          </Suspense>
        ) : (mainPanel === 'agent' || mainPanel === 'relationship') && workspaceDetail ? (
          <RelationshipDetailPanel
            key={workspaceDetail.agent ? `agent-${workspaceDetail.agent.id}` : `relationship-${workspaceDetail.relationship?.id}`}
            relationship={workspaceDetail.relationship}
            agent={workspaceDetail.agent}
            onClose={handleAgentDetailClose}
            onUpdate={() => { void loadRelationships(); }}
            onDelete={() => {
              handleAgentDetailClose();
              void loadRelationships();
            }}
            onAgentDeleted={() => {
              handleAgentDetailClose();
              void refetchMembers();
              void loadRelationships();
            }}
            embedded
          />
        ) : (
          <>
            <div className="sidebar-collapse-offset flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                  {t('taskChannel')}
                </span>
                <h2 className="truncate font-heading text-lg font-bold text-foreground">{channel.name}</h2>
              </div>
            </div>
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
              onOpenArtifactReference={handleOpenArtifactReference}
              onAgentClick={openAgentDetail}
            />
            <MessageInput
              onSend={async (content, _mentionedAgentIds, asTask, taskTitle, attachmentIds) => {
                if (asTask) {
                  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
                    pushDashboardState({
                      panel: 'thread',
                      threadId: result.id,
                      taskId: null,
                      agentId: null,
                      relationshipId: null,
                      messageId: null,
                    });
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
          </>
        )}
      </div>

      {isWorkspaceCollapsed && (
        <button
          type="button"
          onClick={() => setIsWorkspaceCollapsed(false)}
          className={panelToggleButtonClass(false, 'absolute right-3 top-3.5 z-30')}
          aria-label={t('workspaceExpandPanel')}
          title={t('workspaceExpandPanel')}
        >
          <PanelToggleIcon side="right" />
        </button>
      )}

      {/* Right: channel workspace */}
      {!isWorkspaceCollapsed && (
      <div className={cn(
        'flex min-w-0 flex-1 flex-col overflow-hidden bg-brutal-cream',
        isWorkspaceFullscreen && 'fixed inset-0 z-[80] h-screen border-4 border-black',
      )}>
        <div className="flex h-14 flex-shrink-0 items-center justify-between gap-3 border-b-2 border-black px-4">
          <div className="flex min-w-0 items-center gap-3">
            <button
              type="button"
              onClick={() => pushDashboardState({ view: 'team' })}
              className={tabButtonClass(workspaceView === 'team')}
            >
              <Network className="h-3.5 w-3.5" />
              {t('navTeams')}
            </button>
            <button
              type="button"
              onClick={() => pushDashboardState({ view: 'task' })}
              className={tabButtonClass(workspaceView === 'task')}
            >
              <SquareCheckBig className="h-3.5 w-3.5" />
              {t('navTasks')}
            </button>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              onClick={() => setIsWorkspaceFullscreen((fullscreen) => !fullscreen)}
              className={panelToggleButtonClass(isWorkspaceFullscreen)}
              aria-label={isWorkspaceFullscreen ? t('workspaceExitFullscreenPanel') : t('workspaceFullscreenPanel')}
              title={isWorkspaceFullscreen ? t('workspaceExitFullscreenPanel') : t('workspaceFullscreenPanel')}
            >
              {isWorkspaceFullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
            </button>
            <button
              type="button"
              onClick={() => {
                setIsWorkspaceFullscreen(false);
                setIsWorkspaceCollapsed(true);
              }}
              className={panelToggleButtonClass(true)}
              aria-label={t('workspaceCollapsePanel')}
              title={t('workspaceCollapsePanel')}
            >
              <PanelToggleIcon side="right" />
            </button>
          </div>
        </div>

        {workspaceView === 'team' && (
          <RelationshipWorkspace
            embedded
            title={t('navTeams')}
            channelFilterId={channel.id}
            channelTeam={channelTeam}
            agentTasks={latestTaskByAgent}
            onOpenTask={handleTeamTaskOpen}
            onOpenTaskArtifact={handleTeamTaskArtifactOpen}
            onChannelTeamRefresh={() => {
              void refetchMembers();
              void loadRelationships();
            }}
            onDetailOpen={openWorkspaceDetail}
            onDetailClose={handleAgentDetailClose}
            embeddedActions={
              <>
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
              </>
            }
          />
        )}

        {workspaceView === 'task' && (
          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="flex-1 overflow-y-auto px-4 py-4">
              <TaskBoard
                tasks={taskBoardTasks}
                isLoading={tasksLoading}
                error={tasksError}
                onTaskClick={handleTaskClickInTab}
                onRefetch={refetchTasks}
                onActionComplete={handleTaskActionComplete}
                onGenerateArtifact={handleGenerateArtifact}
                isArtifactGenerating={(task) => isGeneratingTask(task.id)}
                selectedTaskId={dashboardState.taskId ?? threadTask?.id ?? null}
              />
            </div>
          </div>
        )}
      </div>
      )}

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

      {/* Add Agent to Channel modal */}
      <AddAgentModal
        open={isAddAgentModalOpen}
        onOpenChange={setIsAddAgentModalOpen}
        existingAgentIds={existingAgentIds}
        onAdd={addAgentToChannel}
        onChanged={() => {
          void refetchMembers();
          void loadRelationships();
        }}
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
