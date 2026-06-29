package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAgentRunServiceLifecycle(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	var agentName string
	if err := pool.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, agentID).Scan(&agentName); err != nil {
		t.Fatalf("load agent name: %v", err)
	}
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	taskID := agentRunTask(t, pool, channelID, ownerID)
	var runID string
	t.Cleanup(func() {
		if runID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = $1`, runID)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	if _, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:  agentID,
		Provider: "codex",
	}); err == nil {
		t.Fatal("UpsertSession without external_session_id or transcript_path succeeded, want error")
	}

	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "codex",
		ExternalSessionID: "provider-session-1",
		TranscriptPath:    agentRunTranscriptFile(t),
	})
	if err != nil {
		t.Fatalf("CreateOrResumeSession: %v", err)
	}

	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:          agentID,
		TriggerType:      AgentRunTriggerMessage,
		TriggerMessageID: messageID,
		ChannelID:        channelID,
		Status:           AgentRunStatusQueued,
		ActivityText:     "等待执行",
		Source:           "codex",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	runID = run.ID
	if run.Status != AgentRunStatusQueued {
		t.Fatalf("run status = %q, want %q", run.Status, AgentRunStatusQueued)
	}
	if run.SessionID != "" {
		t.Fatalf("run session_id = %q, want empty before provider session is known", run.SessionID)
	}
	run, err = svc.BindRunSession(ctx, BindRunSessionInput{
		RunID:     run.ID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("BindRunSession: %v", err)
	}
	if run.SessionID != session.ID {
		t.Fatalf("bound run session_id = %q, want %q", run.SessionID, session.ID)
	}
	if run.Status != AgentRunStatusRunning {
		t.Fatalf("bound run status = %q, want %q", run.Status, AgentRunStatusRunning)
	}
	activeRuns, err := svc.ListActiveRuns(ctx)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	var activeRun AgentRun
	foundActive := false
	for _, active := range activeRuns {
		if active.ID == run.ID {
			activeRun = active
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("ListActiveRuns did not include active run %s", run.ID)
	}
	if activeRun.AgentName != agentName {
		t.Fatalf("active run agent_name = %q, want %q", activeRun.AgentName, agentName)
	}
	transcript, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(transcript) != 1 || transcript[0].Text != "hello from transcript" {
		t.Fatalf("transcript = %+v, want session transcript fallback", transcript)
	}
	if _, err := svc.UpdateRunTranscript(ctx, UpdateRunTranscriptInput{
		RunID:          run.ID,
		TranscriptPath: session.TranscriptPath,
	}); err != nil {
		t.Fatalf("UpdateRunTranscript: %v", err)
	}
	if _, err := svc.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
		SessionID:      session.ID,
		TranscriptPath: agentRunTranscriptFileWithText(t, "newer session transcript"),
	}); err != nil {
		t.Fatalf("UpdateSessionMetadata newer transcript: %v", err)
	}
	transcript, err = svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript after snapshot: %v", err)
	}
	if len(transcript) != 1 || transcript[0].Text != "hello from transcript" {
		t.Fatalf("transcript = %+v, want run transcript snapshot", transcript)
	}

	pathOnly, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:        agentID,
		Provider:       "codex",
		TranscriptPath: agentRunTranscriptFileWithText(t, "path only transcript"),
	})
	if err != nil {
		t.Fatalf("UpsertSession with transcript path: %v", err)
	}
	pathOnlyAgain, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:        agentID,
		Provider:       "codex",
		TranscriptPath: pathOnly.TranscriptPath,
	})
	if err != nil {
		t.Fatalf("UpsertSession with same transcript path: %v", err)
	}
	if pathOnlyAgain.ID != pathOnly.ID {
		t.Fatalf("path-only session id = %q, want existing %q", pathOnlyAgain.ID, pathOnly.ID)
	}

	if err := svc.LinkTask(ctx, LinkRunTaskInput{
		RunID:      run.ID,
		TaskID:     taskID,
		Role:       AgentRunTaskRolePrimary,
		Confidence: 1,
	}); err != nil {
		t.Fatalf("LinkTask: %v", err)
	}

	event1, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   run.ID,
		Type:    AgentRunEventRunStarted,
		Message: "创建 run",
	})
	if err != nil {
		t.Fatalf("AppendEvent #1: %v", err)
	}
	event2, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:    run.ID,
		Type:     AgentRunEventToolStarted,
		Message:  "Bash: npm test",
		ToolName: "Bash",
	})
	if err != nil {
		t.Fatalf("AppendEvent #2: %v", err)
	}
	if event1.Seq != 1 || event2.Seq != 2 {
		t.Fatalf("event seq = %d, %d; want 1, 2", event1.Seq, event2.Seq)
	}
	longOutput := strings.Repeat("x", 3000)
	slimmedEvent, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:    run.ID,
		Type:     AgentRunEventToolFinished,
		Message:  longOutput,
		ToolName: "Bash",
		Payload: map[string]any{
			"call_id":  "call-1",
			"is_error": false,
			"output":   longOutput,
		},
	})
	if err != nil {
		t.Fatalf("AppendEvent slimmedEvent: %v", err)
	}
	if len(slimmedEvent.Message) >= len(longOutput) {
		t.Fatalf("event message was not slimmed: got %d bytes, want < %d", len(slimmedEvent.Message), len(longOutput))
	}
	var slimmedPayload map[string]any
	if err := json.Unmarshal(slimmedEvent.Payload, &slimmedPayload); err != nil {
		t.Fatalf("slimmed payload json: %v", err)
	}
	if slimmedPayload["call_id"] != "call-1" || slimmedPayload["is_error"] != false {
		t.Fatalf("slimmed payload lost metadata: %+v", slimmedPayload)
	}
	output, _ := slimmedPayload["output"].(string)
	if len(output) >= len(longOutput) {
		t.Fatalf("payload output was not slimmed: got %d bytes, want < %d", len(output), len(longOutput))
	}

	updated, err := svc.UpdateStatus(ctx, UpdateRunStatusInput{
		RunID:            run.ID,
		Status:           AgentRunStatusRunning,
		ActivityText:     "Bash: npm test",
		ToolName:         "Bash",
		ToolInputSummary: "Bash: npm test",
	})
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != AgentRunStatusRunning || updated.ToolName != "Bash" {
		t.Fatalf("updated run = %+v", updated)
	}

	finished, err := svc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       AgentRunStatusCompleted,
		ActivityText: "已完成",
	})
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if finished.Status != AgentRunStatusCompleted {
		t.Fatalf("finished status = %q, want %q", finished.Status, AgentRunStatusCompleted)
	}
	if finished.FinishedAt == nil {
		t.Fatal("finished_at is nil")
	}
	recentRuns, err := svc.ListRecentRuns(ctx)
	if err != nil {
		t.Fatalf("ListRecentRuns: %v", err)
	}
	if !agentRunListContains(recentRuns, run.ID) {
		t.Fatalf("ListRecentRuns did not include completed run %s", run.ID)
	}

	failedRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		SessionID:    session.ID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusQueued,
		ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun failedRun: %v", err)
	}
	if _, err := svc.FinishRun(ctx, FinishRunInput{
		RunID:        failedRun.ID,
		Status:       AgentRunStatusFailed,
		ActivityText: "执行失败",
	}); err != nil {
		t.Fatalf("FinishRun failedRun: %v", err)
	}
	if err := svc.LinkTask(ctx, LinkRunTaskInput{
		RunID:  failedRun.ID,
		TaskID: taskID,
		Role:   AgentRunTaskRolePrimary,
	}); err != nil {
		t.Fatalf("LinkTask failedRun: %v", err)
	}
	taskRuns, err := svc.ListRunsByTask(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRunsByTask: %v", err)
	}
	if len(taskRuns) != 1 || taskRuns[0].ID != run.ID {
		t.Fatalf("ListRunsByTask = %+v, want only completed run %s", taskRuns, run.ID)
	}
	taskSummaries, err := svc.ListAgentTasks(ctx, agentID)
	if err != nil {
		t.Fatalf("ListAgentTasks: %v", err)
	}
	var foundTask *AgentTaskSummary
	for i := range taskSummaries {
		if taskSummaries[i].ID == taskID {
			foundTask = &taskSummaries[i]
			break
		}
	}
	if foundTask == nil {
		t.Fatal("ListAgentTasks did not include linked task")
	}
	if foundTask.LinkedRunCount != 1 || foundTask.LastRunID != run.ID {
		t.Fatalf("task summary = %+v, want linked_run_count=1 last_run_id=%s", foundTask, run.ID)
	}
	activeRuns, err = svc.ListActiveRuns(ctx)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	for _, active := range activeRuns {
		if active.ID == failedRun.ID {
			t.Fatalf("failed run %s should not be listed as active", failedRun.ID)
		}
	}
}

func TestParseOpenClawMessageTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"type":"message","timestamp":"2026-06-28T08:56:32Z","message":{"role":"user","content":"hello from openclaw"}}`))
	if len(entries) != 1 || entries[0].Role != "user" || entries[0].Text != "hello from openclaw" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCodexPayloadTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello from codex"}]}}`))
	if len(entries) != 1 || entries[0].Role != "user" || entries[0].Text != "hello from codex" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCodexPayloadSkipsDeveloperMessages(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"hidden instructions"}]}}`))
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want no visible transcript entries", entries)
	}
}

func TestParseCodexToolPayload(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"call-1","arguments":"{\"cmd\":\"go test\"}"}}`))
	if len(entries) != 1 || entries[0].Type != "tool_use" || entries[0].ToolName != "shell" || entries[0].ToolID != "call-1" || entries[0].Text == "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseOpenClawTrajectoryTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"type":"prompt.submitted","ts":"2026-06-28T08:56:32Z","data":{"prompt":"hello from trajectory"}}`))
	if len(entries) != 1 || entries[0].Text != "hello from trajectory" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGetRunTranscriptResolvesProviderPathWhenRunPathMissing(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	codexHome := filepath.Join(t.TempDir(), ".codex")
	externalID := "codex-session-1"
	path := filepath.Join(codexHome, "sessions", "2026", "06", "28", "rollout-2026-06-28T12-00-00-"+externalID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"live transcript"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)

	svc := NewAgentRunService(pool)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "codex",
		ExternalSessionID: externalID,
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:     agentID,
		SessionID:   session.ID,
		TriggerType: AgentRunTriggerManual,
		ChannelID:   channelID,
		Status:      AgentRunStatusRunning,
		Source:      "codex",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	entries, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "live transcript" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGetRunTranscriptReadsHermesStateDBDirectly(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, ".hermes", "state.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	sql := `
CREATE TABLE sessions (id TEXT PRIMARY KEY);
CREATE TABLE messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT,
  tool_name TEXT,
  tool_calls TEXT,
  reasoning TEXT,
  reasoning_content TEXT,
  timestamp REAL NOT NULL,
  active INTEGER NOT NULL DEFAULT 1
);
INSERT INTO sessions (id) VALUES ('hermes-live');
INSERT INTO messages (session_id, role, content, timestamp) VALUES ('hermes-live', 'assistant', 'from state db', 1760000000.5);
`
	cmd := exec.Command("sqlite3", dbPath, sql)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, output)
	}

	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "hermes",
		ExternalSessionID: "hermes-live",
		TranscriptPath:    filepath.Join(home, ".solo", "hermes-transcripts", "hermes-live.jsonl"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:     agentID,
		SessionID:   session.ID,
		TriggerType: AgentRunTriggerManual,
		ChannelID:   channelID,
		Status:      AgentRunStatusRunning,
		Source:      "hermes",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, time.Unix(1760000000, 0).UTC()); err != nil {
		t.Fatalf("set run time: %v", err)
	}

	entries, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "from state db" {
		t.Fatalf("entries = %+v", entries)
	}
}

func agentRunListContains(runs []AgentRun, id string) bool {
	for _, run := range runs {
		if run.ID == id {
			return true
		}
	}
	return false
}

func agentRunTestPool(t *testing.T) *pgxpool.Pool {
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
	t.Cleanup(pool.Close)
	return pool
}

func agentRunUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	email := fmt.Sprintf("agent-run-%s@example.test", id)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, $3, 'test')`,
		id, email, "Agent Run Tester",
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func agentRunAgent(t *testing.T, pool *pgxpool.Pool, ownerID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO agents (id, name, owner_id, model_name) VALUES ($1, $2, $3, 'test-model')`,
		id, "agent-run-"+id[:8], ownerID,
	)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return id
}

func agentRunChannel(t *testing.T, pool *pgxpool.Pool, creatorID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`,
		id, "agent-run-"+id[:8], creatorID,
	)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return id
}

func agentRunMessage(t *testing.T, pool *pgxpool.Pool, channelID, senderID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content) VALUES ($1, $2, 'user', $3, 'please work')`,
		id, channelID, senderID,
	)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	return id
}

func agentRunTask(t *testing.T, pool *pgxpool.Pool, channelID, creatorID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tasks (id, channel_id, creator_id, title, status, priority, task_number)
		 VALUES ($1, $2, $3, 'agent-run-test', $4, 'normal',
		   (SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = $2))`,
		id, channelID, creatorID, TaskStatusTodo,
	)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}

func agentRunTranscriptFile(t *testing.T) string {
	return agentRunTranscriptFileWithText(t, "hello from transcript")
}

func agentRunTranscriptFileWithText(t *testing.T, text string) string {
	t.Helper()
	path := t.TempDir() + "/session.jsonl"
	raw := fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":%q}}`+"\n", time.Now().UTC().Format(time.RFC3339), text)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}
