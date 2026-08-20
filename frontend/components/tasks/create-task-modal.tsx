// ============================================================================
// CreateTaskModal — simplified task creation modal
// - Single title field (no assignee, priority, description, due date)
// - "Create Task" / "Cancel" buttons
// - Keyboard: Enter to submit, Escape to close
// - Error handling: empty title validation, server error display

// ============================================================================

'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import {
  Dialog,
  DialogCloseButton,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type { CreateTaskInput } from '@/lib/types';
import { t } from '@/lib/i18n';

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

  const handleSubmit = useCallback(async () => {
    setValidationError(null);
    setSubmitError(null);

    const trimmed = title.trim();
    if (!trimmed) {
      setValidationError(t('taskTitleRequired'));
      inputRef.current?.focus();
      return;
    }

    if (trimmed.length > 500) {
      setValidationError(t('taskTitleMaxLen'));
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
      setSubmitError(err instanceof Error ? err.message : t('somethingWentWrong'));
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
  const handleOpenChange = (next: boolean) => {
    if (!isDisabled) onOpenChange(next);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogHeader>
        <DialogTitle>{t('createTask')}</DialogTitle>
        <DialogCloseButton onClick={() => handleOpenChange(false)} />
      </DialogHeader>

      <div className="space-y-4">
        <div>
          <Label htmlFor="task-create-title" className="mb-2 block">
            {t('taskTitle')}
          </Label>
          <Input
            ref={inputRef}
            id="task-create-title"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              if (validationError) setValidationError(null);
            }}
            onKeyDown={handleKeyDown}
            placeholder={t('taskTitlePlaceholder')}
            disabled={isDisabled}
            aria-required="true"
            aria-invalid={!!validationError}
            className={validationError ? 'input-error' : undefined}
          />

          {validationError && (
            <p className="mt-2 font-mono text-xs font-bold text-brutal-danger">
              {validationError}
            </p>
          )}

          {submitError && (
            <div className="mt-3 border-2 border-brutal-danger bg-brutal-danger-light p-2.5">
              <p className="font-mono text-xs font-bold text-brutal-danger">
                {submitError}
              </p>
            </div>
          )}
        </div>
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => handleOpenChange(false)}
          disabled={isDisabled}
        >
          {t('cancel')}
        </Button>
        <Button
          type="button"
          variant="primary"
          size="sm"
          onClick={handleSubmit}
          disabled={isDisabled}
        >
          {isDisabled ? t('submitting') : t('createTask')}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
