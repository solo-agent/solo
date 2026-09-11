'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';

import { useId, useCallback, useEffect, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { Button } from '@/components/ui/button';

interface WorkState {
  policy: string;
  inbox: { kind: string; work_id: string; channel_id: string; task_id?: string; thinking_node_id?: string; title: string; priority: number; created_at: string; ready_at: string; last_error: string }[];
  feedback: { task_id: string; channel_id: string; title: string; source_id: string; kind: string; category: string; note: string }[];
  channel_policies: { channel_id: string; policy: string }[];
  marks: { id: string; channel_id: string; description: string; next_action: string }[];
  pending: { channel_id: string; requires_visible_result: boolean }[];
  reviews: { task_id: string; title: string }[];
  runs: { id: string; channel_id: string; thread_id?: string; activity: string; status: string }[];
  drafts: { run_id: string; content: string; sha256: string }[];
  waits: { id: string; task_id: string; task_number: number; channel_id: string; title: string; next_action: string }[];
}

export function AgentWorkPanel({ agentId, ownerId }: { agentId: string; ownerId?: string }) {
  const fieldId = useId();
  const { user } = useAuth();
  const [state, setState] = useState<WorkState | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const allowed = user?.id === ownerId;
  const load = useCallback(async () => {
    setState(await apiClient.get<WorkState>(`/api/v1/agents/${agentId}/work`));
  }, [agentId]);
  useEffect(() => { if (allowed) void load().catch((err: Error) => setError(err.message)); }, [allowed, load]);
  if (!allowed) return null;
  const mutate = async (path: string, body: unknown) => {
    setBusy(true); setError('');
    try { await apiClient.post(`/api/v1/agents/${agentId}/${path}`, body); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : '更新失败'); }
    finally { setBusy(false); }
  };
  return <section className={detailSectionClass("min-w-0 space-y-3 break-words text-sm leading-relaxed")} aria-label="成员工作状态">
    <h4><span className={detailSectionTitleClass()}>当前工作</span></h4>
    {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
    {state && <p>{state.runs[0]?.activity || (state.waits.length ? '等待工作所需的条件' : '当前没有正在执行的工作')}</p>}
    <details onToggle={(event) => { if (event.currentTarget.open) void load().catch((err: Error) => setError(err.message)); }}>
      <summary className="cursor-pointer select-none"><span className={detailSectionTitleClass()}>工作记录</span></summary>
      <p className="mt-3 text-muted-foreground">成员自行处理排队工作与未完事项。补充要求请回到原对话。</p>

    {state && <>

      <p>执行中 {state.runs.length} · 排队 {state.inbox?.length ?? state.pending.length} · 待审核 {state.reviews.length} · 待跟进 {state.marks.length} · 等待条件 {state.waits.length}</p>
      {!!state.inbox?.length && <ol aria-label="Agent 工作队列" className="space-y-2">{state.inbox.map((item, index) => <li key={`${item.kind}:${item.work_id}`} className="border-t border-border pt-2">
        <p>{index + 1}. <a className="underline" href={`/dashboard?channel=${item.channel_id}${item.task_id ? `&view=task&task=${item.task_id}` : item.thinking_node_id ? `&view=thinking&node=${item.thinking_node_id}` : ''}`}>{item.title}</a> · {({ message: '会话', review: '交付审核', wait: '继续原任务', selection: '对照评估', task: '任务', artifact: '产物协作', thinking: 'Thinking', greeting: '入场问候' } as Record<string, string>)[item.kind]}</p>
        <p className="text-xs leading-relaxed text-muted-foreground">{item.priority === 10 ? '明确请求' : '订阅与问候'} · {new Date(item.created_at).toLocaleString()}</p>
        {item.last_error && <p className="text-brutal-danger">{item.last_error} · 下次尝试：{new Date(item.ready_at).toLocaleString()}</p>}
      </li>)}</ol>}
      {state.waits.map((wait) => <p key={wait.id}>等待条件：<a className="underline" href={`/dashboard?channel=${wait.channel_id}&view=task&task=${wait.task_id}`}>#{wait.task_number} {wait.title}</a>；就绪后：{wait.next_action}</p>)}
      {state.runs.map((run) => <p key={run.id}>{run.activity}</p>)}
      {state.reviews.map((review) => <p key={review.task_id}>待审核：{review.title}</p>)}
      {!!state.feedback?.length && <details><summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>近期交付与协作反馈（{state.feedback.length}）</span></summary><ul className="space-y-2 pt-2">{state.feedback.map((item) => <li key={item.source_id}>
        <a className="underline" href={`/dashboard?channel=${item.channel_id}&view=task&task=${item.task_id}`}>{item.title}</a> · {item.category || item.kind}<p className="whitespace-pre-wrap">{item.note}</p>
      </li>)}</ul><p className="mt-2 text-xs">需要调整分工或交接时，在原频道告诉成员；涉及授权的约定由相关负责人确认。</p></details>}

      {state.marks.map((mark) => <div key={mark.id} className="space-y-2 border-t border-border pt-2">
        <p>{mark.description}</p><p>后续动作：{mark.next_action}</p>
      </div>)}
      {state.drafts.map((draft) => <details key={draft.run_id} data-draft-run-id={draft.run_id}>
        <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>发送前暂存的草稿</span></summary>
        <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-brutal-primary-light p-3 font-mono text-xs">{draft.content}</pre>
        <p className="text-xs leading-relaxed text-muted-foreground">成员会根据最新要求重新检查草稿；任务与待跟进事项单独记录。</p>
      </details>)}
    </>}
    </details>
    {state && <details>
      <summary className="cursor-pointer select-none"><span className={detailSectionTitleClass()}>消息接收</span></summary>
      <div className="space-y-3 pt-3">
      <div className="space-y-2"><Label htmlFor={`${fieldId}-field-1`} className="block">自动接收消息</Label><Select id={`${fieldId}-field-1`} aria-label="自动接收消息" value={state.policy} disabled={busy} onChange={(value) => void mutate('attention', { policy: value })} size="md" className="w-full min-w-0" options={[{ value: "all", label: "全部" }, { value: "mentions", label: "仅提及我" }, { value: "nothing", label: "暂停接收" }]} /></div>
      {state.channel_policies.map((policy) => <div key={policy.channel_id} className="flex flex-wrap items-center gap-2"><span className="break-all">频道 {policy.channel_id.slice(0, 8)}：{policy.policy}</span><Button size="sm" variant="outline" disabled={busy} onClick={() => void mutate('attention', { channel_id: policy.channel_id, policy: 'inherit' })}>恢复默认</Button></div>)}
        <p className="text-xs text-muted-foreground">修改接收范围不会取消已承担的工作。</p>
      </div>
    </details>}
  </section>;
}
