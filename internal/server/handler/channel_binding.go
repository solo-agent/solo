package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

// ChannelBindingHandler handles channel project binding HTTP endpoints.
type ChannelBindingHandler struct {
	pool *pgxpool.Pool
	svc  *service.ChannelBindingService
}

// NewChannelBindingHandler creates a new ChannelBindingHandler.
func NewChannelBindingHandler(pool *pgxpool.Pool) *ChannelBindingHandler {
	return &ChannelBindingHandler{
		pool: pool,
		svc:  service.NewChannelBindingService(pool),
	}
}

type bindProjectRequest struct {
	RepoURL    string `json:"repo_url"`
	RepoBranch string `json:"repo_branch,omitempty"`
}

// BindProject handles POST /api/v1/channels/{channelID}/bind-project
func (h *ChannelBindingHandler) BindProject(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req bindProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}

	binding, err := h.svc.BindProject(r.Context(), channelID, req.RepoURL, req.RepoBranch, userID)
	if err != nil {
		slog.Error("failed to bind project", "channel_id", channelID, "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, binding)
}

// GetBinding handles GET /api/v1/channels/{channelID}/binding
func (h *ChannelBindingHandler) GetBinding(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	binding, err := h.svc.GetBinding(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no project binding found for this channel")
		return
	}

	writeJSON(w, http.StatusOK, binding)
}

// UnbindProject handles DELETE /api/v1/channels/{channelID}/bind-project
func (h *ChannelBindingHandler) UnbindProject(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.svc.UnbindProject(r.Context(), channelID, userID); err != nil {
		slog.Error("failed to unbind project", "channel_id", channelID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unbound"})
}
