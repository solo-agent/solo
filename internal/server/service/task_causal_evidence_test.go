package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestTaskMutationRecordsCausalEvidenceAtomically(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner'), ($1, 'agent', $3, 'member')`, channelID, ownerID, agentID); err != nil {
		t.Fatalf("add members: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	taskSvc := NewTaskService(pool)
	task, err := taskSvc.CreateTask(ctx, channelID, ownerID, TaskCreateRequest{Title: "causal task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerTask, ChannelID: channelID,
		Status: AgentRunStatusRunning, ActivityText: "working",
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	claimCtx := WithTaskActionOrigin(ctx, TaskActionOrigin{RunID: run.ID, ActorID: agentID, Action: "claim"})
	claimed, err := taskSvc.ClaimTask(claimCtx, channelID, task.ID, agentID)
	if err != nil {
		t.Fatalf("claim task: %v", err)
	}
	if claimed.Status != TaskStatusInProgress || claimed.ClaimerID != agentID {
		t.Fatalf("claimed task = %+v", claimed)
	}
	var actionCount, linkCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_task_actions WHERE run_id = $1 AND task_id = $2 AND action = 'claim'`, run.ID, task.ID).Scan(&actionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_task_links WHERE run_id = $1 AND task_id = $2 AND role = 'executor'`, run.ID, task.ID).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if actionCount != 1 || linkCount != 1 {
		t.Fatalf("action_count=%d link_count=%d, want 1/1", actionCount, linkCount)
	}
}

func TestTaskMutationRollsBackWhenCausalEvidenceCannotBeStored(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner'), ($1, 'agent', $3, 'member')`, channelID, ownerID, agentID); err != nil {
		t.Fatalf("add members: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	taskSvc := NewTaskService(pool)
	task, err := taskSvc.CreateTask(ctx, channelID, ownerID, TaskCreateRequest{Title: "rollback task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	badRunID := uuid.NewString()
	claimCtx := WithTaskActionOrigin(ctx, TaskActionOrigin{RunID: badRunID, ActorID: agentID, Action: "claim"})
	if _, err := taskSvc.ClaimTask(claimCtx, channelID, task.ID, agentID); err == nil {
		t.Fatal("claim unexpectedly succeeded with missing origin run")
	}

	unchanged, err := taskSvc.GetTask(ctx, channelID, task.ID, ownerID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if unchanged.Status != TaskStatusTodo || unchanged.ClaimerID != "" {
		t.Fatalf("task mutation was not rolled back: %+v", unchanged)
	}
	var actionCount, linkCount int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_task_actions WHERE task_id = $1`, task.ID).Scan(&actionCount)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_task_links WHERE task_id = $1`, task.ID).Scan(&linkCount)
	if actionCount != 0 || linkCount != 0 {
		t.Fatalf("partial evidence remained: action_count=%d link_count=%d", actionCount, linkCount)
	}
}
