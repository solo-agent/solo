// ============================================================================
// ChannelList — displays channels with loading/empty/list states
// ============================================================================

'use client';

import { useState } from 'react';
import { Plus, ChevronDown, Search, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { selectableRowClass, selectableRowIconClass } from '@/components/ui/selectable-row';
import { iconActionClass } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import type { Channel } from '@/lib/types';

interface ChannelListProps {
  channels: Channel[];
  isLoading: boolean;
  selectedChannelId: string | null;
  onSelectChannel: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  isExpanded?: boolean;
  onToggleExpand?: () => void;
  showHeader?: boolean;
  railSurface?: boolean;
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
      <p className="text-sm text-sidebar-muted-foreground">{t('noChannelsYet')}</p>
      <button
        onClick={onCreateChannel}
        className="inline-flex items-center gap-1 border-2 border-black bg-brutal-primary px-3 py-1.5 text-sm font-medium text-black shadow-brutal-sm hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
      >
        <Plus className="h-3.5 w-3.5" />
        {t('createChannel')}
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
  canDelete,
  railSurface,
}: {
  channel: Channel;
  isSelected: boolean;
  onSelect: () => void;
  onDelete: () => void;
  canDelete: boolean;
  railSurface?: boolean;
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
      className={selectableRowClass(
        isSelected,
        cn(
          'justify-between',
          railSurface && (isSelected ? 'bg-white' : 'hover:bg-white/50'),
        ),
      )}
      aria-current={isSelected ? 'true' : undefined}
    >
      <div className="flex min-w-0 items-center gap-2">
        <div className={selectableRowIconClass('channel-hash-icon bg-brutal-info')}>
          <span className="font-mono text-base font-bold leading-none select-none">#</span>
        </div>
        <span className="truncate font-body">{channel.name}</span>
      </div>
      {canDelete && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className={iconActionClass('invisible h-7 w-7 shrink-0 p-0 hover:bg-brutal-danger-light group-hover:visible')}
          aria-label={t('closeChannel', { name: channel.name })}
          title={t('closeChannel', { name: channel.name })}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
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
  isExpanded = true,
  onToggleExpand,
  showHeader = true,
  railSurface = false,
}: ChannelListProps) {
  const [query, setQuery] = useState('');
  const [showAll, setShowAll] = useState(false);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredChannels = normalizedQuery
    ? channels.filter((channel) => channel.name.toLocaleLowerCase().includes(normalizedQuery))
    : channels;
  let visibleChannels = filteredChannels;
  if (!normalizedQuery && !showAll && channels.length > 12) {
    visibleChannels = channels.slice(0, 12);
    const selected = channels.find((channel) => channel.id === selectedChannelId);
    if (selected && !visibleChannels.some((channel) => channel.id === selected.id)) {
      visibleChannels = [...visibleChannels.slice(0, 11), selected];
    }
  }

  const content = isLoading ? (
    <ChannelListSkeleton />
  ) : channels.length === 0 ? (
    <ChannelListEmpty onCreateChannel={onCreateChannel} />
  ) : (
    <div className="space-y-0.5">
      {channels.length > 12 && (
        <label className="relative mb-2 block px-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-black/45" />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('channelListSearchPlaceholder')}
            aria-label={t('channelListSearchPlaceholder')}
            className="input-brutal h-8 w-full pl-8 pr-2 text-xs"
          />
        </label>
      )}
      {visibleChannels.map((channel) => (
        <ChannelItem
          key={channel.id}
          channel={channel}
          isSelected={channel.id === selectedChannelId}
          onSelect={() => onSelectChannel(channel.id)}
          onDelete={() => onDeleteChannel(channel.id)}
          canDelete={!channel.name.startsWith('all-')}
          railSurface={railSurface}
        />
      ))}
      {!normalizedQuery && channels.length > 12 && (
        <button
          type="button"
          onClick={() => setShowAll((value) => !value)}
          className="mt-1 w-full px-3 py-2 text-left font-mono text-[10px] font-bold uppercase tracking-wider text-black/55 hover:bg-white/50 hover:text-black"
        >
          {showAll ? t('showFewerChannels') : t('showAllChannels', { count: channels.length })}
        </button>
      )}
      {normalizedQuery && visibleChannels.length === 0 && (
        <p className="px-3 py-4 text-center text-xs text-black/45">{t('channelSearchEmpty')}</p>
      )}
    </div>
  );

  return (
    <div>
      {/* Section header — group hover covers both chevron and + button so the
          entire row highlights as one unit (chevron + count + + are visually
          grouped) */}
      {showHeader && (
        <div className="group flex items-center justify-between border-2 border-transparent transition-all hover:border-black">
          <button
            type="button"
            onClick={onToggleExpand}
            className="flex flex-1 items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider text-sidebar-muted-foreground font-heading"
            aria-label={t('navCollapseChannels')}
            aria-expanded={isExpanded}
          >
            <ChevronDown
              aria-hidden="true"
              className={cn(
                'h-3 w-3 transition-transform',
                isExpanded ? 'rotate-0' : '-rotate-90',
              )}
            />
            <span>{t('navChannels')}</span>
            <span className="ml-auto text-xs tabular-nums opacity-50">{channels.length}</span>
          </button>
          <button
            onClick={onCreateChannel}
            className="mr-2 flex h-5 w-5 cursor-pointer items-center justify-center border-2 border-transparent text-sidebar-muted-foreground transition-all group-hover:border-black group-hover:text-black hover:bg-brutal-primary/40 active:bg-brutal-primary active:text-black active:ring-2 active:ring-black"
            aria-label={t('createChannel')}
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      {/* Content */}
      {(!showHeader || isExpanded) && content}
    </div>
  );
}
