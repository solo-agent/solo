// ============================================================================
// SOLO-21-F: useChannel Hook — 频道维度的消息管理
// - 自动订阅/取消订阅频道
// - 接收实时消息更新
// - 提供发送消息方法（通过 REST API）
// ============================================================================

'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { t } from '@/lib/i18n';
import { apiClient, ApiError } from './api-client';
import { useWebSocket } from './ws-context';
import type { WSMessage, WSMessageSource } from './ws-types';

export interface UseChannelReturn {
  /** 当前频道的消息列表（按时间正序） */
  messages: WSMessage[];
  /** WebSocket 是否已连接 */
  isConnected: boolean;
  /** 发送消息 */
  sendMessage: (content: string, threadParentId?: string) => Promise<WSMessage>;
  /** 发送错误 */
  error: string | null;
  /** 清除错误 */
  clearError: () => void;
  /** 清空本地消息列表 */
  clearMessages: () => void;
}

/**
 * 频道级 Hook：自动订阅/取消订阅，维护频道消息列表。
 *
 * @param channelId - 频道 ID，传 null 时不订阅任何频道
 */
export function useChannel(channelId: string | null): UseChannelReturn {
  const { subscribe, unsubscribe, onEvent, isConnected } = useWebSocket();
  const [messages, setMessages] = useState<WSMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  // 稳定的 channelId ref，用于事件回调中比较
  const channelIdRef = useRef(channelId);
  channelIdRef.current = channelId;

  // 订阅/取消订阅频道
  useEffect(() => {
    if (!channelId) return;

    // 切换频道时清空消息
    setMessages([]);
    subscribe(channelId);

    return () => {
      unsubscribe(channelId);
    };
  }, [channelId, subscribe, unsubscribe]);

  // 监听频道相关的 WS 事件
  useEffect(() => {
    if (!channelId) return;

    const unsub = onEvent((event) => {
      // 使用 ref 读取当前 channelId，避免闭包捕获过期值
      const cid = channelIdRef.current;
      if (!cid) return;

      switch (event.type) {
        case 'message.new':
          if (event.channel_id === cid) {
            const newMsg: WSMessage = {
              id: event.id,
              channel_id: event.channel_id,
              sender_type: event.sender_type as WSMessageSource,
              sender_id: event.sender_id,
              sender_name: event.sender_name,
              display_name: event.sender_name || event.sender_id,
              content: event.content,
              content_type: event.content_type,
              thread_parent_id: event.thread_id,
              created_at: event.created_at,
            };
            setMessages((prev) => {
              if (prev.some((m) => m.id === newMsg.id)) return prev;
              return [...prev, newMsg];
            });
          }
          break;

        case 'message.updated':
          if (event.channel_id === cid) {
            setMessages((prev) =>
              prev.map((m) =>
                m.id === event.id
                  ? { ...m, content: event.content }
                  : m,
              ),
            );
          }
          break;

        case 'message.deleted':
          if (event.channel_id === cid) {
            setMessages((prev) =>
              prev.filter((m) => m.id !== event.message_id),
            );
          }
          break;
      }
    });

    return unsub;
  }, [channelId, onEvent]);

  // 发送消息（通过 REST API，WS 只负责接收推送）
  const sendMessage = useCallback(
    async (content: string, threadParentId?: string): Promise<WSMessage> => {
      if (!channelId) {
        throw new Error(t('channelSendError'));
      }

      try {
        const msg = await apiClient.post<WSMessage>(
          `/api/v1/channels/${channelId}/messages`,
          { content, thread_parent_id: threadParentId },
        );
        // 不乐观添加消息，等待 WS 推送保证消息顺序
        return msg;
      } catch (err) {
        const message =
          err instanceof ApiError ? err.message : t('messageSendError');
        setError(message);
        throw err;
      }
    },
    [channelId],
  );

  const clearError = useCallback(() => setError(null), []);
  const clearMessages = useCallback(() => setMessages([]), []);

  return {
    messages,
    isConnected,
    sendMessage,
    error,
    clearError,
    clearMessages,
  };
}
