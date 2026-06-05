// ============================================================================
// useInbox — fetch inbox list with cursor pagination (v1.5)
// - GET /api/v1/inbox?before=&limit=30
// - Auto-refetch on WS 'inbox.updated' event
// - Support load more (cursor pagination with `before` param)
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { InboxItem } from '@/lib/types';

interface InboxResponse {
  items: InboxItem[];
  has_more: boolean;
}

const DEFAULT_LIMIT = 30;

export function useInbox() {
  const [items, setItems] = useState<InboxItem[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const mountedRef = useRef(true);
  const { onEvent } = useWebSocket();

  const fetchInbox = useCallback(async (before?: string, append = false) => {
    const isLoadMore = append;
    if (isLoadMore) {
      setIsLoadingMore(true);
    } else {
      setIsLoading(true);
    }

    try {
      const params: Record<string, string> = { limit: String(DEFAULT_LIMIT) };
      if (before) params.before = before;

      const query = new URLSearchParams(params).toString();
      const res = await apiClient.get<InboxResponse>(`/api/v1/inbox?${query}`);

      if (mountedRef.current) {
        const newItems = Array.isArray(res.items) ? res.items : [];
        if (append) {
          setItems((prev) => [...prev, ...newItems]);
        } else {
          setItems(newItems);
        }
        setHasMore(res.has_more ?? false);
      }
    } catch (err) {
      // Silently handle — user can retry with load more
      if (err instanceof ApiError && !append) {
        // Only set empty on initial load error
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
        setIsLoadingMore(false);
      }
    }
  }, []);

  const loadMore = useCallback(() => {
    if (isLoadingMore || !hasMore || items.length === 0) return;
    const oldest = items[items.length - 1];
    if (oldest?.created_at) {
      fetchInbox(oldest.created_at, true);
    }
  }, [items, hasMore, isLoadingMore, fetchInbox]);

  // Initial fetch
  useEffect(() => {
    mountedRef.current = true;
    fetchInbox();
    return () => { mountedRef.current = false; };
  }, [fetchInbox]);

  // Refetch when inbox is updated via WS
  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'inbox.updated') {
        fetchInbox();
      }
    });
    return unsub;
  }, [onEvent, fetchInbox]);

  const dismissItem = useCallback(async (messageId: string) => {
    // Optimistic removal
    setItems((prev) => prev.filter((item) => item.message_id !== messageId));
    try {
      await apiClient.post(`/api/v1/inbox/${messageId}/dismiss`);
    } catch {
      // Refetch on failure to restore correct state
      fetchInbox();
    }
  }, [fetchInbox]);

  const dismissAll = useCallback(async () => {
    // Optimistic clear
    setItems([]);
    setHasMore(false);
    try {
      await apiClient.post('/api/v1/inbox/dismiss-all');
    } catch {
      fetchInbox();
    }
  }, [fetchInbox]);

  return {
    items,
    hasMore,
    isLoading,
    isLoadingMore,
    loadMore,
    refetch: fetchInbox,
    dismissItem,
    dismissAll,
  } as const;
}
