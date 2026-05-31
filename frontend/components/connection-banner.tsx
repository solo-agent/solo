// ============================================================================
// SOLO-61-F: ConnectionBanner — global WebSocket reconnection banner
// - Shows "重新连接中..." when the WS client is reconnecting
// - Shows "已连接" briefly after reconnection succeeds
// - Shows "连接断开" when disconnected but not reconnecting
// ============================================================================

'use client';

import { useEffect, useState, useRef } from 'react';
import { Loader2, Wifi, WifiOff } from 'lucide-react';
import { useWebSocket } from '@/lib/ws-context';
import { usePathname } from 'next/navigation';

/**
 * Connection Banner — renders at the top of the page when WebSocket
 * connection is lost or reconnecting.
 * - isReconnecting && !connected: "正在重新连接..." (yellow)
 * - !isReconnecting && !connected: "连接已断开" (red, after initial connect)
 * - Just reconnected: "连接已恢复" (green, auto-hide after 3s)
 */
export function ConnectionBanner() {
  const { connectionState, isReconnecting, isConnected } = useWebSocket();
  const pathname = usePathname();
  const isAuthPage = pathname.startsWith('/auth');
  const [visible, setVisible] = useState(false);
  const [bannerText, setBannerText] = useState('');
  const [bannerType, setBannerType] = useState<'reconnecting' | 'disconnected' | 'connected'>('disconnected');
  const wasDisconnectedRef = useRef(false);

  useEffect(() => {
    // Don't show connection banner on auth pages
    if (isAuthPage) {
      setVisible(false);
      return;
    }
    if (isConnected) {
      // Just reconnected — show success momentarily
      if (wasDisconnectedRef.current) {
        setBannerText('连接已恢复');
        setBannerType('connected');
        setVisible(true);
        const timer = setTimeout(() => {
          setVisible(false);
          wasDisconnectedRef.current = false;
        }, 2000);
        return () => clearTimeout(timer);
      }
      setVisible(false);
      wasDisconnectedRef.current = false;
    } else if (isReconnecting) {
      wasDisconnectedRef.current = true;
      setBannerText('正在重新连接...');
      setBannerType('reconnecting');
      setVisible(true);
    } else if (connectionState === 'disconnected' && wasDisconnectedRef.current) {
      setBannerText('连接已断开');
      setBannerType('disconnected');
      setVisible(true);
    } else if (connectionState === 'disconnected') {
      // Initial disconnected state (before first connect attempt)
      setVisible(false);
    } else if (connectionState === 'connecting') {
      // First time connecting — no banner needed
      if (!wasDisconnectedRef.current) {
        setVisible(false);
      } else {
        setBannerText('正在重新连接...');
        setBannerType('reconnecting');
        setVisible(true);
      }
    }
  }, [isConnected, isReconnecting, connectionState]);

  if (!visible) return null;

  const bgColor = bannerType === 'reconnecting'
    ? 'bg-amber-500'
    : bannerType === 'disconnected'
      ? 'bg-destructive'
      : 'bg-green-500';

  const IconComponent = bannerType === 'reconnecting'
    ? Loader2
    : bannerType === 'disconnected'
      ? WifiOff
      : Wifi;

  return (
    <div
      role="alert"
      className={`fixed left-0 right-0 top-0 z-50 flex items-center justify-center gap-2 py-1.5 text-xs font-medium text-white ${bgColor} animate-in slide-in-from-top-0.5 transition-all duration-300`}
    >
      <IconComponent
        className={`h-3.5 w-3.5 ${bannerType === 'reconnecting' ? 'animate-spin' : ''}`}
      />
      <span>{bannerText}</span>
    </div>
  );
}
