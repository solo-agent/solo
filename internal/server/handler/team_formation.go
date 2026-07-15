package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/solo-ai/solo/internal/server/service"
)

const maxTeamFormationBodyBytes = 64 * 1024

type TeamFormationHandler struct {
	svc *service.TeamFormationService
}

func NewTeamFormationHandler(svc *service.TeamFormationService) *TeamFormationHandler {
	return &TeamFormationHandler{svc: svc}
}

func (h *TeamFormationHandler) Form(w http.ResponseWriter, r *http.Request) {
	callerID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTeamFormationBodyBytes)
	var req service.TeamFormationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid team formation request")
		return
	}

	result, err := h.svc.Form(r.Context(), callerID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTeamFormationForbidden):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, service.ErrTeamFormationSourceNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, service.ErrTeamFormationInProgress):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, service.ErrInvalidTeamFormationPlan):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("team formation failed", "caller_id", callerID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to form team")
		}
		return
	}

	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}
