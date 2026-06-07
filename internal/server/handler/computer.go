package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

// ComputerHandler handles computer-related HTTP requests.
type ComputerHandler struct {
	svc *service.ComputerService
	dm  *service.DaemonManager
	pool *pgxpool.Pool
}

// NewComputerHandler creates a new ComputerHandler.
func NewComputerHandler(svc *service.ComputerService, dm *service.DaemonManager, pool *pgxpool.Pool) *ComputerHandler {
	return &ComputerHandler{svc: svc, dm: dm, pool: pool}
}

// CreateComputerRequest is the request body for creating a computer.
type CreateComputerRequest struct {
	Name string `json:"name"`
}

// ComputerResponse is the API response for a computer.
type ComputerResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id"`
	DaemonID      string   `json:"daemon_id,omitempty"`
	DaemonURL     string   `json:"daemon_url,omitempty"`
	Status        string   `json:"status"`
	LastHeartbeat string   `json:"last_heartbeat,omitempty"`
	AgentIDs      []string `json:"agent_ids,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// UpdateComputerRequest is the request body for updating a computer.
type UpdateComputerRequest struct {
	Name *string `json:"name,omitempty"`
}

// toResponse converts a service.Computer to a ComputerResponse.
func toResponse(c *service.Computer) ComputerResponse {
	resp := ComputerResponse{
		ID:        c.ID,
		Name:      c.Name,
		OwnerID:   c.OwnerID,
		DaemonID:  c.DaemonID,
		DaemonURL: c.DaemonURL,
		Status:    c.Status,
		AgentIDs:  c.AgentIDs,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
	}
	if c.LastHeartbeat != nil {
		resp.LastHeartbeat = c.LastHeartbeat.Format(time.RFC3339)
	}
	if resp.AgentIDs == nil {
		resp.AgentIDs = []string{}
	}
	return resp
}

// Create handles POST /api/v1/computers
func (h *ComputerHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateComputerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "computer name is required")
		return
	}
	if len(name) > 200 {
		writeError(w, http.StatusBadRequest, "computer name must be 200 characters or less")
		return
	}

	c, err := h.svc.CreateComputer(r.Context(), userID, name)
	if err != nil {
		slog.Error("failed to create computer", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create computer")
		return
	}

	slog.Info("computer created", "computer_id", c.ID, "name", name, "owner_id", userID)
	writeJSON(w, http.StatusCreated, toResponse(c))
}

// List handles GET /api/v1/computers
func (h *ComputerHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	computers, err := h.svc.ListComputers(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list computers", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list computers")
		return
	}

	resp := make([]ComputerResponse, len(computers))
	for i, c := range computers {
		resp[i] = toResponse(&c)
	}

	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /api/v1/computers/{computerID}
func (h *ComputerHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	computerID := chi.URLParam(r, "computerID")
	if computerID == "" {
		writeError(w, http.StatusBadRequest, "computer ID is required")
		return
	}

	c, err := h.svc.GetComputer(r.Context(), computerID, userID)
	if err != nil {
		if err == service.ErrNotFound {
			writeError(w, http.StatusNotFound, "computer not found")
			return
		}
		slog.Error("failed to get computer", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(c))
}

// Update handles PATCH /api/v1/computers/{computerID}
func (h *ComputerHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	computerID := chi.URLParam(r, "computerID")
	if computerID == "" {
		writeError(w, http.StatusBadRequest, "computer ID is required")
		return
	}

	var req UpdateComputerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "name is required for update")
		return
	}

	name := strings.TrimSpace(*req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "computer name cannot be empty")
		return
	}
	if len(name) > 200 {
		writeError(w, http.StatusBadRequest, "computer name must be 200 characters or less")
		return
	}

	c, err := h.svc.UpdateComputer(r.Context(), computerID, userID, name)
	if err != nil {
		if err == service.ErrNotFound {
			writeError(w, http.StatusNotFound, "computer not found")
			return
		}
		slog.Error("failed to update computer", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update computer")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(c))
}

// Delete handles DELETE /api/v1/computers/{computerID}
func (h *ComputerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	computerID := chi.URLParam(r, "computerID")
	if computerID == "" {
		writeError(w, http.StatusBadRequest, "computer ID is required")
		return
	}

	err := h.svc.DeleteComputer(r.Context(), computerID, userID)
	if err != nil {
		if err == service.ErrNotFound {
			writeError(w, http.StatusNotFound, "computer not found")
			return
		}
		slog.Error("failed to delete computer", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete computer")
		return
	}

	slog.Info("computer deleted", "computer_id", computerID, "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "computer deleted"})
}

// ComputerAgentResponse is the API response for an agent running on a computer.
type ComputerAgentResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	ActiveTasks int    `json:"active_tasks"`
}

// ComputerAgentsResponse is the API response for listing agents on a computer.
type ComputerAgentsResponse struct {
	ComputerID string                  `json:"computer_id"`
	Agents     []ComputerAgentResponse `json:"agents"`
}

// ListAgents handles GET /api/v1/computers/{computerID}/agents
//
// The agent<->daemon mapping is determined through two sources:
//  1. computers.agent_ids — populated from daemon heartbeat AgentIDs field
//  2. DaemonManager pending tasks — for actively dispatched agent tasks
//
// Both sources are combined and deduplicated. Agent details (name) come from
// the agents table, and active_tasks is the count of in_progress tasks claimed
// by the agent.
func (h *ComputerHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	computerID := chi.URLParam(r, "computerID")
	if computerID == "" {
		writeError(w, http.StatusBadRequest, "computer ID is required")
		return
	}

	c, err := h.svc.GetComputer(r.Context(), computerID, userID)
	if err != nil {
		if err == service.ErrNotFound {
			writeError(w, http.StatusNotFound, "computer not found")
			return
		}
		slog.Error("failed to get computer for agents", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Collect agent IDs from computer record (populated by daemon heartbeat)
	// and from pending tasks on the daemon.
	agentIDSet := make(map[string]struct{})
	for _, id := range c.AgentIDs {
		if id != "" {
			agentIDSet[id] = struct{}{}
		}
	}

	// Supplement with agent IDs from pending daemon tasks.
	if h.dm != nil && c.DaemonID != "" {
		for _, pt := range h.dm.GetDaemonPendingTasks(c.DaemonID) {
			if pt.AgentID != "" {
				agentIDSet[pt.AgentID] = struct{}{}
			}
		}
	}

	// Build response
	agents := make([]ComputerAgentResponse, 0, len(agentIDSet))
	for agentID := range agentIDSet {
		var name string
		err := h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(name, '') FROM agents WHERE id = $1`,
			agentID,
		).Scan(&name)
		if err != nil {
			// Agent may have been deleted; skip.
			slog.Debug("agent not found for computer agents list", "agent_id", agentID, "error", err)
			continue
		}

		// Count active (in_progress) tasks claimed by this agent.
		var activeTasks int
		err = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM tasks WHERE claimer_id = $1 AND status = $2`,
			agentID, service.TaskStatusInProgress,
		).Scan(&activeTasks)
		if err != nil {
			slog.Warn("failed to count active tasks for agent", "agent_id", agentID, "error", err)
			activeTasks = 0
		}

		// Determine agent status: "running" if there are pending tasks on this
		// daemon, otherwise "online" (the agent exists and the daemon is online).
		status := "online"
		if h.dm != nil && c.DaemonID != "" {
			for _, pt := range h.dm.GetDaemonPendingTasks(c.DaemonID) {
				if pt.AgentID == agentID {
					status = "running"
					break
				}
			}
		}

		agents = append(agents, ComputerAgentResponse{
			ID:          agentID,
			Name:        name,
			Status:      status,
			ActiveTasks: activeTasks,
		})
	}

	writeJSON(w, http.StatusOK, ComputerAgentsResponse{
		ComputerID: computerID,
		Agents:     agents,
	})
}
