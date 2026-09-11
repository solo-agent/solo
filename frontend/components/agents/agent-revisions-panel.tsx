'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Select } from '@/components/ui/select';

import { useId, useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useBackendMeta } from '@/lib/hooks/use-backend-meta';
import { Button } from '@/components/ui/button';
import type { Agent, SkillBundle } from '@/lib/types';
import { AgentSelectionsPanel } from './agent-selections-panel';
import { AgentImprovementProposals } from './agent-improvement-proposals';

type Config = { system_prompt: string; model_provider: string; model_name: string; custom_args: string[]; skills?: SkillBundle[] };
type Revision = { id: string; config: Config; config_hash: string; summary: string; published: boolean; accepted_selection: boolean; evaluation_agent_id?: string; evaluations: { task_id: string; title: string; status: string }[]; deliveries: { submissions: number; accepted: number; rejected: number; needs_human: number }; measurements: { runs: number; completed: number; mean_seconds: number | null; input_tokens: number | null; output_tokens: number | null } };

function skillContents(skills: SkillBundle[]) {
 return JSON.stringify(skills.map((skill) => ({ name: skill.name, files: skill.files.map((file) => ({ path: file.path, content: file.content, executable: !!file.executable })).sort((a,b) => a.path.localeCompare(b.path)) })).sort((a,b) => a.name.localeCompare(b.name)));
}

function ManualAgentRevisionsPanel({ agent }: { agent: Agent }) {
  const fieldId = useId();
 const { user } = useAuth();
 const { metas } = useBackendMeta();
 const [items, setItems] = useState<Revision[]>([]);
 const skillsEdited = useRef(false);
 const [skills, setSkills] = useState<SkillBundle[]>(agent.skills ?? []);
 const [summary, setSummary] = useState('');
 const [prompt, setPrompt] = useState(agent.system_prompt ?? '');
 const [provider, setProvider] = useState(agent.model_provider);
 const [model, setModel] = useState(agent.model_name);
 const [requirement, setRequirement] = useState('');
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState('');
 const [notice, setNotice] = useState('');
 const allowed = user?.id === agent.owner_id;
 const load = useCallback(async () => setItems(await apiClient.get<Revision[]>(`/api/v1/agents/${agent.id}/revisions`)), [agent.id]);
 useEffect(() => { if (allowed) void load().catch((err: Error) => setError(err.message)); }, [allowed, load]);
 useEffect(() => { if (allowed) void apiClient.get<Agent>(`/api/v1/agents/${agent.id}`).then((current) => { if (!skillsEdited.current) setSkills(current.skills ?? []); }).catch((err: Error) => setError(err.message)); }, [allowed, agent.id]);
 const addSkill = async (files: FileList | null) => {
  if (!files?.length) return;
  skillsEdited.current = true;
  setBusy(true); setError('');
  try {
   const selected = Array.from(files);
   if (selected.length > 500 || selected.reduce((n, file) => n + file.size, 0) > 4 * 1024 * 1024) throw new Error('Skill 最多包含 500 个文件、4 MiB 内容');
   const contents = await Promise.all(selected.map(async (file) => {
    const path = file.webkitRelativePath.split('/').slice(1).join('/');
    const bytes = new Uint8Array(await file.arrayBuffer());
    let binary = ''; for (const byte of bytes) binary += String.fromCharCode(byte);
    return { path, content: btoa(binary), executable: false };
   }));
   const skill = contents.find((file) => file.path === 'SKILL.md');
   if (!skill) throw new Error('请选择根目录包含 SKILL.md 的 Skill 文件夹');
   const text = new TextDecoder().decode(Uint8Array.from(atob(skill.content), (c) => c.charCodeAt(0)));
   const name = text.match(/^name:\s*["']?([a-z0-9-]+)["']?\s*$/m)?.[1];
   if (!name) throw new Error('SKILL.md 需要有效的 name');
   setSkills((previous) => [...previous.filter((item) => item.name !== name), { name, files: contents }]);
  } catch (err) { setError(err instanceof Error ? err.message : '读取 Skill 失败'); }
  finally { setBusy(false); }
 };
 if (!allowed) return null;
 const create = async () => {
  setBusy(true); setError(''); setNotice('');
  let evaluationId: string | undefined;
  try {
   const config = { system_prompt: prompt, model_provider: provider, model_name: model, custom_args: agent.custom_args ?? [], ...(skills.length ? { skills } : {}) };
   const existing=items.find((item)=>item.config.system_prompt===config.system_prompt&&item.config.model_provider===config.model_provider&&item.config.model_name===config.model_name&&JSON.stringify(item.config.custom_args)===JSON.stringify(config.custom_args)&&skillContents(item.config.skills ?? [])===skillContents(skills));
   let clone: Agent;
   if (existing?.evaluation_agent_id) clone=await apiClient.get<Agent>(`/api/v1/agents/${existing.evaluation_agent_id}`);
   else {
    const channel=await apiClient.post<{id:string}>('/api/v1/channels',{name:`eval-${agent.id.slice(0,8)}-${Date.now().toString(36)}`,description:`${agent.name} 的独立评测`});
    try { clone=await apiClient.post<Agent>(`/api/v1/channels/${channel.id}/agents`, { ...config, name: `${agent.name}-eval-${Date.now().toString(36)}`, computer_id: agent.computer_id, description: `评测副本：${summary}` }); }
    catch(err){ await apiClient.delete(`/api/v1/channels/${channel.id}`).catch(()=>undefined);throw err; }
   }
   evaluationId = clone.id;
   await apiClient.post(`/api/v1/agents/${agent.id}/revisions`, { config, summary, evaluation_agent_id: clone.id });
   const task = await apiClient.post<{ task_number: number }>(`/api/v1/channels/${clone.home_channel_id}/tasks`, { title: `评测：${summary}`, description: '执行本轮评测，提交实际产物与测试证据。', assignee: clone.id, contract: { requirements: requirement.split('\n').filter((r) => r.trim()).map((text, i) => ({ id: `R${i + 1}`, text })), gate: { kind: 'human', reviewer_id: user?.id, max_revisions: 3 } } });
   setNotice(`已建立独立评测成员 ${clone.name} 和任务 #${task.task_number}。单侧验收用于检查候选，正式发布还需新旧版本对照。`); await load();
  } catch (err) { setError(`${err instanceof Error ? err.message : '创建失败'}${evaluationId ? `；已建立评测成员 ${evaluationId}，可在频道继续安排评测。` : ''}`); }
  finally { setBusy(false); }
 };
 const publish = async (item: Revision) => {
  setBusy(true); setError('');
  try { await apiClient.post(`/api/v1/agents/${agent.id}/revisions/${item.id}/publish`, { evaluation_task_id: item.evaluations.find((t) => t.status === 'done')?.task_id, reason: summary.trim() || (item.published ? 'Owner restored this published revision' : 'Owner selected the accepted evaluation') }); await load(); setNotice('已更新正式成员配置。后续未固定团队版本的运行使用此版本。'); }
  catch (err) { setError(err instanceof Error ? err.message : '发布失败'); }
  finally { setBusy(false); }
 };
 return <div className="min-w-0 space-y-3 break-words text-sm leading-relaxed" aria-label="手动改进配置">
  <p className="text-muted-foreground">评测副本使用独立频道、工作目录和记忆；频道按当前 Workspace 的规则可见。单侧验收用于检查候选，正式发布须完成下方新旧版本对照与接收检查。</p>
  {error && <BrutalAlert variant="error">{error}</BrutalAlert>}{notice && <BrutalAlert variant="success">{notice}</BrutalAlert>}
  <Label className="block space-y-2"><span className="block">改进说明</span><Input aria-label="改进说明" value={summary} onChange={(e) => setSummary(e.target.value)} className="font-body font-normal" /></Label>
  <Label className="block space-y-2"><span className="block">候选工作说明</span><Textarea aria-label="候选工作说明" value={prompt} onChange={(e) => setPrompt(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
  <div className="space-y-2"><Label htmlFor={`${fieldId}-field-1`} className="block">候选 Provider</Label><Select id={`${fieldId}-field-1`} aria-label="候选 Provider" value={provider} onChange={(value) => setProvider(value)} size="md" className="w-full min-w-0" options={[...new Set([...Object.keys(metas), 'openai', 'anthropic', agent.model_provider])].sort().map((value) => ({ value, label: value }))} /></div>
  <Label className="block space-y-2"><span className="block">候选模型</span><Input aria-label="候选模型" value={model} onChange={(e)=>setModel(e.target.value)} className="font-body font-normal" /></Label>
  <Label className="block space-y-2"><span className="block">加入 Skill 文件夹</span><Input aria-label="加入 Skill 文件夹" type="file" {...{ webkitdirectory: '', directory: '' }} disabled={busy} onChange={(e) => { void addSkill(e.target.files); e.target.value = ''; }} className="font-body font-normal" /></Label>
  <p className="text-xs leading-relaxed text-muted-foreground">只选择可复用文件。完整文件包将进入版本记录；脚本需要直接执行时勾选执行权限。</p>
  {skills.map((skill) => <details key={skill.name} className="space-y-3 rounded-lg border border-border p-3"><summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>{skill.name} · {skill.files.length} 个文件</span></summary>
   <Button size="sm" variant="outline" disabled={busy} onClick={() => { skillsEdited.current = true; setSkills((items) => items.filter((item) => item.name !== skill.name)); }}>移除 {skill.name}</Button>
   {skill.files.map((file) => <label key={file.path} className="flex items-start gap-2 break-all text-xs leading-relaxed"><input type="checkbox" checked={!!file.executable} disabled={busy} onChange={(e) => { skillsEdited.current = true; setSkills((items) => items.map((item) => item.name !== skill.name ? item : { ...item, sha256: undefined, files: item.files.map((entry) => entry.path === file.path ? { ...entry, executable: e.target.checked } : entry) })); }} className="h-4 w-4 shrink-0 accent-brutal-accent" /> 可执行 · {file.path}</label>)}
  </details>)}
  <Label className="block space-y-2"><span className="block">评测要求（每行一项）</span><Textarea aria-label="评测要求" value={requirement} onChange={(e) => setRequirement(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
  <div className="flex flex-wrap gap-2"><Button disabled={busy || !summary.trim() || !requirement.trim() || !prompt.trim()} onClick={() => void create()}>创建独立评测</Button><Button variant="outline" disabled={busy} onClick={() => void load().catch((err: Error) => setError(err.message))}>刷新版本</Button></div>
  <p className="text-xs leading-relaxed text-muted-foreground">运行统计包含不同任务，只提供原始记录；不代表相同条件下的能力提升。</p>
  <AgentSelectionsPanel agent={agent} summary={summary} config={{system_prompt:prompt,model_provider:provider,model_name:model,custom_args:agent.custom_args??[],...(skills.length?{skills}:{})}} onChange={load}/>
  {items.map((item) => <div key={item.id} className="space-y-2 border-t border-border pt-2" data-revision-id={item.id}>
   <p>{item.config.skills?.map((skill) => `${skill.name}@${skill.sha256?.slice(0,12)}`).join(' · ')}</p>
   <p>{item.summary || '运行配置快照'} · <code>{item.config_hash.slice(0, 12)}</code></p>
   <p>运行 {item.measurements.runs} · 完成 {item.measurements.completed} · 平均执行 {item.measurements.mean_seconds?.toFixed(1) ?? '—'} 秒 · 输入／输出 Token {item.measurements.input_tokens ?? '—'}／{item.measurements.output_tokens ?? '—'}</p>
   <p>提交 {item.deliveries.submissions} · 验收通过 {item.deliveries.accepted} · 退回 {item.deliveries.rejected} · 转交人工 {item.deliveries.needs_human}</p>
   {item.evaluations.map((task) => <p key={task.task_id}>评测任务：{task.title}（{task.status}）</p>)}
   <Button size="sm" disabled={busy || (!item.published && !item.accepted_selection)} onClick={() => void publish(item)}>{item.published ? '全局恢复此版本' : '全局发布通过评测的版本'}</Button>
  </div>)}
 </div>;
}


export function AgentRevisionsPanel({ agent }: { agent: Agent }) {
 const { user } = useAuth();
 const [items, setItems] = useState<Revision[]>([]);
 const [manualOpen, setManualOpen] = useState(false);
 const [error, setError] = useState('');
 const allowed = user?.id === agent.owner_id;
 const load = useCallback(async () => {
  setError('');
  try { setItems(await apiClient.get<Revision[]>(`/api/v1/agents/${agent.id}/revisions`)); }
  catch (err) { setError(err instanceof Error ? err.message : '读取改进历史失败'); }
 }, [agent.id]);
 if (!allowed) return null;
 return <>
  <AgentImprovementProposals key={agent.id} agent={agent} />
  <details className={detailSectionClass("min-w-0 space-y-3 break-words text-sm leading-relaxed")} aria-label="改进历史" onToggle={(event) => { if (event.currentTarget.open) void load(); }}>
   <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>改进历史</span></summary>
   {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
   {items.length === 0 && !error && <p className="text-muted-foreground">暂无改进记录。</p>}
   {items.map((item) => <details key={item.id} data-revision-id={item.id} className="space-y-2 border-t border-border pt-3">
    <summary className="cursor-pointer select-none">{item.summary || '工作配置记录'}</summary>
    <p>{item.published ? '曾用于正式配置' : item.accepted_selection ? '对照已获采用' : '配置记录或待验证提案'}</p>
    <p className="text-xs text-muted-foreground">运行 {item.measurements.runs} 次 · 完成 {item.measurements.completed} 次 · 提交 {item.deliveries.submissions} 次 · 通过 {item.deliveries.accepted} 次 · 退回 {item.deliveries.rejected} 次</p>
    {item.config.skills?.map((skill) => <p key={skill.name} className="text-xs break-all">{skill.name} · {skill.sha256?.slice(0, 12)}</p>)}
    <p className="text-xs text-muted-foreground">版本 {item.config_hash.slice(0, 12)}</p>
   </details>)}
  </details>
  <details className={detailSectionClass("space-y-3 text-sm")} aria-label="高级">
   <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>高级</span></summary>
   <details className="space-y-3" onToggle={(event) => setManualOpen(event.currentTarget.open)}>
    <summary className="cursor-pointer select-none font-bold">手动改进</summary>
    {manualOpen && <ManualAgentRevisionsPanel key={agent.id} agent={agent} />}
   </details>
  </details>
 </>;
}
