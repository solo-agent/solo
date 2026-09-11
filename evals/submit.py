#!/usr/bin/env python3
"""Transport an Agent-written file through the real Solo CLI. This does not solve or grade tasks."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

channel, number, filename = sys.argv[1:4]
submitted=Path(filename)
source=submitted.resolve()
if not source.is_relative_to(Path.cwd().resolve()) or submitted.is_symlink():
    raise SystemExit('Submit only a file from your own working directory')
content=source.read_text()
if not content.strip() or len(content.encode())>262144: raise SystemExit('Invalid artifact size')
def cli(*args):
    p=subprocess.run(['solo',*args],capture_output=True,text=True,timeout=30)
    if p.returncode: raise RuntimeError(p.stdout+p.stderr)
    return p.stdout

task=json.loads(cli('task','get','-c',channel,'-n',number))
digest=hashlib.sha256(content.encode()).hexdigest()
body={'expected_task_version':task['version'],'idempotency_key':'eval-'+digest[:32],
      'artifact_version':digest,'handoff':{'summary':'EVAL_ARTIFACT '+source.name,'changes':'Submitted actual file content','risks':'Awaiting independent verification','next_steps':'Independent automatic grading'},
      'evidence':[{'id':'E1','description':'Actual file: '+source.name,'content':content}]}
payload=Path.cwd()/'eval-submission.json'; payload.write_text(json.dumps(body))
cli('task','submit','-c',channel,'-n',number,'--file',str(payload))
cli('message','send','--target',channel+':'+task['message_id'][:8],'-c','EVAL_ARTIFACT '+source.name+' SHA256 '+digest)
print('Submitted',digest)
