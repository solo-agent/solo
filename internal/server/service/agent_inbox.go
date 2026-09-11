package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/pkg/agent"
)

// The caller holds agent-work:<id>. Every source uses this same persisted order.
func agentInboxTurnTx(ctx context.Context, tx pgx.Tx, agentID, kind, workID string) (bool, error) {
	var head bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_inbox_heads WHERE agent_id=$1 AND kind=$2 AND work_id=$3)`, agentID, kind, workID).Scan(&head)
	return head, err
}

// enqueueAgentWork persists ordinary requests before they compete with message,
// review, wait and Selection queues. No runtime credentials/configuration are copied.
func (s *AgentService) enqueueAgentWork(ctx context.Context, req daemonTaskRequest) error {
	kind, priority := "greeting", 20
	if req.NodeID != "" {
		kind, priority = "thinking", 10
	} else if req.OriginTaskID != "" {
		kind, priority = "task", 10
		if req.ResultContract == agentResultContractNone {
			kind = "artifact"
		}
	}
	req.SystemPrompt = ""
	req.ThinkingRuntimePrompt = ""
	req.RelationshipsMarkdown = ""
	req.CustomEnv = nil
	req.CustomArgs = nil
	req.Skills = nil
	req.AgentToken = ""
	req.ModelConfig = agent.ModelConfig{}
	req.ColdStartMessages = nil
	if req.NodeID != "" {
		// Branch awareness is read after claiming; never replay a Handoff captured
		// before another queued branch completed.
		filtered := make([]agent.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			if m.SenderID == "system" && m.Role == agent.RoleUser && strings.HasPrefix(m.Content, "[Solo Thinking branch context; Agent") {
				continue
			}
			filtered = append(filtered, m)
		}
		req.Messages = filtered
	}
	recovery, err := marshalJSON(req.Recovery)
	if err != nil {
		return err
	}
	var version *int64
	if req.OriginTaskID != "" {
		var v int64
		if err = s.pool.QueryRow(ctx, `SELECT version,COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1 AND channel_id=$2`, req.OriginTaskID, req.ChannelID).Scan(&v, &req.OriginTaskAssigneeID); err != nil {
			return err
		}
		version = &v
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	// Exact persisted trigger identities survive retries; distinct protocol or
	// artifact requests on a single Task/branch retain their own work.
	identity := req.Messages
	if req.TriggerMessageID != "" && req.OriginTaskID == "" {
		identity = nil
	}
	source, _ := json.Marshal([]any{kind, req.ChannelID, req.NodeID, req.OriginTaskID, version, req.TriggerMessageID, req.ModelSeenSeq, identity, req.Recovery})
	key := fmt.Sprintf("%x", sha256.Sum256(source))
	if kind == "greeting" {
		var joined string
		if err = s.pool.QueryRow(ctx, `SELECT joined_at::text FROM channel_members WHERE channel_id=$1 AND member_type='agent' AND member_id=$2`, req.ChannelID, req.AgentID).Scan(&joined); err != nil {
			return err
		}
		key += ":" + joined
	}
	// A new explicit checkpoint/return or artifact request can be meaningful
	// even without another visible message. Its durable row owns that request.
	if req.ResultContract == agentResultContractHandoff || kind == "artifact" {
		key += ":" + uuid.NewString()
	}

	tag, err := s.pool.Exec(ctx, `INSERT INTO agent_pending_work(agent_id,channel_id,task_id,task_version,thinking_node_id,work_key,kind,priority,payload,recovery,seen_seq)
 SELECT a.id,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM agents a JOIN channels c ON c.id=$2
 WHERE a.id=$1 AND a.is_active AND NOT c.is_archived AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=$2 AND m.member_type='agent' AND m.member_id=a.id)
 ON CONFLICT(agent_id,work_key) DO NOTHING`, req.AgentID, req.ChannelID, nullableUUID(req.OriginTaskID), version, nullableUUID(req.NodeID), key, kind, priority, payload, recovery, req.ModelSeenSeq)
	if err == nil && tag.RowsAffected() == 0 {
		var replay bool
		if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_pending_work WHERE agent_id=$1 AND work_key=$2)`, req.AgentID, key).Scan(&replay); err != nil {
			return err
		}
		if !replay {
			return ErrAgentWorkForbidden
		}
	}
	return err
}

func (s *AgentService) dispatchPendingAgentWork(ctx context.Context) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// Withdrawals are durable and do not silently reassign responsibility.
	if _, err = tx.Exec(ctx, `UPDATE agent_pending_work p SET status='cancelled',last_error='Agent or channel access was withdrawn',updated_at=now() WHERE status='pending' AND (
 NOT EXISTS(SELECT 1 FROM agents a WHERE a.id=p.agent_id AND a.is_active) OR
 NOT EXISTS(SELECT 1 FROM channel_members m JOIN channels c ON c.id=m.channel_id WHERE m.member_id=p.agent_id AND m.member_type='agent' AND m.channel_id=p.channel_id AND NOT c.is_archived))`); err != nil {
		return false, err
	}
	var id, kind string
	var req daemonTaskRequest
	var recovery json.RawMessage
	var seen int64
	err = tx.QueryRow(ctx, `SELECT p.id::text,p.kind,p.payload,p.recovery,p.seen_seq FROM agent_pending_work p JOIN agent_inbox_heads h ON h.work_id=p.id::text AND h.agent_id=p.agent_id AND h.kind=p.kind
 WHERE p.status='pending' ORDER BY h.priority,h.created_at,h.work_id FOR UPDATE OF p SKIP LOCKED LIMIT 1`).Scan(&id, &kind, &req, &recovery, &seen)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "agent-work:"+req.AgentID).Scan(&locked); err != nil || !locked {
		return false, err
	}
	head, err := agentInboxTurnTx(ctx, tx, req.AgentID, kind, id)
	if err != nil || !head {
		return false, err
	}
	cancelWork := func(reason string) (bool, error) {
		_, e := tx.Exec(ctx, `UPDATE agent_pending_work SET status='cancelled',last_error=$2,updated_at=now() WHERE id=$1`, id, reason)
		if e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	failed := func(cause error) (bool, error) {
		_, e := tx.Exec(ctx, `UPDATE agent_pending_work SET last_error=$2,next_attempt_at=now()+interval '30 seconds',updated_at=now() WHERE id=$1`, id, truncateRunes(cause.Error(), 2000))
		if e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	var ag agentChannelInfo
	if err = tx.QueryRow(ctx, `SELECT a.id,a.name,a.model_provider,a.model_name,a.system_prompt FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' JOIN channels c ON c.id=m.channel_id WHERE a.id=$1 AND m.channel_id=$2 AND a.is_active AND NOT c.is_archived FOR SHARE OF a,m,c`, req.AgentID, req.ChannelID).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName, &ag.SystemPrompt); err != nil {
		return failed(err)
	}
	req.SystemPrompt = ag.SystemPrompt
	req.ModelConfig = agent.ModelConfig{Provider: ag.ModelProvider, Model: ag.ModelName}
	req.ModelSeenSeq = seen
	if len(recovery) > 0 && string(recovery) != "null" {
		if err = json.Unmarshal(recovery, &req.Recovery); err != nil {
			return failed(err)
		}
	}
	if req.OriginTaskID != "" {
		var current json.RawMessage
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT to_jsonb(t), $3='artifact' OR (t.status IN ('todo','in_progress') AND (t.claimer_id IS NULL OR t.claimer_id=$2) AND ($5='' OR COALESCE(t.claimer_id::text,'')=$5) AND NOT EXISTS(SELECT 1 FROM task_waits w WHERE w.task_id=t.id AND w.status='waiting')) FROM tasks t WHERE t.id=$1 AND t.channel_id=$4 FOR SHARE`, req.OriginTaskID, req.AgentID, kind, req.ChannelID, req.OriginTaskAssigneeID).Scan(&current, &allowed)
		if errors.Is(err, pgx.ErrNoRows) {
			return cancelWork("Task was removed")
		}
		if err != nil {
			return failed(err)
		}
		if !allowed {
			return cancelWork("Task no longer belongs to this pending execution")
		}
		if kind == "artifact" {
			var task Task
			if err = json.Unmarshal(current, &task); err != nil {
				return failed(err)
			}
			leader, found := s.findArtifactLeader(ctx, &task)
			if !found || leader.ID != req.AgentID {
				return cancelWork("Artifact responsible Agent changed")
			}
		}
		req.Messages = append(req.Messages, agent.Message{Role: agent.RoleUser, SenderID: "system", Content: "Current authoritative Task at Inbox claim (supersedes the historical trigger):\n" + string(current)})
	}
	if req.NodeID != "" {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT returned_at IS NULL AND agent_id=$2 AND (NOT $3 OR returning_at IS NOT NULL) FROM thinking_nodes WHERE id=$1 FOR SHARE`, req.NodeID, req.AgentID, req.ReturnHandoff).Scan(&active); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return cancelWork("Thinking branch was removed")
			}
			return failed(e)
		}
		if !active {
			return cancelWork("Thinking branch completed or return was cancelled")
		}
		node, e := s.getThinkingNodeRuntimeContext(ctx, req.ChannelID, req.NodeID)
		if e != nil {
			return failed(e)
		}
		if node.AgentID != req.AgentID {
			return cancelWork("Thinking branch owner changed")
		}
		req.ThinkingRuntimePrompt = node.StaticPrompt
		req.ResumeSessionID = node.ResumeSessionID
		req.ColdStartMessages, e = s.getRecentMessagesForNode(ctx, req.ChannelID, req.NodeID, defaultNodeContextMessageCount)
		if e != nil {
			return failed(e)
		}
		if req.ResultContract == agentResultContractHandoff {
			req.ColdStartMessages = append(req.ColdStartMessages, req.Messages...)
		}
		if node.TurnContext != "" {
			m := agent.Message{Role: agent.RoleUser, SenderID: "system", Content: "[Solo Thinking branch context; Agent-authored Handoffs only]\n" + node.TurnContext}
			req.Messages = append([]agent.Message{m}, req.Messages...)
			req.ColdStartMessages = append([]agent.Message{m}, req.ColdStartMessages...)
		}
	}
	dmn, err := s.dm.ResolveDaemonForAgent(ctx, req.AgentID, "llm")
	if err != nil {
		return failed(err)
	}
	trigger := AgentRunTriggerMessage
	if req.OriginTaskID != "" {
		trigger = AgentRunTriggerTask
	}
	// A failed reservation/payload preparation must not leave an orphan Run.
	attempt, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	run, err := NewAgentRunService(s.pool).startRunTx(ctx, attempt, StartRunInput{AgentID: req.AgentID, DaemonID: dmn.ID, ChannelID: req.ChannelID, ThreadID: req.ThreadID, ThinkingNodeID: req.NodeID, TriggerMessageID: req.TriggerMessageID, TriggerType: trigger, Source: ag.ModelProvider, ActivityText: "等待执行", FreshnessSeenSeq: seen})
	if err == nil && req.OriginTaskID != "" {
		role := req.TaskLinkRole
		if role == "" {
			role = AgentRunTaskRolePrimary
		}
		_, err = attempt.Exec(ctx, `INSERT INTO agent_run_task_links(run_id,task_id,role,confidence) VALUES($1,$2,$3,1) ON CONFLICT DO NOTHING`, run.ID, req.OriginTaskID, role)
	}
	if err == nil {
		err = s.persistRemoteMessageRunTx(ctx, attempt, dmn, run, req, ag)
	}
	if err != nil {
		if e := attempt.Rollback(ctx); e != nil {
			return false, e
		}
		return failed(err)
	}
	if err = attempt.Commit(ctx); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_pending_work SET status='dispatched',run_id=$2,last_error='',updated_at=now() WHERE id=$1`, id, run.ID); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	req.PrestartedRun = true
	req.RemotePrequeued = dmn.ComputerID != ""
	go s.runStreamingAgentTask(context.Background(), dmn, req, ag, run)
	return true, nil
}
