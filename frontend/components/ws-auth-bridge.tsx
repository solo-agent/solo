// ============================================================================
// WSAuthBridge — 将 Auth 认证状态同步到 WebSocket 连接
// - 用户登录后自动建立 WS 连接
// - 用户登出后自动断开 WS 连接
// ============================================================================

'use client';

import { useEffect } from 'react';
import { useAuth } from '@/lib/auth-context';
import { useWebSocket } from '@/lib/ws-context';

/**
 * 监听 AuthProvider 的认证状态变化，自动连接/断开 WebSocket。
 * 必须放在 AuthProvider 和 WSProvider 的子孙节点中。
 *
 * 等待 AuthProvider 完成初始化（isLoading=false）后再做连接决策，
 * 避免刷新页面时 auth 尚未解析导致误断连。
 */
export function WSAuthBridge() {
  const { isAuthenticated, isLoading } = useAuth();
  const { connect, disconnect } = useWebSocket();

  useEffect(() => {
    if (isLoading) return; // auth 还未解析，不做任何操作
    if (isAuthenticated) {
      connect();
    } else {
      disconnect();
    }
  }, [isAuthenticated, isLoading, connect, disconnect]);

  return null;
}
