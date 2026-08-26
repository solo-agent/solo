// ============================================================================
// InboxBadge — red circle/dot badge with white unread count (v1.5)
// - Shows total unread count from useInboxUnread
// - Hidden when count is 0
// - Click toggles inbox panel open/close
// ============================================================================

'use client';

import { Mail } from 'lucide-react';
import { selectableRowClass, selectableRowIconClass } from '@/components/ui/selectable-row';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface InboxBadgeProps {
  unreadCount: number;
  isSelected: boolean;
  onClick: () => void;
}

export function InboxBadge({ unreadCount, isSelected, onClick }: InboxBadgeProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={selectableRowClass(
        isSelected,
        cn('w-full text-left', isSelected ? 'bg-white' : 'hover:bg-white/50'),
      )}
      aria-label={t('inboxAriaLabel', { n: unreadCount })}
      aria-current={isSelected ? 'page' : undefined}
    >
      <span className={selectableRowIconClass('bg-white')}>
        <Mail className="h-4 w-4" />
      </span>
      <span className="truncate font-body">{t('sidebarInbox')}</span>
      {unreadCount > 0 && (
        <span
          className="ml-auto flex h-5 min-w-5 items-center justify-center border border-black/20 bg-brutal-accent-light px-1.5 font-mono text-[10px] font-bold"
          aria-label={t('inboxUnread', { n: unreadCount })}
        >
          {unreadCount > 99 ? '99+' : unreadCount}
        </span>
      )}
    </button>
  );
}
