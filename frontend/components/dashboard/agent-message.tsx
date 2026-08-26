// ============================================================================
// AgentMessage — brutalist Agent message with Markdown rendering
// - Pink left border (3px, #fe7da8)
// - Cream background
// - Bot icon + badge-brutal "Agent" label
// - Markdown code blocks: black bg, Space Mono, green text
// ============================================================================

'use client';

import { useState } from 'react';
import { Copy, ListChecks, Loader2, MessageSquare, Pin, PinOff } from 'lucide-react';
import type { AgentDetailTarget, Message } from '@/lib/types';
import { cn } from '@/lib/utils';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';
import { formatMessageTime, formatMessageTimestamp } from '@/lib/utils/time';
import { MessageMarkdown } from './message-markdown';
import { ThreadPreview } from './thread-preview';
import {
  MessageReactionChips,
  MessageReactionPicker,
  useMessageReactions,
} from './message-reactions';
import { MessageSelectMark } from './message-share';

interface AgentMessageProps {
  message: Message;
  isGrouped?: boolean;
  onReply?: (message: Message) => void;
  /** Lowercased display_names that may receive highlight. Empty = no @mentions highlighted. */
  validNames?: string[];
  isHighlighted?: boolean;
  onOpenArtifactReference?: (ref: string) => void;
  onAgentClick?: (agent: AgentDetailTarget) => void;
  onPin?: (message: Message) => void | Promise<void>;
  pinned?: boolean;
  onCopy?: (message: Message) => void;
  onSelect?: (message: Message) => void;
  selectionMode?: boolean;
  selected?: boolean;
}

export function AgentMessage({ message, isGrouped, onReply, validNames = [], isHighlighted, onOpenArtifactReference, onAgentClick, onPin, pinned, onCopy, onSelect, selectionMode, selected }: AgentMessageProps) {
  const time = formatMessageTimestamp(message.created_at);
  const compactTime = formatMessageTime(message.created_at);
  const [isTogglingPin, setIsTogglingPin] = useState(false);
  const [reactionOpen, setReactionOpen] = useState(false);
  const reactionState = useMessageReactions(message.id, message.reactions);

  const hasUnreadThread = message.has_unread_thread === true && (message.reply_count ?? 0) > 0;
  return (
    <div
      data-message-id={message.id}
      data-message-grouped={isGrouped ? 'true' : 'false'}
      className={cn(
        'group relative flex gap-3 px-6 agent-message',
        isGrouped ? 'py-1' : 'pt-3 pb-1.5',
        isHighlighted && 'bg-brutal-primary-light ring-2 ring-brutal-accent',
        selected && 'bg-brutal-primary-light/60 ring-2 ring-brutal-accent',
        selectionMode && 'cursor-pointer',
      )}
      role="listitem"
      onClick={selectionMode ? () => onSelect?.(message) : undefined}
      onKeyDown={selectionMode ? (event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        event.preventDefault();
        onSelect?.(message);
      } : undefined}
      tabIndex={selectionMode ? 0 : undefined}
    >
      {selectionMode && <MessageSelectMark selected={Boolean(selected)} />}
      {isGrouped ? (
        <div className="mt-0.5 w-8 flex-shrink-0 text-center">
          <time
            dateTime={message.created_at}
            className="font-mono text-[9px] text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
          >
            {compactTime}
          </time>
        </div>
      ) : (
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
      )}

      <div className="min-w-0 flex-1">
        {isGrouped && <span className="sr-only">{message.display_name}, {compactTime}</span>}
        <div className={cn('mb-1.5 items-baseline gap-2', isGrouped ? 'hidden' : 'flex')}>
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
        <MessageReactionChips {...reactionState} />

        {/* Thread preview */}
        {(message.reply_count ?? 0) > 0 && onReply && (
          <ThreadPreview
            channelId={message.channel_id}
            messageId={message.id}
            replyCount={message.reply_count ?? 0}
            hasUnread={hasUnreadThread}
            onOpen={() => onReply(message)}
          />
        )}
      </div>

      {/* Hover reply button */}
      <div className={cn(
        'absolute right-3 top-2 flex items-center gap-1 transition-[opacity,transform] duration-200',
        reactionOpen
          ? 'opacity-100 translate-x-0'
          : 'opacity-0 translate-x-2 group-hover:opacity-100 group-hover:translate-x-0',
      )}>
        <MessageReactionPicker
          open={reactionOpen}
          setOpen={setReactionOpen}
          isSaving={reactionState.isSaving}
          toggleReaction={reactionState.toggleReaction}
        />
        {onCopy && <button
          data-message-copy
          type="button"
          onClick={(e) => { e.stopPropagation(); onCopy(message); }}
          className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
          aria-label={t('copyMessage')}
          title={t('copyMessage')}
        >
          <Copy className="h-3.5 w-3.5" />
        </button>}
        {onSelect && <button
          data-message-select
          type="button"
          onClick={(e) => { e.stopPropagation(); onSelect(message); }}
          className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
          aria-label={t('selectMessage')}
          title={t('selectMessage')}
        >
          <ListChecks className="h-3.5 w-3.5" />
        </button>}
        {onPin && <button
          type="button"
          onClick={async (e) => {
            e.stopPropagation();
            if (isTogglingPin) return;
            setIsTogglingPin(true);
            try {
              await onPin(message);
            } finally {
              setIsTogglingPin(false);
            }
          }}
          disabled={isTogglingPin}
          aria-pressed={Boolean(pinned)}
          className={`btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0 transition-transform active:translate-x-[2px] active:translate-y-[2px] active:shadow-none disabled:cursor-wait disabled:opacity-70 ${pinned ? 'bg-brutal-warning' : ''}`}
          aria-label={pinned ? t('channelUnpin') : t('channelPin')}
          title={pinned ? t('channelUnpin') : t('channelPin')}
        >
          {isTogglingPin ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : pinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
        </button>}
        {onReply && <button
          type="button"
          onClick={(e) => { e.stopPropagation(); onReply(message); }}
          className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
          aria-label={t('replyToMessage', { name: message.display_name })}
          title="Reply"
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </button>}
      </div>
    </div>
  );
}
