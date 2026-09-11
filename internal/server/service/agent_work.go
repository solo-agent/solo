package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAgentWorkForbidden = errors.New("only the Agent or its owner can manage its work")

func requireAgentWorkOwner(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string) error {
	var allowed bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1 AND (id=$2 OR owner_id=$2) AND is_active)`, agentID, actorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrAgentWorkForbidden
	}
	return nil
}

func GetAgentWork(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string) (json.RawMessage, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return nil, err
	}
	var result json.RawMessage
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
	 'policy',attention_policy,
	 'inbox',COALESCE((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.priority,i.created_at,i.kind,i.work_id) FROM agent_inbox i WHERE i.agent_id=a.id),'[]'),
	 'channel_policies',COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM agent_channel_attention p WHERE p.agent_id=a.id),'[]'),
	 'marks',COALESCE((SELECT jsonb_agg(to_jsonb(m) ORDER BY m.created_at) FROM agent_work_marks m WHERE m.agent_id=a.id AND m.status='open'),'[]'),
	 'waits',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',w.id,'task_id',t.id,'task_number',t.task_number,'channel_id',t.channel_id,'title',t.title,'condition',w.condition,'next_action',w.next_action)) FROM task_waits w JOIN tasks t ON t.id=w.task_id WHERE w.agent_id=a.id AND w.status='waiting'),'[]'),
	 'pending',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.requires_visible_result DESC,p.created_at) FROM agent_pending_message_wakes p WHERE p.agent_id=a.id),'[]'),
	 'reviews',COALESCE((SELECT jsonb_agg(jsonb_build_object('task_id',t.id,'title',t.title,'submission_id',t.current_submission_id)) FROM tasks t WHERE t.status='in_review' AND t.contract->'gate'->>'reviewer_id'=a.id::text),'[]'),
	 'runs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',r.id,'channel_id',r.channel_id,'thread_id',r.thread_id,'status',r.status,'activity',r.activity_text)) FROM agent_runs r WHERE r.agent_id=a.id AND r.finished_at IS NULL),'[]'),
	 'feedback',COALESCE((SELECT jsonb_agg(to_jsonb(f) ORDER BY f.created_at DESC) FROM (
       SELECT t.id task_id,t.channel_id,t.title,r.id source_id,'review' kind,r.decision category,r.reason note,r.created_at
       FROM task_reviews r JOIN task_submissions sub ON sub.id=r.submission_id JOIN tasks t ON t.id=sub.task_id
       WHERE sub.submitted_by=a.id AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.channel_id=t.channel_id AND cm.member_type='agent' AND cm.member_id=a.id)
       AND ($2=a.id OR EXISTS(SELECT 1 FROM channel_members cm WHERE cm.channel_id=t.channel_id AND cm.member_type='user' AND cm.member_id=$2))
       UNION ALL
       SELECT t.id,t.channel_id,t.title,o.id,o.kind,o.category,o.note,o.created_at
       FROM task_observations o JOIN tasks t ON t.id=o.task_id
       WHERE o.kind IN ('rework','handoff','reexplanation','recovery') AND (t.claimer_id=a.id OR EXISTS(SELECT 1 FROM task_submissions sub WHERE sub.task_id=t.id AND sub.submitted_by=a.id))
       AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.channel_id=t.channel_id AND cm.member_type='agent' AND cm.member_id=a.id)
       AND ($2=a.id OR EXISTS(SELECT 1 FROM channel_members cm WHERE cm.channel_id=t.channel_id AND cm.member_type='user' AND cm.member_id=$2))
       ORDER BY created_at DESC LIMIT 20
     ) f),'[]'),
	 'drafts',COALESCE((SELECT jsonb_agg(to_jsonb(d) || jsonb_build_object('sha256',encode(sha256(convert_to(d.content,'UTF8')),'hex'))) FROM agent_held_drafts d JOIN agent_runs r ON r.id=d.run_id WHERE r.agent_id=a.id),'[]')
	 ) FROM agents a WHERE a.id=$1`, agentID, actorID).Scan(&result)
	return result, err
}

func SetAgentAttention(ctx context.Context, pool *pgxpool.Pool, agentID, actorID, channelID, policy string) error {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return err
	}
	if policy != "all" && policy != "mentions" && policy != "nothing" && !(policy == "inherit" && channelID != "") {
		return invalidDelivery("policy must be all, mentions, nothing, or a channel override of inherit")
	}
	if channelID != "" {
		if _, err := uuid.Parse(channelID); err != nil {
			return invalidDelivery("invalid channel ID")
		}
		if err := NewTaskService(pool).requireChannelMember(ctx, channelID, agentID); err != nil {
			return err
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "agent-work:"+agentID); err != nil {
		return err
	}
	switch {
	case channelID == "":
		_, err = tx.Exec(ctx, `UPDATE agents SET attention_policy=$2 WHERE id=$1`, agentID, policy)
	case policy == "inherit":
		_, err = tx.Exec(ctx, `DELETE FROM agent_channel_attention WHERE agent_id=$1 AND channel_id=$2`, agentID, channelID)
	default:
		_, err = tx.Exec(ctx, `INSERT INTO agent_channel_attention(agent_id,channel_id,policy) VALUES($1,$2,$3) ON CONFLICT(agent_id,channel_id) DO UPDATE SET policy=EXCLUDED.policy`, agentID, channelID, policy)
	}
	if err != nil {
		return err
	}
	// Muting withdraws automatic, unclaimed delivery only. Task ownership,
	// active Runs, promises and the original messages remain intact.
	if _, err = tx.Exec(ctx, `DELETE FROM agent_pending_message_wakes p USING agents a WHERE p.agent_id=a.id AND a.id=$1
 AND COALESCE((SELECT policy FROM agent_channel_attention x WHERE x.agent_id=a.id AND x.channel_id=p.channel_id),a.attention_policy)='nothing'`, agentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type AgentWorkMarkRequest struct {
	ID              string `json:"id"`
	ChannelID       string `json:"channel_id"`
	SourceMessageID string `json:"source_message_id"`
	TaskID          string `json:"task_id"`
	Description     string `json:"description"`
	NextAction      string `json:"next_action"`
	Status          string `json:"status"`
	Resolution      string `json:"resolution"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func SaveAgentWorkMark(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string, req AgentWorkMarkRequest) (string, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return "", err
	}
	if req.ID != "" {
		if _, err := uuid.Parse(req.ID); err != nil {
			return "", invalidDelivery("invalid work mark ID")
		}
		if (req.Status != "resolved" && req.Status != "cancelled") || strings.TrimSpace(req.Resolution) == "" || len(req.Resolution) > 8000 {
			return "", invalidDelivery("resolution and final status are required")
		}
		result, err := pool.Exec(ctx, `UPDATE agent_work_marks SET status=$3,resolution=$4,updated_at=now() WHERE id=$1 AND agent_id=$2 AND (status='open' OR (status=$3 AND resolution=$4))`, req.ID, agentID, req.Status, req.Resolution)
		if err != nil {
			return "", err
		}
		if result.RowsAffected() != 1 {
			return "", ErrTaskVersionConflict
		}
		return req.ID, nil
	}
	if !deliveryID.MatchString(req.IdempotencyKey) || strings.TrimSpace(req.Description) == "" || len(req.Description) > 8000 || len(req.NextAction) > 8000 {
		return "", invalidDelivery("work description and idempotency key are required")
	}
	if _, err := uuid.Parse(req.ChannelID); err != nil {
		return "", invalidDelivery("invalid channel ID")
	}
	if err := NewTaskService(pool).requireChannelMember(ctx, req.ChannelID, agentID); err != nil {
		return "", err
	}
	if req.SourceMessageID != "" {
		if _, err := uuid.Parse(req.SourceMessageID); err != nil {
			return "", invalidDelivery("invalid source message ID")
		}
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id=$1 AND channel_id=$2 AND NOT is_deleted)`, req.SourceMessageID, req.ChannelID).Scan(&valid); err != nil {
			return "", err
		}
		if !valid {
			return "", invalidDelivery("source message is not in this channel")
		}
	}
	if req.TaskID != "" {
		if _, err := NewTaskService(pool).GetTask(ctx, req.ChannelID, req.TaskID, agentID); err != nil {
			return "", err
		}
	}
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO agent_work_marks(agent_id,channel_id,source_message_id,task_id,description,next_action,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7)
	 ON CONFLICT(agent_id,idempotency_key) DO UPDATE SET id=agent_work_marks.id
	 WHERE agent_work_marks.channel_id=EXCLUDED.channel_id AND agent_work_marks.description=EXCLUDED.description AND agent_work_marks.next_action=EXCLUDED.next_action AND agent_work_marks.source_message_id IS NOT DISTINCT FROM EXCLUDED.source_message_id AND agent_work_marks.task_id IS NOT DISTINCT FROM EXCLUDED.task_id
	 RETURNING id::text`, agentID, req.ChannelID, nullableStr(req.SourceMessageID), nullableStr(req.TaskID), req.Description, req.NextAction, req.IdempotencyKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrTaskVersionConflict
	}
	return id, err
}

func (s *AgentService) filterAttention(ctx context.Context, ids []string, req wakeRouteRequest) ([]string, error) {
	if req.Scope == wakeScopeTask || len(ids) == 0 {
		return ids, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id::text FROM agents a
	 LEFT JOIN agent_channel_attention p ON p.agent_id=a.id AND p.channel_id=$2
	 LEFT JOIN thread_subscriptions ts ON ts.actor_id=a.id AND ts.thread_id=NULLIF($3,'')::uuid
	 WHERE a.id::text=ANY($1) AND COALESCE(p.policy,a.attention_policy)<>'nothing'
	 AND (a.id::text=ANY($4) OR ts.followed=true OR (COALESCE(p.policy,a.attention_policy)='all' AND COALESCE(ts.followed,true)))`, ids, req.ChannelID, req.ThreadID, req.MentionedAgentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowed := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = true
	}
	result := []string{}
	for _, id := range ids {
		if allowed[id] {
			result = append(result, id)
		}
	}
	return result, rows.Err()
}

type DiscardAgentDraftRequest struct {
	RunID  string `json:"run_id"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
}

// DiscardAgentDraft ends one exact draft, never the Run, Task or work obligation.
func DiscardAgentDraft(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string, req DiscardAgentDraftRequest) error {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return err
	}
	if _, err := uuid.Parse(req.RunID); err != nil {
		return invalidDelivery("invalid draft Run")
	}
	if len(req.SHA256) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 4000 {
		return invalidDelivery("draft SHA256 and reason are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runAgent string
	if err = tx.QueryRow(ctx, `SELECT agent_id::text FROM agent_runs WHERE id=$1 FOR UPDATE`, req.RunID).Scan(&runAgent); err != nil {
		return err
	}
	if runAgent != agentID {
		return ErrAgentWorkForbidden
	}
	var digest string
	err = tx.QueryRow(ctx, `SELECT encode(sha256(convert_to(content,'UTF8')),'hex') FROM agent_held_drafts WHERE run_id=$1`, req.RunID).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		var replay bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_run_events WHERE run_id=$1 AND type='visible_message_draft_discarded' AND payload->>'sha256'=$2 AND payload->>'reason'=$3 AND payload->>'actor_id'=$4)`, req.RunID, req.SHA256, req.Reason, actorID).Scan(&replay)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		return ErrTaskVersionConflict
	}
	if err != nil {
		return err
	}
	if digest != req.SHA256 {
		return ErrTaskVersionConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM agent_held_drafts WHERE run_id=$1`, req.RunID); err != nil {
		return err
	}
	if _, err = appendRunEventTx(ctx, tx, AppendRunEventInput{RunID: req.RunID, Type: "visible_message_draft_discarded", Message: "Held draft explicitly discarded", Payload: map[string]any{"sha256": req.SHA256, "reason": req.Reason, "actor_id": actorID}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
