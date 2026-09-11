package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func (h *AgentHandler) Work(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	data, err := service.GetAgentWork(r.Context(), h.pool, chi.URLParam(r, "agentID"), actorID)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, data)
}

func (h *AgentHandler) SaveWorkMark(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req service.AgentWorkMarkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid work mark")
		return
	}
	id, err := service.SaveAgentWorkMark(r.Context(), h.pool, chi.URLParam(r, "agentID"), actorID, req)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
}

func (h *AgentHandler) Attention(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req struct {
		ChannelID string `json:"channel_id"`
		Policy    string `json:"policy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, 400, "invalid attention policy")
		return
	}
	if err := service.SetAgentAttention(r.Context(), h.pool, chi.URLParam(r, "agentID"), actorID, req.ChannelID, req.Policy); err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func writeAgentWorkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrAgentWorkForbidden), errors.Is(err, service.ErrTaskNotChannelMember):
		writeError(w, 403, err.Error())
	case errors.Is(err, service.ErrTaskDeliveryInvalid), errors.Is(err, service.ErrRelationshipCycle):
		writeError(w, 400, err.Error())
	case errors.Is(err, service.ErrTaskVersionConflict):
		writeError(w, 409, err.Error())
	default:
		writeError(w, 500, "could not update Agent work")
	}
}

func (h *AgentHandler) DiscardDraft(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req service.DiscardAgentDraftRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid draft decision")
		return
	}
	if err := service.DiscardAgentDraft(r.Context(), h.pool, chi.URLParam(r, "agentID"), actor, req); err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"discarded": true})
}
