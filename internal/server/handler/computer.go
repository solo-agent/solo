package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

// ComputerHandler handles computer-related HTTP requests.
type ComputerHandler struct {
	svc *service.ComputerService
}

// NewComputerHandler creates a new ComputerHandler.
func NewComputerHandler(svc *service.ComputerService) *ComputerHandler {
	return &ComputerHandler{svc: svc}
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
