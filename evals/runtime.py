#!/usr/bin/env python3
"""Read actual response metadata without changing trial scores or settings."""
import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
from uuid import UUID


def response_metadata(entries, started_at, finished_at):
    start, finish = (datetime.fromisoformat(value.replace('Z', '+00:00')) for value in [started_at, finished_at])
    models = set()
    block_types = set()
    first_response = None
    for entry in entries:
        if entry.get('type') != 'assistant' or not entry.get('timestamp'):
            continue
        timestamp = datetime.fromisoformat(entry['timestamp'].replace('Z', '+00:00'))
        model = entry.get('message', {}).get('model')
        if start <= timestamp <= finish and model and not model.startswith('<'):
            models.add(model)
            first_response = min(first_response, timestamp) if first_response else timestamp
            block_types.update(block['type'] for block in entry.get('message', {}).get('content', []) if isinstance(block, dict) and isinstance(block.get('type'), str))
    return {'response_models': sorted(models), 'response_block_types': sorted(block_types),
            'first_recorded_response_seconds': (first_response - start).total_seconds() if first_response else None}


def codex_metadata(entries, started_at, finished_at):
    start, finish = (datetime.fromisoformat(value.replace('Z', '+00:00')) for value in [started_at, finished_at])
    models, efforts, block_types = set(), set(), set()
    first_response = None
    for entry in entries:
        if not entry.get('timestamp'):
            continue
        timestamp = datetime.fromisoformat(entry['timestamp'].replace('Z', '+00:00'))
        if not start <= timestamp <= finish:
            continue
        payload = entry.get('payload', {})
        if entry.get('type') == 'turn_context':
            if payload.get('model'):
                models.add(payload['model'])
            if payload.get('effort'):
                efforts.add(payload['effort'])
        kind = payload.get('type')
        if entry.get('type') == 'response_item' and (kind in {'reasoning', 'function_call', 'custom_tool_call'} or (kind == 'message' and payload.get('role') == 'assistant')):
            block_types.add(kind)
            first_response = min(first_response, timestamp) if first_response else timestamp
    return {'response_models': [], 'runtime_models': sorted(models), 'runtime_efforts': sorted(efforts),
            'response_block_types': sorted(block_types),
            'first_recorded_response_seconds': (first_response - start).total_seconds() if first_response else None}


def collect_runtime(report):
    ids = sorted({str(UUID(r['id'])) for t in report['trials'] for r in t.get('runs', [])})
    records = {}
    if ids:
        quoted = ','.join("'" + ident + "'" for ident in ids)
        query = "SELECT COALESCE(jsonb_agg(jsonb_build_object('id',r.id,'source',r.source,'started_at',r.started_at,'finished_at',r.finished_at,'transcript_path',s.transcript_path)),'[]') FROM agent_runs r LEFT JOIN agent_sessions s ON s.id=r.session_id WHERE r.id IN (" + quoted + ")"
        rows = json.loads(subprocess.check_output(['docker', 'exec', os.environ.get('SOLO_POSTGRES_CONTAINER', 'solo-postgres'), 'psql', '-U', os.environ.get('POSTGRES_USER', 'solo'), '-d', os.environ.get('POSTGRES_DB', 'solo'), '-At', '-v', 'ON_ERROR_STOP=1', '-c', query], text=True))
        records = {row['id']: row for row in rows}
    runs = []
    cache = {}
    for ident in ids:
        row = records.get(ident, {})
        result = {'run_id': ident, 'response_models': [], 'status': 'unverified'}
        runs.append(result)
        if row.get('source') not in {'claude', 'codex'}:
            result['reason'] = 'Runtime attribution is supported only for Claude Code and Codex transcripts'
            continue
        if not row.get('transcript_path') or not row.get('started_at') or not row.get('finished_at'):
            result['reason'] = 'Missing transcript path or completed Run time range'
            continue
        try:
            path = Path(row['transcript_path']).resolve()
            codex_home = Path(os.environ.get('CODEX_HOME') or Path.home() / '.codex')
            roots = [Path.home() / '.claude/projects'] if row['source'] == 'claude' else [codex_home / 'sessions', codex_home / 'archived_sessions']
            if not any(path.is_relative_to(root.resolve()) for root in roots):
                raise ValueError('Transcript is outside the local provider session directories')
            if path not in cache:
                raw = path.read_bytes()
                cache[path] = (hashlib.sha256(raw).hexdigest(), [json.loads(line) for line in raw.splitlines() if line.strip()])
            sha, entries = cache[path]
            parse = response_metadata if row['source'] == 'claude' else codex_metadata
            metadata = parse(entries, row['started_at'], row['finished_at'])
            result.update(**metadata, transcript_sha256=sha, started_at=row['started_at'], finished_at=row['finished_at'])
            result.update(status='observed' if metadata['response_models'] else 'unverified', reason=None if metadata['response_models'] else 'No provider response model in this Run time range')
            if metadata.get('runtime_models'):
                result.update(status='context_observed', reason='Codex turn_context records runtime configuration, not a provider response model')
        except (OSError, ValueError, TypeError, AttributeError) as error:
            result['reason'] = type(error).__name__ + ': unable to verify local transcript metadata'
    return {'scope': 'Read-only response identifiers and block types. Codex runtime_models/runtime_efforts are turn_context configuration, not provider response identifiers or verified weights. No thinking content is exported. First recorded response time is not streaming time-to-first-token. Requested overrides do not prove provider behavior.',
            'requested_provider': report.get('provider'), 'requested_model': report.get('model'),
            'requested_thinking': report.get('requested_thinking'),
            'response_models': sorted({model for run in runs for model in run['response_models']}),
            'runtime_models': sorted({model for run in runs for model in run.get('runtime_models', [])}),
            'runtime_efforts': sorted({effort for run in runs for effort in run.get('runtime_efforts', [])}),
            'context_observed_runs': sum(run['status'] == 'context_observed' for run in runs),
            'observed_thinking_runs': sum(bool({'thinking', 'redacted_thinking', 'reasoning'} & set(run.get('response_block_types', []))) for run in runs),
            'observed_runs': sum(run['status'] == 'observed' for run in runs), 'total_runs': len(ids), 'runs': runs}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('report', type=Path)
    parser.add_argument('--output', type=Path, required=True, help='New supplemental file; never rewrites the report')
    args = parser.parse_args()
    audit = collect_runtime(json.loads(args.report.read_text()))
    with args.output.open('x') as output:
        output.write(json.dumps(audit, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: value for key, value in audit.items() if key != 'runs'}, ensure_ascii=False))
