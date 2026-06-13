package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

// ChannelMemoryHandler handles channel memory HTTP endpoints.
// Memory files are stored under ~/.solo/channels/<channelID>/memory/.
type ChannelMemoryHandler struct {
	pool *pgxpool.Pool
	svc  *service.ChannelMemoryService
}

// NewChannelMemoryHandler creates a new ChannelMemoryHandler.
func NewChannelMemoryHandler(pool *pgxpool.Pool) *ChannelMemoryHandler {
	return &ChannelMemoryHandler{
		pool: pool,
		svc:  service.NewChannelMemoryService(""),
	}
}

// channelMemoryRequest is the body for writing CHANNEL.md.
type channelMemoryRequest struct {
	Content string `json:"content"`
}

// decisionRequest is the body for appending a decision entry.
type decisionRequest struct {
	Content string `json:"content"`
}

// isChannelAgent checks if the given user ID belongs to an agent that is a
// member of the specified channel. Agents in the channel have read/write
// access to channel memory.
func (h *ChannelMemoryHandler) isChannelAgent(channelID, userID string) bool {
	var exists bool
	err := h.pool.QueryRow(context.Background(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'agent' AND member_id = $2
		)`, channelID, userID,
	).Scan(&exists)
	return err == nil && exists
}

// isChannelMember checks if the user is any type of channel member.
func (h *ChannelMemoryHandler) isChannelMember(channelID, userID string) bool {
	var exists bool
	err := h.pool.QueryRow(context.Background(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_id = $2
		)`, channelID, userID,
	).Scan(&exists)
	return err == nil && exists
}

// GetChannelMd handles GET /api/v1/channels/{channelID}/memory/channel-md
func (h *ChannelMemoryHandler) GetChannelMd(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.isChannelMember(channelID, userID) {
		writeError(w, http.StatusForbidden, "not a channel member")
		return
	}

	content, err := h.svc.ReadCHANNEL(channelID)
	if err != nil {
		slog.Error("failed to read channel memory", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read channel memory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// PutChannelMd handles POST /api/v1/channels/{channelID}/memory/channel-md
func (h *ChannelMemoryHandler) PutChannelMd(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.isChannelAgent(channelID, userID) {
		writeError(w, http.StatusForbidden, "only channel agents can write channel memory")
		return
	}

	var req channelMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	if err := h.svc.WriteCHANNEL(channelID, req.Content); err != nil {
		slog.Error("failed to write channel memory", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to write channel memory")
		return
	}

	slog.Info("channel memory updated", "channel_id", channelID, "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "written"})
}

// AppendDecision handles POST /api/v1/channels/{channelID}/memory/decisions
func (h *ChannelMemoryHandler) AppendDecision(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.isChannelAgent(channelID, userID) {
		writeError(w, http.StatusForbidden, "only channel agents can write decisions")
		return
	}

	var req decisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Resolve agent name for the decision entry.
	agentName := userID
	var name string
	err := h.pool.QueryRow(r.Context(),
		`SELECT name FROM agents WHERE id = $1`, userID,
	).Scan(&name)
	if err == nil && name != "" {
		agentName = name
	}

	if err := h.svc.AppendDecision(channelID, agentName, req.Content); err != nil {
		slog.Error("failed to append decision", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to append decision")
		return
	}

	slog.Info("decision appended", "channel_id", channelID, "agent", agentName)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "appended"})
}

// GetDecisions handles GET /api/v1/channels/{channelID}/memory/decisions
func (h *ChannelMemoryHandler) GetDecisions(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.isChannelMember(channelID, userID) {
		writeError(w, http.StatusForbidden, "not a channel member")
		return
	}

	content, err := h.svc.ReadDecisions(channelID)
	if err != nil {
		slog.Error("failed to read decisions", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read decisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// LoadChannelMemory is a helper for the daemon to read CHANNEL.md directly.
// It bypasses auth since the daemon calls it internally.
func LoadChannelMemory(channelID string) string {
	svc := service.NewChannelMemoryService("")
	content, err := svc.ReadCHANNEL(channelID)
	if err != nil {
		slog.Debug("failed to load channel memory", "channel_id", channelID, "error", err)
		return ""
	}
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n## Channel Shared Memory\n\n%s\n", content)
}
