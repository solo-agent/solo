package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func (h *AgentHandler) Selections(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	id := chi.URLParam(r, "agentID")
	if r.Method == http.MethodGet {
		data, err := service.ListAgentSelections(r.Context(), h.pool, id, actor)
		if err != nil {
			writeAgentWorkError(w, err)
			return
		}
		writeJSON(w, 200, data)
		return
	}
	var req service.AgentSelectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		writeError(w, 400, "invalid Selection plan")
		return
	}
	selection, err := service.CreateAgentSelection(r.Context(), h.pool, id, actor, req)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": selection})
}

func (h *AgentHandler) DecideSelection(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid Selection decision")
		return
	}
	if err := service.DecideAgentSelection(r.Context(), h.pool, chi.URLParam(r, "agentID"), actor, chi.URLParam(r, "selectionID"), req.Decision, req.Reason); err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}

func (h *AgentHandler) Revisions(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	id := chi.URLParam(r, "agentID")
	if r.Method == http.MethodGet {
		data, err := service.ListAgentRevisions(r.Context(), h.pool, id, actor)
		if err != nil {
			writeAgentWorkError(w, err)
			return
		}
		writeJSON(w, 200, data)
		return
	}
	var req service.AgentRevisionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&req); err != nil {
		writeError(w, 400, "invalid revision")
		return
	}
	revision, err := service.CreateAgentRevision(r.Context(), h.pool, id, actor, req)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": revision})
}

func (h *AgentHandler) ApplyRevision(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req service.AgentRevisionApplicationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid team application")
		return
	}
	id, err := service.ApplyAgentRevision(r.Context(), h.pool, chi.URLParam(r, "agentID"), actor, chi.URLParam(r, "revisionID"), req)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"version_id": id})
}

func (h *AgentHandler) PublishRevision(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req struct {
		EvaluationTaskID string `json:"evaluation_task_id"`
		Reason           string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid publication")
		return
	}
	if err := service.PublishAgentRevision(r.Context(), h.pool, chi.URLParam(r, "agentID"), actor, chi.URLParam(r, "revisionID"), req.EvaluationTaskID, req.Reason); err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"published": true})
}

func (h *AgentHandler) TeamVersions(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	channel := chi.URLParam(r, "channelID")
	if r.Method == http.MethodGet {
		data, err := service.ListTeamVersions(r.Context(), h.pool, channel, actor)
		if err != nil {
			writeAgentWorkError(w, err)
			return
		}
		writeJSON(w, 200, data)
		return
	}
	var req struct {
		VersionID string `json:"version_id"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid team publication")
		return
	}
	id, err := service.PublishTeamVersion(r.Context(), h.pool, channel, actor, req.VersionID, req.Reason)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"version_id": id})
}

func (h *AgentHandler) TeamAgreements(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	channel := chi.URLParam(r, "channelID")
	if r.Method == http.MethodGet {
		data, err := service.ListTeamAgreements(r.Context(), h.pool, channel, actor)
		if err != nil {
			writeAgentWorkError(w, err)
			return
		}
		writeJSON(w, 200, data)
		return
	}
	var req service.CreateRelationshipRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil {
		writeError(w, 400, "invalid agreement")
		return
	}
	id, err := service.ProposeTeamAgreement(r.Context(), h.pool, channel, actor, req)
	if err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (h *AgentHandler) DecideAgreement(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(r)
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, 400, "invalid decision")
		return
	}
	if err := service.DecideTeamAgreement(r.Context(), h.pool, chi.URLParam(r, "channelID"), actor, chi.URLParam(r, "proposalID"), req.Decision); err != nil {
		writeAgentWorkError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}
