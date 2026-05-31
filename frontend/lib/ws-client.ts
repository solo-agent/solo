// ============================================================================
// SOLO-21-F: WebSocket 客户端类
// - 自动连接、断线重连（指数退避 1s→30s）
// - 订阅管理：subscribe / unsubscribe channel
// - 事件分发：监听器模式
// - 重连后自动重新订阅之前的频道
// ============================================================================

import type { ConnectionState, WSClientCommand, WSServerEvent } from './ws-types';

export interface WSClientConfig {
  /** WebSocket 基地址，例如 ws://localhost:8080 */
  baseUrl: string;
  /** 获取 JWT token（每次调用时读取最新值） */
  getToken: () => string | null;
  /** 连接状态变化回调 */
  onStateChange?: (state: ConnectionState) => void;
  /**
   * 重连状态变化回调。
   * isReconnecting: 是否正在尝试重连（之前曾连接成功过）
   * attemptCount: 当前重连尝试次数
   */
  onReconnectChange?: (isReconnecting: boolean, attemptCount: number) => void;
  /** 收到服务端事件回调 */
  onEvent?: (event: WSServerEvent) => void;
}

const INITIAL_RECONNECT_DELAY = 1000; // 1s
const MAX_RECONNECT_DELAY = 30000;    // 30s
const BACKOFF_MULTIPLIER = 2;

export class WSClient {
  private readonly config: WSClientConfig;
  private ws: WebSocket | null = null;
  private state: ConnectionState = 'disconnected';
  private subscribedChannels: Set<string> = new Set();
  private subscribedThreads: Set<string> = new Set();
  private subscribedDMs: Set<string> = new Set();
  private reconnectDelay: number = INITIAL_RECONNECT_DELAY;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalClose: boolean = false;
  private readonly handlers: Set<(event: WSServerEvent) => void> = new Set();

  /** True if the client has ever been successfully connected */
  private hasConnectedBefore: boolean = false;
  /** Current reconnection attempt count (resets on successful connect) */
  private reconnectAttemptCount: number = 0;

  constructor(config: WSClientConfig) {
    this.config = config;
  }

  // ---- 公共方法 ----

  /** 当前连接状态 */
  getState(): ConnectionState {
    return this.state;
  }

  /** 当前已订阅的频道 ID 列表 */
  getSubscribedChannels(): string[] {
    return Array.from(this.subscribedChannels);
  }

  /** 当前已订阅的线程 ID 列表 */
  getSubscribedThreads(): string[] {
    return Array.from(this.subscribedThreads);
  }

  /** 当前已订阅的 DM 频道 ID 列表 */
  getSubscribedDMs(): string[] {
    return Array.from(this.subscribedDMs);
  }

  /** 连接 WebSocket 服务 */
  connect(): void {
    // 已连接或正在连接中，跳过
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const token = this.config.getToken();
    if (!token) {
      return;
    }

    this.intentionalClose = false;
    this.cancelReconnect();
    this.cleanupSocket();

    this.setState('connecting');

    const url = `${this.config.baseUrl}/api/v1/ws?token=${encodeURIComponent(token)}`;
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.hasConnectedBefore = true;
      this.reconnectAttemptCount = 0;
      this.setState('connected');
      this.config.onReconnectChange?.(false, 0);
      this.reconnectDelay = INITIAL_RECONNECT_DELAY;

      // 重连后自动重新订阅之前的频道
      if (this.subscribedChannels.size > 0) {
        for (const channelId of this.subscribedChannels) {
          this.sendCommand({ type: 'subscribe', channel_id: channelId });
        }
      }

      // 重连后自动重新订阅之前的线程
      if (this.subscribedThreads.size > 0) {
        for (const threadId of this.subscribedThreads) {
          this.sendCommand({ type: 'thread.subscribe', thread_id: threadId });
        }
      }

      // 重连后自动重新订阅之前的 DM
      if (this.subscribedDMs.size > 0) {
        for (const dmId of this.subscribedDMs) {
          this.sendCommand({ type: 'dm.subscribe', dm_id: dmId });
        }
      }
    };

    this.ws.onclose = () => {
      this.setState('disconnected');
      this.ws = null;

      if (!this.intentionalClose) {
        this.reconnectAttemptCount++;
        this.config.onReconnectChange?.(true, this.reconnectAttemptCount);
        this.scheduleReconnect();
      } else {
        this.hasConnectedBefore = false;
        this.reconnectAttemptCount = 0;
        this.config.onReconnectChange?.(false, 0);
      }
    };

    this.ws.onerror = () => {
      // onclose 会随后触发，不需要额外处理
    };

    this.ws.onmessage = (event: MessageEvent) => {
      this.handleMessage(event.data);
    };
  }

  /** 断开连接（不会触发重连） */
  disconnect(): void {
    this.intentionalClose = true;
    this.cancelReconnect();
    this.cleanupSocket();
    this.setState('disconnected');
  }

  /** 订阅频道（未连接时自动触发连接） */
  subscribe(channelId: string): void {
    this.subscribedChannels.add(channelId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'subscribe', channel_id: channelId });
    } else {
      // 未连接时触发连接，连接建立后会自动发送订阅
      this.connect();
    }
  }

  /** 取消订阅频道 */
  unsubscribe(channelId: string): void {
    this.subscribedChannels.delete(channelId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'unsubscribe', channel_id: channelId });
    }
  }

  /** 订阅线程（未连接时自动触发连接） */
  subscribeThread(threadId: string): void {
    this.subscribedThreads.add(threadId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'thread.subscribe', thread_id: threadId });
    } else {
      this.connect();
    }
  }

  /** 取消订阅线程 */
  unsubscribeThread(threadId: string): void {
    this.subscribedThreads.delete(threadId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'thread.unsubscribe', thread_id: threadId });
    }
  }

  /** 订阅 DM 频道（未连接时自动触发连接） */
  subscribeDM(dmId: string): void {
    this.subscribedDMs.add(dmId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'dm.subscribe', dm_id: dmId });
    } else {
      this.connect();
    }
  }

  /** 取消订阅 DM 频道 */
  unsubscribeDM(dmId: string): void {
    this.subscribedDMs.delete(dmId);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendCommand({ type: 'dm.unsubscribe', dm_id: dmId });
    }
  }

  /** 注册事件监听器，返回取消注册函数 */
  onEvent(handler: (event: WSServerEvent) => void): () => void {
    this.handlers.add(handler);
    return () => {
      this.handlers.delete(handler);
    };
  }

  // ---- 内部方法 ----

  private setState(state: ConnectionState): void {
    this.state = state;
    this.config.onStateChange?.(state);
  }

  private sendCommand(command: WSClientCommand): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      // Backend expects { type: "...", payload: {...} } envelope format
      const { type, ...payload } = command;
      this.ws.send(JSON.stringify({ type, payload }));
    }
  }

  private handleMessage(rawData: string): void {
    let event: WSServerEvent;
    try {
      // Backend Envelope: { "type": "...", "payload": {...} }
      // 将 payload 展开到顶层，形成扁平结构
      const parsed = JSON.parse(rawData);
      if (parsed.payload && typeof parsed.payload === 'object') {
        event = { type: parsed.type, ...parsed.payload } as WSServerEvent;
      } else {
        event = parsed as WSServerEvent;
      }
    } catch {
      // 无法解析的消息直接忽略
      return;
    }

    // 分发到 config 上的全局回调
    this.config.onEvent?.(event);

    // 分发到注册的监听器
    this.handlers.forEach((handler) => {
      try {
        handler(event);
      } catch {
        // 单个监听器的异常不影响其他监听器
      }
    });
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer !== null) return;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.reconnectDelay);

    // 指数退避，封顶 30s
    this.reconnectDelay = Math.min(
      this.reconnectDelay * BACKOFF_MULTIPLIER,
      MAX_RECONNECT_DELAY,
    );
  }

  private cancelReconnect(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private cleanupSocket(): void {
    if (this.ws) {
      // 清除回调以防止旧 socket 触发 onclose 导致意外重连
      this.ws.onopen = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.onmessage = null;
      this.ws.close();
      this.ws = null;
    }
  }
}
