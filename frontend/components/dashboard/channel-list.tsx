// ============================================================================
// ChannelList — displays channels with loading/empty/list states
// ============================================================================

'use client';

import { Hash, Plus, Trash2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';
import type { Channel } from '@/lib/types';

interface ChannelListProps {
  channels: Channel[];
  isLoading: boolean;
  selectedChannelId: string | null;
  onSelectChannel: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
}

// ---- Loading skeleton ----

function ChannelListSkeleton() {
  return (
    <div className="space-y-1">
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-2 px-2 py-1.5">
          <Skeleton className="h-4 w-4 rounded-none" />
          <Skeleton className={`h-4 ${i === 1 ? 'w-24' : i === 2 ? 'w-20' : 'w-28'}`} />
        </div>
      ))}
    </div>
  );
}

// ---- Empty state ----

function ChannelListEmpty({ onCreateChannel }: { onCreateChannel: () => void }) {
  return (
    <div className="space-y-3 px-2 py-4 text-center">
      <p className="text-sm text-sidebar-muted-foreground">还没有频道</p>
      <button
        onClick={onCreateChannel}
        className="inline-flex items-center gap-1 rounded-md bg-sidebar-accent px-3 py-1.5 text-sm font-medium text-sidebar-accent-foreground hover:bg-sidebar-accent/80 transition-colors"
      >
        <Plus className="h-3.5 w-3.5" />
        创建频道
      </button>
    </div>
  );
}

// ---- Channel item ----

function ChannelItem({
  channel,
  isSelected,
  onSelect,
  onDelete,
}: {
  channel: Channel;
  isSelected: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect();
        }
      }}
      className={cn(
        'group flex cursor-pointer items-center justify-between px-2 py-1.5 text-sm transition-all',
        isSelected
          ? 'bg-brutal-pink text-black border-2 border-black shadow-brutal-sm'
          : 'text-black hover:bg-brutal-pink/60 border-2 border-transparent',
      )}
      aria-current={isSelected ? 'true' : undefined}
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <Hash className="h-4 w-4 flex-shrink-0" />
        <span className="truncate font-body">{channel.name}</span>
      </div>

      <div className="flex items-center gap-1">
        <span className="text-xs tabular-nums opacity-60 group-hover:opacity-100">
          {channel.member_count}
        </span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="flex h-5 w-5 items-center justify-center rounded-none opacity-0 hover:bg-sidebar-border group-hover:opacity-100 transition-opacity"
          aria-label={`删除 #${channel.name}`}
        >
          <Trash2 className="h-3 w-3" />
        </button>
      </div>
    </div>
  );
}

// ---- Main component ----

export function ChannelList({
  channels,
  isLoading,
  selectedChannelId,
  onSelectChannel,
  onCreateChannel,
  onDeleteChannel,
}: ChannelListProps) {
  return (
    <div>
      {/* Section header */}
      <div className="mb-2 flex items-center justify-between px-2 pb-2 border-b-2 border-sidebar-border">
        <h3 className="text-xs font-bold uppercase tracking-wider text-sidebar-muted-foreground font-heading">
          频道
        </h3>
        <button
          onClick={onCreateChannel}
          className="flex h-5 w-5 items-center justify-center text-sidebar-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground transition-colors"
          aria-label="创建频道"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Content */}
      {isLoading ? (
        <ChannelListSkeleton />
      ) : channels.length === 0 ? (
        <ChannelListEmpty onCreateChannel={onCreateChannel} />
      ) : (
        <div className="space-y-0.5">
          {channels.map((channel) => (
            <ChannelItem
              key={channel.id}
              channel={channel}
              isSelected={channel.id === selectedChannelId}
              onSelect={() => onSelectChannel(channel.id)}
              onDelete={() => onDeleteChannel(channel.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
