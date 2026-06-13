package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/internal/server/service"
)

// DaemonHandler handles internal daemon communication endpoints.
type DaemonHandler struct {
	dm          *service.DaemonManager
	agent       *service.AgentService
	computerSvc *service.ComputerService
}

// NewDaemonHandler creates a new DaemonHandler.
func NewDaemonHandler(dm *service.DaemonManager, agent *service.AgentService, computerSvc *service.ComputerService) *DaemonHandler {
	return &DaemonHandler{dm: dm, agent: agent, computerSvc: computerSvc}
}

// Register handles POST /internal/daemon/register
// Daemons register themselves on startup with their capabilities.
func (h *DaemonHandler) Register(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.DaemonRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("daemon register: invalid body", "request_id", reqID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DaemonID == "" {
		slog.Warn("daemon register: missing daemon_id", "request_id", reqID)
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}
	if req.Host == "" {
		slog.Warn("daemon register: missing host", "request_id", reqID, "daemon_id", req.DaemonID)
		writeError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.Port == 0 {
		slog.Warn("daemon register: missing port", "request_id", reqID, "daemon_id", req.DaemonID)
		writeError(w, http.StatusBadRequest, "port is required")
		return
	}
	if req.MaxConcurrent <= 0 {
		req.MaxConcurrent = 10
	}

	info := &service.DaemonInfo{
		ID:            req.DaemonID,
		Host:          req.Host,
		Port:          req.Port,
		Version:       req.Version,
		Capabilities:  req.Capabilities,
		MaxConcurrent: req.MaxConcurrent,
		AgentTypes:    req.AgentTypes,
	}

	h.dm.Register(info)

	// Persist computer record via ComputerService
	if h.computerSvc != nil {
		daemonURL := fmt.Sprintf("http://%s:%d", req.Host, req.Port)
		sysinfo := service.ComputerSystemInfo{
			OS:       req.SystemInfo.OS,
			Hostname: req.SystemInfo.Hostname,
			IP:       req.SystemInfo.IP,
		}
		if err := h.computerSvc.UpsertComputerByDaemonID(r.Context(), req.DaemonID, daemonURL, "", sysinfo); err != nil {
			slog.Error("failed to upsert computer on register",
				"request_id", reqID,
				"daemon_id", req.DaemonID,
				"error", err,
			)
		}
	}

	slog.Info("daemon registered via handler",
		"request_id", reqID,
		"daemon_id", req.DaemonID,
		"host", req.Host,
		"port", req.Port,
		"version", req.Version,
		"capabilities", req.Capabilities,
	)

	writeJSON(w, http.StatusOK, service.DaemonRegisterResponse{
		Status:            "registered",
		HeartbeatInterval: 30,
	})
}

// Heartbeat handles POST /internal/daemon/heartbeat
// Daemons send heartbeats to confirm they are alive.
func (h *DaemonHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.DaemonHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("daemon heartbeat: invalid body", "request_id", reqID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DaemonID == "" {
		slog.Warn("daemon heartbeat: missing daemon_id", "request_id", reqID)
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}

	ok := h.dm.Heartbeat(req.DaemonID, req.Load)
	if !ok {
		slog.Warn("daemon heartbeat: unknown daemon",
			"request_id", reqID, "daemon_id", req.DaemonID,
		)
		writeError(w, http.StatusNotFound, "daemon not registered")
		return
	}

	// Persist heartbeat to computers table
	if h.computerSvc != nil {
		info, found := h.dm.GetDaemon(req.DaemonID)
		if found {
			daemonURL := fmt.Sprintf("http://%s:%d", info.Host, info.Port)
			sysinfo := service.ComputerSystemInfo{
				OS:       req.SystemInfo.OS,
				Hostname: req.SystemInfo.Hostname,
				IP:       req.SystemInfo.IP,
			}
			if err := h.computerSvc.UpdateHeartbeat(r.Context(), req.DaemonID, daemonURL, req.AgentIDs, sysinfo); err != nil {
				slog.Error("failed to update computer heartbeat in DB — returning 404 to trigger daemon re-registration",
					"request_id", reqID,
					"daemon_id", req.DaemonID,
					"error", err,
				)
				writeError(w, http.StatusNotFound, "computer record missing — re-register required")
				return
			}
		}
	}

	slog.Debug("daemon heartbeat received",
		"request_id", reqID,
		"daemon_id", req.DaemonID,
		"load", req.Load,
	)

	writeJSON(w, http.StatusOK, service.DaemonHeartbeatResponse{
		Status: "ok",
	})
}

// Unregister handles POST /internal/daemon/unregister
// Called when a daemon shuts down cleanly to remove it from tracking.
func (h *DaemonHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req struct {
		DaemonID string `json:"daemon_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("daemon unregister: invalid body", "request_id", reqID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DaemonID == "" {
		slog.Warn("daemon unregister: missing daemon_id", "request_id", reqID)
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}

	h.dm.Unregister(req.DaemonID)

	slog.Info("daemon unregistered via handler",
		"request_id", reqID,
		"daemon_id", req.DaemonID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

// WorkspaceConflict handles POST /internal/daemon/workspace/conflict
// Accepts conflict notifications from daemons and broadcasts workspace_conflict
// WebSocket events to connected clients in the affected channel.
func (h *DaemonHandler) WorkspaceConflict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID       string                   `json:"channel_id"`
		AgentID         string                   `json:"agent_id"`
		ConflictingWith []map[string]interface{} `json:"conflicting_with"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}

	// Broadcast workspace_conflict event to the channel.
	hub := h.dm.Broadcaster()
	if hub != nil {
		payload := map[string]interface{}{
			"channel_id":       req.ChannelID,
			"agent_id":         req.AgentID,
			"conflicting_with": req.ConflictingWith,
		}
		envelope := realtime.Envelope("workspace_conflict", payload)
		hub.BroadcastToChannel(req.ChannelID, envelope)
		slog.Info("workspace_conflict broadcast to channel",
			"channel_id", req.ChannelID,
			"agent_id", req.AgentID,
			"conflict_count", len(req.ConflictingWith),
		)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "broadcast"})
}

// TaskComplete handles POST /internal/daemon/tasks/{taskID}/complete
// Called by the daemon when a task finishes executing.
func (h *DaemonHandler) TaskComplete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.TaskCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("task complete: invalid body", "request_id", reqID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TaskID == "" {
		slog.Warn("task complete: missing task_id", "request_id", reqID)
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := h.agent.HandleTaskComplete(r.Context(), &req); err != nil {
		slog.Error("failed to handle task complete",
			"request_id", reqID,
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to process task completion")
		return
	}

	slog.Info("task completed callback handled",
		"request_id", reqID,
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TaskError handles POST /internal/daemon/tasks/{taskID}/error
// Called by the daemon when a task encounters an error.
func (h *DaemonHandler) TaskError(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.TaskErrorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("task error: invalid body", "request_id", reqID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.agent.HandleTaskError(r.Context(), &req); err != nil {
		slog.Error("failed to handle task error",
			"request_id", reqID,
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to process task error")
		return
	}

	slog.Info("task error callback handled",
		"request_id", reqID,
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
