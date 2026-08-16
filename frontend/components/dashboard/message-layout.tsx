import { formatMessageDateLabel, messageDateKey } from '@/lib/utils/time';

interface GroupableMessage {
  sender_type?: string;
  user_id?: string;
  sender_id?: string;
  display_name?: string;
  task_number?: number;
  content_type?: string;
  status?: string;
  created_at: string;
}

const MESSAGE_GROUP_WINDOW_MS = 5 * 60 * 1000;

export function canGroupMessages(previous: GroupableMessage | undefined, current: GroupableMessage): boolean {
  if (!previous) return false;
  if (
    previous.sender_type === 'system'
    || current.sender_type === 'system'
    || previous.task_number != null
    || current.task_number != null
    || previous.content_type === 'channel_created'
    || current.content_type === 'channel_created'
    || current.status === 'streaming'
  ) return false;

  const previousTime = new Date(previous.created_at).getTime();
  const currentTime = new Date(current.created_at).getTime();
  return previous.sender_type === current.sender_type
    && (previous.user_id || previous.sender_id || previous.display_name)
      === (current.user_id || current.sender_id || current.display_name)
    && messageDateKey(previous.created_at) === messageDateKey(current.created_at)
    && Number.isFinite(previousTime)
    && Number.isFinite(currentTime)
    && currentTime >= previousTime
    && currentTime - previousTime < MESSAGE_GROUP_WINDOW_MS;
}

export function MessageDateSeparator({ createdAt }: { createdAt: string }) {
  return (
    <div
      className="flex items-center gap-3 px-6 py-4"
      role="separator"
      data-message-date-separator={messageDateKey(createdAt)}
    >
      <div className="flex-1 border-t border-brutal-muted" />
      <time
        dateTime={createdAt}
        className="flex-shrink-0 font-mono text-[10px] font-bold uppercase tracking-widest text-muted-foreground"
      >
        {formatMessageDateLabel(createdAt)}
      </time>
      <div className="flex-1 border-t border-brutal-muted" />
    </div>
  );
}
