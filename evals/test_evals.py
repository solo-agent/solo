import json
import subprocess
import sys
from pathlib import Path
import unittest
from grade import execute, grade

class Graders(unittest.TestCase):
    def test_v2_only_clarifies_repeated_slashes_without_changing_oracles(self):
        directory=Path(__file__).with_name('datasets')
        previous=json.loads((directory/'solo-skills-v1.json').read_text())
        current=json.loads((directory/'solo-skills-v2.json').read_text())
        self.assertEqual(len(previous),len(current))
        for old,new in zip(previous,current):
            self.assertEqual(new.pop('suite'),'solo-skills-v2')
            self.assertEqual(old.pop('suite'),'solo-skills-v1')
            if old['id']=='relative-path':
                self.assertEqual(new['instruction'],old['instruction'].replace('collapse .','collapse repeated slashes and ignore empty segments, collapse .'))
                new['instruction']=old['instruction']
            self.assertEqual(old,new)

    def test_v3_only_clarifies_csv_terminators_without_changing_oracles(self):
        directory=Path(__file__).with_name('datasets')
        previous=json.loads((directory/'solo-skills-v2.json').read_text())
        current=json.loads((directory/'solo-skills-v3.json').read_text())
        self.assertEqual(len(previous),len(current))
        for old,new in zip(previous,current):
            self.assertEqual(new.pop('suite'),'solo-skills-v3')
            self.assertEqual(old.pop('suite'),'solo-skills-v2')
            if old['id']=='csv-export':
                self.assertEqual(new['instruction'],old['instruction'].replace('Use \n record terminators and standard double-quote escaping.','Terminate every record with \n, including the header and the final record; use standard double-quote escaping.'))
                new['instruction']=old['instruction']
            self.assertEqual(old,new)

    def test_dataset_references_and_negative_controls(self):
        cases=json.loads(Path(__file__).with_name('datasets').joinpath('solo-skills-v3.json').read_text())
        self.assertEqual(len(cases),20)
        self.assertEqual(len({c['id'] for c in cases}),20)
        self.assertEqual(sum(c['split']=='holdout' for c in cases),10)
        for case in cases:
            with self.subTest(case=case['id']):
                self.assertTrue(grade(case,case['reference'])['passed'])
                self.assertFalse(grade(case,case['starter'])['passed'])
                self.assertFalse(grade(case,f"def {case['entry_point']}(*args): return None")['passed'])
    def test_execution_isolation_and_failures(self):
        payload={'entry_point':'probe','inputs':[{'args':[]}]}
        result=execute({**payload,'source':"def probe():\n open('/outside','w').write('x')"})
        self.assertEqual(result['data']['outputs'][0]['error'],'OSError')
        result=execute({**payload,'source':'def probe():\n while True: pass'},timeout=3)
        self.assertEqual(result['status'],'failed')
        self.assertIn('timeout',result['detail'])
        result=execute({**payload,'source':'import os\nos._exit(0)'})
        self.assertEqual(result['status'],'failed')
    def test_json_types_and_required_order(self):
        from grade import equal_json
        self.assertFalse(equal_json({'x':[True]}, {'x':[1]}))
        self.assertTrue(equal_json({'x':[1.0]}, {'x':[1]}))
        case={'entry_point':'f','checks':[{'args':[],'expected':{'a':1,'z':2},'key_order':['a','z']}]}
        self.assertFalse(grade(case,"def f(): return {'z':2,'a':1}")['passed'])
        self.assertTrue(grade(case,"def f(): return {'a':1,'z':2}")['passed'])
    def test_mutation_cannot_pass(self):
        case={'entry_point':'f','checks':[{'args':[[1,2]],'expected':3}]}
        self.assertFalse(grade(case,'def f(xs):\n total=sum(xs); xs.clear(); return total')['passed'])
        self.assertFalse(grade({'entry_point':'f','checks':[{'args':[[True]],'expected':True}]},'def f(xs):\n xs[0]=1; return True')['passed'])


class Decisions(unittest.TestCase):
    def test_submit_rejects_nonlocal_or_nonregular_handoff_before_runtime(self):
        import os
        import tempfile
        script = Path(__file__).with_name('submit.py')
        with tempfile.TemporaryDirectory() as workspace, tempfile.TemporaryDirectory() as outside:
            work = Path(workspace)
            (work / 'solution.py').write_text('def f(): return 1\n')
            external = Path(outside) / 'handoff.json'
            external.write_text('{}')
            (work / 'link.json').symlink_to(external)
            os.mkfifo(work / 'pipe.json')
            for path in [external, work / 'link.json', work, work / 'pipe.json']:
                with self.subTest(path=path.name):
                    result = subprocess.run([sys.executable, str(script), 'channel', '1', 'solution.py', '--file', str(path)], cwd=work, capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertIn('regular file from your own working directory', result.stderr)
                    self.assertNotIn('Traceback', result.stderr)

    def test_submit_help_and_missing_arguments_need_no_runtime(self):
        script = Path(__file__).with_name('submit.py')
        for args, code in [(['--help'], 0), ([], 2)]:
            with self.subTest(args=args):
                result = subprocess.run([sys.executable, str(script), *args], capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, code, result.stderr)
                self.assertIn('channel number filename', result.stdout + result.stderr)
                self.assertNotIn('Traceback', result.stderr)

    def test_runtime_models_use_only_actual_responses_in_the_current_run(self):
        from runtime import response_models
        entries = json.loads('''[
          {"type":"assistant","timestamp":"2026-09-12T01:00:00Z","message":{"model":"old-session-model"}},
          {"type":"user","timestamp":"2026-09-12T01:02:00Z","message":{"model":"sonnet"}},
          {"type":"assistant","timestamp":"2026-09-12T01:02:00Z","message":{"model":"MiniMax-M3"}},
          {"type":"assistant","timestamp":"2026-09-12T01:02:01Z","message":{"model":"MiniMax-M3"}},
          {"type":"assistant","timestamp":"2026-09-12T01:02:02Z","message":{"model":"<synthetic>"}},
          {"type":"assistant","timestamp":"2026-09-12T01:04:00Z","message":{"model":"next-run-model"}}
        ]''')
        self.assertEqual(response_models(entries, '2026-09-12T01:01:00+00:00', '2026-09-12T09:03:00+08:00'), ['MiniMax-M3'])
        self.assertEqual(response_models(entries[:2], '2026-09-12T01:01:00Z', '2026-09-12T01:03:00Z'), [])

    def test_paired_adoption_and_failure_gates(self):
        from report import compare
        rows=[{'case_id':str(case),'repetition':rep,'strategy':strategy,'passed':strategy=='evolved',
               'actual_tokens':100,'status':'passed','cleanup_errors':[]} for case in range(10) for rep in range(3) for strategy in ['team','evolved']]
        self.assertEqual(compare(rows)['decision'],'adopt')
        for t in rows: t['passed']=True
        self.assertEqual(compare(rows)['decision'],'inconclusive')
        for t in rows:
            if t['strategy']=='evolved': t['actual_tokens']=70
        self.assertEqual(compare(rows)['decision'],'adopt')
        rows[1]['passed']=False
        self.assertEqual(compare(rows)['decision'],'reject')
        rows[1]['passed']=True; rows[1]['actual_tokens']=None
        self.assertEqual(compare(rows)['decision'],'inconclusive')
        self.assertEqual(compare(rows[:-1])['decision'],'inconclusive')
        self.assertEqual(compare(rows[:20])['decision'],'inconclusive')
    def test_regression_outcome_keeps_failures_and_requires_all_groups(self):
        from pipeline import REGRESSIONS, regression_outcome
        rows=[{'name':'regression-'+name,'status':'completed'} for name in REGRESSIONS]
        self.assertEqual(regression_outcome(rows),'completed')
        self.assertEqual(regression_outcome(rows[:-1]),'failed')
        self.assertEqual(regression_outcome(rows+[{'name':rows[0]['name'],'status':'failed'}]),'failed')
        rows[0]['status']='incomplete'
        self.assertEqual(regression_outcome(rows),'failed')

    def test_learning_rejects_holdout_data(self):
        from evolve import learning_case
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'report.json'
            path.write_text(json.dumps({'status':'completed','trials':[{'split':'holdout'}]}))
            with self.assertRaisesRegex(ValueError,'development'):
                learning_case(path,Path(__file__).with_name('datasets')/'solo-skills-v3.json')
    def test_fixed_public_dataset_and_oracles(self):
        import gzip,hashlib
        directory=Path(__file__).parent
        upstream=[json.loads(line) for line in gzip.decompress((directory/'vendor/human-eval/HumanEval.jsonl.gz').read_bytes()).decode().splitlines()]
        selected=sorted(upstream,key=lambda c:hashlib.sha256(('solo-v1:'+c['task_id']).encode()).hexdigest())[:20]
        cases=json.loads((directory/'datasets/humaneval-smoke.json').read_text())
        self.assertEqual([c['id'] for c in cases],[c['task_id'] for c in selected])
        for c,u in zip(cases,selected):
            with self.subTest(case=c['id']):
                self.assertEqual(c['test'],u['test'])
                self.assertEqual(c['reference'],u['prompt']+u['canonical_solution'])
                self.assertTrue(grade(c,c['reference'])['passed'])
                self.assertFalse(grade(c,f"def {c['entry_point']}(*args): return None")['passed'])

if __name__=='__main__': unittest.main()
