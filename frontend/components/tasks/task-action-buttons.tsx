'use client';

import { Check, RotateCcw, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { useToast } from '@/components/ui/toast';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';
import type { Task } from '@/lib/types';

type TaskAction = 'accept' | 'reject' | 'close' | 'reopen';

interface TaskActionButtonsProps {
  task: Task;
  onActionComplete?: (task: Task) => void;
}

export function TaskActionButtons({ task, onActionComplete }: TaskActionButtonsProps) {
  const { user } = useAuth();
  const { showToast } = useToast();
  const [busy, setBusy] = useState<TaskAction | null>(null);
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState('');
  const [confirmingClose, setConfirmingClose] = useState(false);

  const isCreator = !!user?.id && task.creator_id === user.id;
  const disabled = busy !== null;

  const run = async (action: TaskAction, body?: unknown) => {
    setBusy(action);
    try {
      const updated = await apiClient.post<Task>(`/api/v1/tasks/${task.id}/${action}`, body);
      showToast(t('taskActionSucceeded', { action: taskActionLabel(action) }), 'success');
      onActionComplete?.(updated);
      if (action === 'close') setConfirmingClose(false);
      setRejecting(false);
      setReason('');
    } catch {
      showToast(t('taskActionFailed', { action: taskActionLabel(action) }), 'error');
    } finally {
      setBusy(null);
    }
  };

  if (task.status === 'in_review') {
    return (
      <>
        <CloseHoverButton disabled={disabled} onClick={() => setConfirmingClose(true)} />
        {isCreator && (
          <div className="mt-2 flex flex-wrap gap-2">
            <ActionButton disabled={disabled} onClick={() => run('accept')} tone="success">
              <Check className="h-3 w-3" />
              {t('taskActionAccept')}
            </ActionButton>
            <ActionButton disabled={disabled} onClick={() => setRejecting((v) => !v)} tone="warning">
              <RotateCcw className="h-3 w-3" />
              {t('taskActionReject')}
            </ActionButton>
          </div>
        )}
        {rejecting && (
          <div className="mt-2 space-y-1">
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
              placeholder={t('taskActionReason')}
              className="input-brutal h-8 w-full px-2 py-1 font-body text-xs"
            />
            <ActionButton
              disabled={disabled || reason.trim() === ''}
              onClick={() => run('reject', { reason: reason.trim() })}
              tone="warning"
            >
              {t('taskActionSendBack')}
            </ActionButton>
          </div>
        )}
        <CloseTaskDialog
          open={confirmingClose}
          disabled={disabled}
          taskTitle={task.title}
          onOpenChange={setConfirmingClose}
          onConfirm={() => run('close')}
        />
      </>
    );
  }

  if (task.status === 'done' || task.status === 'closed') {
    if (!isCreator) return null;
    return (
      <div className="mt-2">
        <ActionButton disabled={disabled} onClick={() => run('reopen')} tone="info">
          <RotateCcw className="h-3 w-3" />
          {t('taskActionReopen')}
        </ActionButton>
      </div>
    );
  }

  return (
    <>
      <CloseHoverButton disabled={disabled} onClick={() => setConfirmingClose(true)} />
      <CloseTaskDialog
        open={confirmingClose}
        disabled={disabled}
        taskTitle={task.title}
        onOpenChange={setConfirmingClose}
        onConfirm={() => run('close')}
      />
    </>
  );
}

function CloseTaskDialog({
  open,
  disabled,
  taskTitle,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  disabled?: boolean;
  taskTitle: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <div onClick={(e) => e.stopPropagation()}>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogHeader>
          <DialogTitle>{t('taskActionCloseTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => onOpenChange(false)} />
        </DialogHeader>
        <DialogDescription>
          {t('taskActionCloseDesc', { title: taskTitle })}
        </DialogDescription>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={disabled}>
            {t('cancel')}
          </Button>
          <Button type="button" variant="danger" onClick={onConfirm} disabled={disabled}>
            {disabled ? t('taskActionClosing') : t('taskActionCloseTitle')}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}

function CloseHoverButton({ disabled, onClick }: { disabled?: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      aria-label={t('taskActionClose')}
      className="absolute right-2 top-2 z-20 inline-flex h-6 w-6 items-center justify-center border-2 border-black bg-white p-0 text-black opacity-0 shadow-brutal-sm transition-all duration-100 hover:-translate-y-px hover:bg-brutal-danger hover:shadow-brutal focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-danger focus-visible:ring-offset-2 group-hover:opacity-100 disabled:cursor-not-allowed disabled:grayscale disabled:opacity-50"
    >
      <X className="h-3.5 w-3.5" />
    </button>
  );
}

function taskActionLabel(action: TaskAction): string {
  return {
    accept: t('taskActionAccept'),
    reject: t('taskActionReject'),
    close: t('taskActionClose'),
    reopen: t('taskActionReopen'),
  }[action];
}

function ActionButton({
  children,
  disabled,
  onClick,
  tone,
}: {
  children: ReactNode;
  disabled?: boolean;
  onClick: () => void;
  tone: 'success' | 'warning' | 'muted' | 'info';
}) {
  const bg = tone === 'success'
    ? 'bg-brutal-success'
    : tone === 'warning'
      ? 'bg-brutal-warning'
      : tone === 'info'
        ? 'bg-brutal-info'
        : 'bg-brutal-muted';
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={`${bg} inline-flex cursor-pointer select-none items-center gap-1 border-2 border-black px-2 py-1 font-heading text-[10px] font-black uppercase text-black shadow-brutal-sm transition-all duration-100 hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-info focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:grayscale disabled:hover:translate-x-0 disabled:hover:translate-y-0 disabled:hover:shadow-brutal-sm disabled:opacity-50`}
    >
      {children}
    </button>
  );
}
