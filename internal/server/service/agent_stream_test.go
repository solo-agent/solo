package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

func TestAgentRunPhaseTimeouts(t *testing.T) {
	if agentRunQueueTimeout != 20*time.Minute {
		t.Fatalf("agentRunQueueTimeout = %s, want 20m", agentRunQueueTimeout)
	}
	if agentRunExecutionTimeout != 6*time.Minute {
		t.Fatalf("agentRunExecutionTimeout = %s, want 6m", agentRunExecutionTimeout)
	}
}

func TestApplySessionDispatchKeepsFreshTaskContextAndBuildsColdContinuity(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	taskID := agentRunTask(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := &AgentService{pool: pool}
	resumed := daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID, TaskContext: "stale task context",
		ColdStartMessages: []agent.Message{{Role: agent.RoleSystem, Content: continuityMessagePrefix + "\nstale"}},
	}
	if err := svc.applySessionDispatch(ctx, pool, &resumed, SessionDispatch{ResumeSessionID: "provider-session"}); err != nil {
		t.Fatal(err)
	}
	if resumed.ResumeSessionID != "provider-session" || !strings.Contains(resumed.TaskContext, "agent-run-test") || strings.Contains(resumed.TaskContext, "stale") {
		t.Fatalf("resumed dispatch = %+v", resumed)
	}
	if len(resumed.ColdStartMessages) != 0 {
		t.Fatalf("stale continuity survived resume: %+v", resumed.ColdStartMessages)
	}

	cold := daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID, OriginTaskID: taskID,
		Messages:    []agent.Message{{Role: agent.RoleUser, Content: "continue"}},
		TaskContext: "legacy task context",
	}
	if err := svc.applySessionDispatch(ctx, pool, &cold, SessionDispatch{ColdStart: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cold.TaskContext, "agent-run-test") || strings.Contains(cold.TaskContext, "legacy") ||
		len(cold.ColdStartMessages) != 2 || cold.ColdStartMessages[0].Role != agent.RoleSystem ||
		!strings.HasPrefix(cold.ColdStartMessages[0].Content, continuityMessagePrefix) ||
		!strings.Contains(cold.ColdStartMessages[0].Content, "agent-run-test") {
		t.Fatalf("cold dispatch = %+v", cold)
	}
}

func TestStreamingAgentTaskSettlesRunWhenSessionDispatchFails(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		originTaskID string
	}{
		{name: "resolve", provider: ""},
		{name: "continuity", provider: "claude", originTaskID: "not-a-uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := agentRunTestPool(t)
			ctx := context.Background()
			ownerID := agentRunUser(t, pool)
			agentID := agentRunAgent(t, pool, ownerID)
			channelID := agentRunChannel(t, pool, ownerID)
			t.Cleanup(func() {
				_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
				_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
				_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
				_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
			})

			daemonCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				daemonCalled = true
				http.Error(w, "unexpected dispatch", http.StatusInternalServerError)
			}))
			defer server.Close()

			rec := newRecordingBroadcaster()
			dm := NewDaemonManager(pool, rec)
			daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
			dm.Register(daemon)
			svc := NewAgentService(pool, dm, rec, nil)
			svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
				AgentID: agentID, ChannelID: channelID, OriginTaskID: tt.originTaskID,
				Messages:       []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
				ModelConfig:    agent.ModelConfig{Provider: tt.provider, Model: "test"},
				ResultContract: agentResultContractNone,
			}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

			if daemonCalled {
				t.Fatal("daemon was called after Session dispatch failed")
			}
			var runID, status, budgetState string
			var finishedAt *time.Time
			if err := pool.QueryRow(ctx, `
				SELECT r.id::text, r.status, r.finished_at, u.state
				  FROM agent_runs r
				  JOIN agent_run_token_usage u ON u.run_id = r.id
				 WHERE r.agent_id = $1
				 ORDER BY r.started_at DESC LIMIT 1`, agentID,
			).Scan(&runID, &status, &finishedAt, &budgetState); err != nil {
				t.Fatal(err)
			}
			if status != string(AgentRunStatusFailed) || finishedAt == nil || budgetState != "released" {
				t.Fatalf("Run %s = status %q finished %v budget %q", runID, status, finishedAt, budgetState)
			}
			assertRunEventCount(t, pool, runID, AgentRunEventError, 1)
			if !rec.hasChannelEvent(channelID, "agent.run.finished", `"status":"failed"`) {
				t.Fatalf("failed Run was not broadcast: %q", rec.channelMessages[channelID])
			}
		})
	}
}

func TestStreamingAgentTaskBindsSessionAndTranscript(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	transcriptPath := agentRunTranscriptFileWithUsage(t, "stream transcript", 3, 4)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	daemonID := uuid.NewString()
	var gotTaskID, gotRunID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/internal/daemon/run":
			var req daemonTaskRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode daemon request: %v", err)
			}
			gotTaskID, gotRunID = req.TaskID, req.RunID
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"task_id":%q,"status":"accepted"}`, req.TaskID)
		case r.URL.Path == "/internal/daemon/tasks/"+gotTaskID+"/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "event: session\ndata: {\"external_session_id\":\"provider-session-1\"}\n\n")
			_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"external_session_id\":\"provider-session-1\",\"transcript_path\":%q,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}\n\n", transcriptPath)
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	daemon := daemonInfoForTest(t, server.URL, daemonID)
	dm := NewDaemonManager(pool, noopBroadcaster{})
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		AgentID:          agentID,
		ChannelID:        channelID,
		TriggerMessageID: messageID,
		Messages:         []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
		ModelConfig:      agent.ModelConfig{Provider: "claude", Model: "test"},
		ResultContract:   agentResultContractNone,
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	var runID, sessionID, runStatus, runTranscript string
	var inputUsage, outputUsage int
	err := pool.QueryRow(ctx,
		`SELECT id::text, COALESCE(session_id::text, ''), status, COALESCE(transcript_path, ''),
		        COALESCE((usage_json->>'input_tokens')::int, 0),
		        COALESCE((usage_json->>'output_tokens')::int, 0)
		   FROM agent_runs
		  WHERE agent_id = $1
		  ORDER BY started_at DESC
		  LIMIT 1`, agentID,
	).Scan(&runID, &sessionID, &runStatus, &runTranscript, &inputUsage, &outputUsage)
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	if runStatus != string(AgentRunStatusCompleted) {
		t.Fatalf("run status = %q, want completed", runStatus)
	}
	if sessionID == "" {
		t.Fatal("run session_id is empty")
	}
	if runTranscript != transcriptPath {
		t.Fatalf("run transcript path = %q, want %q", runTranscript, transcriptPath)
	}
	if gotTaskID != runID || gotRunID != runID {
		t.Fatalf("daemon identity = task %q run %q, want agent run %q", gotTaskID, gotRunID, runID)
	}
	if inputUsage != 3 || outputUsage != 4 {
		t.Fatalf("usage = (%d, %d), want (3, 4)", inputUsage, outputUsage)
	}

	var externalID, sessionTranscript string
	err = pool.QueryRow(ctx,
		`SELECT external_session_id, COALESCE(transcript_path, '')
		   FROM agent_sessions
		  WHERE id = $1`, sessionID,
	).Scan(&externalID, &sessionTranscript)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if externalID != "provider-session-1" || sessionTranscript != transcriptPath {
		t.Fatalf("session = (%q, %q), want provider-session-1 and %q", externalID, sessionTranscript, transcriptPath)
	}

	timeline, err := NewAgentRunService(pool).GetSessionTimeline(ctx, sessionID, 100)
	if err != nil {
		t.Fatalf("GetSessionTimeline: %v", err)
	}
	if len(timeline.Entries) != 1 || timeline.Entries[0].Text != "stream transcript" {
		t.Fatalf("timeline entries = %+v", timeline.Entries)
	}
}

func TestStreamingAgentTaskDoesNotAddTranscriptUsageToDaemonUsage(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	transcriptPath := agentRunTranscriptFileWithUsage(t, "stream transcript", 30, 40)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	var taskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/internal/daemon/run":
			var req daemonTaskRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode daemon request: %v", err)
			}
			taskID = req.TaskID
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"task_id":%q,"status":"accepted"}`, taskID)
		case r.URL.Path == "/internal/daemon/tasks/"+taskID+"/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"external_session_id\":\"provider-session-usage\",\"transcript_path\":%q,\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"cache_read_tokens\":5,\"cache_write_tokens\":6}}\n\n", transcriptPath)
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
	dm := NewDaemonManager(pool, noopBroadcaster{})
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID, TriggerMessageID: messageID,
		Messages:       []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
		ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
		ResultContract: agentResultContractNone,
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	var input, output, cacheRead, cacheWrite int
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE((usage_json->>'input_tokens')::int,0), COALESCE((usage_json->>'output_tokens')::int,0),
		       COALESCE((usage_json->>'cache_read_tokens')::int,0), COALESCE((usage_json->>'cache_write_tokens')::int,0)
		  FROM agent_runs WHERE agent_id=$1 ORDER BY started_at DESC LIMIT 1`, agentID).
		Scan(&input, &output, &cacheRead, &cacheWrite); err != nil {
		t.Fatal(err)
	}
	if input != 3 || output != 4 || cacheRead != 5 || cacheWrite != 6 {
		t.Fatalf("usage=(%d,%d,%d,%d), want daemon usage (3,4,5,6)", input, output, cacheRead, cacheWrite)
	}
}

func TestStreamingAgentTaskDoesNotKeepPreCompactionSnapshot(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	var taskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/internal/daemon/run":
			var req daemonTaskRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode daemon request: %v", err)
			}
			taskID = req.TaskID
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"task_id":%q,"status":"accepted"}`, taskID)
		case r.URL.Path == "/internal/daemon/tasks/"+taskID+"/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: backend_started\ndata: {}\n\n")
			_, _ = fmt.Fprint(w, `event: context`+"\n"+`data: {"context":{"type":"usage","used_tokens":900,"window_tokens":1000}}`+"\n\n")
			_, _ = fmt.Fprint(w, `event: context`+"\n"+`data: {"context":{"type":"compaction_start","before_tokens":900,"window_tokens":1000}}`+"\n\n")
			_, _ = fmt.Fprint(w, `event: context`+"\n"+`data: {"context":{"type":"compaction_end","before_tokens":900,"window_tokens":1000}}`+"\n\n")
			_, _ = fmt.Fprint(w, "event: complete\ndata: {\"usage\":{}}\n\n")
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
	dm := NewDaemonManager(pool, noopBroadcaster{})
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID,
		Messages:       []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
		ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
		ResultContract: agentResultContractNone,
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	var runID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM agent_runs WHERE agent_id=$1 ORDER BY started_at DESC LIMIT 1`, agentID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	assertRunEventCount(t, pool, runID, AgentRunEventContextCompaction, 1)
	assertRunEventCount(t, pool, runID, AgentRunEventContextSnapshot, 0)
}

func TestStreamingAgentTaskKeepsTerminalRolloverCapabilitySnapshot(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	computerID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, DaemonID: computerID, TriggerType: AgentRunTriggerTask,
		ChannelID: channelID, Status: AgentRunStatusQueued, Source: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	dm := NewDaemonManager(pool, noopBroadcaster{})
	daemon := &DaemonInfo{
		ID: computerID, ComputerID: computerID, Capabilities: []string{"llm", contextRolloverCapability},
		Status: DaemonStatusOnline, MaxConcurrent: 1,
	}
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)

	finished := make(chan struct{})
	go func() {
		svc.runStreamingAgentTask(ctx, daemon, daemonTaskRequest{
			AgentID: agentID, ChannelID: channelID, PrestartedRun: true, RemotePrequeued: true,
			Messages:       []agent.Message{{Role: agent.RoleUser, Content: "continue"}},
			ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
			ResultContract: agentResultContractNone,
		}, agentChannelInfo{ID: agentID, Name: "Test Agent"}, run)
		close(finished)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		dm.remoteMu.Lock()
		streamReady := dm.remoteStreams[run.ID] != nil
		dm.remoteMu.Unlock()
		if streamReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("remote Run stream was not subscribed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	dm.deliverRemoteRunEvent(run.ID, "attempt-1", 1, SSEDaemonEvent{
		Event: "session", Data: `{"external_session_id":"provider-session-terminal"}`,
	})
	for {
		var sessionID string
		if err := pool.QueryRow(ctx, `SELECT COALESCE(session_id::text, '') FROM agent_runs WHERE id = $1`, run.ID).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
		if sessionID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("provider Session was not bound")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The terminal event already carries the daemon's recommendation. A
	// transient control disconnect after that must not erase the decision.
	dm.mu.Lock()
	delete(dm.daemons, computerID)
	dm.mu.Unlock()
	dm.deliverRemoteRunEvent(run.ID, "attempt-1", 2, SSEDaemonEvent{
		Event: "complete",
		Data:  `{"external_session_id":"provider-session-terminal","session_rollover":{"requested":true,"reason":"ineffective_compaction"}}`,
	})
	dm.deliverRemoteRunEvent(run.ID, "attempt-1", 3, SSEDaemonEvent{Event: "done", Data: `{}`})

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("remote Run did not finish")
	}
	var sessionStatus string
	if err := pool.QueryRow(ctx, `
		SELECT s.status
		  FROM agent_runs r JOIN agent_sessions s ON s.id = r.session_id
		 WHERE r.id = $1`, run.ID).Scan(&sessionStatus); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != AgentSessionStatusRolloverPending {
		t.Fatalf("Session status = %q, want rollover_pending", sessionStatus)
	}
	assertRunEventCount(t, pool, run.ID, AgentRunEventSessionRolloverRequested, 1)
}

func TestStreamingAgentTaskFailsWithoutVisibleMessage(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	taskID, server := streamingTestDaemon(t)
	defer server.Close()
	rec := newRecordingBroadcaster()
	dm := NewDaemonManager(pool, rec)
	daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, rec, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		TaskID:           taskID,
		AgentID:          agentID,
		ChannelID:        channelID,
		TriggerMessageID: messageID,
		ModelConfig:      agent.ModelConfig{Provider: "claude", Model: "test"},
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	var runID, status, failureCode string
	err := pool.QueryRow(ctx, `
		SELECT r.id::text, r.status, COALESCE((
			SELECT e.payload->>'failure_code'
			  FROM agent_run_events e
			 WHERE e.run_id = r.id AND e.type = $2
			 ORDER BY e.seq DESC
			 LIMIT 1
		), '')
		  FROM agent_runs r
		 WHERE r.agent_id = $1
		 ORDER BY r.started_at DESC
		 LIMIT 1`,
		agentID, AgentRunEventError,
	).Scan(&runID, &status, &failureCode)
	if err != nil {
		t.Fatalf("query failed run: %v", err)
	}
	if status != string(AgentRunStatusFailed) || failureCode != agentFailureMissingVisibleResult {
		t.Fatalf("run %s = status %q failure %q", runID, status, failureCode)
	}
	if !rec.hasChannelEvent(channelID, "agent.error", agentErrorMissingVisibleResult) {
		t.Fatalf("missing visible-result error not broadcast: %q", rec.channelMessages[channelID])
	}
}

func TestStreamingAgentTaskMarksRunFailedWhenStreamStartFails(t *testing.T) {
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

	taskID := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "daemon refused task", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
	dm := NewDaemonManager(pool, noopBroadcaster{})
	dm.Register(daemon)
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
	svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
		TaskID:           taskID,
		AgentID:          agentID,
		ChannelID:        channelID,
		TriggerMessageID: messageID,
		Messages:         []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
		ModelConfig:      agent.ModelConfig{Provider: "claude", Model: "test"},
	}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

	var status string
	var finishedAt *time.Time
	err := pool.QueryRow(ctx,
		`SELECT status, finished_at
		   FROM agent_runs
		  WHERE agent_id = $1
		  ORDER BY started_at DESC
		  LIMIT 1`, agentID,
	).Scan(&status, &finishedAt)
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	if status != string(AgentRunStatusFailed) {
		t.Fatalf("run status = %q, want failed", status)
	}
	if finishedAt == nil {
		t.Fatal("finished_at is nil")
	}
}

func TestStreamingAgentTaskUsesTerminalStatusFromErrorEvent(t *testing.T) {
	tests := []struct {
		name        string
		eventStatus string
		want        AgentRunStatus
	}{
		{"timeout", "timeout", AgentRunStatusTimeout},
		{"cancelled", "cancelled", AgentRunStatusCancelled},
		{"aborted", "aborted", AgentRunStatusCancelled},
		{"empty", "", AgentRunStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			taskID := uuid.NewString()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/internal/daemon/run":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusAccepted)
					_, _ = fmt.Fprintf(w, `{"task_id":%q,"status":"accepted"}`, taskID)
				case "/internal/daemon/tasks/" + taskID + "/events":
					w.Header().Set("Content-Type", "text/event-stream")
					if tt.eventStatus == "" {
						_, _ = fmt.Fprint(w, `event: error`+"\n"+`data: {"agent_id":"agent-1","error":"backend failed"}`+"\n\n")
					} else {
						_, _ = fmt.Fprintf(w, "event: error\ndata: {\"agent_id\":\"agent-1\",\"error\":\"backend failed\",\"status\":%q}\n\n", tt.eventStatus)
					}
					_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			daemon := daemonInfoForTest(t, server.URL, uuid.NewString())
			dm := NewDaemonManager(pool, noopBroadcaster{})
			dm.Register(daemon)
			svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
			svc.handleStreamingAgentTask(ctx, daemon, daemonTaskRequest{
				TaskID:           taskID,
				AgentID:          agentID,
				ChannelID:        channelID,
				TriggerMessageID: messageID,
				Messages:         []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
				ModelConfig:      agent.ModelConfig{Provider: "claude", Model: "test"},
			}, agentChannelInfo{ID: agentID, Name: "Test Agent"})

			var status string
			err := pool.QueryRow(ctx,
				`SELECT status
				   FROM agent_runs
				  WHERE agent_id = $1
				  ORDER BY started_at DESC
				  LIMIT 1`, agentID,
			).Scan(&status)
			if err != nil {
				t.Fatalf("query run: %v", err)
			}
			if status != string(tt.want) {
				t.Fatalf("run status = %q, want %q", status, tt.want)
			}
		})
	}
}

func daemonInfoForTest(t *testing.T, rawURL, id string) *DaemonInfo {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return &DaemonInfo{
		ID:            id,
		Host:          host,
		Port:          port,
		MaxConcurrent: 1,
		Capabilities:  []string{"claude"},
		Status:        DaemonStatusOnline,
	}
}

type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastToScope(string, string, []byte) {}
func (noopBroadcaster) BroadcastToChannel(string, []byte)       {}
func (noopBroadcaster) SendToUser(string, []byte)               {}
func (noopBroadcaster) BroadcastToThread(string, []byte)        {}
func (noopBroadcaster) Broadcast([]byte)                        {}
func (noopBroadcaster) BroadcastToWorkspace(string, []byte)     {}

var _ realtime.Broadcaster = noopBroadcaster{}
