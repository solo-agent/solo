package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
)

type AgentRunHandler struct {
	svc *service.AgentRunService
}

func NewAgentRunHandler(pool *pgxpool.Pool, daemonManagers ...*service.DaemonManager) *AgentRunHandler {
	return &AgentRunHandler{svc: service.NewAgentRunService(pool, daemonManagers...)}
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
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	runs, err := h.svc.ListRecentRunsForUser(r.Context(), userID)
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
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	runID := chi.URLParam(r, "runID")
	allowed, err := h.svc.UserCanAccessRun(r.Context(), userID, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authorize run transcript")
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	entries, err := h.svc.GetRunTranscript(r.Context(), runID, limit)
	if err != nil {
		if errors.Is(err, service.ErrComputerOffline) || errors.Is(err, service.ErrControlRPCUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "computer is offline")
			return
		}
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
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	allowed, err := h.svc.UserCanAccessSession(r.Context(), userID, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authorize session timeline")
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	limit := timelineLimit(r)
	timeline, err := h.svc.GetSessionTimeline(r.Context(), sessionID, limit)
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
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	allowed, err := h.svc.UserCanAccessTask(r.Context(), userID, taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authorize task timeline")
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	limit := timelineLimit(r)
	timeline, err := h.svc.GetTaskTimeline(r.Context(), taskID, r.URL.Query().Get("agent_id"), limit)
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

func timelineLimit(r *http.Request) int {
	limit := 2000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	return limit
}
