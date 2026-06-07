// ============================================================================
// SOLO-60-F: NetworkStatus — global network offline/online indicator
// - Shows a banner at the top of the page when the browser detects
//   that the network connection is lost.
// - Uses navigator.onLine + online/offline events
// - Auto-hides when connection is restored
// ============================================================================

'use client';

import { useState, useEffect } from 'react';
import { WifiOff, Wifi } from 'lucide-react';

export function NetworkStatus() {
  const [isOnline, setIsOnline] = useState(true);
  const [show, setShow] = useState(false);

  useEffect(() => {
    // Set initial state
    setIsOnline(navigator.onLine);

    const handleOnline = () => {
      setIsOnline(true);
      // Show "back online" briefly, then hide
      setShow(true);
      setTimeout(() => setShow(false), 3000);
    };

    const handleOffline = () => {
      setIsOnline(false);
      setShow(true);
    };

    window.addEventListener('online', handleOnline);
    window.addEventListener('offline', handleOffline);

    return () => {
      window.removeEventListener('online', handleOnline);
      window.removeEventListener('offline', handleOffline);
    };
  }, []);

  if (!show) return null;

  return (
    <div
      role="alert"
      className={`fixed left-0 right-0 top-0 z-50 flex items-center justify-center gap-2 border-b-2 border-black py-2 text-sm font-medium text-black transition-transform duration-100 ease-linear ${
        isOnline ? 'bg-brutal-success' : 'bg-brutal-danger'
      }`}
    >
      {isOnline ? (
        <>
          <Wifi className="h-4 w-4" />
          <span>网络已恢复</span>
        </>
      ) : (
        <>
          <WifiOff className="h-4 w-4" />
          <span>网络连接已断开，部分功能不可用</span>
        </>
      )}
    </div>
  );
}
