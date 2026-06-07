// ============================================================================
// ChannelList — displays channels with loading/empty/list states
// ============================================================================

'use client';

import { Plus, X, ChevronDown } from 'lucide-react';
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
  isExpanded: boolean;
  onToggleExpand: () => void;
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
        className="inline-flex items-center gap-1 border-2 border-black bg-brutal-primary px-3 py-1.5 text-sm font-medium text-black shadow-brutal-sm hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
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
        'group flex cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-sm transition-all',
        isSelected
          ? 'bg-brutal-primary text-black border-2 border-black shadow-brutal-sm'
          : 'text-black border-2 border-transparent hover:border-black',
      )}
      aria-current={isSelected ? 'true' : undefined}
    >
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-info shadow-brutal-sm">
          <span className="font-mono text-base font-bold leading-none select-none">#</span>
        </div>
        <span className="truncate font-body">{channel.name}</span>
      </div>

      <div className="flex items-center gap-1">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="hidden group-hover:flex items-center justify-center rounded-none p-1 hover:bg-brutal-primary-light transition-colors flex-shrink-0"
          aria-label={`关闭 ${channel.name}`}
        >
          <X className="h-4 w-4" />
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
  isExpanded,
  onToggleExpand,
}: ChannelListProps) {
  return (
    <div>
      {/* Section header — group hover covers both chevron and + button so the
          entire row highlights as one unit (chevron + count + + are visually
          grouped) */}
      <div className="group flex items-center justify-between border-2 border-transparent hover:border-black transition-all">
        <button
          type="button"
          onClick={onToggleExpand}
          className="flex flex-1 items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider text-sidebar-muted-foreground font-heading"
          aria-label="展开或折叠 频道"
          aria-expanded={isExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              isExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>Channels</span>
          <span className="ml-auto text-xs tabular-nums opacity-50">{channels.length}</span>
        </button>
        <button
          onClick={onCreateChannel}
          className="mr-2 flex h-5 w-5 items-center justify-center border-2 border-transparent text-sidebar-muted-foreground group-hover:border-black group-hover:text-black hover:bg-brutal-primary/40 active:bg-brutal-primary active:text-black active:ring-2 active:ring-black transition-all cursor-pointer"
          aria-label="创建频道"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Content */}
      {isExpanded && (
        isLoading ? (
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
        )
      )}
    </div>
  );
}
