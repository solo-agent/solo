"""Original, deterministic Skill-maintenance cases. Oracles never enter Agent prompts."""
import json
from pathlib import Path

CASES = []

def case(name, split, entry, requirement, starter, reference, inputs, anchor):
    scope = {}
    exec(reference, scope)
    checks = []
    for args in inputs:
        try:
            checks.append({'args': args, 'expected': scope[entry](*args)})
            if name=='csv-money': checks[-1]['key_order']=list(checks[-1]['expected'])
        except Exception as error:
            checks.append({'args': args, 'error': type(error).__name__})
    assert checks[0].get('expected') == anchor, (name, checks[0], anchor)
    CASES.append({'id': name, 'suite': 'solo-skills-v3', 'split': split, 'entry_point': entry,
                  'instruction': requirement, 'starter': starter, 'reference': reference,
                  'checks': checks, 'source': 'Solo authored controlled maintenance task'})

case('csv-money', 'dev', 'totals',
     'Repair totals(text). Parse CSV columns account,amount using CSV quoting (commas and embedded newlines allowed). Sum exact decimal amounts by account, including refunds. Return a dict sorted by account with two-decimal string totals rounded HALF_UP only after summing. Header-only input returns {}. Do not use binary float for money.',
     'def totals(text):\n    return {line.split(",")[0]: line.split(",")[1] for line in text.splitlines()[1:]}\n',
     '''import csv,io
from decimal import Decimal,ROUND_HALF_UP
def totals(text):
 result={}
 for row in csv.DictReader(io.StringIO(text)):
  result[row['account']]=result.get(row['account'],Decimal(0))+Decimal(row['amount'])
 return {k:format(v.quantize(Decimal('.01'),rounding=ROUND_HALF_UP),'.2f') for k,v in sorted(result.items())}
''', [['account,amount\na,0.1\na,0.2\n'],['account,amount\n"a,b",2.005\n"a,b",-1\n'],['account,amount\n'],['account,amount\n"a\nb",1\nz,-0.005\n'],['account,amount\na,999999.995\na,-0.005\n'],['account,amount\nz,2\na,1\nz,-1\n']], {'a':'0.30'})

case('event-latest', 'dev', 'latest',
     'Repair latest(events). Each event has id,at (timezone-aware ISO8601),value. Return one original event per id, choosing the latest instant, with later input winning equal instants; sort output by id. Z and UTC offsets are supported. Any naive timestamp raises ValueError. Do not mutate inputs.',
     'def latest(events):\n    return list({e["id"]: e for e in events}.values())\n',
     '''from datetime import datetime
def latest(events):
 result={}
 for e in events:
  t=datetime.fromisoformat(e['at'].replace('Z','+00:00'))
  if t.tzinfo is None: raise ValueError('timezone required')
  if e['id'] not in result or t>=result[e['id']][0]: result[e['id']]=(t,e)
 return [result[k][1] for k in sorted(result)]
''', [[[]],[[{'id':'x','at':'2026-01-01T00:00:00Z','value':2},{'id':'x','at':'2025-12-31T23:00:00Z','value':1}]],[[{'id':'z','at':'2026-01-01T08:00:00+08:00','value':1},{'id':'z','at':'2026-01-01T00:00:00Z','value':2},{'id':'a','at':'2026-01-01T00:00:00-05:00','value':3}]],[[{'id':'x','at':'2026-01-01T00:00:00','value':1}]]], [])

case('dependency-order', 'dev', 'order',
     'Repair order(graph), mapping node -> prerequisite list. Include nodes referenced only as prerequisites. Return a topological order, choosing the lexicographically smallest currently available node at every step. Empty graph returns []. Duplicate edges are ignored; any cycle including a self-loop raises ValueError.',
     'def order(graph):\n    return sorted(graph)\n',
     '''import heapq
def order(graph):
 nodes=set(graph)|{d for ds in graph.values() for d in ds}
 deps={n:set(graph.get(n,[])) for n in nodes}
 ready=[n for n in nodes if not deps[n]]; heapq.heapify(ready); result=[]
 while ready:
  n=heapq.heappop(ready); result.append(n)
  for k in nodes:
   if n in deps[k]:
    deps[k].remove(n)
    if not deps[k]: heapq.heappush(ready,k)
 if len(result)!=len(nodes): raise ValueError('cycle')
 return result
''', [[{'deploy':['test'],'test':['build'],'build':[]}],[{'b':['a','a'],'c':[]}],[{}],[{'a':['b'],'b':['a']}],[{'a':['a']}],[{'z':['a'],'b':[]}]], ['build','test','deploy'])

case('nested-redaction', 'dev', 'redact',
     'Repair redact(value). Recursively traverse dicts/lists without changing the input. Replace the entire value of case-insensitive exact keys password,token,api_key with "[REDACTED]", regardless of value type. Preserve other keys, scalar types and strings, including text merely containing those words.',
     'def redact(value):\n    return {k: "[REDACTED]" if "token" in k else v for k,v in value.items()}\n',
     '''def redact(value):
 if isinstance(value,dict): return {k:'[REDACTED]' if k.lower() in {'password','token','api_key'} else redact(v) for k,v in value.items()}
 if isinstance(value,list): return [redact(v) for v in value]
 return value
''', [[{'PASSWORD':None,'token_count':12}],[[{'API_KEY':{'x':1}},{'nested':{'token':'a','ok':False}}]],['password token'],[None],[{'a':[],'b':[1,True,'x']}]], {'PASSWORD':'[REDACTED]','token_count':12})

case('retry-delay', 'dev', 'delay',
     'Repair delay(attempt,base,cap,retry_after). attempt is a nonnegative integer; base and cap are nonnegative numbers. Return min(cap, retry_after) when retry_after is supplied (including zero), otherwise min(cap, base * 2**attempt). Negative attempt/base/cap/retry_after raises ValueError. Large attempts such as 100000 must not overflow.',
     'def delay(attempt,base,cap,retry_after=None):\n    return min(cap,retry_after or base*2**attempt)\n',
     '''def delay(attempt,base,cap,retry_after=None):
 if attempt<0 or base<0 or cap<0 or (retry_after is not None and retry_after<0): raise ValueError('negative')
 if retry_after is not None: return min(cap,retry_after)
 value=base
 for _ in range(attempt):
  if value==0 or value>=cap: break
  value=min(cap,value*2)
 return min(cap,value)
''', [[2,3,20,None],[4,2,30,0],[100000,1,100,None],[100000,0,100,None],[-1,1,10,None],[1,1,10,-2],[0,3,0,None],[0,2.5,100,None],[100000,1.0,100.0,None],[0,-1,10,None],[0,1,-10,None]], 12)

case('markdown-section', 'dev', 'section',
     'Repair section(text,title). Find the first exact level-2 ATX heading "## title" outside fenced blocks. Return following lines through but excluding the next level-1 or level-2 heading, strip only surrounding blank lines, join with \n. Level-3 headings remain. Backtick and tilde fences of 3+ markers close only with the same marker and at least opening length. Missing title returns empty string.',
     'def section(text,title):\n    return text.split("## "+title+"\\n")[-1].split("## ")[0].strip()\n',
     '''import re
def section(text,title):
 active=False; out=[]; marker=''; length=0
 for line in text.splitlines():
  fence=re.match(r'^ {0,3}(`{3,}|~{3,})(.*)$',line)
  outside=not marker
  if fence:
   marks,tail=fence.groups()
   if not marker: marker=marks[0]; length=len(marks)
   elif marks[0]==marker and len(marks)>=length and not tail.strip(): marker=''
  h=re.match(r'^ {0,3}(#{1,6}) +(.+?) *#* *$',line) if outside and not fence else None
  if h and len(h[1])<=2:
   if active: break
   if h[1]=='##' and h[2]==title: active=True; continue
  if active: out.append(line)
 while out and not out[0].strip(): out.pop(0)
 while out and not out[-1].strip(): out.pop()
 return '\\n'.join(out)
''', [['## Plan\na\n### Detail\nb\n## End\nc','Plan'],['```\n## Plan\nfake\n```\n## Plan\nreal','Plan'],['## Other\nx','Plan'],['## Plan\n~~~~\n## Fake\n~~~\ninside\n~~~~\nlast\n# End','Plan']], 'a\n### Detail\nb')

case('cursor-pagination', 'dev', 'collect',
     'Repair collect(pages,start). pages maps cursor strings to {items:list,next:string|null}. Follow pages from start until next is null, concatenating items. Empty-string cursors are valid. Missing cursor raises KeyError; revisiting a cursor raises ValueError even when its items are empty. Input objects must remain unchanged.',
     'def collect(pages,start):\n    out=[]\n    while start:\n        page=pages[start]; out.extend(page["items"]); start=page["next"]\n    return out\n',
     '''def collect(pages,start):
 seen=set(); result=[]
 while start is not None:
  if start in seen: raise ValueError('cursor cycle')
  seen.add(start); p=pages[start]; result.extend(p['items']); start=p['next']
 return result
''', [[{'a':{'items':[1],'next':''},'':{'items':[2],'next':None}},'a'],[{'x':{'items':[],'next':'x'}},'x'],[{},'a'],[{},None],[{'a':{'items':[1,1],'next':None}},'a']], [1,2])

case('relative-path', 'dev', 'safe_path',
     'Repair safe_path(path). Normalize a nonempty slash-separated relative file path; collapse repeated slashes and ignore empty segments, collapse . and interior a/.., but reject any attempt to climb above the root, even if later segments return inside. Reject leading slash, backslash, NUL and colon anywhere. Reject paths normalizing to empty. Return normalized slash path; invalid paths raise ValueError. This is lexical validation, not a filesystem symlink resolver.',
     'def safe_path(path):\n    return path.replace("../", "").strip("/")\n',
     '''def safe_path(path):
 if not path or path.startswith('/') or any(c in path for c in ['\\\\','\\0',':']): raise ValueError('invalid')
 out=[]
 for part in path.split('/'):
  if part in ('','.'): continue
  if part=='..':
   if not out: raise ValueError('escape')
   out.pop()
  else: out.append(part)
 if not out: raise ValueError('empty')
 return '/'.join(out)
''', [['a/./b/../c'],['../a'],['a/../../b'],['/a'],['a\\b'],['C:a'],['a/..'],['a//中文.py'],['a\0b']], 'a/c')

case('duration-union', 'dev', 'covered',
     'Repair covered(intervals). Given numeric [start,end] intervals, return total length of their union. Overlap and touching boundaries count once; zero-length intervals contribute zero. Reject reversed intervals with ValueError. Do not reorder/mutate the input.',
     'def covered(intervals):\n    return sum(end-start for start,end in intervals)\n',
     '''def covered(intervals):
 if any(b<a for a,b in intervals): raise ValueError('reversed')
 end=None; total=0
 for a,b in sorted(intervals):
  if end is None or a>end: total+=b-a; end=b
  elif b>end: total+=b-end; end=b
 return total
''', [[[[1,4],[3,6],[6,8]]],[[]],[[[-3,-1],[-2,2],[5,5]]],[[[5,1]]],[[[1,10],[2,3],[1,10]]]], 7)

case('stable-dedupe', 'dev', 'dedupe',
     'Repair dedupe(records). Records have id,version,payload. Retain the record with greatest integer version per id; last input wins ties. Output ids in order of their first occurrence, not lexical order. Empty input returns []. Do not mutate input.',
     'def dedupe(records):\n    return list({r["id"]:r for r in records}.values())\n',
     '''def dedupe(records):
 out={}
 for r in records:
  if r['id'] not in out or r['version']>=out[r['id']]['version']: out[r['id']]=r
 return list(out.values())
''', [[[]],[[{'id':'b','version':2,'payload':'new'},{'id':'a','version':1,'payload':'a'},{'id':'b','version':1,'payload':'old'}]],[[{'id':'x','version':1,'payload':1},{'id':'x','version':1,'payload':2}]]], [])

case('allocate-cents', 'holdout', 'allocate',
     'Repair allocate(total,weights). Allocate nonnegative integer cents proportionally to a list of nonnegative integer weights using largest remainders, ties by original index. Sum must equal total. Empty weights allowed only when total=0; zero total with all-zero weights returns zero shares. Negative values or positive total with zero total weight raise ValueError. Use exact integer arithmetic, including large totals.',
     'def allocate(total,weights):\n    return [round(total*w/sum(weights)) for w in weights]\n',
     '''def allocate(total,weights):
 if total<0 or any(w<0 for w in weights): raise ValueError('negative')
 s=sum(weights)
 if not s:
  if total: raise ValueError('zero weights')
  return [0]*len(weights)
 parts=[total*w//s for w in weights]
 priority=sorted(range(len(weights)),key=lambda i:(-(total*weights[i]%s),i))
 for i in priority[:total-sum(parts)]: parts[i]+=1
 return parts
''', [[10,[1,1,1]],[1,[1,1,1]],[0,[0,0]],[1,[]],[0,[]],[-1,[1]],[10,[0,3,0,1]],[10000000000000003,[1,1,1]]], [4,3,3])

case('dependency-closure', 'holdout', 'closure',
     'Repair closure(graph,target). Return sorted unique transitive prerequisites of target, excluding target. Missing nodes are leaves. A cycle reachable from target raises ValueError; unrelated cycles must not affect the result. A shared dependency is not a cycle.',
     'def closure(graph,target):\n    return sorted(graph.get(target,[]))\n',
     '''def closure(graph,target):
 seen=set(); active=set()
 def visit(n):
  if n in active: raise ValueError('cycle')
  if n in seen: return
  active.add(n)
  for child in graph.get(n,[]): visit(child)
  active.remove(n); seen.add(n)
 visit(target); seen.remove(target); return sorted(seen)
''', [[{'a':['b','c'],'b':['d'],'c':['d']},'a'],[{'x':['x']},'a'],[{'a':['b'],'b':['a']},'a'],[{},'z'],[{'a':['a']},'a']], ['b','c','d'])

case('csv-export', 'holdout', 'export_csv',
     'Repair export_csv(rows,columns). Return CSV text with columns as header, preserving column and row order. Terminate every record with \n, including the header and the final record; use standard double-quote escaping. Missing or null cells become empty strings; other values use str(). Commas, quotes and embedded newlines must round-trip. Always include the header, even for no rows.',
     'def export_csv(rows,columns):\n    return ",".join(columns)+"\\n"+"\\n".join(",".join(str(r.get(c,"")) for c in columns) for r in rows)\n',
     '''import csv,io
def export_csv(rows,columns):
 f=io.StringIO(newline=''); w=csv.writer(f,lineterminator='\\n'); w.writerow(columns)
 for row in rows: w.writerow(['' if row.get(c) is None else str(row[c]) for c in columns])
 return f.getvalue()
''', [[[{'name':'a,b','n':2}],['name','n']],[[],['a','b']],[[{'x':'line\n"two"','y':None}],['x','y']],[[{'b':0,'a':False}],['a','b','c']]], 'name,n\n"a,b",2\n')

case('canonical-json', 'holdout', 'canonical',
     'Repair canonical(value,ignored). Recursively remove dict keys exactly present in ignored, including dictionaries inside lists. Return compact JSON with dictionary keys sorted, Unicode unescaped, list order and scalar types preserved. The input must not be mutated. Inputs contain only finite JSON numbers.',
     'import json\ndef canonical(value,ignored):\n    return json.dumps({k:v for k,v in value.items() if k not in ignored},sort_keys=True)\n',
     '''import json
def canonical(value,ignored):
 def clean(x):
  if isinstance(x,dict): return {k:clean(v) for k,v in x.items() if k not in ignored}
  if isinstance(x,list): return [clean(v) for v in x]
  return x
 return json.dumps(clean(value),sort_keys=True,ensure_ascii=False,separators=(',',':'),allow_nan=False)
''', [[{'z':1,'a':{'time':2,'x':'中'}},['time']],[[{'time':2,'x':None},False,0],['time']],[None,[]],[{'foo':[]},[]]], '{"a":{"x":"中"},"z":1}')

case('half-open-overlap', 'holdout', 'conflicts',
     'Repair conflicts(bookings). Each booking is {id,start,end} with numeric times and unique id. Return sorted [smaller_id,larger_id] pairs whose half-open intervals overlap. Touching endpoints and zero-duration bookings do not conflict. Reversed intervals raise ValueError. Input order does not matter.',
     'def conflicts(bookings):\n    return [[a["id"],b["id"]] for i,a in enumerate(bookings) for b in bookings[i+1:] if a["start"]<=b["end"] and b["start"]<=a["end"]]\n',
     '''def conflicts(bookings):
 if any(b['end']<b['start'] for b in bookings): raise ValueError('reversed')
 return sorted(sorted([a['id'],b['id']]) for i,a in enumerate(bookings) for b in bookings[i+1:] if max(a['start'],b['start'])<min(a['end'],b['end']))
''', [[[{'id':'z','start':1,'end':4},{'id':'a','start':3,'end':5},{'id':'b','start':5,'end':7}]],[[{'id':'a','start':0,'end':10},{'id':'b','start':5,'end':5}]],[[]],[[{'id':'x','start':5,'end':1}]]], [['a','z']])

case('utc-elapsed', 'holdout', 'elapsed',
     'Repair elapsed(start,end). Inputs are timezone-aware ISO8601 strings, supporting Z and offsets. Return elapsed seconds between their actual instants (fractional seconds allowed). Reversed instants or either naive timestamp raise ValueError. Equal instants return 0.',
     'from datetime import datetime\ndef elapsed(start,end):\n    return (datetime.fromisoformat(end).replace(tzinfo=None)-datetime.fromisoformat(start).replace(tzinfo=None)).total_seconds()\n',
     '''from datetime import datetime
def elapsed(start,end):
 a=datetime.fromisoformat(start.replace('Z','+00:00')); b=datetime.fromisoformat(end.replace('Z','+00:00'))
 if a.tzinfo is None or b.tzinfo is None: raise ValueError('timezone')
 seconds=(b-a).total_seconds()
 if seconds<0: raise ValueError('reversed')
 return seconds
''', [['2026-01-01T08:00:00+08:00','2026-01-01T00:01:30Z'],['2026-01-01T00:00:00.250Z','2026-01-01T00:00:00.750Z'],['2026-01-01T08:00:00+08:00','2026-01-01T00:00:00Z'],['2026-01-02T00:00:00Z','2026-01-01T00:00:00Z'],['2026-01-01T00:00:00','2026-01-01T00:00:00Z']], 90)

case('longest-route', 'holdout', 'route',
     'Repair route(path,routes). Routes map slash paths to arbitrary values (including false and null). Select the longest matching prefix at a segment boundary: /api matches /api and /api/x, not /apix. / is a fallback for any absolute path. Return {matched:prefix,value:...}, or null if no match. Ignore query and fragment in path. Nonabsolute path raises ValueError.',
     'def route(path,routes):\n    return next((v for k,v in routes.items() if path.startswith(k)),None)\n',
     '''def route(path,routes):
 path=path.split('?',1)[0].split('#',1)[0]
 if not path.startswith('/'): raise ValueError('absolute path')
 matches=[k for k in routes if k=='/' or path==k or path.startswith(k.rstrip('/')+'/')]
 if not matches: return None
 key=max(matches,key=len); return {'matched':key,'value':routes[key]}
''', [['/api/v1/x',{'/':'root','/api':'api','/api/v1':'v1'}],['/apix',{'/api':1}],['/api?x=1#z',{'/api':False}],['/',{'/':None}],['relative',{'/':1}]], {'matched':'/api/v1','value':'v1'})

case('frontmatter', 'holdout', 'frontmatter',
     'Repair frontmatter(text). Return {header,body}. Recognize a header only when the very first line (after optional UTF-8 BOM) is exactly ---; close on the next line exactly ---. Keep all content as strings, normalize line endings to \n, remove delimiter lines, and preserve trailing newlines in header/body. Without an opening delimiter return empty header and normalized text (without BOM) as body. An unclosed opening delimiter raises ValueError.',
     'def frontmatter(text):\n    parts=text.split("---")\n    return {"header":parts[1],"body":parts[2]}\n',
     '''def frontmatter(text):
 text=text.removeprefix('\\ufeff').replace('\\r\\n','\\n').replace('\\r','\\n')
 lines=text.splitlines(keepends=True)
 if not lines or lines[0].rstrip('\\n')!='---': return {'header':'','body':text}
 for i in range(1,len(lines)):
  if lines[i].rstrip('\\n')=='---': return {'header':''.join(lines[1:i]),'body':''.join(lines[i+1:])}
 raise ValueError('unclosed')
''', [['---\nname: a\n---\nbody\n'],['text\n---\nordinary'],['\ufeff---\r\nname: a\r\n---\r\nx'],['---\nx'],[''],['---\n---\n']], {'header':'name: a\n','body':'body\n'})

case('batch-boundaries', 'holdout', 'batches',
     'Repair batches(items,max_items,max_bytes). items are strings. Preserve order and duplicates, returning batches where count<=max_items and summed UTF-8 byte lengths<=max_bytes (no separator cost). Oversized single item raises ValueError; max_items<=0 or max_bytes<0 raises ValueError. Empty strings cost zero bytes. Empty input returns []. Do not emit empty batches.',
     'def batches(items,max_items,max_bytes):\n    return [items[i:i+max_items] for i in range(0,len(items),max_items)]\n',
     '''def batches(items,max_items,max_bytes):
 if max_items<=0 or max_bytes<0: raise ValueError('limits')
 result=[]; group=[]; size=0
 for item in items:
  n=len(item.encode('utf-8'))
  if n>max_bytes: raise ValueError('oversized')
  if group and (len(group)>=max_items or size+n>max_bytes): result.append(group); group=[]; size=0
  group.append(item); size+=n
 if group: result.append(group)
 return result
''', [[['中','a','文','b'],3,4],[['','',''],2,0],[[],2,0],[['中'],5,2],[['a'],0,10],[['a'],1,-1],[['a','a','a'],2,2]], [['中','a'],['文','b']])

case('patch-config', 'holdout', 'merge',
     'Repair merge(base,patch) using JSON Merge Patch semantics: if patch is not an object, replace base entirely. For object patches, treat nonobject base as {}, recursively merge object members, delete members whose patch value is null, and replace arrays/scalars. Return a new result without mutating either argument. A missing null-valued key remains absent.',
     'def merge(base,patch):\n    return dict(base,**patch)\n',
     '''import copy
def merge(base,patch):
 if not isinstance(patch,dict): return copy.deepcopy(patch)
 out=copy.deepcopy(base) if isinstance(base,dict) else {}
 for k,v in patch.items():
  if v is None: out.pop(k,None)
  else: out[k]=merge(out.get(k),v)
 return out
''', [[{'a':{'x':1,'y':2},'b':[1]}, {'a':{'x':None,'z':3},'b':[2]}],[{'a':1},None],[None,{'a':{'b':1},'gone':None}],[{'a':1},[]],[{},{}]], {'a':{'y':2,'z':3},'b':[2]})

if __name__ == '__main__':
    target=Path(__file__).with_name('datasets')/'solo-skills-v3.json'
    target.parent.mkdir(exist_ok=True)
    target.write_text(json.dumps(CASES,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'cases':len(CASES),'checks':sum(len(c['checks']) for c in CASES),'path':str(target)}))
