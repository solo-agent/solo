"use client";

import { useEffect, useState, useCallback, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import dynamic from "next/dynamic";
import { MessageSquare, AlertCircle, RefreshCw } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { useChannels } from "@/lib/hooks/use-channels";
import { useDM } from "@/lib/hooks/use-dm";
import { useDMTasks } from "@/lib/hooks/use-tasks";
import { Sidebar } from "@/components/dashboard/sidebar";
import { NavBar } from "@/components/ui/navbar";
import { CreateChannelModal } from "@/components/dashboard/create-channel-modal";
import { CreateDMModal } from "@/components/dashboard/create-dm-modal";
import { DeleteChannelDialog } from "@/components/dashboard/delete-channel-dialog";

import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

// SOLO-63-F: Lazy-load heavy view components
const ChannelView = dynamic(
  () => import("@/components/dashboard/channel-view").then((m) => ({ default: m.ChannelView })),
  {
    loading: () => (
      <div className="flex flex-1 items-center justify-center">
        <Spinner size="md" />
      </div>
    ),
  },
);

const DMView = dynamic(
  () => import("@/components/dashboard/dm-view").then((m) => ({ default: m.DMView })),
  {
    loading: () => (
      <div className="flex flex-1 items-center justify-center">
        <Spinner size="md" />
      </div>
    ),
  },
);
import { InboxView } from "@/components/inbox/inbox-view";
import type { Channel, DMChannel, CreateChannelInput, CreateDMInput, Message, Task, TaskStatus } from "@/lib/types";
import { useToast } from "@/components/ui/toast";

export default function DashboardPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-brutal-cream">
          <div className="flex flex-col items-center gap-3">
            <Spinner size="md" />
            <p className="font-mono text-sm text-muted-foreground">加载中...</p>
          </div>
        </div>
      }
    >
      <DashboardContent />
    </Suspense>
  );
}

function DashboardContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // ---- URL params drive the entire view ----
  const channelFromUrl = searchParams.get('channel');
  const dmFromUrl = searchParams.get('dm');
  const threadFromUrl = searchParams.get('thread');
  const messageFromUrl = searchParams.get('message');
  const inboxFromUrl = searchParams.has('inbox');

  // Derive view mode entirely from URL
  const viewMode = channelFromUrl ? 'channel' as const : dmFromUrl ? 'dm' as const : inboxFromUrl ? 'inbox' as const : null;
  const selectedChannelId = channelFromUrl;
  const selectedDmId = dmFromUrl;

  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const {
    channels,
    isLoading: channelsLoading,
    error: channelsError,
    createChannel,
    deleteChannel,
    refetch: refetchChannels,
  } = useChannels();

  const {
    dmChannels,
    isLoadingDMs,
    dmError,
    createOrGetDM,
    markAsRead,
    refetchDMs,
    selectDM,
    messages: dmMessages,
    isLoadingMessages: dmMessagesLoading,
    messagesError: dmMessagesError,
    sendMessage: dmSendMessage,
    retryMessage: dmRetryMessage,
    hasMore: dmHasMore,
    isLoadingMore: dmIsLoadingMore,
    loadMoreError: dmLoadMoreError,
    loadMore: dmLoadMore,
    cancelMessage: dmCancelMessage,
    editMessage: dmEditMessage,
    deleteMessage: dmDeleteMessage,
  } = useDM(dmFromUrl);

  const { showToast } = useToast();

  // ---- DM Tasks (v1.2 Phase 2+3) ----
  const {
    tasks: dmTasks,
    isLoading: dmTasksLoading,
    error: dmTasksError,
    updateTask: dmUpdateTask,
    claimTask: dmClaimTask,
    unclaimTask: dmUnclaimTask,
    convertMessageToTask: dmConvertMessageToTask,
    refetch: dmRefetchTasks,
  } = useDMTasks(selectedDmId);

  const handleDMTaskClaim = useCallback(
    async (task: Task) => {
      if (!selectedDmId) return;
      try {
        await dmClaimTask(selectedDmId, task.id);
        showToast(`已认领任务 #${task.task_number ?? '?'}`, 'success');
      } catch {
        // 409: silent
      }
    },
    [selectedDmId, dmClaimTask, showToast],
  );

  const handleDMTaskUnclaim = useCallback(
    async (task: Task) => {
      if (!selectedDmId) return;
      try {
        await dmUnclaimTask(selectedDmId, task.id);
        showToast(`已释放任务 #${task.task_number ?? '?'}`, 'info');
      } catch {
        // silent
      }
    },
    [selectedDmId, dmUnclaimTask, showToast],
  );

  const handleDMTaskStatusChange = useCallback(
    async (task: Task, newStatus: TaskStatus): Promise<Task | void> => {
      try {
        return await dmUpdateTask(task.id, { status: newStatus });
      } catch {
        // handled by hook
      }
    },
    [dmUpdateTask],
  );

  // ---- SOLO-island PR3: AgentViewPanel state lives at the dashboard
  // level so the AgentIsland (also mounted here) can summon it on click.
  // ChannelView consumes these as controlled props.
  const [agentViewVisible, setAgentViewVisible] = useState(false);
  const [agentViewWidth, setAgentViewWidth] = useState(320);
  const [agentViewFocusedAgentId, setAgentViewFocusedAgentId] = useState<string | null>(null);

  const handleInvokeAgent = useCallback((agentId: string) => {
    setAgentViewFocusedAgentId(agentId);
    setAgentViewVisible(true);
  }, []);

  // Toggling to false should also clear the focused-agent highlight so
  // a later reopen starts from a clean state. Forwarded to ChannelView as
  // the onAgentViewVisibleChange callback.
  const handleAgentViewVisibleChange = useCallback((visible: boolean) => {
    setAgentViewVisible(visible);
    if (!visible) {
      setAgentViewFocusedAgentId(null);
    }
  }, []);

  // ---- DM AsTask handler (SOLO-232-F) ----
  const handleDMAsTask = useCallback(
    async (message: Message) => {
      if (!selectedDmId) return;
      const title = message.content.slice(0, 200);
      try {
        const task = await dmConvertMessageToTask(selectedDmId, message.id, title);
        showToast(`已转为任务 #${task.task_number ?? '?'}`, 'success');
        dmRefetchTasks();
      } catch {
        showToast('转换任务失败，请稍后再试', 'error');
      }
    },
    [selectedDmId, dmConvertMessageToTask, dmRefetchTasks, showToast],
  );

  const handleDMConvertToTask = useCallback(
    async (dmId: string, messageId: string, title?: string): Promise<Task> => {
      return dmConvertMessageToTask(dmId, messageId, title || '');
    },
    [dmConvertMessageToTask],
  );

  // ---- Select DM (URL-driven) ----
  useEffect(() => {
    if (dmFromUrl) {
      selectDM(dmFromUrl);
      markAsRead(dmFromUrl);
    } else {
      selectDM(null);
    }
    // Only run when dmFromUrl changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dmFromUrl]);

  const selectedChannel: Channel | undefined = channels.find(
    (c) => c.id === selectedChannelId,
  );

  const selectedDM: DMChannel | undefined = dmChannels.find(
    (dm) => dm.id === selectedDmId,
  );

  // ---- URL-driven channel selection ----
  const handleSelectChannel = useCallback((channelId: string) => {
    router.push(`/dashboard?channel=${channelId}`);
  }, [router]);

  // ---- URL-driven DM selection ----
  const handleSelectDM = useCallback((dmId: string) => {
    router.push(`/dashboard?dm=${dmId}`);
    selectDM(dmId);
    markAsRead(dmId);
  }, [router, selectDM, markAsRead]);

  // ---- URL-driven Inbox selection ----
  const handleSelectInbox = useCallback(() => {
    router.push('/dashboard?inbox');
  }, [router]);

  // ---- Thread URL management ----
  const handleChannelThreadChange = useCallback(
    (threadId: string | null) => {
      if (threadId && channelFromUrl) {
        router.push(`/dashboard?channel=${channelFromUrl}&thread=${threadId}`);
      } else if (channelFromUrl) {
        router.push(`/dashboard?channel=${channelFromUrl}`);
      }
    },
    [router, channelFromUrl],
  );

  const handleDMThreadChange = useCallback(
    (threadId: string | null) => {
      if (threadId && dmFromUrl) {
        router.push(`/dashboard?dm=${dmFromUrl}&thread=${threadId}`);
      } else if (dmFromUrl) {
        router.push(`/dashboard?dm=${dmFromUrl}`);
      }
    },
    [router, dmFromUrl],
  );

  // ---- create channel modal ----
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  const handleCreateChannel = useCallback(
    async (input: CreateChannelInput) => {
      const channel = await createChannel(input);
      handleSelectChannel(channel.id);
    },
    [createChannel, handleSelectChannel],
  );

  // ---- create DM modal ----
  const [isCreateDMModalOpen, setIsCreateDMModalOpen] = useState(false);

  const handleCreateDM = useCallback(
    async (input: CreateDMInput): Promise<string> => {
      const dm = await createOrGetDM(input);
      handleSelectDM(dm.id);
      return dm.id;
    },
    [createOrGetDM, handleSelectDM],
  );

  // ---- delete channel dialog ----
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const deleteTarget = channels.find((c) => c.id === deleteTargetId);

  const handleDeleteChannel = useCallback(
    async (channelId: string) => {
      if (viewMode === 'channel' && selectedChannelId === channelId) {
        // If the deleted channel is currently selected, navigate away
        const remaining = channels.filter((c) => c.id !== channelId);
        if (remaining.length > 0) {
          handleSelectChannel(remaining[0].id);
        } else {
          router.push('/dashboard');
        }
      }
      await deleteChannel(channelId);
    },
    [viewMode, selectedChannelId, channels, handleSelectChannel, deleteChannel, router],
  );

  // ---- auth guard ----
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push("/auth/login");
    }
  }, [authLoading, isAuthenticated, router]);

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-muted/20">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  // Determine which view to show
  const renderMainContent = () => {
    if (viewMode === 'channel') {
      // Channel is selected via URL but may not be loaded yet
      if (!selectedChannel) {
        return (
          <div className="flex flex-1 items-center justify-center">
            <Spinner size="md" />
          </div>
        );
      }
      return (
        <ChannelView
          key={`chan-${selectedChannel.id}`}
          channel={selectedChannel}
          initialThreadMessageId={threadFromUrl ?? undefined}
          initialScrollToMessageId={messageFromUrl ?? undefined}
          onThreadChange={handleChannelThreadChange}
          agentViewVisible={agentViewVisible}
          onAgentViewVisibleChange={handleAgentViewVisibleChange}
          agentViewWidth={agentViewWidth}
          onAgentViewWidthChange={setAgentViewWidth}
          agentViewFocusedAgentId={agentViewFocusedAgentId}
        />
      );
    }

    if (viewMode === 'dm') {
      if (!selectedDM) {
        return (
          <div className="flex flex-1 items-center justify-center">
            <Spinner size="md" />
          </div>
        );
      }
      return (
        <DMView
          key={`dm-${selectedDM.id}`}
          dm={selectedDM}
          initialThreadMessageId={threadFromUrl ?? undefined}
          initialScrollToMessageId={messageFromUrl ?? undefined}
          messages={dmMessages}
          isLoading={dmMessagesLoading}
          error={dmMessagesError}
          sendMessage={dmSendMessage}
          retryMessage={dmRetryMessage}
          hasMore={dmHasMore}
          isLoadingMore={dmIsLoadingMore}
          loadMoreError={dmLoadMoreError}
          loadMore={dmLoadMore}
          cancelMessage={dmCancelMessage}
          editMessage={dmEditMessage}
          deleteMessage={dmDeleteMessage}
          onAsTask={handleDMAsTask}
          refetch={refetchDMs}
          tasks={dmTasks}
          tasksLoading={dmTasksLoading}
          tasksError={dmTasksError}
          refetchTasks={dmRefetchTasks}
          onTaskStatusChange={handleDMTaskStatusChange}
          onClaimTask={handleDMTaskClaim}
          onUnclaimTask={handleDMTaskUnclaim}
          onConvertToTask={handleDMConvertToTask}
          onTaskCreated={dmRefetchTasks}
          onThreadChange={handleDMThreadChange}
        />
      );
    }

    if (viewMode === 'inbox') {
      return <InboxView />;
    }

    // Empty/no-selection state (default: no URL params)
    return (
      <div className="flex flex-1 items-center justify-center bg-muted/5">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
            <MessageSquare className="h-8 w-8 text-white" />
          </div>
          <h2 className="text-xl font-semibold text-foreground">
            {channelsLoading || isLoadingDMs
              ? "加载中..."
              : channelsError || dmError
                ? "加载失败"
                : channels.length === 0 && dmChannels.length === 0
                  ? "还没有频道和私信"
                  : "选择一个频道或私信"}
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {channelsError || dmError
              ? channelsError || dmError
              : channels.length === 0 && dmChannels.length === 0
                ? "创建一个频道，开始与团队成员协作"
                : "从左侧选择一个频道或私信开始交流"}
          </p>
          {(channelsError || dmError) && (
            <Button
              variant="outline"
              size="sm"
              className="mt-4"
              onClick={() => {
                refetchChannels();
                refetchDMs();
              }}
            >
              <RefreshCw className="mr-2 h-4 w-4" />
              重试
            </Button>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <NavBar />
      <Sidebar
        routeTitle="Chat"
        channels={channels}
        isLoading={channelsLoading}
        selectedChannelId={selectedChannelId}
        onSelectChannel={handleSelectChannel}
        onCreateChannel={() => setIsCreateModalOpen(true)}
        onDeleteChannel={(id) => setDeleteTargetId(id)}
        dms={dmChannels}
        dmsLoading={isLoadingDMs}
        selectedDmId={selectedDmId}
        onSelectDM={handleSelectDM}
        onCreateDM={() => setIsCreateDMModalOpen(true)}
        inboxSelected={inboxFromUrl}
        onSelectInbox={handleSelectInbox}
        activeChannelId={selectedChannelId ?? selectedDmId ?? null}
        onInvokeAgent={handleInvokeAgent}
      />

      {/* Main content area */}
      <main className="flex flex-1 flex-col overflow-hidden">
        {renderMainContent()}
      </main>

      {/* Modals */}
      <CreateChannelModal
        open={isCreateModalOpen}
        onOpenChange={setIsCreateModalOpen}
        onSubmit={handleCreateChannel}
      />

      <CreateDMModal
        open={isCreateDMModalOpen}
        onOpenChange={setIsCreateDMModalOpen}
        onCreateDM={handleCreateDM}
        dms={dmChannels}
      />

      {deleteTarget && (
        <DeleteChannelDialog
          open={!!deleteTargetId}
          onOpenChange={() => setDeleteTargetId(null)}
          channelName={deleteTarget.name}
          onConfirm={() => handleDeleteChannel(deleteTarget.id)}
        />
      )}


    </div>
  );
}
