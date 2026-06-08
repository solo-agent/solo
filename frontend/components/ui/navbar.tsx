'use client';

import Link from 'next/link';
import { t } from '@/lib/i18n';
import { usePathname } from 'next/navigation';
import {
  Hash,
  ClipboardList,
  Users,
  Monitor,
  Settings,
  FolderOpen,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth-context';
import { PixelAvatar } from '@/components/ui/pixel-avatar';

const NAV_ITEMS = [
  { href: '/dashboard', icon: Hash, label: t('navChannels') },
  { href: '/tasks', icon: ClipboardList, label: t('navTasks') },
  { href: '/teams', icon: Users, label: t('navTeams') },
  { href: '/computers', icon: Monitor, label: t('navComputers') },
  { href: '/workspace', icon: FolderOpen, label: 'Workspace' },
] as const;

export function NavBar() {
  const pathname = usePathname();
  const { user } = useAuth();

  return (
    <nav className="navbar-brutal flex w-14 flex-shrink-0 flex-col items-center gap-1 border-r-2 border-black py-3">
      {/* Workspace logo */}
      <Link
        href="/dashboard"
        className="mb-2 flex h-9 w-9 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm"
        aria-label={t('navSoloWorkspace')}
      >
        <span className="font-heading text-sm font-black text-black">S</span>
      </Link>

      {/* Divider */}
      <div className="mb-1 h-px w-8 bg-black/20" />

      {/* Nav items */}
      {NAV_ITEMS.map((item) => {
        const isActive = pathname === item.href || pathname.startsWith(item.href + '/');
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              'navbar-icon',
              isActive && 'navbar-icon-active',
            )}
            aria-label={item.label}
            title={item.label}
          >
            <item.icon className="h-4 w-4" />
          </Link>
        );
      })}

      {/* Spacer */}
      <div className="mt-auto flex flex-col items-center gap-1">
        {/* Settings */}
        <Link
          href="/settings"
          className={cn(
            'navbar-icon',
            pathname.startsWith('/settings') && 'navbar-icon-active',
          )}
          aria-label={t('navSettings')}
          title={t('navSettings')}
        >
          <Settings className="h-4 w-4" />
        </Link>

        {/* User avatar (pixel style for consistency) */}
        {user && (
          <Link
            href="/settings"
            className="navbar-icon mt-1"
            aria-label={user.display_name || user.email || t('user')}
            title={user.display_name || user.email || t('user')}
          >
            <PixelAvatar
              agentId={user.id || 'user'}
              size="sm"
            />
          </Link>
        )}
      </div>
    </nav>
  );
}
