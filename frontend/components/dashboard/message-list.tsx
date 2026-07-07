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
  useEffect,
  useRef,
  useState,
  useLayoutEffect,
  memo,
  useCallback,
  type ReactNode,
} from 'react';
import {
  AlertCircle,
  RefreshCw,
  ChevronDown,
  Loader2,
  MessageSquare,
  Pencil,
  Trash2,
  SquareCheckBig,
  Sparkles,
  Check,
  RotateCcw,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { apiClient } from '@/lib/api-client';
import { buildValidNames } from '@/lib/utils/highlight';
import { Avatar } from '@/components/ui/avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { AgentMessage } from './agent-message';
import { StreamingMessage } from './streaming-message';
import { MessageAttachments } from './message-attachments';
import type { AgentDetailTarget, ChannelMember, Message } from '@/lib/types';
import { sanitizeHtml } from '@/lib/sanitize';
import { t } from '@/lib/i18n';
// SOLO-island PR2: TypingIndicator removed — AgentIsland (mounted at the
// dashboard root) now surfaces agent status. The unused import is removed
// along with the agentActivities prop and the inline <TypingIndicator />.
interface MessageListProps {
  messages: Message[];
  beforeItems?: ReactNode;
  showBeginningMarker?: boolean;
  isLoading: boolean;
  error: string | null;
  onRetry: (messageId: string, content: string) => void;
  onCancel?: (messageId: string) => void;
  onReply?: (message: Message) => void;
  onEdit?: (messageId: string, content: string) => void;
  onDelete?: (messageId: string) => void;
  onAsTask?: (message: Message) => void;
  onCreateChannelFromCard?: (message: Message, input: { channel_name: string; template: string }) => Promise<void>;
  onStartWorkFromCard?: (message: Message) => Promise<void>;
  onCompleteThoughtFromCard?: (message: Message, thoughtId: string) => Promise<void>;
  onTaskReviewAction?: () => Promise<void> | void;
  onViewTaskGraph?: () => void;
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
    bgClass: 'bg-brutal-info-light/20',
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
  isHighlighted?: boolean;
  onRetry: (id: string, content: string) => void;
  onCancel?: (id: string) => void;
  onReply?: (message: Message) => void;
  onEdit?: (id: string, content: string) => void;
  onDelete?: (id: string) => void;
  onAsTask?: (message: Message) => void;
  onCreateChannelFromCard?: (message: Message, input: { channel_name: string; template: string }) => Promise<void>;
  onStartWorkFromCard?: (message: Message) => Promise<void>;
  onCompleteThoughtFromCard?: (message: Message, thoughtId: string) => Promise<void>;
  onTaskReviewAction?: () => Promise<void> | void;
  onViewTaskGraph?: () => void;
}

interface ChannelCreateCardPayload {
  card_type: 'channel_create';
  channel_name: string;
  template: string;
  target: string;
  status?: string;
  agents?: Array<{ name: string; role: string }>;
  agenda?: Array<{ id: string; title: string; status: string; children?: Array<{ id: string; title: string; status: string }> }>;
}

function parseChannelCreateCard(message: Message): ChannelCreateCardPayload | null {
  if (message.content_type !== 'card.channel_create') return null;
  try {
    const payload = JSON.parse(message.content) as ChannelCreateCardPayload;
    return payload.card_type === 'channel_create' ? payload : null;
  } catch {
    return null;
  }
}

interface NextStepCardPayload {
  card_type: 'next_step';
  target: string;
  status?: string;
}

interface ThoughtReviewCardPayload {
  card_type: 'thought_review';
  thought_id: string;
  title: string;
  summary?: string;
  status?: string;
}

interface TasksCreatedCardPayload {
  card_type: 'tasks_created';
  title: string;
  status?: string;
  tasks?: Array<{ id: string; task_number?: number; title: string; status: string; parent_task_id?: string | null }>;
}

interface TaskReviewCardPayload {
  card_type: 'task_review';
  task_id: string;
  task_number?: number;
  title: string;
  task_status?: string;
  artifact_status?: string;
  status?: string;
}

function parseNextStepCard(message: Message): NextStepCardPayload | null {
  if (message.content_type !== 'card.next_step') return null;
  try {
    const payload = JSON.parse(message.content) as NextStepCardPayload;
    return payload.card_type === 'next_step' ? payload : null;
  } catch {
    return null;
  }
}

function parseThoughtReviewCard(message: Message): ThoughtReviewCardPayload | null {
  if (message.content_type !== 'card.thought_review') return null;
  try {
    const payload = JSON.parse(message.content) as ThoughtReviewCardPayload;
    return payload.card_type === 'thought_review' ? payload : null;
  } catch {
    return null;
  }
}

function parseTasksCreatedCard(message: Message): TasksCreatedCardPayload | null {
  if (message.content_type !== 'card.tasks_created') return null;
  try {
    const payload = JSON.parse(message.content) as TasksCreatedCardPayload;
    return payload.card_type === 'tasks_created' ? payload : null;
  } catch {
    return null;
  }
}

function parseTaskReviewCard(message: Message): TaskReviewCardPayload | null {
  if (message.content_type !== 'card.task_review') return null;
  try {
    const payload = JSON.parse(message.content) as TaskReviewCardPayload;
    return payload.card_type === 'task_review' ? payload : null;
  } catch {
    return null;
  }
}

function ChannelCreateCard({
  message,
  payload,
  onCreate,
}: {
  message: Message;
  payload: ChannelCreateCardPayload;
  onCreate?: (message: Message, input: { channel_name: string; template: string }) => Promise<void>;
}) {
  const [isEditing, setIsEditing] = useState(false);
  const [channelName, setChannelName] = useState(payload.channel_name);
  const [template, setTemplate] = useState(payload.template);
  const [busy, setBusy] = useState(false);
  const accepted = payload.status === 'accepted';

  const submit = async () => {
    if (!onCreate || busy || accepted) return;
    setBusy(true);
    try {
      await onCreate(message, {
        channel_name: channelName.trim() || payload.channel_name,
        template: template.trim() || payload.template,
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-2 max-w-2xl border-2 border-black bg-white shadow-brutal">
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-primary px-3 py-2">
        <div className="flex items-center gap-2 font-heading text-sm font-black">
          <Sparkles className="h-4 w-4" />
          Channel Create
        </div>
        <span className="border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
          {accepted ? 'created' : 'matched'}
        </span>
      </div>
      <div className="space-y-3 p-3">
        <p className="font-body text-sm leading-6 text-foreground">{payload.target}</p>

        <div className="grid gap-2 sm:grid-cols-2">
          <label className="space-y-1">
            <span className="font-heading text-[10px] font-black uppercase text-muted-foreground">Channel</span>
            {isEditing ? (
              <input
                value={channelName}
                onChange={(e) => setChannelName(e.target.value)}
                className="input-brutal h-9 text-sm"
              />
            ) : (
              <div className="border-2 border-black bg-brutal-cream px-2 py-1.5 font-mono text-sm font-bold">
                #{channelName}
              </div>
            )}
          </label>
          <label className="space-y-1">
            <span className="font-heading text-[10px] font-black uppercase text-muted-foreground">Template</span>
            {isEditing ? (
              <select
                value={template}
                onChange={(e) => setTemplate(e.target.value)}
                className="input-brutal h-9 text-sm"
              >
                <option>Solo Project</option>
                <option>Research Project</option>
                <option>Conversation Bot</option>
              </select>
            ) : (
              <div className="border-2 border-black bg-brutal-cream px-2 py-1.5 font-mono text-sm font-bold">
                {template}
              </div>
            )}
          </label>
        </div>

        <div className="flex flex-wrap gap-1.5">
          {(payload.agents ?? []).map((agent) => (
            <span key={`${agent.name}-${agent.role}`} className="border-2 border-black bg-brutal-info-light px-2 py-1 font-mono text-[10px] font-bold uppercase shadow-brutal-sm">
              {agent.name} · {agent.role}
            </span>
          ))}
        </div>

        <div className="flex items-center gap-2">
          <Button data-testid="channel-create-card-submit" size="sm" variant="default" disabled={!onCreate || busy || accepted} onClick={submit}>
            {busy ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
            {accepted ? 'Channel created' : 'Create channel'}
          </Button>
          {!accepted && (
            <Button size="sm" variant="outline" onClick={() => setIsEditing((v) => !v)}>
              {isEditing ? 'Done editing' : 'Edit channel'}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

function NextStepCard({
  message,
  payload,
  onStartWork,
}: {
  message: Message;
  payload: NextStepCardPayload;
  onStartWork?: (message: Message) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const accepted = payload.status === 'accepted';

  const startWork = async () => {
    if (!onStartWork || busy || accepted) return;
    setBusy(true);
    try {
      await onStartWork(message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-2 max-w-2xl border-2 border-black bg-white shadow-brutal">
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-info-light px-3 py-2">
        <div className="flex items-center gap-2 font-heading text-sm font-black">
          <SquareCheckBig className="h-4 w-4" />
          Next Step
        </div>
        <span className="border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
          {accepted ? 'started' : 'ready'}
        </span>
      </div>
      <div className="space-y-3 p-3">
        <p className="font-body text-sm leading-6 text-foreground">
          Lucy 已经有足够上下文。下一步可以直接开工，生成 tasks 并分配给 team agents。
        </p>
        <div className="border-2 border-black bg-brutal-cream px-3 py-2 font-body text-sm">
          {payload.target}
        </div>
        <div className="flex items-center gap-2">
          <Button data-testid="next-step-start-work" size="sm" variant="default" disabled={!onStartWork || busy || accepted} onClick={startWork}>
            {busy ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
            {accepted ? 'Started' : '开始行动'}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ThoughtReviewCard({
  message,
  payload,
  onComplete,
}: {
  message: Message;
  payload: ThoughtReviewCardPayload;
  onComplete?: (message: Message, thoughtId: string) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const accepted = payload.status === 'accepted';

  const complete = async () => {
    if (!onComplete || busy || accepted) return;
    setBusy(true);
    try {
      await onComplete(message, payload.thought_id);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-2 max-w-2xl border-2 border-black bg-white shadow-brutal">
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-success-light px-3 py-2">
        <div className="flex items-center gap-2 font-heading text-sm font-black">
          <SquareCheckBig className="h-4 w-4" />
          Thought Review
        </div>
        <span className="border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
          {accepted ? 'done' : 'review'}
        </span>
      </div>
      <div className="space-y-3 p-3">
        <div>
          <div className="font-heading text-sm font-black">{payload.title}</div>
          {payload.summary && (
            <p className="mt-1 font-body text-sm leading-6 text-muted-foreground">{payload.summary}</p>
          )}
        </div>
        <Button data-testid="thought-review-done" size="sm" variant="success" disabled={!onComplete || busy || accepted} onClick={complete}>
          {busy ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
          {accepted ? 'Done' : 'Done'}
        </Button>
      </div>
    </div>
  );
}

function TasksCreatedCard({
  payload,
  onViewTaskGraph,
}: {
  payload: TasksCreatedCardPayload;
  onViewTaskGraph?: () => void;
}) {
  const tasks = payload.tasks ?? [];

  return (
    <div className="mt-2 max-w-2xl border-2 border-black bg-white shadow-brutal">
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-warning px-3 py-2">
        <div className="flex items-center gap-2 font-heading text-sm font-black">
          <SquareCheckBig className="h-4 w-4" />
          Tasks Created
        </div>
        <span className="border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
          {tasks.length} tasks
        </span>
      </div>
      <div className="space-y-3 p-3">
        <div className="font-heading text-sm font-black">{payload.title}</div>
        <div className="space-y-1.5">
          {tasks.map((task) => (
            <div key={task.id} className="flex items-center justify-between gap-2 border-2 border-black bg-brutal-cream px-2 py-1.5 shadow-brutal-sm">
              <span className="truncate font-body text-sm font-bold">
                {task.task_number ? `#${task.task_number} ` : ''}{task.title}
              </span>
              <span className="shrink-0 font-mono text-[10px] font-bold uppercase text-muted-foreground">
                {task.status.replace('_', ' ')}
              </span>
            </div>
          ))}
        </div>
        <Button data-testid="tasks-created-view-graph" size="sm" variant="default" onClick={onViewTaskGraph}>
          View Task graph
        </Button>
      </div>
    </div>
  );
}

function TaskReviewCard({
  payload,
  onAction,
}: {
  payload: TaskReviewCardPayload;
  onAction?: () => Promise<void> | void;
}) {
  const [busy, setBusy] = useState<'accept' | 'reject' | null>(null);
  const [localStatus, setLocalStatus] = useState(payload.task_status ?? 'in_review');
  const [error, setError] = useState<string | null>(null);
  const canReview = localStatus === 'in_review';

  const run = async (action: 'accept' | 'reject') => {
    if (busy || !canReview) return;
    setBusy(action);
    setError(null);
    try {
      await apiClient.post(
        `/api/v1/tasks/${payload.task_id}/${action}`,
        action === 'reject' ? { reason: 'Needs refinement from review card.' } : undefined,
      );
      setLocalStatus(action === 'accept' ? 'done' : 'in_progress');
      await onAction?.();
    } catch {
      setError('Review action failed.');
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="mt-2 max-w-2xl border-2 border-black bg-white shadow-brutal">
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-violet-light px-3 py-2">
        <div className="flex items-center gap-2 font-heading text-sm font-black">
          <SquareCheckBig className="h-4 w-4" />
          Review Task
        </div>
        <span className="border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
          {localStatus.replace('_', ' ')}
        </span>
      </div>
      <div className="space-y-3 p-3">
        <div>
          <div className="font-heading text-sm font-black">
            {payload.task_number ? `#${payload.task_number} ` : ''}{payload.title}
          </div>
          <p className="mt-1 font-body text-sm leading-6 text-muted-foreground">
            验收后 task 进入 Done，并更新 project memory。
          </p>
        </div>
        {payload.artifact_status && payload.artifact_status !== 'none' ? (
          <div className="inline-flex border-2 border-black bg-brutal-cream px-2 py-1 font-mono text-[10px] font-bold uppercase">
            artifact {payload.artifact_status}
          </div>
        ) : null}
        <div className="flex items-center gap-2">
          <Button data-testid="task-review-accept" size="sm" variant="success" disabled={!canReview || Boolean(busy)} onClick={() => run('accept')}>
            {busy === 'accept' ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : <Check className="mr-1 h-3.5 w-3.5" />}
            Accept
          </Button>
          <Button data-testid="task-review-reject" size="sm" variant="danger" disabled={!canReview || Boolean(busy)} onClick={() => run('reject')}>
            {busy === 'reject' ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="mr-1 h-3.5 w-3.5" />}
            Reject
          </Button>
        </div>
        {error ? <div className="font-mono text-[10px] font-bold uppercase text-brutal-danger">{error}</div> : null}
      </div>
    </div>
  );
}

const MessageItem = memo(function MessageItem({
  message,
  isHighlighted,
  onRetry,
  onCancel,
  onReply,
  onEdit,
  onDelete,
  onAsTask,
  onCreateChannelFromCard,
  onStartWorkFromCard,
  onCompleteThoughtFromCard,
  onTaskReviewAction,
  onViewTaskGraph,
}: MessageItemProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState(message.content || '');
  const [isSaving, setIsSaving] = useState(false);
  const [isHovered, setIsHovered] = useState(false);
  const editRef = useRef<HTMLTextAreaElement>(null);

  const isFailed = message.status === 'failed';
  const isSending = message.status === 'sending';

  // SOLO-225-F: task message detection
  const taskStatus = message.task_status as string | undefined;
  const isTaskMessage = message.task_number != null && taskStatus != null;
  const headerConfig = isTaskMessage && taskStatus ? TASK_HEADER_CONFIG[taskStatus] : null;
  const channelCreateCard = parseChannelCreateCard(message);
  const nextStepCard = parseNextStepCard(message);
  const thoughtReviewCard = parseThoughtReviewCard(message);
  const tasksCreatedCard = parseTasksCreatedCard(message);
  const taskReviewCard = parseTaskReviewCard(message);

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

  const time = new Date(message.created_at).toLocaleString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });

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

  return (
    <div
      data-message-id={message.id}
      className={cn(
        'group relative flex gap-3 px-6 py-2.5 transition-colors border-b border-brutal-muted',
        !isTaskMessage && 'hover:bg-brutal-muted/15',
        isFailed && 'bg-brutal-danger-light/30',
        isEditing && 'border-l-[3px] border-l-brutal-primary bg-brutal-primary-light/30',
        isHighlighted && 'bg-brutal-info-light ring-2 ring-black',
        isTaskMessage && 'border-l-4',
        isTaskMessage && headerConfig?.accentClass,
        isTaskMessage && headerConfig?.bgClass,
        isTaskMessage && 'cursor-pointer',
      )}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      role="listitem"
      onClick={isTaskMessage && onReply ? () => onReply(message) : undefined}
      onKeyDown={isTaskMessage && onReply ? (e) => { if (e.key === 'Enter') onReply(message); } : undefined}
      tabIndex={isTaskMessage ? 0 : undefined}
      aria-label={isTaskMessage ? `Task #${message.task_number} — ${headerConfig?.label || ''}` : undefined}
    >
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

      <Avatar
        name={message.display_name}
        className="mt-0.5 h-8 w-8 flex-shrink-0"
      />

      <div className="min-w-0 flex-1">
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
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-foreground">
            {message.display_name}
          </span>
          {message.sender_type === 'agent' && message.sender_active === false && (
            <span className="badge-brutal bg-brutal-muted text-black">
              {t('deleted')}
            </span>
          )}
          <span className="font-mono text-[11px] text-muted-foreground">
            {time}
          </span>
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
                <span className="font-mono text-[11px] text-muted-foreground">{t('savingMessage')}</span>
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
        ) : channelCreateCard ? (
          <ChannelCreateCard
            message={message}
            payload={channelCreateCard}
            onCreate={onCreateChannelFromCard}
          />
        ) : nextStepCard ? (
          <NextStepCard
            message={message}
            payload={nextStepCard}
            onStartWork={onStartWorkFromCard}
          />
        ) : thoughtReviewCard ? (
          <ThoughtReviewCard
            message={message}
            payload={thoughtReviewCard}
            onComplete={onCompleteThoughtFromCard}
          />
        ) : tasksCreatedCard ? (
          <TasksCreatedCard
            payload={tasksCreatedCard}
            onViewTaskGraph={onViewTaskGraph}
          />
        ) : taskReviewCard ? (
          <TaskReviewCard
            payload={taskReviewCard}
            onAction={onTaskReviewAction}
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

        {/* Task claimer + reply badges */}
        {(isTaskMessage || (message.reply_count ?? 0) > 0) && (
        <div className="mt-2 flex items-center gap-2">
          {isTaskMessage && headerConfig && (
            <span className={cn('badge-brutal', headerConfig.badgeClass)}>
              {message.task_claimer_name ? (
                <>@{message.task_claimer_name}{message.task_claimer_deleted ? ` (${t('deleted')})` : ''}</>
              ) : (
                t('unclaimed')
              )}
            </span>
          )}

        {/* Thread reply count — brutalist badge */}
        {(message.reply_count ?? 0) > 0 && onReply && (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onReply(message); }}
            className={cn(
              'badge-brutal cursor-pointer transition-all',
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
        )}
      </div>

      {/* Hover actions: edit / delete / reply */}
      {!isEditing && !isFailed && !isSending && (
        <div className="absolute right-3 top-2 flex items-center gap-1
                        opacity-0 group-hover:opacity-100
                        translate-x-2 group-hover:translate-x-0
                        transition-all duration-200">
          {onEdit && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setEditContent(message.content || '');
                setIsEditing(true);
              }}
              className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
              aria-label={t('editMessage')}
              title={t('edit')}
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
          )}
          {onDelete && (
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); onDelete(message.id); }}
              className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
              aria-label={t('deleteMessage')}
              title={t('delete')}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}
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
          {onAsTask && message.sender_type !== 'system' && !isTaskMessage && (
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); onAsTask(message); }}
              className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
              aria-label={t('convertToTask')}
              title={t('convertToTask')}
            >
              <SquareCheckBig className="h-3.5 w-3.5" />
            </button>
          )}
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
    <div className="flex flex-1 items-center justify-center">
      <div className="text-center">
        <p className="font-body text-sm text-muted-foreground">
          {t('noMessages')}
        </p>
      </div>
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
        className="btn-brutal btn-brutal-sm h-8 gap-1 px-2.5 text-xs"
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
      <span className="font-mono text-xs text-brutal-danger">{t('loadError')}</span>
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

// ---- Main component ----

export function MessageList({
  messages,
  beforeItems,
  showBeginningMarker = true,
  isLoading,
  error,
  onRetry,
  onCancel,
  onReply,
  onEdit,
  onDelete,
  onAsTask,
  onCreateChannelFromCard,
  onStartWorkFromCard,
  onCompleteThoughtFromCard,
  onTaskReviewAction,
  onViewTaskGraph,
  hasMore,
  isLoadingMore,
  loadMoreError,
  onLoadMore,
  scrollToMessageId,
  scrollKey,
  members = [],
  onOpenArtifactReference,
  onAgentClick,
}: MessageListProps) {
  const validNames = buildValidNames(members);
  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const prevMessageCountRef = useRef(0);
  const scrollRestoreRef = useRef<number | null>(null);
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null);

  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    displayName: string;
  } | null>(null);

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
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
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
        el.scrollIntoView({ behavior: 'smooth', block: 'center' });
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
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
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
  if (messages.length === 0 && !beforeItems) {
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

        {showBeginningMarker && !hasMore && !isLoadingMore && !loadMoreError && (
          <ChannelBeginning />
        )}

        <div className="py-4 space-y-1">
          {beforeItems}

          {messages.map((message) =>
            message.status === 'streaming' ? (
              <StreamingMessage
                key={message.id}
                message={message}
                onAgentClick={onAgentClick}
              />
            ) : message.sender_type === 'agent' ? (
              <AgentMessage
                key={message.id}
                message={message}
                onReply={onReply}
                validNames={validNames}
                isHighlighted={highlightedMessageId === message.id}
                onOpenArtifactReference={onOpenArtifactReference}
                onAgentClick={onAgentClick}
              />
            ) : (
              <MessageItem
                key={message.id}
                message={message}
                isHighlighted={highlightedMessageId === message.id}
                onRetry={onRetry}
                onCancel={onCancel}
                onReply={onReply}
                onEdit={onEdit}
                onAsTask={onAsTask}
                onCreateChannelFromCard={onCreateChannelFromCard}
                onStartWorkFromCard={onStartWorkFromCard}
                onCompleteThoughtFromCard={onCompleteThoughtFromCard}
                onTaskReviewAction={onTaskReviewAction}
                onViewTaskGraph={onViewTaskGraph}
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
            ),
          )}
        </div>

        {/* SOLO-island PR2: TypingIndicator removed — AgentIsland
            (mounted at the dashboard root) is the new home for
            "agent is working" status. */}

        <div ref={bottomRef} />
      </div>

      {!isAtBottom && messages.length > 0 && (
        <ScrollToBottom onClick={scrollToBottom} />
      )}

      {/* Delete confirmation dialog */}
      <DeleteConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        onConfirm={handleDeleteConfirm}
        messageAuthor={deleteTarget?.displayName ?? ''}
      />
    </div>
  );
}
