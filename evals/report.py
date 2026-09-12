#!/usr/bin/env python3
"""Aggregate immutable real trials and render a standalone, dependency-free report."""
import argparse
from collections import defaultdict
from html import escape
import json
from pathlib import Path
import random
import re
from statistics import mean


def summarize(trials):
    groups=defaultdict(list)
    for t in trials: groups[t['strategy']].append(t)
    result={}
    for strategy, rows in groups.items():
        cases=defaultdict(list)
        for t in rows: cases[t['case_id']].append(t)
        known=[t['actual_tokens'] for t in rows if t.get('actual_tokens') is not None]
        result[strategy]={'trials':len(rows),'passed':sum(t['passed'] for t in rows),
                         'success_rate':mean(t['passed'] for t in rows),'artifact_passed':sum(bool(t.get('grade',{}).get('passed')) for t in rows),
                         'all_repeats_passed_cases':sum(all(t['passed'] for t in ts) for ts in cases.values()),
                         'cases':len(cases),'actual_tokens':sum(known) if len(known)==len(rows) else None,
                         'known_usage_trials':len(known),
                         'elapsed_seconds':sum(t.get('elapsed_seconds',0) for t in rows),
                         'infrastructure_errors':sum(t['status'] in ('infra_error','running','interrupted') for t in rows)}
    return result


def compare(trials, baseline='team', candidate='evolved', seed=4307):
    """Paired bootstrap over cases, keeping repeated trials in the same cluster."""
    base={(t['case_id'],t['repetition']):t for t in trials if t['strategy']==baseline}
    cand={(t['case_id'],t['repetition']):t for t in trials if t['strategy']==candidate}
    decision={'decision':'inconclusive','baseline':baseline,'candidate':candidate,
              'policy':'v1: complete matched trials; no case regression; positive 95% paired-case success CI and tokens <=1.25x, or equal success with >=20% token saving and positive 95% saving CI; otherwise keep baseline'}
    if not base or base.keys()!=cand.keys(): return {**decision,'reason':'Missing or unmatched paired trials'}
    if len(base)!=sum(t['strategy']==baseline for t in trials) or len(cand)!=sum(t['strategy']==candidate for t in trials):
        return {**decision,'reason':'Duplicate trial keys'}
    if any(t['status'] in ('running','infra_error','interrupted') or t.get('cleanup_errors') or t.get('actual_tokens') is None for t in [*base.values(),*cand.values()]):
        return {**decision,'reason':'Incomplete infrastructure or unknown usage'}
    grouped=defaultdict(list)
    for key in base: grouped[key[0]].append((base[key],cand[key]))
    if len(grouped)<10 or any(len(rows)<3 for rows in grouped.values()):
        return {**decision,'reason':'Need at least 10 cases with 3 paired repetitions'}
    case_delta=[mean(b['passed']-a['passed'] for a,b in rows) for rows in grouped.values()]
    cost=[(sum(a['actual_tokens'] for a,b in rows),sum(b['actual_tokens'] for a,b in rows)) for rows in grouped.values()]
    if not sum(a for a,b in cost): return {**decision,'reason':'Zero recorded baseline usage'}
    rng=random.Random(seed); gains=[]; savings=[]
    for _ in range(5000):
        picks=rng.choices(range(len(grouped)),k=len(grouped))
        gains.append(mean(case_delta[i] for i in picks))
        denominator=sum(cost[i][0] for i in picks)
        savings.append(1-sum(cost[i][1] for i in picks)/denominator if denominator else 0)
    ci=lambda xs:[sorted(xs)[124],sorted(xs)[4874]]
    gain=mean(case_delta); ratio=sum(b for a,b in cost)/sum(a for a,b in cost)
    decision.update(success_delta=gain,success_delta_95_ci=ci(gains),token_ratio=ratio,token_saving_95_ci=ci(savings),paired_cases=len(grouped),paired_trials=len(base))
    if min(case_delta)<0: return {**decision,'decision':'reject','reason':'At least one holdout case regressed'}
    if ratio>1.25: return {**decision,'decision':'reject','reason':'Candidate exceeds 1.25x token qualification budget'}
    if ci(gains)[0]>0 or (gain==0 and ratio<=.8 and ci(savings)[0]>0):
        return {**decision,'decision':'adopt','reason':'Frozen holdout passes predeclared quality/cost gates; scope is evaluation work method only'}
    return {**decision,'reason':'No reliable improvement under predeclared gates'}


def render(report, output):
    stats=summarize(report['trials']); report['summary']=stats
    if 'evolved' in stats:
        report['comparison']=compare(report['trials'])
        if report['status']!='completed':report['comparison'].update(decision='inconclusive',reason='Experiment infrastructure is incomplete')
    h=lambda value:escape(re.sub(r'\x1b\[[0-9;]*m','',str(value)))
    rows=''.join(f"<tr><td>{h(s)}</td><td>{v['artifact_passed']}/{v['trials']}</td><td>{v['passed']}/{v['trials']}</td><td>{v['success_rate']:.1%}</td><td>{v['all_repeats_passed_cases']}/{v['cases']}</td><td>{h(v['actual_tokens'] if v['actual_tokens'] is not None else '缺失')}</td><td>{v['elapsed_seconds']/60:.1f} 分钟</td></tr>" for s,v in stats.items())
    details=''
    for t in report['trials']:
        ident=h(t['id']); grade=t.get('grade') or {}
        failed_checks=[c['name'] for c in grade.get('checks',[]) if not c['passed']]
        messages=[t.get('error'),grade.get('detail'),'未通过独立判卷：'+', '.join(failed_checks) if failed_checks else None]
        err=h(' · '.join(str(message) for message in messages if message))
        names=[(t.get('artifact_name','solution.py'),'提交产物'),('attempted-'+t.get('artifact_name','solution.py'),'工作文件'),('submissions.json','提交记录'),('runs.json','Runs'),('events.json','事件'),('grading.json','判卷'),('product.png','页面')]
        links=' · '.join(f"<a href='{ident}/{h(name)}'>{label}</a>" for name,label in names if (output/t['id']/name).exists()) or '未产生证据文件'
        details+=f"<tr class=\"{'pass' if t['passed'] else 'fail'}\"><td>{h(t['case_id'])}</td><td>{h(t['strategy'])}</td><td>{t['repetition']+1}</td><td>{h(t['status'])}</td><td>{h(t.get('actual_tokens') if t.get('actual_tokens') is not None else '缺失')}</td><td>{links}</td><td>{err}</td></tr>"
    decision=report.get('comparison',{})
    decision_text={'adopt':'候选通过采用门槛，仅用于评测工作方法。','reject':'候选未通过采用门槛，保留基线。','inconclusive':'证据不足，保留基线。'}.get(decision.get('decision'),'本批次没有候选对照，不作采用判断。')
    decision_details='<details><summary>查看判定规则与数据</summary><pre>'+h(json.dumps(decision,ensure_ascii=False,indent=2))+'</pre></details>' if decision else ''
    runtime=report.get('runtime',{})
    models=', '.join(runtime.get('response_models',[])) or '未核实'
    thinking={'disabled':'请求关闭（兼容性试验）','unchanged':'保持现有设置'}.get(report.get('requested_thinking'),'未记录')
    runtime_link=' · <a href="runtime.json">型号归因证据</a>' if (output/'runtime.json').exists() else ''
    runtime_context='<p>Codex 运行时上下文（不等同于服务端响应型号）：'+h(', '.join(runtime.get('runtime_models', [])))+' · 推理档位：'+h(', '.join(runtime.get('runtime_efforts', [])))+' · 已核对 Runs：'+h(runtime.get('context_observed_runs', 0))+'/'+h(runtime.get('total_runs', 0))+'</p>' if runtime.get('runtime_models') else ''
    page=f'''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Solo 自动评测报告</title>
<style>body{{font:15px/1.6 system-ui,sans-serif;margin:40px auto;padding:0 24px;max-width:1300px;color:#222;background:#faf9f6}}h1{{font-size:28px}}table{{border-collapse:collapse;width:100%;background:white;margin:20px 0}}th,td{{padding:10px;text-align:left;border-bottom:1px solid #deddd8;vertical-align:top}}th{{background:#eeede8;white-space:nowrap}}a{{color:#275d68}}.fail td:nth-child(4){{color:#a03523}}.pass td:nth-child(4){{color:#28603a}}pre{{white-space:pre-wrap;overflow-wrap:anywhere}}.scroll{{overflow:auto}}select{{font:inherit;padding:6px}}small{{color:#666}}</style>
<h1>Solo 自动评测报告</h1><p>状态：<b>{h(report['status'])}</b> · Runtime：{h(report.get('provider'))} · 请求型号：{h(report.get('model'))} · {len(report['trials'])}/{report.get('planned_trials')} 次</p>
{runtime_context}
<p>响应型号：{h(models)} · 已核实 Run：{h(runtime.get('observed_runs','未知'))}/{h(runtime.get('total_runs','未知'))}{runtime_link}。响应标识不证明供应商内部权重固定；实际思考强度未核实。</p>
<p>思考输出设置：{h(thinking)} · 观测到思考响应的 Run：{h(runtime.get('observed_thinking_runs','未知'))}。请求设置不代表供应商已生效；未核实 Run 不计为关闭成功。</p>
<p>真实 Solo Task → 本地 Agent → 实际文件 → 独立容器判卷 → PostgreSQL 与前端成果核对。失败和缺失数据保留。</p>
<div class="scroll"><table><thead><tr><th>策略</th><th>独立判卷通过</th><th>可交付通过</th><th>单次成功率</th><th>全部重复通过的题</th><th>实际总 Token</th><th>总耗时</th></tr></thead><tbody>{rows}</tbody></table></div>
<p>所有成员、介绍、审核、失败与返工的用量均计入；Token 不是账单金额。重复全过比例是本次观察值，不能作为未来成功保证。人工介入次数：{h(report.get('human_interventions','未知'))}；真实用户节省分钟未测量。</p>
<h2>自动进化决策</h2><p>{h(decision_text)}</p>{decision_details}
<h2>逐项证据</h2><label>显示 <select id="filter"><option value="all">全部</option><option value="fail">仅失败</option><option value="pass">仅通过</option></select></label><div class="scroll"><table id="trials"><thead><tr><th>任务</th><th>策略</th><th>重复</th><th>结果</th><th>Token</th><th>证据</th><th>错误</th></tr></thead><tbody>{details}</tbody></table></div>
<p><a href="report.json">原始报告</a> · <a href="summary.json">汇总数据</a> · <a href="execution.log">执行日志</a></p><small>原创受控任务不代表生产整体能力；HumanEval 为固定公开子集及完整模块口径，不能冒充官方排行榜。候选只用于评测成员，未修改正式 Agent。</small>
<script>document.getElementById('filter').onchange=e=>document.querySelectorAll('#trials tbody tr').forEach(r=>r.hidden=e.target.value!=='all'&&!r.classList.contains(e.target.value));</script></html>'''
    output.mkdir(parents=True,exist_ok=True)
    (output/'index.html').write_text(page)
    (output/'summary.json').write_text(json.dumps({'status':report['status'],'summary':stats,'comparison':decision},ensure_ascii=False,indent=2))
    return stats

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('report',type=Path);args=p.parse_args()
    print(json.dumps(render(json.loads(args.report.read_text()),args.report.parent),ensure_ascii=False,indent=2))
