'use client';

import Link from 'next/link';
import { t } from '@/lib/i18n';
import { usePathname } from 'next/navigation';
import {
  Gauge,
  LayoutTemplate,
  Monitor,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth-context';
import { UserAvatar } from '@/components/ui/user-avatar';

export const NAV_ITEMS = [
  { href: '/templates', icon: LayoutTemplate, labelKey: 'navTemplates', key: 'templates' },
  { href: '/observability/live', icon: Gauge, labelKey: 'observabilityDashboard', key: 'dashboard' },
  { href: '/computers', icon: Monitor, labelKey: 'navComputers', key: 'computers' },
] as const;

interface NavBarProps {
  onLogoClick?: () => void;
  logoLabel?: string;
}

export function NavBar({ onLogoClick, logoLabel }: NavBarProps = {}) {
  const pathname = usePathname();
  const { user } = useAuth();
  const logoClassName = 'mb-2 flex h-9 w-9 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm';

  return (
    <nav className="navbar-brutal flex w-14 flex-shrink-0 flex-col items-center gap-1 border-r-2 border-black py-3">
      {/* Workspace logo */}
      {onLogoClick ? (
        <button
          type="button"
          onClick={onLogoClick}
          className={logoClassName}
          aria-label={logoLabel ?? t('navSoloWorkspace')}
          title={logoLabel ?? t('navSoloWorkspace')}
        >
          <span className="font-heading text-sm font-black text-black">S</span>
        </button>
      ) : (
        <Link
          href="/dashboard"
          className={logoClassName}
          aria-label={t('navSoloWorkspace')}
        >
          <span className="font-heading text-sm font-black text-black">S</span>
        </Link>
      )}

      {/* Divider */}
      <div className="mb-1 h-px w-8 bg-black/20" />

      {/* Nav items */}
      {NAV_ITEMS.map((item) => {
        const isActive = item.key === 'dashboard'
          ? pathname.startsWith('/observability')
          : pathname === item.href || pathname.startsWith(item.href + '/');
        const label = t(item.labelKey);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              'navbar-icon',
              isActive && 'navbar-icon-active',
            )}
            aria-label={label}
            title={label}
          >
            <item.icon className="h-4 w-4" />
          </Link>
        );
      })}

      {/* Spacer */}
      <div className="mt-auto flex flex-col items-center gap-1">
        {/* User avatar (settings / profile) */}
        {user && (
          <Link
            href="/settings"
            className={cn(
              'navbar-icon mt-1',
              pathname.startsWith('/settings') && 'navbar-icon-active',
            )}
            aria-label={t('navSettings')}
            title={t('navSettings')}
          >
            <UserAvatar
              userId={user.id || 'user'}
              name={user.display_name}
              avatarUrl={user.avatar_url}
              size="sm"
            />
          </Link>
        )}
      </div>
    </nav>
  );
}
