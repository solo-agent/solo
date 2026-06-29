// ============================================================================
// useMessages — real-time message hook backed by REST API + WebSocket
// - Initial load via REST API (cursor-based pagination)
// - Send via REST API with optimistic update
// - Receive real-time messages via WebSocket message.new events
// - loadMore for cursor-based pagination
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { t } from '@/lib/i18n';
import type { WSMessage } from '@/lib/ws-types';
import type { Attachment, Message } from '@/lib/types';

// ---- Constants ----

const PAGE_SIZE = 50;

// ---- Backend response shapes ----

interface MessageResponse {
  id: string;
  channel_id: string;
  sender_type: string;
  sender_id: string;
  sender_name: string;
  content: string;
  content_type: string;
  thread_id?: string;
  reply_count?: number;
  task_number?: number;
  task_title?: string;
  task_status?: string;
  task_claimer_name?: string;
  task_claimer_deleted?: boolean;
  has_unread_thread?: boolean;
  sender_active?: boolean;
  created_at: string;
  /** SOLO-249-F: attachments on the message */
  attachments?: Attachment[];
}

interface MessageListResponse {
  messages: MessageResponse[];
  has_more: boolean;
}

// ---- Mapping helpers ----

function mapMessageResponse(resp: MessageResponse): Message {
  return {
    id: resp.id,
    channel_id: resp.channel_id,
    user_id: resp.sender_id,
    display_name: resp.sender_name || resp.sender_id,
    content: resp.content,
    created_at: resp.created_at,
    status: 'sent',
    thread_id: resp.thread_id,
    reply_count: resp.reply_count,
    sender_type: resp.sender_type as 'user' | 'agent' | 'system' | undefined,
    sender_active: resp.sender_active,
    task_number: resp.task_number,
    task_title: resp.task_title,
    task_status: resp.task_status,
    task_claimer_name: resp.task_claimer_name,
    task_claimer_deleted: resp.task_claimer_deleted,
    has_unread_thread: resp.has_unread_thread,
    attachments: resp.attachments,
  };
}

function mapWSMessage(ws: WSMessage): Message {
  return {
    id: ws.id,
    channel_id: ws.channel_id,
    user_id: ws.sender_id || ws.user_id || '',
    display_name: ws.sender_name || ws.display_name || ws.sender_id || '',
    content: ws.content,
    created_at: ws.created_at,
    status: 'sent',
    thread_parent_id: ws.thread_parent_id,
    sender_type: ws.sender_type as 'user' | 'agent' | 'system' | undefined,
    attachments: ws.attachments,
  };
}

/** Convert a flattened WS message event to a Message */
function flatToMessage(event: {
  id: string;
  channel_id: string;
  sender_type: string;
  sender_id: string;
  sender_name?: string;
  content: string;
  thread_id?: string;
  reply_count?: number;
  task_number?: number;
  task_title?: string;
  task_status?: string;
  task_claimer_name?: string;
  task_claimer_deleted?: boolean;
  has_unread_thread?: boolean;
  created_at: string;
  attachments?: Attachment[];
}): Message {
  return {
    id: event.id,
    channel_id: event.channel_id,
    user_id: event.sender_id,
    display_name: event.sender_name || event.sender_id,
    content: event.content,
    created_at: event.created_at,
    status: 'sent',
    thread_parent_id: event.thread_id,
    thread_id: event.thread_id,
    reply_count: event.reply_count,
    sender_type: event.sender_type as 'user' | 'agent' | 'system' | undefined,
    task_number: event.task_number,
    task_title: event.task_title,
    task_status: event.task_status,
    task_claimer_name: event.task_claimer_name,
    task_claimer_deleted: event.task_claimer_deleted,
    has_unread_thread: event.has_unread_thread,
    attachments: event.attachments,
  };
}

// ---- Hook ----

export function useMessages(channelId: string | null) {
  const { subscribe, unsubscribe, onEvent, isConnected } = useWebSocket();
  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);
  const loadingRef = useRef(false);
  const loadingMoreRef = useRef(false);
  const channelRef = useRef<string | null>(null);

  // Stable refs for async callback reads
  const messagesRef = useRef<Message[]>([]);
  messagesRef.current = messages;

  const channelIdRef = useRef(channelId);
  channelIdRef.current = channelId;

  // Keep channelRef immediately in sync so sendMessage and other operations
  // can use it before the first async loadMessages completes.
  channelRef.current = channelId;

  // Track previous connected state for detecting reconnection
  const prevConnectedRef = useRef(false);

  // ---- Thread reply tracking (SOLO-67-FS) ----
  // Maps thread_id → message_id for updating reply counts in real-time
  const threadToMessageMap = useRef<Map<string, string>>(new Map());
  const [replyCounts, setReplyCounts] = useState<Record<string, number>>({});

  // ---- Fetch missed messages after reconnection ----
  useEffect(() => {
    const wasConnected = prevConnectedRef.current;
    prevConnectedRef.current = isConnected;

    // Detected reconnection: was disconnected, now connected
    if (isConnected && !wasConnected) {
      const cid = channelIdRef.current;
      if (!cid) return;

      const currentMsgs = messagesRef.current;
      if (currentMsgs.length === 0) return;

      // Use the most recent message ID as the "after" cursor
      // to fetch messages that arrived during disconnection
      const lastMsg = currentMsgs[currentMsgs.length - 1];
      if (lastMsg.id && !lastMsg.id.startsWith('temp-')) {
        apiClient
          .get<MessageListResponse>(`/api/v1/channels/${cid}/messages`, {
            limit: '50',
            after: lastMsg.id,
          })
          .then((res) => {
            if (res.messages.length > 0) {
              setMessages((prev) => {
                const existingIds = new Set(prev.map((m) => m.id));
                const newMsgs = res.messages
                  .map(mapMessageResponse)
                  .filter((m) => !existingIds.has(m.id));
                if (newMsgs.length === 0) return prev;
                return [...prev, ...newMsgs];
              });
            }
          })
          .catch(() => {
            // Silently fail — messages will arrive via WS subscription
          });
      }
    }
  }, [isConnected]);

  // ---- WebSocket subscription ----

  useEffect(() => {
    if (!channelId) return;

    setMessages([]);
    setHasMore(true);
    subscribe(channelId);

    return () => {
      unsubscribe(channelId);
    };
  }, [channelId, subscribe, unsubscribe]);

  // Listen for message events (new + streaming + update + delete)
  useEffect(() => {
    if (!channelId) return;

    const unsub = onEvent((event) => {
      const cid = channelIdRef.current;
      if (!cid) return;

      if (event.type === 'message.new') {
        if (event.channel_id !== cid) return;
        if (event.thread_id) return;

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          const newMsg = flatToMessage(event);

          if (existing) {
            // Merge WS data into existing message. WS is authoritative
            // (has correct sender_name, task fields, etc.) so we replace
            // stale fields instead of keeping the old message unchanged.
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id
                  ? { ...m, ...newMsg, status: 'sent' as const }
                  : m,
              );
            }
            // Already have a stable message — merge WS fields to ensure
            // display_name and other metadata are up to date.
            return prev.map((m) =>
              m.id === event.id
                ? { ...m, ...newMsg, status: 'sent' as const }
                : m,
            );
          }

          // New message — clean up any orphaned temp/sending messages
          // that might be optimistic duplicates of this real message.
          // Only remove temp messages whose content matches (best-effort dedup).
          const cleaned = prev.filter(
            (m) =>
              !(
                (m.id.startsWith('temp-') || m.status === 'sending') &&
                m.content === event.content &&
                m.channel_id === cid
              ),
          );

          // Track thread mapping if this message has a thread
          if (newMsg.thread_id) {
            threadToMessageMap.current.set(newMsg.thread_id, newMsg.id);
          }
          return [...cleaned, newMsg];
        });
      }

      if (event.type === 'message.agent_typing') {
        if (event.channel_id !== cid) return;
        if (event.thread_id) return;

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          if (existing) {
            // Update content of an existing streaming message
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id ? { ...m, content: event.content } : m,
              );
            }
            // Already finalized — ignore late typing events
            return prev;
          }
          // First typing event for this message — create streaming placeholder
          return [
            ...prev,
            {
              id: event.id,
              channel_id: event.channel_id,
              user_id: event.sender_id || '',
              display_name: event.sender_name || event.sender_id || t('agent'),
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
                ? { ...m,
                    content: event.content || m.content,
                    status: 'sent' as const,
                    task_number: event.task_number ?? m.task_number,
                    task_title: event.task_title ?? m.task_title,
                    task_status: event.task_status ?? m.task_status,
                    task_claimer_name: event.task_claimer_name ?? m.task_claimer_name,
                    task_claimer_deleted: event.task_claimer_deleted ?? m.task_claimer_deleted,
                    reply_count: event.reply_count ?? m.reply_count }
                : m,
            ),
          );
        }
      }

      if (event.type === 'message.deleted') {
        if (event.channel_id === cid) {
          setMessages((prev) =>
            prev.filter((m) => m.id !== event.message_id),
          );
        }
      }

      // ---- Task update (SOLO-122-B) — defense-in-depth alongside message.updated ----
      if (event.type === 'task.updated') {
        if (event.channel_id === cid && event.message_id) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === event.message_id
                ? {
                    ...m,
                    task_number: event.task_number ?? m.task_number,
                    task_title: event.title ?? m.task_title,
                    task_status: event.status ?? m.task_status,
                    task_claimer_name: event.claimer_name ?? m.task_claimer_name,
                    task_claimer_deleted: event.claimer_deleted ?? m.task_claimer_deleted,
                  }
                : m,
            ),
          );
        }
      }

      // ---- Thread reply count update (SOLO-67-FS) ----
      if (event.type === 'thread.reply') {
        if (event.channel_id !== cid) return;

        // root_message_id is the parent message ID — use it to update reply_count and has_unread.
        // Fall back to thread_id for backward compatibility.
        const parentMessageId = event.root_message_id || event.thread_id;
        setMessages((prev) =>
          prev.map((m) =>
            m.id === parentMessageId
              ? { ...m, reply_count: event.reply_count, has_unread_thread: true }
              : m,
          ),
        );
        setReplyCounts((prev) => ({
          ...prev,
          [parentMessageId]: event.reply_count,
        }));
      }
    });

    return unsub;
  }, [channelId, onEvent]);

  // ---- Initial load ----

  const loadMessages = useCallback(async (id: string) => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setIsLoading(true);
    setError(null);
    setLoadMoreError(null);
    try {
      const res = await apiClient.get<MessageListResponse>(
        `/api/v1/channels/${id}/messages`,
        { limit: String(PAGE_SIZE) },
      );
      if (channelRef.current === null || channelRef.current === id) {
        const parsed = res.messages.map(mapMessageResponse);
        setMessages(parsed);
        setHasMore(res.has_more);
        channelRef.current = id;

        // Build thread-to-message mapping for reply count tracking
        const newMap = new Map<string, string>();
        const newCounts: Record<string, number> = {};
        for (const msg of parsed) {
          if (msg.thread_id) {
            newMap.set(msg.thread_id, msg.id);
          }
          if (msg.reply_count !== undefined) {
            newCounts[msg.id] = msg.reply_count;
          }
        }
        threadToMessageMap.current = newMap;
        setReplyCounts(newCounts);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : t('messageLoadError');
      setError(message);
    } finally {
      setIsLoading(false);
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => {
    if (channelId) {
      loadMessages(channelId);
    } else {
      setMessages([]);
      setHasMore(true);
      channelRef.current = null;
    }
  }, [channelId, loadMessages]);

  // ---- Load older messages (cursor-based pagination) ----

  const loadMore = useCallback(async () => {
    const id = channelRef.current;
    if (!id || !hasMore || loadingMoreRef.current) return;

    loadingMoreRef.current = true;
    setIsLoadingMore(true);
    setLoadMoreError(null);

    try {
      // Oldest message's first-page-load ID serves as the "before" cursor
      const currentMsgs = messagesRef.current;
      if (currentMsgs.length === 0) {
        setHasMore(false);
        return;
      }

      const oldestId = currentMsgs[0].id;
      const res = await apiClient.get<MessageListResponse>(
        `/api/v1/channels/${id}/messages`,
        { limit: String(PAGE_SIZE), before: oldestId },
      );

      const older = res.messages.map(mapMessageResponse);
      setMessages((prev) => [...older, ...prev]);
      setHasMore(res.has_more);

      // Update thread-to-message mapping with older messages
      for (const msg of older) {
        if (msg.thread_id) {
          threadToMessageMap.current.set(msg.thread_id, msg.id);
        }
        if (msg.reply_count !== undefined) {
          setReplyCounts((prev) => ({ ...prev, [msg.id]: msg.reply_count! }));
        }
      }
    } catch {
      setLoadMoreError(t('earlierMessageLoadError'));
    } finally {
      setIsLoadingMore(false);
      loadingMoreRef.current = false;
    }
  }, [hasMore]);

  // ---- Send message (optimistic update) ----

  const sendMessage = useCallback(
    async (
      content: string,
      mentionedAgentIds?: string[],
      asTask?: boolean,
      attachmentIds?: string[],
    ): Promise<{ id: string; task_number?: number } | null> => {
      const id = channelRef.current;
      if (!id || !content.trim()) return null;

      const tempId = `temp-${Date.now()}`;
      const optimisticMessage: Message = {
        id: tempId,
        channel_id: id,
        user_id: 'user-1',
        display_name: 'You',
        content: content.trim(),
        created_at: new Date().toISOString(),
        status: 'sending',
      };

      setMessages((prev) => [...prev, optimisticMessage]);

      try {
        const body: Record<string, unknown> = { content: content.trim() };
        if (mentionedAgentIds && mentionedAgentIds.length > 0) {
          body.mentioned_agent_ids = mentionedAgentIds;
        }
        if (asTask) {
          body.as_task = true;
        }
        if (attachmentIds && attachmentIds.length > 0) {
          body.attachment_ids = attachmentIds;
        }
        const confirmed = await apiClient.post<MessageResponse & { task_number?: number; message_id?: string }>(
          `/api/v1/channels/${id}/messages`,
          body,
        );

        // asTask responses return a TaskResponse shape (id=task UUID, message_id=message UUID)
        // instead of MessageResponse (id=message UUID, sender_name, content, etc.).
        // Detect by checking for the message_id field.
        const isTaskResponse = asTask && (confirmed as unknown as Record<string, unknown>).message_id !== undefined;
        const realMessageId = isTaskResponse
          ? (confirmed as unknown as Record<string, unknown>).message_id as string
          : confirmed.id;

        setMessages((prev) => {
          if (isTaskResponse) {
            // Map the optimistic temp message to the real message ID from the TaskResponse.
            // Preserve the optimistic fields (display_name, content) — the WS message.new
            // broadcast will handle final dedup and enrichment.
            const taskResp = confirmed as unknown as Record<string, unknown>;
            return prev.map((m) => {
              if (m.id === tempId) {
                return {
                  ...m,
                  id: realMessageId,
                  status: 'sent' as const,
                  task_number: taskResp.task_number as number | undefined,
                  task_status: taskResp.status as string | undefined,
                  task_claimer_name: taskResp.claimer_name as string | undefined,
                  task_claimer_deleted: taskResp.claimer_deleted as boolean | undefined,
                };
              }
              // If WS message.new already arrived with the real message ID,
              // update its task fields in place.
              if (m.id === realMessageId) {
                return {
                  ...m,
                  task_number: taskResp.task_number as number | undefined,
                  task_status: taskResp.status as string | undefined,
                  task_claimer_name: taskResp.claimer_name as string | undefined,
                  task_claimer_deleted: taskResp.claimer_deleted as boolean | undefined,
                };
              }
              return m;
            });
          }
          // Non-asTask: replace temp message (and any WS duplicate) with the confirmed response.
          // Use reduce to guarantee exactly one confirmed message in the output,
          // regardless of whether 0, 1, or 2 matching messages existed in prev.
          const confirmedMessage: Message = { ...mapMessageResponse(confirmed), status: 'sent' as const };
          const skipIds = new Set([tempId, confirmed.id]);
          let hasConfirmed = false;
          const result: Message[] = [];
          for (const m of prev) {
            if (skipIds.has(m.id)) {
              if (!hasConfirmed) {
                result.push(confirmedMessage);
                hasConfirmed = true;
              }
              // else: duplicate — skip it
            } else {
              result.push(m);
            }
          }
          // Edge case: neither temp nor WS-dup was in prev (shouldn't happen but safe)
          if (!hasConfirmed) {
            result.push(confirmedMessage);
          }
          return result;
        });

        return {
          id: realMessageId,
          task_number: (confirmed as unknown as Record<string, unknown>).task_number as number | undefined,
        };
      } catch {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === tempId ? { ...m, status: 'failed' as const } : m,
          ),
        );
        return null;
      }
    },
    [],
  );

  // ---- Retry failed message ----

  const retryMessage = useCallback(
    async (messageId: string, content: string) => {
      const id = channelRef.current;
      if (!id) return;

      setMessages((prev) =>
        prev.map((m) =>
          m.id === messageId ? { ...m, status: 'sending' as const } : m,
        ),
      );

      try {
        const confirmed = await apiClient.post<MessageResponse>(
          `/api/v1/channels/${id}/messages`,
          { content },
        );

        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== messageId && m.id !== confirmed.id);
          return [...filtered, { ...mapMessageResponse(confirmed), status: 'sent' as const }];
        });
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

  // ---- Edit message (W2-06-FE) ----

  const editMessage = useCallback(
    async (messageId: string, content: string): Promise<void> => {
      const id = channelRef.current;
      if (!id || !content.trim()) return;

      // Optimistic update
      setMessages((prev) =>
        prev.map((m) =>
          m.id === messageId ? { ...m, content: content.trim() } : m,
        ),
      );

      try {
        await apiClient.patch(`/api/v1/channels/${id}/messages/${messageId}`, {
          content: content.trim(),
        });
      } catch {
        // Optimistic update already applied; WS message.updated will sync
      }
    },
    [],
  );

  // ---- Mark thread as read locally (P25-08-F) ----

  const markMessageThreadRead = useCallback((messageId: string) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.id === messageId ? { ...m, has_unread_thread: false } : m,
      ),
    );
  }, []);

  // ---- Delete message (W2-06-FE) ----

  const deleteMessage = useCallback(
    async (messageId: string): Promise<void> => {
      const id = channelRef.current;
      if (!id) return;

      // Optimistic removal
      setMessages((prev) => prev.filter((m) => m.id !== messageId));

      try {
        await apiClient.delete(
          `/api/v1/channels/${id}/messages/${messageId}`,
        );
      } catch {
        // Optimistic removal already applied; WS message.deleted will sync
      }
    },
    [],
  );

  return {
    messages,
    isLoading,
    error,
    sendMessage,
    retryMessage,
    cancelMessage,
    editMessage,
    deleteMessage,
    refetch: channelId ? () => loadMessages(channelId) : () => {},
    hasMore,
    isLoadingMore,
    loadMoreError,
    loadMore,
    /** IDs of messages currently streaming (status === 'streaming') */
    activeStreamingIds,
    /** Reply counts per message_id (SOLO-67-FS) */
    replyCounts,
    /** Mark a message's thread as read locally (P25-08-F) */
    markMessageThreadRead,
  } as const;
}
