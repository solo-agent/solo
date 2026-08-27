'use client';

import { useEffect, useRef, useState } from 'react';
import { ChevronDown, Link2, MessageCircleMore, Settings, UserPlus, UsersRound } from 'lucide-react';
import { useWorkspace, type ManageTabKey } from '@/lib/workspace-context';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';

const ROLE_KEY = {
  owner: 'workspaceRoleOwner',
  admin: 'workspaceRoleAdmin',
  member: 'workspaceRoleMember',
} as const;

interface MenuItem {
  key: ManageTabKey;
  icon: typeof Settings;
  label: 'workspaceMenuOverview' | 'workspaceMenuMembers' | 'workspaceMenuInvites' | 'workspaceMenuExternal';
}

const MENU_ITEMS: MenuItem[] = [
  { key: 'overview', icon: Settings, label: 'workspaceMenuOverview' },
  { key: 'members', icon: UsersRound, label: 'workspaceMenuMembers' },
  { key: 'invites', icon: Link2, label: 'workspaceMenuInvites' },
  { key: 'external', icon: MessageCircleMore, label: 'workspaceMenuExternal' },
];

// Sidebar header: workspace name + chevron. Click reveals a menu with
// Overview / Members / Invites. Each menu item opens its matching dialog tab.
export function WorkspaceSwitcher() {
  const { activeWorkspace, openManage } = useWorkspace();
  const rootRef = useRef<HTMLDivElement>(null);
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    if (!menuOpen) return;
    const close = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', escape);
    };
  }, [menuOpen]);

  const pick = (item: MenuItem) => {
    setMenuOpen(false);
    openManage(item.key);
  };

  return (
    <div ref={rootRef} className="relative min-w-0 flex-1">
      <button
        type="button"
        onClick={() => setMenuOpen((value) => !value)}
        className="flex w-full min-w-0 items-center gap-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black"
        aria-label={t('workspaceMenuAria')}
        aria-haspopup="menu"
        aria-expanded={menuOpen}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate font-heading text-base font-black text-black">{activeWorkspace?.name ?? 'Solo'}</span>
          <span className="block truncate font-mono text-[9px] font-bold uppercase tracking-wider text-black/50">{activeWorkspace?.role && ROLE_KEY[activeWorkspace.role] ? t(ROLE_KEY[activeWorkspace.role]) : 'Workspace'}</span>
        </span>
        <ChevronDown className={cn('h-4 w-4 shrink-0 transition-transform', menuOpen && 'rotate-180')} />
      </button>

      {menuOpen && (
        <div
          className="absolute left-0 top-[calc(100%+10px)] z-40 w-[220px] rounded-xl border border-border bg-white p-2 shadow-lg"
          role="menu"
        >
          {MENU_ITEMS.filter((item) => item.key !== 'external' || activeWorkspace?.role === 'owner' || activeWorkspace?.role === 'admin').map((item) => (
            <button
              key={item.label}
              type="button"
              onClick={() => pick(item)}
              className="flex w-full items-center gap-3 px-2 py-2 text-left font-body text-sm font-bold hover:bg-brutal-cream"
              role="menuitem"
            >
              <item.icon className="h-5 w-5 shrink-0" />
              <span className="flex-1">{t(item.label)}</span>
              {item.key === 'invites' && (
                <UserPlus className="h-4 w-4 shrink-0 text-black/40" />
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
