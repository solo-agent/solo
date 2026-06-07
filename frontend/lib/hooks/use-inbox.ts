// ============================================================================
// useInbox — fetch inbox list with cursor pagination and filters (v1.5)
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
  const [typeFilter, setTypeFilter] = useState<string[]>([]);
  const [senderFilter, setSenderFilter] = useState('');
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
      if (typeFilter.length > 0) params.types = typeFilter.join(',');
      if (senderFilter) params.sender = senderFilter;

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
      if (err instanceof ApiError && !append) {
        // Only set empty on initial load error
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
        setIsLoadingMore(false);
      }
    }
  }, [typeFilter, senderFilter]);

  const loadMore = useCallback(() => {
    if (isLoadingMore || !hasMore || items.length === 0) return;
    const oldest = items[items.length - 1];
    if (oldest?.created_at) {
      fetchInbox(oldest.created_at, true);
    }
  }, [items, hasMore, isLoadingMore, fetchInbox]);

  // Initial fetch + refetch when filters change
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

  const markRead = useCallback(async (messageId: string) => {
    setItems((prev) =>
      prev.map((item) =>
        item.message_id === messageId ? { ...item, is_unread: false } : item,
      ),
    );
    try {
      await apiClient.post(`/api/v1/inbox/${messageId}/mark-read`);
    } catch {
      fetchInbox();
    }
  }, [fetchInbox]);

  const markAllRead = useCallback(async () => {
    try {
      await apiClient.post('/api/v1/inbox/mark-all-read');
      fetchInbox();
    } catch {
      // silently handle
    }
  }, [fetchInbox]);

  const clearAll = useCallback(async () => {
    setItems([]);
    setHasMore(false);
    try {
      await apiClient.post('/api/v1/inbox/clear-all');
      fetchInbox();
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
    markRead,
    markAllRead,
    clearAll,
    typeFilter,
    setTypeFilter,
    senderFilter,
    setSenderFilter,
  } as const;
}
