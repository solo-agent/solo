'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { Button } from '@/components/ui/button';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton } from '@/components/ui/dialog';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import type { Channel } from '@/lib/types';

type Version = { approved_owner_ids: string[]; id: string; reason: string; lockfile: { schema_version: number; based_on_version_id?: string; members: { agent_id: string; owner_id?: string; name: string; revision_id: string; config_hash: string }[]; relationships: unknown[] } };
type Agreement = { id: string; from_name: string; to_name: string; from_owner_id: string; to_owner_id: string; proposed_by: string; proposed_by_agent_name?: string; instruction: string; status: string; approved_owner_ids: string[] };

export function TeamVersionsPanel({ channel }: { channel: Channel }) {
  const { user } = useAuth();
  const menu = useRef<HTMLDetailsElement>(null);
  const [open, setOpen] = useState(false);
  const [data, setData] = useState<{ current_version_id: string | null; versions: Version[] }>();
  const [agreements, setAgreements] = useState<Agreement[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    const [versions, proposals] = await Promise.all([
      apiClient.get<{ current_version_id: string | null; versions: Version[] }>('/api/v1/channels/' + channel.id + '/team-versions'),
      apiClient.get<Agreement[]>('/api/v1/channels/' + channel.id + '/team-agreements'),
    ]);
    setData(versions); setAgreements(proposals);
  }, [channel.id]);
  useEffect(() => {
    setData(undefined); setAgreements([]); setError('');
    if (open) void load().catch((err: Error) => setError(err.message));
  }, [open, load]);
  useEffect(() => {
    const fromLink = () => { if (window.location.hash === '#team-records') setOpen(true); };
    fromLink(); window.addEventListener('hashchange', fromLink);
    return () => window.removeEventListener('hashchange', fromLink);
  }, [channel.id]);
  const close = () => {
    if (busy) return;
    setOpen(false);
    if (window.location.hash === '#team-records') window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search);
  };
  const act = async (operation: () => Promise<unknown>) => {
    setBusy(true); setError('');
    try { await operation(); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : '操作失败，请重新读取。'); }
    finally { setBusy(false); }
  };
  const decideAgreement = (id: string, decision: string) => act(() => apiClient.post('/api/v1/channels/' + channel.id + '/team-agreements/' + id, { decision }));
  const pendingVersions = data?.versions.filter((version) => (version.lockfile.based_on_version_id === undefined || version.lockfile.based_on_version_id === (data.current_version_id ?? '')) && (channel.created_by === user?.id || version.lockfile.members.some((member) => member.owner_id === user?.id)) && !version.approved_owner_ids?.includes(user?.id ?? '')) ?? [];
  return <>
    <details ref={menu} className="relative ml-2 shrink-0" onKeyDown={(event) => { if (event.key === 'Escape' && menu.current) menu.current.open = false; }}>
      <summary aria-label="频道更多" className="flex h-8 w-8 cursor-pointer list-none items-center justify-center rounded-lg border border-border bg-card text-foreground [&::-webkit-details-marker]:hidden"><MoreHorizontal className="h-4 w-4" /></summary>
      <div className="absolute right-0 top-full z-30 mt-2 min-w-36 rounded-xl border border-border bg-card p-1.5 shadow-lg">
        <Button variant="ghost" size="sm" className="w-full justify-start" onClick={() => { if (menu.current) menu.current.open = false; setOpen(true); }}>团队记录</Button>
      </div>
    </details>
    <Dialog open={open} onOpenChange={(value) => { if (value) setOpen(true); else close(); }} width="lg">
      <DialogHeader><DialogTitle>团队记录 · {channel.name}</DialogTitle><DialogCloseButton onClick={close} /></DialogHeader>
      <div className="min-w-0 space-y-4 break-words text-sm leading-relaxed">
        {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
        {pendingVersions.map((version) => <section key={version.id} className={detailSectionClass('space-y-3')}>
          <h4 className="font-bold">待确认团队变更</h4><p>{version.reason}</p>
          <p>{version.lockfile.members.map((member) => member.name).join(' · ')}</p>
          <Button size="sm" disabled={busy} onClick={() => void act(() => apiClient.post('/api/v1/channels/' + channel.id + '/team-versions', { version_id: version.id, reason: '确认我的成员参与这次团队变更' }))}>同意我的成员使用此版本</Button>
        </section>)}
        <section className={detailSectionClass('space-y-3')} aria-label="团队版本历史">
          <h4><span className={detailSectionTitleClass()}>版本历史</span></h4>
          {data && data.versions.length === 0 && <p className="text-muted-foreground">尚无已记录的团队版本。</p>}
          {data?.versions.map((version) => <details key={version.id} className="border-t border-border pt-3">
            <summary className="cursor-pointer">{version.reason}{data.current_version_id === version.id ? '（当前）' : ''}</summary>
            <p className="my-2">{version.lockfile.members.map((member) => member.name).join(' · ')}</p>
            <p className="mb-2 break-all text-xs text-muted-foreground">版本 {version.id}</p>
            <Button size="sm" variant="outline" onClick={() => {
              const url = URL.createObjectURL(new Blob([JSON.stringify(version.lockfile, null, 2)], { type: 'application/json' }));
              const link = document.createElement('a'); link.href = url; link.download = 'solo-team-' + version.id + '.json'; link.click(); URL.revokeObjectURL(url);
            }}>导出 Lockfile</Button>
          </details>)}
        </section>
        <section className={detailSectionClass('space-y-3')} aria-label="合作约定记录">
          <h4><span className={detailSectionTitleClass()}>合作约定</span></h4>
          <p className="text-muted-foreground">成员根据工作提出分工；涉及你的授权时，在这里确认具体范围。</p>
          {data && agreements.length === 0 && <p className="text-muted-foreground">暂无合作约定。</p>}
          {agreements.map((item) => <div key={item.id} data-agreement-id={item.id} className="space-y-2 border-t border-border pt-3">
            <p>{item.from_name} → {item.to_name} · {({ pending: '待确认', accepted: '已生效', withdrawn: '已撤回' } as Record<string, string>)[item.status] ?? item.status}</p>
            <p>{item.instruction}</p>{item.proposed_by_agent_name && <p className="text-xs text-muted-foreground">由 @{item.proposed_by_agent_name} 提出</p>}
            <div className="flex flex-wrap gap-2">
              {item.status === 'pending' && [item.from_owner_id, item.to_owner_id].includes(user?.id ?? '') && !item.approved_owner_ids.includes(user?.id ?? '') && <Button size="sm" disabled={busy} onClick={() => void decideAgreement(item.id, 'accept')}>同意我的参与范围</Button>}
              {item.status !== 'withdrawn' && [item.from_owner_id, item.to_owner_id, item.proposed_by].includes(user?.id ?? '') && <Button size="sm" variant="outline" disabled={busy} onClick={() => void decideAgreement(item.id, 'withdraw')}>撤回我的参与</Button>}
            </div>
          </div>)}
        </section>
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void load().catch((err: Error) => setError(err.message))}>刷新记录</Button>
      </div>
    </Dialog>
  </>;
}
