package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

func setupRelationshipRouter(h *AgentRelationshipHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/agent-relationships", func(r chi.Router) {
		r.Post("/", h.Create)
	})
	return r
}

func TestAgentRelationshipHandler_CreateValidation(t *testing.T) {
	h := NewAgentRelationshipHandler(service.NewAgentRelationshipService(nil))
	r := setupRelationshipRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       bool
		wantStatus int
	}{
		{"missing auth", `{"from_agent_id":"a","to_agent_id":"b","rel_type":"assigns_to"}`, false, http.StatusUnauthorized},
		{"bad json", `{`, true, http.StatusBadRequest},
		{"bad type", `{"from_agent_id":"a","to_agent_id":"b","rel_type":"reports_to"}`, true, http.StatusBadRequest},
		{"self", `{"from_agent_id":"a","to_agent_id":"a","rel_type":"assigns_to"}`, true, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/agent-relationships", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth {
				req.Header.Set("X-User-ID", "user-1")
			}
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}
