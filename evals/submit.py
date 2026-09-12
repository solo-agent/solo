#!/usr/bin/env python3
"""Transport an Agent-written file through the real Solo CLI. This does not solve or grade tasks."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('channel', help='Task channel ID')
parser.add_argument('number', help='Task number in this channel')
parser.add_argument('filename', help='Actual artifact file inside your working directory')
parser.add_argument('--file', type=Path, help='Full submission JSON in your workspace, including additional verification evidence')
args = parser.parse_args()
channel, number, filename = args.channel, args.number, args.filename
def workspace_file(filename):
    submitted=Path(filename)
    source=submitted.resolve()
    if not source.is_relative_to(Path.cwd().resolve()) or submitted.is_symlink() or not source.is_file():
        raise SystemExit('Submit only a regular file from your own working directory')
    return source

source=workspace_file(filename)
payload=workspace_file(args.file) if args.file else Path.cwd()/'eval-submission.json'
content=source.read_bytes().decode('utf-8')
if not content.strip() or len(content.encode())>64000: raise SystemExit('Invalid artifact size')
def cli(*args):
    p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
    if p.returncode: raise RuntimeError(p.stdout+p.stderr)
    return p.stdout

task=json.loads(cli('task','get','-c',channel,'-n',number))
digest=hashlib.sha256(content.encode()).hexdigest()
if not args.file:
    body={'expected_task_version':task['version'],'idempotency_key':'eval-'+digest[:32],
          'artifact_version':digest,'handoff':{'summary':'EVAL_ARTIFACT '+source.name,'changes':'Submitted actual file content','risks':'Awaiting independent verification','next_steps':'Independent automatic grading'},
          'evidence':[{'id':'E1','description':'Actual file: '+source.name,'content':content}]}
    payload.write_text(json.dumps(body))
updated=json.loads(cli('task','submit','-c',channel,'-n',number,'--file',str(payload),'--artifact',str(source),'--evidence-id','E1'))
print(json.dumps({'task_id':updated['id'],'status':updated['status'],'submission_id':updated['current_submission_id'],'artifact_sha256':digest}),flush=True)
print(cli('message','send','--target',channel+':'+task['message_id'][:8],'-c','EVAL_ARTIFACT '+source.name+' SHA256 '+digest),end='')
