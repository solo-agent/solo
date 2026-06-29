// ============================================================================
// SOLO-31-F: useThread — 线程消息状态管理 hook
// - 调用真实 REST API 加载线程消息 (Task 4)
// - 通过 REST API 发送线程回复，支持乐观更新
// - 通过 WS 接收实时线程事件推送
// - 跟踪实际 thread_id 用于 WS 订阅
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { t } from '@/lib/i18n';
import type { WSMessage, WSMessageSource } from '@/lib/ws-types';

// ---- API 响应类型（与后端 handler/thread.go 对齐） ----

interface ThreadReplyResponse {
  id: string;
  channel_id: string;
  thread_id: string;
  sender_type: string;
  sender_id: string;
  sender_name?: string;
  sender_active?: boolean;
  content: string;
  content_type: string;
  created_at: string;
}

interface ThreadMessageListResponse {
  messages: ThreadReplyResponse[];
  has_more: boolean;
  thread_id?: string;
}

// ---- 转换工具 ----

function toWSMessage(r: ThreadReplyResponse): WSMessage {
  return {
    id: r.id,
    channel_id: r.channel_id,
    sender_type: r.sender_type as WSMessageSource,
    sender_id: r.sender_id,
    sender_name: r.sender_name || (r.sender_type === 'system' ? 'Solo' : undefined),
    sender_active: r.sender_active,
    display_name: r.sender_name || (r.sender_type === 'system' ? 'Solo' : undefined),
    content: r.content,
    content_type: r.content_type,
    thread_parent_id: r.thread_id,
    created_at: r.created_at,
  };
}

// ---- Hook ----

export interface UseThreadReturn {
  /** 线程消息列表（按时间正序） */
  messages: WSMessage[];
  /** 是否正在加载 */
  isLoading: boolean;
  /** 加载错误 */
  error: string | null;
  /** 当前线程 ID（来自后端 threads 表） */
  threadId: string | null;
  /** 加载线程消息 */
  loadThreadMessages: (channelId: string, messageId: string) => Promise<void>;
  /** 发送线程回复（乐观更新） */
  sendReply: (content: string, mentionedAgentIds?: string[]) => Promise<void>;
  /** 刷新当前线程 */
  refetch: () => void;
  /** 标记当前线程为已读 (P25-08-F) */
  markRead: () => Promise<void>;
}

export function useThread(): UseThreadReturn {
  const { subscribeThread, unsubscribeThread, onEvent } = useWebSocket();
  const [messages, setMessages] = useState<WSMessage[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [threadId, setThreadId] = useState<string | null>(null);

  // Refs 用于回调中读取最新值，避免闭包过期
  const channelIdRef = useRef<string>('');
  const messageIdRef = useRef<string>('');
  const threadIdRef = useRef<string | null>(null);
  threadIdRef.current = threadId;
  const loadingRef = useRef(false);

  // ---- 加载线程消息 ----

  const loadThreadMessages = useCallback(
    async (channelId: string, messageId: string) => {
      if (loadingRef.current) return;
      loadingRef.current = true;
      setIsLoading(true);
      setError(null);

      channelIdRef.current = channelId;
      messageIdRef.current = messageId;

      try {
        const res = await apiClient.get<ThreadMessageListResponse>(
          `/api/v1/channels/${channelId}/messages/${messageId}/thread`,
        );
        const threadMsgs = res.messages;
        setMessages(threadMsgs.map(toWSMessage));

        // Get thread_id from response (works even with 0 messages) or fall back to first message
        const tid = res.thread_id || (threadMsgs.length > 0 ? threadMsgs[0].thread_id : null);
        setThreadId(tid || null);
      } catch {
        // 线程不存在（还没有回复）时返回空列表
        setMessages([]);
        setThreadId(null);
      } finally {
        setIsLoading(false);
        loadingRef.current = false;
      }
    },
    [],
  );

  // ---- WS 订阅生命周期 ----
  // 当获得实际的 thread_id 后，订阅该线程的实时事件

  useEffect(() => {
    const tid = threadIdRef.current;
    if (!tid) return;

    subscribeThread(tid);
    return () => {
      unsubscribeThread(tid);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId, subscribeThread, unsubscribeThread]);

  // ---- WS 事件监听：实时接收新线程消息 + 流式输出 ----

  useEffect(() => {
    const unsub = onEvent((event) => {
      const tid = threadIdRef.current;
      if (!tid) return;

      // Streaming tokens from agent in thread (P25-10-F)
      if (event.type === 'message.agent_typing' && event.thread_id === tid) {
        setMessages((prev) => {
          const existing = prev.find((m) => m.id === event.id);
          if (existing) {
            // Update content of existing streaming message
            if (existing.status === 'streaming') {
              return prev.map((m) =>
                m.id === event.id ? { ...m, content: event.content } : m,
              );
            }
            // Already finalized (e.g., via thread.message.new)
            return prev;
          }
          // First token — create streaming placeholder
          const streamingMsg: WSMessage = {
            id: event.id,
            channel_id: event.channel_id,
            sender_type: 'agent' as WSMessageSource,
            sender_id: event.sender_id,
            sender_name: event.sender_name || event.sender_id || t('agent'),
            display_name: event.sender_name || event.sender_id || t('agent'),
            content: event.content,
            thread_parent_id: event.thread_id,
            created_at: event.created_at,
            status: 'streaming',
          };
          return [...prev, streamingMsg];
        });
        return;
      }

      if (event.type === 'thread.message.new' && event.thread.thread_id === tid) {
        // Replace streaming placeholder if one exists, otherwise append
        setMessages((prev) => {
          // Skip WS if the user has a pending send — API response will handle it.
          // Otherwise WS and API race and the same message appears twice.
          if (event.message.sender_type === 'user' && prev.some((m) => m.status === 'sending' && m.sender_type === 'user')) {
            return prev;
          }
          const hasStreaming = prev.some((m) => m.id === event.message.id && m.status === 'streaming');
          const exists = prev.some((m) => m.id === event.message.id);
          const newMsg = {
            id: event.message.id,
            channel_id: event.message.channel_id,
            sender_type: event.message.sender_type as WSMessageSource,
            sender_id: event.message.sender_id,
            sender_name: event.message.sender_name,
            display_name: event.message.sender_name,
            content: event.message.content,
            content_type: event.message.content_type,
            thread_parent_id: event.message.thread_id,
            created_at: event.message.created_at,
          };
          const result = prev.map((m) =>
            m.id === event.message.id ? newMsg : m,
          );
          if (!exists && !hasStreaming) {
            result.push(newMsg);
          }
          return result;
        });
      }
    });
    return unsub;
  }, [onEvent]);

  // ---- 发送线程回复（乐观更新） ----

  const sendReply = useCallback(
    async (content: string, mentionedAgentIds?: string[]) => {
      const cid = channelIdRef.current;
      const mid = messageIdRef.current;
      if (!cid || !mid || !content.trim()) return;

      const tempId = `thread-temp-${Date.now()}`;
      const optimistic: WSMessage = {
        id: tempId,
        channel_id: cid,
        sender_type: 'user',
        sender_id: 'local',
        sender_name: 'You',
        display_name: 'You',
        content: content.trim(),
        created_at: new Date().toISOString(),
        status: 'sending',
      };

      setMessages((prev) => [...prev, optimistic]);

      try {
        const res = await apiClient.post<ThreadReplyResponse>(
          `/api/v1/channels/${cid}/messages/${mid}/thread`,
          { content: content.trim(), mentioned_agent_ids: mentionedAgentIds || [] },
        );

        const confirmed = { ...toWSMessage(res), status: 'sent' as const };
        setMessages((prev) =>
          prev.map((m) => (m.id === tempId ? confirmed : m)),
        );

        // 如果这是第一条回复，thread_id 刚刚创建，更新并订阅
        if (res.thread_id && !threadIdRef.current) {
          setThreadId(res.thread_id);
        }
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

  // ---- 标记线程已读 (P25-08-F) ----

  const markRead = useCallback(async () => {
    const tid = threadIdRef.current;
    if (!tid) return;
    try {
      await apiClient.post(`/api/v1/threads/${tid}/mark-read`);
    } catch {
      // Silently fail — the request is best-effort
    }
  }, []);

  // ---- 重新加载 ----

  const refetch = useCallback(() => {
    const cid = channelIdRef.current;
    const mid = messageIdRef.current;
    if (cid && mid) {
      loadThreadMessages(cid, mid);
    }
  }, [loadThreadMessages]);

  return {
    messages,
    isLoading,
    error,
    threadId,
    loadThreadMessages,
    sendReply,
    refetch,
    markRead,
  } as const;
}
