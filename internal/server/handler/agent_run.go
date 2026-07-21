package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
)

type AgentRunHandler struct {
	svc           *service.AgentRunService
	trajectorySvc *service.TaskTrajectoryService
}

func NewAgentRunHandler(pool *pgxpool.Pool) *AgentRunHandler {
	return &AgentRunHandler{
		svc:           service.NewAgentRunService(pool),
		trajectorySvc: service.NewTaskTrajectoryService(pool),
	}
}

func (h *AgentRunHandler) Active(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	runs, err := h.svc.ListActiveRunsForUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list active runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *AgentRunHandler) Recent(w http.ResponseWriter, r *http.Request) {
	runs, err := h.svc.ListRecentRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recent runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *AgentRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.GetRun(r.Context(), chi.URLParam(r, "runID"))
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *AgentRunHandler) Events(w http.ResponseWriter, r *http.Request) {
	events, err := h.svc.ListEvents(r.Context(), chi.URLParam(r, "runID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list run events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *AgentRunHandler) Transcript(w http.ResponseWriter, r *http.Request) {
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	entries, err := h.svc.GetRunTranscript(r.Context(), chi.URLParam(r, "runID"), limit)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read run transcript")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *AgentRunHandler) AgentRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.svc.ListRunsByAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *AgentRunHandler) AgentSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.svc.ListSessionsByAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *AgentRunHandler) SessionTimeline(w http.ResponseWriter, r *http.Request) {
	limit := timelineLimit(r)
	timeline, err := h.svc.GetSessionTimeline(r.Context(), chi.URLParam(r, "sessionID"), limit)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read session timeline")
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func (h *AgentRunHandler) AgentTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAgentTasks(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent tasks")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *AgentRunHandler) TaskRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.svc.ListRunsByTask(r.Context(), chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list task runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *AgentRunHandler) TaskTimeline(w http.ResponseWriter, r *http.Request) {
	limit := timelineLimit(r)
	timeline, err := h.svc.GetTaskTimeline(r.Context(), chi.URLParam(r, "taskID"), r.URL.Query().Get("agent_id"), limit)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read task timeline")
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func (h *AgentRunHandler) TaskTrajectory(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if _, err := uuid.Parse(taskID); err != nil {
		writeError(w, http.StatusBadRequest, "taskID must be a UUID")
		return
	}

	snapshot, err := h.trajectorySvc.Export(r.Context(), taskID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTaskNotFound):
			writeError(w, http.StatusNotFound, "task not found")
		case errors.Is(err, service.ErrTaskNotChannelMember):
			writeError(w, http.StatusForbidden, "not authorized to read this task trajectory")
		default:
			writeError(w, http.StatusInternalServerError, "failed to export task trajectory")
		}
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func timelineLimit(r *http.Request) int {
	limit := 2000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	return limit
}
