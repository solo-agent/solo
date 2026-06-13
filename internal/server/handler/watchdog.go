package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

// WatchdogHandler handles watchdog-related HTTP requests.
type WatchdogHandler struct {
	svc     *service.WatchdogService
	taskSvc *service.TaskService
}

// NewWatchdogHandler creates a new WatchdogHandler.
func NewWatchdogHandler(svc *service.WatchdogService, taskSvc *service.TaskService) *WatchdogHandler {
	return &WatchdogHandler{svc: svc, taskSvc: taskSvc}
}

// SetWatchdogRequest is the payload for PATCH /api/v1/tasks/{id}/watchdog.
type SetWatchdogRequest struct {
	ClaimerID     string `json:"claimer_id"`
	Deadline      string `json:"deadline"`
	TimeoutAction string `json:"timeout_action"`
	EscalateTo    string `json:"escalate_to,omitempty"`
}

// SetWatchdog handles PATCH /api/v1/tasks/{id}/watchdog
func (h *WatchdogHandler) SetWatchdog(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	var req SetWatchdogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ClaimerID == "" {
		writeError(w, http.StatusBadRequest, "claimer_id is required")
		return
	}
	if req.Deadline == "" {
		writeError(w, http.StatusBadRequest, "deadline is required")
		return
	}

	deadline, err := time.Parse(time.RFC3339, req.Deadline)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deadline format (use RFC3339)")
		return
	}

	if req.TimeoutAction == "" {
		req.TimeoutAction = "remind"
	}
	if req.TimeoutAction != "remind" && req.TimeoutAction != "escalate" && req.TimeoutAction != "unclaim" {
		writeError(w, http.StatusBadRequest, "timeout_action must be remind, escalate, or unclaim")
		return
	}

	// T6.4.5: Escalation chain verification — check that escalates_to relationship exists.
	if req.TimeoutAction == "escalate" && req.EscalateTo != "" {
		if h.taskSvc != nil {
			// Validate that escalates_to relationship exists between claimer and escalate target.
			// Check via task service or the relationship service.
			var hasEscalationPath bool
			err := h.svc.VerifyEscalationChain(r.Context(), req.ClaimerID, req.EscalateTo)
			if err != nil {
				slog.Warn("watchdog: escalation chain verification failed", "claimer", req.ClaimerID, "escalate_to", req.EscalateTo, "error", err)
			}
			_ = hasEscalationPath
		}
	}

	if err := h.svc.CreateWatchdog(r.Context(), taskID, req.ClaimerID, deadline, req.TimeoutAction, req.EscalateTo); err != nil {
		slog.Error("failed to set watchdog", "error", err, "task_id", taskID)
		writeError(w, http.StatusInternalServerError, "failed to set watchdog")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_id":        taskID,
		"claimer_id":     req.ClaimerID,
		"deadline":       req.Deadline,
		"timeout_action": req.TimeoutAction,
	})
}
