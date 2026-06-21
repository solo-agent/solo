'use client';

import { Check, RotateCcw, XCircle } from 'lucide-react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useToast } from '@/components/ui/toast';
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

  const isCreator = !!user?.id && task.creator_id === user.id;
  const disabled = busy !== null;

  const run = async (action: TaskAction, body?: unknown) => {
    setBusy(action);
    try {
      const updated = await apiClient.post<Task>(`/api/v1/tasks/${task.id}/${action}`, body);
      showToast(`Task ${action} succeeded`, 'success');
      onActionComplete?.(updated);
      setRejecting(false);
      setReason('');
    } catch {
      showToast(`Task ${action} failed`, 'error');
    } finally {
      setBusy(null);
    }
  };

  if (task.status === 'in_review') {
    return (
      <div className="mt-2 space-y-2">
        {isCreator && (
          <div className="flex flex-wrap gap-2">
            <ActionButton disabled={disabled} onClick={() => run('accept')} tone="success">
              <Check className="h-3 w-3" />
              Accept
            </ActionButton>
            <ActionButton disabled={disabled} onClick={() => setRejecting((v) => !v)} tone="warning">
              <RotateCcw className="h-3 w-3" />
              Reject
            </ActionButton>
          </div>
        )}
        {rejecting && (
          <div className="space-y-1">
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
              placeholder="Reason"
              className="w-full border-2 border-black bg-white px-2 py-1 font-body text-xs"
            />
            <ActionButton
              disabled={disabled || reason.trim() === ''}
              onClick={() => run('reject', { reason: reason.trim() })}
              tone="warning"
            >
              Send back
            </ActionButton>
          </div>
        )}
        <ActionButton disabled={disabled} onClick={() => run('close')} tone="muted">
          <XCircle className="h-3 w-3" />
          Close
        </ActionButton>
      </div>
    );
  }

  if (task.status === 'done' || task.status === 'closed') {
    if (!isCreator) return null;
    return (
      <div className="mt-2">
        <ActionButton disabled={disabled} onClick={() => run('reopen')} tone="info">
          <RotateCcw className="h-3 w-3" />
          Reopen
        </ActionButton>
      </div>
    );
  }

  return (
    <div className="mt-2">
      <ActionButton disabled={disabled} onClick={() => run('close')} tone="muted">
        <XCircle className="h-3 w-3" />
        Close
      </ActionButton>
    </div>
  );
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
