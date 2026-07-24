// ============================================================================
// Sidebar — merged Solo navigation + channel list
// ============================================================================

'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Plus, Sparkles } from 'lucide-react';
import { ChannelList } from './channel-list';
import { NAV_ITEMS } from '@/components/ui/navbar';
import { UserAvatar } from '@/components/ui/user-avatar';
import { PanelToggleIcon, panelToggleButtonClass } from '@/components/ui/button';
import { selectableRowClass, selectableRowIconClass } from '@/components/ui/selectable-row';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth-context';
import type { Channel, DMChannel } from '@/lib/types';

interface SidebarProps {
  channels: Channel[];
  lucyChannel?: Channel | null;
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
  isCollapsed = false,
  onToggleCollapsed,
}: SidebarProps) {
  const pathname = usePathname();
  const { user } = useAuth();
  const userName = user?.display_name || user?.email || t('navSettings');

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
      className="navbar-brutal flex h-full w-[240px] flex-shrink-0 flex-col border-r-2 border-black py-3"
    >
      <div className="flex flex-col gap-2">
        <div className="flex w-full items-center gap-3 px-3">
          <Link
            href="/dashboard"
            className="flex h-9 w-9 shrink-0 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm"
            aria-label={t('navSoloWorkspace')}
          >
            <span className="font-heading text-sm font-black text-black">S</span>
          </Link>
          <div className="min-w-0 flex-1">
            <div className="truncate font-heading text-xl font-black text-black">Solo</div>
          </div>
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

      <div className="mt-3 flex min-h-0 flex-1 flex-col border-t-2 border-black pt-2">
        <div className="flex items-center gap-2 px-3 py-2">
          <div className="min-w-0 flex-1 font-heading text-xs font-black uppercase tracking-wider text-black/70">
            {t('navChannels')}
          </div>
          <span className="font-mono text-xs font-bold tabular-nums text-black/45">
            {channels.length}
          </span>
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
        <div className="min-h-0 flex-1 overflow-y-auto pb-2">
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
      </div>

      <div className="mt-auto flex flex-col gap-0.5 pt-3">
        <Link
          href="/settings"
          className={selectableRowClass(
            pathname.startsWith('/settings'),
            cn(
              'w-full text-left',
              pathname.startsWith('/settings') ? 'bg-white' : 'hover:bg-white/50',
            ),
          )}
          aria-label={t('navSettings')}
          aria-current={pathname.startsWith('/settings') ? 'page' : undefined}
        >
          {user ? (
            <UserAvatar
              userId={user.id || 'user'}
              name={user.display_name}
              avatarUrl={user.avatar_url}
              size="sm"
            />
          ) : (
            <span className={selectableRowIconClass('bg-white font-heading text-sm font-black')}>
              S
            </span>
          )}
          <span className="min-w-0">
            <span className="block truncate font-heading text-sm font-black text-black">
              {t('navSettings')}
            </span>
            <span className="block truncate font-mono text-[10px] font-bold text-black/55">
              {userName}
            </span>
          </span>
        </Link>
      </div>
    </aside>
  );
}
