package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

type TaskDependencyHandler struct {
	taskSvc *service.TaskService
}

func NewTaskDependencyHandler(taskSvc *service.TaskService) *TaskDependencyHandler {
	return &TaskDependencyHandler{taskSvc: taskSvc}
}

// AddDependency handles POST /api/v1/task-dependencies
func (h *TaskDependencyHandler) AddDependency(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	var body struct {
		BlockerTaskID string `json:"blocker_task_id"`
		BlockedTaskID string `json:"blocked_task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.BlockerTaskID == "" || body.BlockedTaskID == "" {
		writeError(w, http.StatusBadRequest, "blocker_task_id and blocked_task_id are required")
		return
	}

	dep, err := h.taskSvc.AddDependency(r.Context(), body.BlockerTaskID, body.BlockedTaskID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, dep)
}

// ListBlockers handles GET /api/v1/tasks/{taskID}/blockers
func (h *TaskDependencyHandler) ListBlockers(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	deps, err := h.taskSvc.ListBlockers(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deps)
}

// ListBlocked handles GET /api/v1/tasks/{taskID}/blocked
func (h *TaskDependencyHandler) ListBlocked(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	deps, err := h.taskSvc.ListBlocked(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deps)
}

// IsBlocked handles GET /api/v1/tasks/{taskID}/is-blocked
func (h *TaskDependencyHandler) IsBlocked(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	blocked, err := h.taskSvc.IsTaskBlocked(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"blocked": blocked})
}

// RemoveDependency handles DELETE /api/v1/task-dependencies
func (h *TaskDependencyHandler) RemoveDependency(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BlockerTaskID string `json:"blocker_task_id"`
		BlockedTaskID string `json:"blocked_task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.taskSvc.RemoveDependency(r.Context(), body.BlockerTaskID, body.BlockedTaskID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
