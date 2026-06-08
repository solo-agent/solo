// ============================================================================
// ThreadPanel — right-side thread reply panel (neubrutalist)
// - card-brutal container with thick left border
// - Optional task metadata bar: #N title [status] -> @claimer + claim/unclaim
// - Parent message: message-bubble style
// - Input: input-brutal font-mono
// - Send: btn-brutal btn-brutal-primary
// - Close: btn-brutal-sm
// - Empty: "还没有回复，发起讨论吧"
// - States: loading skeleton, empty, error, normal
// ============================================================================

'use client';

import {
  useEffect,
  useRef,
  useState,
  useCallback,
} from 'react';
import { X, AlertCircle, RefreshCw, Send, MessageSquare } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import { cn } from '@/lib/utils';
import { Avatar } from '@/components/ui/avatar';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { useThread } from '@/lib/hooks/use-thread';
import { useMentions } from '@/lib/hooks/use-mentions';
import { useWebSocket } from '@/lib/ws-context';
import { MentionDropdown, type DropdownAnchor } from './mention-dropdown';
import { t } from '@/lib/i18n';
import type { Message, ChannelMember, Task, TaskStatus } from '@/lib/types';

interface ThreadPanelProps {
  parentMessage: Message;
  onClose: () => void;
  members?: ChannelMember[];
  /** Initial reply count from parent message (P25-10-F) */
  replyCount?: number;
  /** Optional task associated with this thread's parent message */
  task?: Task;
  /** Callback when user claims the task from the metadata bar */
  onClaimTask?: (task: Task) => void;
  /** Callback when user unclaims the task from the metadata bar */
  onUnclaimTask?: (task: Task) => void;
  /** Callback after the thread has been marked as read (P25-08-F) */
  onMarkRead?: () => void;
}

// ---- Parent message display ----

function ParentMessageBlock({ message, task }: { message: Message; task?: Task }) {
  const isAgent = message.sender_type === 'agent';
  const displayName = task?.creator_name || message.display_name;
  const time = new Date(message.created_at).toLocaleString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  });

  return (
    <div className={`flex gap-3 px-6 py-4 border-b-2 border-black ${isAgent ? 'border-l-2 border-l-brutal-primary' : ''}`}>
      {isAgent ? (
        <PixelAvatar agentId={message.user_id || message.id} size="md" />
      ) : (
        <Avatar
          name={displayName}
          className="mt-0.5 h-8 w-8 flex-shrink-0"
        />
      )}
      <div className="min-w-0 flex-1">
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-foreground">
            {displayName}
          </span>
          <span className="font-mono text-[11px] text-muted-foreground">
            {time}
          </span>
        </div>
        <div className="font-body text-sm leading-relaxed space-y-1">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeRaw]}
            components={mdComponents as any}
          >
            {message.content}
          </ReactMarkdown>
        </div>
      </div>
    </div>
  );
}

// ---- Shared Markdown components ----

function highlightSpecials(text: string): string {
  const parts = text.split(/(```[\s\S]*?```)/g);
  return parts
    .map((part, i) => {
      if (i % 2 === 1) return part;
      let processed = part.replace(/@([^\s@#]+)/g, '<span class="mention-highlight">@$1</span>');
      processed = processed.replace(/#(\d+)/g, '<span class="tasknum-highlight">#$1</span>');
      return processed;
    })
    .join('');
}

function CodeBlock({ className, children }: { className?: string; children?: React.ReactNode }) {
  const language = className?.replace('language-', '') ?? '';
  return (
    <div className="my-2 border-2 border-black shadow-brutal-sm overflow-x-auto">
      {language && (
        <div className="border-b-2 border-black bg-brutal-primary px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black">
          {language}
        </div>
      )}
      <pre className="bg-black p-3 text-xs leading-relaxed">
        <code className={`${className ?? ''} font-mono text-brutal-success`}>
          {children}
        </code>
      </pre>
    </div>
  );
}

const mdComponents = {
  p({ children }: { children?: React.ReactNode }) {
    return <p className="my-1 whitespace-pre-wrap break-words">{children}</p>;
  },
  strong({ children }: { children?: React.ReactNode }) {
    return <strong className="font-heading font-black">{children}</strong>;
  },
  a({ href, children }: { href?: string; children?: React.ReactNode }) {
    return <a href={href} target="_blank" rel="noopener noreferrer" className="text-brutal-info font-bold underline decoration-2 underline-offset-2 hover:text-brutal-primary transition-colors">{children}</a>;
  },
  ul({ children }: { children?: React.ReactNode }) {
    return <ul className="my-1 list-disc pl-4 space-y-0.5">{children}</ul>;
  },
  ol({ children }: { children?: React.ReactNode }) {
    return <ol className="my-1 list-decimal pl-4 space-y-0.5">{children}</ol>;
  },
  li({ children }: { children?: React.ReactNode }) {
    return <li className="leading-relaxed">{children}</li>;
  },
  blockquote({ children }: { children?: React.ReactNode }) {
    return <blockquote className="my-1.5 border-l-2 border-brutal-primary/50 pl-3 italic text-muted-foreground">{children}</blockquote>;
  },
  code({ className, children, ...props }: { className?: string; children?: React.ReactNode; [key: string]: unknown }) {
    const isInline = !className;
    if (isInline) {
      return <code className="rounded-none border border-black bg-black/5 px-1 py-0.5 font-mono text-xs text-foreground">{children}</code>;
    }
    return <CodeBlock className={className}>{children}</CodeBlock>;
  },
  pre({ children }: { children?: React.ReactNode }) {
    return <>{children}</>;
  },
  hr() {
    return <hr className="divider-brutal my-3" />;
  },
  table({ children }: { children?: React.ReactNode }) {
    return <div className="my-2 overflow-x-auto border-2 border-black shadow-brutal-sm"><table className="w-full text-sm font-body">{children}</table></div>;
  },
  th({ children }: { children?: React.ReactNode }) {
    return <th className="border-b-2 border-black bg-brutal-primary px-3 py-2 text-left font-heading font-bold text-black">{children}</th>;
  },
  td({ children }: { children?: React.ReactNode }) {
    return <td className="border-t border-black px-3 py-1.5">{children}</td>;
  },
};

// ---- Reply item ----

function ReplyItem({ message }: { message: { id: string; display_name?: string; content: string; created_at: string; status?: string; sender_type?: string } }) {
  const isFailed = message.status === 'failed';
  const isSending = message.status === 'sending';
  const isStreaming = message.status === 'streaming';
  const isAgent = message.sender_type === 'agent';

  const time = new Date(message.created_at).toLocaleString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  });

  return (
    <div className={`flex gap-3 px-6 py-2 border-b-2 border-black ${isAgent ? 'border-l-2 border-l-brutal-primary' : ''}`}>
      {isAgent ? (
        <PixelAvatar agentId={message.id} size="sm" />
      ) : (
        <Avatar
          name={message.display_name || '?'}
          className="mt-0.5 h-7 w-7 flex-shrink-0"
        />
      )}
      <div className="min-w-0 flex-1">
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-foreground">
            {message.display_name}
          </span>
          <span className="font-mono text-[11px] text-muted-foreground">
            {time}
          </span>
          {isAgent && (
            <span className="badge-brutal bg-brutal-primary text-black">
              Agent
            </span>
          )}
        </div>
        <div className="">
          {isStreaming && !message.content ? (
            <div className="py-1">
              <span className="inline-flex items-center gap-1" aria-label="Agent is typing">
                {[0, 1, 2].map((i) => (
                  <span
                    key={i}
                    className="inline-block h-1.5 w-1.5 bg-brutal-primary animate-bounce"
                    style={{ animationDelay: `${i * 0.15}s`, animationDuration: '0.8s' }}
                  />
                ))}
              </span>
            </div>
          ) : (
            <div className={`font-body text-sm leading-relaxed space-y-1 ${isFailed ? 'text-brutal-danger/80' : ''}`}>
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[rehypeRaw]}
                components={mdComponents as any}
              >
                {highlightSpecials(message.content)}
              </ReactMarkdown>
              {isStreaming && message.content && (
                <span className="inline-block h-4 w-[2px] bg-brutal-primary align-middle animate-pulse ml-0.5" />
              )}
            </div>
          )}
        </div>
        {isFailed && (
          <div className="mt-2 flex items-center gap-1">
            <AlertCircle className="h-3.5 w-3.5 text-brutal-danger" />
            <span className="font-mono text-[11px] text-brutal-danger">{t('sendFailed')}</span>
          </div>
        )}
        {isSending && (
          <div className="mt-1.5">
            <span className="font-mono text-[11px] text-muted-foreground">{t('sending')}</span>
          </div>
        )}
      </div>
    </div>
  );
}

// ---- Skeleton ----

function ThreadSkeleton() {
  return (
    <div className="space-y-1">
      <div className="flex gap-3 px-6 py-4">
        <Skeleton className="h-8 w-8 flex-shrink-0 rounded-none" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-4 w-32 rounded-none" />
          <Skeleton className="h-12 w-3/4 rounded-none" />
        </div>
      </div>
      <div className="divider-brutal mx-6" />
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex gap-3 px-6 py-2">
          <Skeleton className="h-7 w-7 flex-shrink-0 rounded-none" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-3.5 w-24 rounded-none" />
            <Skeleton className="h-10 w-2/3 rounded-none" />
          </div>
        </div>
      ))}
    </div>
  );
}

// ---- Empty state ----

function ThreadEmpty() {
  return (
    <div className="flex flex-1 items-center justify-center px-6">
      <div className="text-center">
        <p className="font-body text-sm text-muted-foreground">
          No replies yet. Start the discussion.
        </p>
      </div>
    </div>
  );
}

// ---- Error state ----

function ThreadError({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div className="flex flex-1 items-center justify-center px-6">
      <div className="text-center space-y-3">
        <AlertCircle className="mx-auto h-8 w-8 text-brutal-danger" />
        <p className="font-mono text-sm text-brutal-danger">{message}</p>
        <button
          type="button"
          onClick={onRetry}
          className="btn-brutal btn-brutal-sm"
        >
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          Retry
        </button>
      </div>
    </div>
  );
}

// ---- Reply input ----

function ThreadReplyInput({
  onSend,
  disabled,
  members,
}: {
  onSend: (content: string, mentionedAgentIds?: string[]) => Promise<void> | void;
  disabled?: boolean;
  members: ChannelMember[];
}) {
  const [content, setContent] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [cursorPosition, setCursorPosition] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const isSendingRef = useRef(false);

  const {
    suggestions,
    showSuggestions,
    selectedIndex,
    searchQuery,
    handleKeyDown: mentionHandleKeyDown,
    selectSuggestion: mentionSelectSuggestion,
    resetMention,
    mentionedAgentIds,
  } = useMentions(members, content, cursorPosition);

  const mentionActive = showSuggestions || searchQuery !== '';

  // Dropdown anchor for mention list
  const [dropdownAnchor, setDropdownAnchor] = useState<DropdownAnchor | null>(null);
  const updateDropdownPosition = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setDropdownAnchor({ top: rect.top - 8, left: rect.left + 16, width: rect.width - 32 });
  }, []);
  useEffect(() => {
    if (mentionActive) { updateDropdownPosition(); }
    else { setDropdownAnchor(null); }
  }, [mentionActive, updateDropdownPosition]);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const trimmed = content.trim();
  const canSend = trimmed.length > 0 && !isSending && !disabled;

  const handleSend = useCallback(async () => {
    if (!canSend || isSendingRef.current) return;
    isSendingRef.current = true;
    setIsSending(true);
    try {
      await onSend(trimmed, mentionedAgentIds);
      setContent('');
      resetMention();
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } finally {
      isSendingRef.current = false;
      setIsSending(false);
      textareaRef.current?.focus();
    }
  }, [canSend, trimmed, onSend, mentionedAgentIds, resetMention]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (mentionActive) { e.preventDefault(); mentionHandleKeyDown(e as any); return; }
    }
    if (e.key === 'Escape') {
      if (mentionActive) { e.preventDefault(); resetMention(); return; }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      if (showSuggestions) {
        e.preventDefault();
        const newValue = mentionSelectSuggestion(selectedIndex);
        if (newValue !== null) { setContent(newValue); }
        return;
      }
      e.preventDefault();
      handleSend();
    }
  };

  const handleCursorMove = useCallback(() => {
    if (textareaRef.current) setCursorPosition(textareaRef.current.selectionStart);
  }, []);

  const handleInput = (value: string) => {
    setContent(value);
    const el = textareaRef.current;
    if (el) {
      setCursorPosition(el.selectionStart);
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
    }
  };

  return (
    <div className="border-t-2 border-black px-4 py-3">
      <div className="relative flex flex-col">
        {mentionActive && dropdownAnchor && (
          <MentionDropdown
            suggestions={suggestions}
            selectedIndex={selectedIndex}
            searchQuery={searchQuery}
            anchor={dropdownAnchor}
            onSelect={(index) => {
              const newValue = mentionSelectSuggestion(index);
              if (newValue !== null) { setContent(newValue); textareaRef.current?.focus(); }
            }}
          />
        )}
        <div className="relative flex items-end gap-2">
          <textarea
            ref={textareaRef}
            value={content}
            onChange={(e) => handleInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onSelect={handleCursorMove}
            onClick={handleCursorMove}
            placeholder={t('threadReplyPlaceholder')}
            rows={1}
            autoFocus
            disabled={isSending || disabled}
            aria-label={t('threadReplyInput')}
            className={cn(
              'input-brutal min-h-[44px] resize-none pr-12 font-mono text-sm leading-relaxed',
              'placeholder:font-mono placeholder:text-muted-foreground/60',
              'disabled:opacity-50',
            )}
          />
          <button
            onClick={handleSend}
            disabled={!canSend}
            aria-label={t('sendReply')}
            className={cn(
              'btn-brutal btn-brutal-primary absolute bottom-2 right-2 flex h-8 w-8 items-center justify-center p-0',
              !canSend && 'opacity-40 pointer-events-none',
            )}
          >
            <Send className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  );
}

// ---- Status & priority display helpers ----

const STATUS_BADGE_COLORS: Record<string, string> = {
  todo: 'bg-brutal-warning text-black border-black',
  in_progress: 'bg-brutal-info text-black border-black',
  in_review: 'bg-brutal-violet text-black border-black',
  done: 'bg-brutal-success text-black border-black',
  closed: 'bg-brutal-muted text-black border-black',
};

const STATUS_LABELS: Record<string, string> = {
  todo: 'TODO',
  in_progress: 'In Progress',
  in_review: 'In Review',
  done: 'Done',
  closed: 'Closed',
};

const PRIORITY_LABELS: Record<string, string> = {
  urgent: 'P0',
  high: 'P1',
  normal: 'P2',
  low: 'P3',
};

// ---- Task metadata bar from message fields (fallback when no Task object) ----

function ParentMessageTaskBar({ message }: { message: Message }) {
  const statusKey = message.task_status as TaskStatus | undefined;
  const badgeClass = statusKey ? (STATUS_BADGE_COLORS[statusKey] ?? STATUS_BADGE_COLORS.todo) : '';
  const statusLabel = statusKey ? (STATUS_LABELS[statusKey] ?? statusKey) : '';
  const taskNum = message.task_number ? `#${message.task_number}` : '';
  const claimerName = message.task_claimer_name;

  if (!statusKey || !badgeClass) return null;

  return (
    <div className="border-b-2 border-black bg-brutal-cream px-6 py-3">
      <div className="flex items-center gap-2 flex-wrap">
        {taskNum && (
          <span className="font-mono text-xs font-bold text-muted-foreground">
            {taskNum}
          </span>
        )}
        <span className={cn('border-2 px-2 py-0.5 text-xs font-bold shadow-brutal-sm', badgeClass)}>
          {statusLabel}
        </span>
        <span className="font-mono text-xs text-muted-foreground">&middot;</span>
        {claimerName ? (
          <span className="font-heading text-xs font-bold text-foreground">
            @{claimerName}
          </span>
        ) : (
          <span className="font-heading text-xs text-muted-foreground">
            Unclaimed
          </span>
        )}
      </div>
    </div>
  );
}

// ---- Task metadata bar (shown above parent message when thread is task-bound) ----

function TaskMetaBar({
  task,
  onClaim,
  onUnclaim,
}: {
  task: Task;
  onClaim?: (task: Task) => void;
  onUnclaim?: (task: Task) => void;
}) {
  const badgeClass = STATUS_BADGE_COLORS[task.status] ?? STATUS_BADGE_COLORS.todo;
  const statusLabel = STATUS_LABELS[task.status] ?? task.status;
  const priorityLabel = PRIORITY_LABELS[task.priority] ?? task.priority;
  const taskNum = task.task_number ? `#${task.task_number}` : '';
  const isClaimed = !!task.claimer_id;
  const claimerDisplay =
    task.claimer_name ||
    task.assignee_name ||
    (task.claimer_id ? task.claimer_id.slice(0, 8) : '');

  return (
    <div className="border-b-2 border-black bg-brutal-cream px-6 py-3">
      {/* Line 1: task number + title + status badge */}
      <div className="flex items-center gap-2 flex-wrap mb-1.5">
        {taskNum && (
          <span className="font-mono text-xs font-bold text-muted-foreground">
            {taskNum}
          </span>
        )}
        <span className="font-heading text-lg font-bold text-foreground">
          {task.title}
        </span>
        <span className={cn('border-2 px-2 py-0.5 text-xs font-bold shadow-brutal-sm', badgeClass)}>
          {statusLabel}
        </span>
      </div>

      {/* Line 2: priority + claimer + claim/unclaim */}
      <div className="flex items-center gap-2 text-xs flex-wrap">
        <span className="font-mono text-muted-foreground">
          Priority:{' '}
          <span className="font-bold text-foreground">{priorityLabel}</span>
        </span>
        <span className="font-mono text-muted-foreground">|</span>
        <span className="font-mono text-muted-foreground">Claimer:</span>
        {isClaimed ? (
          <div className="">
            <span className="flex items-center gap-1">
              <span className="flex h-4 w-4 items-center justify-center border-2 border-black bg-brutal-success font-heading text-[9px] font-bold text-black">
                {(claimerDisplay || '?').charAt(0).toUpperCase()}
              </span>
              <span className="font-heading text-xs font-bold text-foreground">
                @{claimerDisplay}
              </span>
            </span>
            {onUnclaim && (
              <button
                type="button"
                onClick={() => onUnclaim(task)}
                className="btn-brutal btn-brutal-sm ml-1 text-[10px]"
                aria-label={t('releaseTask')}
              >
                Release
              </button>
            )}
          </div>
        ) : (
          <div className="">
            <span className="font-heading text-xs text-muted-foreground">
              Not yet claimed
            </span>
            {onClaim && (
              <button
                type="button"
                onClick={() => onClaim(task)}
                className="btn-brutal btn-brutal-sm ml-1 text-[10px]"
                aria-label={t('claimTask')}
              >
                Claim
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ---- Main component ----

export function ThreadPanel({
  parentMessage,
  onClose,
  members = [],
  replyCount: initialReplyCount = 0,
  task,
  onClaimTask,
  onUnclaimTask,
  onMarkRead,
}: ThreadPanelProps) {
  const {
    messages,
    isLoading,
    error,
    loadThreadMessages,
    sendReply: rawSendReply,
    refetch,
    threadId,
    markRead,
  } = useThread();

  // Track reply count locally (P25-10-F)
  const [replyCount, setReplyCount] = useState(initialReplyCount);

  // Sync replyCount when initialReplyCount changes (e.g., when parent re-opens thread)
  useEffect(() => {
    setReplyCount(initialReplyCount);
  }, [initialReplyCount]);

  const { isConnected, subscribeThread, unsubscribeThread, onEvent } =
    useWebSocket();

  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const prevMessageCount = useRef(0);

  useEffect(() => {
    loadThreadMessages(parentMessage.channel_id, parentMessage.id);
  }, [parentMessage.channel_id, parentMessage.id, loadThreadMessages]);

  // Thread subscription is handled by useThread hook — no duplicate needed here.

  // P25-08-F: Auto mark-thread-as-read when ThreadPanel opens with a valid threadId
  useEffect(() => {
    if (!threadId) return;
    let cancelled = false;
    markRead().then(() => {
      if (!cancelled) onMarkRead?.();
    });
    return () => { cancelled = true; };
    // Only run when threadId first becomes available (panel opens)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId]);

  // Wrap sendReply to increment local reply count (P25-10-F)
  const handleSendReply = useCallback(
    async (content: string, mentionedAgentIds?: string[]) => {
      await rawSendReply(content, mentionedAgentIds);
      setReplyCount((prev) => prev + 1);
    },
    [rawSendReply],
  );

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'thread.reply' && event.thread_id === threadId) {
        setReplyCount(event.reply_count);
      }

    });
    return unsub;
  }, [threadId, onEvent]);

  useEffect(() => {
    if (messages.length > prevMessageCount.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
    prevMessageCount.current = messages.length;
  }, [messages.length]);

  useEffect(() => {
    if (!isLoading && messages.length > 0) {
      bottomRef.current?.scrollIntoView();
    }
  }, [isLoading, messages.length]);

  return (
    <div
      className={cn(
        'flex h-full flex-col bg-brutal-cream border-l-2 border-r-2 border-b-2 border-black shadow-brutal-sm',
        'animate-slide-in-from-right',
      )}
    >
      {/* Header */}
      <div className="flex h-14 flex-shrink-0 items-center justify-between border-b-2 border-black px-4">
        <h3 className="font-heading text-base font-bold text-foreground">
          Thread{replyCount > 0 && (
            <span className="ml-1.5 font-mono text-sm text-muted-foreground">
              &middot; {replyCount} replies
            </span>
          )}
        </h3>
        <button
          type="button"
          onClick={onClose}
          className="btn-brutal btn-brutal-sm flex h-8 w-8 items-center justify-center p-0"
          aria-label={t('closeThreadPanel')}
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Content area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Task metadata bar: prefer full Task object, fallback to message fields */}
        {task ? (
          <TaskMetaBar
            task={task}
            onClaim={onClaimTask}
            onUnclaim={onUnclaimTask}
          />
        ) : parentMessage.task_status ? (
          <ParentMessageTaskBar message={parentMessage} />
        ) : null}

        {/* Parent message */}
        <ParentMessageBlock message={parentMessage} task={task} />

        {/* Reply list */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto">
          {(() => {
            if (isLoading) {
              return <ThreadSkeleton />;
            }
            if (error) {
              return <ThreadError message={error} onRetry={refetch} />;
            }
            if (messages.length === 0) {
              return <ThreadEmpty />;
            }
            return (
              <div className="space-y-0 py-2">
                {messages.map((reply) => (
                  <ReplyItem key={reply.id} message={reply} />
                ))}
              </div>
            );
          })()}
          <div ref={bottomRef} />
        </div>
      </div>

      {/* Reply input */}
      <ThreadReplyInput
        key={parentMessage.id}
        onSend={handleSendReply}
        disabled={!!error || isLoading}
        members={members}
      />
    </div>
  );
}
