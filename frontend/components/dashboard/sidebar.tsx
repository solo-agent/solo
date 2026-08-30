// ============================================================================
// Sidebar — merged Solo navigation + channel list
// ============================================================================

'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { usePathname } from 'next/navigation';
import { ChevronDown, Plus, Sparkles } from 'lucide-react';
import { ChannelList } from './channel-list';
import { InboxBadge } from '@/components/inbox/inbox-badge';
import { NAV_ITEMS } from '@/components/ui/navbar';
import { PanelToggleIcon, panelToggleButtonClass } from '@/components/ui/button';
import { selectableRowClass, selectableRowIconClass } from '@/components/ui/selectable-row';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import type { Channel, DMChannel } from '@/lib/types';
import { WorkspaceSwitcher } from '@/components/workspaces/workspace-switcher';
import { WorkspacePeople } from '@/components/workspaces/workspace-people';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
import { PixelAvatar } from '@/components/ui/pixel-avatar';

interface SidebarProps {
  channels: Channel[];
  lucyChannel?: Channel | null;
  isLoading: boolean;
  selectedChannelId: string | null;
  onSelectChannel: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  dms: DMChannel[];
  selectedDmId: string | null;
  onStartAgentDM?: (agentId: string) => void | Promise<void>;
  /** Inbox props */
  inboxSelected: boolean;
  onSelectInbox: () => void;
  isCollapsed?: boolean;
  onToggleCollapsed?: () => void;
}

export function Sidebar({
  channels,
  lucyChannel,
  isLoading,
  selectedChannelId,
  onSelectChannel,
  onCreateChannel,
  onDeleteChannel,
  dms,
  selectedDmId,
  onStartAgentDM,
  inboxSelected,
  onSelectInbox,
  isCollapsed = false,
  onToggleCollapsed,
}: SidebarProps) {
  const pathname = usePathname();
  const { unreadCount, isLoading: unreadLoading } = useInboxUnread();
  const [channelsExpanded, setChannelsExpanded] = useState(true);
  const [agentsExpanded, setAgentsExpanded] = useState(true);
  const [agentChannelId, setAgentChannelId] = useState(selectedChannelId);
  useEffect(() => {
    if (selectedChannelId) setAgentChannelId(selectedChannelId);
  }, [selectedChannelId]);
  const { agents } = useChannelMembers(agentChannelId);

  if (isCollapsed) {
    return (
      <div className="relative h-full w-0 flex-shrink-0">
        <button
          type="button"
          onClick={onToggleCollapsed}
          className={panelToggleButtonClass(false, 'absolute left-3 top-3 z-30')}
          aria-label={t('navCollapseChannels')}
          title={t('navCollapseChannels')}
        >
          <PanelToggleIcon side="left" />
        </button>
      </div>
    );
  }

  return (
    <aside
      className="navbar-brutal flex h-full w-[240px] flex-shrink-0 flex-col py-3"
    >
      <div className="flex flex-col gap-2">
        <div className="flex w-full items-center gap-2 px-3">
          <WorkspaceSwitcher />
          <button
            type="button"
            onClick={onToggleCollapsed}
            className={panelToggleButtonClass(true, 'shrink-0')}
            aria-label={t('navCollapseChannels')}
            title={t('navCollapseChannels')}
          >
            <PanelToggleIcon side="left" />
          </button>
        </div>

        <div className="mx-3 h-px bg-black/20" />

        <div className="space-y-0.5">
          {lucyChannel && (
            <button
              type="button"
              onClick={() => onSelectChannel(lucyChannel.id)}
              className={selectableRowClass(
                selectedChannelId === lucyChannel.id,
                cn(
                  'w-full text-left',
                  selectedChannelId === lucyChannel.id ? 'bg-white' : 'hover:bg-white/50',
                ),
              )}
              aria-label="Lucy"
              aria-current={selectedChannelId === lucyChannel.id ? 'true' : undefined}
            >
              <span className={selectableRowIconClass('bg-brutal-accent-light')}>
                <Sparkles className="h-4 w-4 text-brutal-accent" />
              </span>
              <span>
                <span className="block font-heading text-sm font-black">Lucy</span>
                <span className="block font-mono text-[9px] font-bold uppercase text-black/55">
                  {t('lucyStewardChannel')}
                </span>
              </span>
            </button>
          )}
          <InboxBadge
            unreadCount={unreadLoading ? 0 : unreadCount.total}
            isSelected={inboxSelected}
            onClick={onSelectInbox}
          />
          {NAV_ITEMS.map((item) => {
            const isActive = item.key === 'dashboard'
              ? pathname.startsWith('/observability')
              : pathname === item.href || pathname.startsWith(item.href + '/');
            const label = t(item.labelKey);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={selectableRowClass(
                  isActive,
                  cn(
                    'w-full text-left',
                    isActive ? 'bg-white' : 'hover:bg-white/50',
                  ),
                )}
                aria-label={label}
                aria-current={isActive ? 'page' : undefined}
              >
                <span className={selectableRowIconClass('bg-white')}>
                  <item.icon className="h-4 w-4" />
                </span>
                <span className="truncate font-body">{label}</span>
              </Link>
            );
          })}
        </div>
      </div>

      <div className="mt-3 min-h-0 flex-1 overflow-y-auto pt-2">
        <div className="flex items-center gap-2 px-3 py-2">
          <button
            type="button"
            onClick={() => setChannelsExpanded((value) => !value)}
            className="flex min-w-0 flex-1 items-center gap-2 text-left"
            aria-expanded={channelsExpanded}
            aria-label={`${t('navChannels')} ${channels.length}`}
          >
            <ChevronDown className={cn('h-3.5 w-3.5 shrink-0 transition-transform', !channelsExpanded && '-rotate-90')} />
            <span className="min-w-0 flex-1 font-heading text-xs font-black uppercase tracking-wider text-black/70">
              {t('navChannels')}
            </span>
            <span className="font-mono text-xs font-bold tabular-nums text-black/45">
              {channels.length}
            </span>
          </button>
          <button
            type="button"
            onClick={onCreateChannel}
            className="flex h-7 w-7 shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm transition-[transform,box-shadow] hover:-translate-y-px hover:shadow-brutal"
            aria-label={t('createChannel')}
            title={t('createChannel')}
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>
        {channelsExpanded && (
          <div className="pb-1">
            <ChannelList
              channels={channels}
              isLoading={isLoading}
              selectedChannelId={selectedChannelId}
              onSelectChannel={onSelectChannel}
              onCreateChannel={onCreateChannel}
              onDeleteChannel={onDeleteChannel}
              showHeader={false}
              railSurface
            />
          </div>
        )}
        <WorkspacePeople />
        {agentChannelId && (
          <div>
            <button
              type="button"
              onClick={() => setAgentsExpanded((value) => !value)}
              className="flex w-full items-center gap-2 px-3 py-2 text-left"
              aria-expanded={agentsExpanded}
              aria-label={t('expandOrCollapseAgents')}
            >
              <ChevronDown className={cn('h-3.5 w-3.5 transition-transform', !agentsExpanded && '-rotate-90')} />
              <span className="min-w-0 flex-1 font-heading text-xs font-black uppercase tracking-wider text-black/70">
                {t('observabilityAgents')}
              </span>
              <span className="font-mono text-xs font-bold tabular-nums text-black/45">{agents.length}</span>
            </button>
            {agentsExpanded && (
              <div className="space-y-0.5 pb-1">
                {agents.length === 0 ? (
                  <p className="px-8 py-2 font-body text-xs text-black/45">{t('noAgentsHint')}</p>
                ) : agents.map((agent) => {
                  const dm = dms.find((item) => item.other_agent?.id === agent.member_id);
                  const isSelected = dm?.id === selectedDmId;
                  return (
                    <button
                      key={agent.member_id}
                      type="button"
                      onClick={() => { void onStartAgentDM?.(agent.member_id); }}
                      className={selectableRowClass(isSelected, 'w-full text-left hover:bg-white/50')}
                      aria-label={agent.display_name}
                      aria-current={isSelected ? 'true' : undefined}
                    >
                      <PixelAvatar agentId={agent.member_id} avatarUrl={agent.avatar_url} size="sm" />
                      <span className="min-w-0 flex-1 truncate font-body text-sm">{agent.display_name}</span>
                      {!!dm?.unread_count && (
                        <span className="font-mono text-xs font-bold tabular-nums text-black/55">{dm.unread_count}</span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </aside>
  );
}
