// ============================================================================
// Dialog (Modal) — accessible modal overlay with backdrop, escape-to-close
// ============================================================================

'use client';

import { useEffect, useRef, type ReactNode } from 'react';
import { t } from '@/lib/i18n';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
  width?: 'sm' | 'md' | 'lg';
}

const DIALOG_WIDTHS = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  lg: 'max-w-lg',
} as const;

export function Dialog({ open, onOpenChange, children, width = 'md' }: DialogProps) {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) {
        onOpenChange(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={(e) => {
        if (e.target === overlayRef.current) onOpenChange(false);
      }}
      ref={overlayRef}
    >
      <div
        className={cn(
          'mx-4 w-full border-4 border-black bg-card p-6 shadow-brutal-xl max-h-[90vh] overflow-y-auto',
          DIALOG_WIDTHS[width],
        )}
        role="dialog"
        aria-modal="true"
      >
        {children}
      </div>
    </div>
  );
}

export function DialogCloseButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex h-7 w-7 items-center justify-center rounded-none border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-primary-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
      aria-label={t('close')}
    >
      <X className="h-4 w-4" />
    </button>
  );
}

export function DialogHeader({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('mb-4 flex items-center justify-between', className)}>{children}</div>;
}

export function DialogTitle({ children, className }: { children: ReactNode; className?: string }) {
  return <h2 className={cn('text-lg font-heading font-bold', className)}>{children}</h2>;
}

export function DialogDescription({ children, className }: { children: ReactNode; className?: string }) {
  return <p className={cn('mt-1 text-sm text-muted-foreground', className)}>{children}</p>;
}

export function DialogFooter({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('mt-6 flex justify-end gap-2', className)}>{children}</div>;
}
