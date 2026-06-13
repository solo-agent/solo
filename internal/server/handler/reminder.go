package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

// ReminderHandler handles reminder-related HTTP requests.
type ReminderHandler struct {
	svc *service.ReminderService
}

// NewReminderHandler creates a new ReminderHandler.
func NewReminderHandler(svc *service.ReminderService) *ReminderHandler {
	return &ReminderHandler{svc: svc}
}

type reminderResponse struct {
	ID            string  `json:"id"`
	AgentID       string  `json:"agent_id"`
	ChannelID     *string `json:"channel_id,omitempty"`
	TaskID        *string `json:"task_id,omitempty"`
	ReminderType  string  `json:"reminder_type"`
	RemindAt      string  `json:"remind_at"`
	Message       string  `json:"message"`
	IsRecurring   bool    `json:"is_recurring"`
	RecurringRule string  `json:"recurring_rule,omitempty"`
	IsFired       bool    `json:"is_fired"`
	FiredAt       *string `json:"fired_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func toReminderResponse(r *service.Reminder) reminderResponse {
	resp := reminderResponse{
		ID:            r.ID,
		AgentID:       r.AgentID,
		ChannelID:     r.ChannelID,
		TaskID:        r.TaskID,
		ReminderType:  r.ReminderType,
		RemindAt:      r.RemindAt.Format(time.RFC3339),
		Message:       r.Message,
		IsRecurring:   r.IsRecurring,
		RecurringRule: r.RecurringRule,
		IsFired:       r.IsFired,
		CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.Format(time.RFC3339),
	}
	if r.FiredAt != nil {
		s := r.FiredAt.Format(time.RFC3339)
		resp.FiredAt = &s
	}
	return resp
}

// Create handles POST /api/v1/reminders
func (h *ReminderHandler) Create(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req service.CreateReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if req.RemindAt == "" {
		writeError(w, http.StatusBadRequest, "remind_at is required")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	reminder, err := h.svc.Create(r.Context(), req)
	if err != nil {
		slog.Error("failed to create reminder", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toReminderResponse(reminder))
}

// List handles GET /api/v1/reminders
func (h *ReminderHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	includeFired := r.URL.Query().Get("status") == "all"

	reminders, err := h.svc.List(r.Context(), agentID, includeFired)
	if err != nil {
		slog.Error("failed to list reminders", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list reminders")
		return
	}

	resp := make([]reminderResponse, len(reminders))
	for i, r := range reminders {
		resp[i] = toReminderResponse(&r)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Delete handles DELETE /api/v1/reminders/{id}
func (h *ReminderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "reminder ID is required")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if err.Error() == "reminder not found" {
			writeError(w, http.StatusNotFound, "reminder not found")
			return
		}
		slog.Error("failed to delete reminder", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete reminder")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
