// ============================================================================
// InboxBadge — red circle/dot badge with white unread count (v1.5)
// - Shows total unread count from useInboxUnread
// - Hidden when count is 0
// - Click toggles inbox panel open/close
// ============================================================================

'use client';

import { Mail } from 'lucide-react';
import { cn } from '@/lib/utils';

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
      className={cn(
        'flex items-center justify-between w-full px-3 pt-4 pb-0 text-sm font-body transition-colors',
        isSelected
          ? 'bg-brutal-pink text-black'
          : 'text-muted-foreground hover:bg-brutal-pink/40',
      )}
      aria-label={`收件箱${unreadCount > 0 ? `，${unreadCount} 条未读` : ''}`}
    >
      <span className="flex items-center gap-1.5">
        <Mail className="h-3.5 w-3.5" />
        <span className="font-heading text-xs font-bold uppercase tracking-wider">
          Inbox
        </span>
      </span>
      {unreadCount > 0 && (
        <span
          className="flex h-5 min-w-[20px] items-center justify-center border-2 border-black bg-brutal-red px-1.5 font-mono text-[11px] font-bold text-white"
          aria-label={`${unreadCount} 条未读`}
        >
          {unreadCount > 99 ? '99+' : unreadCount}
        </span>
      )}
    </button>
  );
}
