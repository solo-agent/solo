'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Home, Monitor, Settings } from 'lucide-react';
import { WorkspaceRail } from '@/components/workspaces/workspace-rail';
import { UserAvatar } from '@/components/ui/user-avatar';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { isPersonalRouteActive } from '@/lib/personal-navigation';
import { cn } from '@/lib/utils';

const items = [
  { href: '/home', icon: Home, label: 'personalHome' },
  { href: '/computers', icon: Monitor, label: 'personalComputers' },
  { href: '/settings', icon: Settings, label: 'personalSettings' },
] as const;

export function PersonalFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { user } = useAuth();

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <WorkspaceRail />
      <aside className="navbar-brutal flex w-[240px] flex-shrink-0 flex-col border-r-2 border-black">
        <div className="border-b-2 border-black px-5 py-5">
          <div className="flex items-center gap-3">
            {user && (
              <UserAvatar
                userId={user.id}
                name={user.display_name}
                avatarUrl={user.avatar_url}
                size="md"
              />
            )}
            <div className="min-w-0">
              <p className="truncate font-heading text-base font-black">{user?.display_name ?? 'Solo'}</p>
              <p className="truncate font-mono text-[10px] text-muted-foreground">{user?.email}</p>
            </div>
          </div>
        </div>

        <nav aria-label={t('personalNavLabel')} className="space-y-1 p-3">
          {items.map((item) => {
            const active = isPersonalRouteActive(pathname, item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                data-onboarding={item.href === '/computers' ? 'computers-nav' : undefined}
                aria-current={active ? 'page' : undefined}
                className={cn(
                  'flex items-center gap-3 border-2 px-3 py-3 font-body text-sm font-bold transition-colors',
                  active
                    ? 'border-black bg-white shadow-brutal-sm'
                    : 'border-transparent hover:border-black hover:bg-white/60',
                )}
              >
                <item.icon className="h-5 w-5" aria-hidden="true" />
                {t(item.label)}
              </Link>
            );
          })}
        </nav>
      </aside>
      <main className="min-w-0 flex-1 overflow-hidden bg-white">{children}</main>
    </div>
  );
}
