#!/usr/bin/env python3
"""Grade actual submitted source in a fresh, restricted Docker container."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import resource
import subprocess
import tempfile
import time
import uuid

IMAGE = os.environ.get('SOLO_EVAL_IMAGE', 'python:3.12-alpine@sha256:b64631e04e4920160c50fbe8d8df828f7f35f06f425cb44aa09bca53e708a35a')
WRAPPER = r'''
import contextlib,copy,io,json,sys,traceback
request=json.load(sys.stdin)
namespace={'__name__':'solution'}
outputs=[]
try:
 with contextlib.redirect_stdout(io.StringIO()),contextlib.redirect_stderr(io.StringIO()):
  exec(compile(request['source'],'solution.py','exec'),namespace)
  if 'test' in request:
   exec(compile(request['test'],'official_test.py','exec'),namespace)
   namespace['check'](namespace[request['entry_point']])
   outputs=[{'official_test':'passed'}]
  else:
   for check in request['inputs']:
    args=copy.deepcopy(check['args']); before=copy.deepcopy(args)
    try:
     value=namespace[request['entry_point']](*args)
     item={'value':value}
    except BaseException as error:
     item={'error':type(error).__name__}
    item['input_unchanged']=json.dumps(args,sort_keys=True,allow_nan=False)==json.dumps(before,sort_keys=True,allow_nan=False)
    outputs.append(item)
 print(json.dumps({'outputs':outputs},ensure_ascii=False,allow_nan=False))
except BaseException as error:
 print(json.dumps({'error':type(error).__name__,'detail':str(error)[:1500]}))
'''

def _limits():
    resource.setrlimit(resource.RLIMIT_FSIZE, (2 << 20, 2 << 20))


def execute(payload, timeout=15):
    name='solo-eval-grade-'+uuid.uuid4().hex
    command=['docker','run','--rm','-i','--name',name,'--network','none','--read-only',
             '--tmpfs','/tmp:rw,noexec,nosuid,size=32m','--memory','256m','--cpus','1',
             '--pids-limit','64','--cap-drop','ALL','--security-opt','no-new-privileges',
             '--user','65534:65534',IMAGE,'python3','-I','-c',WRAPPER]
    started=time.monotonic()
    with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
        try:
            result=subprocess.run(command,input=json.dumps(payload).encode(),stdout=stdout,stderr=stderr,
                                  timeout=timeout,preexec_fn=_limits)
            stdout.seek(0); raw=stdout.read(2 << 20).decode(errors='replace')
            stderr.seek(0); diagnostic=stderr.read(4000).decode(errors='replace')
            if result.returncode in (125,126,127):
                return {'status':'infra_error','detail':diagnostic,'seconds':time.monotonic()-started}
            if result.returncode:
                return {'status':'failed','detail':f'execution exit {result.returncode}: {diagnostic}', 'seconds':time.monotonic()-started}
            try: data=json.loads(raw)
            except json.JSONDecodeError: return {'status':'failed','detail':'No valid grader result','seconds':time.monotonic()-started}
            return {'status':'completed','data':data,'seconds':time.monotonic()-started}
        except subprocess.TimeoutExpired:
            return {'status':'failed','detail':'execution timeout','seconds':time.monotonic()-started}
        finally:
            subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=20)


def equal_json(actual, expected):
    # Python's True == 1 is not equality of JSON values.
    if isinstance(actual,bool) or isinstance(expected,bool): return type(actual) is type(expected) and actual==expected
    if isinstance(actual,dict) and isinstance(expected,dict):
        return actual.keys()==expected.keys() and all(equal_json(actual[k],expected[k]) for k in expected)
    if isinstance(actual,list) and isinstance(expected,list):
        return len(actual)==len(expected) and all(equal_json(a,b) for a,b in zip(actual,expected))
    if isinstance(actual,(int,float)) and isinstance(expected,(int,float)):return actual==expected
    return type(actual) is type(expected) and actual==expected


def grade(case, source):
    if not isinstance(source,str) or not source.strip() or len(source.encode())>262144:
        return {'status':'failed','passed':False,'detail':'Missing or oversized source','checks':[]}
    if case.get('kind')=='learning':
        passed=100<=len(source)<=6000
        return {'status':'passed' if passed else 'failed','passed':passed,'checks':[{'name':'proposal format only; quality decided on frozen holdout','passed':passed}]}
    payload={'source':source,'entry_point':case['entry_point']}
    if 'test' in case: payload['test']=case['test']
    else: payload['inputs']=[{'args':check['args']} for check in case['checks']]
    execution=execute(payload)
    result={**execution,'source_sha256':hashlib.sha256(source.encode()).hexdigest(),'checks':[],'passed':False}
    if execution['status']!='completed': return result
    data=result.pop('data')
    if 'error' in data:
        result.update(status='failed',detail=data); return result
    actual=data.get('outputs',[])
    if 'test' in case:
        result['checks']=[{'name':'original upstream tests','passed':actual==[{'official_test':'passed'}]}]
    elif len(actual)!=len(case['checks']):
        result.update(status='failed',detail='Missing check outputs'); return result
    else:
        for i,(check,output) in enumerate(zip(case['checks'],actual)):
            value_ok=(output.get('error')==check['error']) if 'error' in check else ('value' in output and equal_json(output['value'],check['expected']))
            if 'key_order' in check:
                value_ok=value_ok and isinstance(output.get('value'),dict) and list(output['value'])==check['key_order']
            result['checks'].append({'name':f'case-{i+1}','passed':bool(value_ok and output.get('input_unchanged')),
                                     'actual':output,'expected':{k:v for k,v in check.items() if k!='args'}})
    result['passed']=bool(result['checks']) and all(c['passed'] for c in result['checks'])
    result['status']='passed' if result['passed'] else 'failed'
    return result


def main():
    parser=argparse.ArgumentParser(); parser.add_argument('dataset'); parser.add_argument('case_id'); parser.add_argument('source')
    args=parser.parse_args(); cases=json.loads(Path(args.dataset).read_text())
    case=next(c for c in cases if c['id']==args.case_id)
    print(json.dumps(grade(case,Path(args.source).read_text()),ensure_ascii=False))

if __name__=='__main__': main()
