// ============================================================================
// CreateTaskModal — simplified task creation modal
// - Single title field (no assignee, priority, description, due date)
// - "Create Task" / "Cancel" buttons
// - Keyboard: Enter to submit, Escape to close
// - Error handling: empty title validation, server error display
// - Matches slock.ai patterns: minimal creation, add details later
// ============================================================================

'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { CreateTaskInput } from '@/lib/types';

// ---- Props ----

interface CreateTaskModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channelId?: string;
  /** Submit handler — returns created task or throws */
  onSubmit: (input: CreateTaskInput) => Promise<unknown>;
  /** Whether submission is in progress */
  isSubmitting?: boolean;
}

// ---- Component ----

export function CreateTaskModal({
  open,
  onOpenChange,
  channelId,
  onSubmit,
  isSubmitting = false,
}: CreateTaskModalProps) {
  const [title, setTitle] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const isSubmittingRef = useRef(false);

  // Reset form when modal opens
  useEffect(() => {
    if (open) {
      setTitle('');
      setValidationError(null);
      setSubmitError(null);
      // Focus input after a tick for animation
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Escape key to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open && !isSubmittingRef.current) {
        onOpenChange(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, onOpenChange]);

  // Lock body scroll
  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  const handleSubmit = useCallback(async () => {
    setValidationError(null);
    setSubmitError(null);

    const trimmed = title.trim();
    if (!trimmed) {
      setValidationError('请输入任务标题');
      inputRef.current?.focus();
      return;
    }

    if (trimmed.length > 500) {
      setValidationError('任务标题不能超过 500 个字符');
      return;
    }

    isSubmittingRef.current = true;
    try {
      await onSubmit({
        channel_id: channelId || '',
        title: trimmed,
      });
      onOpenChange(false);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : '创建任务失败，请稍后再试');
    } finally {
      isSubmittingRef.current = false;
    }
  }, [title, channelId, onSubmit, onOpenChange]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  if (!open) return null;

  const isDisabled = isSubmitting || isSubmittingRef.current;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70"
      onClick={(e) => {
        if (e.target === overlayRef.current && !isDisabled) onOpenChange(false);
      }}
      ref={overlayRef}
    >
      <div
        className="mx-4 w-full max-w-md rounded-none border-brutal-thick bg-card shadow-brutal-xl"
        role="dialog"
        aria-modal="true"
        aria-label="创建任务"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b-2 border-black px-5 py-3">
          <h2 className="font-heading text-base font-bold text-foreground">
            创建任务
          </h2>
          <button
            type="button"
            onClick={() => !isDisabled && onOpenChange(false)}
            disabled={isDisabled}
            className="flex h-7 w-7 items-center justify-center border-2 border-black bg-white hover:bg-brutal-pink-light transition-colors disabled:opacity-50"
            aria-label="关闭"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Body */}
        <div className="px-5 py-4">
          <label
            htmlFor="task-create-title"
            className="mb-2 block font-heading text-sm font-bold text-foreground"
          >
            任务标题
          </label>
          <input
            ref={inputRef}
            id="task-create-title"
            type="text"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              if (validationError) setValidationError(null);
            }}
            onKeyDown={handleKeyDown}
            placeholder="输入任务标题..."
            disabled={isDisabled}
            aria-required="true"
            aria-invalid={!!validationError}
            className={cn(
              'input-brutal',
              validationError && 'input-error',
            )}
          />

          {/* Validation error */}
          {validationError && (
            <p className="mt-2 font-mono text-xs font-bold text-brutal-red">
              {validationError}
            </p>
          )}

          {/* Submit error (server) */}
          {submitError && (
            <div className="mt-3 border-2 border-brutal-red bg-brutal-red-light p-2.5">
              <p className="font-mono text-xs font-bold text-brutal-red">
                {submitError}
              </p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 border-t-2 border-black px-5 py-3">
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            disabled={isDisabled}
            className="btn-brutal btn-brutal-sm bg-white"
          >
            取消
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={isDisabled}
            className="btn-brutal btn-brutal-sm btn-brutal-pink"
          >
            {isDisabled ? '创建中...' : '创建任务'}
          </button>
        </div>
      </div>
    </div>
  );
}
