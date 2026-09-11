'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { Agent, Channel } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';
import { TaskDeliveryDialog } from '@/components/tasks/task-delivery-dialog';
import type { Selection } from './agent-selections-panel';

type TeamVersion = {
  id: string; approved_owner_ids: string[];
  lockfile: { changed_agent_id?: string; selection_id?: string; previous_revision_id?: string; based_on_version_id?: string; members: { agent_id: string; revision_id: string; owner_id?: string }[] };
};
type Team = { channel: Channel; current_version_id: string | null; versions: TeamVersion[] };

export function AgentImprovementProposals({ agent }: { agent: Agent }) {
  const [items, setItems] = useState<Selection[]>([]);
  const [teams, setTeams] = useState<Record<string, Team>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [reviewTask, setReviewTask] = useState<string | null>(null);
  const keys = useRef(new Map<string, string>());
  const load = useCallback(async () => {
    const list = await apiClient.get<Selection[]>(`/api/v1/agents/${agent.id}/selections`);
    const visible = list.filter((item, index) => item.plan.channel_id && item.status !== 'rejected' && list.findIndex((other) => other.plan.channel_id === item.plan.channel_id && other.candidate_revision_id === item.candidate_revision_id) === index);
    const loaded: Record<string, Team> = {};
    const results = await Promise.allSettled([...new Set(visible.map((item) => item.plan.channel_id!))].map(async (channelID) => {
      const [channel, records] = await Promise.all([
        apiClient.get<Channel>(`/api/v1/channels/${channelID}`),
        apiClient.get<{ current_version_id: string | null; versions: TeamVersion[] }>(`/api/v1/channels/${channelID}/team-versions`),
      ]);
      loaded[channelID] = { channel, ...records };
    }));
    if (results.some((result) => result.status === 'rejected')) setError('部分团队记录未能读取，请刷新后再处理。');
    setItems(visible); setTeams(loaded);
    return loaded;
  }, [agent.id]);
  useEffect(() => {
    const refresh = () => { void load().catch((err: Error) => setError(err.message)); };
    refresh(); window.addEventListener('focus', refresh);
    return () => window.removeEventListener('focus', refresh);
  }, [load]);
  const apply = async (item: Selection, restoreRevision?: string) => {
    const team = teams[item.plan.channel_id!];
    if (!team) return;
    setBusy(true); setError(''); setNotice('');
    const revision = restoreRevision ?? item.candidate_revision_id;
    const reason = restoreRevision ? `用户恢复 @${agent.name} 在 #${team.channel.name} 此前的工作版本` : `用户采用到 #${team.channel.name}：${item.plan.change}`;
    const data = { channel_id: team.channel.id, expected_team_version_id: team.current_version_id ?? '', reason, ...(restoreRevision ? { restore: true } : { selection_id: item.id }) };
    const identity = JSON.stringify({ revision, ...data });
    if (!keys.current.has(identity)) keys.current.set(identity, crypto.randomUUID());
    try {
      const result = await apiClient.post<{ version_id: string }>(`/api/v1/agents/${agent.id}/revisions/${revision}/apply`, { ...data, idempotency_key: keys.current.get(identity) });
      const refreshed = await load();
      const current = refreshed[team.channel.id];
      setNotice(!current ? '决定已记录，请刷新团队状态。' : current.current_version_id === result.version_id ? `已${restoreRevision ? '恢复' : '应用'}到 #${team.channel.name}，后续工作使用此版本。` : '决定已记录，等待相关主人确认团队版本。');
    } catch (err) { setError(err instanceof Error ? err.message : '应用失败，请刷新当前团队。'); }
    finally { setBusy(false); }
  };
  const dismiss = async (item: Selection) => {
    setBusy(true); setError(''); setNotice('');
    try {
      await apiClient.post(`/api/v1/agents/${agent.id}/selections/${item.id}/decide`, { decision: 'rejected', reason: '用户暂不采用这项改进' });
      await load(); setNotice('已记录暂不采用，原工作配置继续使用。');
    } catch (err) { setError(err instanceof Error ? err.message : '记录失败'); }
    finally { setBusy(false); }
  };
  if (!items.length && !error && !notice) return null;
  return <section className={detailSectionClass('min-w-0 space-y-3 break-words text-sm leading-relaxed')} aria-label="改进建议">
    <h3 className={detailSectionTitleClass()}>改进建议</h3>
    {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
    {notice && <BrutalAlert variant="success">{notice}</BrutalAlert>}
    {items.map((item) => {
      const team = teams[item.plan.channel_id!];
      const current = team?.versions.find((version) => version.id === team.current_version_id);
      const currentRevision = current?.lockfile.members.find((member) => member.agent_id === agent.id)?.revision_id;
      const applied = currentRevision === item.candidate_revision_id;
      const pending = team?.versions.find((version) => version.id !== team.current_version_id && version.lockfile.changed_agent_id === agent.id && version.lockfile.based_on_version_id === (team.current_version_id ?? '') && (!version.approved_owner_ids.includes(team.channel.created_by) || version.lockfile.members.some((member) => member.owner_id && !version.approved_owner_ids.includes(member.owner_id))));
      const restoreRevision = applied && current?.lockfile.changed_agent_id === agent.id ? current.lockfile.previous_revision_id : undefined;
      const baselineMatches = !current || currentRevision === item.baseline_revision_id;
      return <article key={item.id} data-improvement-id={item.id} className="space-y-3 border-t border-border pt-3">
        <p className="font-bold">{item.plan.change}</p><p>{item.plan.problem}</p>
        <p>作用范围：{team ? `#${team.channel.name} · @${agent.name} 的后续工作` : '原团队（记录未加载）'}</p>
        <p className="text-muted-foreground">{pending ? (applied ? '等待相关主人确认恢复此前版本' : '等待相关主人确认') : applied ? '此团队正在使用' : !baselineMatches ? '团队版本已改变，需要重新核对' : item.ready_to_apply ? '对照和接收检查已完成，等待你的决定' : '成员正在处理评测或尚未满足采用条件'}</p>
        <details className="space-y-2">
          <summary className="cursor-pointer select-none">查看对照结果与成本</summary>
          {item.results.trials.map((trial) => <p key={trial.arm} className="text-xs">{({ baseline: '原版本', candidate: '改进版本', receiver: '接收成员', reviewer: '独立审核' } as Record<string, string>)[trial.arm] ?? trial.arm}：{trial.runs} 次运行 · 已确认 {trial.actual_tokens.toLocaleString()} Token · 用量未知 {trial.unknown_runs} 次{trial.config_valid ? '' : ' · 配置已变化'}</p>)}
          {item.results.tasks.map((task) => <div key={task.task_id} className="space-y-1"><Button size="sm" variant="outline" onClick={() => setReviewTask(task.task_id)}>查看成果：{task.title}</Button><p className="text-xs text-muted-foreground">{task.verified ? '已验证当前成果' : task.last_error || '尚未完成验证'}</p></div>)}
        </details>
        <div className="flex flex-wrap gap-2">
          {!applied && !pending && <><Button size="sm" disabled={busy || !team || !item.ready_to_apply || !baselineMatches} onClick={() => void apply(item)}>采用到这个团队</Button><Button size="sm" variant="outline" disabled={busy} onClick={() => void dismiss(item)}>暂不采用</Button></>}
          {restoreRevision && !pending && <Button size="sm" variant="outline" disabled={busy} onClick={() => void apply(item, restoreRevision)}>恢复此前版本</Button>}
          {(pending || applied) && <a className="self-center underline underline-offset-4" href={`/dashboard?channel=${item.plan.channel_id}#team-records`}>查看团队记录</a>}
        </div>
      </article>;
    })}
    <Button size="sm" variant="outline" disabled={busy} onClick={() => { setError(''); void load().catch((err: Error) => setError(err.message)); }}>刷新改进状态</Button>
    {reviewTask && <TaskDeliveryDialog taskId={reviewTask} open onOpenChange={(open) => { if (!open) setReviewTask(null); }} onComplete={() => { void load().catch((err: Error) => setError(err.message)); }} />}
  </section>;
}
