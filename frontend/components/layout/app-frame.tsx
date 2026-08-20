'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Sidebar } from '@/components/dashboard/sidebar';
import { useChannels } from '@/lib/hooks/use-channels';
import { useDM } from '@/lib/hooks/use-dm';
import { CreateChannelModal } from '@/components/dashboard/create-channel-modal';
import { WorkspaceManageDialog } from '@/components/workspaces/workspace-manage-dialog';
import { WorkspaceRail } from '@/components/workspaces/workspace-rail';
import { GlobalAccountBar } from '@/components/layout/global-account-bar';
import type { CreateChannelInput } from '@/lib/types';

/**
 * AppFrame — persistent layout (Sidebar + Content).
 *
 * Wraps standalone app pages such as /computers so that
 * navigation does not cause layout jumps. The dashboard page renders its own
 * Sidebar due to complex modal state management.
 *
 * Channels/DMs are fetched for the Sidebar list; clicking one navigates
 * to the dashboard with the appropriate query param.
 *
 * The left meta wrapper is `flex flex-col` so the GlobalAccountBar can
 * sit at the bottom spanning col 1 (WorkspaceRail) + col 2 (Sidebar)
 * — same Discord/Slack pattern as PersonalFrame.
 */
export function AppFrame({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
  const [isCreateChannelOpen, setIsCreateChannelOpen] = useState(false);
  const { channels, lucyChannel, isLoading: channelsLoading, createChannel, deleteChannel } = useChannels();
  const { dmChannels, isLoadingDMs } = useDM();

  const handleSelectChannel = (channelId: string) => {
    router.push(`/dashboard?channel=${channelId}`);
  };

  const handleCreateChannel = async (input: CreateChannelInput) => {
    const channel = await createChannel(input);
    router.push(`/dashboard?channel=${channel.id}`);
  };

  const handleSelectDM = (dmId: string) => {
    router.push(`/dashboard?dm=${dmId}`);
  };

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      {/* Left meta column — WorkspaceRail (col 1) + Sidebar (col 2) + GlobalAccountBar (spans 1+2) */}
      <div className="flex flex-shrink-0 flex-col border-r border-border bg-skin-primary">
        <div className="flex flex-1 overflow-hidden">
          <WorkspaceRail />
          <Sidebar
            channels={channels}
            lucyChannel={lucyChannel}
            isCollapsed={isSidebarCollapsed}
            onToggleCollapsed={() => setIsSidebarCollapsed((value) => !value)}
            isLoading={channelsLoading}
            selectedChannelId={null}
            onSelectChannel={handleSelectChannel}
            onCreateChannel={() => setIsCreateChannelOpen(true)}
            onDeleteChannel={(id) => deleteChannel(id)}
            dms={dmChannels}
            dmsLoading={isLoadingDMs}
            selectedDmId={null}
            onSelectDM={handleSelectDM}
            inboxSelected={false}
            onSelectInbox={() => router.push('/dashboard?inbox')}
          />
        </div>
        <GlobalAccountBar />
      </div>
      <main className={`min-w-0 flex flex-1 flex-col overflow-hidden bg-skin-canvas ${isSidebarCollapsed ? '[&_.sidebar-collapse-offset]:pl-20' : ''}`}>
        {children}
      </main>
      <CreateChannelModal
        open={isCreateChannelOpen}
        onOpenChange={setIsCreateChannelOpen}
        onSubmit={handleCreateChannel}
        onChooseTemplate={() => router.push('/templates?create=1')}
        onAskLucy={() => router.push('/dashboard?lucy=1')}
      />
      <WorkspaceManageDialog />
    </div>
  );
}
