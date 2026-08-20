'use client';

import Link from 'next/link';
import { Settings } from 'lucide-react';
import { UserAvatar } from '@/components/ui/user-avatar';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';

/**
 * GlobalAccountBar — the persistent personal-area account chip.
 *
 * Lives in the PersonalFrame / AppFrame / dashboard chrome wrapper, sitting
 * beneath both the workspace rail (col 1) and the personal nav aside (col 2).
 * Visually it spans the combined width of those two columns and is the
 * cross-workspace anchor for "this is you, and here is your settings".
 *
 * Layout contract:
 *   - The parent wrapper is `flex flex-col` with `bg-skin-primary` and contains:
 *       (a) the col 1 + col 2 row (`flex-1`)
 *       (b) this bar as the second flex child (auto height).
 *   - Because (a) takes the remaining height, the bar is naturally pinned
 *     to the bottom of the combined left-side chrome without needing
 *     `mt-auto` on the aside.
 *   - The bar itself is `bg-transparent` so it sits on the wrapper's
 *     `bg-skin-primary` and visually merges with the chrome above it —
 *     the only separator is the hairline `border-skin-rule`. This is
 *     the alook / Discord pattern: chrome and bar are one continuous
 *     primary block, the card is defined by its rounded corners and
 *     border, not by a different background color. Avoids a
 *     "color-stack" fault at the bottom of the sidebar.
 */
export function GlobalAccountBar() {
  const { user } = useAuth();

  return (
    <div className="px-2 pb-2">
      <div className="flex items-center gap-2.5 rounded-2xl border border-skin-rule bg-transparent px-2.5 py-2">
        {user ? (
          <UserAvatar
            userId={user.id}
            name={user.display_name}
            avatarUrl={user.avatar_url}
            size="md"
            // Round + remove brutalist chrome for the editorial floating bar.
            className="rounded-full border-0 shadow-none"
          />
        ) : (
          <div
            aria-hidden
            className="h-8 w-8 shrink-0 rounded-full border border-skin-rule bg-skin-primary"
          />
        )}
        <div className="min-w-0 flex-1">
          <p className="truncate font-heading text-sm font-medium leading-tight text-skin-ink">
            {user?.display_name ?? 'Solo'}
          </p>
          <p className="truncate font-mono text-[10px] leading-tight text-skin-subtle-text">
            {user?.email ?? ''}
          </p>
        </div>
        <Link
          href="/settings"
          aria-label={t('personalSettings')}
          title={t('personalSettings')}
          className={cn(
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-skin-subtle-text transition-colors',
            'hover:bg-skin-canvas hover:text-skin-ink',
          )}
        >
          <Settings className="h-4 w-4" aria-hidden="true" />
        </Link>
      </div>
    </div>
  );
}
