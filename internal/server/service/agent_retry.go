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
	RunID           string
	AgentID         string
	TaskID          string
	FinishedAt      time.Time
	FailureCode     string
	Retryable       bool
	ResumeSessionID string
}

const (
	taskRecoveryModeResumeSession = "resume_session"
	taskRecoveryModeFreshSession  = "fresh_session"
)

type taskRunRecovery struct {
	PreviousRunID   string `json:"previous_run_id"`
	Attempt         int    `json:"attempt"`
	MaxAttempts     int    `json:"max_attempts"`
	Mode            string `json:"mode"`
	WorkspaceReused bool   `json:"workspace_reused"`
	FailureCode     string `json:"failure_code"`
	ResumeSessionID string `json:"-"`
}

func (s *AgentService) retryFailedTaskRuns(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.agent_id::text, link.task_id::text, r.finished_at,
		       COALESCE(failure.payload->>'failure_code', ''),
		       COALESCE((failure.payload->>'retryable')::boolean, false),
		       COALESCE(sess.external_session_id, '')
		  FROM agent_runs r
		  JOIN agent_run_task_links link ON link.run_id = r.id AND link.role = $1
		  JOIN tasks t ON t.id = link.task_id
		  LEFT JOIN agent_sessions sess ON sess.id = r.session_id
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
		   AND (
		     SELECT payload->>'result_contract'
		       FROM agent_run_events
		      WHERE run_id = r.id AND type = $5
		      ORDER BY seq
		      LIMIT 1
		   ) = $6
		   AND NOT EXISTS (
		     SELECT 1
		       FROM agent_run_events
		      WHERE run_id = r.id AND type = ANY($7)
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
		AgentRunEventRunStarted,
		agentResultContractVisibleMessage,
		[]string{AgentRunEventTaskReassigned, AgentRunEventTaskRecoveryScheduled, AgentRunEventTaskRecoveryBlocked, AgentRunEventTaskRetryExhausted},
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var runs []retryableTaskRun
	for rows.Next() {
		var run retryableTaskRun
		if err := rows.Scan(&run.RunID, &run.AgentID, &run.TaskID, &run.FinishedAt, &run.FailureCode, &run.Retryable, &run.ResumeSessionID); err != nil {
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
		)`, failed.RunID, []string{AgentRunEventTaskReassigned, AgentRunEventTaskRecoveryScheduled, AgentRunEventTaskRecoveryBlocked, AgentRunEventTaskRetryExhausted},
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

	mode, canRecover := automaticTaskRecoveryMode(failed)
	if attempts >= maxTaskRunAttempts || !canRecover {
		eventType := AgentRunEventTaskRecoveryBlocked
		exhausted := false
		if attempts >= maxTaskRunAttempts {
			eventType = AgentRunEventTaskRetryExhausted
			exhausted = true
		}
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
		if err := appendTaskRetryEvent(ctx, tx, failed.RunID, eventType, map[string]any{
			"attempts":       attempts,
			"max_attempts":   maxTaskRunAttempts,
			"failure_code":   failed.FailureCode,
			"next_owner":     "task_creator",
			"recovery_state": "needs_human",
		}); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		s.broadcastTaskClaimed(&task, task.ChannelID)
		if exhausted {
			return s.persistTaskRetryMessage(ctx, &task,
				fmt.Sprintf("Task #%d 已自动恢复 %d 次，现已退回待处理。下一步由任务创建者决定。", task.TaskNumber, attempts),
				attempts, true)
		}
		return s.persistTaskRetryMessage(ctx, &task,
			fmt.Sprintf("Task #%d 因%s未自动恢复，现已退回待处理。下一步由任务创建者决定。", task.TaskNumber, taskFailureName(failed.FailureCode)),
			attempts, false)
	}

	var agentName string
	if err := tx.QueryRow(ctx, `
		SELECT name FROM agents WHERE id = $1 AND is_active = true`, failed.AgentID,
	).Scan(&agentName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if _, err := s.dm.ResolveDaemonForAgent(ctx, failed.AgentID, "llm"); err != nil {
		return nil
	}

	task.Status = TaskStatusInProgress
	task.ClaimerID = failed.AgentID
	task.ClaimerName = agentName
	if err := tx.QueryRow(ctx, `
		UPDATE tasks
		   SET status = $2, claimer_id = $3, updated_at = now()
		 WHERE id = $1
		 RETURNING updated_at`, task.ID, TaskStatusInProgress, failed.AgentID,
	).Scan(&task.UpdatedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	nextAttempt := attempts + 1
	recovery := &taskRunRecovery{
		PreviousRunID:   failed.RunID,
		Attempt:         nextAttempt,
		MaxAttempts:     maxTaskRunAttempts,
		Mode:            mode,
		WorkspaceReused: true,
		FailureCode:     failed.FailureCode,
		ResumeSessionID: failed.ResumeSessionID,
	}
	if !s.triggerAgentForTask(ctx, task.ChannelID, task.ID, failed.AgentID, task.TaskNumber, task.Title, task.Description, nil, nil, recovery) {
		if _, err := s.pool.Exec(ctx, `
			UPDATE tasks
			   SET status = $2, claimer_id = NULL, updated_at = now()
			 WHERE id = $1 AND claimer_id = $3`,
			task.ID, TaskStatusTodo, failed.AgentID,
		); err != nil {
			return err
		}
		task.Status = TaskStatusTodo
		task.ClaimerID = ""
		task.ClaimerName = ""
		runSvc := NewAgentRunService(s.pool)
		if _, err := runSvc.AppendEvent(ctx, AppendRunEventInput{
			RunID:   failed.RunID,
			Type:    AgentRunEventTaskRecoveryBlocked,
			Message: "task recovery could not start",
			Payload: map[string]any{
				"attempts": attempts, "max_attempts": maxTaskRunAttempts,
				"failure_code": failed.FailureCode, "next_owner": "task_creator",
				"recovery_state": "needs_human",
			},
		}); err != nil {
			return err
		}
		s.broadcastTaskClaimed(&task, task.ChannelID)
		return s.persistTaskRetryMessage(ctx, &task,
			fmt.Sprintf("Task #%d 自动恢复未能启动，现已退回待处理。下一步由任务创建者决定。", task.TaskNumber),
			attempts, false)
	}

	runSvc := NewAgentRunService(s.pool)
	_, err = runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   failed.RunID,
		Type:    AgentRunEventTaskRecoveryScheduled,
		Message: "task recovery scheduled",
		Payload: recovery,
	})
	if err != nil {
		return err
	}
	s.broadcastTaskClaimed(&task, task.ChannelID)
	return s.persistTaskRetryMessage(ctx, &task,
		fmt.Sprintf("本次执行因%s中断，Solo 正由原 Agent @%s 自动恢复（第 %d/%d 次）。", taskFailureName(failed.FailureCode), agentName, nextAttempt, maxTaskRunAttempts),
		nextAttempt, false)
}

func automaticTaskRecoveryMode(failed retryableTaskRun) (string, bool) {
	if !failed.Retryable {
		return "", false
	}
	switch failed.FailureCode {
	case agentFailureDaemonLost, agentFailureTimeout, agentFailureProviderTransient:
		return taskRecoveryModeResumeSession, true
	case agentFailureMissingVisibleResult:
		return taskRecoveryModeFreshSession, true
	default:
		return "", false
	}
}

func taskFailureName(code string) string {
	switch code {
	case agentFailureDaemonLost:
		return "本机运行程序断线"
	case agentFailureTimeout:
		return "执行超时"
	case agentFailureProviderTransient:
		return "模型服务临时故障"
	case agentFailureMissingVisibleResult:
		return "没有最终交付"
	case agentFailureConfiguration:
		return "配置问题"
	default:
		return "未知故障"
	}
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
		"auto_recovery": true,
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
