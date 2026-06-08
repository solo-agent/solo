package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/i18n"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

// testBroadcaster is a no-op implementation of realtime.Broadcaster for testing.
type testBroadcaster struct{}

func (b *testBroadcaster) BroadcastToScope(scopeType, scopeID string, message []byte) {}
func (b *testBroadcaster) BroadcastToChannel(channelID string, message []byte)     {}
func (b *testBroadcaster) BroadcastToThread(threadID string, message []byte)       {}
func (b *testBroadcaster) SendToUser(userID string, message []byte)                {}
func (b *testBroadcaster) Broadcast(message []byte)                                {}

var _ realtime.Broadcaster = (*testBroadcaster)(nil)

func setupChiRouterForTask(h *TaskHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/channels/{channelID}/tasks", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)

		r.Route("/{taskID}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Patch("/", h.Update)
			r.Delete("/", h.Delete)
		})
	})
	return r
}

func newTestTaskHandler() *TaskHandler {
	return &TaskHandler{
		svc:      service.NewTaskService(nil),
		hub:      &testBroadcaster{},
		agentSvc: nil,
	}
}

func TestTaskHandler_Create_Validation(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "empty request body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty title",
			body:       `{"title":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing auth",
			body:       `{"title":"Test Task"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/tasks", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			if tt.name != "missing auth" {
				req.Header.Set("X-User-ID", "user-1")
			}

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestTaskHandler_Create_EmptyName(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	body := `{"title":""}`
	req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/tasks", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty title, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "task title is required" {
		t.Errorf("expected 'task title is required', got %q", resp.Message)
	}
}

func TestTaskHandler_Get_MissingAuth(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestTaskHandler_Delete_MissingAuth(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	req := httptest.NewRequest("DELETE", "/api/v1/channels/ch-1/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTaskHandler_Update_MissingAuth(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	body := `{"status":"in_progress"}`
	req := httptest.NewRequest("PATCH", "/api/v1/channels/ch-1/tasks/task-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTaskHandler_List_MissingAuth(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/tasks", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTaskHandler_Create_TitleTooLong(t *testing.T) {
	h := newTestTaskHandler()
	r := setupChiRouterForTask(h)

	longTitle := make([]byte, 501)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	body := `{"title":"` + string(longTitle) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/tasks", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long title, got %d", rr.Code)
	}
}

func TestTaskHandler_ResponseFormat(t *testing.T) {
	resp := TaskResponse{
		ID:        "task-1",
		ChannelID: "ch-1",
		CreatorID: "user-1",
		Title:     "Test Task",
		Status:    "todo",
		Priority:  "none",
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded TaskResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "task-1" {
		t.Errorf("expected id task-1, got %s", decoded.ID)
	}
	if decoded.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", decoded.Title)
	}
	if decoded.Status != "todo" {
		t.Errorf("expected status 'todo', got %s", decoded.Status)
	}
}

func TestTaskHandler_ListResponseFormat(t *testing.T) {
	resp := []TaskResponse{
		{ID: "task-1", Title: "First", Status: "todo", Priority: "high", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2025-01-01T00:00:00Z"},
		{ID: "task-2", Title: "Second", Status: "done", Priority: "low", CreatedAt: "2025-01-01T00:00:01Z", UpdatedAt: "2025-01-01T00:00:01Z"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded []TaskResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(decoded))
	}
	if decoded[0].ID != "task-1" {
		t.Errorf("expected first task id task-1, got %s", decoded[0].ID)
	}
	if decoded[1].Status != "done" {
		t.Errorf("expected second status 'done', got %s", decoded[1].Status)
	}
}

// spyBroadcaster is a Broadcaster implementation that records calls for test assertions.
type spyBroadcaster struct {
	channelMessages map[string][][]byte // channelID -> messages
	userMessages    map[string][][]byte
}

func newSpyBroadcaster() *spyBroadcaster {
	return &spyBroadcaster{
		channelMessages: make(map[string][][]byte),
		userMessages:    make(map[string][][]byte),
	}
}

func (b *spyBroadcaster) BroadcastToScope(scopeType, scopeID string, message []byte) {
	if scopeType == realtime.ScopeChannel {
		b.channelMessages[scopeID] = append(b.channelMessages[scopeID], message)
	}
}

func (b *spyBroadcaster) BroadcastToChannel(channelID string, message []byte) {
	b.channelMessages[channelID] = append(b.channelMessages[channelID], message)
}

func (b *spyBroadcaster) BroadcastToThread(threadID string, message []byte) {
	b.channelMessages[threadID] = append(b.channelMessages[threadID], message)
}

func (b *spyBroadcaster) SendToUser(userID string, message []byte) {
	b.userMessages[userID] = append(b.userMessages[userID], message)
}

func (b *spyBroadcaster) Broadcast(message []byte) {}

var _ realtime.Broadcaster = (*spyBroadcaster)(nil)

func TestTaskNumber_ResponseJSON(t *testing.T) {
	resp := TaskResponse{
		ID:         "task-1",
		TaskNumber: 42,
		ChannelID:  "ch-1",
		CreatorID:  "user-1",
		Title:      "Test Task",
		Status:     "todo",
		Priority:   "none",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	tn, ok := decoded["task_number"]
	if !ok {
		t.Error("task_number field missing from JSON response")
	}
	if tn.(float64) != 42 {
		t.Errorf("expected task_number 42, got %v", tn)
	}
}

func TestFormatSystemMessage(t *testing.T) {
	msg := formatSystemMessage(3, "Fix login page", i18n.Active.SysTaskCreated)
	expected := fmt.Sprintf("📋 Task #%d %s: %s", 3, i18n.Active.SysTaskCreated, "Fix login page")
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	msg2 := formatSystemMessage(1, "Test task", i18n.Active.SysTaskDeleted)
	expected2 := fmt.Sprintf("📋 Task #%d %s: %s", 1, i18n.Active.SysTaskDeleted, "Test task")
	if msg2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, msg2)
	}
}

func TestFormatStatusDisplay(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"todo", "TODO"},
		{"in_progress", "IN PROGRESS"},
		{"in_review", "IN REVIEW"},
		{"done", "DONE"},
		{"closed", "CLOSED"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := formatStatusDisplay(tt.status)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestStatusTransition_Closed(t *testing.T) {
	// Test that closed is a valid status and transitions work
	// These test the constant definition and transition map in the service package

	// Verify TaskStatusClosed is defined
	if service.TaskStatusClosed != "closed" {
		t.Errorf("expected TaskStatusClosed = 'closed', got %q", service.TaskStatusClosed)
	}

	// Verify TaskStatusCancelled no longer exists (check by comparing constants)
	for _, s := range service.ValidTaskStatuses {
		if s == "cancelled" {
			t.Error("'cancelled' should not appear in ValidTaskStatuses")
		}
	}

	// Verify closed is in ValidTaskStatuses
	found := false
	for _, s := range service.ValidTaskStatuses {
		if s == "closed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'closed' should be in ValidTaskStatuses")
	}
}

func TestCreateTask_SystemMessage(t *testing.T) {
	// Test that the spy broadcaster captures system message format correctly
	spy := newSpyBroadcaster()
	h := &TaskHandler{
		svc:      service.NewTaskService(nil),
		hub:      spy,
		agentSvc: nil,
	}

	// Phase 5: broadcastSystemMessage only sends to threads. For channel-level
	// broadcasts, use broadcastSystemMessageWithID with showInChannel=true.
	h.broadcastSystemMessageWithID("ch-test", "", 5, "Task title", i18n.Active.SysTaskCreated, "msg-fixed-id", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), true)

	// Verify broadcast was sent
	msgs := spy.channelMessages["ch-test"]
	if len(msgs) != 1 {
		t.Fatalf("expected 1 broadcast message, got %d", len(msgs))
	}

	// Verify message structure
	var envelope ws.WSMessage
	if err := json.Unmarshal(msgs[0], &envelope); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	if envelope.Type != ws.EventMessageNew {
		t.Errorf("expected type %q, got %q", ws.EventMessageNew, envelope.Type)
	}

	var payload ws.MessageNewPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.SenderType != "system" {
		t.Errorf("expected sender_type 'system', got %q", payload.SenderType)
	}
	if payload.SenderID != "system" {
		t.Errorf("expected sender_id 'system', got %q", payload.SenderID)
	}
	if payload.SenderName != "Solo" {
		t.Errorf("expected sender_name 'Solo', got %q", payload.SenderName)
	}
	if payload.ContentType != "system" {
		t.Errorf("expected content_type 'system', got %q", payload.ContentType)
	}
	if payload.ChannelID != "ch-test" {
		t.Errorf("expected channel_id 'ch-test', got %q", payload.ChannelID)
	}
	if payload.ID == "" {
		t.Error("expected non-empty message ID")
	}
	expectedContent := fmt.Sprintf("📋 Task #%d %s: %s", 5, i18n.Active.SysTaskCreated, "Task title")
	if payload.Content != expectedContent {
		t.Errorf("unexpected content: got %q, want %q", payload.Content, expectedContent)
	}
}

func TestTaskHandler_ResponseIncludesTaskNumber(t *testing.T) {
	// Verify toTaskResponse includes task_number
	dueDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	svcTask := &service.Task{
		ID:         "task-uuid",
		TaskNumber: 99,
		ChannelID:  "ch-uuid",
		CreatorID:  "creator-uuid",
		Title:      "Task with number",
		Status:     "todo",
		Priority:   "none",
		DueDate:    &dueDate,
		CreatedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	resp := toTaskResponse(svcTask)
	if resp.TaskNumber != 99 {
		t.Errorf("expected task_number 99, got %d", resp.TaskNumber)
	}
	if resp.ID != "task-uuid" {
		t.Errorf("expected id task-uuid, got %s", resp.ID)
	}
}
