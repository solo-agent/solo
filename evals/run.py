#!/usr/bin/env python3
"""One entry point for real Solo evaluations. Reuses the make-managed E2E lifecycle."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
import sys
from evolve import learning_case
from report import render
from runtime import collect_runtime

ROOT=Path(__file__).resolve().parent.parent

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--stack-root',type=Path,default=ROOT)
    p.add_argument('--dataset',choices=['native','humaneval'],default='native')
    p.add_argument('--cases',default='')
    p.add_argument('--split',choices=['dev','holdout','public'])
    p.add_argument('--strategies',default='single,team')
    p.add_argument('--repetitions',type=int,default=3)
    p.add_argument('--model',default='sonnet')
    p.add_argument('--provider',choices=['claude','codex'],default='claude')
    p.add_argument('--candidate',type=Path)
    p.add_argument('--generate-from',type=Path,help='Generate a reusable method through a real Solo Task using only this development report')
    p.add_argument('--output',type=Path)
    p.add_argument('--timeout',type=int,default=360)
    args=p.parse_args()
    if not 1<=args.repetitions<=20: p.error('repetitions must be 1..20')
    if not 10<=args.timeout<=1800: p.error('timeout must be 10..1800 seconds')
    strategies=args.strategies.split(',')
    if len(strategies)!=len(set(strategies)) or not set(strategies)<=set(['single','team','evolved']): p.error('Use distinct single,team,evolved strategies')
    if 'evolved' in strategies and not args.candidate: p.error('evolved needs a frozen candidate file')
    stack=args.stack_root.resolve()
    # Do not use a running stack whose product source differs from these tests.
    product=['cmd','internal','pkg','migrations','frontend/components','frontend/lib']
    for relative in product:
        left={str(f.relative_to(ROOT/relative)):f.read_bytes() for f in (ROOT/relative).rglob('*') if f.is_file()}
        right={str(f.relative_to(stack/relative)):f.read_bytes() for f in (stack/relative).rglob('*') if f.is_file()}
        if left!=right: p.error('Product source differs from stack-root in '+relative)
    if not (stack/'.env').exists(): p.error('stack-root needs its existing .env and paired Computer')
    node=ROOT/'frontend/node_modules/@playwright/test/cli.js'
    if not node.exists(): p.error('Install existing frontend dependencies before running evals')
    dataset=ROOT/'evals/datasets'/('solo-skills-v2.json' if args.dataset=='native' else 'humaneval-smoke.json')
    if args.cases and not set(args.cases.split(','))<={c['id'] for c in json.loads(dataset.read_text())}: p.error('Unknown case ID')
    if args.split and not any(c['split']==args.split for c in json.loads(dataset.read_text())): p.error('Split not present in dataset')
    image='python:3.12-alpine@sha256:b64631e04e4920160c50fbe8d8df828f7f35f06f425cb44aa09bca53e708a35a'
    if subprocess.run(['docker','image','inspect',image],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:
        subprocess.run(['docker','pull',image],check=True)
    output=(args.output or ROOT/'evals/results'/datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')).resolve()
    if output.exists() and any(output.iterdir()): p.error('Use an empty output directory; old trials must never be overwritten')
    output.mkdir(parents=True,exist_ok=True)
    if args.generate_from:
        dataset=output/'learning-input.json'
        dataset.write_text(json.dumps(learning_case(args.generate_from.resolve(),ROOT/'evals/datasets/solo-skills-v2.json'),ensure_ascii=False,indent=2))
        args.strategies='single'; args.repetitions=1; args.cases=''; args.split=None
    env=os.environ.copy()
    env.update(CI='1',SOLO_EVAL_DATASET=str(dataset),SOLO_EVAL_OUTPUT=str(output),SOLO_EVAL_STRATEGIES=args.strategies,
               SOLO_EVAL_REPETITIONS=str(args.repetitions),SOLO_E2E_PROVIDER=args.provider,SOLO_E2E_MODEL=args.model,
               SOLO_EVAL_TRIAL_TIMEOUT=str(args.timeout*1000),SOLO_EVAL_IMAGE='python:3.12-alpine@sha256:b64631e04e4920160c50fbe8d8df828f7f35f06f425cb44aa09bca53e708a35a')
    for key in ['SOLO_EVAL_CASES','SOLO_EVAL_SPLIT','SOLO_EVAL_CANDIDATE']:
        env.pop(key,None)
    if args.cases: env['SOLO_EVAL_CASES']=args.cases
    if args.split: env['SOLO_EVAL_SPLIT']=args.split
    if args.candidate: env['SOLO_EVAL_CANDIDATE']=str(args.candidate.resolve())
    command=['bash',str(stack/'scripts/run-local-e2e.sh'),'agent-evals','node',str(node),'test','--config',str(ROOT/'frontend/playwright.config.ts'),'agent-evals.spec.ts','--reporter=line','--output',str(output/'test-results')]
    print('Output:',output,flush=True)
    with (output/'execution.log').open('w') as log:
        result=subprocess.run(command,cwd=stack,env=env,stdout=log,stderr=subprocess.STDOUT)
    print('Lifecycle exit:',result.returncode,flush=True)
    if (output/'report.json').exists():
        report=json.loads((output/'report.json').read_text())
        report['lifecycle_exit_code']=result.returncode
        if result.returncode:
            report['status']='incomplete'
            for trial in report['trials']:
                if trial['status']=='running': trial.update(status='interrupted',passed=False,error='Evaluation lifecycle interrupted; not an Agent capability score')
        try:
            runtime=collect_runtime(report)
            (output/'runtime.json').write_text(json.dumps(runtime,ensure_ascii=False,indent=2))
            report['runtime']={key:value for key,value in runtime.items() if key!='runs'}
        except (OSError,ValueError,subprocess.SubprocessError) as error:
            report['runtime']={'response_models':[],'reason':type(error).__name__+': runtime attribution unavailable'}
        (output/'report.json.tmp').write_text(json.dumps(report,ensure_ascii=False,indent=2))
        (output/'report.json.tmp').replace(output/'report.json')
        render(report,output)
        if args.generate_from and report['status']=='completed' and all(t['passed'] for t in report['trials']):
            (output/'candidate.md').write_bytes((output/report['trials'][0]['id']/'candidate.md').read_bytes())
        print(json.dumps({'status':report['status'],'trials':len(report['trials']),'passed':sum(t['passed'] for t in report['trials'])}))
    return result.returncode

if __name__=='__main__': sys.exit(main())
