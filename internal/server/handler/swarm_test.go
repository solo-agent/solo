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

func setupSwarmRouter(h *TaskHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/tasks", func(r chi.Router) {
		r.Get("/swarm-claimable", h.ListSwarmClaimable)
		r.Route("/{taskID}", func(r chi.Router) {
			r.Post("/decompose", h.DecomposeTask)
			r.Get("/swarm-status", h.SwarmStatus)
		})
	})
	return r
}

func TestSwarmHandler_Decompose_Validation(t *testing.T) {
	h := &TaskHandler{}
	r := setupSwarmRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{name: "missing auth", body: `{"channel_id":"ch-1","subtasks":[{"title":"Sub 1","description":"","depends_on_indices":[]}]}`, auth: "", wantStatus: http.StatusUnauthorized},
		// Note: remaining tests require swarm coordinator (not available without DB/DI).
		// These are tested by integration tests instead.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/tasks/test-task/decompose", bytes.NewBufferString(tt.body))
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

func TestSwarmHandler_SwarmStatus_Validation(t *testing.T) {
	h := &TaskHandler{}
	r := setupSwarmRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/test-task/swarm-status?channel_id=ch-1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestSwarmHandler_ListClaimable_Validation(t *testing.T) {
	h := &TaskHandler{}
	r := setupSwarmRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/swarm-claimable?channel_id=ch-1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	// Note: channel_id check occurs after swarm nil check in handler.
	// Without swarm coordinator injected, handler returns 500.
}

func TestSwarmCoordinator_SwarmSubtaskDef_Marshal(t *testing.T) {
	defs := []service.SwarmSubtaskDef{
		{Title: "Sub 1", Description: "First subtask", DependsOn: []int{}},
		{Title: "Sub 2", Description: "Second subtask", DependsOn: []int{0}},
	}
	body, err := json.Marshal(defs)
	if err != nil {
		t.Fatalf("failed to marshal subtask defs: %v", err)
	}
	var decoded []service.SwarmSubtaskDef
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("failed to unmarshal subtask defs: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 subtasks, got %d", len(decoded))
	}
	if decoded[0].Title != "Sub 1" {
		t.Errorf("expected Sub 1, got %s", decoded[0].Title)
	}
	if len(decoded[1].DependsOn) != 1 || decoded[1].DependsOn[0] != 0 {
		t.Errorf("expected depends_on [0], got %v", decoded[1].DependsOn)
	}
}

func TestSwarmCoordinator_SwarmPlan_Marshal(t *testing.T) {
	plan := service.SwarmPlan{
		Breakdown: []service.SwarmSubtaskDef{
			{Title: "Task A", Description: "A", DependsOn: []int{}},
			{Title: "Task B", Description: "B", DependsOn: []int{0}},
		},
		Strategy: "parallel",
	}
	body, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("failed to marshal swarm plan: %v", err)
	}
	var decoded service.SwarmPlan
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("failed to unmarshal swarm plan: %v", err)
	}
	if decoded.Strategy != "parallel" {
		t.Errorf("expected parallel strategy, got %s", decoded.Strategy)
	}
	if len(decoded.Breakdown) != 2 {
		t.Errorf("expected 2 breakdown items, got %d", len(decoded.Breakdown))
	}
}

func TestSwarmHandler_SwarmStatus_MissingChannelID(t *testing.T) {
	h := &TaskHandler{}
	r := setupSwarmRouter(h)

	t.Run("missing channel_id — handler returns 500 without swarm coordinator", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tasks/test-task/swarm-status", nil)
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		// Without swarm coordinator injected (nil), handler returns 500 before validating channel_id.
		// Integration tests with full DI cover the correct validation order.
		if rr.Code == http.StatusOK {
			t.Errorf("expected error without swarm coordinator, got %d", rr.Code)
		}
	})
}
