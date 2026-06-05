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
  isOpen: boolean;
  onClick: () => void;
}

export function InboxBadge({ unreadCount, isOpen, onClick }: InboxBadgeProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex items-center justify-between w-full px-2 py-2 text-sm font-body',
        'border-b-2 border-sidebar-border transition-colors',
        isOpen
          ? 'bg-brutal-pink text-black'
          : 'text-black hover:bg-brutal-pink/40',
      )}
      aria-label={`收件箱${unreadCount > 0 ? `，${unreadCount} 条未读` : ''}`}
      aria-expanded={isOpen}
    >
      <span className="flex items-center gap-2">
        <Mail className="h-4 w-4" />
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
