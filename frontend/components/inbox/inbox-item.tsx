'use client';

import { cn } from '@/lib/utils';
import { Hash, AtSign, MessageCircle } from 'lucide-react';
import { relativeTime } from '@/lib/utils/time';
import type { InboxItem as InboxItemType } from '@/lib/types';

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

interface InboxItemProps {
  item: InboxItemType;
  onClick: (item: InboxItemType) => void;
}

export function InboxItem({ item, onClick }: InboxItemProps) {
  const config = typeConfig[item.type];

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
        'group relative flex gap-3 px-6 py-2.5 cursor-pointer transition-colors border-b border-black/10',
        'hover:bg-brutal-stone/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-inset',
        item.is_unread && 'border-l-[3px] border-l-brutal-pink bg-brutal-pink-light/30',
      )}
    >
      {/* Unread dot */}
      <div className="flex-shrink-0 mt-1.5">
        {item.is_unread ? (
          <span className="block h-2.5 w-2.5 rounded-full bg-brutal-pink border-2 border-black" />
        ) : (
          <span className="block h-2.5 w-2.5" />
        )}
      </div>

      <div className="flex-1 min-w-0">
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

        <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground truncate font-body">
          {item.type === 'dm'
            ? `与 ${item.sender_name} 的私信`
            : item.type === 'thread_reply'
              ? `在 #${item.channel_name || 'unknown'} 中回复`
              : `在 #${item.channel_name || 'unknown'} 中提及你`
          }
        </p>

        <p className="mt-0.5 text-[12px] leading-snug text-foreground/80 line-clamp-2 font-body">
          {item.content_preview}
        </p>

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
