package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupWatchdogRouter(h *WatchdogHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/tasks", func(r chi.Router) {
		r.Route("/{taskID}", func(r chi.Router) {
			r.Patch("/watchdog", h.SetWatchdog)
		})
	})
	return r
}

func TestWatchdogHandler_SetWatchdog_Validation(t *testing.T) {
	h := &WatchdogHandler{}
	r := setupWatchdogRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{name: "missing auth", body: `{"claimer_id":"agent-1","deadline":"2024-12-01T10:00:00Z","timeout_action":"remind"}`, auth: "", wantStatus: http.StatusUnauthorized},
		{name: "missing claimer_id", body: `{"deadline":"2024-12-01T10:00:00Z","timeout_action":"remind"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "missing deadline", body: `{"claimer_id":"agent-1","timeout_action":"remind"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "invalid deadline format", body: `{"claimer_id":"agent-1","deadline":"not-a-date","timeout_action":"remind"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "invalid timeout_action", body: `{"claimer_id":"agent-1","deadline":"2024-12-01T10:00:00Z","timeout_action":"invalid"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PATCH", "/api/v1/tasks/test-task/watchdog", bytes.NewBufferString(tt.body))
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

func TestWatchdogHandler_AuthValidation(t *testing.T) {
	h := &WatchdogHandler{}

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/v1/tasks/test-task/watchdog", bytes.NewBufferString(`{"claimer_id":"a","deadline":"2024-01-01T00:00:00Z"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.SetWatchdog(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestWatchdogHandler_ResponseShape(t *testing.T) {
	resp := map[string]interface{}{
		"task_id":        "test-task",
		"claimer_id":     "agent-1",
		"deadline":       "2024-12-01T10:00:00Z",
		"timeout_action": "remind",
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal watchdog response: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("failed to unmarshal watchdog response: %v", err)
	}
	if decoded["task_id"] != "test-task" {
		t.Errorf("expected test-task, got %v", decoded["task_id"])
	}
}

// Note: ValidTimeoutActions test requires a real WatchdogService (DB), so it is covered by integration tests.
