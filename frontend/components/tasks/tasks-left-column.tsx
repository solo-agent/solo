// ============================================================================
// TasksLeftColumn — 220px-wide left navigation column on /tasks.
// - Static "Tasks" label at the top (matches Sidebar / Teams / Computers).
// - Channels section: collapsible, default expanded. Header has chevron +
//   UPPERCASE name + plain count (no badge, no icon — the chevron is the
//   marker). Item click emits onChannelClick.
// - Direct Messages section: collapsible, default expanded. Same header
//   pattern. Items use a PixelAvatar so agent/user identity is glanceable
//   (matches Teams' Agents row treatment).
// - Selection + data are owned by the parent. Expand/collapse is the only
//   internal state.
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChevronDown, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Button } from '@/components/ui/button';
import type { Channel, DMChannel } from '@/lib/types';
import { t } from '@/lib/i18n';

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

const SECTION_HEADER =
  'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading text-muted-foreground';
const SECTION_COUNT = 'ml-auto text-xs tabular-nums opacity-50';

function getDmDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return t('unknown');
}

function getDmAvatarId(dm: DMChannel): string {
  return dm.other_user?.id || dm.other_agent?.id || dm.id;
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
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-brutal-cream">
      {/* Page label — matches Sidebar / Teams / Computers top label style */}
      <div className="flex items-center h-14 border-b-2 border-black px-4">
        <span className="font-heading text-lg font-bold">Tasks</span>
      </div>

      {/* Sections */}
      <div className="flex-1 overflow-y-auto pt-0 pb-2">
        {/* Channels */}
        <button
          type="button"
          onClick={() => toggle('channels')}
          className={SECTION_HEADER}
          aria-label={t('navCollapseChannels')}
          aria-expanded={isChannelsExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              isChannelsExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>Channels</span>
          <span className={SECTION_COUNT}>{channels.length}</span>
        </button>
        {isChannelsExpanded && (
          <div>
            {channelsLoading && channels.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                {t('loading')}
              </p>
            ) : channelsError ? (
              <div className="flex flex-col items-center gap-2 px-3 py-3">
                <div className="flex items-center gap-1.5 text-brutal-danger">
                  <AlertCircle className="h-4 w-4 flex-shrink-0" />
                  <span className="font-body text-xs">{channelsError}</span>
                </div>
                <Button size="sm" variant="outline" onClick={onRetryChannels}>
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  {t('retry')}
                </Button>
              </div>
            ) : channels.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                {t('noChannelsOrDMs')}
              </p>
            ) : (
              channels.map((channel) => (
                <button
                  key={channel.id}
                  type="button"
                  onClick={() => onChannelClick(channel.id)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm border-2',
                    channel.id === selectedChannelId
                      ? 'border-black bg-brutal-primary text-black shadow-brutal-sm'
                      : 'border-transparent hover:border-black',
                  )}
                  aria-current={channel.id === selectedChannelId ? 'true' : undefined}
                >
                  <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-info shadow-brutal-sm">
                    <span className="font-mono text-base font-bold leading-none select-none">#</span>
                  </div>
                  <span className="truncate font-body">{channel.name}</span>
                </button>
              ))
            )}
          </div>
        )}

        {/* Direct Messages */}
        <button
          type="button"
          onClick={() => toggle('dms')}
          className={SECTION_HEADER}
          aria-label={t('navCollapseDMs')}
          aria-expanded={isDmsExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              isDmsExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>Direct Messages</span>
          <span className={SECTION_COUNT}>{dms.length}</span>
        </button>
        {isDmsExpanded && (
          <div>
            {dmsLoading && dms.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                {t('loading')}
              </p>
            ) : dmsError ? (
              <div className="flex flex-col items-center gap-2 px-3 py-3">
                <div className="flex items-center gap-1.5 text-brutal-danger">
                  <AlertCircle className="h-4 w-4 flex-shrink-0" />
                  <span className="font-body text-xs">{dmsError}</span>
                </div>
                <Button size="sm" variant="outline" onClick={onRetryDMs}>
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  {t('retry')}
                </Button>
              </div>
            ) : dms.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                {t('noDMsYet')}
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
                      'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm border-2',
                      dm.id === selectedDmId
                        ? 'border-black bg-brutal-primary text-black shadow-brutal-sm'
                        : 'border-transparent hover:border-black',
                    )}
                    aria-current={dm.id === selectedDmId ? 'true' : undefined}
                  >
                    <PixelAvatar agentId={getDmAvatarId(dm)} size="sm" />
                    <span className="truncate font-body">{displayName}</span>
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
