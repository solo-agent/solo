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

func NewAgentRelationshipHandler(pool *pgxpool.Pool) *AgentRelationshipHandler {
	return &AgentRelationshipHandler{svc: service.NewAgentRelationshipService(pool)}
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
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "relationship ID is required")
		return
	}

	var body struct {
		Weight *float64 `json:"weight,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Weight == nil {
		writeError(w, http.StatusBadRequest, "weight is required")
		return
	}

	rel, err := h.svc.UpdateWeight(r.Context(), id, *body.Weight)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// Delete handles DELETE /api/v1/agent-relationships/{id}
func (h *AgentRelationshipHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
