"use client";

import { useEffect, useState, useCallback, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import dynamic from "next/dynamic";
import { MessageSquare, RefreshCw } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { t } from '@/lib/i18n';
import { useChannels } from "@/lib/hooks/use-channels";
import { useChannelMembers } from "@/lib/hooks/use-channel-members";
import { useDM } from "@/lib/hooks/use-dm";
import { useDMTasks } from "@/lib/hooks/use-tasks";
import { Sidebar } from "@/components/dashboard/sidebar";
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
import type { Channel, DMChannel, CreateChannelInput, CreateDMInput, Message, Task } from "@/lib/types";
import { useToast } from "@/components/ui/toast";

export default function DashboardPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-brutal-cream">
          <div className="flex flex-col items-center gap-3">
            <Spinner size="md" />
            <p className="font-mono text-sm text-muted-foreground">{t('loading')}</p>
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
  const lucyFromUrl = searchParams.has('lucy');

  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const {
    channels,
    lucyChannel,
    isLoading: channelsLoading,
    error: channelsError,
    createChannel,
    deleteChannel,
    refetch: refetchChannels,
  } = useChannels();

  // Derive view mode entirely from URL. The stable `?lucy=1` entry point
  // resolves to Lucy's real Channel once it has loaded.
  const selectedChannelId = channelFromUrl ?? (lucyFromUrl ? lucyChannel?.id ?? null : null);
  const selectedDmId = dmFromUrl;
  const viewMode = selectedChannelId ? 'channel' as const : dmFromUrl ? 'dm' as const : inboxFromUrl ? 'inbox' as const : null;

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
  } = useDM(dmFromUrl);

  const { showToast } = useToast();

  // ---- DM Tasks (v1.2 Phase 2+3) ----
  const {
    tasks: dmTasks,
    isLoading: dmTasksLoading,
    error: dmTasksError,
    convertMessageToTask: dmConvertMessageToTask,
    refetch: dmRefetchTasks,
  } = useDMTasks(selectedDmId);

  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);

  // ---- DM AsTask handler (SOLO-232-F) ----
  const handleDMAsTask = useCallback(
    async (message: Message) => {
      if (!selectedDmId) return;
      const title = message.content.slice(0, 200);
      try {
        const task = await dmConvertMessageToTask(selectedDmId, message.id, title);
        showToast(t('taskConverted', { n: task.task_number ?? '?' }), 'success');
        dmRefetchTasks();
      } catch {
        showToast(t('taskConvertError'), 'error');
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

  // ---- Onboarding wizard detection ----
  const { agents: channelAgents } = useChannelMembers(selectedChannelId);

  const selectedChannel: Channel | undefined = selectedChannelId === lucyChannel?.id
    ? lucyChannel
    : channels.find((c) => c.id === selectedChannelId);

  const isOnboardingChannel = selectedChannel?.name?.startsWith('welcome-')
    || (selectedChannel?.type === 'lucy' && channelAgents.length === 0);
  const showOnboardingWizard = isOnboardingChannel && channelAgents.length === 0;

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

  const handleDMThreadChange = useCallback(
    (threadId: string | null) => {
      if (!dmFromUrl) return;
      const params = new URLSearchParams(searchParams.toString());
      params.set('dm', dmFromUrl);
      if (threadId) params.set('thread', threadId);
      else params.delete('thread');
      router.push(`/dashboard?${params.toString()}`);
    },
    [router, dmFromUrl, searchParams],
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
      const channel = channels.find((c) => c.id === channelId);
      if (channel?.name.startsWith('all-')) return;

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
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
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
          showOnboardingWizard={showOnboardingWizard}
          initialThreadMessageId={threadFromUrl ?? undefined}
          initialScrollToMessageId={messageFromUrl ?? undefined}
          onChannelCreated={refetchChannels}
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
          onAsTask={handleDMAsTask}
          refetch={refetchDMs}
          tasks={dmTasks}
          tasksLoading={dmTasksLoading}
          tasksError={dmTasksError}
          refetchTasks={dmRefetchTasks}
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
              ? t('loading')
              : channelsError || dmError
                ? t('loadError')
                : channels.length === 0 && dmChannels.length === 0
                  ? t('noChannelsOrDMs')
                  : t('selectChannelPrompt')}
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {channelsError || dmError
              ? channelsError || dmError
              : channels.length === 0 && dmChannels.length === 0
                ? t('createChannelPrompt')
                : t('selectChannelPrompt')}
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
              {t('retry')}
            </Button>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <Sidebar
        isCollapsed={isSidebarCollapsed}
        onToggleCollapsed={() => setIsSidebarCollapsed((value) => !value)}
        channels={channels}
        lucyChannel={lucyChannel}
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
      />
      {/* Main content area */}
      <main className="relative flex flex-1 flex-col overflow-hidden">
        <div className={`flex min-h-0 flex-1 overflow-hidden ${isSidebarCollapsed ? '[&_.sidebar-collapse-offset]:pl-14' : ''}`}>
          {renderMainContent()}
        </div>
      </main>

      {/* Modals */}
      <CreateChannelModal
        open={isCreateModalOpen}
        onOpenChange={setIsCreateModalOpen}
        onSubmit={handleCreateChannel}
        onChooseTemplate={() => router.push('/templates?create=1')}
        onAskLucy={() => router.push('/dashboard?lucy=1')}
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
