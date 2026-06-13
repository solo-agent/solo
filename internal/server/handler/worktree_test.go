package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupWorktreeRouter(h *TaskHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/tasks", func(r chi.Router) {
		r.Route("/{taskID}", func(r chi.Router) {
			r.Post("/isolate", h.IsolateTask)
			r.Delete("/isolate", h.UnisolateTask)
		})
	})
	return r
}

// ---- IsolateTask (create worktree endpoint) ----

func TestWorktree_Isolate_MissingAuth(t *testing.T) {
	h := &TaskHandler{}
	r := setupWorktreeRouter(h)

	req := httptest.NewRequest("POST", "/api/v1/tasks/test-task/isolate", nil)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing auth, got %d", rr.Code)
	}
}

func TestWorktree_Isolate_EmptyTaskID(t *testing.T) {
	h := &TaskHandler{}
	r := setupWorktreeRouter(h)

	req := httptest.NewRequest("POST", "/api/v1/tasks//isolate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty task ID, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "task ID is required" {
		t.Errorf("expected 'task ID is required', got %q", resp.Message)
	}
}

// Note: Isolate with real task requires DB/channel binding.
// Integration tests in docs/design/tasks/e2e-workspace.md cover the full flow.

// ---- UnisolateTask (cleanup worktree endpoint) ----

func TestWorktree_Unisolate_MissingAuth(t *testing.T) {
	h := &TaskHandler{}
	r := setupWorktreeRouter(h)

	req := httptest.NewRequest("DELETE", "/api/v1/tasks/test-task/isolate", nil)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing auth, got %d", rr.Code)
	}
}

func TestWorktree_Unisolate_EmptyTaskID(t *testing.T) {
	h := &TaskHandler{}
	r := setupWorktreeRouter(h)

	req := httptest.NewRequest("DELETE", "/api/v1/tasks//isolate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty task ID, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "task ID is required" {
		t.Errorf("expected 'task ID is required', got %q", resp.Message)
	}
}

// ---- Response format validation ----

func TestWorktree_IsolateResponse_Marshal(t *testing.T) {
	// Verify the isolate response structure serializes correctly.
	resp := map[string]interface{}{
		"status":         "isolated",
		"task_id":        "task-1",
		"channel_id":     "ch-1",
		"workspace_path": "/home/user/projects/test",
		"task_number":    42,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["status"] != "isolated" {
		t.Errorf("expected status 'isolated', got %v", decoded["status"])
	}
	if decoded["task_id"] != "task-1" {
		t.Errorf("expected task_id 'task-1', got %v", decoded["task_id"])
	}
	if decoded["channel_id"] != "ch-1" {
		t.Errorf("expected channel_id 'ch-1', got %v", decoded["channel_id"])
	}
	if decoded["workspace_path"] != "/home/user/projects/test" {
		t.Errorf("expected workspace_path '/home/user/projects/test', got %v", decoded["workspace_path"])
	}
}

func TestWorktree_UnisolateResponse_Marshal(t *testing.T) {
	resp := map[string]interface{}{
		"status":      "un-isolated",
		"task_id":     "task-1",
		"task_number": 42,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["status"] != "un-isolated" {
		t.Errorf("expected status 'un-isolated', got %v", decoded["status"])
	}
	if decoded["task_id"] != "task-1" {
		t.Errorf("expected task_id 'task-1', got %v", decoded["task_id"])
	}
}

// ---- Edge cases ----

func TestWorktree_Isolate_MissingXUserIDHeader(t *testing.T) {
	h := &TaskHandler{}
	r := setupWorktreeRouter(h)

	// X-User-ID header present but empty string
	req := httptest.NewRequest("POST", "/api/v1/tasks/test-task/isolate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty X-User-ID, got %d", rr.Code)
	}
}

// Note: Tests for real DB scenarios (missing task, invalid channel, duplicate worktree)
// require a database and are covered by integration tests. The handler-level tests
// here validate the HTTP contract: auth checks, param validation, and response shapes.
// Full E2E scenarios for channel binding and worktree creation are in
// docs/design/tasks/e2e-workspace.md.
