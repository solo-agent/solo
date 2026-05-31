package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

func setupChiRouterForDMTask(h *DMHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/dm/{dmID}/tasks", func(r chi.Router) {
		r.Get("/", h.ListTasks)
		r.Post("/", h.CreateTask)

		r.Route("/{taskID}", func(r chi.Router) {
			r.Get("/", h.GetTask)

			r.Post("/claim", h.ClaimTask)
			r.Delete("/claim", h.UnclaimTask)
		})
	})
	return r
}

func newTestDMHandler() *DMHandler {
	return &DMHandler{
		pool:    nil,
		hub:     &ws.Hub{},
		agentSvc: nil,
		taskSvc:  service.NewTaskService(nil),
	}
}

// TestDMTaskHandler_Create_Validation tests input validation for DM task creation.
func TestDMTaskHandler_Create_Validation(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

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
			name:       "missing auth",
			body:       `{"title":"DM Task"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/dm/dm-1/tasks", bytes.NewBufferString(tt.body))
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

// TestDMTaskHandler_List_MissingAuth tests that listing requires auth.
func TestDMTaskHandler_List_MissingAuth(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

	req := httptest.NewRequest("GET", "/api/v1/dm/dm-1/tasks", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

// TestDMTaskHandler_Get_MissingAuth tests that getting a task requires auth.
func TestDMTaskHandler_Get_MissingAuth(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

	req := httptest.NewRequest("GET", "/api/v1/dm/dm-1/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

// TestDMTaskHandler_Claim_MissingAuth tests that claiming requires auth.
func TestDMTaskHandler_Claim_MissingAuth(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

	req := httptest.NewRequest("POST", "/api/v1/dm/dm-1/tasks/task-1/claim", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

// TestDMTaskHandler_Unclaim_MissingAuth tests that unclaiming requires auth.
func TestDMTaskHandler_Unclaim_MissingAuth(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

	req := httptest.NewRequest("DELETE", "/api/v1/dm/dm-1/tasks/task-1/claim", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

// TestDMTaskHandler_Create_TitleTooLong tests title length validation.
func TestDMTaskHandler_Create_TitleTooLong(t *testing.T) {
	h := newTestDMHandler()
	r := setupChiRouterForDMTask(h)

	longTitle := make([]byte, 501)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	body := `{"title":"` + string(longTitle) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/dm/dm-1/tasks", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long title, got %d", rr.Code)
	}
}

// TestDMTaskHandler_ResponseFormat verifies serialization of the task response.
func TestDMTaskHandler_ResponseFormat(t *testing.T) {
	resp := TaskResponse{
		ID:        "task-1",
		ChannelID: "dm-1",
		CreatorID: "user-1",
		Title:     "DM Task",
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
	if decoded.Title != "DM Task" {
		t.Errorf("expected title 'DM Task', got %s", decoded.Title)
	}
	if decoded.ChannelID != "dm-1" {
		t.Errorf("expected channel_id 'dm-1', got %s", decoded.ChannelID)
	}
}
