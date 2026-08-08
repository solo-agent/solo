package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/internal/server/service"
)

var daemonControlUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     middleware.WebSocketOriginAllowed,
}

type DaemonControlHandler struct {
	computers *service.ComputerService
	dm        *service.DaemonManager
}

func NewDaemonControlHandler(computers *service.ComputerService, dm *service.DaemonManager) *DaemonControlHandler {
	return &DaemonControlHandler{computers: computers, dm: dm}
}

func (h *DaemonControlHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ComputerID      string `json:"computer_id"`
		EnrollmentToken string `json:"enrollment_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	request.ComputerID = strings.TrimSpace(request.ComputerID)
	request.EnrollmentToken = strings.TrimSpace(request.EnrollmentToken)
	if request.ComputerID == "" || request.EnrollmentToken == "" {
		writeError(w, http.StatusBadRequest, "computer_id and enrollment_token are required")
		return
	}
	credential, err := h.computers.ExchangeEnrollment(r.Context(), request.ComputerID, request.EnrollmentToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidEnrollment) {
			writeError(w, http.StatusUnauthorized, "invalid or expired enrollment")
			return
		}
		slog.Error("daemon enrollment failed", "computer_id", request.ComputerID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to enroll computer")
		return
	}
	h.dm.DisconnectComputer(request.ComputerID)
	writeJSON(w, http.StatusCreated, credential)
}

func (h *DaemonControlHandler) Connect(w http.ResponseWriter, r *http.Request) {
	computerID := middleware.ComputerID(r)
	protocolVersion, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Solo-Protocol-Version")))
	if err != nil || protocolVersion != service.ComputerProtocolVersion {
		w.Header().Set("X-Solo-Supported-Protocol-Versions", strconv.Itoa(service.ComputerProtocolVersion))
		writeError(w, http.StatusUpgradeRequired, "unsupported daemon protocol version")
		return
	}
	conn, err := daemonControlUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("daemon control upgrade failed", "computer_id", computerID, "error", err)
		return
	}
	if err := h.dm.ServeControlConnection(r.Context(), conn, computerID, h.computers); err != nil {
		slog.Info("daemon control ended", "computer_id", computerID, "error", err)
	}
}

func (h *DaemonControlHandler) PendingRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.dm.PendingRemoteRuns(r.Context(), middleware.ComputerID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load pending Runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_ids": runs})
}

func (h *DaemonControlHandler) AcceptRun(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil || strings.TrimSpace(request.ConnectionID) == "" {
		writeError(w, http.StatusBadRequest, "connection_id is required")
		return
	}
	delivery, err := h.dm.AcceptRemoteRun(r.Context(), middleware.ComputerID(r), chi.URLParam(r, "runID"), request.ConnectionID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRemoteRunNotFound):
			writeError(w, http.StatusNotFound, "Run not found")
		case errors.Is(err, service.ErrRemoteRunExpired):
			writeError(w, http.StatusGone, "Run delivery expired")
		case errors.Is(err, service.ErrStaleControlLease):
			writeError(w, http.StatusConflict, "stale control connection")
		default:
			slog.Error("accept remote Run failed", "run_id", chi.URLParam(r, "runID"), "error", err)
			writeError(w, http.StatusInternalServerError, "failed to accept Run")
		}
		return
	}
	writeJSON(w, http.StatusOK, delivery)
}

func (h *DaemonControlHandler) AppendRunEvent(w http.ResponseWriter, r *http.Request) {
	var request service.RemoteRunEventInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid event")
		return
	}
	err := h.dm.AppendRemoteRunEvent(r.Context(), middleware.ComputerID(r), chi.URLParam(r, "runID"), request)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrStaleControlLease), errors.Is(err, service.ErrRemoteRunAttempt):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
