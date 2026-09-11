'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';

import { useCallback, useEffect, useState } from 'react';
import { apiClient } from '@/lib/api-client';

const kinds = { intervention: '人工投入', comparison: '任务对照组', rework: '退回原因', duplicate_work: '重复工作', reexplanation: '重复解释', handoff: '额外交接', recovery: '恢复首个动作' };
interface Metric {
  qualified: boolean; elapsed_seconds: number | null; cohort: string;
  runs: { id: string; actual_tokens: number | null; accounted_tokens: number; shared: boolean; active: boolean }[];
  reworks: { id: string; reason: string; category: string }[];
  recoveries: { run_id: string; first_tool_seconds: number | null; first_action_correct: boolean | null }[];
  observations: { id: string; observer_id: string; kind: keyof typeof kinds; note: string; minutes?: number; category?: string; created_at: string }[];
}

export function TaskMetricsPanel({ taskId }: { taskId: string }) {
  const [metric, setMetric] = useState<Metric | null>(null);
  const [error, setError] = useState('');
  const load = useCallback(() => apiClient.get<Metric>(`/api/v1/tasks/${taskId}/metrics`), [taskId]);
  useEffect(() => { let current = true; void load().then((value) => { if (current) setMetric(value); }).catch((err: Error) => { if (current) setError(err.message); }); return () => { current = false; }; }, [load]);
  return <section className={detailSectionClass("min-w-0 space-y-3 break-words text-sm leading-relaxed")} aria-label="交付成本">
    <h4 className="text-foreground"><span className={detailSectionTitleClass()}>交付成本</span></h4>
    {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
    {metric && <>
      <p className="my-2">{metric.qualified ? '当前交付已验收' : '当前交付未验收'} · {metric.runs.length} 次运行 · 已确认 Token {metric.runs.reduce((sum, run) => sum + (run.actual_tokens ?? 0), 0).toLocaleString()} · 用量未知 {metric.runs.filter((run) => run.actual_tokens === null).length} 次</p>
      <p>失败与返工运行均已包含。共享运行 {metric.runs.filter((run) => run.shared).length} 次，不能跨任务重复相加。</p>
      <p>{metric.elapsed_seconds === null ? '验收历时尚未确定' : `创建到验收 ${(metric.elapsed_seconds / 60).toFixed(1)} 分钟`} · 退回 {metric.reworks.length} 次 · 已记录人工投入 {metric.observations.some((item) => item.kind === 'intervention') ? `${metric.observations.reduce((sum, item) => sum + (item.minutes ?? 0), 0)} 分钟` : '未记录'}</p>
      <p>对照组：{metric.cohort || '未记录'}</p>
      {metric.recoveries.map((item) => <p key={item.run_id}>恢复 {item.run_id.slice(0, 8)}：{item.first_tool_seconds === null ? '尚无工具事件' : `${item.first_tool_seconds.toFixed(1)} 秒到首个工具动作`} · 正确性{item.first_action_correct === null ? '未记录' : item.first_action_correct ? '通过' : '未通过'}</p>)}
      {metric.observations.map((item) => <p key={item.id} className="my-1 text-xs">{kinds[item.kind]}{item.minutes != null ? ` ${item.minutes} 分钟` : ''}{item.category ? ` · ${item.category}` : ''}：{item.note} · {item.observer_id.slice(0, 8)} · {new Date(item.created_at).toLocaleString()}</p>)}
    </>}
  </section>;
}
