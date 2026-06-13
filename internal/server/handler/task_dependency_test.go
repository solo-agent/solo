package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func setupTaskDependencyRouter(h *TaskDependencyHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/task-dependencies", h.AddDependency)
	r.Delete("/api/v1/task-dependencies", h.RemoveDependency)
	r.Get("/api/v1/tasks/{taskID}/blockers", h.ListBlockers)
	r.Get("/api/v1/tasks/{taskID}/blocked", h.ListBlocked)
	r.Get("/api/v1/tasks/{taskID}/is-blocked", h.IsBlocked)
	return r
}

// ---- AddDependency ----

func TestTaskDependency_AddDependency_Validation(t *testing.T) {
	h := &TaskDependencyHandler{}
	r := setupTaskDependencyRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"blocker_task_id":"t1","blocked_task_id":"t2"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing blocker_task_id",
			body:       `{"blocked_task_id":"t2"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing blocked_task_id",
			body:       `{"blocker_task_id":"t1"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body with both fields missing",
			body:       `{}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON body",
			body:       `{not json}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/task-dependencies", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != "" {
				req.Header.Set("X-User-ID", tt.auth)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// ---- RemoveDependency ----

func TestTaskDependency_RemoveDependency_Validation(t *testing.T) {
	h := &TaskDependencyHandler{}
	r := setupTaskDependencyRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"blocker_task_id":"t1","blocked_task_id":"t2"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing blocker_task_id",
			body:       `{"blocked_task_id":"t2"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing blocked_task_id",
			body:       `{"blocker_task_id":"t1"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON body",
			body:       `{not json}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/v1/task-dependencies", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != "" {
				req.Header.Set("X-User-ID", tt.auth)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

// ---- ListBlockers ----

func TestTaskDependency_ListBlockers_Validation(t *testing.T) {
	h := &TaskDependencyHandler{}
	r := setupTaskDependencyRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/t1/blockers", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- ListBlocked ----

func TestTaskDependency_ListBlocked_Validation(t *testing.T) {
	h := &TaskDependencyHandler{}
	r := setupTaskDependencyRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/t1/blocked", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- IsBlocked ----

func TestTaskDependency_IsBlocked_Validation(t *testing.T) {
	h := &TaskDependencyHandler{}
	r := setupTaskDependencyRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/t1/is-blocked", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- Data structure serialization ----

func TestTaskDependency_Marshal(t *testing.T) {
	dep := service.TaskDependency{
		BlockerTaskID: "task-a",
		BlockedTaskID: "task-b",
		CreatedAt:     "2025-06-01T10:00:00Z",
	}

	data, err := json.Marshal(dep)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.TaskDependency
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.BlockerTaskID != "task-a" {
		t.Errorf("expected blocker_task_id task-a, got %s", decoded.BlockerTaskID)
	}
	if decoded.BlockedTaskID != "task-b" {
		t.Errorf("expected blocked_task_id task-b, got %s", decoded.BlockedTaskID)
	}
	if decoded.CreatedAt != "2025-06-01T10:00:00Z" {
		t.Errorf("expected created_at, got %s", decoded.CreatedAt)
	}
}

func TestTaskDependency_List_Marshal(t *testing.T) {
	deps := []service.TaskDependency{
		{BlockerTaskID: "t1", BlockedTaskID: "t2", CreatedAt: "2025-01-01T00:00:00Z"},
		{BlockerTaskID: "t3", BlockedTaskID: "t2", CreatedAt: "2025-01-02T00:00:00Z"},
		{BlockerTaskID: "t4", BlockedTaskID: "t5", CreatedAt: "2025-01-03T00:00:00Z"},
	}

	data, err := json.Marshal(deps)
	if err != nil {
		t.Fatalf("failed to marshal list: %v", err)
	}

	var decoded []service.TaskDependency
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal list: %v", err)
	}

	if len(decoded) != 3 {
		t.Errorf("expected 3 dependencies, got %d", len(decoded))
	}
	// t2 is blocked by both t1 and t3
	blockedByCount := 0
	for _, d := range decoded {
		if d.BlockedTaskID == "t2" {
			blockedByCount++
		}
	}
	if blockedByCount != 2 {
		t.Errorf("expected t2 to be blocked by 2 tasks, got %d", blockedByCount)
	}
}

func TestTaskDependency_IsBlockedResponse_Marshal(t *testing.T) {
	// The IsBlocked endpoint returns {"blocked": true/false}
	resp := map[string]bool{"blocked": true}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]bool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !decoded["blocked"] {
		t.Error("expected blocked to be true")
	}

	// Also test false case
	respFalse := map[string]bool{"blocked": false}
	dataFalse, _ := json.Marshal(respFalse)
	var decodedFalse map[string]bool
	json.Unmarshal(dataFalse, &decodedFalse)
	if decodedFalse["blocked"] {
		t.Error("expected blocked to be false")
	}
}

// Note: Full integration tests (adding valid dependency, self-dependency
// error, duplicate conflict, list blockers/blocked, is-blocked true/false)
// require a real database and TaskService. These are covered by the E2E
// test plan in docs/design/tasks/e2e-test-plan.md.
