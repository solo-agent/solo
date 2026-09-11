// Create a task with an optional delivery contract.

'use client';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';

import { useId, useState, useCallback, useRef, useEffect } from 'react';
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
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import type { ChannelMember, CreateTaskInput } from '@/lib/types';
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
  const fieldId = useId();
  const { user } = useAuth();
  const [requirements, setRequirements] = useState('');
  const [gateKind, setGateKind] = useState<'human' | 'agent' | 'code'>('human');
  const [humanDecision, setHumanDecision] = useState(true);
  const [repository, setRepository] = useState('');
  const [baseCommit, setBaseCommit] = useState('');
  const [checks, setChecks] = useState('[{"requirement_id":"R1","command":["make","test"]}]');
  const [reviewerId, setReviewerId] = useState('');
  const [assignee, setAssignee] = useState('');
  const [members, setMembers] = useState<ChannelMember[]>([]);
  const [title, setTitle] = useState('');
  useEffect(() => {
    if (open && channelId) void apiClient.get<ChannelMember[]>(`/api/v1/channels/${channelId}/members`).then(setMembers).catch(() => setMembers([]));
  }, [open, channelId]);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const isSubmittingRef = useRef(false);

  // Reset form when modal opens
  useEffect(() => {
    if (open) {
      setTitle(''); setRequirements(''); setGateKind('human'); setHumanDecision(true); setReviewerId(''); setAssignee('');
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
        assignee: assignee || undefined,
        contract: requirements.trim() ? {
          requirements: requirements.split('\n').map((line) => line.trim()).filter(Boolean).map((text, i) => ({ id: `R${i + 1}`, text })),
          gate: { kind: gateKind, ...(gateKind === 'human' && humanDecision ? { human_review_mode: 'decision' as const } : {}), reviewer_id: reviewerId || (gateKind === 'human' ? user?.id ?? '' : ''), max_revisions: 3, ...(gateKind === 'code' ? { code: { repository_path: repository.trim(), base_commit: baseCommit.trim(), timeout_seconds: 120, checks: JSON.parse(checks) } } : {}) },
        } : undefined,
      });
      onOpenChange(false);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : t('somethingWentWrong'));
    } finally {
      isSubmittingRef.current = false;
    }
  }, [title, channelId, onSubmit, onOpenChange, requirements, gateKind, humanDecision, reviewerId, assignee, user?.id, repository, baseCommit, checks]);

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
    <Dialog width="lg" open={open} onOpenChange={handleOpenChange}>
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

      <div className="mt-4 space-y-4 text-sm">
        <div className="space-y-2"><Label htmlFor={`${fieldId}-field-1`} className="block">负责人</Label><Select id={`${fieldId}-field-1`} aria-label="负责人" value={assignee} onChange={(value) => setAssignee(value)} disabled={isDisabled} size="md" className="w-full min-w-0" options={[{ value: "", label: "等待认领" }, ...members.filter((m) => m.member_type === 'agent').map((m) => ({ value: m.member_id, label: m.display_name }))]} /></div>
        <details className="space-y-3"><summary className="cursor-pointer select-none font-bold">自定义验收</summary>
        <Label className="block space-y-2"><span className="block">验收要求（可选，每行一项）</span><Textarea aria-label="验收要求" value={requirements} onChange={(e) => setRequirements(e.target.value)} rows={3} disabled={isDisabled} className="min-h-24 resize-y font-body font-normal" /></Label>
        {requirements.trim() && <div className="space-y-4 rounded-xl border border-border bg-brutal-primary-light/40 p-3">
          <div className="space-y-2"><Label htmlFor={`${fieldId}-field-2`} className="block">验收方式</Label><Select id={`${fieldId}-field-2`} aria-label="验收方式" value={gateKind === 'human' && humanDecision ? 'confirm' : gateKind} onChange={(value) => { setHumanDecision(value === 'confirm'); setGateKind((value === 'confirm' ? 'human' : value) as 'human' | 'agent' | 'code'); setReviewerId(''); }} disabled={isDisabled} size="md" className="w-full min-w-0" options={[{ value: "confirm", label: "由人确认交付结果" }, { value: "human", label: "人工逐项审核" }, { value: "agent", label: "指定 Agent 审核" }, { value: "code", label: "固定 Git 版本运行检查" }]} /></div>
          <div className="space-y-2"><Label htmlFor={`${fieldId}-field-3`} className="block">审核者</Label><Select id={`${fieldId}-field-3`} aria-label="审核者" value={reviewerId} onChange={(value) => setReviewerId(value)} disabled={isDisabled} size="md" className="w-full min-w-0" options={[{ value: "", label: gateKind === 'human' ? '由我验收' : '请选择审核 Agent' }, ...members.filter((m) => m.member_type === (gateKind === 'human' ? 'user' : 'agent') && m.member_id !== assignee).map((m) => ({ value: m.member_id, label: m.display_name }))]} /></div>
          {gateKind === 'code' && <div className="space-y-2">
            <p className="text-xs leading-relaxed text-muted-foreground">检查由你拥有的审核 Agent 所在 Computer 执行。负责人也需要能访问该仓库与提交。命令直接执行，请只配置你认可的检查。</p>
            <Label className="block space-y-2"><span className="block">仓库绝对路径</span><Input aria-label="仓库绝对路径" value={repository} onChange={(e) => setRepository(e.target.value)} className="font-body font-normal" /></Label>
            <Label className="block space-y-2"><span className="block">基准 Git commit</span><Input aria-label="基准 Git commit" value={baseCommit} onChange={(e) => setBaseCommit(e.target.value)} placeholder="完整 40 位提交 SHA" className="font-body font-normal" /></Label>
            <Label className="block space-y-2"><span className="block">验收命令 JSON</span><Textarea aria-label="验收命令 JSON" value={checks} onChange={(e) => setChecks(e.target.value)} rows={4} className="min-h-32 resize-y font-mono text-xs font-normal" /></Label>
            <p className="text-xs leading-relaxed text-muted-foreground">每项要求 R1、R2… 对应一个 command 参数数组。整体超时 120 秒。</p>
          </div>}
        </div>}
        </details>
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
