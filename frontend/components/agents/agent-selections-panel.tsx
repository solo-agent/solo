'use client';

import { BrutalAlert } from '@/components/ui/brutal-alert';
import { detailSectionClass, detailSectionTitleClass } from '@/components/ui/detail-section';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Select } from '@/components/ui/select';
import { Input } from '@/components/ui/input';

import { useId, useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { Agent, SkillBundle } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { TaskDeliveryDialog } from '@/components/tasks/task-delivery-dialog';

export type Selection = {
 id: string; baseline_revision_id: string; candidate_revision_id: string; status: string; ready_to_apply?: boolean;
 plan: { channel_id?: string; problem: string; change: string; team_check: string; token_budget: number };
 results: {
  tasks: { arm: string; case_index: number; task_id: string; channel_id: string; title: string; status: string; verified: boolean; last_error: string; attempts: number }[];
  trials: { arm: string; actual_tokens: number; accounted_tokens: number; token_budget: number; runs: number; active_runs: number; unknown_runs: number; execution_seconds: number; config_valid: boolean }[];
 };
 decisions: { id: string; decision: string; reason: string }[];
};
const armName: Record<string,string> = { baseline:'基线',candidate:'候选',receiver:'接收方',reviewer:'独立审核' };
const decisionName: Record<string,string> = { running:'执行中',accepted:'接受',rejected:'拒绝',observe:'继续观察' };

export function AgentSelectionsPanel({ agent, config, summary, onChange }: {
 agent: Agent;
 config: { system_prompt: string; model_provider: string; model_name: string; custom_args: string[]; skills?: SkillBundle[] };
 summary: string;
 onChange: () => Promise<void>;
}) {
  const fieldId = useId();
 const [items,setItems]=useState<Selection[]>([]);
 const [receivers,setReceivers]=useState<Agent[]>([]);
 const [receiver,setReceiver]=useState('');
 const [problem,setProblem]=useState('');
 const [teamCheck,setTeamCheck]=useState('');
 const [budget,setBudget]=useState(500000);
 const [cases,setCases]=useState([{title:'回归样例 1',input:'',requirements:''}]);
 const [reason,setReason]=useState('');
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const [notice,setNotice]=useState('');
 const [reviewTask,setReviewTask]=useState<string|null>(null);
 const creationKey=useRef<{ request: string; key: string }|null>(null);
 const load=useCallback(async()=>setItems(await apiClient.get<Selection[]>(`/api/v1/agents/${agent.id}/selections`)),[agent.id]);
 useEffect(()=>{void load().catch((err:Error)=>setError(err.message));void apiClient.get<Agent[]>('/api/v1/agents?owned=true').then((list)=>setReceivers(list.filter((item)=>item.id!==agent.id&&!item.name.startsWith('selection-')))).catch((err:Error)=>setError(err.message));},[agent.id,load]);
 const act=async(operation:()=>Promise<void>)=>{setBusy(true);setError('');setNotice('');try{await operation();await load();await onChange();}catch(err){setError(err instanceof Error?err.message:'操作失败');}finally{setBusy(false);}};
 const create=()=>act(async()=>{
  const revision=await apiClient.post<{id:string}>(`/api/v1/agents/${agent.id}/revisions`,{config,summary});
  const plan={candidate_revision_id:revision.id,receiver_agent_id:receiver,problem,change:summary,team_check:teamCheck,token_budget:budget,cases:cases.map((item)=>({title:item.title,input:item.input,requirements:item.requirements.split('\n').filter((text)=>text.trim()).map((text,i)=>({id:`R${i+1}`,text}))}))};
  const request=JSON.stringify(plan);
  if(creationKey.current?.request!==request)creationKey.current={request,key:crypto.randomUUID()};
  await apiClient.post(`/api/v1/agents/${agent.id}/selections`,{...plan,idempotency_key:creationKey.current.key});
  setNotice('已固定对照计划。基线、候选与接收方将在独立工作目录中执行。');
 });
 const decide=(item:Selection,decision:string)=>act(async()=>{await apiClient.post(`/api/v1/agents/${agent.id}/selections/${item.id}/decide`,{decision,reason});setNotice('已保留选择及依据。接受后可单独发布。');});
 const publish=(item:Selection)=>act(async()=>{await apiClient.post(`/api/v1/agents/${agent.id}/revisions/${item.candidate_revision_id}/publish`,{reason});setNotice('已发布经对照与接收检查的候选版本。');});
 return <details className={detailSectionClass("min-w-0 space-y-3 break-words text-sm leading-relaxed")} aria-label="新旧版本对照">
  <summary className="cursor-pointer select-none text-foreground"><span className={detailSectionTitleClass()}>新旧版本对照</span></summary>
  <p className="text-muted-foreground">使用上方候选配置和改进说明，双方固定相同模型、输入、验收要求与 Token 额度；接收方另做实际交接检查。</p>
  {error&&<BrutalAlert variant="error">{error}</BrutalAlert>}{notice&&<BrutalAlert variant="success">{notice}</BrutalAlert>}
  <Label className="block space-y-2"><span className="block">问题与工作引用</span><Textarea aria-label="问题与工作引用" value={problem} onChange={(e)=>setProblem(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
  <div className="space-y-2"><Label htmlFor={`${fieldId}-field-1`} className="block">接收成员</Label><Select id={`${fieldId}-field-1`} aria-label="接收成员" value={receiver} onChange={(value) => setReceiver(value)} size="md" className="w-full min-w-0" options={[{ value: "", label: "选择自己的另一位成员" }, ...receivers.map((item)=>({ value: item.id, label: item.name }))]} /></div>
  <Label className="block space-y-2"><span className="block">接收检查</span><Textarea aria-label="接收检查" value={teamCheck} onChange={(e)=>setTeamCheck(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
  <Label className="block space-y-2"><span className="block">每侧 Token 额度</span><Input aria-label="每侧 Token 额度" type="number" min={1000} max={10000000} value={budget} onChange={(e)=>setBudget(Number(e.target.value))} className="font-body font-normal" /></Label>
  <p className="text-xs leading-relaxed text-muted-foreground">额度控制后续 Run 启动，单次运行可能超支；所有失败、返工和未知用量都会保留。接收方使用单独额度。</p>
  {cases.map((item,index)=><fieldset key={index} className="space-y-3 rounded-lg border border-border bg-brutal-primary-light/40 p-3"><legend className="px-1 font-heading text-xs font-bold">样例 {index+1}</legend>
   {(['title','input','requirements'] as const).map((field)=><Label key={field} className="block space-y-2"><span className="block">{({title:'样例名称',input:'固定输入',requirements:'验收要求（每行一项）'})[field]}</span><Textarea aria-label={`样例 ${index+1} ${field}`} value={item[field]} onChange={(e)=>setCases((previous)=>previous.map((value,i)=>i===index?{...value,[field]:e.target.value}:value))} className="min-h-24 resize-y font-body font-normal" /></Label>)}
   {cases.length>1&&<Button variant="outline" size="sm" onClick={()=>setCases((previous)=>previous.filter((_,i)=>i!==index))}>移除样例 {index+1}</Button>}
  </fieldset>)}
  <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy||cases.length>=10} onClick={()=>setCases((previous)=>[...previous,{title:`回归样例 ${previous.length+1}`,input:'',requirements:''}])}>加入样例</Button><Button disabled={busy||!summary.trim()||!receiver||!problem.trim()||!teamCheck.trim()||cases.some((item)=>!item.title.trim()||!item.input.trim()||!item.requirements.trim())} onClick={()=>void create()}>开始固定条件对照</Button><Button variant="outline" disabled={busy} onClick={()=>void act(async()=>{})}>刷新对照结果</Button></div>
  <Label className="block space-y-2"><span className="block">选择依据</span><Textarea aria-label="选择依据" value={reason} onChange={(e)=>setReason(e.target.value)} className="min-h-24 resize-y font-body font-normal" /></Label>
  {items.map((item)=><section key={item.id} data-selection-id={item.id} className="space-y-3 border-t border-border pt-3">
   <p className="font-bold">{item.plan.change} · {decisionName[item.status]}</p><p>{item.plan.problem}</p>
   <div className="overflow-x-auto rounded-lg border border-border"><table className="w-full min-w-[30rem] text-left text-xs leading-relaxed tabular-nums"><caption className="p-3 text-left font-heading font-bold text-muted-foreground">本次固定任务集的实际消耗</caption><thead className="bg-brutal-primary-light"><tr><th scope="col" className="whitespace-nowrap px-3 py-2 font-heading">版本</th><th scope="col" className="whitespace-nowrap px-3 py-2 font-heading">Run</th><th scope="col" className="whitespace-nowrap px-3 py-2 font-heading">Token</th><th scope="col" className="whitespace-nowrap px-3 py-2 font-heading">未知用量</th><th scope="col" className="whitespace-nowrap px-3 py-2 font-heading">执行秒数</th></tr></thead><tbody>{item.results.trials.map((trial)=><tr key={trial.arm} className="border-t border-border"><td className="px-3 py-2">{armName[trial.arm]}{!trial.config_valid?' · 配置已变化':''}</td><td className="px-3 py-2">{trial.runs}（活跃 {trial.active_runs}）</td><td className="px-3 py-2">{trial.actual_tokens} / {trial.token_budget}</td><td className="px-3 py-2">{trial.unknown_runs}</td><td className="px-3 py-2">{trial.execution_seconds.toFixed(1)}</td></tr>)}</tbody></table></div>
   {item.results.tasks.map((task)=><div key={task.task_id} className="flex flex-wrap items-center gap-2"><a className="underline" href={`/dashboard?channel=${task.channel_id}&view=task&task=${task.task_id}`}>{armName[task.arm]} · {task.title}</a><span>{task.status} · {task.attempts} 次执行{task.verified?' · 已验收':''}</span><Button size="sm" variant="outline" onClick={()=>setReviewTask(task.task_id)}>查看交付与验收</Button>{task.last_error&&<BrutalAlert variant="error" className="w-full">{task.last_error}</BrutalAlert>}</div>)}
   <div className="flex flex-wrap gap-2">{['accepted','rejected','observe'].map((decision)=><Button key={decision} size="sm" variant="outline" disabled={busy||!reason.trim()} onClick={()=>void decide(item,decision)}>{decisionName[decision]}</Button>)}<Button size="sm" disabled={busy||item.status!=='accepted'||!reason.trim()} onClick={()=>void publish(item)}>全局发布此候选</Button></div>
   {item.decisions.map((decision)=><p key={decision.id} className="text-xs leading-relaxed text-muted-foreground">{decisionName[decision.decision]}：{decision.reason}</p>)}
  </section>)}
  {reviewTask&&<TaskDeliveryDialog taskId={reviewTask} open onOpenChange={(open)=>{if(!open){setReviewTask(null);void load().catch((err:Error)=>setError(err.message));}}} onComplete={()=>{void load().catch((err:Error)=>setError(err.message));}}/>}
 </details>;
}
