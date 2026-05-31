// ============================================================================
// SOLO-21-F: WebSocket React Context — 全局 WS 状态管理
// - WSProvider: 管理 WSClient 生命周期（创建、连接、销毁）
// - useWebSocket: 获取 WS 连接状态和订阅控制
// ============================================================================

'use client';

import {
  createContext,
  useContext,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { defaultTokenStorage } from './api-client';
import { WSClient, type WSClientConfig } from './ws-client';
import type {
  ConnectionState,
  WSMessage,
  WSServerEvent,
} from './ws-types';

// ---- Context 类型 ----

export interface WSContextValue {
  /** 是否已连接 */
  isConnected: boolean;
  /** 当前连接状态（断开/连接中/已连接） */
  connectionState: ConnectionState;
  /** 是否正在尝试重连（之前曾连接成功过） */
  isReconnecting: boolean;
  /** 当前重连尝试次数 */
  reconnectAttemptCount: number;
  /** 最新收到的消息（跨所有频道） */
  lastMessage: WSMessage | null;
  /** 手动建立连接 */
  connect: () => void;
  /** 手动断开连接 */
  disconnect: () => void;
  /** 订阅频道 */
  subscribe: (channelId: string) => void;
  /** 取消订阅频道 */
  unsubscribe: (channelId: string) => void;
  /** 订阅线程 */
  subscribeThread: (threadId: string) => void;
  /** 取消订阅线程 */
  unsubscribeThread: (threadId: string) => void;
  /** 订阅 DM 频道 */
  subscribeDM: (dmId: string) => void;
  /** 取消订阅 DM 频道 */
  unsubscribeDM: (dmId: string) => void;
  /**
   * 注册全局 WS 事件监听器。
   * 返回取消注册函数，在组件卸载或 effect 清理时调用。
   */
  onEvent: (handler: (event: WSServerEvent) => void) => () => void;
}

const WSContext = createContext<WSContextValue | null>(null);

// ---- Provider ----

interface WSProviderProps {
  children: ReactNode;
  /**
   * 覆盖 WS 服务地址。
   * 默认从 `NEXT_PUBLIC_API_URL` 推导，将 http(s) 切换为 ws(s)。
   */
  wsBaseUrl?: string;
  /**
   * 覆盖 token 读取函数。
   * 默认从 localStorage 读取 `access_token`。
   */
  getToken?: () => string | null;
}

export function WSProvider({
  children,
  wsBaseUrl,
  getToken,
}: WSProviderProps) {
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('disconnected');
  const [isReconnecting, setIsReconnecting] = useState(false);
  const [reconnectAttemptCount, setReconnectAttemptCount] = useState(0);
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null);

  const clientRef = useRef<WSClient | null>(null);
  const listenersRef = useRef<Set<(event: WSServerEvent) => void>>(new Set());

  // 初始化 WS 客户端（仅执行一次）
  useEffect(() => {
    const baseUrl =
      wsBaseUrl ??
      (process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080').replace(
        /^http/,
        'ws',
      );

    const resolvedGetToken = getToken ?? defaultTokenStorage.getAccessToken;

    const config: WSClientConfig = {
      baseUrl,
      getToken: resolvedGetToken,
      onStateChange: setConnectionState,
      onReconnectChange: (reconnecting, attempt) => {
        setIsReconnecting(reconnecting);
        setReconnectAttemptCount(attempt);
      },
      onEvent: (event) => {
        // 记录最新的 message.new 事件（后端 Envelope 解包后的扁平结构）
        if (event.type === 'message.new') {
          setLastMessage({
            id: event.id,
            channel_id: event.channel_id,
            sender_type: event.sender_type as WSMessage['sender_type'],
            sender_id: event.sender_id,
            sender_name: event.sender_name,
            display_name: event.sender_name,
            content: event.content,
            content_type: event.content_type,
            thread_parent_id: event.thread_id,
            created_at: event.created_at,
          });
        }
        // 分发到 React 侧注册的监听器
        listenersRef.current.forEach((handler) => {
          try {
            handler(event);
          } catch {
            // 单个监听器的异常不影响其他监听器
          }
        });
      },
    };

    const client = new WSClient(config);
    clientRef.current = client;

    return () => {
      client.disconnect();
      clientRef.current = null;
    };
    // 依赖项仅在挂载时求值，后续变化由 onStateChange 和 ref 处理
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const connect = useCallback(() => {
    clientRef.current?.connect();
  }, []);

  const disconnect = useCallback(() => {
    clientRef.current?.disconnect();
  }, []);

  const subscribe = useCallback((channelId: string) => {
    clientRef.current?.subscribe(channelId);
  }, []);

  const unsubscribe = useCallback((channelId: string) => {
    clientRef.current?.unsubscribe(channelId);
  }, []);

  const subscribeThread = useCallback((threadId: string) => {
    clientRef.current?.subscribeThread(threadId);
  }, []);

  const unsubscribeThread = useCallback((threadId: string) => {
    clientRef.current?.unsubscribeThread(threadId);
  }, []);

  const subscribeDM = useCallback((dmId: string) => {
    clientRef.current?.subscribeDM(dmId);
  }, []);

  const unsubscribeDM = useCallback((dmId: string) => {
    clientRef.current?.unsubscribeDM(dmId);
  }, []);

  const onEvent = useCallback(
    (handler: (event: WSServerEvent) => void): (() => void) => {
      listenersRef.current.add(handler);
      return () => {
        listenersRef.current.delete(handler);
      };
    },
    [],
  );

  const value = useMemo<WSContextValue>(
    () => ({
      isConnected: connectionState === 'connected',
      connectionState,
      isReconnecting,
      reconnectAttemptCount,
      lastMessage,
      connect,
      disconnect,
      subscribe,
      unsubscribe,
      subscribeThread,
      unsubscribeThread,
      subscribeDM,
      unsubscribeDM,
      onEvent,
    }),
    [
      connectionState,
      isReconnecting,
      reconnectAttemptCount,
      lastMessage,
      connect,
      disconnect,
      subscribe,
      unsubscribe,
      subscribeThread,
      unsubscribeThread,
      subscribeDM,
      unsubscribeDM,
      onEvent,
    ],
  );

  return <WSContext.Provider value={value}>{children}</WSContext.Provider>;
}

// ---- Hook ----

export function useWebSocket(): WSContextValue {
  const context = useContext(WSContext);
  if (!context) {
    throw new Error('useWebSocket 必须在 WSProvider 内使用');
  }
  return context;
}

