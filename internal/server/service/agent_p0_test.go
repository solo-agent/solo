package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

func TestTriggerAgentResponseBroadcastsErrorWhenNoDaemonAvailable(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	_, err := pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'agent', $2)`,
		channelID, agentID,
	)
	if err != nil {
		t.Fatalf("add agent member: %v", err)
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	svc.TriggerAgentResponse(ctx, channelID, messageID, "user", ownerID, []string{agentID}, true, nil)

	if !rec.hasChannelEvent(channelID, "agent.error", agentErrorNoAvailableDaemon) {
		t.Fatalf("agent.error with no available daemon not broadcast: %q", rec.channelMessages[channelID])
	}
}

func TestStreamingAgentTaskStaysQueuedWithoutBackendStart(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	taskID, server := streamingTestDaemon(t)
	defer server.Close()
	rec := newRecordingBroadcaster()
	dm := NewDaemonManager(pool, rec)
	daemon := daemonInfoForTest(t, server.URL, "daemon-"+agentID[:8])
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, rec, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		TaskID:           taskID,
		AgentID:          agentID,
		ChannelID:        channelID,
		TriggerMessageID: messageID,
		ModelConfig:      agentModelConfigForTest(),
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	joined := strings.Join(rec.broadcastMessages, "\n")
	if !strings.Contains(joined, `"type":"agent.run.started"`) || !strings.Contains(joined, `"status":"queued"`) {
		t.Fatalf("queued agent.run.started not broadcast: %q", rec.broadcastMessages)
	}
	if rec.hasBroadcastEvent("agent.run.updated", agentActivityAccepted) {
		t.Fatalf("run was acknowledged before backend start: %q", rec.broadcastMessages)
	}
}

func TestAgentRunWatchdogsWarnOnce(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusThinking,
		ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	old := time.Now().Add(-agentNoVisibleReplyAfter - time.Second)
	_, err = pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, backend_started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, old)
	if err != nil {
		t.Fatalf("age run: %v", err)
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs: %v", err)
	}
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs second: %v", err)
	}

	assertRunEventCount(t, pool, run.ID, agentRunEventNoVisibleReplyWatchdog, 1)
	if !rec.hasBroadcastEvent("agent.run.updated", agentActivityNoVisibleReply) {
		t.Fatalf("no-visible watchdog update not broadcast: %q", rec.broadcastMessages)
	}
}

func TestAgentRunWatchdogsTimeoutOrphanedQueuedRun(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: channelID,
		Status: AgentRunStatusQueued, ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	old := time.Now().Add(-agentRunQueueTimeout - time.Minute)
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, old); err != nil {
		t.Fatalf("age queued run: %v", err)
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs: %v", err)
	}
	current, err := NewAgentRunService(pool).GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != AgentRunStatusTimeout || current.BackendStartedAt != nil || current.FinishedAt == nil {
		t.Fatalf("orphaned queued run did not converge to timeout: %+v", current)
	}
	if !rec.hasBroadcastEvent("agent.run.finished", agentActivityQueueTimeout) {
		t.Fatalf("queued timeout finish not broadcast: %q", rec.broadcastMessages)
	}
}

func TestAgentRunProgressWatchdogWarnsOnce(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusRunning,
		ActivityText: "调用 Bash",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	_, err = runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   run.ID,
		Type:    AgentRunEventAssistantMessage,
		Message: "visible reply already happened",
	})
	if err != nil {
		t.Fatalf("AppendEvent assistant: %v", err)
	}
	old := time.Now().Add(-agentNoProgressAfter - time.Second)
	_, err = pool.Exec(ctx, `UPDATE agent_runs SET backend_started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, old)
	if err != nil {
		t.Fatalf("age run: %v", err)
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs: %v", err)
	}
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs second: %v", err)
	}

	assertRunEventCount(t, pool, run.ID, agentRunEventNoProgressWatchdog, 1)
	if !rec.hasBroadcastEvent("agent.run.updated", agentActivityNoProgress) {
		t.Fatalf("progress watchdog update not broadcast: %q", rec.broadcastMessages)
	}
}

func TestAgentRunWatchdogTimesOutStaleActiveRun(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusThinking,
		ActivityText: agentActivityNoProgress,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	old := time.Now().Add(-agentRunExecutionTimeout - agentRunWatchdogInterval - time.Second)
	_, err = pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, backend_started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, old)
	if err != nil {
		t.Fatalf("age run: %v", err)
	}

	rec := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, rec), rec, nil)
	if err := svc.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		t.Fatalf("CheckAgentRunWatchdogs: %v", err)
	}

	var status string
	var finishedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, finished_at FROM agent_runs WHERE id = $1`, run.ID).Scan(&status, &finishedAt); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if status != string(AgentRunStatusTimeout) {
		t.Fatalf("status = %q, want timeout", status)
	}
	if finishedAt == nil {
		t.Fatal("finished_at is nil")
	}
	if !rec.hasBroadcastEvent("agent.run.finished", agentActivityTimeout) {
		t.Fatalf("timeout finish not broadcast: %q", rec.broadcastMessages)
	}
}

func TestDaemonOfflineTimesOutTrackedAgentRun(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusThinking,
		ActivityText: agentActivityAccepted,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	rec := newRecordingBroadcaster()
	dm := NewDaemonManager(pool, rec)
	daemonID := "daemon-" + agentID[:8]
	dm.Register(&DaemonInfo{ID: daemonID, Host: "127.0.0.1", Port: 1, Capabilities: []string{"agent"}})
	taskID := uuid.NewString()
	dm.TrackTask(taskID, daemonID, agentID)
	dm.AttachTaskRun(taskID, run.ID)

	dm.mu.Lock()
	dm.daemons[daemonID].LastHeartbeat = time.Now().Add(-dm.heartbeatInterval - time.Second)
	dm.daemons[daemonID].MissedHeartbeats = dm.maxMissedHB - 1
	dm.mu.Unlock()

	dm.checkHealth()

	var status string
	var finishedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, finished_at FROM agent_runs WHERE id = $1`, run.ID).Scan(&status, &finishedAt); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if status != string(AgentRunStatusTimeout) {
		t.Fatalf("status = %q, want timeout", status)
	}
	if finishedAt == nil {
		t.Fatal("finished_at is nil")
	}
	if !rec.hasBroadcastEvent("agent.run.finished", agentActivityTimeout) {
		t.Fatalf("offline timeout finish not broadcast: %q", rec.broadcastMessages)
	}
}

func TestDaemonUnregisterTimesOutTrackedAgentRun(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusThinking,
		ActivityText: agentActivityAccepted,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	rec := newRecordingBroadcaster()
	dm := NewDaemonManager(pool, rec)
	daemonID := "daemon-" + agentID[:8]
	dm.Register(&DaemonInfo{ID: daemonID, Host: "127.0.0.1", Port: 1, Capabilities: []string{"agent"}})
	taskID := uuid.NewString()
	dm.TrackTask(taskID, daemonID, agentID)
	dm.AttachTaskRun(taskID, run.ID)

	dm.Unregister(daemonID)

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_runs WHERE id = $1`, run.ID).Scan(&status); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if status != string(AgentRunStatusTimeout) {
		t.Fatalf("status = %q, want timeout", status)
	}
	if !rec.hasBroadcastEvent("agent.run.finished", agentActivityTimeout) {
		t.Fatalf("unregister timeout finish not broadcast: %q", rec.broadcastMessages)
	}
}

func assertRunEventCount(t *testing.T, pool *pgxpool.Pool, runID, eventType string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM agent_run_events WHERE run_id = $1 AND type = $2`,
		runID, eventType,
	).Scan(&count); err != nil {
		t.Fatalf("count run events: %v", err)
	}
	if count != want {
		t.Fatalf("event count for %s = %d, want %d", eventType, count, want)
	}
}

func agentModelConfigForTest() agent.ModelConfig {
	return agent.ModelConfig{Provider: "claude", Model: "test"}
}

func streamingTestDaemon(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	taskID := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/daemon/run":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"task_id":%q,"status":"accepted"}`, taskID)
		case "/internal/daemon/tasks/" + taskID + "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: complete\ndata: {\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}\n\n")
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	return taskID, server
}

type recordingBroadcaster struct {
	channelMessages   map[string][]string
	threadMessages    map[string][]string
	broadcastMessages []string
}

func newRecordingBroadcaster() *recordingBroadcaster {
	return &recordingBroadcaster{
		channelMessages: make(map[string][]string),
		threadMessages:  make(map[string][]string),
	}
}

func (b *recordingBroadcaster) BroadcastToScope(scopeType, scopeID string, message []byte) {
	if scopeType == realtime.ScopeChannel {
		b.BroadcastToChannel(scopeID, message)
		return
	}
	if scopeType == realtime.ScopeThread {
		b.BroadcastToThread(scopeID, message)
	}
}

func (b *recordingBroadcaster) BroadcastToChannel(channelID string, message []byte) {
	b.channelMessages[channelID] = append(b.channelMessages[channelID], string(message))
}

func (b *recordingBroadcaster) SendToUser(string, []byte) {}

func (b *recordingBroadcaster) BroadcastToThread(threadID string, message []byte) {
	b.threadMessages[threadID] = append(b.threadMessages[threadID], string(message))
}

func (b *recordingBroadcaster) Broadcast(message []byte) {
	b.broadcastMessages = append(b.broadcastMessages, string(message))
}

func (b *recordingBroadcaster) hasChannelEvent(channelID, eventType, contains string) bool {
	for _, raw := range b.channelMessages[channelID] {
		if eventEnvelopeType(raw) == eventType && strings.Contains(raw, contains) {
			return true
		}
	}
	return false
}

func (b *recordingBroadcaster) hasBroadcastEvent(eventType, contains string) bool {
	for _, raw := range b.broadcastMessages {
		if eventEnvelopeType(raw) == eventType && strings.Contains(raw, contains) {
			return true
		}
	}
	return false
}

func eventEnvelopeType(raw string) string {
	var envelope struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(raw), &envelope)
	return envelope.Type
}

var _ realtime.Broadcaster = (*recordingBroadcaster)(nil)
