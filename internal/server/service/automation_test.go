package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNextScheduledAtUsesTimezoneAndFrequency(t *testing.T) {
	t.Run("daily uses local time", func(t *testing.T) {
		got, err := nextScheduledAt(AutomationInput{
			ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, ScheduleMinute: 0, Timezone: "Asia/Shanghai",
		}, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("next run = %s, want %s", got, want)
		}
	})

	t.Run("weekdays skip weekend", func(t *testing.T) {
		got, err := nextScheduledAt(AutomationInput{
			ScheduleType: AutomationScheduleWeekdays, ScheduleHour: 9, ScheduleMinute: 0, Timezone: "UTC",
		}, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) // Friday
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("next run = %s, want %s", got, want)
		}
	})

	t.Run("weekly selects chosen day", func(t *testing.T) {
		monday := int(time.Monday)
		got, err := nextScheduledAt(AutomationInput{
			ScheduleType: AutomationScheduleWeekly, ScheduleHour: 9, ScheduleMinute: 30,
			ScheduleWeekday: &monday, Timezone: "UTC",
		}, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)) // Tuesday
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 8, 17, 9, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("next run = %s, want %s", got, want)
		}
	})
}

func TestAutomationRunNowRecordsAlreadyRunningWithoutDuplicateTask(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(context.Background(), channelID, userID, AutomationInput{
		Name: "daily check", TaskTitle: "check competitors", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activeID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO automation_runs (id, automation_id, source, status, scheduled_for)
		VALUES ($1,$2,'manual','running',now())`, activeID, item.ID); err != nil {
		t.Fatal(err)
	}

	run, err := svc.RunNow(context.Background(), channelID, item.ID, userID)
	if !errors.Is(err, ErrAutomationAlreadyActive) {
		t.Fatalf("RunNow error = %v, want already active", err)
	}
	if run == nil || run.Status != AutomationRunSkipped || run.CoalescedIntoRunID != activeID {
		t.Fatalf("unexpected skipped run: %#v", run)
	}
	var runCount, taskCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM automation_runs WHERE automation_id=$1`, item.ID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM tasks WHERE channel_id=$1`, channelID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 2 || taskCount != 0 {
		t.Fatalf("runs=%d tasks=%d, want 2 runs and no duplicate task", runCount, taskCount)
	}
}

func TestAutomationTickCollapsesMissedOccurrencesToOne(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(context.Background(), channelID, userID, AutomationInput{
		Name: "missed check", TaskTitle: "check once", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE automations SET next_run_at=now()-interval '30 days' WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}

	svc.Tick(context.Background())
	svc.Tick(context.Background())
	var runCount int
	var next time.Time
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM automation_runs WHERE automation_id=$1`, item.ID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT next_run_at FROM automations WHERE id=$1`, item.ID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("missed schedule created %d runs, want exactly 1", runCount)
	}
	if !next.After(time.Now()) {
		t.Fatalf("next_run_at = %s, want future time", next)
	}
}

func TestAutomationCompletesAfterAgentFinishesWhileTaskWaitsForReview(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	ctx := context.Background()
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(ctx, channelID, userID, AutomationInput{
		Name: "daily review", TaskTitle: "prepare report", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := automationRunFixture(t, pool, item.ID, channelID, userID, agentID)
	runSvc := NewAgentRunService(pool)
	agentRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerTask, ChannelID: channelID,
		Status: AgentRunStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: agentRun.ID, TaskID: taskID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runSvc.FinishRun(ctx, FinishRunInput{RunID: agentRun.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileRuns(ctx); err != nil {
		t.Fatal(err)
	}
	var automationStatus, taskStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM automation_runs WHERE automation_id=$1`, item.ID).Scan(&automationStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM tasks WHERE id=$1`, taskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if automationStatus != AutomationRunCompleted || taskStatus != TaskStatusInReview {
		t.Fatalf("automation=%q task=%q, want completed/in_review", automationStatus, taskStatus)
	}
}

func TestAutomationStaysActiveWhileAnyAgentAttemptIsRunning(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	ctx := context.Background()
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(ctx, channelID, userID, AutomationInput{
		Name: "retrying report", TaskTitle: "prepare report", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := automationRunFixture(t, pool, item.ID, channelID, userID, agentID)
	runSvc := NewAgentRunService(pool)
	completed, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerTask, ChannelID: channelID,
		Status: AgentRunStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: completed.ID, TaskID: taskID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runSvc.FinishRun(ctx, FinishRunInput{RunID: completed.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	active, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerTask, ChannelID: channelID,
		Status: AgentRunStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: active.ID, TaskID: taskID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatal(err)
	}

	if err := svc.reconcileRuns(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM automation_runs WHERE automation_id=$1`, item.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != AutomationRunRunning {
		t.Fatalf("automation status=%q, want running while retry is active", status)
	}
}

func TestAutomationAllowsNextTriggerAfterCompletedAgentBeforeReview(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	ctx := context.Background()
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(ctx, channelID, userID, AutomationInput{
		Name: "next report", TaskTitle: "prepare report", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstTaskID := automationRunFixture(t, pool, item.ID, channelID, userID, agentID)
	runSvc := NewAgentRunService(pool)
	agentRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerTask, ChannelID: channelID,
		Status: AgentRunStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: agentRun.ID, TaskID: firstTaskID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runSvc.FinishRun(ctx, FinishRunInput{RunID: agentRun.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}

	run, err := svc.RunNow(ctx, channelID, item.ID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.Status != AutomationRunSkipped || run.FailureReason != "target_unavailable" {
		t.Fatalf("next run = %#v, want target availability check instead of already running", run)
	}
	var firstTaskStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tasks WHERE id=$1`, firstTaskID).Scan(&firstTaskStatus); err != nil {
		t.Fatal(err)
	}
	if firstTaskStatus != TaskStatusInReview {
		t.Fatalf("first task status=%q, want in_review", firstTaskStatus)
	}
}

func TestAutomationPausesAfterThreeRelevantFailures(t *testing.T) {
	pool := automationTestPool(t)
	userID, channelID, agentID := automationTestFixture(t, pool)
	svc := NewAutomationService(pool, NewTaskService(pool), nil, nil)
	item, err := svc.Create(context.Background(), channelID, userID, AutomationInput{
		Name: "pause after failures", TaskTitle: "check", TargetAgentID: agentID,
		ScheduleType: AutomationScheduleDaily, ScheduleHour: 9, Timezone: "UTC", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"target_unavailable", "already_running", "dispatch_failed", "task_returned_to_human"} {
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO automation_runs (automation_id, source, status, scheduled_for, failure_reason, completed_at)
			VALUES ($1,'manual','skipped',now(),$2,now())`, item.ID, reason); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.pauseRepeatedFailures(context.Background()); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var next *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT enabled,next_run_at FROM automations WHERE id=$1`, item.ID).Scan(&enabled, &next); err != nil {
		t.Fatal(err)
	}
	if enabled || next != nil {
		t.Fatalf("automation enabled=%v next=%v, want auto-paused after three failures", enabled, next)
	}
}

func automationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: %v", err)
	}
	var table *string
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('public.automations')::text`).Scan(&table); err != nil || table == nil {
		pool.Close()
		t.Skip("skipping DB test until automation migration is applied")
	}
	t.Cleanup(pool.Close)
	return pool
}

func automationTestFixture(t *testing.T, pool *pgxpool.Pool) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	channelID := uuid.NewString()
	agentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,display_name,password_hash) VALUES ($1,$2,'Automation Tester','test')`, userID, fmt.Sprintf("automation-%s@example.test", userID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channels (id,name,created_by) VALUES ($1,$2,$3)`, channelID, "automation-"+channelID[:8], userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id,member_type,member_id,role) VALUES ($1,'user',$2,'owner')`, channelID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id,name,owner_id,model_name,home_channel_id) VALUES ($1,$2,$3,'test-model',$4)`, agentID, "automation-agent-"+agentID[:8], userID, channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id,member_type,member_id) VALUES ($1,'agent',$2)`, channelID, agentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id=$1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id=$1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	return userID, channelID, agentID
}

func automationRunFixture(t *testing.T, pool *pgxpool.Pool, automationID, channelID, userID, agentID string) string {
	t.Helper()
	taskID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO tasks (id, task_number, channel_id, creator_id, title, status, claimer_id, priority)
		VALUES ($1,1,$2,$3,'automation test','in_progress',$4,'normal')`,
		taskID, channelID, userID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO automation_runs (automation_id, source, status, scheduled_for, task_id)
		VALUES ($1,'manual','running',now(),$2)`, automationID, taskID); err != nil {
		t.Fatal(err)
	}
	return taskID
}
