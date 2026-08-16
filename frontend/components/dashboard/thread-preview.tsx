'use client';

import { useEffect, useState } from 'react';
import { MessageSquare } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { UserAvatar } from '@/components/ui/user-avatar';
import { apiClient } from '@/lib/api-client';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface ThreadReply {
  id: string;
  sender_type: string;
  sender_id: string;
  sender_name?: string;
  sender_avatar?: string | null;
  content: string;
}

interface ThreadPreviewProps {
  channelId: string;
  messageId: string;
  replyCount: number;
  hasUnread?: boolean;
  onOpen: () => void;
}

export function ThreadPreview({ channelId, messageId, replyCount, hasUnread, onOpen }: ThreadPreviewProps) {
  const [replies, setReplies] = useState<ThreadReply[]>([]);

  useEffect(() => {
    let active = true;
    apiClient.get<{ messages: ThreadReply[] }>(
      `/api/v1/channels/${channelId}/messages/${messageId}/thread`,
      { limit: '3' },
    ).then((response) => {
      if (active) setReplies(response.messages);
    }).catch(() => {
      if (active) setReplies([]);
    });
    return () => { active = false; };
  }, [channelId, messageId, replyCount]);

  return (
    <div data-thread-preview className="mt-2 ml-1 border-l-2 border-brutal-muted pl-3">
      <div className="space-y-1.5">
        {replies.map((reply) => {
          const name = reply.sender_name || (reply.sender_type === 'agent' ? t('agent') : t('user'));
          return (
            <div key={reply.id} data-thread-preview-reply className="flex min-w-0 items-center gap-2">
              {reply.sender_type === 'agent' ? (
                <PixelAvatar agentId={reply.sender_id} avatarUrl={reply.sender_avatar} size="sm" />
              ) : (
                <UserAvatar userId={reply.sender_id} name={name} avatarUrl={reply.sender_avatar} size="sm" />
              )}
              <span className="flex-shrink-0 font-heading text-xs font-bold">{name}</span>
              <span className="min-w-0 truncate text-sm text-muted-foreground">{reply.content}</span>
            </div>
          );
        })}
      </div>
      <button
        type="button"
        onClick={(event) => { event.stopPropagation(); onOpen(); }}
        className={cn(
          'mt-2 inline-flex items-center rounded-full border px-2 py-0.5 font-mono text-[11px] font-bold transition-colors',
          hasUnread
            ? 'border-brutal-muted bg-brutal-muted-light text-foreground'
            : 'border-brutal-muted bg-brutal-cream text-muted-foreground hover:bg-brutal-muted-light hover:text-foreground',
        )}
      >
        <MessageSquare className="mr-1 h-3 w-3" />
        {t('threadReplies', { n: replyCount })}
      </button>
    </div>
  );
}
