package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/llm"
)

func TestControlRPCReadsTranscriptFromLocalRuntime(t *testing.T) {
	codexHome := filepath.Join(t.TempDir(), ".codex")
	sessionID := "remote-transcript-session"
	path := filepath.Join(codexHome, "sessions", "2026", "08", "07", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"2026-08-07T00:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"remote transcript works"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	h := newDaemonHandler(newTaskManager(), nil, "", "")
	h.workspaceManager = agent.NewWorkspaceManager(t.TempDir())
	payload, _ := json.Marshal(map[string]any{
		"agent_id": "agent-1", "provider": "codex", "external_session_id": sessionID, "limit": 10,
	})
	result, err := h.handleControlRPC(context.Background(), "transcript.read", payload)
	if err != nil {
		t.Fatalf("transcript.read: %v", err)
	}
	var entries []struct {
		Text string          `json:"text"`
		Raw  json.RawMessage `json:"raw"`
	}
	if err := json.Unmarshal(result, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Text != "remote transcript works" || len(entries[0].Raw) != 0 {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestAgentChannelCleanupEntrypointsRequireAndAcceptExactScope(t *testing.T) {
	h := newDaemonHandler(newTaskManager(), nil, "", "")
	if _, err := h.handleControlRPC(context.Background(), "agent.channel.cleanup", json.RawMessage(`{"agent_id":"agent-1"}`)); err == nil {
		t.Fatal("remote cleanup accepted a missing channel_id")
	}
	if _, err := h.handleControlRPC(context.Background(), "agent.channel.cleanup", json.RawMessage(`{"agent_id":"agent-1","channel_id":"channel-a"}`)); err != nil {
		t.Fatalf("remote scoped cleanup: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/daemon/agents/agent-1/channels/channel-a/cleanup", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("agentID", "agent-1")
	route.URLParams.Add("channelID", "channel-a")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	recorder := httptest.NewRecorder()
	h.CleanupAgentChannel(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("local scoped cleanup status = %d", recorder.Code)
	}
}

func TestProxyTemplateListUsesCurrentExecutingRunCredential(t *testing.T) {
	const freshToken = "fresh-current-run-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/templates" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+freshToken {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"dev-team"}]`)
	}))
	defer server.Close()

	tm := newTaskManager()
	tm.AddTask("task-current", &taskState{
		TaskID:     "task-current",
		RunID:      "run-current",
		AgentID:    "agent-1",
		ChannelID:  "channel-1",
		Status:     taskStatusRunning,
		AgentToken: freshToken,
	})
	h := newDaemonHandler(tm, nil, server.URL, "")

	body := bytes.NewBufferString(`{"agent_id":"agent-1","action":"template_list"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/daemon/proxy", body)
	req.RemoteAddr = "127.0.0.1:45678"
	recorder := httptest.NewRecorder()
	h.ProxyRequest(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"id":"dev-team"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestProxyWithoutExecutingRunExplainsRuntimeStateNotPermission(t *testing.T) {
	h := newDaemonHandler(newTaskManager(), nil, "http://remote-server.invalid", "")
	body := bytes.NewBufferString(`{"agent_id":"agent-1","action":"template_list"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/daemon/proxy", body)
	req.RemoteAddr = "127.0.0.1:45678"
	recorder := httptest.NewRecorder()
	h.ProxyRequest(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := strings.ToLower(recorder.Body.String())
	if !strings.Contains(response, "active agent turn") || strings.Contains(response, "permission") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestBackendFinalStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		result     *agent.Result
		wantStatus string
		wantTask   string
	}{
		{"completed", &agent.Result{Status: "completed"}, "completed", taskStatusCompleted},
		{"failed", &agent.Result{Status: "failed"}, "failed", taskStatusFailed},
		{"aborted", &agent.Result{Status: "aborted"}, "cancelled", taskStatusCancelled},
		{"timeout", &agent.Result{Status: "timeout"}, "timeout", taskStatusFailed},
		{"cancelled", &agent.Result{Status: "cancelled"}, "cancelled", taskStatusCancelled},
		{"empty", &agent.Result{}, "failed", taskStatusFailed},
		{"nil", nil, "failed", taskStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus := backendFinalStatus(tt.result)
			if gotStatus != tt.wantStatus {
				t.Fatalf("backendFinalStatus = %q, want %q", gotStatus, tt.wantStatus)
			}
			if gotTask := backendTaskStatus(gotStatus); gotTask != tt.wantTask {
				t.Fatalf("backendTaskStatus = %q, want %q", gotTask, tt.wantTask)
			}
		})
	}
}

func TestTaskFailureDetails(t *testing.T) {
	tests := []struct {
		message       string
		wantCode      string
		wantRetryable bool
	}{
		{"codex executable not found at /missing", "configuration", false},
		{"API key is missing", "configuration", false},
		{"Prompt is too long: 213208 tokens > 200000 maximum", "context_exhausted", true},
		{"The input exceeds the context window of this model.", "context_exhausted", true},
		{"context_length_exceeded", "context_exhausted", true},
		{"This model's maximum context length is 16385 tokens.", "context_exhausted", true},
		{"provider temporarily unavailable", "provider_transient", true},
	}
	for _, tt := range tests {
		got := taskFailureDetails(tt.message)
		if got.Code != tt.wantCode || got.Retryable != tt.wantRetryable {
			t.Fatalf("taskFailureDetails(%q) = %+v, want %s/%t", tt.message, got, tt.wantCode, tt.wantRetryable)
		}
	}
}

func TestResumableContextFailureUsesExactLocalSession(t *testing.T) {
	failure := taskFailureDetails("context_length_exceeded")
	session := &agent.PersistentSession{SessionID: "actual-local-session"}
	if got := resumableContextFailureSessionID("stale-server-session", session, failure); got != session.SessionID {
		t.Fatalf("session ID = %q, want %q", got, session.SessionID)
	}
	if got := resumableContextFailureSessionID("", session, failure); got != "" {
		t.Fatalf("fresh request returned session ID %q", got)
	}
	if got := resumableContextFailureSessionID("server-session", session, taskFailure{Code: "provider_transient"}); got != "" {
		t.Fatalf("transient failure returned session ID %q", got)
	}
}

func TestCleanupAgentCancelsEveryChannelTaskForAgent(t *testing.T) {
	tm := newTaskManager()
	h := newDaemonHandler(tm, nil, "", "")

	contexts := make(map[string]context.Context)
	for _, task := range []*taskState{
		{TaskID: "agent-channel-a", AgentID: "agent-1", ChannelID: "channel-a", Status: taskStatusThinking},
		{TaskID: "agent-channel-b", AgentID: "agent-1", ChannelID: "channel-b", Status: taskStatusQueued},
		{TaskID: "other-agent", AgentID: "agent-2", ChannelID: "channel-a", Status: taskStatusThinking},
	} {
		tm.AddTask(task.TaskID, task)
		ctx, cancel := context.WithCancel(context.Background())
		contexts[task.TaskID] = ctx
		tm.SetCancelFunc(task.TaskID, cancel)
	}

	h.cleanupAgent("agent-1")
	for _, taskID := range []string{"agent-channel-a", "agent-channel-b"} {
		if contexts[taskID].Err() != context.Canceled {
			t.Fatalf("task %s was not cancelled", taskID)
		}
	}
	if contexts["other-agent"].Err() != nil {
		t.Fatal("another Agent's task was cancelled")
	}
}

func TestCleanupAgentChannelCancelsOnlyMatchingTasks(t *testing.T) {
	tm := newTaskManager()
	h := newDaemonHandler(tm, nil, "", "")

	contexts := make(map[string]context.Context)
	for _, task := range []*taskState{
		{TaskID: "matching-active", AgentID: "agent-1", ChannelID: "channel-a", Status: taskStatusThinking},
		{TaskID: "matching-queued", AgentID: "agent-1", ChannelID: "channel-a", Status: taskStatusQueued},
		{TaskID: "other-channel", AgentID: "agent-1", ChannelID: "channel-b", Status: taskStatusThinking},
		{TaskID: "other-agent", AgentID: "agent-2", ChannelID: "channel-a", Status: taskStatusThinking},
	} {
		tm.AddTask(task.TaskID, task)
		ctx, cancel := context.WithCancel(context.Background())
		contexts[task.TaskID] = ctx
		tm.SetCancelFunc(task.TaskID, cancel)
	}

	h.cleanupAgentChannel("agent-1", "channel-a")
	for _, taskID := range []string{"matching-active", "matching-queued"} {
		if contexts[taskID].Err() != context.Canceled {
			t.Fatalf("task %s was not cancelled", taskID)
		}
	}
	for _, taskID := range []string{"other-channel", "other-agent"} {
		if contexts[taskID].Err() != nil {
			t.Fatalf("task %s was cancelled", taskID)
		}
	}
}

func TestStartTaskRejectsRetireSessionWithoutFreshStart(t *testing.T) {
	tm := newTaskManager()
	h := newDaemonHandler(tm, nil, "", "")
	err := h.startTask(runTaskRequest{
		TaskID: "task-1", AgentID: "agent-1", ChannelID: "channel-1",
		RetireSessionID: "provider-session-1",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "requires force_fresh_session") {
		t.Fatalf("startTask error = %v", err)
	}
	if _, exists := tm.GetTask("task-1"); exists {
		t.Fatal("invalid task was registered")
	}
}

func TestBackendTurnFinalizerForwardsContextAndAddsTerminalSnapshot(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("task-1", &taskState{TaskID: "task-1", Status: taskStatusThinking})
	h := newDaemonHandler(tm, nil, "", "")
	used, window := int64(91), int64(100)
	finalizer := backendTurnFinalizer{
		h:   h,
		req: runTaskRequest{TaskID: "task-1", AgentID: "agent-1"},
	}
	finalizer.observeContext(&agent.ContextEvent{
		Type: "usage", UsedTokens: &used, WindowTokens: &window, Accuracy: "reported",
	})
	finalizer.finish(taskStatusCompleted, "complete", map[string]interface{}{"agent_id": "agent-1"})
	finalizer.finalize()

	events := tm.EventsAfter("task-1", 0)
	if len(events) != 3 || events[0].Event != "context" || events[1].Event != "complete" || events[2].Event != "done" {
		t.Fatalf("events = %+v", events)
	}
	var terminal map[string]json.RawMessage
	if err := json.Unmarshal([]byte(events[1].Data), &terminal); err != nil {
		t.Fatal(err)
	}
	if len(terminal["context_snapshot"]) == 0 {
		t.Fatalf("terminal payload missing context_snapshot: %s", events[1].Data)
	}
	task, _ := tm.GetTask("task-1")
	if task.Status != taskStatusCompleted {
		t.Fatalf("task status = %q", task.Status)
	}
}

func TestFinishCancelledTaskDefersTerminalStateDuringShutdown(t *testing.T) {
	taskID := "task-daemon-shutdown"
	tm := newTaskManager()
	tm.AddTask(taskID, &taskState{TaskID: taskID, Status: taskStatusRunning})
	h := newDaemonHandler(tm, nil, "", "")
	h.shuttingDown.Store(true)

	h.finishCancelledTask(runTaskRequest{TaskID: taskID})

	task, ok := tm.GetTask(taskID)
	if !ok {
		t.Fatal("task was removed")
	}
	if task.Status != taskStatusRunning {
		t.Fatalf("task status = %q, want server-owned daemon_lost transition", task.Status)
	}
	if len(tm.eventHistory[taskID]) != 0 {
		t.Fatalf("shutdown emitted terminal events: %#v", tm.eventHistory[taskID])
	}
}

func TestProcessTaskWithProviderFailsWhenStreamClosesWithoutDone(t *testing.T) {
	taskID := "task-missing-done"
	tm := newTaskManager()
	tm.AddTask(taskID, &taskState{
		TaskID:    taskID,
		AgentID:   "agent-1",
		ChannelID: "channel-1",
		Status:    taskStatusRunning,
	})
	h := newDaemonHandler(tm, fakeStreamProvider{
		chunks: []llm.StreamChunk{{Content: "partial output"}},
	}, "", "")

	h.processTaskWithProvider(context.Background(), runTaskRequest{
		TaskID:    taskID,
		AgentID:   "agent-1",
		ChannelID: "channel-1",
		Messages: []llmMessage{
			{Role: "user", Content: "hello"},
		},
	})

	task, ok := tm.GetTask(taskID)
	if !ok {
		t.Fatalf("task was removed")
	}
	if task.Status != taskStatusFailed {
		t.Fatalf("task status = %q, want %q", task.Status, taskStatusFailed)
	}

	var sawError, sawComplete bool
	for _, evt := range tm.eventHistory[taskID] {
		switch evt.Event {
		case "error":
			sawError = strings.Contains(evt.Data, "provider stream closed without completion")
		case "complete":
			sawComplete = true
		}
	}
	if !sawError {
		t.Fatalf("missing replayable error event: %+v", tm.eventHistory[taskID])
	}
	if sawComplete {
		t.Fatalf("unexpected complete event: %+v", tm.eventHistory[taskID])
	}
}

func TestProcessTaskWithProviderUsesColdStartSystemPrompt(t *testing.T) {
	taskID := "task-cold-start"
	tm := newTaskManager()
	tm.AddTask(taskID, &taskState{TaskID: taskID, AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusRunning})
	var request *llm.CompletionRequest
	h := newDaemonHandler(tm, fakeStreamProvider{
		chunks: []llm.StreamChunk{{Done: true}},
		onRequest: func(got *llm.CompletionRequest) {
			copy := *got
			request = &copy
		},
	}, "", "")

	h.processTaskWithProvider(context.Background(), runTaskRequest{
		TaskID: taskID, AgentID: "agent-1", ChannelID: "channel-1", SystemPrompt: "base",
		Messages:          []llmMessage{{Role: "user", Content: "latest"}},
		ColdStartMessages: []llmMessage{{Role: "system", Content: "# Session Continuity\nresume task"}, {Role: "user", Content: "cold history"}},
	})

	if request == nil {
		t.Fatal("provider did not receive a request")
	}
	if request.SystemPrompt != "base\n\n# Session Continuity\nresume task" {
		t.Fatalf("system prompt = %q", request.SystemPrompt)
	}
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != "cold history" {
		t.Fatalf("messages = %+v", request.Messages)
	}
}

func TestReadBackendFinalResultTimesOut(t *testing.T) {
	ch := make(chan *agent.Result)
	result, ok := readBackendFinalResult(context.Background(), ch, time.Millisecond)
	if ok {
		t.Fatalf("readBackendFinalResult ok = true, want false")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
}

func TestReadBackendFinalResultReturnsCancelledOnContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan *agent.Result)

	result, ok := readBackendFinalResult(ctx, ch, time.Second)
	if !ok {
		t.Fatalf("readBackendFinalResult ok = false, want true")
	}
	if result == nil || result.Status != "cancelled" {
		t.Fatalf("result = %+v, want cancelled", result)
	}
}

func TestMaterializeMessageAttachmentsCopiesFilesIntoWorkspace(t *testing.T) {
	root := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("ATTACHMENTS_DIR", root)
	storagePath := filepath.Join("2026-07", "note.txt")
	fullPath := filepath.Join(root, storagePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("hello from attachment"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newDaemonHandler(newTaskManager(), fakeStreamProvider{}, "http://127.0.0.1:8080", "")
	messages := h.materializeMessageAttachments(context.Background(), "agent-1", []llmMessage{
		{
			Role:    "user",
			Content: "please read it",
			Attachments: []agent.Attachment{
				{
					ID:          "550e8400-e29b-41d4-a716-446655440000",
					Filename:    "note.txt",
					MIMEType:    "text/plain",
					Size:        21,
					URL:         "/api/v1/attachments/550e8400-e29b-41d4-a716-446655440000",
					StoragePath: storagePath,
					LocalPath:   agent.AttachmentLocalPath("550e8400-e29b-41d4-a716-446655440000", "note.txt"),
				},
			},
		},
	}, workDir)

	if len(messages) != 1 || len(messages[0].Attachments) != 1 {
		t.Fatalf("messages = %+v", messages)
	}
	localPath := messages[0].Attachments[0].LocalPath
	if localPath == "" {
		t.Fatal("materialized LocalPath is empty")
	}
	data, err := os.ReadFile(filepath.Join(workDir, filepath.FromSlash(localPath)))
	if err != nil {
		t.Fatalf("read materialized attachment: %v", err)
	}
	if string(data) != "hello from attachment" {
		t.Fatalf("materialized data = %q", string(data))
	}
	if !strings.Contains(messages[0].Content, "Materialized attachment files in this workspace") {
		t.Fatalf("content missing materialized paths: %s", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, localPath) {
		t.Fatalf("content missing local path %q: %s", localPath, messages[0].Content)
	}
}

type fakeStreamProvider struct {
	chunks    []llm.StreamChunk
	onRequest func(*llm.CompletionRequest)
}

func (p fakeStreamProvider) Complete(context.Context, *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}

func (p fakeStreamProvider) CompleteStream(_ context.Context, request *llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	if p.onRequest != nil {
		p.onRequest(request)
	}
	ch := make(chan llm.StreamChunk, len(p.chunks))
	for _, chunk := range p.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func TestRefreshTranscriptPathForProvider(t *testing.T) {
	existing := "/tmp/existing.jsonl"
	if got := refreshTranscriptPathForProvider("claude", "/tmp/workspace", "session-1", existing); got != existing {
		t.Fatalf("existing transcript path = %q, want %q", got, existing)
	}

	got := refreshTranscriptPathForProvider("claude", "/Users/me/.solo/agents/a1/workspace", "session-1", "")
	want := "/Users/me/.claude/projects/-Users-me--solo-agents-a1-workspace/session-1.jsonl"
	if got != want {
		t.Fatalf("refreshed transcript path = %q, want %q", got, want)
	}

	if got := refreshTranscriptPathForProvider("claude", "/tmp/workspace", "", ""); got != "" {
		t.Fatalf("empty session transcript path = %q, want empty", got)
	}
}

func TestCloneHTTPClientWithTimeoutPreservesTransport(t *testing.T) {
	transport := &http.Transport{}
	original := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	clone := cloneHTTPClientWithTimeout(original, 55*time.Second)
	if clone == original || clone.Transport != transport || clone.Timeout != 55*time.Second {
		t.Fatalf("unexpected cloned client: %#v", clone)
	}
	if original.Timeout != 10*time.Second {
		t.Fatalf("original timeout changed to %s", original.Timeout)
	}
}

func TestMergeAgentCustomEnvProtectsRuntimeIdentity(t *testing.T) {
	base := map[string]string{
		"SOLO_AGENT_ID":   "agent-1",
		"SOLO_AUTH_TOKEN": "run-token",
		"SOLO_RUN_ID":     "run-1",
		"PATH":            "/base/bin",
	}
	mergeAgentCustomEnv(base, map[string]string{
		"SOLO_AGENT_ID":   "other-agent",
		"SOLO_AUTH_TOKEN": "stolen-token",
		"SOLO_RUN_ID":     "other-run",
		"PATH":            "/custom/bin",
		"CUSTOM_FLAG":     "enabled",
	})

	if base["SOLO_AGENT_ID"] != "agent-1" || base["SOLO_AUTH_TOKEN"] != "run-token" || base["SOLO_RUN_ID"] != "run-1" {
		t.Fatalf("runtime-owned environment was overwritten: %#v", base)
	}
	if base["PATH"] != "/custom/bin" || base["CUSTOM_FLAG"] != "enabled" {
		t.Fatalf("ordinary custom environment was not preserved: %#v", base)
	}
}

func TestCleanupThinkingSessionsValidatesNodeIDs(t *testing.T) {
	h := newDaemonHandler(newTaskManager(), fakeStreamProvider{}, "", "")

	invalid := httptest.NewRequest(http.MethodPost, "/internal/daemon/thinking/cleanup", bytes.NewBufferString(`{"node_ids":["not-a-uuid"]}`))
	invalidResponse := httptest.NewRecorder()
	h.CleanupThinkingSessions(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid node status = %d, want %d", invalidResponse.Code, http.StatusBadRequest)
	}

	valid := httptest.NewRequest(http.MethodPost, "/internal/daemon/thinking/cleanup", bytes.NewBufferString(`{"node_ids":["550e8400-e29b-41d4-a716-446655440000"]}`))
	validResponse := httptest.NewRecorder()
	h.CleanupThinkingSessions(validResponse, valid)
	if validResponse.Code != http.StatusNoContent {
		t.Fatalf("valid node status = %d, want %d", validResponse.Code, http.StatusNoContent)
	}
}

func TestDurationFromEnv(t *testing.T) {
	t.Setenv("TEST_THINKING_DURATION", "5s")
	if got := durationFromEnv("TEST_THINKING_DURATION", time.Minute); got != 5*time.Second {
		t.Fatalf("duration = %s, want 5s", got)
	}
	t.Setenv("TEST_THINKING_DURATION", "invalid")
	if got := durationFromEnv("TEST_THINKING_DURATION", time.Minute); got != time.Minute {
		t.Fatalf("invalid duration fallback = %s, want 1m", got)
	}
}

func TestBackendContextTurnDropsPreCompactionSnapshot(t *testing.T) {
	used, window := int64(95), int64(100)
	turn := backendContextTurn{}
	turn.observe(&agent.ContextEvent{Type: "usage", UsedTokens: &used, WindowTokens: &window})
	turn.observe(&agent.ContextEvent{Type: "compaction_start"})

	observation := turn.observation()
	if observation.FinalSnapshot != nil || !observation.IncompleteCompaction {
		t.Fatalf("observation = %+v, want incomplete compaction without stale snapshot", observation)
	}
}

func TestBackendTurnFinalizerDrainsContextOnStop(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("task-drain", &taskState{TaskID: "task-drain", Status: taskStatusThinking})
	h := newDaemonHandler(tm, nil, "", "")
	before, after, window := int64(95), int64(40), int64(100)
	messages := make(chan agent.OutputChunk, 1)
	messages <- agent.OutputChunk{Type: string(agent.MessageContext), Context: &agent.ContextEvent{
		Type: "compaction_start", BeforeTokens: &before, WindowTokens: &window, Accuracy: "snapshot",
	}}
	session := &agent.Session{
		Messages: messages,
		Stop: func() error {
			messages <- agent.OutputChunk{Type: string(agent.MessageContext), Context: &agent.ContextEvent{
				Type: "compaction_end", BeforeTokens: &before, AfterTokens: &after, WindowTokens: &window, Accuracy: "snapshot",
			}}
			close(messages)
			return nil
		},
	}
	finalizer := backendTurnFinalizer{h: h, req: runTaskRequest{TaskID: "task-drain", AgentID: "agent-1"}}
	if !finalizer.stopAndDrainContext(session, time.Second) {
		t.Fatal("closed message stream was not an authoritative drain")
	}
	observation := finalizer.context.observation()
	if observation.IncompleteCompaction || len(observation.Compactions) != 1 {
		t.Fatalf("observation = %+v, want one completed compaction", observation)
	}
}

func TestBackendTurnFinalizerRejectsPartialDrain(t *testing.T) {
	tm := newTaskManager()
	h := newDaemonHandler(tm, nil, "", "")
	messages := make(chan agent.OutputChunk)
	session := &agent.Session{Messages: messages, Stop: func() error { return nil }}
	finalizer := backendTurnFinalizer{h: h, req: runTaskRequest{TaskID: "task-timeout", AgentID: "agent-1"}}
	if finalizer.stopAndDrainContext(session, 10*time.Millisecond) {
		t.Fatal("open message stream was treated as an authoritative drain")
	}
	close(messages)
}
