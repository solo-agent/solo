// ============================================================================
// SOLO-56-F: useDMMessages — DM message hook (mock data + REST API pattern)
// ============================================================================
// Follows the same pattern as useMessages but for DM conversations.
// Backend API not yet available — uses mock data.
// Future API endpoints:
//   GET  /api/v1/dms/{dmId}/messages    → list messages
//   POST /api/v1/dms/{dmId}/messages    → send a message
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { Message } from '@/lib/types';

// ---- Constants ----

const PAGE_SIZE = 50;

// ---- Mock data ----

function createMockMessage(
  id: string,
  dmId: string,
  content: string,
  senderName: string,
  senderType: 'user' | 'agent' | 'system',
  minutesAgo: number,
): Message {
  return {
    id,
    channel_id: dmId,
    user_id: senderType === 'user' ? 'user-1' : senderType === 'agent' ? 'agent-1' : 'system',
    display_name: senderName,
    content,
    created_at: new Date(Date.now() - 1000 * 60 * minutesAgo).toISOString(),
    status: 'sent',
    sender_type: senderType,
  };
}

const MOCK_MESSAGES: Record<string, Message[]> = {
  'dm-1': [
    createMockMessage('dm1-m1', 'dm-1', 'Hey Alice', 'Me', 'user', 120),
    createMockMessage('dm1-m2', 'dm-1', 'Hi! Are you joining the project sync tomorrow?', 'Alice', 'user', 115),
    createMockMessage('dm1-m3', 'dm-1', 'Yes, I\'ll have the progress report ready', 'Me', 'user', 110),
    createMockMessage('dm1-m4', 'dm-1', 'Great, let\'s meet in the conference room at 2pm', 'Alice', 'user', 60),
    createMockMessage('dm1-m5', 'dm-1', 'Don\'t forget the meeting tomorrow afternoon', 'Alice', 'user', 5),
  ],
  'dm-2': [
    createMockMessage('dm2-m1', 'dm-2', 'This is your DM with AI Assistant', 'System', 'system', 120),
    createMockMessage('dm2-m2', 'dm-2', 'Hey, can you help me analyze some data?', 'Me', 'user', 60),
    createMockMessage('dm2-m3', 'dm-2', 'Sure! Send me the data and I\'ll take a look', 'AI Assistant', 'agent', 58),
    createMockMessage('dm2-m4', 'dm-2', 'Here\'s a sales report', 'Me', 'user', 40),
    createMockMessage('dm2-m5', 'dm-2', 'I\'ve analyzed the data. Here\'s the detailed report...', 'AI Assistant', 'agent', 30),
  ],
};

// ---- Hook ----

export function useDMMessages(dmId: string | null) {
  const { onEvent } = useWebSocket();
  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);
  const loadingRef = useRef(false);
  const loadingMoreRef = useRef(false);
  const dmIdRef = useRef<string | null>(null);

  const messagesRef = useRef<Message[]>([]);
  messagesRef.current = messages;

  // ---- Initial load ----

  const loadMessages = useCallback(async (id: string) => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setIsLoading(true);
    setError(null);

    try {
      // TODO: Replace with real API call:
      //   const res = await apiClient.get<MessageListResponse>(
      //     `/api/v1/dms/${id}/messages`,
      //     { limit: String(PAGE_SIZE) },
      //   );
      await new Promise((r) => setTimeout(r, 400));

      const mockMsgs = MOCK_MESSAGES[id] ?? [];
      // Mock has_more: pretend there are more if we have fewer than PAGE_SIZE (with mock we always say false)
      setMessages(mockMsgs);
      setHasMore(false);
      dmIdRef.current = id;
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('dmMessageLoadError')}`;
      setError(message);
    } finally {
      setIsLoading(false);
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => {
    if (dmId) {
      setMessages([]);
      setHasMore(false);
      loadMessages(dmId);
    } else {
      setMessages([]);
      setHasMore(false);
      dmIdRef.current = null;
    }
  }, [dmId, loadMessages]);

  // ---- Load older messages (for future use) ----

  const loadMore = useCallback(async () => {
    const id = dmIdRef.current;
    if (!id || !hasMore || loadingMoreRef.current) return;

    loadingMoreRef.current = true;
    setIsLoadingMore(true);
    setLoadMoreError(null);

    try {
      const currentMsgs = messagesRef.current;
      if (currentMsgs.length === 0) {
        setHasMore(false);
        return;
      }

      const oldestId = currentMsgs[0].id;
      // TODO: Replace with real API:
      // const res = await apiClient.get(...)
      await new Promise((r) => setTimeout(r, 400));
      setHasMore(false);
    } catch {
      setLoadMoreError(`${t('dmEarlierMessageError')}`);
    } finally {
      setIsLoadingMore(false);
      loadingMoreRef.current = false;
    }
  }, [hasMore]);

  // ---- Listen for WS events ----

  useEffect(() => {
    if (!dmId) return;

    const unsub = onEvent((event) => {
      const cid = dmIdRef.current;
      if (!cid) return;

      if (event.type === 'message.new') {
        if (event.channel_id !== cid) return;
        if (event.thread_id) return;

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          if (existing) {
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id
                  ? {
                      id: event.id,
                      channel_id: event.channel_id,
                      user_id: event.sender_id,
                      display_name: event.sender_name || event.sender_id,
                      content: event.content,
                      created_at: event.created_at,
                      status: 'sent' as const,
                      sender_type: event.sender_type as 'user' | 'agent' | 'system' | undefined,
                    }
                  : m,
              );
            }
            return prev;
          }
          return [
            ...prev,
            {
              id: event.id,
              channel_id: event.channel_id,
              user_id: event.sender_id,
              display_name: event.sender_name || event.sender_id,
              content: event.content,
              created_at: event.created_at,
              status: 'sent' as const,
              sender_type: event.sender_type as 'user' | 'agent' | 'system' | undefined,
            },
          ];
        });
      }

      if (event.type === 'message.agent_typing') {
        if (event.channel_id !== cid) return;

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          if (existing) {
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id ? { ...m, content: event.content } : m,
              );
            }
            return prev;
          }
          return [
            ...prev,
            {
              id: event.id,
              channel_id: event.channel_id,
              user_id: event.sender_id,
              display_name: event.sender_name || event.sender_id,
              content: event.content,
              created_at: event.created_at,
              status: 'streaming' as const,
              sender_type: 'agent' as const,
            },
          ];
        });
      }

      if (event.type === 'message.updated') {
        if (event.channel_id === cid) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === event.id
                ? { ...m, content: event.content, status: 'sent' as const }
                : m,
            ),
          );
        }
      }
    });

    return unsub;
  }, [dmId, onEvent]);

  // ---- Send message (optimistic update) ----

  const sendMessage = useCallback(
    async (content: string, mentionedAgentIds?: string[]): Promise<void> => {
      const id = dmIdRef.current;
      if (!id || !content.trim()) return;

      const tempId = `temp-${Date.now()}`;
      const optimisticMessage: Message = {
        id: tempId,
        channel_id: id,
        user_id: 'user-1',
        display_name: t('selfRef'),
        content: content.trim(),
        created_at: new Date().toISOString(),
        status: 'sending',
      };

      setMessages((prev) => [...prev, optimisticMessage]);

      try {
        // TODO: Replace with real API:
        //   await apiClient.post(`/api/v1/dms/${id}/messages`, { content: content.trim() });
        await new Promise((r) => setTimeout(r, 300));

        const confirmedId = `dm-msg-${Date.now()}`;
        setMessages((prev) =>
          prev.map((m) =>
            m.id === tempId
              ? {
                  id: confirmedId,
                  channel_id: id,
                  user_id: 'user-1',
                  display_name: t('selfRef'),
                  content: content.trim(),
                  created_at: new Date().toISOString(),
                  status: 'sent' as const,
                  sender_type: 'user' as const,
                }
              : m,
          ),
        );
      } catch {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === tempId ? { ...m, status: 'failed' as const } : m,
          ),
        );
      }
    },
    [],
  );

  // ---- Retry failed message ----

  const retryMessage = useCallback(
    async (messageId: string, content: string) => {
      const id = dmIdRef.current;
      if (!id) return;

      setMessages((prev) =>
        prev.map((m) =>
          m.id === messageId ? { ...m, status: 'sending' as const } : m,
        ),
      );

      try {
        // TODO: Replace with real API
        await new Promise((r) => setTimeout(r, 300));
        setMessages((prev) =>
          prev.map((m) =>
            m.id === messageId
              ? {
                  ...m,
                  id: `dm-msg-${Date.now()}`,
                  status: 'sent' as const,
                  sender_type: 'user' as const,
                }
              : m,
          ),
        );
      } catch {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === messageId ? { ...m, status: 'failed' as const } : m,
          ),
        );
      }
    },
    [],
  );

  // ---- Active streaming IDs ----

  const activeStreamingIds = useMemo(
    () => messages.filter((m) => m.status === 'streaming').map((m) => m.id),
    [messages],
  );

  /** Cancel (remove from list) a failed or sending message */
  const cancelMessage = useCallback((messageId: string) => {
    setMessages((prev) => prev.filter((m) => m.id !== messageId));
  }, []);

  return {
    messages,
    isLoading,
    error,
    sendMessage,
    retryMessage,
    cancelMessage,
    refetch: dmId ? () => loadMessages(dmId) : () => {},
    hasMore,
    isLoadingMore,
    loadMoreError,
    loadMore,
    activeStreamingIds,
  } as const;
}
