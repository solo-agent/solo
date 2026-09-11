CREATE TABLE agent_pending_work (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
 channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
 task_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
 task_version BIGINT,
 thinking_node_id UUID REFERENCES thinking_nodes(id) ON DELETE CASCADE,
 work_key TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('task','thinking','artifact','greeting')),
 priority INTEGER NOT NULL CHECK(priority IN (10,20)),
 payload JSONB NOT NULL,
 recovery JSONB,
 seen_seq BIGINT NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','dispatched','cancelled')),
 run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
 last_error TEXT NOT NULL DEFAULT '',
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(agent_id,work_key)
);
CREATE INDEX agent_pending_work_ready ON agent_pending_work(next_attempt_at,agent_id) WHERE status='pending';

-- Derived from the existing source queues. There is no second copy to reconcile.
CREATE VIEW agent_inbox AS
 SELECT p.agent_id,'message'::text kind,p.channel_id::text||':'||p.scope_key work_id,p.channel_id,p.thread_id,NULL::uuid task_id,NULL::uuid thinking_node_id,
 '会话消息'::text title,CASE WHEN p.requires_visible_result THEN 10 ELSE 20 END priority,p.created_at,p.created_at ready_at,''::text last_error
 FROM agent_pending_message_wakes p JOIN agents a ON a.id=p.agent_id AND a.is_active JOIN channels c ON c.id=p.channel_id AND NOT c.is_archived
 WHERE EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=p.channel_id AND m.member_type='agent' AND m.member_id=p.agent_id)
 UNION ALL
 SELECT d.reviewer_id,'review',d.submission_id::text,t.channel_id,NULL::uuid,t.id,NULL::uuid,t.title,10,s.created_at,d.next_attempt_at,d.last_error
 FROM task_review_deliveries d JOIN task_submissions s ON s.id=d.submission_id JOIN tasks t ON t.id=s.task_id LEFT JOIN agent_runs r ON r.id=d.run_id
 WHERE t.status='in_review' AND t.current_submission_id=d.submission_id AND t.version=s.task_version+1 AND d.attempts<3 AND (r.id IS NULL OR r.finished_at IS NOT NULL)
 UNION ALL
 SELECT w.agent_id,'wait',w.id::text,t.channel_id,NULL::uuid,t.id,NULL::uuid,t.title,10,w.created_at,w.next_attempt_at,w.last_error
 FROM task_waits w JOIN tasks t ON t.id=w.task_id JOIN channels c ON c.id=t.channel_id LEFT JOIN tasks dep ON dep.id=NULLIF(w.condition->>'task_id','')::uuid
 WHERE w.status='waiting' AND t.version=w.task_version AND t.status='in_progress' AND t.claimer_id=w.agent_id AND NOT c.is_archived
 AND ((w.condition->>'kind'='task_done' AND dep.status='done') OR (w.condition->>'kind'='at_time' AND (w.condition->>'at')::timestamptz<=now()) OR (w.condition->>'kind'='signal' AND w.fulfillment<>'{}'))
 UNION ALL
 SELECT tr.agent_id,'selection',t.id::text,t.channel_id,NULL::uuid,t.id,NULL::uuid,t.title,10,t.created_at,st.next_attempt_at,st.last_error
 FROM agent_selection_tasks st JOIN agent_selections sel ON sel.id=st.selection_id JOIN agent_selection_trials tr ON tr.selection_id=st.selection_id AND tr.arm=st.arm JOIN tasks t ON t.id=st.task_id JOIN agents a ON a.id=tr.agent_id AND a.is_active JOIN channels c ON c.id=t.channel_id AND NOT c.is_archived LEFT JOIN agent_runs r ON r.id=st.run_id
 WHERE sel.status IN ('running','observe') AND t.status='in_progress' AND t.claimer_id=tr.agent_id AND st.attempts<3 AND (r.id IS NULL OR r.finished_at IS NOT NULL)
 AND (st.dispatched_version<t.version OR r.finished_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM agent_selection_tasks prev JOIN tasks pt ON pt.id=prev.task_id WHERE prev.selection_id=st.selection_id AND prev.arm=st.arm AND prev.case_index<st.case_index AND pt.status NOT IN ('done','closed','cancelled'))
 AND (st.arm<>'receiver' OR EXISTS(SELECT 1 FROM agent_selection_tasks ct JOIN tasks candidate ON candidate.id=ct.task_id WHERE ct.selection_id=st.selection_id AND ct.arm='candidate' AND ct.case_index=st.case_index AND candidate.status='done'))
 UNION ALL
 SELECT p.agent_id,p.kind,p.id::text,p.channel_id,NULLIF(p.payload->>'thread_id','')::uuid,p.task_id,p.thinking_node_id,
 COALESCE(t.title,CASE WHEN p.kind='thinking' THEN 'Thinking 分支工作' ELSE '入场问候' END),p.priority,p.created_at,p.next_attempt_at,p.last_error
 FROM agent_pending_work p LEFT JOIN tasks t ON t.id=p.task_id
 WHERE p.status='pending';

CREATE VIEW agent_inbox_heads AS
 SELECT DISTINCT ON (i.agent_id) i.* FROM agent_inbox i
 WHERE ready_at<=now() AND NOT EXISTS(SELECT 1 FROM agent_runs r WHERE r.agent_id=i.agent_id AND r.finished_at IS NULL)
 ORDER BY i.agent_id,i.priority,i.created_at,i.kind,i.work_id;

CREATE FUNCTION release_cancelled_inbox_return() RETURNS trigger AS $$
BEGIN
 IF OLD.status='pending' AND NEW.status='cancelled' AND OLD.payload->>'return_handoff'='true' THEN
  UPDATE thinking_nodes SET returning_at=NULL,updated_at=now()
  WHERE id=OLD.thinking_node_id AND agent_id=OLD.agent_id AND returned_at IS NULL;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER inbox_cancelled_return AFTER UPDATE OF status ON agent_pending_work FOR EACH ROW EXECUTE FUNCTION release_cancelled_inbox_return();
