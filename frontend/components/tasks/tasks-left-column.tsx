// ============================================================================
// TasksLeftColumn — 220px-wide left navigation column on /tasks.
// - Static "Tasks" label at the top (matches Sidebar / Teams / Computers).
// - Channels section: collapsible, default expanded. Hash icon + label +
//   count badge (bg-brutal-yellow). Item click emits onChannelClick.
// - Direct Messages section: collapsible, default expanded. MessageSquare
//   icon + label + count badge (bg-brutal-stone, white text). Item click
//   emits onDmClick.
// - Selection + data are owned by the parent. Expand/collapse is the only
//   internal state (mirrors TeamsLeftColumn).
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChevronDown, Hash, MessageSquare, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Channel, DMChannel } from '@/lib/types';

interface TasksLeftColumnProps {
  channels: Channel[];
  channelsLoading: boolean;
  channelsError: string | null;
  onRetryChannels: () => void;
  selectedChannelId: string | null;
  onChannelClick: (channelId: string) => void;

  dms: DMChannel[];
  dmsLoading: boolean;
  dmsError: string | null;
  onRetryDMs: () => void;
  selectedDmId: string | null;
  onDmClick: (dmId: string) => void;
}

type SectionKey = 'channels' | 'dms';

function getDmDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return '未知对话';
}

export function TasksLeftColumn({
  channels,
  channelsLoading,
  channelsError,
  onRetryChannels,
  selectedChannelId,
  onChannelClick,
  dms,
  dmsLoading,
  dmsError,
  onRetryDMs,
  selectedDmId,
  onDmClick,
}: TasksLeftColumnProps) {
  // Default: both sections expanded
  const [expanded, setExpanded] = useState<Set<SectionKey>>(
    () => new Set<SectionKey>(['channels', 'dms']),
  );

  const toggle = useCallback((key: SectionKey) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const isChannelsExpanded = expanded.has('channels');
  const isDmsExpanded = expanded.has('dms');

  return (
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-white">
      {/* Page label — matches Sidebar / Teams / Computers top label style */}
      <div className="border-b-2 border-black px-4 py-3">
        <span className="font-heading text-lg font-bold">Tasks</span>
      </div>

      {/* Sections */}
      <div className="flex-1 overflow-y-auto py-2">
        {/* Channels */}
        <button
          type="button"
          onClick={() => toggle('channels')}
          className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold text-foreground hover:bg-brutal-pink/40"
          aria-label="展开或折叠 频道"
          aria-expanded={isChannelsExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-4 w-4 transition-transform',
              isChannelsExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <Hash className="h-4 w-4" />
          <span>Channels</span>
          <span className="ml-auto border border-black bg-brutal-yellow px-1.5 py-0.5 font-mono text-[10px]">
            {channels.length}
          </span>
        </button>
        {isChannelsExpanded && (
          <div>
            {channelsLoading && channels.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                加载中...
              </p>
            ) : channelsError ? (
              <div className="flex flex-col items-center gap-2 px-3 py-3">
                <div className="flex items-center gap-1.5 text-brutal-red">
                  <AlertCircle className="h-4 w-4 flex-shrink-0" />
                  <span className="font-body text-xs">{channelsError}</span>
                </div>
                <button
                  type="button"
                  onClick={onRetryChannels}
                  className="btn-brutal btn-brutal-sm"
                >
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  重试
                </button>
              </div>
            ) : channels.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无频道
              </p>
            ) : (
              channels.map((channel) => (
                <button
                  key={channel.id}
                  type="button"
                  onClick={() => onChannelClick(channel.id)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-2 text-left text-sm border-2',
                    channel.id === selectedChannelId
                      ? 'border-black bg-brutal-pink text-black shadow-brutal-sm'
                      : 'border-transparent hover:bg-brutal-pink/60',
                  )}
                  aria-current={channel.id === selectedChannelId ? 'true' : undefined}
                >
                  <Hash className="h-4 w-4 flex-shrink-0" />
                  <span className="truncate font-body">#{channel.name}</span>
                </button>
              ))
            )}
          </div>
        )}

        {/* Direct Messages */}
        <button
          type="button"
          onClick={() => toggle('dms')}
          className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold text-foreground hover:bg-brutal-pink/40"
          aria-label="展开或折叠 直接消息"
          aria-expanded={isDmsExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-4 w-4 transition-transform',
              isDmsExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <MessageSquare className="h-4 w-4" />
          <span>Direct Messages</span>
          <span className="ml-auto border border-black bg-brutal-stone px-1.5 py-0.5 font-mono text-[10px] text-white">
            {dms.length}
          </span>
        </button>
        {isDmsExpanded && (
          <div>
            {dmsLoading && dms.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                加载中...
              </p>
            ) : dmsError ? (
              <div className="flex flex-col items-center gap-2 px-3 py-3">
                <div className="flex items-center gap-1.5 text-brutal-red">
                  <AlertCircle className="h-4 w-4 flex-shrink-0" />
                  <span className="font-body text-xs">{dmsError}</span>
                </div>
                <button
                  type="button"
                  onClick={onRetryDMs}
                  className="btn-brutal btn-brutal-sm"
                >
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  重试
                </button>
              </div>
            ) : dms.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无对话
              </p>
            ) : (
              dms.map((dm) => {
                const displayName = getDmDisplayName(dm);
                return (
                  <button
                    key={dm.id}
                    type="button"
                    onClick={() => onDmClick(dm.id)}
                    className={cn(
                      'flex w-full items-center gap-2 px-3 py-2 text-left text-sm border-2',
                      dm.id === selectedDmId
                        ? 'border-black bg-brutal-pink text-black shadow-brutal-sm'
                        : 'border-transparent hover:bg-brutal-pink/60',
                    )}
                    aria-current={dm.id === selectedDmId ? 'true' : undefined}
                  >
                    <span className="truncate font-body">@{displayName}</span>
                  </button>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
}
