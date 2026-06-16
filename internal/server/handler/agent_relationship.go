package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

type AgentRelationshipHandler struct {
	svc *service.AgentRelationshipService
}

func NewAgentRelationshipHandler(pool *pgxpool.Pool, svc *service.AgentRelationshipService) *AgentRelationshipHandler {
	if svc == nil {
		svc = service.NewAgentRelationshipService(pool)
	}
	return &AgentRelationshipHandler{svc: svc}
}

// Create handles POST /api/v1/agent-relationships
func (h *AgentRelationshipHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	var req service.CreateRelationshipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FromAgentID == "" || req.ToAgentID == "" || req.RelType == "" {
		writeError(w, http.StatusBadRequest, "from_agent_id, to_agent_id, and rel_type are required")
		return
	}

	rel, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

// ListByAgent handles GET /api/v1/agents/{agentID}/relationships
func (h *AgentRelationshipHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required")
		return
	}

	rels, err := h.svc.ListByAgent(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

// ListByChannel handles GET /api/v1/channels/{channelID}/relationships
func (h *AgentRelationshipHandler) ListByChannel(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	rels, err := h.svc.ListByChannel(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

// Update handles PATCH /api/v1/agent-relationships/{id}
func (h *AgentRelationshipHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "relationship ID is required")
		return
	}

	var body struct {
		Weight      *float64 `json:"weight,omitempty"`
		Instruction *string  `json:"instruction,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Weight != nil {
		rel, err := h.svc.UpdateWeight(r.Context(), id, *body.Weight)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rel)
		return
	}
	if body.Instruction != nil {
		rel, err := h.svc.UpdateInstruction(r.Context(), id, *body.Instruction)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rel)
		return
	}

	writeError(w, http.StatusBadRequest, "weight or instruction is required")
}

// Delete handles DELETE /api/v1/agent-relationships/{id}
func (h *AgentRelationshipHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "relationship ID is required")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /api/v1/agent-relationships (T1.1.3).
// Supports query filters: from_agent_id, to_agent_id, rel_type, channel_id.
func (h *AgentRelationshipHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rels, err := h.svc.List(r.Context(),
		r.URL.Query().Get("from_agent_id"),
		r.URL.Query().Get("to_agent_id"),
		r.URL.Query().Get("rel_type"),
		r.URL.Query().Get("channel_id"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

// CheckCycle handles POST /api/v1/agent-relationships/check-cycle (T1.1.3).
func (h *AgentRelationshipHandler) CheckCycle(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		FromAgentID string `json:"from_agent_id"`
		ToAgentID   string `json:"to_agent_id"`
		RelType     string `json:"rel_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FromAgentID == "" || req.ToAgentID == "" || req.RelType == "" {
		writeError(w, http.StatusBadRequest, "from_agent_id, to_agent_id, and rel_type are required")
		return
	}

	hasCycle, err := h.svc.CheckCycle(r.Context(), req.FromAgentID, req.ToAgentID, req.RelType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"has_cycle": hasCycle})
}

// GraphNode represents a node in the relationship graph.
type GraphNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// GraphEdge represents an edge in the relationship graph.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// GraphData is the response for GET /api/v1/relationships/graph.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph handles GET /api/v1/relationships/graph
func (h *AgentRelationshipHandler) Graph(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := r.URL.Query().Get("channel_id")

	rels, err := h.svc.List(r.Context(), "", "", "", channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build node set from relationships.
	nodeMap := make(map[string]*GraphNode)
	for _, rel := range rels {
		if _, exists := nodeMap[rel.FromAgentID]; !exists {
			name := resolveAgentName(r, h.svc, rel.FromAgentID)
			nodeMap[rel.FromAgentID] = &GraphNode{
				ID:     rel.FromAgentID,
				Name:   name,
				Status: "active",
			}
		}
		if _, exists := nodeMap[rel.ToAgentID]; !exists {
			name := resolveAgentName(r, h.svc, rel.ToAgentID)
			nodeMap[rel.ToAgentID] = &GraphNode{
				ID:     rel.ToAgentID,
				Name:   name,
				Status: "active",
			}
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}

	edges := make([]GraphEdge, 0, len(rels))
	for _, rel := range rels {
		edges = append(edges, GraphEdge{
			From: rel.FromAgentID,
			To:   rel.ToAgentID,
			Type: rel.RelType,
		})
	}

	writeJSON(w, http.StatusOK, GraphData{
		Nodes: nodes,
		Edges: edges,
	})
}

// resolveAgentName looks up an agent name; falls back to truncated ID.
func resolveAgentName(r *http.Request, svc *service.AgentRelationshipService, agentID string) string {
	// Try to get name from relationships — we have it in service.
	// For now return truncated ID.
	if len(agentID) > 8 {
		return agentID[:8]
	}
	return agentID
}
