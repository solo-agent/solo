package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

type AutomationHandler struct {
	svc *service.AutomationService
}

func NewAutomationHandler(svc *service.AutomationService) *AutomationHandler {
	return &AutomationHandler{svc: svc}
}

func (h *AutomationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	items, err := h.svc.List(r.Context(), chi.URLParam(r, "channelID"), userID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AutomationHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var input service.AutomationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.svc.Create(r.Context(), chi.URLParam(r, "channelID"), userID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *AutomationHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	item, err := h.svc.Get(r.Context(), chi.URLParam(r, "channelID"), chi.URLParam(r, "automationID"), userID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AutomationHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var input service.AutomationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.svc.Update(r.Context(), chi.URLParam(r, "channelID"), chi.URLParam(r, "automationID"), userID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AutomationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "channelID"), chi.URLParam(r, "automationID"), userID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	run, err := h.svc.RunNow(r.Context(), chi.URLParam(r, "channelID"), chi.URLParam(r, "automationID"), userID)
	if errors.Is(err, service.ErrAutomationAlreadyActive) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "Conflict", "code": "automation_already_running",
			"message": "automation already has an active run", "run": run,
		})
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (h *AutomationHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.ListRuns(r.Context(), chi.URLParam(r, "channelID"), chi.URLParam(r, "automationID"), userID, limit)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AutomationHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrTaskNotChannelMember):
		writeError(w, http.StatusForbidden, "not a channel member")
	case errors.Is(err, service.ErrAutomationNotFound):
		writeError(w, http.StatusNotFound, "automation not found")
	case errors.Is(err, service.ErrAutomationTargetMissing):
		writeErrorCode(w, http.StatusBadRequest, "automation_target_unavailable", err.Error())
	case errors.Is(err, service.ErrAutomationInvalidInput):
		writeErrorCode(w, http.StatusBadRequest, "automation_invalid_input", err.Error())
	default:
		if errors.Is(err, service.ErrAutomationNotDue) {
			writeErrorCode(w, http.StatusConflict, "automation_not_due", err.Error())
			return
		}
		slog.Error("automation request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "automation request failed")
	}
}
