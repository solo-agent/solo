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
	AgentNames    []string `json:"agent_names,omitempty"`
	OS            string   `json:"os,omitempty"`
	Hostname      string   `json:"hostname,omitempty"`
	IP            string   `json:"ip,omitempty"`
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
		OS:        c.OS,
		Hostname:  c.Hostname,
		IP:        c.IP,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
	}
	if c.LastHeartbeat != nil {
		resp.LastHeartbeat = c.LastHeartbeat.Format(time.RFC3339)
	}
	if resp.AgentIDs == nil {
		resp.AgentIDs = []string{}
	}
	if resp.AgentNames == nil {
		resp.AgentNames = []string{}
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
// Returns agents persistently bound to this computer (via agents.runtime_id).
// Only shows agents owned by the current user.
//
// Agent status is determined by heartbeat agent_ids (persistent session state):
//   - "online"  — active persistent session on the daemon
//   - "offline" — bound but no active session (or computer offline)
//   - "running" — active session + currently executing a task
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

	// Build lookup set from heartbeat agent_ids (persistent session state).
	sessionSet := make(map[string]struct{}, len(c.AgentIDs))
	for _, id := range c.AgentIDs {
		if id != "" {
			sessionSet[id] = struct{}{}
		}
	}

	// Query agents persistently bound to this computer, filtered by owner.
	rows, err := h.pool.Query(r.Context(),
		`SELECT a.id, a.name
		 FROM agents a
		 WHERE a.runtime_id = $1 AND a.owner_id = $2 AND a.is_active = true
		 ORDER BY a.created_at ASC`,
		computerID, userID,
	)
	if err != nil {
		slog.Error("failed to query bound agents", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	agents := make([]ComputerAgentResponse, 0)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}

		var activeTasks int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM tasks WHERE claimer_id = $1 AND status = $2`,
			id, service.TaskStatusInProgress,
		).Scan(&activeTasks)

		status := "offline"
		computerOnline := c.Status == "online"
		_, hasSession := sessionSet[id]

		if computerOnline && hasSession {
			status = "online"
			if h.dm != nil && c.DaemonID != "" {
				for _, pt := range h.dm.GetDaemonPendingTasks(c.DaemonID) {
					if pt.AgentID == id {
						status = "running"
						break
					}
				}
			}
		}

		agents = append(agents, ComputerAgentResponse{
			ID:          id,
			Name:        name,
			Status:      status,
			ActiveTasks: activeTasks,
		})
	}

	if agents == nil {
		agents = []ComputerAgentResponse{}
	}

	writeJSON(w, http.StatusOK, ComputerAgentsResponse{
		ComputerID: computerID,
		Agents:     agents,
	})
}
