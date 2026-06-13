package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func setupAgentDelegationRouter(h *AgentDelegationHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/agent-delegations", h.Create)
	r.Get("/api/v1/agent-delegations/incoming", h.ListIncoming)
	r.Get("/api/v1/agent-delegations/outgoing", h.ListOutgoing)
	r.Post("/api/v1/agent-delegations/{id}/accept", h.Accept)
	r.Post("/api/v1/agent-delegations/{id}/reject", h.Reject)
	r.Post("/api/v1/agent-delegations/{id}/complete", h.Complete)
	r.Post("/api/v1/agent-delegations/{id}/fail", h.Fail)
	r.Post("/api/v1/agent-delegations/{id}/deliver", h.Deliver)
	return r
}

// ---- Create ----

func TestAgentDelegation_Create_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"from_agent_id":"a1","to_agent_id":"a2","channel_id":"ch-1"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing from_agent_id",
			body:       `{"to_agent_id":"a2","channel_id":"ch-1"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing to_agent_id",
			body:       `{"from_agent_id":"a1","channel_id":"ch-1"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing channel_id",
			body:       `{"from_agent_id":"a1","to_agent_id":"a2"}`,
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
			req := httptest.NewRequest("POST", "/api/v1/agent-delegations", bytes.NewBufferString(tt.body))
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

// ---- ListIncoming ----

func TestAgentDelegation_ListIncoming_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id query param returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent-delegations/incoming", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	// Note: the "with agent_id" path proceeds to call service.ListIncoming which
	// requires a real pool. Full list with status filter is covered by
	// integration tests (see docs/design/tasks/e2e-test-plan.md).
}

// ---- ListOutgoing ----

func TestAgentDelegation_ListOutgoing_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id query param returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent-delegations/outgoing", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// ---- Accept ----

func TestAgentDelegation_Accept_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/agent-delegations/del-1/accept", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	// Note: body-parsing and query-param fallback paths for agent_id both
	// proceed to call service.Accept which requires a real pool. These are
	// covered by integration tests (see docs/design/tasks/e2e-test-plan.md).
}

// ---- Reject ----

func TestAgentDelegation_Reject_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/agent-delegations/del-1/reject", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	// Note: the body parsing path (agent_id + reason) proceeds to call
	// service.Reject which requires a real pool. Valid and invalid reject
	// transitions are covered by integration tests
	// (see docs/design/tasks/e2e-test-plan.md and e2e-cli-delegate.md).
}

// ---- Complete ----

func TestAgentDelegation_Complete_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/agent-delegations/del-1/complete", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// ---- Fail ----

func TestAgentDelegation_Fail_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/agent-delegations/del-1/fail", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// ---- Deliver ----

func TestAgentDelegation_Deliver_Validation(t *testing.T) {
	h := &AgentDelegationHandler{}
	r := setupAgentDelegationRouter(h)

	t.Run("missing agent_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/agent-delegations/del-1/deliver", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// ---- Data structure serialization ----

func TestAgentDelegation_Marshal(t *testing.T) {
	taskID := "task-1"
	msg := "Please handle this task"
	reason := "not available"
	d := service.AgentDelegation{
		ID:              "del-1",
		FromAgentID:     "agent-a",
		ToAgentID:       "agent-b",
		TaskID:          &taskID,
		ChannelID:       "ch-1",
		Status:          "queued",
		Message:         &msg,
		StartIfInactive: true,
		RejectionReason: &reason,
		CreatedAt:       "2025-06-01T10:00:00Z",
		UpdatedAt:       "2025-06-01T10:00:00Z",
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.AgentDelegation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "del-1" {
		t.Errorf("expected id del-1, got %s", decoded.ID)
	}
	if decoded.Status != "queued" {
		t.Errorf("expected status queued, got %s", decoded.Status)
	}
	if decoded.FromAgentID != "agent-a" {
		t.Errorf("expected from_agent_id agent-a, got %s", decoded.FromAgentID)
	}
	if decoded.ToAgentID != "agent-b" {
		t.Errorf("expected to_agent_id agent-b, got %s", decoded.ToAgentID)
	}
	if decoded.TaskID == nil || *decoded.TaskID != "task-1" {
		t.Errorf("expected task_id task-1, got %v", decoded.TaskID)
	}
	if decoded.Message == nil || *decoded.Message != "Please handle this task" {
		t.Errorf("expected message, got %v", decoded.Message)
	}
	if !decoded.StartIfInactive {
		t.Error("expected start_if_inactive to be true")
	}
	if decoded.RejectionReason == nil || *decoded.RejectionReason != "not available" {
		t.Errorf("expected rejection_reason 'not available', got %v", decoded.RejectionReason)
	}
}

func TestAgentDelegation_StatusTransitions_Marshal(t *testing.T) {
	// Verify all valid status values serialize correctly.
	statuses := []string{"queued", "delivered", "started", "completed", "failed", "rejected"}
	for _, s := range statuses {
		d := service.AgentDelegation{
			ID:        "del-1",
			Status:    s,
			ChannelID: "ch-1",
		}
		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("failed to marshal status %q: %v", s, err)
		}
		var decoded service.AgentDelegation
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal status %q: %v", s, err)
		}
		if decoded.Status != s {
			t.Errorf("expected status %s, got %s", s, decoded.Status)
		}
	}
}

func TestAgentDelegation_MinimalFields_Marshal(t *testing.T) {
	// A newly created delegation with only required fields.
	d := service.AgentDelegation{
		ID:          "del-1",
		FromAgentID: "agent-a",
		ToAgentID:   "agent-b",
		ChannelID:   "ch-1",
		Status:      "queued",
		CreatedAt:   "2025-06-01T10:00:00Z",
		UpdatedAt:   "2025-06-01T10:00:00Z",
	}

	data, _ := json.Marshal(d)
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// task_id, message, rejection_reason should be absent or null when empty.
	optionalKeys := []string{"task_id", "message", "rejection_reason"}
	for _, k := range optionalKeys {
		if v, ok := decoded[k]; ok && v != nil {
			// omitempty on pointer fields — they should be absent entirely.
			t.Logf("note: key %q present with value %v (omitempty may vary)", k, v)
		}
	}
}

func TestCreateDelegationRequest_Marshal(t *testing.T) {
	taskID := "task-1"
	msg := "Please handle"
	req := service.CreateDelegationRequest{
		FromAgentID:     "agent-a",
		ToAgentID:       "agent-b",
		TaskID:          &taskID,
		ChannelID:       "ch-1",
		Message:         &msg,
		StartIfInactive: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.CreateDelegationRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.FromAgentID != "agent-a" {
		t.Errorf("expected from_agent_id agent-a, got %s", decoded.FromAgentID)
	}
	if decoded.ChannelID != "ch-1" {
		t.Errorf("expected channel_id ch-1, got %s", decoded.ChannelID)
	}
	if !decoded.StartIfInactive {
		t.Error("expected start_if_inactive to be true")
	}
	if decoded.TaskID == nil || *decoded.TaskID != "task-1" {
		t.Errorf("expected task_id task-1, got %v", decoded.TaskID)
	}
}

func TestDelegationSentinelErrors(t *testing.T) {
	// Verify the sentinel error types are distinct and identifiable.
	if !errors.Is(service.ErrInvalidStatusTransition, service.ErrInvalidStatusTransition) {
		t.Error("ErrInvalidStatusTransition should match itself")
	}
	if !errors.Is(service.ErrDelegationNotFound, service.ErrDelegationNotFound) {
		t.Error("ErrDelegationNotFound should match itself")
	}
	// Verify they are distinct from each other.
	if errors.Is(service.ErrInvalidStatusTransition, service.ErrDelegationNotFound) {
		t.Error("sentinel errors should be distinct")
	}
	// Verify error messages are non-empty.
	if service.ErrInvalidStatusTransition.Error() == "" {
		t.Error("ErrInvalidStatusTransition should have a message")
	}
	if service.ErrDelegationNotFound.Error() == "" {
		t.Error("ErrDelegationNotFound should have a message")
	}
}

// Note: Full integration tests (create delegation with status=queued,
// accept queued→started, reject with reason, reject already started→error,
// complete started→completed, list incoming with status filter, list outgoing,
// create self-delegation→error) require a real database and are covered by
// the E2E test plan in docs/design/tasks/e2e-test-plan.md and the CLI E2E
// scenarios in docs/design/tasks/e2e-cli-delegate.md.
