package handler

import (
	"encoding/json"
	"errors"
	"io"
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

// agentIDFromRequest returns the agent ID from query param only.
// BUG-001: No longer falls back to X-User-ID (human UUID != agent UUID).
func agentIDFromRequest(r *http.Request) string {
	return r.URL.Query().Get("agent_id")
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
		writeDelegationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// Accept handles POST /api/v1/agent-delegations/{id}/accept
func (h *AgentDelegationHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// BUG-001: Read agent_id from request body, not X-User-ID.
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	agentID := body.AgentID
	if agentID == "" {
		agentID = agentIDFromRequest(r) // fallback to query param
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.Accept(r.Context(), id, agentID)
	if err != nil {
		writeDelegationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Reject handles POST /api/v1/agent-delegations/{id}/reject
func (h *AgentDelegationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// BUG-001: Read agent_id from request body.
	var body struct {
		AgentID string `json:"agent_id"`
		Reason  string `json:"reason"`
	}
	// BUG-007: Check json.Decode error (body is optional for reject).
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID := body.AgentID
	if agentID == "" {
		agentID = agentIDFromRequest(r) // fallback to query param
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.Reject(r.Context(), id, agentID, body.Reason)
	if err != nil {
		writeDelegationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Complete handles POST /api/v1/agent-delegations/{id}/complete
func (h *AgentDelegationHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// BUG-001: Read agent_id from request body.
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	agentID := body.AgentID
	if agentID == "" {
		agentID = agentIDFromRequest(r) // fallback to query param
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.MarkComplete(r.Context(), id, agentID)
	if err != nil {
		writeDelegationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Fail handles POST /api/v1/agent-delegations/{id}/fail
func (h *AgentDelegationHandler) Fail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// BUG-001: Read agent_id from request body.
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	agentID := body.AgentID
	if agentID == "" {
		agentID = agentIDFromRequest(r) // fallback to query param
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.MarkFailed(r.Context(), id, agentID)
	if err != nil {
		writeDelegationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// Deliver handles POST /api/v1/agent-delegations/{id}/deliver (BUG-009).
func (h *AgentDelegationHandler) Deliver(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	agentID := body.AgentID
	if agentID == "" {
		agentID = agentIDFromRequest(r)
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	d, err := h.svc.MarkDelivered(r.Context(), id, agentID)
	if err != nil {
		writeDelegationError(w, err)
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

// ListByAgent handles GET /api/v1/agents/{agentID}/delegations?direction=incoming|outgoing (T2.1.6).
func (h *AgentDelegationHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required")
		return
	}
	status := r.URL.Query().Get("status")
	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "incoming"
	}

	var delegations []service.AgentDelegation
	var err error
	switch direction {
	case "incoming":
		delegations, err = h.svc.ListIncoming(r.Context(), agentID, status)
	case "outgoing":
		delegations, err = h.svc.ListOutgoing(r.Context(), agentID, status)
	default:
		writeError(w, http.StatusBadRequest, "direction must be 'incoming' or 'outgoing'")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, delegations)
}

// writeDelegationError writes an appropriate HTTP error for delegation operations (BUG-013).
func writeDelegationError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrInvalidStatusTransition) || errors.Is(err, service.ErrDelegationNotFound) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
