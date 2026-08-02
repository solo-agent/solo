package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/internal/realtime"
)

const maxTaskRunAttempts = 3

type retryableTaskRun struct {
	RunID      string
	AgentID    string
	TaskID     string
	FinishedAt time.Time
}

func (s *AgentService) retryFailedTaskRuns(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.agent_id::text, link.task_id::text, r.finished_at
		  FROM agent_runs r
		  JOIN agent_run_task_links link ON link.run_id = r.id AND link.role = $1
		  JOIN tasks t ON t.id = link.task_id
		  JOIN LATERAL (
		    SELECT payload
		      FROM agent_run_events
		     WHERE run_id = r.id AND type = $2
		     ORDER BY seq DESC
		     LIMIT 1
		  ) failure ON true
		 WHERE r.status = ANY($3)
		   AND t.status = ANY($4)
		   AND (t.claimer_id IS NULL OR t.claimer_id = r.agent_id)
		   AND t.updated_at <= r.finished_at
		   AND failure.payload->>'failure_code' = ANY($5)
		   AND COALESCE((failure.payload->>'retryable')::boolean, false)
		   AND (
		     SELECT payload->>'result_contract'
		       FROM agent_run_events
		      WHERE run_id = r.id AND type = $6
		      ORDER BY seq
		      LIMIT 1
		   ) = $7
		   AND NOT EXISTS (
		     SELECT 1
		       FROM agent_run_events
		      WHERE run_id = r.id AND type = ANY($8)
		   )
		   AND r.id = (
		     SELECT r2.id
		       FROM agent_runs r2
		       JOIN agent_run_task_links link2 ON link2.run_id = r2.id AND link2.role = $1
		      WHERE link2.task_id = link.task_id
		      ORDER BY r2.started_at DESC, r2.id DESC
		      LIMIT 1
		   )
		 ORDER BY r.finished_at ASC
		 LIMIT 100`,
		AgentRunTaskRolePrimary,
		AgentRunEventError,
		[]string{string(AgentRunStatusFailed), string(AgentRunStatusTimeout)},
		[]string{TaskStatusTodo, TaskStatusInProgress},
		[]string{agentFailureDaemonLost, agentFailureTimeout, agentFailureProviderTransient, agentFailureMissingVisibleResult},
		AgentRunEventRunStarted,
		agentResultContractVisibleMessage,
		[]string{AgentRunEventTaskReassigned, AgentRunEventTaskRetryExhausted},
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var runs []retryableTaskRun
	for rows.Next() {
		var run retryableTaskRun
		if err := rows.Scan(&run.RunID, &run.AgentID, &run.TaskID, &run.FinishedAt); err != nil {
			return err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.retryTaskRun(ctx, run); err != nil {
			slog.Warn("automatic task reassignment failed", "run_id", run.RunID, "task_id", run.TaskID, "error", err)
		}
	}
	return nil
}

func (s *AgentService) retryTaskRun(ctx context.Context, failed retryableTaskRun) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var task Task
	var currentClaimer string
	if err := tx.QueryRow(ctx, `
		SELECT id::text, task_number, channel_id::text, title, COALESCE(description, ''),
		       status, COALESCE(claimer_id::text, ''), priority, due_date,
		       COALESCE(message_id::text, ''), updated_at
		  FROM tasks
		 WHERE id = $1
		 FOR UPDATE`, failed.TaskID,
	).Scan(
		&task.ID, &task.TaskNumber, &task.ChannelID, &task.Title, &task.Description,
		&task.Status, &currentClaimer, &task.Priority, &task.DueDate,
		&task.MessageID, &task.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if (task.Status != TaskStatusTodo && task.Status != TaskStatusInProgress) ||
		(currentClaimer != "" && currentClaimer != failed.AgentID) ||
		task.UpdatedAt.After(failed.FinishedAt) {
		return nil
	}

	var latestRunID string
	if err := tx.QueryRow(ctx, `
		SELECT r.id::text
		  FROM agent_runs r
		  JOIN agent_run_task_links link ON link.run_id = r.id AND link.role = $2
		 WHERE link.task_id = $1
		 ORDER BY r.started_at DESC, r.id DESC
		 LIMIT 1`, failed.TaskID, AgentRunTaskRolePrimary,
	).Scan(&latestRunID); err != nil || latestRunID != failed.RunID {
		return err
	}
	var handled bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM agent_run_events
		   WHERE run_id = $1 AND type = ANY($2)
		)`, failed.RunID, []string{AgentRunEventTaskReassigned, AgentRunEventTaskRetryExhausted},
	).Scan(&handled); err != nil {
		return err
	}
	if handled {
		return nil
	}

	var attempts int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM agent_runs r
		  JOIN agent_run_task_links link ON link.run_id = r.id AND link.role = $2
		 WHERE link.task_id = $1
		   AND (
		     SELECT payload->>'result_contract'
		       FROM agent_run_events
		      WHERE run_id = r.id AND type = $3
		      ORDER BY seq
		      LIMIT 1
		   ) = $4`,
		failed.TaskID, AgentRunTaskRolePrimary, AgentRunEventRunStarted, agentResultContractVisibleMessage,
	).Scan(&attempts); err != nil {
		return err
	}

	if attempts >= maxTaskRunAttempts {
		task.Status = TaskStatusTodo
		task.ClaimerID = ""
		task.ClaimerName = ""
		if err := tx.QueryRow(ctx, `
			UPDATE tasks
			   SET status = $2, claimer_id = NULL, updated_at = now()
			 WHERE id = $1
			 RETURNING updated_at`, task.ID, TaskStatusTodo,
		).Scan(&task.UpdatedAt); err != nil {
			return err
		}
		if err := appendTaskRetryEvent(ctx, tx, failed.RunID, AgentRunEventTaskRetryExhausted, map[string]any{
			"attempts": maxTaskRunAttempts,
		}); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		s.broadcastTaskClaimed(&task, task.ChannelID)
		return s.persistTaskRetryMessage(ctx, &task,
			fmt.Sprintf("Task #%d 自动重派已尝试 %d 次，已退回 TODO 等待处理。", task.TaskNumber, maxTaskRunAttempts),
			maxTaskRunAttempts, true)
	}

	var nextAgentID, nextAgentName string
	if err := tx.QueryRow(ctx, `
		SELECT a.id::text, a.name
		  FROM channel_members cm
		  JOIN agents a ON a.id = cm.member_id
		 WHERE cm.channel_id = $1
		   AND cm.member_type = 'agent'
		   AND a.is_active = true
		 ORDER BY (a.id = $2), cm.joined_at, a.created_at, a.id
		 LIMIT 1`, task.ChannelID, failed.AgentID,
	).Scan(&nextAgentID, &nextAgentName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.dm.SelectDaemon("llm") == nil {
		return nil
	}

	previousStatus := task.Status
	previousClaimer := currentClaimer
	task.Status = TaskStatusInProgress
	task.ClaimerID = nextAgentID
	task.ClaimerName = nextAgentName
	if err := tx.QueryRow(ctx, `
		UPDATE tasks
		   SET status = $2, claimer_id = $3, updated_at = now()
		 WHERE id = $1
		 RETURNING updated_at`, task.ID, TaskStatusInProgress, nextAgentID,
	).Scan(&task.UpdatedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if !s.TriggerAgentForTask(ctx, task.ChannelID, task.ID, nextAgentID, task.TaskNumber, task.Title, task.Description, nil, nil) {
		_, _ = s.pool.Exec(ctx, `
			UPDATE tasks
			   SET status = $2, claimer_id = $3, updated_at = now()
			 WHERE id = $1 AND claimer_id = $4`,
			task.ID, previousStatus, nullableStr(previousClaimer), nextAgentID,
		)
		return nil
	}

	nextAttempt := attempts + 1
	runSvc := NewAgentRunService(s.pool)
	_, err = runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   failed.RunID,
		Type:    AgentRunEventTaskReassigned,
		Message: "task automatically reassigned",
		Payload: map[string]any{
			"attempt":       nextAttempt,
			"max_attempts":  maxTaskRunAttempts,
			"next_agent_id": nextAgentID,
		},
	})
	if err != nil {
		return err
	}
	s.broadcastTaskClaimed(&task, task.ChannelID)
	return s.persistTaskRetryMessage(ctx, &task,
		fmt.Sprintf("本次尝试未成功交付，Solo 正在自动改派（第 %d/%d 次）：@%s", nextAttempt, maxTaskRunAttempts, nextAgentName),
		nextAttempt, false)
}

func appendTaskRetryEvent(ctx context.Context, tx pgx.Tx, runID, eventType string, payload any) error {
	raw, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO agent_run_events (id, run_id, seq, type, message, payload)
		SELECT $1, $2, COALESCE(MAX(seq), 0) + 1, $3, $4, $5
		  FROM agent_run_events
		 WHERE run_id = $2`,
		uuid.NewString(), runID, eventType, eventType, raw,
	)
	return err
}

func (s *AgentService) persistTaskRetryMessage(ctx context.Context, task *Task, content string, attempt int, exhausted bool) error {
	messageID := uuid.NewString()
	now := time.Now()
	metadata := map[string]any{
		"task_id":       task.ID,
		"auto_reassign": true,
		"attempt":       attempt,
		"exhausted":     exhausted,
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, metadata, created_at, updated_at)
		VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $5, $5)`,
		messageID, task.ChannelID, content, metadata, now,
	); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.BroadcastToChannel(task.ChannelID, realtime.Envelope("message.new", map[string]any{
			"id":           messageID,
			"channel_id":   task.ChannelID,
			"sender_type":  "system",
			"sender_id":    "system",
			"sender_name":  "Solo",
			"content":      content,
			"content_type": "system",
			"metadata":     metadata,
			"created_at":   now.UTC().Format(time.RFC3339),
		}))
	}
	return nil
}
