package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/solo-ai/solo/internal/server/service"
)

type createThinkingSynthesisRequest struct {
	Title              string                               `json:"title"`
	Objective          string                               `json:"objective"`
	NodeIDs            []string                             `json:"node_ids"`
	Constraints        service.ThinkingSynthesisConstraints `json:"constraints"`
	Mode               string                               `json:"mode"`
	CoordinatorAgentID string                               `json:"coordinator_agent_id"`
}

func (h *ThinkingHandler) CreateSynthesis(w http.ResponseWriter, r *http.Request) {
	actorID, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	var req createThinkingSynthesisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	synthesis, err := h.synthesisSvc.Create(r.Context(), channelID, actorID, service.CreateThinkingSynthesisInput{
		Title:              req.Title,
		Objective:          req.Objective,
		NodeIDs:            req.NodeIDs,
		Constraints:        req.Constraints,
		Mode:               req.Mode,
		CoordinatorAgentID: req.CoordinatorAgentID,
	})
	switch {
	case errors.Is(err, service.ErrThinkingNotFound):
		writeError(w, http.StatusNotFound, "thinking space not found")
	case errors.Is(err, service.ErrThinkingSynthesisInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrThinkingSynthesisSourceAbsent):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to create thinking synthesis")
	default:
		writeJSON(w, http.StatusCreated, synthesis)
	}
}

func (h *ThinkingHandler) ListSyntheses(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	syntheses, err := h.synthesisSvc.List(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list thinking syntheses")
		return
	}
	writeJSON(w, http.StatusOK, syntheses)
}

func (h *ThinkingHandler) GetSynthesis(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	synthesisID := strings.TrimSpace(chi.URLParam(r, "synthesisID"))
	if _, err := uuid.Parse(synthesisID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid synthesis ID")
		return
	}
	synthesis, err := h.synthesisSvc.Get(r.Context(), channelID, synthesisID)
	if errors.Is(err, service.ErrThinkingSynthesisNotFound) {
		writeError(w, http.StatusNotFound, "thinking synthesis not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thinking synthesis")
		return
	}
	writeJSON(w, http.StatusOK, synthesis)
}
