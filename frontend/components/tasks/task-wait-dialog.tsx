'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';

import { useCallback, useEffect, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { Task } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton } from '@/components/ui/dialog';

export interface TaskWait {
  id: string;
  status: 'waiting' | 'resumed' | 'cancelled';
  condition: { kind: string; description?: string; task_id?: string; at?: string };
  handoff: { summary: string };
  next_action: string;
  fulfillment: Record<string, unknown>;
  resolution_reason: string;
  blocker: string;
  run_id?: string;
}
interface WaitState { waits: TaskWait[]; can_manage: boolean; can_signal: boolean }

export function TaskWaitDialog({ task, onClose }: { task: Task; onClose: () => void }) {
  const [state, setState] = useState<WaitState>({ waits: [], can_manage: false, can_signal: false });
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const load = useCallback(async () => setState(await apiClient.get<WaitState>(`/api/v1/tasks/${task.id}/waits`)), [task.id]);
  useEffect(() => { void load().catch((e: Error) => setError(e.message)); }, [load]);
  const execute = async (action: () => Promise<unknown>) => {
    setBusy(true); setError('');
    try { await action(); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : '操作失败'); }
    finally { setBusy(false); }
  };
  const resolve = (wait: TaskWait, action: string) => execute(() => apiClient.post(`/api/v1/tasks/${task.id}/resolve-wait`, { wait_id: wait.id, action, reason }));
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose(); }} width="lg">
    <DialogHeader><DialogTitle>工作等待 · #{task.task_number}</DialogTitle><DialogCloseButton onClick={() => { if (!busy) onClose(); }} /></DialogHeader>
    <div className="min-w-0 space-y-4 break-words text-sm leading-relaxed">
      <p className="text-muted-foreground">条件满足后自动通知原负责人继续这项任务。等待期间可处理其他工作；交付仍需按原要求验收。</p>
      {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
      <Button variant="outline" disabled={busy} onClick={() => void execute(load)}>刷新等待状态</Button>
      {state.waits.map((wait) => <article key={wait.id} data-wait-id={wait.id} className="space-y-2 rounded-xl border border-border bg-card p-4">
        <p className="font-bold">{wait.status === 'waiting' ? (Object.keys(wait.fulfillment).length ? '条件已确认，等待负责人空闲' : '等待条件') : wait.status === 'resumed' ? '已恢复原任务' : '已取消等待'}</p>
        <p>{wait.condition.description || (wait.condition.at ? `到达 ${new Date(wait.condition.at).toLocaleString()}` : `任务 ${wait.condition.task_id} 验收完成`)}</p>
        <details><summary className="cursor-pointer">查看等待记录</summary><div className="space-y-2 pt-2"><p>已有进展：{wait.handoff.summary}</p><p>继续动作：{wait.next_action}</p></div></details>
        {wait.blocker && <p role="status">{wait.blocker}</p>}
        {wait.resolution_reason && <p>处理依据：{wait.resolution_reason}</p>}
        {wait.run_id && <p>恢复执行：<code className="break-all text-xs">{wait.run_id}</code></p>}
        {wait.status === 'waiting' && state.can_manage && <div className="space-y-2">
          <Label className="block space-y-2"><span className="block">{wait.condition.kind === 'signal' && !Object.keys(wait.fulfillment).length ? '资料或确认依据' : '取消等待的原因'}</span><Textarea aria-label="条件证据或取消原因" value={reason} onChange={(e) => setReason(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
          <div className="flex flex-wrap gap-2">
            {wait.condition.kind === 'signal' && state.can_signal && !Object.keys(wait.fulfillment).length && <Button disabled={busy || !reason.trim()} onClick={() => void resolve(wait, 'signal')}>确认条件已满足</Button>}
            <Button variant="outline" disabled={busy || !reason.trim()} onClick={() => void resolve(wait, 'cancel')}>取消这次等待</Button>
          </div>
        </div>}
      </article>)}

    </div>
  </Dialog>;
}
