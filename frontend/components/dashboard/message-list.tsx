// ============================================================================
// MessageList — renders message items with brutalist styling
// - User messages: white bg + 2px border + shadow + Space Mono timestamp
// - Agent messages: pink left border + cream bg + Bot icon (via AgentMessage)
// - Streaming messages: pink cursor + pink left border (via StreamingMessage)
// - Hover actions: reply / edit / delete buttons
// - Edit mode: inline textarea + save/cancel
// - Delete confirmation dialog
// ============================================================================

'use client';

import {
  Fragment,
  useEffect,
  useRef,
  useState,
  useLayoutEffect,
  memo,
  useCallback,
} from 'react';
import Link from 'next/link';
import {
  AlertCircle,
  RefreshCw,
  ChevronDown,
  Loader2,
  MessageSquare,
  SquareCheckBig,
  ArrowUpRight,
  CheckCircle2,
  UserRoundCheck,
  FolderSync,
  GitBranch,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { buildValidNames } from '@/lib/utils/highlight';
import { Avatar } from '@/components/ui/avatar';
import { UserAvatar } from '@/components/ui/user-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { Button } from '@/components/ui/button';
import { useToast } from '@/components/ui/toast';
import { motionScrollBehavior } from '@/lib/motion';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { AgentMessage } from './agent-message';
import { MessageMarkdown } from './message-markdown';
import { StreamingMessage } from './streaming-message';
import { MessageAttachments } from './message-attachments';
import {
  MessageReactionChips,
  MessageReactionPicker,
  useMessageReactions,
} from './message-reactions';
import { canGroupMessages, MessageDateSeparator } from './message-layout';
import { ThreadPreview } from './thread-preview';
import {
  copyMessageText,
  MessageSelectionToolbar,
  MessageSelectMark,
  ShareMessagesDialog,
  type ShareableMessage,
} from './message-share';
import type { AgentDetailTarget, Channel, ChannelMember, Message } from '@/lib/types';
import { sanitizeHtml } from '@/lib/sanitize';
import { t, type TranslationKey } from '@/lib/i18n';
import {
  formatMessageTime,
  formatMessageTimestamp,
  messageDateKey,
} from '@/lib/utils/time';
import { MessageReuseMenu } from './message-reuse-menu';
// Agent activity now lives in team/observability surfaces, not inline typing badges.
interface MessageListProps {
  messages: Message[];
  isLoading: boolean;
  error: string | null;
  onRetry: (messageId: string, content: string) => void;
  onCancel?: (messageId: string) => void;
  onReply?: (message: Message) => void;
  onEdit?: (messageId: string, content: string) => void;
  onDelete?: (messageId: string) => void;
  onAsTask?: (message: Message) => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMoreError: string | null;
  onLoadMore: () => void;
  /** SOLO-237-F: message ID to scroll to (cleared after scroll) */
  scrollToMessageId?: string;
  /** Re-trigger key so clicking the same search result twice still scrolls */
  scrollKey?: number;
  /** Channel members for @mention whitelist in agent messages. */
  members?: ChannelMember[];
  onOpenArtifactReference?: (ref: string) => void;
  onAgentClick?: (agent: AgentDetailTarget) => void;
  onPin?: (message: Message) => void | Promise<void>;
  pinnedMessageIds?: Set<string>;
  contextLabel?: string;
  forwardChannels?: Channel[];
  onFavorite?: (message: Message) => Promise<void>;
  onForward?: (message: Message, targetChannelId: string) => Promise<void>;
  onBranch?: (message: Message, title: string) => Promise<void>;
}

// ---- Task header config (SOLO-225-F) ----

const TASK_HEADER_CONFIG: Record<string, { label: string; accentClass: string; bgClass: string; badgeClass: string; lightClass: string }> = {
  todo: {
    label: t('statusTodo'),
    accentClass: 'border-l-brutal-warning',
    bgClass: 'bg-brutal-warning-light/20',
    badgeClass: 'bg-brutal-warning text-black border-2 border-black',
    lightClass: 'bg-brutal-warning-light',
  },
  in_progress: {
    label: t('statusInProgress'),
    accentClass: 'border-l-brutal-info',
    bgClass: 'bg-brutal-primary-light/35',
    badgeClass: 'bg-brutal-info text-black border-2 border-black',
    lightClass: 'bg-brutal-info-light',
  },
  in_review: {
    label: t('statusPendingReview'),
    accentClass: 'border-l-brutal-violet',
    bgClass: 'bg-brutal-violet-light/20',
    badgeClass: 'bg-brutal-violet text-black border-2 border-black',
    lightClass: 'bg-brutal-violet-light',
  },
  done: {
    label: t('statusDone'),
    accentClass: 'border-l-brutal-success',
    bgClass: 'bg-brutal-success-light/20',
    badgeClass: 'bg-brutal-success text-black border-2 border-black',
    lightClass: 'bg-brutal-success-light',
  },
};

// ---- Single message (memo'd to reduce re-renders) ----

interface MessageItemProps {
  message: Message;
  isGrouped?: boolean;
  isHighlighted?: boolean;
  onRetry: (id: string, content: string) => void;
  onCancel?: (id: string) => void;
  onReply?: (message: Message) => void;
  onEdit?: (id: string, content: string) => void;
  onDelete?: (id: string) => void;
  onAsTask?: (message: Message) => void;
  onOpenArtifactReference?: (ref: string) => void;
  onPin?: (message: Message) => void | Promise<void>;
  pinned?: boolean;
  onCopy?: (message: Message) => void;
  onSelect?: (message: Message) => void;
  selectionMode?: boolean;
  selected?: boolean;
  onFavorite?: (message: Message) => void;
  onForward?: (message: Message) => void;
  onBranch?: (message: Message) => void;
}

const MessageItem = memo(function MessageItem({
  message,
  isGrouped,
  isHighlighted,
  onRetry,
  onCancel,
  onReply,
  onEdit,
  onDelete,
  onAsTask,
  onOpenArtifactReference,
  onPin,
  pinned,
  onCopy,
  onSelect,
  selectionMode,
  selected,
  onFavorite,
  onForward,
  onBranch,
}: MessageItemProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState(message.content || '');
  const [isSaving, setIsSaving] = useState(false);
  const [isTogglingPin, setIsTogglingPin] = useState(false);
  const [isHovered, setIsHovered] = useState(false);
  const [reactionOpen, setReactionOpen] = useState(false);
  const editRef = useRef<HTMLTextAreaElement>(null);
  const reactionState = useMessageReactions(message.id, message.reactions);

  const isFailed = message.status === 'failed';
  const isSending = message.status === 'sending';

  // SOLO-225-F: task message detection
  const taskStatus = message.task_status as string | undefined;
  const isTaskMessage = message.task_number != null && taskStatus != null;
  const isThinkingHandoff = message.content_type === 'thinking_handoff'
    || (message.sender_type === 'system' && message.content.startsWith('Handoff returned from '));
  const headerConfig = isTaskMessage && taskStatus ? TASK_HEADER_CONFIG[taskStatus] : null;

  // P25-08-F: unread thread dot condition
  const hasUnreadThread = message.has_unread_thread === true && (message.reply_count ?? 0) > 0;

  // Reset edit content when message content changes externally
  useEffect(() => {
    if (!isEditing) {
      setEditContent(message.content || '');
    }
  }, [message.content, isEditing]);

  // Focus the edit textarea when entering edit mode
  useEffect(() => {
    if (isEditing && editRef.current) {
      editRef.current.focus();
      editRef.current.setSelectionRange(editContent.length, editContent.length);
    }
  }, [isEditing, editContent.length]);

  // Keyboard shortcuts — active when mouse is hovering over this message
  useEffect(() => {
    if (!isHovered) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept when user is typing in an input or textarea
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement ||
        e.target instanceof HTMLSelectElement
      ) {
        return;
      }

      // E — enter edit mode
      if (e.key === 'e' && !e.ctrlKey && !e.metaKey && onEdit && !isFailed && !isSending) {
        e.preventDefault();
        setEditContent(message.content || '');
        setIsEditing(true);
        return;
      }

      // Delete / Backspace — delete with confirmation
      if ((e.key === 'Delete' || e.key === 'Backspace') && onDelete && !isEditing && !isFailed && !isSending) {
        e.preventDefault();
        onDelete(message.id);
        return;
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isHovered, onEdit, onDelete, message.content, message.id, isEditing, isFailed, isSending]);

  const time = formatMessageTimestamp(message.created_at);
  const compactTime = formatMessageTime(message.created_at);

  const handleSaveEdit = useCallback(async () => {
    if (isSaving) return;
    const trimmed = editContent.trim();
    if (!trimmed || trimmed === message.content) {
      setIsEditing(false);
      return;
    }
    setIsSaving(true);
    try {
      await onEdit?.(message.id, trimmed);
      setIsEditing(false);
    } finally {
      setIsSaving(false);
    }
  }, [isSaving, editContent, message.id, message.content, onEdit]);

  const handleCancelEdit = useCallback(() => {
    setEditContent(message.content || '');
    setIsEditing(false);
  }, [message.content]);

  const handleEditKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSaveEdit();
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        handleCancelEdit();
      }
    },
    [handleSaveEdit, handleCancelEdit],
  );

  const projectEvent = typeof message.metadata?.event === 'string' ? message.metadata.event : '';
  if (projectEvent.startsWith('channel.project.')) {
    const actor = typeof message.metadata?.actor_name === 'string' ? message.metadata.actor_name : t('unknown');
    const source = typeof message.metadata?.source === 'string' && message.metadata.source
      ? message.metadata.source : t('channelProjectUnnamed');
    const key: TranslationKey = projectEvent === 'channel.project.unlinked'
      ? 'channelProjectEventUnlinked'
      : projectEvent === 'channel.project.folder_changed'
        ? 'channelProjectEventFolderChanged'
        : projectEvent === 'channel.project.folder_unlinked'
          ? 'channelProjectEventFolderUnlinked'
          : 'channelProjectEventChanged';
    return (
      <div data-message-id={message.id} className="px-6 py-2" role="listitem">
        <div className="flex items-center gap-2 rounded-lg border border-brutal-border bg-brutal-cream/70 px-3 py-2 font-body text-sm text-brutal-text-muted">
          <FolderSync className="h-4 w-4 shrink-0" />
          <span>{t(key, { name: actor, project: source })}</span>
        </div>
      </div>
    );
  }

  if (message.content_type === 'channel_created') {
    const channelId = typeof message.metadata?.channel_id === 'string' ? message.metadata.channel_id : '';
    const channelName = typeof message.metadata?.channel_name === 'string' ? message.metadata.channel_name : t('lucyNewChannel');
    const templateId = typeof message.metadata?.template_id === 'string' ? message.metadata.template_id : '';
    const memberCount = typeof message.metadata?.member_count === 'number' ? message.metadata.member_count : 0;
    return (
      <div data-message-id={message.id} className="px-5 py-3" role="listitem">
        <div className="border-4 border-black bg-brutal-cream p-4 shadow-brutal">
          <div className="flex items-start gap-3">
            <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-success-light shadow-brutal-sm">
              <CheckCircle2 className="h-5 w-5" />
            </span>
            <div className="min-w-0 flex-1">
              <div className="font-body text-[11px] font-semibold uppercase tracking-wider text-black/55">{t('lucyChannelReady')}</div>
              <h3 className="truncate font-heading text-lg font-black"># {channelName}</h3>
              <p className="mt-1 font-body text-sm text-black/65">{message.content}</p>
              <div className="mt-3 flex flex-wrap gap-2">
                {templateId && (
                  <span className="border-2 border-black bg-white px-2 py-1 font-mono text-[10px] font-bold uppercase">
                    {templateId}
                  </span>
                )}
                <span className="border-2 border-black bg-white px-2 py-1 font-mono text-[10px] font-bold uppercase">
                  {t('lucyAgentCount', { n: memberCount })}
                </span>
              </div>
            </div>
          </div>
          {channelId && (
            <Link
              href={`/dashboard?channel=${encodeURIComponent(channelId)}`}
              className="mt-4 flex w-full items-center justify-between border-2 border-black bg-brutal-primary px-3 py-2 font-heading text-sm font-black shadow-brutal-sm hover:-translate-y-px"
            >
              {t('lucyOpenChannel')}
              <ArrowUpRight className="h-4 w-4" />
            </Link>
          )}
        </div>
      </div>
    );
  }

  return (
    <div
      data-message-id={message.id}
      data-message-grouped={isGrouped ? 'true' : 'false'}
      className={cn(
        'group relative flex gap-3 px-6 transition-colors',
        isGrouped ? 'py-1' : 'pt-3 pb-1.5',
        !isTaskMessage && 'hover:bg-brutal-muted/15',
        isFailed && 'bg-brutal-danger-light/30',
        isEditing && 'border-l-[3px] border-l-brutal-primary bg-brutal-primary-light/30',
        isHighlighted && 'bg-brutal-primary-light ring-2 ring-brutal-accent',
        selected && 'bg-brutal-primary-light/60 ring-2 ring-brutal-accent',
        isTaskMessage && 'border-l-4',
        isTaskMessage && headerConfig?.accentClass,
        isTaskMessage && headerConfig?.bgClass,
        (isTaskMessage || selectionMode) && 'cursor-pointer',
      )}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      role="listitem"
      onClick={selectionMode ? () => onSelect?.(message) : isTaskMessage && onReply ? () => onReply(message) : undefined}
      onKeyDown={(selectionMode || (isTaskMessage && onReply)) ? (e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        e.preventDefault();
        if (selectionMode) onSelect?.(message);
        else onReply?.(message);
      } : undefined}
      tabIndex={(selectionMode || isTaskMessage) ? 0 : undefined}
      aria-label={isTaskMessage ? `Task #${message.task_number} — ${headerConfig?.label || ''}` : undefined}
    >
      {selectionMode && <MessageSelectMark selected={Boolean(selected)} />}
      {/* P25-08-F: Unread thread red dot */}
      {hasUnreadThread && onReply && (
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); onReply(message); }}
          className="flex-shrink-0 self-center -mr-1.5 -ml-2"
          aria-label={t('unreadThreadReply')}
          title={t('unreadReply')}
        >
          {/* v3.1: fade-in plays once on first render, then bounce-slow
              keeps the dot gently noticeable so users see the unread
              reply on subsequent scrolls. Killed by prefers-reduced-motion. */}
          <span className="block h-2.5 w-2.5 bg-brutal-danger border border-black animate-fade-in animate-bounce-slow" />
        </button>
      )}

      {isGrouped ? (
        <div className="mt-0.5 w-8 flex-shrink-0 text-center">
          <time
            dateTime={message.created_at}
            className="font-mono text-[9px] text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
          >
            {compactTime}
          </time>
        </div>
      ) : message.sender_type === 'user' ? (
        <UserAvatar
          userId={message.user_id}
          name={message.display_name}
          avatarUrl={message.avatar_url}
          size="md"
          className="mt-0.5"
        />
      ) : (
        <Avatar
          name={message.display_name}
          className="mt-0.5 h-8 w-8 flex-shrink-0"
        />
      )}

      <div className="min-w-0 flex-1">
        {isGrouped && <span className="sr-only">{message.display_name}, {compactTime}</span>}
        {/* SOLO-225-F: Task header row — above sender name + timestamp */}
        {isTaskMessage && headerConfig && (
          <div className="flex items-center gap-2 mb-1.5">
            <SquareCheckBig className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
            <span className="font-mono text-xs font-bold">
              #{message.task_number}
            </span>
            {message.task_title && (
              <span className="font-heading text-sm font-bold truncate">
                {message.task_title}
              </span>
            )}
            <span className={cn('badge-brutal ml-auto flex-shrink-0', headerConfig.badgeClass)}>
              {headerConfig.label}
            </span>
          </div>
        )}

        {/* Sender name + timestamp */}
        <div className={cn('mb-1.5 items-baseline gap-2', isGrouped ? 'hidden' : 'flex')}>
          <span className="font-heading text-sm font-bold text-foreground">
            {message.display_name}
          </span>
          {message.sender_type === 'agent' && message.sender_active === false && (
            <span className="badge-brutal bg-brutal-muted text-black">
              {t('deleted')}
            </span>
          )}
          <time dateTime={message.created_at} className="font-mono text-[11px] text-muted-foreground">
            {time}
          </time>
          {isEditing && (
            <span className="font-mono text-[11px] text-brutal-primary animate-pulse ml-auto">
              {t('editingMessage')}
            </span>
          )}
        </div>

        {/* Message content or edit mode */}
        {isEditing ? (
          <div className="space-y-2">
            {isSaving && (
              <div className="flex items-center gap-1.5">
                <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                <span className="font-body text-xs text-muted-foreground">{t('savingMessage')}</span>
              </div>
            )}
            <textarea
              ref={editRef}
              value={editContent}
              onChange={(e) => setEditContent(e.target.value)}
              onKeyDown={handleEditKeyDown}
              className="input-brutal min-h-[60px] resize-y py-2 text-sm"
              aria-label={t('editMessage')}
              disabled={isSaving}
            />
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleSaveEdit}
                disabled={isSaving || !editContent.trim()}
                className="btn-brutal btn-brutal-sm btn-brutal-success"
              >
                {isSaving ? t('savingMessage') : t('saveMessage')}
              </button>
              <button
                type="button"
                onClick={handleCancelEdit}
                disabled={isSaving}
                className="btn-brutal btn-brutal-sm"
              >
                {t('cancel')}
              </button>
            </div>
          </div>
        ) : message.content_type === 'forwarded' ? (
          <ForwardedMessageCard message={message} />
        ) : isThinkingHandoff ? (
          <MessageMarkdown
            content={message.content}
            onOpenArtifactReference={onOpenArtifactReference}
          />
        ) : (
          <p
            className={cn(
              'whitespace-pre-wrap break-words leading-relaxed',
              isFailed && 'text-brutal-danger/80',
            )}
            dangerouslySetInnerHTML={{
              __html: sanitizeHtml(
                message.content
                  .replace(/&/g, '&amp;')
                  .replace(/</g, '&lt;')
                  .replace(/>/g, '&gt;')
                  .replace(/#(\d+)/g, '<span class="tasknum-highlight">#$1</span>'),
              ),
            }}
          />
        )}

        {/* SOLO-249-F: Inline attachments */}
        {!isEditing && message.attachments && message.attachments.length > 0 && (
          <MessageAttachments attachments={message.attachments} />
        )}

        {!isEditing && !isFailed && !isSending && (
          <MessageReactionChips {...reactionState} />
        )}

        {/* Failed state actions */}
        {isFailed && (
          <div className="mt-2 flex items-center gap-2">
            <AlertCircle className="h-3.5 w-3.5 text-brutal-danger" />
            <span className="font-mono text-[11px] text-brutal-danger">
              {t('sendFailed')}
            </span>
            <button
              type="button"
              onClick={() => onRetry(message.id, message.content)}
              className="btn-brutal btn-brutal-sm"
            >
              <RefreshCw className="mr-1 h-3 w-3" />
              {t('retry')}
            </button>
            <button
              type="button"
              onClick={() => onCancel?.(message.id)}
              className="btn-brutal btn-brutal-sm"
            >
              {t('cancel')}
            </button>
          </div>
        )}

        {/* Sending indicator */}
        {isSending && (
          <div className="mt-1.5">
            <span className="font-mono text-[11px] text-muted-foreground">
              {t('sending')}
            </span>
          </div>
        )}

        {/* Task claimer */}
        {isTaskMessage && headerConfig && (
          <div
            data-task-claimer
            className="mt-2 inline-flex max-w-full items-center gap-1.5 rounded-full border border-brutal-muted bg-brutal-cream px-2.5 py-1 text-xs"
          >
            <UserRoundCheck className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="flex-shrink-0 text-muted-foreground">{t('claimerLabel')}</span>
            <span className="min-w-0 truncate font-heading font-bold text-foreground">
              {message.task_claimer_name
                ? `${message.task_claimer_name}${message.task_claimer_deleted ? ` (${t('deleted')})` : ''}`
                : t('unclaimed')}
            </span>
          </div>
        )}

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
        {(message.branch_count ?? 0) > 0 && message.latest_branch_node_id && (
          <Link
            href={`/dashboard?channel=${encodeURIComponent(message.channel_id)}&view=thinking&panel=conversation&node=${encodeURIComponent(message.latest_branch_node_id)}`}
            className="mt-2 inline-flex items-center gap-1.5 rounded-full border border-brutal-border bg-brutal-cream px-2.5 py-1 font-body text-xs font-semibold text-muted-foreground hover:bg-brutal-muted-light hover:text-foreground"
          >
            <GitBranch className="h-3.5 w-3.5" />
            {t('messageBranches', { n: message.branch_count ?? 0 })}
          </Link>
        )}
      </div>

      {/* Hover actions: edit / delete / reply */}
      {!isEditing && !isFailed && !isSending && (
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
          {onReply && (
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); onReply(message); }}
              className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
              aria-label={t('replyToMessage', { name: message.display_name })}
              title={t('replyToMessage', { name: message.display_name })}
            >
              <MessageSquare className="h-3.5 w-3.5" />
            </button>
          )}
          <MessageReuseMenu
            message={message}
            onCopy={onCopy}
            onSelect={onSelect}
            onEdit={onEdit ? () => {
              setEditContent(message.content || '');
              setIsEditing(true);
            } : undefined}
            onDelete={onDelete ? () => onDelete(message.id) : undefined}
            onAsTask={onAsTask && message.sender_type !== 'system' && !isTaskMessage ? onAsTask : undefined}
            onPin={onPin ? async () => {
              if (isTogglingPin) return;
              setIsTogglingPin(true);
              try {
                await onPin(message);
              } finally {
                setIsTogglingPin(false);
              }
            } : undefined}
            pinned={pinned}
            onFavorite={onFavorite}
            onForward={onForward}
            onBranch={onBranch}
          />
        </div>
      )}
    </div>
  );
});

// ---- Delete confirmation dialog ----

interface DeleteConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  messageAuthor: string;
}

function DeleteConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
  messageAuthor,
}: DeleteConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader>
        <DialogTitle>{t('deleteMessageTitle')}</DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>
      <DialogDescription>
        {t('deleteMessageConfirm', { name: messageAuthor })}
      </DialogDescription>
      <DialogFooter>
        <Button
          type="button"
          onClick={() => onOpenChange(false)}
          variant="outline"
          size="sm"
        >
          {t('cancel')}
        </Button>
        <Button
          type="button"
          onClick={onConfirm}
          variant="danger"
          size="sm"
        >
          {t('delete')}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}

// ---- Skeleton ----

function MessageSkeleton() {
  return (
    <div className="flex gap-3 px-6 py-3">
      <Skeleton className="h-8 w-8 flex-shrink-0 rounded-full" />
      <div className="flex-1 space-y-2">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3 w-12" />
        </div>
        <Skeleton className="h-12 w-3/4 rounded-none" />
      </div>
    </div>
  );
}

// ---- Empty state ----

function MessageListEmpty() {
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <EmptyState
        size="md"
        illustration={{ src: '/illustrations/empty-tasks.png', alt: t('noMessages') }}
        title={t('noMessages')}
        className="bg-transparent border-0 shadow-none"
      />
    </div>
  );
}

// ---- Scroll-to-bottom button ----

function ScrollToBottom({ onClick }: { onClick: () => void }) {
  return (
    <div className="absolute bottom-0 left-1/2 z-10 -translate-x-1/2 -translate-y-4">
      <button
        type="button"
        onClick={onClick}
        // v3.1: added px-2.5 — the .btn-brutal class deliberately doesn't
        // set padding (consumers set it per use), and this button was
        // shipping without it, so the long "Back to latest" text was
        // flush against the right 2px border. px-2.5 (10px each side)
        // balances the existing 18px left margin (14px icon + 4px gap)
        // so the button reads as a proper brutal pill, not a chopped label.
        className="btn-brutal btn-brutal-sm h-8 gap-1 bg-white px-2.5 text-xs"
        aria-label={t('scrollToLatest')}
      >
        <ChevronDown className="h-3.5 w-3.5" />
        {t('scrollToLatest')}
      </button>
    </div>
  );
}

// ---- Top-of-list UI elements for infinite scroll ----

function LoadMoreSpinner() {
  return (
    <div className="flex items-center justify-center gap-2 py-3 font-mono text-xs text-muted-foreground">
      {/* v3.1: spin-slow (10s/rev) reads as a deliberate "fetching older
          history" rather than the default 1s spin which feels urgent.
          Killed by prefers-reduced-motion. */}
      <Loader2 className="h-3.5 w-3.5 animate-spin-slow" />
      <span>{t('loadEarlierMessages')}</span>
    </div>
  );
}

function ChannelBeginning() {
  return (
    <div className="px-6 py-4 text-center">
      <div className="flex items-center gap-3">
        <div className="flex-1 border-t-2 border-black" />
        <span className="font-mono text-[11px] flex-shrink-0 text-muted-foreground">
          {t('beginningOfChannel')}
        </span>
        <div className="flex-1 border-t-2 border-black" />
      </div>
    </div>
  );
}

function LoadMoreFailed({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="flex items-center justify-center gap-2 py-3">
      <AlertCircle className="h-3.5 w-3.5 text-brutal-danger" />
      <span className="font-body text-xs text-brutal-danger">{t('loadError')}</span>
      <button
        type="button"
        onClick={onRetry}
        className="btn-brutal btn-brutal-sm"
      >
        <RefreshCw className="mr-1 h-3 w-3" />
        {t('retry')}
      </button>
    </div>
  );
}

// ---- Keyboard shortcuts help tip ----

const SHORTCUTS_HELP_KEY = 'solo-keyboard-shortcuts-dismissed';

function KeyboardShortcutsHelp({ onDismiss }: { onDismiss: () => void }) {
  return (
    <div className="mx-6 mb-2 flex items-center justify-between border-2 border-black bg-brutal-primary-light px-3 py-1.5">
      <span className="font-mono text-[11px] text-muted-foreground">
        {t('keyboardShortcutHint')}
      </span>
      <button
        type="button"
        onClick={onDismiss}
        className="ml-2 font-mono text-[11px] text-muted-foreground hover:text-foreground transition-colors"
        aria-label={t('closeShortcutHint')}
      >
        x
      </button>
    </div>
  );
}

// ---- Main component ----

export function MessageList({
  messages,
  isLoading,
  error,
  onRetry,
  onCancel,
  onReply,
  onEdit,
  onDelete,
  onAsTask,
  hasMore,
  isLoadingMore,
  loadMoreError,
  onLoadMore,
  scrollToMessageId,
  scrollKey,
  members = [],
  onOpenArtifactReference,
  onAgentClick,
  onPin,
  pinnedMessageIds = new Set(),
  contextLabel = 'Solo',
  forwardChannels = [],
  onFavorite,
  onForward,
  onBranch,
}: MessageListProps) {
  const { showToast } = useToast();
  const validNames = buildValidNames(members);
  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const prevMessageCountRef = useRef(0);
  const scrollRestoreRef = useRef<number | null>(null);
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [shareOpen, setShareOpen] = useState(false);
  const [forwardTarget, setForwardTarget] = useState<Message | null>(null);
  const [forwardChannelId, setForwardChannelId] = useState('');
  const [branchTarget, setBranchTarget] = useState<Message | null>(null);
  const [branchTitle, setBranchTitle] = useState('');
  const [reuseBusy, setReuseBusy] = useState(false);

  const openForward = useCallback((message: Message) => {
    const first = forwardChannels.find((item) => item.id !== message.channel_id);
    setForwardChannelId(first?.id ?? '');
    setForwardTarget(message);
  }, [forwardChannels]);

  const openBranch = useCallback((message: Message) => {
    setBranchTitle(message.content.trim().split('\n')[0].slice(0, 100));
    setBranchTarget(message);
  }, []);

  const submitForward = useCallback(async () => {
    if (!forwardTarget || !forwardChannelId || !onForward || reuseBusy) return;
    setReuseBusy(true);
    try {
      await onForward(forwardTarget, forwardChannelId);
      setForwardTarget(null);
    } catch {
      // The parent already shows the server error; keep the dialog open for retry.
    } finally {
      setReuseBusy(false);
    }
  }, [forwardChannelId, forwardTarget, onForward, reuseBusy]);

  const submitBranch = useCallback(async () => {
    if (!branchTarget || !branchTitle.trim() || !onBranch || reuseBusy) return;
    setReuseBusy(true);
    try {
      await onBranch(branchTarget, branchTitle.trim());
      setBranchTarget(null);
    } catch {
      // The parent already shows the server error; keep the dialog open for retry.
    } finally {
      setReuseBusy(false);
    }
  }, [branchTarget, branchTitle, onBranch, reuseBusy]);

  const toggleSelection = useCallback((message: Message) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(message.id)) next.delete(message.id);
      else next.add(message.id);
      return next;
    });
  }, []);

  const handleCopy = useCallback(async (message: Message) => {
    try {
      await copyMessageText(message.content);
      showToast(t('messageCopied'), 'success');
    } catch {
      showToast(t('copyFailed'), 'error');
    }
  }, [showToast]);

  const handleShareError = useCallback(() => {
    showToast(t('shareImageFailed'), 'error');
  }, [showToast]);

  const selectedMessages: ShareableMessage[] = messages
    .filter((message) => selectedIds.has(message.id))
    .map((message) => ({
      id: message.id,
      displayName: message.display_name,
      content: message.content,
      createdAt: message.created_at,
    }));

  useEffect(() => {
    const visible = new Set(messages.map((message) => message.id));
    setSelectedIds((current) => {
      if ([...current].every((id) => visible.has(id))) return current;
      return new Set([...current].filter((id) => visible.has(id)));
    });
  }, [messages]);

  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    displayName: string;
  } | null>(null);

  // Keyboard shortcuts help tip — show once per browser
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(() => {
    if (typeof window === 'undefined') return false;
    return !localStorage.getItem(SHORTCUTS_HELP_KEY);
  });

  const dismissShortcutsHelp = useCallback(() => {
    if (typeof window !== 'undefined') {
      localStorage.setItem(SHORTCUTS_HELP_KEY, '1');
    }
    setShowShortcutsHelp(false);
  }, []);

  const handleDeleteConfirm = useCallback(() => {
    if (deleteTarget) {
      onDelete?.(deleteTarget.id);
      setDeleteTarget(null);
    }
  }, [deleteTarget, onDelete]);

  // IntersectionObserver for infinite scroll
  const onLoadMoreRef = useRef(onLoadMore);
  onLoadMoreRef.current = onLoadMore;

  useEffect(() => {
    const sentinel = sentinelRef.current;
    const container = scrollRef.current;
    if (!sentinel || !container || !hasMore) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (
          entry.isIntersecting &&
          hasMore &&
          !isLoadingMore &&
          !loadMoreError
        ) {
          const el = scrollRef.current;
          if (el) {
            scrollRestoreRef.current = el.scrollHeight;
          }
          onLoadMoreRef.current();
        }
      },
      {
        root: container,
        threshold: 0.1,
      },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, isLoadingMore, loadMoreError]);

  // Scroll position preservation after loading older messages
  const prevLoadingMoreRef = useRef(isLoadingMore);

  useLayoutEffect(() => {
    if (
      prevLoadingMoreRef.current &&
      !isLoadingMore &&
      scrollRestoreRef.current !== null
    ) {
      const el = scrollRef.current;
      if (el) {
        const diff = el.scrollHeight - scrollRestoreRef.current;
        el.scrollTop += diff;
      }
      scrollRestoreRef.current = null;
    }
    prevLoadingMoreRef.current = isLoadingMore;
  }, [isLoadingMore]);

  // Auto-scroll to bottom for new messages
  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;

    const threshold = 80;
    const atBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
    setIsAtBottom(atBottom);
  };

  useEffect(() => {
    if (isAtBottom && messages.length > prevMessageCountRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: motionScrollBehavior() });
    }
    prevMessageCountRef.current = messages.length;
  }, [messages.length, isAtBottom]);

  useEffect(() => {
    if (!isLoading && messages.length > 0) {
      bottomRef.current?.scrollIntoView();
    }
  }, [isLoading, messages.length]);

  // SOLO-237-F: Scroll to a specific message by ID
  useEffect(() => {
    if (!scrollToMessageId || isLoading) return;
    // Small delay to ensure the DOM is rendered
    const timer = setTimeout(() => {
      const el = document.querySelector(`[data-message-id="${scrollToMessageId}"]`);
      if (el) {
        el.scrollIntoView({ behavior: motionScrollBehavior(), block: 'center' });
        setHighlightedMessageId(scrollToMessageId);
      }
    }, 100);
    const clearTimer = setTimeout(() => {
      setHighlightedMessageId((current) => current === scrollToMessageId ? null : current);
    }, 2600);
    return () => {
      clearTimeout(timer);
      clearTimeout(clearTimer);
    };
  }, [scrollToMessageId, scrollKey, isLoading]);

  const scrollToBottom = () => {
    bottomRef.current?.scrollIntoView({ behavior: motionScrollBehavior() });
    setIsAtBottom(true);
  };

  // Loading state
  if (isLoading) {
    return (
      <div className="flex-1 overflow-y-auto">
        <div className="pt-4 space-y-1">
          <MessageSkeleton />
          <MessageSkeleton />
          <MessageSkeleton />
        </div>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="text-center space-y-2">
          <AlertCircle className="mx-auto h-8 w-8 text-brutal-danger" />
          <p className="font-body text-sm text-brutal-danger">{error}</p>
        </div>
      </div>
    );
  }

  // Empty state
  if (messages.length === 0) {
    return <MessageListEmpty />;
  }

  // Messages list
  return (
    <div className="relative flex-1 overflow-hidden">
      <div
        ref={scrollRef}
        className="h-full overflow-y-auto"
        onScroll={handleScroll}
        role="list"
        aria-label={t('messageList')}
        data-streaming-container="true"
      >
        {hasMore && !loadMoreError && (
          <div ref={sentinelRef} className="h-px" />
        )}

        {isLoadingMore && <LoadMoreSpinner />}

        {loadMoreError && (
          <LoadMoreFailed onRetry={() => onLoadMore()} />
        )}

        {!hasMore && !isLoadingMore && !loadMoreError && (
          <ChannelBeginning />
        )}

        <div className="py-2">
          {messages.map((message, index) => {
            const previous = messages[index - 1];
            const startsDay = !previous || messageDateKey(previous.created_at) !== messageDateKey(message.created_at);
            const isGrouped = !startsDay && canGroupMessages(previous, message);
            return (
              <Fragment key={message.id}>
                {startsDay && <MessageDateSeparator createdAt={message.created_at} />}
                {message.status === 'streaming' ? (
                  <StreamingMessage
                    message={message}
                    isGrouped={isGrouped}
                    onAgentClick={onAgentClick}
                  />
                ) : message.sender_type === 'agent' ? (
                  <AgentMessage
                    message={message}
                    isGrouped={isGrouped}
                    onReply={onReply}
                    validNames={validNames}
                    isHighlighted={highlightedMessageId === message.id}
                    onOpenArtifactReference={onOpenArtifactReference}
                    onPin={onPin}
                    pinned={pinnedMessageIds.has(message.id)}
                    onAgentClick={onAgentClick}
                    onCopy={handleCopy}
                    onSelect={toggleSelection}
                    selectionMode={selectedIds.size > 0}
                    selected={selectedIds.has(message.id)}
                    onFavorite={onFavorite ? (item) => { void onFavorite(item); } : undefined}
                    onForward={onForward ? openForward : undefined}
                    onBranch={onBranch ? openBranch : undefined}
                  />
                ) : (
                  <MessageItem
                    message={message}
                    isGrouped={isGrouped}
                    isHighlighted={highlightedMessageId === message.id}
                    onRetry={onRetry}
                    onCancel={onCancel}
                    onReply={onReply}
                    onEdit={onEdit}
                    onAsTask={onAsTask}
                    onOpenArtifactReference={onOpenArtifactReference}
                    onPin={onPin}
                    pinned={pinnedMessageIds.has(message.id)}
                    onCopy={handleCopy}
                    onSelect={toggleSelection}
                    selectionMode={selectedIds.size > 0}
                    selected={selectedIds.has(message.id)}
                    onFavorite={onFavorite ? (item) => { void onFavorite(item); } : undefined}
                    onForward={onForward ? openForward : undefined}
                    onBranch={onBranch ? openBranch : undefined}
                    onDelete={
                      onDelete
                        ? (id) => {
                            const msg = messages.find((m) => m.id === id);
                            setDeleteTarget({
                              id,
                              displayName: msg?.display_name ?? t('user'),
                            });
                          }
                        : undefined
                    }
                  />
                )}
              </Fragment>
            );
          })}
        </div>

        {showShortcutsHelp && messages.length > 0 && (
          <KeyboardShortcutsHelp onDismiss={dismissShortcutsHelp} />
        )}

        <div ref={bottomRef} />
      </div>

      {!isAtBottom && messages.length > 0 && (
        <ScrollToBottom onClick={scrollToBottom} />
      )}

      <MessageSelectionToolbar
        count={selectedIds.size}
        onCancel={() => setSelectedIds(new Set())}
        onCreateImage={() => setShareOpen(true)}
      />

      {/* Delete confirmation dialog */}
      <DeleteConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        onConfirm={handleDeleteConfirm}
        messageAuthor={deleteTarget?.displayName ?? ''}
      />
      <ShareMessagesDialog
        open={shareOpen}
        onOpenChange={setShareOpen}
        messages={selectedMessages}
        contextLabel={contextLabel}
        onError={handleShareError}
        onCopied={() => showToast(t('imageCopied'), 'success')}
      />
      <Dialog open={!!forwardTarget} onOpenChange={(open) => { if (!open && !reuseBusy) setForwardTarget(null); }}>
        <DialogHeader>
          <DialogTitle>{t('forwardMessage')}</DialogTitle>
          <DialogCloseButton onClick={() => setForwardTarget(null)} />
        </DialogHeader>
        <DialogDescription>{t('forwardMessageDescription')}</DialogDescription>
        <label className="mt-4 block font-body text-sm font-semibold" htmlFor="forward-channel">{t('forwardToChannel')}</label>
        <select id="forward-channel" value={forwardChannelId} onChange={(event) => setForwardChannelId(event.target.value)} className="input-brutal mt-2" autoFocus>
          {forwardChannels.filter((item) => item.id !== forwardTarget?.channel_id && item.type !== 'lucy').map((item) => (
            <option key={item.id} value={item.id}>#{item.name}</option>
          ))}
        </select>
        {!forwardChannelId && <p className="mt-2 font-body text-xs text-muted-foreground">{t('noForwardChannel')}</p>}
        <DialogFooter>
          <Button type="button" variant="outline" size="sm" onClick={() => setForwardTarget(null)} disabled={reuseBusy}>{t('cancel')}</Button>
          <Button type="button" size="sm" onClick={() => { void submitForward(); }} disabled={!forwardChannelId || reuseBusy}>{reuseBusy ? t('submitting') : t('forwardMessage')}</Button>
        </DialogFooter>
      </Dialog>
      <Dialog open={!!branchTarget} onOpenChange={(open) => { if (!open && !reuseBusy) setBranchTarget(null); }}>
        <DialogHeader>
          <DialogTitle>{t('branchFromMessage')}</DialogTitle>
          <DialogCloseButton onClick={() => setBranchTarget(null)} />
        </DialogHeader>
        <DialogDescription>{t('branchFromMessageDescription')}</DialogDescription>
        <label className="mt-4 block font-body text-sm font-semibold" htmlFor="branch-title">{t('branchTitle')}</label>
        <input id="branch-title" value={branchTitle} onChange={(event) => setBranchTitle(event.target.value)} maxLength={100} className="input-brutal mt-2" autoFocus />
        <DialogFooter>
          <Button type="button" variant="outline" size="sm" onClick={() => setBranchTarget(null)} disabled={reuseBusy}>{t('cancel')}</Button>
          <Button type="button" size="sm" onClick={() => { void submitBranch(); }} disabled={!branchTitle.trim() || reuseBusy}>{reuseBusy ? t('submitting') : t('createBranch')}</Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}

function ForwardedMessageCard({ message }: { message: Message }) {
  const channelId = typeof message.metadata?.forwarded_channel_id === 'string' ? message.metadata.forwarded_channel_id : '';
  const channelName = typeof message.metadata?.forwarded_channel_name === 'string' ? message.metadata.forwarded_channel_name : t('unknown');
  const senderName = typeof message.metadata?.forwarded_sender_name === 'string' ? message.metadata.forwarded_sender_name : t('unknown');
  const sourceMessageId = typeof message.metadata?.forwarded_message_id === 'string' ? message.metadata.forwarded_message_id : '';
  return (
    <Link
      href={channelId ? `/dashboard?channel=${encodeURIComponent(channelId)}${sourceMessageId ? `&message=${encodeURIComponent(sourceMessageId)}` : ''}` : '#'}
      className="block rounded-lg border-l-4 border-brutal-accent bg-brutal-muted-light/60 px-3 py-2 hover:bg-brutal-muted-light"
    >
      <div className="mb-1 font-body text-xs font-semibold text-muted-foreground">{t('forwardedFrom', { channel: channelName, sender: senderName })}</div>
      <p className="whitespace-pre-wrap break-words font-body text-sm leading-relaxed">{message.content}</p>
    </Link>
  );
}
