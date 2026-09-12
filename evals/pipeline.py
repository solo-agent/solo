#!/usr/bin/env python3
"""Run the whole autonomous evaluation cycle; resume only completed, frozen stages."""
import argparse
from datetime import datetime, timezone
import hashlib
from html import escape
import json
import os
from pathlib import Path
import subprocess
import sys
from uuid import uuid4
from evolve import decide

ROOT=Path(__file__).resolve().parent.parent
REGRESSIONS=['task-delivery-contract','agent-selection','task-wait-resume','team-work-compounding','thinking-mode']

def regression_outcome(stages):
    rows=[s for s in stages if s['name'].startswith('regression-')]
    expected={'regression-'+name for name in REGRESSIONS}
    return 'completed' if expected<={s['name'] for s in rows} and all(s['status']=='completed' for s in rows) else 'failed'

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--stack-root',type=Path,default=ROOT);p.add_argument('--output',type=Path,required=True)
    p.add_argument('--provider',choices=['claude','codex'],default='claude');p.add_argument('--model',default='sonnet');a=p.parse_args()
    out=a.output.resolve();out.mkdir(parents=True,exist_ok=True);stack=a.stack_root.resolve()
    frozen=[*sorted((ROOT/'evals').glob('*.py')),*sorted((ROOT/'evals/datasets').glob('*.json')),ROOT/'frontend/e2e/agent-evals.spec.ts']
    versions={str(f.relative_to(ROOT)):hashlib.sha256(f.read_bytes()).hexdigest() for f in frozen}
    plan={'provider':a.provider,'model':a.model,'versions':versions,'native_repetitions':3,'public_repetitions':1,'regressions':REGRESSIONS,
          'stack_root':str(stack),'token_qualification_limit':3000000}
    plan_path=out/'plan.json'
    if plan_path.exists() and json.loads(plan_path.read_text())!=plan: p.error('Frozen plan changed; use a new output directory, never mix versions')
    plan_path.write_text(json.dumps(plan,indent=2))
    state_path=out/'pipeline.json'
    state=json.loads(state_path.read_text()) if state_path.exists() else {'started_at':datetime.now(timezone.utc).isoformat(),'status':'running','stages':[]}
    def save():
        (out/'pipeline.json.tmp').write_text(json.dumps(state,ensure_ascii=False,indent=2));(out/'pipeline.json.tmp').replace(state_path)
        links=''
        for s in state['stages']:
            evidence=' · '.join(f"<a href='{escape(s['directory'])}/{filename}'>{label}</a>" for filename,label in [('index.html','报告'),('execution.log','日志')] if (out/s['directory']/filename).exists())
            links+=f"<li>{escape(s['name'])}：{escape(s['status'])} {evidence}</li>"
        decision_link='<a href="decision/decision.json">自动进化决策</a>' if (out/'decision/decision.json').exists() else '进化决策尚未生成'
        (out/'index.html').write_text(f'''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Solo 评测与进化</title><style>body{{font:16px/1.8 system-ui;max-width:1000px;margin:50px auto;padding:0 24px;background:#faf9f6;color:#222}}a{{color:#275d68}}li{{margin:10px 0}}pre{{white-space:pre-wrap}}</style><h1>Solo 评测与进化</h1><p>状态：{escape(state['status'])}。开发集对照 → 真实 Agent 提出方法 → 冻结验收集 → 自动采用/保留 → 开源补充 → 真实工作流回归。</p><p>原始试验和失败保留；此报告不会修改正式 Agent，也不代表官方排行榜成绩。</p><ul>{links}</ul><p>{decision_link} · <a href="plan.json">冻结计划</a> · <a href="pipeline.json">阶段状态</a></p></html>''')
    save()
    def stage(name, options=None):
        if any(hashlib.sha256((ROOT/f).read_bytes()).hexdigest()!=digest for f,digest in versions.items()):
            raise RuntimeError('Frozen evaluation source changed during execution')
        completed=next((s for s in state['stages'] if s['name']==name and s['status']=='completed'),None)
        if completed:return out/completed['directory']
        for s in state['stages']:
            if s['status']=='running':s['status']='interrupted'
        attempt=1+sum(s['name']==name for s in state['stages']);directory=out/f'{name}-{attempt}'
        record={'name':name,'directory':directory.name,'status':'running','started_at':datetime.now(timezone.utc).isoformat()};state['stages'].append(record);save()
        if options is not None:
            command=[sys.executable,str(ROOT/'evals/run.py'),'--stack-root',str(stack),'--output',str(directory),'--provider',a.provider,'--model',a.model,*options]
            result=subprocess.run(command,cwd=ROOT,env={**os.environ,'SOLO_EVAL_TOKEN_LIMIT':'3000000'})
        elif name=='graders':
            directory.mkdir()
            with (directory/'execution.log').open('w') as log:
                result=subprocess.run([sys.executable,'-m','unittest','discover','-s','evals','-p','test_evals.py','-v'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT)
            (directory/'index.html').write_text(f'<meta charset="utf-8"><h1>隔离判卷器与采用规则</h1><p>退出码：{result.returncode}</p><a href="execution.log">真实正反例检查日志</a>')
        else:
            directory.mkdir();spec=name.removeprefix('regression-')+'.spec.ts'
            command=['bash',str(stack/'scripts/run-local-e2e.sh'),name,'node',str(ROOT/'frontend/node_modules/@playwright/test/cli.js'),'test','--config',str(ROOT/'frontend/playwright.config.ts'),spec,'--reporter=line,json','--output',str(directory/'test-results')]
            env={**os.environ,'CI':'1','SOLO_E2E_PROVIDER':a.provider,'SOLO_E2E_MODEL':a.model,'PLAYWRIGHT_JSON_OUTPUT_NAME':str(directory/'playwright.json')}
            if name=='regression-thinking-mode':
                env.update(SOLO_E2E_EMAIL='thinking-eval-'+uuid4().hex+'@solo.local',SOLO_E2E_EXPECT_IDLE_REAPER='1',AGENT_SESSION_IDLE_TTL='20s',THINKING_SESSION_IDLE_TTL='20s',SESSION_IDLE_SWEEP_INTERVAL='1s')
            with (directory/'execution.log').open('w') as log:
                result=subprocess.run(command,cwd=stack,env=env,stdout=log,stderr=subprocess.STDOUT)
            try:
                stats=json.loads((directory/'playwright.json').read_text())['stats'];record['stats']=stats
                if stats['expected']<1 or stats['unexpected'] or stats['skipped']:record['validation_error']='Regression assertions failed, were missing, or skipped'
            except (OSError,ValueError,KeyError) as error:
                record['validation_error']='Missing or invalid regression report: '+str(error)
            (directory/'index.html').write_text(f'<meta charset="utf-8"><h1>{escape(name)}</h1><p>真实前端/API/PostgreSQL/本地 Runtime 回归：退出码 {result.returncode}</p><a href="playwright.json">完整断言报告</a> · <a href="execution.log">执行日志</a> · <a href="test-results/">截图与附件</a>')
        if (directory/'report.json').exists():
            trials=json.loads((directory/'report.json').read_text())['trials']
            record['known_actual_tokens']=sum(t['actual_tokens'] for t in trials if t.get('actual_tokens') is not None)
            record['unknown_usage_trials']=sum(t.get('actual_tokens') is None for t in trials)
        passed=result.returncode==0 and not record.get('validation_error')
        record.update(status='completed' if passed else ('failed' if name.startswith('regression-') and record.get('stats') else 'incomplete'),exit_code=result.returncode,completed_at=datetime.now(timezone.utc).isoformat());save()
        if not passed and not name.startswith('regression-'):raise RuntimeError(name+' failed; evidence retained in '+str(directory))
        return directory
    try:
        stage('graders')
        dev=stage('development',['--split','dev','--repetitions','3','--strategies','single,team'])
        learning=stage('learning',['--generate-from',str(dev/'report.json')])
        if not (learning/'candidate.md').exists():raise RuntimeError('Learning Agent did not deliver a valid candidate; baseline retained')
        holdout=stage('holdout',['--split','holdout','--repetitions','3','--strategies','team,evolved','--candidate',str(learning/'candidate.md')])
        if not (out/'decision/decision.json').exists():decide(holdout/'report.json',learning/'candidate.md',out/'decision')
        stage('humaneval',['--dataset','humaneval','--repetitions','1','--strategies','single'])
        for name in REGRESSIONS:stage('regression-'+name)
        state['status']=regression_outcome(state['stages']);state['regressions_finished']=True
        state['completed_at']=datetime.now(timezone.utc).isoformat();save();return 0 if state['status']=='completed' else 1
    except (Exception,KeyboardInterrupt) as error:
        for record in state['stages']:
            if record['status']=='running':record.update(status='incomplete',error=str(error))
        state.update(status='incomplete',error=str(error));save();print(str(error),file=sys.stderr);return 1

if __name__=='__main__':sys.exit(main())
