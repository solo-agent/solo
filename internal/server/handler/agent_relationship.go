package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

type AgentRelationshipHandler struct {
	svc *service.AgentRelationshipService
}

func NewAgentRelationshipHandler(svc *service.AgentRelationshipService) *AgentRelationshipHandler {
	return &AgentRelationshipHandler{svc: svc}
}

func (h *AgentRelationshipHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req service.CreateRelationshipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rel, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		writeRelationshipError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

func (h *AgentRelationshipHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rels, err := h.svc.List(r.Context(), userID, r.URL.Query().Get("agent_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

func (h *AgentRelationshipHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rels, err := h.svc.List(r.Context(), userID, chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

func (h *AgentRelationshipHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req service.UpdateRelationshipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rel, err := h.svc.Update(r.Context(), userID, chi.URLParam(r, "relationshipID"), req)
	if err != nil {
		writeRelationshipError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *AgentRelationshipHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.svc.Delete(r.Context(), userID, chi.URLParam(r, "relationshipID")); err != nil {
		writeRelationshipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeRelationshipError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrRelationshipNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
