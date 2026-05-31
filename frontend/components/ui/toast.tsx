// ============================================================================
// Toast — minimal toast notification system
// Usage: wrap app with <ToastProvider>, then use useToast() to show toasts
// - showToast(message, type?) — shows a toast for 3 seconds
// - Brutalist styling: border-2, shadow, slide-in animation
// ============================================================================

'use client';

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  type ReactNode,
} from 'react';
import { X, CheckCircle, AlertTriangle, Info } from 'lucide-react';
import { cn } from '@/lib/utils';

type ToastType = 'success' | 'error' | 'info';

interface Toast {
  id: string;
  message: string;
  type: ToastType;
}

interface ToastContextValue {
  showToast: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}

// ---- Provider ----

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const counterRef = useRef(0);

  const showToast = useCallback((message: string, type: ToastType = 'info') => {
    const id = `toast-${++counterRef.current}`;
    setToasts((prev) => [...prev, { id, message, type }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 3500);
  }, []);

  const dismissToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      {/* Toast container */}
      <div
        className="fixed bottom-6 right-6 z-[100] flex flex-col gap-2"
        aria-live="polite"
      >
        {toasts.map((toast) => (
          <ToastItem key={toast.id} toast={toast} onDismiss={dismissToast} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// ---- Toast item ----

const ICON_MAP: Record<ToastType, React.ReactNode> = {
  success: <CheckCircle className="h-4 w-4 text-brutal-lime" />,
  error: <AlertTriangle className="h-4 w-4 text-brutal-red" />,
  info: <Info className="h-4 w-4 text-brutal-lavender" />,
};

const BG_MAP: Record<ToastType, string> = {
  success: 'border-brutal-lime',
  error: 'border-brutal-red',
  info: 'border-brutal-lavender',
};

function ToastItem({
  toast,
  onDismiss,
}: {
  toast: Toast;
  onDismiss: (id: string) => void;
}) {
  return (
    <div
      className={cn(
        'flex items-center gap-3 border-2 bg-white px-4 py-3 shadow-brutal',
        'animate-slide-in-from-right min-w-[280px] max-w-[400px]',
        BG_MAP[toast.type],
      )}
    >
      {ICON_MAP[toast.type]}
      <span className="flex-1 font-body text-sm leading-snug text-foreground">
        {toast.message}
      </span>
      <button
        type="button"
        onClick={() => onDismiss(toast.id)}
        className="flex-shrink-0 text-muted-foreground hover:text-foreground transition-colors"
        aria-label="关闭提示"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
