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

var _ realtime.Broadcaster = noopBroadcaster{}
