// ============================================================================
// useDM — Direct Message hook
// - DM list via REST API (GET /api/v1/dm)
// - Create/get DM via REST API (POST /api/v1/dm)
// - DM messages via REST API (GET /api/v1/dm/{dmID}/messages)
// - Send DM message via REST API with optimistic update
// - Receive real-time DM messages via WebSocket dm.message.new events
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { Attachment, DMChannel, Message, CreateDMInput } from '@/lib/types';

// ---- Constants ----

const PAGE_SIZE = 50;

// ---- Backend response shapes ----

interface DMChannelResponse {
  id: string;
  name: string;
  type: string;
  other_member_type: string;
  other_member_id: string;
  other_member_name: string;
  other_member_active?: boolean;
  last_message?: string;
  last_message_at?: string;
  created_at: string;
}

interface DMMessageResponse {
  id: string;
  channel_id: string;
  sender_type: string;
  sender_id: string;
  sender_name: string;
  sender_active?: boolean;
  content: string;
  content_type: string;
  created_at: string;
  // Task fields (populated after convert-to-task)
  task_number?: number;
  task_title?: string;
  task_status?: string;
  task_claimer_name?: string;
  reply_count?: number;
  /** SOLO-249-F: attachments on the message */
  attachments?: Attachment[];
}

interface DMMessageListResponse {
  messages: DMMessageResponse[];
  has_more: boolean;
}

// ---- Mapping helpers ----

function mapDMChannel(resp: DMChannelResponse): DMChannel {
  const channel: DMChannel = {
    id: resp.id,
    type: 'dm',
    unread_count: 0,
    created_at: resp.created_at,
  };
  if (resp.other_member_type === 'user') {
    channel.other_user = {
      id: resp.other_member_id,
      display_name: resp.other_member_name,
    };
  } else if (resp.other_member_type === 'agent') {
    channel.other_agent = {
      id: resp.other_member_id,
      name: resp.other_member_name,
      is_active: resp.other_member_active,
    };
  }
  return channel;
}

function mapDMMessageResponse(resp: DMMessageResponse): Message {
  return {
    id: resp.id,
    channel_id: resp.channel_id,
    user_id: resp.sender_id,
    display_name: resp.sender_name || resp.sender_id,
    content: resp.content,
    created_at: resp.created_at,
    status: 'sent',
    sender_type: resp.sender_type as 'user' | 'agent' | 'system' | undefined,
    sender_active: resp.sender_active,
    task_number: resp.task_number,
    task_title: resp.task_title,
    task_status: resp.task_status,
    task_claimer_name: resp.task_claimer_name,
    reply_count: resp.reply_count ?? 0,
    attachments: resp.attachments,
  };
}

/** Convert a flattened WS DM message event to a Message */
function flatDMToMessage(event: {
  id: string;
  dm_id: string;
  sender_type: string;
  sender_id: string;
  sender_name?: string;
  content: string;
  created_at: string;
  task_number?: number;
  task_title?: string;
  task_status?: string;
  task_claimer_name?: string;
  attachments?: Attachment[];
}): Message {
  return {
    id: event.id,
    channel_id: event.dm_id,
    user_id: event.sender_id,
    display_name: event.sender_name || event.sender_id,
    content: event.content,
    created_at: event.created_at,
    status: 'sent',
    sender_type: event.sender_type as 'user' | 'agent' | 'system' | undefined,
    task_number: event.task_number,
    task_title: event.task_title,
    task_status: event.task_status,
    task_claimer_name: event.task_claimer_name,
    attachments: event.attachments,
  };
}

/** Convert a message.new WS event to a Message (channel_id IS the DM ID) */
function flatToMessage(event: {
  id: string;
  channel_id: string;
  sender_type: string;
  sender_id: string;
  sender_name?: string;
  content: string;
  thread_id?: string;
  created_at: string;
  task_number?: number;
  task_title?: string;
  task_status?: string;
  task_claimer_name?: string;
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
    thread_id: event.thread_id,
    reply_count: 0,
    sender_type: event.sender_type as 'user' | 'agent' | 'system' | undefined,
    task_number: event.task_number,
    task_title: event.task_title,
    task_status: event.task_status,
    task_claimer_name: event.task_claimer_name,
    attachments: event.attachments,
  };
}

// ---- Hook ----

export function useDM() {
  const { subscribeDM, unsubscribeDM, onEvent, isConnected } = useWebSocket();
  const [dmChannels, setDMChannels] = useState<DMChannel[]>([]);
  const [isLoadingDMs, setIsLoadingDMs] = useState(true);
  const [dmError, setDMError] = useState<string | null>(null);

  const [activeDMId, setActiveDMId] = useState<string | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);
  const [messagesError, setMessagesError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);

  const loadingRef = useRef(false);
  const loadingMoreRef = useRef(false);
  const messagesRef = useRef<Message[]>([]);
  messagesRef.current = messages;
  const dmIdRef = useRef<string | null>(null);
  dmIdRef.current = activeDMId;

  // Track previous connected state for detecting reconnection
  const prevConnectedRef = useRef(false);

  // ---- Fetch missed messages after reconnection ----

  useEffect(() => {
    const wasConnected = prevConnectedRef.current;
    prevConnectedRef.current = isConnected;

    if (isConnected && !wasConnected) {
      const did = dmIdRef.current;
      if (!did) return;

      const currentMsgs = messagesRef.current;
      if (currentMsgs.length === 0) return;

      const lastMsg = currentMsgs[currentMsgs.length - 1];
      if (lastMsg.id && !lastMsg.id.startsWith('dm-temp-')) {
        apiClient
          .get<DMMessageListResponse>(`/api/v1/dm/${did}/messages`, {
            limit: '50',
            after: lastMsg.id,
          })
          .then((res) => {
            if (res.messages.length > 0) {
              setMessages((prev) => {
                const existingIds = new Set(prev.map((m) => m.id));
                const newMsgs = res.messages
                  .map(mapDMMessageResponse)
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

  // ---- DM list ----

  const loadDMs = useCallback(async () => {
    setIsLoadingDMs(true);
    setDMError(null);
    try {
      const res = await apiClient.get<DMChannelResponse[]>('/api/v1/dm');
      setDMChannels(res.map(mapDMChannel));
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '加载私信列表失败';
      setDMError(message);
    } finally {
      setIsLoadingDMs(false);
    }
  }, []);

  useEffect(() => {
    loadDMs();
  }, [loadDMs]);

  // ---- Create or get DM ----

  const createOrGetDM = useCallback(
    async (input: CreateDMInput): Promise<DMChannel> => {
      // Map frontend input format to backend expected format
      const body: { member_type: string; member_id: string } = input.user_id
        ? { member_type: 'user', member_id: input.user_id }
        : { member_type: 'agent', member_id: input.agent_id! };
      const res = await apiClient.post<DMChannelResponse>('/api/v1/dm', body);
      const channel = mapDMChannel(res);
      // Add to local state if not already present
      setDMChannels((prev) => {
        const exists = prev.find((c) => c.id === channel.id);
        return exists ? prev : [...prev, channel];
      });
      // Remove from closed DMs so it reappears in the sidebar
      try {
        const key = 'solo-closed-dm-ids';
        const stored = localStorage.getItem(key);
        if (stored) {
          const ids = new Set<string>(JSON.parse(stored));
          if (ids.delete(channel.id)) {
            localStorage.setItem(key, JSON.stringify([...ids]));
            window.dispatchEvent(new CustomEvent('dm-closed-changed'));
          }
        }
      } catch { /* ignore */ }
      return channel;
    },
    [],
  );

  // ---- Select active DM ----

  const selectDM = useCallback((dmId: string | null) => {
    setActiveDMId(dmId);
    if (dmId !== activeDMId) {
      setMessages([]);
      setHasMore(true);
    }
  }, [activeDMId]);

  // ---- WebSocket subscription ----

  useEffect(() => {
    if (!activeDMId) return;

    subscribeDM(activeDMId);

    return () => {
      unsubscribeDM(activeDMId);
    };
  }, [activeDMId, subscribeDM, unsubscribeDM]);

  // ---- Listen for DM message events ----

  useEffect(() => {
    if (!activeDMId) return;

    const unsub = onEvent((event) => {
      const did = dmIdRef.current;
      if (!did) return;

      // ---- message.new (catches agent responses, task system messages, etc.) ----
      if (event.type === 'message.new') {
        if (event.channel_id !== did) return;
        if (event.thread_id) return; // thread messages handled by thread hook

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          const newMsg = flatToMessage(event);

          if (existing) {
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id
                  ? { ...m, ...newMsg, status: 'sent' as const }
                  : m,
              );
            }
            return prev.map((m) =>
              m.id === event.id
                ? { ...m, ...newMsg, status: 'sent' as const }
                : m,
            );
          }

          // Clean up orphaned temp/sending messages
          const cleaned = prev.filter(
            (m) =>
              !(
                (m.id.startsWith('dm-temp-') || m.status === 'sending') &&
                m.content === event.content &&
                m.channel_id === did
              ),
          );

          return [...cleaned, newMsg];
        });
      }

      // ---- dm.message.new (DM-specific event, redundant with message.new but handled for safety) ----
      if (event.type === 'dm.message.new') {
        if (event.dm_id !== did) return;
        if (event.thread_id) return; // thread replies handled by thread hook

        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          if (existing) {
            if (existing.status === 'streaming') {
              const newMsg = flatDMToMessage(event);
              return prev.map((m) =>
                m.id === event.id
                  ? { ...m, ...newMsg, status: 'sent' as const }
                  : m,
              );
            }
            return prev;
          }
          return [...prev, flatDMToMessage(event)];
        });
      }

      // ---- message.agent_typing (streaming agent response) ----
      if (event.type === 'message.agent_typing') {
        if (event.channel_id !== did) return;
        if (event.thread_id) return;

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
              user_id: event.sender_id || '',
              display_name: event.sender_name || event.sender_id || 'Agent',
              content: event.content,
              created_at: event.created_at,
              status: 'streaming' as const,
              sender_type: 'agent' as const,
            },
          ];
        });
      }

      // ---- message.updated (content edit, task fields, reply_count) ----
      if (event.type === 'message.updated') {
        if (event.channel_id === did) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === event.id
                ? {
                    ...m,
                    content: event.content || m.content,
                    status: 'sent' as const,
                    task_number: event.task_number ?? m.task_number,
                    task_title: event.task_title ?? m.task_title,
                    task_status: event.task_status ?? m.task_status,
                    task_claimer_name: event.task_claimer_name ?? m.task_claimer_name,
                  }
                : m,
            ),
          );
        }
      }

      // ---- message.deleted ----
      if (event.type === 'message.deleted') {
        if (event.channel_id === did) {
          setMessages((prev) => prev.filter((m) => m.id !== event.message_id));
        }
      }

      // ---- task.updated (defense-in-depth alongside message.updated) ----
      if (event.type === 'task.updated') {
        if (event.channel_id === did && event.message_id) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === event.message_id
                ? {
                    ...m,
                    task_number: event.task_number ?? m.task_number,
                    task_title: event.title ?? m.task_title,
                    task_status: event.status ?? m.task_status,
                    task_claimer_name: event.claimer_name ?? m.task_claimer_name,
                  }
                : m,
            ),
          );
        }
      }

      // ---- thread.reply (reply count update on parent message) ----
      if (event.type === 'thread.reply') {
        if (event.channel_id !== did) return;

        const parentMessageId = event.root_message_id || event.thread_id;
        setMessages((prev) =>
          prev.map((m) =>
            m.id === parentMessageId
              ? { ...m, reply_count: event.reply_count, has_unread_thread: true }
              : m,
          ),
        );
      }

      // ---- dm.updated (DM channel list metadata update) ----
      if (event.type === 'dm.updated') {
        if (event.dm_id === did) {
          setDMChannels((prev) =>
            prev.map((dm) =>
              dm.id === event.dm_id
                ? {
                    ...dm,
                    last_message: event.last_message ?? dm.last_message,
                    last_reply_at: event.last_reply_at ?? dm.last_reply_at,
                    unread_count: event.unread_count,
                  }
                : dm,
            ),
          );
        } else {
          setDMChannels((prev) =>
            prev.map((dm) =>
              dm.id === event.dm_id
                ? {
                    ...dm,
                    last_message: event.last_message ?? dm.last_message,
                    last_reply_at: event.last_reply_at ?? dm.last_reply_at,
                    unread_count: event.unread_count,
                  }
                : dm,
            ),
          );
        }
      }
    });

    return unsub;
  }, [activeDMId, onEvent]);

  // ---- Load messages for active DM ----

  const loadMessages = useCallback(async (dmId: string) => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setIsLoadingMessages(true);
    setMessagesError(null);

    try {
      const res = await apiClient.get<DMMessageListResponse>(
        `/api/v1/dm/${dmId}/messages`,
        { limit: String(PAGE_SIZE) },
      );
      setMessages(res.messages.map(mapDMMessageResponse));
      setHasMore(res.has_more);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '加载消息失败';
      setMessagesError(message);
    } finally {
      setIsLoadingMessages(false);
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => {
    if (activeDMId) {
      loadMessages(activeDMId);
    } else {
      setMessages([]);
      setHasMore(true);
    }
  }, [activeDMId, loadMessages]);

  // ---- Load older messages (cursor-based pagination) ----

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
      const res = await apiClient.get<DMMessageListResponse>(
        `/api/v1/dm/${id}/messages`,
        { limit: String(PAGE_SIZE), before: oldestId },
      );

      setMessages((prev) => [...res.messages.map(mapDMMessageResponse), ...prev]);
      setHasMore(res.has_more);
    } catch {
      setLoadMoreError('加载更早消息失败');
    } finally {
      setIsLoadingMore(false);
      loadingMoreRef.current = false;
    }
  }, [hasMore]);

  // ---- Mark DM as read ----

  const markAsRead = useCallback((dmId: string) => {
    setDMChannels((prev) =>
      prev.map((dm) =>
        dm.id === dmId ? { ...dm, unread_count: 0 } : dm,
      ),
    );
  }, []);

  // ---- Send DM message (optimistic update) ----

  const sendMessage = useCallback(
    async (
      content: string,
      _mentionedAgentIds?: string[],
      asTask?: boolean,
      attachmentIds?: string[],
    ): Promise<{ id: string; task_number?: number } | null> => {
      const id = dmIdRef.current;
      if (!id || !content.trim()) return null;

      const tempId = `dm-temp-${Date.now()}`;
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
        if (_mentionedAgentIds && _mentionedAgentIds.length > 0) {
          body.mentioned_agent_ids = _mentionedAgentIds;
        }
        if (asTask) {
          body.as_task = true;
        }
        if (attachmentIds && attachmentIds.length > 0) {
          body.attachment_ids = attachmentIds;
        }
        const confirmed = await apiClient.post<DMMessageResponse & { task_number?: number; message_id?: string }>(
          `/api/v1/dm/${id}/messages`,
          body,
        );

        const isTaskResponse = asTask && (confirmed as unknown as Record<string, unknown>).message_id !== undefined;
        const realMessageId = isTaskResponse
          ? (confirmed as unknown as Record<string, unknown>).message_id as string
          : confirmed.id;

        setMessages((prev) => {
          if (isTaskResponse) {
            const taskResp = confirmed as unknown as Record<string, unknown>;
            const creatorName = (taskResp.creator_name as string) || undefined;
            return prev.map((m) => {
              if (m.id === tempId) {
                return {
                  ...m,
                  id: realMessageId,
                  status: 'sent' as const,
                  ...(creatorName && { display_name: creatorName }),
                  task_number: taskResp.task_number as number | undefined,
                  task_status: taskResp.status as string | undefined,
                  task_claimer_name: taskResp.claimer_name as string | undefined,
                };
              }
              if (m.id === realMessageId) {
                return {
                  ...m,
                  task_number: taskResp.task_number as number | undefined,
                  task_status: taskResp.status as string | undefined,
                  task_claimer_name: taskResp.claimer_name as string | undefined,
                };
              }
              return m;
            });
          }
          // Non-asTask: replace temp message and any WS duplicate
          const confirmedMessage: Message = { ...mapDMMessageResponse(confirmed), status: 'sent' as const };
          const skipIds = new Set([tempId, confirmed.id]);
          let hasConfirmed = false;
          const result: Message[] = [];
          for (const m of prev) {
            if (skipIds.has(m.id)) {
              if (!hasConfirmed) {
                result.push(confirmedMessage);
                hasConfirmed = true;
              }
            } else {
              result.push(m);
            }
          }
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
      const id = dmIdRef.current;
      if (!id) return;

      setMessages((prev) =>
        prev.map((m) =>
          m.id === messageId ? { ...m, status: 'sending' as const } : m,
        ),
      );

      try {
        const confirmed = await apiClient.post<DMMessageResponse>(
          `/api/v1/dm/${id}/messages`,
          { content },
        );

        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== messageId && m.id !== confirmed.id);
          return [...filtered, { ...mapDMMessageResponse(confirmed), status: 'sent' as const }];
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

  // ---- Edit message (optimistic update) ----

  const editMessage = useCallback(
    async (messageId: string, content: string): Promise<void> => {
      const id = dmIdRef.current;
      if (!id || !content.trim()) return;

      setMessages((prev) =>
        prev.map((m) =>
          m.id === messageId ? { ...m, content: content.trim() } : m,
        ),
      );

      try {
        await apiClient.patch(`/api/v1/dm/${id}/messages/${messageId}`, {
          content: content.trim(),
        });
      } catch {
        // Optimistic update already applied; WS message.updated will sync
      }
    },
    [],
  );

  // ---- Delete message (optimistic removal) ----

  const deleteMessage = useCallback(
    async (messageId: string): Promise<void> => {
      const id = dmIdRef.current;
      if (!id) return;

      setMessages((prev) => prev.filter((m) => m.id !== messageId));

      try {
        await apiClient.delete(`/api/v1/dm/${id}/messages/${messageId}`);
      } catch {
        // Optimistic removal already applied; WS message.deleted will sync
      }
    },
    [],
  );

  return {
    // DM list
    dmChannels,
    isLoadingDMs,
    dmError,
    createOrGetDM,
    markAsRead,
    refetchDMs: loadDMs,
    // Active DM selection
    activeDMId,
    selectDM,
    // Messages
    messages,
    isLoadingMessages,
    messagesError,
    sendMessage,
    retryMessage,
    cancelMessage,
    editMessage,
    deleteMessage,
    hasMore,
    isLoadingMore,
    loadMoreError,
    loadMore,
    activeStreamingIds,
  } as const;
}
