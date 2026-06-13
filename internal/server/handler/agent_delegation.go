package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
)

type AgentDelegationHandler struct {
	svc *service.AgentDelegationService
}

func NewAgentDelegationHandler(pool *pgxpool.Pool) *AgentDelegationHandler {
	return &AgentDelegationHandler{svc: service.NewAgentDelegationService(pool)}
}

// agentIDFromRequest returns the agent ID from query param, falling back to X-User-ID.
func agentIDFromRequest(r *http.Request) string {
	if id := r.URL.Query().Get("agent_id"); id != "" {
		return id
	}
	return r.Header.Get("X-User-ID")
}

// Create handles POST /api/v1/agent-delegations
func (h *AgentDelegationHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	var req service.CreateDelegationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FromAgentID == "" || req.ToAgentID == "" || req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "from_agent_id, to_agent_id, and channel_id are required")
		return
	}

	d, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// Accept handles POST /api/v1/agent-delegations/{id}/accept
func (h *AgentDelegationHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.Accept(r.Context(), id, agentID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Reject handles POST /api/v1/agent-delegations/{id}/reject
func (h *AgentDelegationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Body is optional for reject
	}

	d, err := h.svc.Reject(r.Context(), id, agentID, body.Reason)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Complete handles POST /api/v1/agent-delegations/{id}/complete
func (h *AgentDelegationHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.MarkComplete(r.Context(), id, agentID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Fail handles POST /api/v1/agent-delegations/{id}/fail
func (h *AgentDelegationHandler) Fail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.MarkFailed(r.Context(), id, agentID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ListIncoming handles GET /api/v1/agent-delegations/incoming?agent_id=...&status=...
func (h *AgentDelegationHandler) ListIncoming(w http.ResponseWriter, r *http.Request) {
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	status := r.URL.Query().Get("status")

	delegations, err := h.svc.ListIncoming(r.Context(), agentID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, delegations)
}

// ListOutgoing handles GET /api/v1/agent-delegations/outgoing?agent_id=...&status=...
func (h *AgentDelegationHandler) ListOutgoing(w http.ResponseWriter, r *http.Request) {
	agentID := agentIDFromRequest(r)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	status := r.URL.Query().Get("status")

	delegations, err := h.svc.ListOutgoing(r.Context(), agentID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, delegations)
}
