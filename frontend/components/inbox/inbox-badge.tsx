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
        'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading transition-all border-2',
        isSelected
          ? 'bg-brutal-primary text-black border-black shadow-brutal-sm'
          : 'text-muted-foreground border-transparent hover:border-black',
      )}
      aria-label={`收件箱${unreadCount > 0 ? `，${unreadCount} 条未读` : ''}`}
    >
      <Mail className="h-3.5 w-3.5" />
      <span>Inbox</span>
      {unreadCount > 0 && (
        <span
          className="ml-auto flex h-5 min-w-[20px] items-center justify-center border-2 border-black bg-brutal-danger px-1.5 font-mono text-[11px] font-bold text-white"
          aria-label={`${unreadCount} 条未读`}
        >
          {unreadCount > 99 ? '99+' : unreadCount}
        </span>
      )}
    </button>
  );
}
