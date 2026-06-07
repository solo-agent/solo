'use client';

import { cn } from '@/lib/utils';
import { Hash, AtSign, MessageCircle } from 'lucide-react';
import { relativeTime } from '@/lib/utils/time';
import type { InboxItem as InboxItemType } from '@/lib/types';
import { Tag } from '@/components/ui/tag';

const typeConfig: Record<InboxItemType['type'], { icon: React.ReactNode; label: string; variant: 'agent' | 'type' | 'status' }> = {
  thread_reply: {
    icon: <Hash className="h-3 w-3" />,
    label: '话题回复',
    variant: 'agent',
  },
  dm: {
    icon: <MessageCircle className="h-3 w-3" />,
    label: '私信',
    variant: 'type',
  },
  mention: {
    icon: <AtSign className="h-3 w-3" />,
    label: '@提及',
    variant: 'status',
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
        'group relative flex gap-3 px-6 py-2.5 cursor-pointer transition-all border-b-2 border-black',
        'hover:bg-brutal-accent-light hover:shadow-brutal-sm hover:-translate-y-px',
        'active:translate-y-0.5 active:shadow-none',
        item.is_unread && 'border-l-[3px] border-l-brutal-accent bg-brutal-primary-light',
      )}
    >
      {/* Unread dot */}
      <div className="flex-shrink-0 mt-1.5">
        {item.is_unread ? (
          <span className="block h-2.5 w-2.5 rounded-full bg-brutal-primary border-2 border-black" />
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
          <span className="flex-shrink-0 text-xs tabular-nums text-muted-foreground">
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
          <Tag variant={config.variant}>
            {config.icon}
            {config.label}
          </Tag>
        </div>
      </div>
    </div>
  );
}
