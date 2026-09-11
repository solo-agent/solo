'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';

import { useCallback, useEffect, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import type { Task, TaskSubmission } from '@/lib/types';
import type { TaskWait } from './task-wait-dialog';
import { TaskMetricsPanel } from './task-metrics-panel';
import { Button } from '@/components/ui/button';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';

export function TaskDeliveryDialog({ taskId, open, onOpenChange, onComplete }: {
  taskId: string; open: boolean; onOpenChange: (open: boolean) => void; onComplete?: (task: Task) => void;
}) {
  const { user } = useAuth();
  const [task, setTask] = useState<Task | null>(null);
  const [submissions, setSubmissions] = useState<TaskSubmission[]>([]);
  const [waits, setWaits] = useState<TaskWait[]>([]);
  const [metricsRevision, setMetricsRevision] = useState(0);
  const [checks, setChecks] = useState<Record<string, boolean>>({});
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [reason, setReason] = useState('');
  const [contractText, setContractText] = useState('');
  const [maxRevisions, setMaxRevisions] = useState(3);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    const [latest, history, waiting] = await Promise.all([
      apiClient.get<Task>(`/api/v1/tasks/${taskId}`),
      apiClient.get<TaskSubmission[]>(`/api/v1/tasks/${taskId}/submissions`),
      apiClient.get<{ waits: TaskWait[] }>(`/api/v1/tasks/${taskId}/waits`),
    ]);
    setTask(latest); setSubmissions(history); setWaits(waiting.waits); setChecks({}); setReasons({});
    setMetricsRevision((revision) => revision + 1);
    setContractText(latest.contract?.requirements.map((item) => item.text).join('\n') ?? '');
    setMaxRevisions(latest.contract?.gate.max_revisions ?? 3);
  }, [taskId]);
  useEffect(() => {
    if (!open) return;
    setError(''); setTask(null); setReason('');
    void load().catch((err: Error) => setError(err.message));
  }, [open, load]);
  const current = submissions.find((item) => item.id === task?.current_submission_id);
  const canReview = !!current && task?.status === 'in_review' && (task.can_review ?? (task.contract?.gate.reviewer_id === user?.id && task.claimer_id !== user?.id));
  const requirements = task?.contract?.requirements ?? [];
  const resultConfirmation = task?.contract?.gate.kind === 'human' && task.contract.gate.human_review_mode === 'decision';
  const acceptReady = resultConfirmation || requirements.every((item) => checks[item.id] && reasons[item.id]?.trim());
  const review = async (decision: 'accepted' | 'rejected' | 'needs_human') => {
    if (!current || !task) return;
    setBusy(true); setError('');
    try {
      const updated = await apiClient.post<Task>(`/api/v1/tasks/${taskId}/review`, {
        submission_id: current.id, artifact_version: current.artifact_version,
        idempotency_key: crypto.randomUUID(), decision, reason: reason.trim() || (resultConfirmation && decision === 'accepted' ? '用户确认接受本次交付' : ''), evidence: [],
        checks: resultConfirmation ? [] : requirements.filter((item) => reasons[item.id]?.trim()).map((item) => ({
          requirement_id: item.id, passed: !!checks[item.id], reason: reasons[item.id].trim(),
          evidence_ids: current.evidence.map((evidence) => evidence.id),
        })),
      });
      await load(); setReason(''); onComplete?.(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : '审核失败，请重新读取任务。');
    } finally { setBusy(false); }
  };
  return (
    <Dialog width="xl" open={open} onOpenChange={(value) => { if (!busy) onOpenChange(value); }}>
      <DialogHeader>
        <DialogTitle>成果与验收</DialogTitle>
        <DialogCloseButton onClick={() => { if (!busy) onOpenChange(false); }} />
      </DialogHeader>
      <div className="min-w-0 space-y-4 break-words text-sm leading-relaxed" data-testid="task-delivery">
        {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
        {task && <>
          <p className="font-heading text-base font-bold">#{task.task_number} {task.title}</p>

          {current && <section className="space-y-3 rounded-xl border border-border bg-card p-4" aria-label="当前交付">
            <h3 className="font-heading font-bold">本次成果</h3>
            <p className="whitespace-pre-wrap">{current.handoff.summary}</p>
            {current.handoff.risks && <p className="whitespace-pre-wrap"><strong>剩余问题：</strong>{current.handoff.risks}</p>}
            {resultConfirmation && <p className="text-xs text-muted-foreground">请查看成果，符合预期后接受；有问题可以退回修改。</p>}
          </section>}
          {task.creator_id === user?.id && task.contract && !['done', 'closed'].includes(task.status) && <details className={detailSectionClass("min-w-0 space-y-3 break-words text-sm leading-relaxed")}>
            <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>验收安排</span></summary><p>保存后，当前提交需要重新交付；旧审核不会改变新要求。</p>
            <Textarea aria-label="调整验收要求" value={contractText} onChange={(e) => setContractText(e.target.value)} disabled={busy} className="min-h-24 resize-y font-body font-normal my-2" />
            <Label className="block space-y-2"><span className="block">最多提交轮次</span><Input aria-label="最多提交轮次" type="number" min={1} max={20} value={maxRevisions} onChange={(e) => setMaxRevisions(Number(e.target.value))} disabled={busy} className="font-body font-normal max-w-32" /></Label>
            <Button size="sm" disabled={busy || !contractText.trim() || maxRevisions < 1 || maxRevisions > 20} onClick={async () => {
              setBusy(true); setError('');
              try { const updated = await apiClient.patch<Task>(`/api/v1/tasks/${taskId}`, { expected_task_version: task.version, contract: { requirements: contractText.split('\n').filter((text) => text.trim()).map((text, i) => ({ id: requirements[i]?.id ?? `R${i + 1}`, text })), gate: { ...task.contract!.gate, max_revisions: maxRevisions } } }); await load(); onComplete?.(updated); }
              catch (err) { setError(err instanceof Error ? err.message : '要求已改变，请重新读取。'); }
              finally { setBusy(false); }
            }}>保存新要求</Button>
          </details>}
          <details open={canReview && !resultConfirmation} className="space-y-3">
            <summary className="cursor-pointer select-none font-bold">约定要求</summary>
          <ul className="space-y-3">
            {requirements.map((item) => <li key={item.id} className="space-y-3 rounded-xl border border-border bg-card p-3">
              <p><strong>{item.id}</strong> {item.text}</p>
              {canReview && !resultConfirmation && <>
                <label className="my-2 flex items-center gap-2"><input type="checkbox" checked={!!checks[item.id]} onChange={(e) => setChecks({ ...checks, [item.id]: e.target.checked })} disabled={busy} className="h-4 w-4 shrink-0 accent-brutal-accent" />确认此项通过</label>
                <Input aria-label={`${item.id} 检查依据`} placeholder="说明检查方法与证据" value={reasons[item.id] ?? ''} onChange={(e) => setReasons({ ...reasons, [item.id]: e.target.value })} disabled={busy} className="font-body font-normal" />
              </>}
            </li>)}
          </ul>
          </details>
          {submissions.length === 0 && <p>等待负责人提交交付说明与证据。</p>}
          <details className="space-y-3">
            <summary className="cursor-pointer select-none font-bold">验证记录与交付成本</summary>
          <p className="text-xs text-muted-foreground">任务版本 {task.version} · {task.status} · 最多提交 {task.contract?.gate.max_revisions} 轮</p>
          <p>验收方式：{task.contract?.gate.kind === 'code' ? '固定 Git 版本运行检查' : task.contract?.gate.kind === 'agent' ? '指定 Agent 审核' : resultConfirmation ? '由人确认结果' : '人工逐项审核'}</p>
          <TaskMetricsPanel key={`${task.id}:${metricsRevision}`} taskId={task.id} />
          {waits.length > 0 && <section aria-label="等待与恢复记录" className="space-y-2 border-t border-border pt-3">
            <h4 className="font-bold">等待与恢复记录</h4>
            {waits.map((wait) => <div key={wait.id} data-wait-id={wait.id} className="space-y-1">
              <p>{({ waiting: '等待条件', resumed: '已恢复原任务', cancelled: '已取消等待' })[wait.status]} · {wait.condition.description || (wait.condition.at ? new Date(wait.condition.at).toLocaleString() : `依赖任务 ${wait.condition.task_id}`)}</p>
              <p>已有进展：{wait.handoff.summary}</p><p>继续动作：{wait.next_action}</p>
              {wait.resolution_reason && <p>处理依据：{wait.resolution_reason}</p>}
            </div>)}
          </section>}
          {submissions.map((sub) => <details key={sub.id} className="space-y-3 rounded-xl border border-border bg-card p-3">
            <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass("inline break-all leading-relaxed")}>提交版本 {sub.task_version} · {sub.artifact_version} {sub.id === current?.id ? '（当前）' : '（历史）'}</span></summary>
            <p className="my-2 whitespace-pre-wrap">{sub.handoff.summary}</p>
            {(['changes', 'risks', 'next_steps'] as const).map((key) => sub.handoff[key] && <p key={key} className="my-2 whitespace-pre-wrap"><strong>{{ changes: '变更', risks: '风险', next_steps: '后续动作' }[key]}：</strong>{sub.handoff[key]}</p>)}
            {sub.evidence.map((evidence) => <div key={evidence.id} className="my-3 space-y-2 rounded-lg border border-border bg-brutal-primary-light/50 p-3">
              <p className="font-bold">{evidence.id} · {evidence.description}</p>
              {evidence.content && <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-brutal-primary-light p-3 font-mono text-xs">{evidence.content}</pre>}
              {evidence.uri && <p className="break-all">{evidence.uri}</p>}
              {evidence.sha256 && <p className="break-all font-mono text-xs">SHA256: {evidence.sha256}</p>}
            </div>)}
            {sub.reviews.map((result, index) => <div key={index} className="mt-3 border-t border-border pt-2">
              <p>{result.decision} · {result.reason}</p>
              {result.evidence?.map((evidence)=><div key={evidence.id} className="my-3 space-y-2 rounded-lg border border-border bg-brutal-primary-light/50 p-3"><p>{evidence.description}</p><pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-brutal-primary-light p-3 font-mono text-xs">{evidence.content || evidence.uri}</pre>{evidence.sha256&&<p className="break-all text-xs">SHA256: {evidence.sha256}</p>}</div>)}
              {result.checks?.map((check) => <p key={check.requirement_id}>{check.passed ? '✓' : '×'} {check.requirement_id}：{check.reason}</p>)}
            </div>)}
          </details>)}
          </details>
          {canReview && <Label className="block space-y-2"><span className="block">{resultConfirmation ? '补充意见（接受时可留空）' : '审核结论与原因'}</span><Textarea aria-label="审核结论与原因" value={reason} onChange={(e) => setReason(e.target.value)} disabled={busy} className="min-h-24 resize-y font-body font-normal" /></Label>}
        </>}
      </div>
      <DialogFooter className="flex-wrap">
        <Button variant="outline" disabled={busy} onClick={() => void load().catch((err: Error) => setError(err.message))}>重新读取</Button>
        {canReview && <>
          {!resultConfirmation && <Button variant="outline" disabled={busy || !reason.trim()} onClick={() => void review('needs_human')}>需要人工决定</Button>}
          <Button variant="outline" disabled={busy || !reason.trim()} onClick={() => void review('rejected')}>退回修改</Button>
          <Button disabled={busy || (!resultConfirmation && !reason.trim()) || !acceptReady} onClick={() => void review('accepted')}>{resultConfirmation ? '接受交付' : '通过验收'}</Button>
        </>}
      </DialogFooter>
    </Dialog>
  );
}
