// ============================================================================
// InboxItem — single inbox entry (v1.5)
// - Shows sender avatar, name, content preview (first 50 chars), time, type label
// - Type labels: "Thread in #channelname" | "DM · username" | "@Mention in #channelname"
// - Unread items have blue dot indicator on left
// - onClick: navigates to appropriate view
// ============================================================================

'use client';

import { cn } from '@/lib/utils';
import { Hash, AtSign, MessageCircle } from 'lucide-react';
import { relativeTime } from '@/lib/utils/time';
import type { InboxItem as InboxItemType } from '@/lib/types';

// ---- Type label config ----

function getTypeLabel(item: InboxItemType): { icon: React.ReactNode; text: string } {
  switch (item.type) {
    case 'thread_reply':
      return {
        icon: <Hash className="h-3 w-3" />,
        text: `Thread in #${item.channel_name || 'unknown'}`,
      };
    case 'dm':
      return {
        icon: <MessageCircle className="h-3 w-3" />,
        text: `DM · ${item.sender_name}`,
      };
    case 'mention':
      return {
        icon: <AtSign className="h-3 w-3" />,
        text: `@Mention in #${item.channel_name || 'unknown'}`,
      };
  }
}

// ---- Props ----

interface InboxItemProps {
  item: InboxItemType;
  onClick: (item: InboxItemType) => void;
}

// ---- Component ----

export function InboxItem({ item, onClick }: InboxItemProps) {
  const typeLabel = getTypeLabel(item);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onClick(item)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick(item);
        }
      }}
      className={cn(
        'flex items-start gap-2.5 px-2 py-2.5 cursor-pointer transition-colors',
        'hover:bg-brutal-pink/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-inset',
        item.is_unread ? 'bg-brutal-cyan-light/50' : '',
      )}
    >
      {/* Unread dot */}
      <div className="flex-shrink-0 mt-1.5">
        {item.is_unread ? (
          <span className="block h-2 w-2 rounded-full bg-brutal-cyan border border-black" />
        ) : (
          <span className="block h-2 w-2" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        {/* Sender name + time */}
        <div className="flex items-center justify-between gap-2">
          <span className="font-heading text-xs font-bold text-foreground truncate">
            {item.sender_name}
          </span>
          <span className="flex-shrink-0 text-[10px] tabular-nums text-muted-foreground">
            {relativeTime(item.created_at)}
          </span>
        </div>

        {/* Content preview (truncated to 50 chars by backend) */}
        <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground line-clamp-2">
          {item.content_preview}
        </p>

        {/* Type label */}
        <div className="mt-1 flex items-center gap-1 text-[10px] text-muted-foreground">
          {typeLabel.icon}
          <span className="truncate">{typeLabel.text}</span>
        </div>
      </div>
    </div>
  );
}
