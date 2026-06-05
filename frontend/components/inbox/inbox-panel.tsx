// ============================================================================
// InboxPanel — dropdown/panel that appears below the inbox badge (v1.5)
// - Header: "Inbox" title + "Mark all read" button
// - Scrollable list of InboxItems
// - Load more at bottom (if hasMore)
// - Empty state: "No new messages" with appropriate icon
// ============================================================================

'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { CheckCheck, Loader2, InboxIcon } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { useInbox } from '@/lib/hooks/use-inbox';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import { InboxItem } from './inbox-item';
import type { InboxItem as InboxItemType } from '@/lib/types';

// ---- Props ----

interface InboxPanelProps {
  onClose: () => void;
  onMarkRead: () => void;
}

// ---- Component ----

export function InboxPanel({ onClose, onMarkRead }: InboxPanelProps) {
  const router = useRouter();
  const { items, hasMore, isLoading, isLoadingMore, loadMore } = useInbox();
  const { unreadCount } = useInboxUnread();

  const handleItemClick = useCallback(
    (item: InboxItemType) => {
      // Navigate based on item type
      switch (item.type) {
        case 'thread_reply':
          if (item.channel_id && item.thread_id) {
            router.push(`/dashboard?channel=${item.channel_id}&thread=${item.thread_id}`);
          } else if (item.channel_id) {
            router.push(`/dashboard?channel=${item.channel_id}`);
          }
          break;
        case 'dm':
          if (item.dm_id) {
            router.push(`/dashboard?dm=${item.dm_id}`);
          }
          break;
        case 'mention':
          if (item.channel_id) {
            router.push(`/dashboard?channel=${item.channel_id}`);
          }
          break;
      }
      onClose();
    },
    [router, onClose],
  );

  const handleMarkAllRead = useCallback(async () => {
    try {
      await apiClient.post('/api/v1/inbox/mark-read');
      onMarkRead();
    } catch {
      // Silently handle
    }
  }, [onMarkRead]);

  return (
    <div
      className="border-2 border-black bg-white shadow-brutal-lg animate-slide-in-from-left"
      role="dialog"
      aria-label="收件箱"
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b-2 border-black px-3 py-2.5">
        <h3 className="font-heading text-sm font-bold text-foreground">
          Inbox
        </h3>
        {unreadCount.total > 0 && (
          <button
            type="button"
            onClick={handleMarkAllRead}
            className="flex items-center gap-1 text-[11px] font-medium text-muted-foreground hover:text-foreground transition-colors"
          >
            <CheckCheck className="h-3.5 w-3.5" />
            Mark all read
          </button>
        )}
      </div>

      {/* Content */}
      <div className="max-h-[400px] overflow-y-auto">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 px-4">
            <div className="mb-3 flex h-10 w-10 items-center justify-center border-2 border-black bg-brutal-cream">
              <InboxIcon className="h-5 w-5 text-muted-foreground" />
            </div>
            <p className="text-center font-body text-sm text-muted-foreground">
              没有新消息
            </p>
          </div>
        ) : (
          <>
            {items.map((item) => (
              <InboxItem
                key={item.id}
                item={item}
                onClick={handleItemClick}
              />
            ))}

            {/* Load more */}
            {hasMore && (
              <div className="px-2 py-2">
                <button
                  type="button"
                  onClick={loadMore}
                  disabled={isLoadingMore}
                  className="flex w-full items-center justify-center gap-1.5 py-1.5 text-xs font-medium text-muted-foreground hover:text-foreground hover:bg-brutal-cream transition-colors disabled:opacity-50"
                >
                  {isLoadingMore ? (
                    <>
                      <Loader2 className="h-3 w-3 animate-spin" />
                      加载中...
                    </>
                  ) : (
                    '加载更多'
                  )}
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
