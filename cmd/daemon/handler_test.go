package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/llm"
)

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

func TestProcessTaskWithProviderFailsWhenStreamClosesWithoutDone(t *testing.T) {
	taskID := "task-missing-done"
	tm := newTaskManager()
	tm.AddTask(taskID, &taskState{
		TaskID:    taskID,
		AgentID:   "agent-1",
		ChannelID: "channel-1",
		Status:    taskStatusRunning,
	})
	h := newDaemonHandler(nil, tm, fakeStreamProvider{
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

	h := newDaemonHandler(nil, newTaskManager(), fakeStreamProvider{}, "http://127.0.0.1:8080", "")
	messages := h.materializeMessageAttachments(context.Background(), []llmMessage{
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
	chunks []llm.StreamChunk
}

func (p fakeStreamProvider) Complete(context.Context, *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}

func (p fakeStreamProvider) CompleteStream(context.Context, *llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
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
