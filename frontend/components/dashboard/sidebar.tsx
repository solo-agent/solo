// ============================================================================
// Sidebar — channel list + DM list sidebar (v1.5: + Inbox)
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChannelList } from './channel-list';
import { DMList } from './dm-list';
import { InboxBadge } from '@/components/inbox/inbox-badge';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import { t } from '@/lib/i18n';
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
  onCreateDM?: () => void;
  /** Inbox props */
  inboxSelected: boolean;
  onSelectInbox: () => void;
  /** Page label rendered at the top of the sidebar. */
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
  routeTitle = t('navChannels'),
}: SidebarProps) {
  const { unreadCount, isLoading: unreadLoading } = useInboxUnread();
  const [channelsExpanded, setChannelsExpanded] = useState(true);
  const toggleChannels = useCallback(() => setChannelsExpanded((v) => !v), []);
  const [dmsExpanded, setDmsExpanded] = useState(true);
  const toggleDMs = useCallback(() => setDmsExpanded((v) => !v), []);

  return (
    <aside className="flex w-[220px] flex-col bg-sidebar text-sidebar-foreground border-r-2 border-sidebar-border flex-shrink-0">
      {/* Page label — matches Teams / Tasks / Computers top label style */}
      <div className="flex items-center h-14 border-b-2 border-sidebar-border px-4">
        <span className="font-heading text-lg font-bold text-sidebar-foreground truncate">
          {routeTitle}
        </span>
      </div>

      {/* Inbox badge — above channel list, navigates to ?inbox */}
      <InboxBadge
        unreadCount={unreadLoading ? 0 : unreadCount.total}
        isSelected={inboxSelected}
        onClick={onSelectInbox}
      />

      {/* Scrollable channel + DM area — matches TeamsLeftColumn's scroll
          container: py-2 only, no horizontal padding (rows pad themselves) */}
      <div className="flex-1 overflow-y-auto pt-0 pb-2">
        <ChannelList
          channels={channels}
          isLoading={isLoading}
          selectedChannelId={selectedChannelId}
          onSelectChannel={onSelectChannel}
          onCreateChannel={onCreateChannel}
          onDeleteChannel={onDeleteChannel}
          isExpanded={channelsExpanded}
          onToggleExpand={toggleChannels}
        />

        <DMList
          dms={dms}
          isLoading={dmsLoading}
          selectedDmId={selectedDmId}
          onSelectDM={onSelectDM}
          onCreateDM={onCreateDM}
          isExpanded={dmsExpanded}
          onToggleExpand={toggleDMs}
        />
      </div>
    </aside>
  );
}
