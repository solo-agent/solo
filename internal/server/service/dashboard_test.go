package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestGetDashboardLiveUsesLatestStartedRunStatusWhenIdle(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	otherOwnerID := agentRunUser(t, pool)
	otherAgentID := agentRunAgent(t, pool, otherOwnerID)
	var runIDs []string
	t.Cleanup(func() {
		if len(runIDs) > 0 {
			_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = ANY($1)`, runIDs)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, otherAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, otherOwnerID)
	})

	svc := NewAgentRunService(pool)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "claude",
		ExternalSessionID: "live-session",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	oldRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		Status:       AgentRunStatusTimeout,
		ActivityText: "旧 run 超时",
	})
	if err != nil {
		t.Fatalf("StartRun old: %v", err)
	}
	newRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		SessionID:    session.ID,
		TriggerType:  AgentRunTriggerMessage,
		Status:       AgentRunStatusCompleted,
		ActivityText: "新 run 完成",
	})
	if err != nil {
		t.Fatalf("StartRun new: %v", err)
	}
	otherRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      otherAgentID,
		TriggerType:  AgentRunTriggerMessage,
		Status:       AgentRunStatusRunning,
		ActivityText: "other owner",
	})
	if err != nil {
		t.Fatalf("StartRun other: %v", err)
	}
	runIDs = append(runIDs, oldRun.ID, newRun.ID, otherRun.ID)

	base := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $3 WHERE id = $1`, oldRun.ID, base, base.Add(10*time.Minute)); err != nil {
		t.Fatalf("adjust old run times: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, newRun.ID, base.Add(time.Minute)); err != nil {
		t.Fatalf("adjust new run times: %v", err)
	}

	live, err := svc.GetDashboardLive(ctx, ownerID)
	if err != nil {
		t.Fatalf("GetDashboardLive: %v", err)
	}
	foundCurrent := false
	for _, group := range live.Groups {
		for _, item := range group.Items {
			if item.AgentID == otherAgentID {
				t.Fatalf("live included other owner agent: %+v", item)
			}
			if item.AgentID == agentID {
				if item.RunID != newRun.ID || item.Status != AgentRunStatusCompleted {
					t.Fatalf("live item = %+v, want latest started completed run %s", item, newRun.ID)
				}
				if item.SessionID != session.ID {
					t.Fatalf("live item session_id = %q, want %q", item.SessionID, session.ID)
				}
				foundCurrent = true
			}
		}
	}
	if !foundCurrent {
		t.Fatal("agent not found in live dashboard")
	}
}

func TestGetDashboardLivePrioritizesExecutingRunOverNewerQueue(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	executing, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerTask,
		Status:       AgentRunStatusQueued,
		ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun executing: %v", err)
	}
	executing, err = svc.MarkBackendStarted(ctx, executing.ID)
	if err != nil {
		t.Fatalf("MarkBackendStarted: %v", err)
	}
	queued, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerTask,
		Status:       AgentRunStatusQueued,
		ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun queued: %v", err)
	}
	base := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, executing.ID, base); err != nil {
		t.Fatalf("adjust executing run time: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, queued.ID, base.Add(time.Minute)); err != nil {
		t.Fatalf("adjust queued run time: %v", err)
	}

	live, err := svc.GetDashboardLive(ctx, ownerID)
	if err != nil {
		t.Fatalf("GetDashboardLive: %v", err)
	}
	for _, group := range live.Groups {
		for _, item := range group.Items {
			if item.AgentID != agentID {
				continue
			}
			if item.RunID != executing.ID || item.Status != AgentRunStatusRunning {
				t.Fatalf("live item = %+v, want executing run %s instead of newer queue %s", item, executing.ID, queued.ID)
			}
			if item.ActiveCount != 2 || item.RunCount != 2 {
				t.Fatalf("live counts = active %d runs %d, want 2 and 2", item.ActiveCount, item.RunCount)
			}
			return
		}
	}
	t.Fatal("agent not found in live dashboard")
}

func TestGetDashboardInsightFiltersAgentsAndReadsTranscriptUsage(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	idleAgentID := agentRunAgent(t, pool, ownerID)
	otherOwnerID := agentRunUser(t, pool)
	otherAgentID := agentRunAgent(t, pool, otherOwnerID)
	var runIDs []string
	var channelID string
	t.Cleanup(func() {
		if len(runIDs) > 0 {
			_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = ANY($1)`, runIDs)
		}
		if channelID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
			_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
			_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, otherAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, idleAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, otherAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, otherOwnerID)
	})

	svc := NewAgentRunService(pool)
	base := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	channelID = agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	taskID := agentRunTask(t, pool, channelID, ownerID)
	if _, err := pool.Exec(ctx, `UPDATE messages SET content = 'alpha alpha beta target', created_at = $2, updated_at = $2 WHERE id = $1`, messageID, base); err != nil {
		t.Fatalf("adjust message: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET updated_at = $2 WHERE id = $1`, taskID, base); err != nil {
		t.Fatalf("adjust task: %v", err)
	}
	transcriptPath := dashboardTranscriptFile(t, base.Add(time.Second), 321, 45)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "claude",
		ExternalSessionID: "dashboard-session",
		TranscriptPath:    transcriptPath,
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		SessionID:    session.ID,
		TriggerType:  AgentRunTriggerMessage,
		Status:       AgentRunStatusCompleted,
		ActivityText: "done",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	otherRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      otherAgentID,
		TriggerType:  AgentRunTriggerMessage,
		Status:       AgentRunStatusCompleted,
		ActivityText: "other done",
		Usage: map[string]any{
			"input_tokens":  999,
			"output_tokens": 999,
		},
	})
	if err != nil {
		t.Fatalf("StartRun other: %v", err)
	}
	runIDs = append(runIDs, run.ID, otherRun.ID)
	if _, err := pool.Exec(ctx,
		`UPDATE agent_runs SET started_at = $2, updated_at = $2, finished_at = $3 WHERE id = $1`,
		run.ID, base, base.Add(2*time.Second),
	); err != nil {
		t.Fatalf("adjust run times: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE agent_runs SET started_at = $2, updated_at = $2, finished_at = $3 WHERE id = $1`,
		otherRun.ID, base, base.Add(2*time.Second),
	); err != nil {
		t.Fatalf("adjust other run times: %v", err)
	}

	insight, err := svc.GetDashboardInsight(ctx, ownerID, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetDashboardInsight: %v", err)
	}
	if insight.AgentRuns != 1 {
		t.Fatalf("agent_runs = %d, want 1", insight.AgentRuns)
	}
	if insight.Tokens.Input != 321 || insight.Tokens.Output != 45 || insight.Tokens.Total != 366 {
		t.Fatalf("tokens = %+v, want transcript usage", insight.Tokens)
	}
	if len(insight.AgentUsage) != 1 || insight.AgentUsage[0].ID != agentID {
		t.Fatalf("agent_usage = %+v, want only current owner agent %s", insight.AgentUsage, agentID)
	}
	day := base.Format("2006-01-02")
	var point *DashboardSeriesPoint
	for i := range insight.Series {
		if insight.Series[i].Date == day {
			point = &insight.Series[i]
			break
		}
	}
	if point == nil || point.AgentRuns != 1 || point.Messages < 1 || point.Tasks < 1 || point.Tokens != 366 {
		t.Fatalf("series[%s] = %+v, want messages/tasks/run/tokens", day, point)
	}
	foundAlpha := false
	for _, term := range insight.Terms {
		if term.Key == "alpha" && term.Count >= 2 {
			foundAlpha = true
			break
		}
	}
	if !foundAlpha {
		t.Fatalf("terms = %+v, want alpha x2", insight.Terms)
	}

	todayInsight, err := svc.GetDashboardInsight(ctx, ownerID, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("GetDashboardInsight today: %v", err)
	}
	if len(todayInsight.Series) == 0 {
		t.Fatal("today insight series is empty")
	}
}

func TestDashboardRunUsageIgnoresUnreadableTranscript(t *testing.T) {
	usage, err := dashboardRunUsage(nil, t.TempDir(), time.Now().UTC(), sql.NullTime{})
	if err != nil {
		t.Fatalf("dashboardRunUsage: %v", err)
	}
	if usage.Total != 0 {
		t.Fatalf("usage = %+v, want empty usage", usage)
	}
}

func dashboardTranscriptFile(t *testing.T, ts time.Time, inputTokens, outputTokens int64) string {
	t.Helper()
	path := t.TempDir() + "/dashboard-session.jsonl"
	raw := fmt.Sprintf(
		`{"type":"assistant","timestamp":%q,"message":{"usage":{"input_tokens":%d,"output_tokens":%d},"content":[{"type":"text","text":"done"}]}}`+"\n",
		ts.Format(time.RFC3339), inputTokens, outputTokens,
	)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write dashboard transcript: %v", err)
	}
	return path
}
