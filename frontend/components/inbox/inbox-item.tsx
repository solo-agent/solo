// ============================================================================
// InboxItem — single inbox entry (v1.5)
// Neo-brutalist style matching channel message items.
// ============================================================================

'use client';

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { Hash, AtSign, MessageCircle, Check } from 'lucide-react';
import { relativeTime } from '@/lib/utils/time';
import type { InboxItem as InboxItemType } from '@/lib/types';

// ---- Type label config ----

const typeConfig: Record<InboxItemType['type'], { icon: React.ReactNode; label: string; bgClass: string }> = {
  thread_reply: {
    icon: <Hash className="h-3 w-3" />,
    label: '话题回复',
    bgClass: 'bg-brutal-lavender text-black border-2 border-black',
  },
  dm: {
    icon: <MessageCircle className="h-3 w-3" />,
    label: '私信',
    bgClass: 'bg-brutal-cyan text-black border-2 border-black',
  },
  mention: {
    icon: <AtSign className="h-3 w-3" />,
    label: '@提及',
    bgClass: 'bg-brutal-pink text-black border-2 border-black',
  },
};

// ---- Props ----

interface InboxItemProps {
  item: InboxItemType;
  onClick: (item: InboxItemType) => void;
  onDismiss: (messageId: string) => void;
}

// ---- Component ----

export function InboxItem({ item, onClick, onDismiss }: InboxItemProps) {
  const config = typeConfig[item.type];
  const [isHovered, setIsHovered] = useState(false);

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
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      className={cn(
        'group relative flex gap-3 px-6 py-2.5 cursor-pointer transition-colors border-b border-black/10',
        'hover:bg-brutal-stone/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-inset',
      )}
    >
      {/* Done button — visible on hover */}
      <div className="flex-shrink-0 mt-0.5">
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDismiss(item.message_id);
          }}
          className={cn(
            'flex h-5 w-5 items-center justify-center border-2 border-black transition-all',
            isHovered
              ? 'opacity-100 bg-brutal-lime hover:bg-brutal-pink'
              : 'opacity-0 bg-transparent',
          )}
          aria-label="完成"
          title="Done"
        >
          <Check className="h-3 w-3" />
        </button>
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        {/* Sender name + time */}
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1.5 min-w-0">
            <span className="font-heading text-xs font-bold text-foreground truncate">
              {item.sender_name}
            </span>
          </div>
          <span className="flex-shrink-0 text-[10px] tabular-nums text-muted-foreground font-mono">
            {relativeTime(item.created_at)}
          </span>
        </div>

        {/* Context line */}
        <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground truncate font-body">
          {item.type === 'dm'
            ? `与 ${item.sender_name} 的私信`
            : item.type === 'thread_reply'
              ? `在 #${item.channel_name || 'unknown'} 中回复`
              : `在 #${item.channel_name || 'unknown'} 中提及你`
          }
        </p>

        {/* Content preview */}
        <p className="mt-0.5 text-[12px] leading-snug text-foreground/80 line-clamp-2 font-body">
          {item.content_preview}
        </p>

        {/* Type badge */}
        <div className="mt-1.5 flex items-center gap-1.5">
          <span className={cn('inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] font-heading font-bold', config.bgClass)}>
            {config.icon}
            {config.label}
          </span>
        </div>
      </div>
    </div>
  );
}
