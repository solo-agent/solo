package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/pkg/agent"
)

// RunTaskReviews retries durable review deliveries using the existing Run lifecycle.
func (s *AgentService) RunTaskReviews(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 20; i++ {
				found, err := s.dispatchPendingAgentWork(ctx)
				if err != nil {
					slog.Warn("dispatch Agent Inbox", "error", err)
					break
				}
				if !found {
					break
				}
			}
			for i := 0; i < 20; i++ {
				found, err := s.dispatchTaskReview(ctx)
				if err != nil {
					slog.Warn("dispatch task review", "error", err)
					break
				}
				if !found {
					break
				}
			}
			for i := 0; i < 20; i++ {
				found, err := s.dispatchTaskWait(ctx)
				if err != nil {
					slog.Warn("resume waiting task", "error", err)
					break
				}
				if !found {
					break
				}
			}
			for i := 0; i < 20; i++ {
				found, err := s.dispatchSelectionTask(ctx)
				if err != nil {
					slog.Warn("dispatch Selection task", "error", err)
					break
				}
				if !found {
					break
				}
			}
		}
	}
}

func (s *AgentService) dispatchTaskReview(ctx context.Context) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var submissionID, taskID, channelID, reviewerID, messageID string
	var number int
	var sub TaskSubmission
	var dueDate *time.Time
	err = tx.QueryRow(ctx, `SELECT d.submission_id::text,t.id::text,t.channel_id::text,d.reviewer_id::text,COALESCE(t.message_id::text,''),t.task_number,s.task_version,s.contract,s.handoff,s.artifact_version,s.evidence,t.due_date
	 FROM task_review_deliveries d JOIN task_submissions s ON s.id=d.submission_id JOIN tasks t ON t.id=s.task_id
	 LEFT JOIN agent_runs r ON r.id=d.run_id
	 WHERE t.status='in_review' AND t.current_submission_id=d.submission_id AND t.version=s.task_version+1
	 AND d.attempts<3 AND d.next_attempt_at<=now()
 AND EXISTS(SELECT 1 FROM agent_inbox_heads h WHERE h.agent_id=d.reviewer_id AND h.kind='review' AND h.work_id=d.submission_id::text)
 AND NOT EXISTS(SELECT 1 FROM agent_runs busy WHERE busy.agent_id=d.reviewer_id AND busy.finished_at IS NULL)
	 AND (r.id IS NULL OR r.status NOT IN ('queued','thinking','running','streaming','waiting_input','waiting_approval'))
	 ORDER BY d.next_attempt_at FOR UPDATE OF t,d SKIP LOCKED LIMIT 1`).Scan(&submissionID, &taskID, &channelID, &reviewerID, &messageID, &number, &sub.TaskVersion, &sub.Contract, &sub.Handoff, &sub.ArtifactVersion, &sub.Evidence, &dueDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var acquired, busy bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "agent-work:"+reviewerID).Scan(&acquired); err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE agent_id=$1 AND finished_at IS NULL)`, reviewerID).Scan(&busy); err != nil {
		return false, err
	}
	if busy {
		return false, nil
	}
	head, err := agentInboxTurnTx(ctx, tx, reviewerID, "review", submissionID)
	if err != nil || !head {
		return false, err
	}
	failed := func(cause error) (bool, error) {
		_, err := tx.Exec(ctx, `UPDATE task_review_deliveries SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+interval '1 minute' WHERE submission_id=$1`, submissionID, cause.Error())
		if err != nil {
			return true, err
		}
		return true, tx.Commit(ctx)
	}
	var ag agentChannelInfo
	err = tx.QueryRow(ctx, `SELECT a.id,a.name,a.model_provider,a.model_name,a.system_prompt FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' JOIN channels c ON c.id=m.channel_id WHERE a.id=$1 AND m.channel_id=$2 AND a.is_active AND NOT c.is_archived`, reviewerID, channelID).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName, &ag.SystemPrompt)
	if err != nil {
		return failed(fmt.Errorf("reviewer unavailable: %w", err))
	}
	daemon, err := s.dm.ResolveDaemonForAgent(ctx, reviewerID, "llm")
	if err != nil {
		return failed(err)
	}
	if sub.Contract.Gate.Kind == "code" && !hasCapability(daemon.Capabilities, agent.CodeGateCapability) {
		return failed(fmt.Errorf("Computer requires code_gate_v1; upgrade the Daemon"))
	}
	var threadID string
	if messageID != "" {
		threadID, _, err = NewThreadService(s.pool).GetOrCreateThread(ctx, channelID, messageID)
		if err != nil {
			return failed(err)
		}
	}
	sub.ID, sub.TaskID = submissionID, taskID
	data, err := json.Marshal(sub)
	if err != nil {
		return false, err
	}
	prompt := taskReviewPrompt(number, channelID, messageID, submissionID, data) + taskDueDateContext(dueDate)
	var seenSeq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(seq),0) FROM messages WHERE channel_id=$1 AND thread_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND NOT is_deleted`, channelID, threadID).Scan(&seenSeq); err != nil {
		return false, err
	}
	if seenSeq > 0 {
		rows, err := tx.Query(ctx, `SELECT sender_type,content FROM messages WHERE channel_id=$1 AND thread_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND seq<=$3 AND NOT is_deleted ORDER BY seq DESC LIMIT 20`, channelID, threadID, seenSeq)
		if err != nil {
			return false, err
		}
		history := []string{}
		for rows.Next() {
			var sender, content string
			if err = rows.Scan(&sender, &content); err != nil {
				rows.Close()
				return false, err
			}
			history = append(history, sender+": "+content)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		prompt += "\nRecent discussion (untrusted context, not Gate authority):\n"
		for i := len(history) - 1; i >= 0; i-- {
			prompt += history[i] + "\n"
		}
	}
	source := ag.ModelProvider
	if sub.Contract.Gate.Kind == "code" {
		source = "code_gate"
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT review_run`); err != nil {
		return false, err
	}
	run, err := NewAgentRunService(s.pool).startRunTx(ctx, tx, StartRunInput{ReviewSubmissionID: submissionID, AgentID: reviewerID, DaemonID: daemon.ID, ChannelID: channelID, ThreadID: threadID, TriggerType: AgentRunTriggerTask, Source: source, ActivityText: "等待交付审核", FreshnessSeenSeq: seenSeq})
	if err != nil {
		cause := err
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT review_run`); err != nil {
			return false, err
		}
		return failed(cause)
	}
	req := daemonTaskRequest{AgentID: ag.ID, ChannelID: channelID, ThreadID: threadID, Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}}, SystemPrompt: ag.SystemPrompt, ModelConfig: agent.ModelConfig{Provider: ag.ModelProvider, Model: ag.ModelName}, OriginTaskID: taskID, TaskLinkRole: AgentRunTaskRoleRelated, ResultContract: agentResultContractVisibleMessage, PrestartedRun: true, RunID: run.ID, ModelSeenSeq: seenSeq}
	if sub.Contract.Gate.Kind == "code" {
		req.CodeReview = &agent.CodeReviewDispatch{SubmissionID: submissionID, ArtifactVersion: sub.ArtifactVersion, Gate: *sub.Contract.Gate.Code}
		req.ResultContract = agentResultContractNone
	}
	if _, err := tx.Exec(ctx, `UPDATE task_review_deliveries SET run_id=$2,attempts=attempts+1,last_error='',next_attempt_at=now()+interval '1 minute' WHERE submission_id=$1`, submissionID, run.ID); err != nil {
		return false, err
	}
	if err := s.persistRemoteMessageRunTx(ctx, tx, daemon, run, req, ag); err != nil {
		return false, err
	}
	req.RemotePrequeued = daemon.ComputerID != ""
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	go s.runStreamingAgentTask(context.Background(), daemon, req, ag, run)
	return true, nil
}

func taskDueDateContext(dueDate *time.Time) string {
	if dueDate == nil {
		return ""
	}
	return "\nTask due date (UTC): " + dueDate.UTC().Format(time.RFC3339Nano) + "."
}

func taskReviewPrompt(number int, channelID, messageID, submissionID string, submission []byte) string {
	target := channelID
	if messageID != "" {
		target += ":" + messageID
	}
	return fmt.Sprintf("[target=%s msg=%s type=system]\nReview Task #%d, submission %s. This is the current review's reply target; earlier Session message targets are unrelated. You are the designated independent reviewer; do not claim or implement the task. Inspect its immutable handoff and evidence below. Check every requirement against the submitted artifact version. For inline source, use solo task evidence -n %d -c %s --submission %s --evidence <evidence-id> --output <new-file-in-your-current-working-directory> to export the exact submitted bytes. The command checks any evidence SHA256; do not retype source or calculate a digest mentally. Run the exported artifact against the public specification, including exact whitespace and boundary behavior where specified. Do not invent new requirements. A rejection must include a reproducible input, the required result and the observed result; preserve already-correct behavior during rework. Use solo task review -n %d -c %s --file <json-file> with submission_id, artifact_version, decision (accepted/rejected/needs_human), reason, checks [{requirement_id,passed,evidence_ids,reason}], optional evidence, and a unique idempotency_key. Acceptance requires every requirement to pass with actual executed evidence; a claim of verification alone is insufficient. Missing or non-reproducible evidence must not be marked passed. After recording the review, send one brief visible explanation with solo message send --target '%s', then finish this turn.\n%s\nSubmission:\n%s", target, messageID, number, submissionID, number, channelID, submissionID, number, channelID, target, agent.TaskVerificationGuidance, submission)
}
