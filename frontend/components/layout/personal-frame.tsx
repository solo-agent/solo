'use client';

import Link from 'next/link';
import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { Bookmark, Home, Menu, Monitor, Settings, X } from 'lucide-react';
import { WorkspaceRail } from '@/components/workspaces/workspace-rail';
import { GlobalAccountBar } from '@/components/layout/global-account-bar';
import { t } from '@/lib/i18n';
import { isPersonalRouteActive } from '@/lib/personal-navigation';
import { cn } from '@/lib/utils';

const items = [
  { href: '/home', icon: Home, label: 'personalHome' },
  { href: '/computers', icon: Monitor, label: 'personalComputers' },
  { href: '/favorites', icon: Bookmark, label: 'personalFavorites' },
  { href: '/settings', icon: Settings, label: 'personalSettings' },
] as const;

export function PersonalFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  return (
    <div className="flex h-[100dvh] min-w-0 overflow-hidden bg-brutal-cream">
      <button type="button" className="fixed left-3 top-3 z-50 flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-white shadow-sm lg:hidden" onClick={() => setMobileNavOpen((open) => !open)} aria-label={t(mobileNavOpen ? 'mobileNavigationClose' : 'mobileNavigationOpen')}>
        {mobileNavOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
      </button>
      {mobileNavOpen && <button type="button" className="fixed inset-0 z-30 bg-black/35 lg:hidden" onClick={() => setMobileNavOpen(false)} aria-label={t('mobileNavigationClose')} />}
      {/* Left meta column — WorkspaceRail (col 1) + PersonalNav (col 2) sit on top,
          and the GlobalAccountBar spans their combined width at the bottom
          (Discord / Slack pattern). Personal settings is kept in the nav
          because it is a global destination, not a per-workspace one. */}
      <div className={`fixed inset-y-0 left-0 z-40 flex flex-shrink-0 flex-col border-r border-border bg-skin-primary transition-transform lg:static lg:translate-x-0 ${mobileNavOpen ? 'translate-x-0' : '-translate-x-full'}`}>
        <div className="flex flex-1 overflow-hidden">
          <WorkspaceRail />
          <aside className="navbar-brutal flex w-[240px] flex-shrink-0 flex-col">
            <nav aria-label={t('personalNavLabel')} className="flex-1 space-y-1 p-3">
              {items.map((item) => {
                const active = isPersonalRouteActive(pathname, item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    onClick={() => setMobileNavOpen(false)}
                    data-onboarding={item.href === '/computers' ? 'computers-nav' : undefined}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'relative flex items-center gap-3 rounded-lg border border-transparent px-3 py-3 font-body text-sm transition-colors',
                      active
                        ? 'text-skin-ink font-medium before:absolute before:left-0 before:top-1/2 before:h-6 before:w-1 before:-translate-y-1/2 before:rounded-full before:bg-skin-accent'
                        : 'text-skin-subtle-text hover:border-skin-rule hover:bg-white/60',
                    )}
                  >
                    <item.icon className="h-5 w-5" aria-hidden="true" />
                    {t(item.label)}
                  </Link>
                );
              })}
            </nav>
          </aside>
        </div>
        <GlobalAccountBar />
      </div>
      <main className="min-w-0 flex-1 overflow-hidden bg-skin-canvas">{children}</main>
    </div>
  );
}
