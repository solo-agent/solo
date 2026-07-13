'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { t } from '@/lib/i18n';
import { Sidebar } from '@/components/dashboard/sidebar';
import { useChannels } from '@/lib/hooks/use-channels';
import { useDM } from '@/lib/hooks/use-dm';

/**
 * AppFrame — persistent layout (Sidebar + Content).
 *
 * Wraps standalone app pages (/tasks, /teams, /agents, /computers) so that
 * navigation does not cause layout jumps. The dashboard page renders its own
 * Sidebar due to complex modal state management.
 *
 * Channels/DMs are fetched for the Sidebar list; clicking one navigates
 * to the dashboard with the appropriate query param.
 */
export function AppFrame({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
  const { channels, isLoading: channelsLoading, createChannel, deleteChannel } = useChannels();
  const { dmChannels, isLoadingDMs } = useDM();

  const handleSelectChannel = (channelId: string) => {
    router.push(`/dashboard?channel=${channelId}`);
  };

  const handleCreateChannel = async () => {
    try {
      const channel = await createChannel({ name: t('newChannel'), description: '' });
      router.push(`/dashboard?channel=${channel.id}`);
    } catch {
      // Error handled by useChannels hook
    }
  };

  const handleSelectDM = (dmId: string) => {
    router.push(`/dashboard?dm=${dmId}`);
  };

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <Sidebar
        channels={channels}
        isCollapsed={isSidebarCollapsed}
        onToggleCollapsed={() => setIsSidebarCollapsed((value) => !value)}
        isLoading={channelsLoading}
        selectedChannelId={null}
        onSelectChannel={handleSelectChannel}
        onCreateChannel={handleCreateChannel}
        onDeleteChannel={(id) => deleteChannel(id)}
        dms={dmChannels}
        dmsLoading={isLoadingDMs}
        selectedDmId={null}
        onSelectDM={handleSelectDM}
        inboxSelected={false}
        onSelectInbox={() => router.push('/dashboard?inbox')}
      />
      <main className={`flex flex-1 flex-col overflow-hidden ${isSidebarCollapsed ? '[&_.sidebar-collapse-offset]:pl-20' : ''}`}>
        {children}
      </main>
    </div>
  );
}
