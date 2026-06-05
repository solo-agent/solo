// ============================================================================
// Sidebar — channel list + DM list sidebar (v1.5: + Inbox)
// ============================================================================

'use client';

import { ChannelList } from './channel-list';
import { DMList } from './dm-list';
import { InboxBadge } from '@/components/inbox/inbox-badge';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import type { Channel, DMChannel } from '@/lib/types';

interface SidebarProps {
  channels: Channel[];
  isLoading: boolean;
  selectedChannelId: string | null;
  onSelectChannel: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  /** DM props */
  dms: DMChannel[];
  dmsLoading: boolean;
  selectedDmId: string | null;
  onSelectDM: (dmId: string) => void;
  onCreateDM: () => void;
  /** Inbox props */
  inboxSelected: boolean;
  onSelectInbox: () => void;
  /** Route context for header */
  routeIcon?: React.ElementType;
  routeTitle?: string;
}

export function Sidebar({
  channels,
  isLoading,
  selectedChannelId,
  onSelectChannel,
  onCreateChannel,
  onDeleteChannel,
  dms,
  dmsLoading,
  selectedDmId,
  onSelectDM,
  onCreateDM,
  inboxSelected,
  onSelectInbox,
  routeIcon: Icon,
  routeTitle = 'Solo',
}: SidebarProps) {
  const { unreadCount, isLoading: unreadLoading } = useInboxUnread();

  return (
    <aside className="flex w-50 flex-col bg-sidebar text-sidebar-foreground border-r-2 border-sidebar-border flex-shrink-0">
      {/* Route-aware header */}
      <div className="flex h-14 items-center border-b-2 border-sidebar-border px-4">
        <div className="flex items-center gap-2">
          {Icon && <Icon className="h-5 w-5 flex-shrink-0" />}
          <span className="font-heading font-bold text-sidebar-foreground text-sm truncate">{routeTitle}</span>
        </div>
      </div>

      {/* Inbox badge — above channel list, navigates to ?inbox */}
      <InboxBadge
        unreadCount={unreadLoading ? 0 : unreadCount.total}
        isSelected={inboxSelected}
        onClick={onSelectInbox}
      />

      {/* Scrollable channel + DM area */}
      <div className="flex-1 overflow-y-auto px-2 py-3">
        <ChannelList
          channels={channels}
          isLoading={isLoading}
          selectedChannelId={selectedChannelId}
          onSelectChannel={onSelectChannel}
          onCreateChannel={onCreateChannel}
          onDeleteChannel={onDeleteChannel}
        />

        {/* DM section */}
        <div className="mt-6">
          <DMList
            dms={dms}
            isLoading={dmsLoading}
            selectedDmId={selectedDmId}
            onSelectDM={onSelectDM}
            onCreateDM={onCreateDM}
          />
        </div>
      </div>

    </aside>
  );
}
