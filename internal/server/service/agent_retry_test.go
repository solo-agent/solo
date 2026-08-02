package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAutomaticTaskRetryExhaustionReturnsTaskToTodoOnce(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	taskID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, task_number, channel_id, creator_id, title, status, claimer_id)
		VALUES ($1, 1, $2, $3, 'retry exhaustion', $4, $5)`,
		taskID, channelID, ownerID, TaskStatusInProgress, agentID,
	)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	runSvc := NewAgentRunService(pool)
	var lastRunID string
	for range maxTaskRunAttempts {
		run, err := runSvc.StartRun(ctx, StartRunInput{
			AgentID:      agentID,
			TriggerType:  AgentRunTriggerTask,
			ChannelID:    channelID,
			Status:       AgentRunStatusRunning,
			ActivityText: agentActivityAccepted,
		})
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		lastRunID = run.ID
		if err := runSvc.LinkTask(ctx, LinkRunTaskInput{
			RunID: run.ID, TaskID: taskID, Role: AgentRunTaskRolePrimary, Confidence: 1,
		}); err != nil {
			t.Fatalf("LinkTask: %v", err)
		}
		if _, err := runSvc.AppendEvent(ctx, AppendRunEventInput{
			RunID: run.ID,
			Type:  AgentRunEventRunStarted,
			Payload: map[string]any{
				"result_contract": agentResultContractVisibleMessage,
			},
		}); err != nil {
			t.Fatalf("append run_started: %v", err)
		}
		if _, err := runSvc.FinishRun(ctx, FinishRunInput{
			RunID: run.ID, Status: AgentRunStatusFailed, ActivityText: agentActivityFailed,
		}); err != nil {
			t.Fatalf("FinishRun: %v", err)
		}
		if _, err := runSvc.AppendEvent(ctx, AppendRunEventInput{
			RunID: run.ID,
			Type:  AgentRunEventError,
			Payload: map[string]any{
				"failure_code": agentFailureMissingVisibleResult,
				"retryable":    true,
			},
		}); err != nil {
			t.Fatalf("append error: %v", err)
		}
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	if err := svc.retryFailedTaskRuns(ctx); err != nil {
		t.Fatalf("retryFailedTaskRuns: %v", err)
	}
	if err := svc.retryFailedTaskRuns(ctx); err != nil {
		t.Fatalf("second retryFailedTaskRuns: %v", err)
	}

	var status, claimerID string
	if err := pool.QueryRow(ctx, `
		SELECT status, COALESCE(claimer_id::text, '') FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&status, &claimerID); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != TaskStatusTodo || claimerID != "" {
		t.Fatalf("task = status %q claimer %q, want todo/unclaimed", status, claimerID)
	}

	var messageCount, eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages
		 WHERE metadata->>'task_id' = $1 AND metadata->>'exhausted' = 'true'`,
		taskID,
	).Scan(&messageCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_run_events
		 WHERE run_id = $1 AND type = $2`,
		lastRunID, AgentRunEventTaskRetryExhausted,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if messageCount != 1 || eventCount != 1 {
		t.Fatalf("exhaustion artifacts = messages %d events %d, want 1/1", messageCount, eventCount)
	}
}
