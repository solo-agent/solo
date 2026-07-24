// ============================================================================
// AgentMessage — brutalist Agent message with Markdown rendering
// - Pink left border (3px, #fe7da8)
// - Cream background
// - Bot icon + badge-brutal "Agent" label
// - Markdown code blocks: black bg, Space Mono, green text
// ============================================================================

'use client';

import { MessageSquare } from 'lucide-react';
import type { AgentDetailTarget, Message } from '@/lib/types';
import { cn } from '@/lib/utils';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';
import { formatMessageTimestamp } from '@/lib/utils/time';
import { MessageMarkdown } from './message-markdown';

interface AgentMessageProps {
  message: Message;
  onReply?: (message: Message) => void;
  /** Lowercased display_names that may receive highlight. Empty = no @mentions highlighted. */
  validNames?: string[];
  isHighlighted?: boolean;
  onOpenArtifactReference?: (ref: string) => void;
  onAgentClick?: (agent: AgentDetailTarget) => void;
}

export function AgentMessage({ message, onReply, validNames = [], isHighlighted, onOpenArtifactReference, onAgentClick }: AgentMessageProps) {
  const time = formatMessageTimestamp(message.created_at);

  const hasUnreadThread = message.has_unread_thread === true && (message.reply_count ?? 0) > 0;
  return (
    <div
      data-message-id={message.id}
      className={cn(
        'group relative flex gap-3 px-6 py-2.5 agent-message border-l-brutal-primary border-b border-brutal-muted',
        isHighlighted && 'bg-brutal-info-light ring-2 ring-black',
      )}
      role="listitem"
    >
      <PixelAvatar
        agentId={message.user_id}
        avatarUrl={message.avatar_url}
        size="md"
        className="mt-0.5 flex-shrink-0"
        onClick={onAgentClick ? () => onAgentClick?.({
          id: message.user_id,
          name: message.display_name,
          is_active: message.sender_active,
        }) : undefined}
        ariaLabel={t('viewAgentDetail', { name: message.display_name })}
      />

      <div className="min-w-0 flex-1">
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-foreground">
            {message.display_name}
          </span>
          {message.sender_active === false ? (
            <span className="badge-brutal bg-brutal-muted text-black">
              {t('deleted')}
            </span>
          ) : (
            <span className="badge-brutal bg-brutal-primary text-black">
              {t('agent')}
            </span>
          )}
          <time dateTime={message.created_at} className="font-mono text-[11px] text-muted-foreground">
            {time}
          </time>
        </div>
        <MessageMarkdown
          content={message.content}
          validNames={validNames}
          onOpenArtifactReference={onOpenArtifactReference}
        />

        {/* Thread reply count — brutalist badge */}
        {(message.reply_count ?? 0) > 0 && onReply && (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onReply(message); }}
            className={cn(
              'mt-2 badge-brutal cursor-pointer transition-all',
              hasUnreadThread
                ? 'bg-brutal-primary text-black border-brutal-primary'
                : 'bg-white text-black hover:bg-brutal-primary hover:-translate-y-px hover:shadow-brutal',
            )}
          >
            <MessageSquare className="mr-1 h-3 w-3" />
            <span>{t('threadReplies', { n: message.reply_count ?? 0 })}</span>
          </button>
        )}
      </div>

      {/* Hover reply button */}
      {onReply && (
        <div className="absolute right-3 top-2 flex items-center gap-1
                        opacity-0 group-hover:opacity-100
                        translate-x-2 group-hover:translate-x-0
                        transition-all duration-200">
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onReply(message); }}
            className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
            aria-label={t('replyToMessage', { name: message.display_name })}
            title="Reply"
          >
            <MessageSquare className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}
