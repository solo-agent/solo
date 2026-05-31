"use client";

import { useEffect, useState, useCallback, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import dynamic from "next/dynamic";
import { Hash, MessageSquare, AlertCircle, RefreshCw } from "lucide-react";
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

// SOLO-63-F: Lazy-load heavy view components
const ChannelView = dynamic(
  () => import("@/components/dashboard/channel-view").then((m) => ({ default: m.ChannelView })),
  {
    loading: () => (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    ),
  },
);

const DMView = dynamic(
  () => import("@/components/dashboard/dm-view").then((m) => ({ default: m.DMView })),
  {
    loading: () => (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    ),
  },
);
import type { Channel, DMChannel, CreateChannelInput, CreateDMInput, Message, Task, TaskStatus } from "@/lib/types";
import { useToast } from "@/components/ui/toast";

export default function DashboardPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-brutal-cream">
          <div className="flex flex-col items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
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
  const channelFromUrl = searchParams.get('channel');
  const messageFromUrl = searchParams.get('message');

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
    activeDMId,
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
  } = useDM();

  const { showToast } = useToast();

  // --- View mode: 'channel' | 'dm' | null ---
  const [viewMode, setViewMode] = useState<'channel' | 'dm' | null>(null);
  const [selectedChannelId, setSelectedChannelId] = useState<string | null>(null);
  const [selectedDmId, setSelectedDmId] = useState<string | null>(null);

  // ---- DM Tasks (v1.2 Phase 2+3) ----
  const {
    tasks: dmTasks,
    isLoading: dmTasksLoading,
    error: dmTasksError,
    createTask: dmCreateTask,
    updateTask: dmUpdateTask,
    claimTask: dmClaimTask,
    unclaimTask: dmUnclaimTask,
    convertMessageToTask: dmConvertMessageToTask,
    refetch: dmRefetchTasks,
  } = useDMTasks(selectedDmId);

  // ---- DM Task handlers ----
  const handleDMCreateTask = useCallback(
    async (title: string) => {
      if (!selectedDmId) return;
      try {
        const task = await dmCreateTask({ dm_id: selectedDmId, title, channel_id: '' });
        showToast(`已创建任务 #${task.task_number ?? '?'}`, 'success');
      } catch {
        showToast('创建任务失败，请稍后再试', 'error');
      }
    },
    [selectedDmId, dmCreateTask, showToast],
  );

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
    async (task: Task, newStatus: TaskStatus) => {
      try {
        await dmUpdateTask(task.id, { status: newStatus });
      } catch {
        // handled by hook
      }
    },
    [dmUpdateTask],
  );

  // ---- DM AsTask handler (SOLO-232-F) ----
  const handleDMAsTask = useCallback(
    async (message: Message) => {
      if (!selectedDmId) return;
      const title = message.content.slice(0, 200);
      try {
        const task = await dmConvertMessageToTask(selectedDmId, message.id, title);
        showToast(`已转为任务 #${task.task_number ?? '?'}`, 'success');
        // Refetch DM tasks to show the new task in the board
        dmRefetchTasks();
      } catch {
        showToast('转换任务失败，请稍后再试', 'error');
      }
    },
    [selectedDmId, dmConvertMessageToTask, dmRefetchTasks, showToast],
  );

  // ---- URL param: auto-select channel ----
  const [urlChannelLoaded, setUrlChannelLoaded] = useState(false);

  // When channel param is present in URL, select it
  useEffect(() => {
    if (channelFromUrl && channels.length > 0 && !urlChannelLoaded) {
      const found = channels.find((c) => c.id === channelFromUrl);
      if (found) {
        setViewMode('channel');
        setSelectedChannelId(channelFromUrl);
        setUrlChannelLoaded(true);
      }
    }
  }, [channelFromUrl, channels, urlChannelLoaded]);

  // Auto-select first channel when channels load (if no URL param)
  useEffect(() => {
    if (!channelsLoading && channels.length > 0 && viewMode === null) {
      // If URL has channel param, wait for it to be handled
      if (channelFromUrl && !urlChannelLoaded) return;

      const stillExists = channels.some((c) => c.id === selectedChannelId);
      if (!stillExists) {
        setViewMode('channel');
        setSelectedChannelId(channelFromUrl && channels.some((c) => c.id === channelFromUrl)
          ? channelFromUrl
          : channels[0].id);
      }
    }
    if (!channelsLoading && channels.length === 0 && dmChannels.length === 0) {
      setViewMode(null);
      setSelectedChannelId(null);
      setSelectedDmId(null);
    }
  }, [channels, channelsLoading, dmChannels, viewMode, selectedChannelId]);

  const selectedChannel: Channel | undefined = channels.find(
    (c) => c.id === selectedChannelId,
  );

  const selectedDM: DMChannel | undefined = dmChannels.find(
    (dm) => dm.id === selectedDmId,
  );

  // --- Channel selection ---
  const handleSelectChannel = useCallback((channelId: string) => {
    setViewMode('channel');
    setSelectedChannelId(channelId);
  }, []);

  // --- DM selection ---
  const handleSelectDM = useCallback((dmId: string) => {
    setViewMode('dm');
    setSelectedDmId(dmId);
    selectDM(dmId);
    markAsRead(dmId);
  }, [selectDM, markAsRead]);

  // --- create channel modal ---
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  const handleCreateChannel = useCallback(
    async (input: CreateChannelInput) => {
      const channel = await createChannel(input);
      handleSelectChannel(channel.id);
    },
    [createChannel, handleSelectChannel],
  );

  // --- create DM modal ---
  const [isCreateDMModalOpen, setIsCreateDMModalOpen] = useState(false);

  const handleCreateDM = useCallback(
    async (input: CreateDMInput): Promise<string> => {
      const dm = await createOrGetDM(input);
      handleSelectDM(dm.id);
      return dm.id;
    },
    [createOrGetDM, handleSelectDM],
  );

  // --- delete channel dialog ---
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const deleteTarget = channels.find((c) => c.id === deleteTargetId);

  const handleDeleteChannel = useCallback(
    async (channelId: string) => {
      if (viewMode === 'channel' && selectedChannelId === channelId) {
        // If the deleted channel is currently selected, try to select another
        const remaining = channels.filter((c) => c.id !== channelId);
        if (remaining.length > 0) {
          handleSelectChannel(remaining[0].id);
        } else {
          setViewMode(null);
          setSelectedChannelId(null);
        }
      }
      await deleteChannel(channelId);
    },
    [viewMode, selectedChannelId, channels, handleSelectChannel, deleteChannel],
  );

  // --- auth guard ---
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push("/auth/login");
    }
  }, [authLoading, isAuthenticated, router]);

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-muted/20">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  // Determine which view to show
  const renderMainContent = () => {
    if (viewMode === 'channel' && selectedChannel) {
      const threadMsgId =
        selectedChannel.id === channelFromUrl ? messageFromUrl : null;
      return (
        <ChannelView
          key={selectedChannel.id}
          channel={selectedChannel}
          initialThreadMessageId={threadMsgId ?? undefined}
        />
      );
    }

    if (viewMode === 'dm' && selectedDM) {
      return (
        <DMView
          key={selectedDM.id}
          dm={selectedDM}
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
          onCreateTask={handleDMCreateTask}
        />
      );
    }

    // Empty/no-selection state
    return (
      <div className="flex flex-1 items-center justify-center bg-muted/5">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
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
        routeIcon={Hash}
        routeTitle="频道与私信"
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
