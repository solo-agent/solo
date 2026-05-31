'use client';

import { useRouter } from 'next/navigation';
import { NavBar } from '@/components/ui/navbar';
import { Sidebar } from '@/components/dashboard/sidebar';
import { useChannels } from '@/lib/hooks/use-channels';
import { useDM } from '@/lib/hooks/use-dm';

/**
 * AppFrame — persistent 3-column layout (NavBar + Sidebar + Content).
 *
 * Wraps standalone app pages (/tasks, /teams, /agents, /computers) so that
 * clicking NavBar icons does not cause layout jumps. The dashboard page
 * renders its own NavBar+Sidebar due to complex modal state management.
 *
 * Channels/DMs are fetched for the Sidebar list; clicking one navigates
 * to the dashboard with the appropriate query param.
 */
export function AppFrame({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const { channels, isLoading: channelsLoading, createChannel, deleteChannel } = useChannels();
  const { dmChannels, isLoadingDMs } = useDM();

  const handleSelectChannel = (channelId: string) => {
    router.push(`/dashboard?channel=${channelId}`);
  };

  const handleCreateChannel = async () => {
    try {
      const channel = await createChannel({ name: '新频道', description: '' });
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
      <NavBar />
      <Sidebar
        channels={channels}
        isLoading={channelsLoading}
        selectedChannelId={null}
        onSelectChannel={handleSelectChannel}
        onCreateChannel={handleCreateChannel}
        onDeleteChannel={(id) => deleteChannel(id)}
        dms={dmChannels}
        dmsLoading={isLoadingDMs}
        selectedDmId={null}
        onSelectDM={handleSelectDM}
        onCreateDM={() => {}}
      />
      <main className="flex flex-1 flex-col overflow-hidden">
        {children}
      </main>
    </div>
  );
}
