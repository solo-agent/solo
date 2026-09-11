ALTER TABLE agent_run_token_usage ADD COLUMN budget_context JSONB;

CREATE TABLE task_observations (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 observer_id UUID NOT NULL REFERENCES users(id),
 kind TEXT NOT NULL CHECK (kind IN ('intervention','rework','duplicate_work','reexplanation','handoff','recovery','comparison')),
 note TEXT NOT NULL CHECK (length(note) BETWEEN 1 AND 4000),
 minutes DOUBLE PRECISION CHECK (minutes >= 0 AND minutes <= 1440),
 category TEXT NOT NULL DEFAULT '',
 run_id UUID REFERENCES agent_runs(id),
 review_id UUID REFERENCES task_reviews(id),
 first_action_correct BOOLEAN,
 idempotency_key TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(task_id,observer_id,idempotency_key),
 CHECK ((kind='intervention') = (minutes IS NOT NULL)),
 CHECK ((kind='rework') = (review_id IS NOT NULL)),
 CHECK ((kind='recovery') = (run_id IS NOT NULL AND first_action_correct IS NOT NULL))
);
CREATE INDEX task_observations_task ON task_observations(task_id,created_at);
CREATE UNIQUE INDEX task_observations_review ON task_observations(review_id) WHERE kind='rework';
CREATE UNIQUE INDEX task_observations_recovery ON task_observations(run_id) WHERE kind='recovery';
CREATE FUNCTION immutable_task_observation() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'Task observations are append-only'; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER task_observations_immutable BEFORE UPDATE ON task_observations FOR EACH ROW EXECUTE FUNCTION immutable_task_observation();

-- Read-only projection; the underlying Task, Review and Run remain authoritative.
CREATE VIEW task_delivery_measurements AS
SELECT t.id,t.channel_id,t.created_at,
 jsonb_build_object('task_id',t.id,'task_number',t.task_number,'title',t.title,'status',t.status,
 'qualified',t.status='done' AND accepted.created_at IS NOT NULL,
 'elapsed_seconds',CASE WHEN t.status='done' THEN EXTRACT(epoch FROM accepted.created_at-t.created_at) END,
 'cohort',COALESCE((SELECT o.category FROM task_observations o WHERE o.task_id=t.id AND o.kind='comparison' ORDER BY o.created_at DESC,o.id LIMIT 1),''),
 'observations',COALESCE((SELECT jsonb_agg(to_jsonb(o) - 'request_hash' ORDER BY o.created_at,o.id) FROM task_observations o WHERE o.task_id=t.id),'[]'),
 'reworks',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',rv.id,'reason',rv.reason,'category',COALESCE(o.category,'unclassified'))) FROM task_reviews rv LEFT JOIN task_observations o ON o.review_id=rv.id WHERE rv.task_id=t.id AND rv.decision='rejected'),'[]'),
 'runs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',r.id,'agent_id',r.agent_id,'status',r.status,'model',CASE WHEN r.source='code_gate' THEN 'code_gate/process' ELSE COALESCE(v.config->>'model_provider','unknown')||'/'||COALESCE(v.config->>'model_name','unknown') END,
 'budget_owner_id',COALESCE(u.owner_id,(SELECT owner_id FROM agents WHERE id=r.agent_id)),'budget',CASE WHEN r.source='code_gate' THEN '{"code_gate_tokens":0}'::jsonb ELSE u.budget_context END,'actual_tokens',CASE WHEN r.source='code_gate' THEN 0 ELSE u.actual_tokens END,'accounted_tokens',COALESCE(u.actual_tokens,u.reserved_tokens,0),
 'active',r.finished_at IS NULL,'execution_seconds',EXTRACT(epoch FROM r.finished_at-COALESCE(r.backend_started_at,r.started_at)),
 'shared',(SELECT count(*) FROM (SELECT all_links.task_id FROM agent_run_task_links all_links WHERE all_links.run_id=r.id UNION SELECT all_sub.task_id FROM task_submissions all_sub WHERE all_sub.run_id=r.id) linked_tasks)>1) ORDER BY r.started_at,r.id)
 FROM agent_runs r LEFT JOIN agent_revisions v ON v.id=r.agent_revision_id LEFT JOIN agent_run_token_usage u ON u.run_id=r.id
 WHERE r.id IN (SELECT l.run_id FROM agent_run_task_links l WHERE l.task_id=t.id UNION SELECT sub.run_id FROM task_submissions sub WHERE sub.task_id=t.id)),'[]'),
 'recoveries',COALESCE((SELECT jsonb_agg(jsonb_build_object('run_id',w.run_id,'first_tool_seconds',EXTRACT(epoch FROM (SELECT min(e.created_at) FROM agent_run_events e WHERE e.run_id=w.run_id AND e.type='tool_started')-w.resolved_at),'first_action_correct',o.first_action_correct)) FROM task_waits w LEFT JOIN task_observations o ON o.run_id=w.run_id WHERE w.task_id=t.id AND w.status='resumed'),'[]')
 ) AS metrics
FROM tasks t LEFT JOIN LATERAL (SELECT r.created_at FROM task_reviews r WHERE r.submission_id=t.current_submission_id AND r.decision='accepted' ORDER BY r.created_at DESC LIMIT 1) accepted ON true;
