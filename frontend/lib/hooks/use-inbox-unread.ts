// ============================================================================
// useInboxUnread — fetch and auto-refresh inbox unread count (v1.5)
// - GET /api/v1/inbox/unread-count
// - Auto-refetch on WS 'inbox.updated' event
// - Refetch on window focus (simpler than interval polling)
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { UnreadCount } from '@/lib/types';

export function useInboxUnread() {
  const [unreadCount, setUnreadCount] = useState<UnreadCount>({
    total: 0,
    mentions: 0,
    thread_replies: 0,
    dm: 0,
  });
  const [isLoading, setIsLoading] = useState(true);
  const mountedRef = useRef(true);
  const { onEvent } = useWebSocket();

  const fetchUnread = useCallback(async () => {
    try {
      const res = await apiClient.get<UnreadCount>('/api/v1/inbox/unread-count');
      if (mountedRef.current) {
        setUnreadCount(res);
      }
    } catch (err) {
      // Silently ignore — we keep the last known count
      if (!(err instanceof ApiError)) {
        // Network errors are fine to ignore for badge
      }
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    mountedRef.current = true;
    fetchUnread();
    return () => { mountedRef.current = false; };
  }, [fetchUnread]);

  // Refetch on WS 'inbox.updated' event
  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'inbox.updated') {
        fetchUnread();
      }
    });
    return unsub;
  }, [onEvent, fetchUnread]);

  // Refetch on window focus
  useEffect(() => {
    const handleFocus = () => {
      fetchUnread();
    };
    window.addEventListener('focus', handleFocus);
    return () => window.removeEventListener('focus', handleFocus);
  }, [fetchUnread]);

  return { unreadCount, isLoading } as const;
}
