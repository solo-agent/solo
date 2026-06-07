'use client';

import { useState, useCallback, lazy, Suspense } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2, InboxIcon, Mail } from 'lucide-react';
import { useInbox } from '@/lib/hooks/use-inbox';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import { InboxItem } from './inbox-item';
import { TabBar } from '@/components/ui/tab-bar';
import type { TabBarTab } from '@/components/ui/tab-bar';
import type { InboxItem as InboxItemType, Message } from '@/lib/types';

const ThreadPanel = lazy(() =>
  import('@/components/dashboard/thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

const INBOX_TABS: TabBarTab[] = [
  { key: 'all', label: '全部' },
  { key: 'mention', label: '@提及' },
  { key: 'thread_reply', label: '话题回复' },
  { key: 'dm', label: '私信' },
];

const KEY_TO_TYPE_FILTER: Record<string, string[]> = {
  all: [],
  mention: ['mention'],
  thread_reply: ['thread_reply'],
  dm: ['dm'],
};

function typeFilterToKey(tf: string[]) {
  if (tf.length === 0) return 'all';
  return tf[0];
}

export function InboxView() {
  const router = useRouter();
  const { items, hasMore, isLoading, isLoadingMore, loadMore, markRead, markAllRead, clearAll, typeFilter, setTypeFilter, senderFilter, setSenderFilter } = useInbox();
  useInboxUnread();

  const handleClearAll = useCallback(async () => {
    await clearAll();
    window.dispatchEvent(new Event('inbox-refresh-unread'));
  }, [clearAll]);

  const handleMarkAllRead = useCallback(async () => {
    await markAllRead();
    window.dispatchEvent(new Event('inbox-refresh-unread'));
  }, [markAllRead]);

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);

  const handleItemClick = useCallback(
    async (item: InboxItemType) => {
      // Mark this specific message as read, then update badge
      await markRead(item.message_id);
      window.dispatchEvent(new Event('inbox-refresh-unread'));

      switch (item.type) {
        case 'thread_reply':
          if (item.channel_id && item.thread_id) {
            const isAgent = item.parent_sender_type === 'agent';
            setThreadMessage({
              id: item.thread_id,
              channel_id: item.channel_id,
              user_id: item.parent_sender_id || '',
              display_name: item.parent_sender_name || '',
              content: item.parent_content || '',
              created_at: item.created_at,
              status: 'sent',
              sender_type: isAgent ? 'agent' : 'user',
            });
          }
          break;
        case 'dm':
          if (item.dm_id) {
            router.push(`/dashboard?dm=${item.dm_id}&message=${item.message_id}`);
          }
          break;
        case 'mention':
          if (item.channel_id) {
            router.push(`/dashboard?channel=${item.channel_id}&message=${item.message_id}`);
          }
          break;
      }
    },
    [router, markRead],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
  }, []);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <Mail className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
            <h2 className="font-bold text-foreground truncate">Inbox</h2>
          </div>
          {items.length > 0 && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleMarkAllRead}
                className="border-2 border-black bg-white px-3 py-1 text-xs font-heading font-bold shadow-brutal-sm hover:bg-brutal-cream active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
              >
                全部已读
              </button>
              <button
                type="button"
                onClick={handleClearAll}
                className="border-2 border-black bg-brutal-pink px-3 py-1 text-xs font-heading font-bold text-black shadow-brutal-sm hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
              >
                清空
              </button>
            </div>
          )}
        </div>

        {/* Filter bar */}
        <TabBar
          tabs={INBOX_TABS}
          activeKey={typeFilterToKey(typeFilter)}
          onChange={(key) => setTypeFilter(KEY_TO_TYPE_FILTER[key])}
          variant="pill"
        >
          <input
            type="text"
            placeholder="筛选发送者..."
            value={senderFilter}
            onChange={(e) => setSenderFilter(e.target.value)}
            className="ml-auto h-7 w-36 text-xs border-2 border-black bg-white px-2 font-body shadow-brutal-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-brutal-info"
          />
        </TabBar>

        <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : items.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center px-4">
              <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
                <InboxIcon className="h-8 w-8 text-black" />
              </div>
              <h2 className="font-heading text-lg font-bold text-foreground">
                没有新消息
              </h2>
              <p className="mt-1 font-body text-sm text-muted-foreground">
                当有人在你的话题中回复、向你发送私信或 @提及你时，消息会出现在这里
              </p>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto">
              {items.map((item) => (
                <InboxItem
                  key={item.id}
                  item={item}
                  onClick={handleItemClick}
                />
              ))}

              {hasMore && (
                <div className="flex justify-center py-4">
                  <button
                    type="button"
                    onClick={loadMore}
                    disabled={isLoadingMore}
                    className="text-xs font-medium text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
                  >
                    {isLoadingMore ? (
                      <span className="flex items-center gap-1.5">
                        <Loader2 className="h-3 w-3 animate-spin" />
                        加载中...
                      </span>
                    ) : (
                      '加载更多'
                    )}
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* ThreadPanel */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] border-l-2 border-transparent"
        style={{ width: threadMessage ? threadPanelWidth : 0, borderLeftColor: threadMessage ? '#000' : 'transparent' }}
      >
        {threadMessage && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-pink/50 transition-colors z-10"
            onMouseDown={(e) => {
              e.preventDefault();
              const startX = e.clientX;
              const startWidth = threadPanelWidth;
              const onMove = (ev: MouseEvent) => {
                const newWidth = Math.max(280, Math.min(800, startWidth + startX - ev.clientX));
                setThreadPanelWidth(newWidth);
              };
              const onUp = () => {
                document.removeEventListener('mousemove', onMove);
                document.removeEventListener('mouseup', onUp);
              };
              document.addEventListener('mousemove', onMove);
              document.addEventListener('mouseup', onUp);
            }}
          />
        )}
        {threadMessage && (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            }
          >
            <ThreadPanel
              parentMessage={threadMessage}
              onClose={handleThreadClose}
              replyCount={0}
            />
          </Suspense>
        )}
      </div>
    </div>
  );
}
