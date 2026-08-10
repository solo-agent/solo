package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

type BudgetHandler struct {
	service *service.BudgetService
}

func NewBudgetHandler(service *service.BudgetService) *BudgetHandler {
	return &BudgetHandler{service: service}
}

func (h *BudgetHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	view, err := h.service.GetUserBudget(r.Context(), userID)
	if err != nil {
		slog.Error("failed to load user budget", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load budget")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *BudgetHandler) SaveCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var input service.SaveBudgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "budget.error.invalid_request", "invalid request body")
		return
	}
	view, err := h.service.SaveUserBudget(r.Context(), userID, input)
	if err != nil {
		writeBudgetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *BudgetHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	view, err := h.service.GetAgentBudget(r.Context(), userID, chi.URLParam(r, "agentID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		slog.Error("failed to load agent budget", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load budget")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *BudgetHandler) SaveAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var input service.SaveAgentBudgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "budget.error.invalid_request", "invalid request body")
		return
	}
	view, err := h.service.SaveAgentBudget(r.Context(), userID, chi.URLParam(r, "agentID"), input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeBudgetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeBudgetError(w http.ResponseWriter, err error) {
	var inputErr *service.BudgetInputError
	if errors.As(err, &inputErr) {
		writeErrorCode(w, http.StatusBadRequest, inputErr.Code, inputErr.Message)
		return
	}
	slog.Error("failed to save budget", "error", err)
	writeError(w, http.StatusInternalServerError, "failed to save budget")
}
