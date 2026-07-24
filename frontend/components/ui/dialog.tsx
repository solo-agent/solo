// ============================================================================
// Dialog (Modal) — accessible modal overlay with backdrop, escape-to-close
// ============================================================================

'use client';

import { createContext, useContext, useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { t } from '@/lib/i18n';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { iconActionClass } from '@/components/ui/button';

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

const DialogTitleIdContext = createContext<string | undefined>(undefined);
const FOCUSABLE_SELECTOR = [
  'button:not([disabled])',
  'input:not([disabled])',
  'textarea:not([disabled])',
  'select:not([disabled])',
  'a[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(',');
let openDialogCount = 0;
let bodyOverflowBeforeDialogs = '';

export function Dialog({ open, onOpenChange, children, width = 'md' }: DialogProps) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const titleId = useId();

  useEffect(() => {
    if (!open) return;
    if (openDialogCount === 0) {
      bodyOverflowBeforeDialogs = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
    }
    openDialogCount += 1;
    return () => {
      openDialogCount = Math.max(0, openDialogCount - 1);
      if (openDialogCount === 0) {
        document.body.style.overflow = bodyOverflowBeforeDialogs;
      }
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    previousFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    const frame = requestAnimationFrame(() => {
      const dialog = dialogRef.current;
      if (!dialog || dialog.contains(document.activeElement)) return;
      const preferred = dialog.querySelector<HTMLElement>('[autofocus]');
      const fallback = dialog.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
      (preferred ?? fallback ?? dialog).focus();
    });
    return () => {
      cancelAnimationFrame(frame);
      if (previousFocusRef.current?.isConnected) previousFocusRef.current.focus();
    };
  }, [open]);

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.stopPropagation();
      onOpenChange(false);
      return;
    }
    if (event.key !== 'Tab') return;

    const dialog = dialogRef.current;
    if (!dialog) return;
    const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
    if (focusable.length === 0) {
      event.preventDefault();
      dialog.focus();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  if (!open) return null;

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center overscroll-contain bg-black/60"
      onClick={(e) => {
        if (e.target === overlayRef.current) onOpenChange(false);
      }}
      ref={overlayRef}
    >
      <DialogTitleIdContext.Provider value={titleId}>
        <div
          ref={dialogRef}
          className={cn(
            'mx-4 max-h-[90vh] w-full overflow-y-auto overscroll-contain border-4 border-black bg-card p-6 shadow-brutal-2xl',
            DIALOG_WIDTHS[width],
          )}
          role="dialog"
          aria-modal="true"
          aria-labelledby={titleId}
          tabIndex={-1}
          onKeyDown={handleKeyDown}
        >
          {children}
        </div>
      </DialogTitleIdContext.Provider>
    </div>,
    document.body,
  );
}

export function DialogCloseButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={iconActionClass('rounded-none shadow-brutal-sm')}
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
  const id = useContext(DialogTitleIdContext);
  return <h2 id={id} className={cn('text-lg font-heading font-bold', className)}>{children}</h2>;
}

export function DialogDescription({ children, className }: { children: ReactNode; className?: string }) {
  return <p className={cn('mt-1 text-sm text-muted-foreground', className)}>{children}</p>;
}

export function DialogFooter({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('mt-6 flex justify-end gap-2', className)}>{children}</div>;
}
