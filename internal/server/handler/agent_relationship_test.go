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

func setupAgentRelationshipRouter(h *AgentRelationshipHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/agent-relationships", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/check-cycle", h.CheckCycle)
		r.Route("/{id}", func(r chi.Router) {
			r.Patch("/", h.Update)
			r.Delete("/", h.Delete)
		})
	})
	r.Get("/api/v1/agents/{agentID}/relationships", h.ListByAgent)
	r.Get("/api/v1/channels/{channelID}/relationships", h.ListByChannel)
	r.Get("/api/v1/relationships/graph", h.Graph)
	return r
}

// ---- Create ----

func TestAgentRelationship_Create_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"from_agent_id":"a1","to_agent_id":"a2","rel_type":"reports_to"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing from_agent_id",
			body:       `{"to_agent_id":"a2","rel_type":"reports_to"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing to_agent_id",
			body:       `{"from_agent_id":"a1","rel_type":"reports_to"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing rel_type",
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
		// Note: the "valid create" path proceeds to call service.Create which
		// requires a real pool. Full create scenarios (all 4 rel types,
		// self-referential, invalid rel_type, channel scope rules) are
		// covered by integration tests (see docs/design/tasks/e2e-test-plan.md).
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/agent-relationships", bytes.NewBufferString(tt.body))
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

// ---- List ----

func TestAgentRelationship_List_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent-relationships", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	// Note: with auth, List proceeds to call service.List which requires
	// a real pool. The success path is covered by integration tests
	// (see docs/design/tasks/e2e-test-plan.md).
}

// ---- ListByAgent ----

func TestAgentRelationship_ListByAgent_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/a1/relationships", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- ListByChannel ----

func TestAgentRelationship_ListByChannel_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/relationships", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- CheckCycle ----

func TestAgentRelationship_CheckCycle_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"from_agent_id":"a1","to_agent_id":"a2","rel_type":"reports_to"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing from_agent_id",
			body:       `{"to_agent_id":"a2","rel_type":"reports_to"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing to_agent_id",
			body:       `{"from_agent_id":"a1","rel_type":"reports_to"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing rel_type",
			body:       `{"from_agent_id":"a1","to_agent_id":"a2"}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/agent-relationships/check-cycle", bytes.NewBufferString(tt.body))
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

// ---- Update (weight) ----

func TestAgentRelationship_Update_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"weight":3.5}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing weight field",
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
			req := httptest.NewRequest("PATCH", "/api/v1/agent-relationships/rel-1", bytes.NewBufferString(tt.body))
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

// ---- Delete ----

func TestAgentRelationship_Delete_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/agent-relationships/rel-1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- Graph ----

func TestAgentRelationship_Graph_Validation(t *testing.T) {
	h := &AgentRelationshipHandler{}
	r := setupAgentRelationshipRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/relationships/graph", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- Data structure serialization ----

func TestAgentRelationship_Marshal(t *testing.T) {
	channelID := "ch-1"
	rel := service.AgentRelationship{
		ID:          "rel-1",
		FromAgentID: "agent-a",
		ToAgentID:   "agent-b",
		RelType:     "reports_to",
		ChannelID:   &channelID,
		Weight:      2.5,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-02T00:00:00Z",
	}

	data, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.AgentRelationship
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "rel-1" {
		t.Errorf("expected id rel-1, got %s", decoded.ID)
	}
	if decoded.FromAgentID != "agent-a" {
		t.Errorf("expected from_agent_id agent-a, got %s", decoded.FromAgentID)
	}
	if decoded.RelType != "reports_to" {
		t.Errorf("expected rel_type reports_to, got %s", decoded.RelType)
	}
	if decoded.Weight != 2.5 {
		t.Errorf("expected weight 2.5, got %f", decoded.Weight)
	}
	if decoded.ChannelID == nil || *decoded.ChannelID != "ch-1" {
		t.Errorf("expected channel_id ch-1, got %v", decoded.ChannelID)
	}
}

func TestAgentRelationship_NoChannelID_Marshal(t *testing.T) {
	// Global relationships (reports_to, escalates_to) must have nil channel_id.
	rel := service.AgentRelationship{
		ID:          "rel-2",
		FromAgentID: "agent-a",
		ToAgentID:   "agent-b",
		RelType:     "escalates_to",
		ChannelID:   nil,
		Weight:      1.0,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-01T00:00:00Z",
	}

	data, _ := json.Marshal(rel)
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	// channel_id should be absent or null for global rels
	if cid, ok := decoded["channel_id"]; ok && cid != nil {
		t.Errorf("expected channel_id to be absent/null, got %v", cid)
	}
}

func TestAgentRelationship_AllRelTypes_Marshal(t *testing.T) {
	rels := []service.AgentRelationship{
		{ID: "r1", FromAgentID: "a1", ToAgentID: "a2", RelType: "reports_to", Weight: 1.0},
		{ID: "r2", FromAgentID: "a1", ToAgentID: "a3", RelType: "escalates_to", Weight: 2.0},
		{ID: "r3", FromAgentID: "a1", ToAgentID: "a4", RelType: "delegates_to", Weight: 3.0},
		{ID: "r4", FromAgentID: "a1", ToAgentID: "a5", RelType: "collaborates_with", Weight: 4.0},
	}

	data, err := json.Marshal(rels)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded []service.AgentRelationship
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded) != 4 {
		t.Errorf("expected 4 relationships, got %d", len(decoded))
	}
	for i, r := range decoded {
		if r.RelType != rels[i].RelType {
			t.Errorf("expected rel_type %s at index %d, got %s", rels[i].RelType, i, r.RelType)
		}
	}
}

func TestCreateRelationshipRequest_Marshal(t *testing.T) {
	channelID := "ch-1"
	req := service.CreateRelationshipRequest{
		FromAgentID: "agent-a",
		ToAgentID:   "agent-b",
		RelType:     "delegates_to",
		ChannelID:   &channelID,
		Weight:      5.0,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.CreateRelationshipRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.FromAgentID != "agent-a" {
		t.Errorf("expected from_agent_id agent-a, got %s", decoded.FromAgentID)
	}
	if decoded.RelType != "delegates_to" {
		t.Errorf("expected rel_type delegates_to, got %s", decoded.RelType)
	}
	if decoded.ChannelID == nil || *decoded.ChannelID != "ch-1" {
		t.Errorf("expected channel_id ch-1, got %v", decoded.ChannelID)
	}
}

func TestGraphData_Marshal(t *testing.T) {
	graph := GraphData{
		Nodes: []GraphNode{
			{ID: "agent-a", Name: "Alice", Status: "active"},
			{ID: "agent-b", Name: "Bob", Status: "active"},
		},
		Edges: []GraphEdge{
			{From: "agent-a", To: "agent-b", Type: "reports_to"},
		},
	}

	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded GraphData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(decoded.Nodes))
	}
	if len(decoded.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(decoded.Edges))
	}
	if decoded.Edges[0].Type != "reports_to" {
		t.Errorf("expected edge type reports_to, got %s", decoded.Edges[0].Type)
	}
}

func TestAgentRelationship_ErrorResponseShape(t *testing.T) {
	// Verify the standard error response format used by writeError.
	// This is the shape handlers return for validation failures.
	errResp := ErrorResponse{
		Error:   "Bad Request",
		Message: "from_agent_id, to_agent_id, and rel_type are required",
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Error != "Bad Request" {
		t.Errorf("expected Error 'Bad Request', got %s", decoded.Error)
	}
	if decoded.Message == "" {
		t.Error("expected non-empty Message")
	}
}

// Note: Full integration tests (creating valid relationships of all 4 types,
// self-referential error, invalid rel_type error, channel_id scope rules,
// cycle detection, list/delete operations) require a real database and are
// covered by the E2E test plan in docs/design/tasks/e2e-test-plan.md.
