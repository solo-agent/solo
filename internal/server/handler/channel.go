package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelHandler handles channel-related HTTP requests.
type ChannelHandler struct {
	pool *pgxpool.Pool
}

// NewChannelHandler creates a new ChannelHandler.
func NewChannelHandler(pool *pgxpool.Pool) *ChannelHandler {
	return &ChannelHandler{pool: pool}
}

// --- Request/Response types ---

type CreateChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type UpdateChannelRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ChannelResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	CreatedBy   string `json:"created_by"`
	IsArchived  bool   `json:"is_archived"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Create handles POST /api/v1/channels
// JoinByTarget handles POST /api/v1/channels/join
// Agent joins a channel by name (e.g. "#test").
func (h *ChannelHandler) JoinByTarget(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Target = r.URL.Query().Get("target")
	}
	if req.Target == "" {
		writeError(w, http.StatusBadRequest, "target is required (e.g. '#channel-name')")
		return
	}
	channelName := strings.TrimPrefix(req.Target, "#")
	var channelID string
	err := h.pool.QueryRow(r.Context(), `SELECT id FROM channels WHERE name = $1`, channelName).Scan(&channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'agent', $2) ON CONFLICT DO NOTHING`,
		channelID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join channel")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined", "channel_id": channelID})
}


// ServerInfo handles GET /api/v1/server/info
// Returns channels, agents, and humans visible to the authenticated user.
func (h *ChannelHandler) ServerInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID
	// List public channels the user can see
	channels := []map[string]interface{}{}
	rows, err := h.pool.Query(r.Context(),
		`SELECT c.id, c.name, COALESCE(c.description,''), c.type,
		 EXISTS(SELECT 1 FROM channel_members WHERE channel_id=c.id AND member_id=$1) as joined
		 FROM channels c WHERE (c.type='channel' AND NOT c.is_archived) OR (c.type='dm' AND EXISTS(SELECT 1 FROM channel_members WHERE channel_id=c.id AND member_id=$1))`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name, desc, ctype string
			var joined bool
			if rows.Scan(&id, &name, &desc, &ctype, &joined) == nil {
				channels = append(channels, map[string]interface{}{
					"id": id, "name": name, "description": desc, "type": ctype, "joined": joined,
				})
			}
		}
	}
	// List active agents
	agents := []map[string]interface{}{}
	rows2, err := h.pool.Query(r.Context(), `SELECT id, name, COALESCE(system_prompt,'') FROM agents WHERE is_active=true`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var id, name, sp string
			if rows2.Scan(&id, &name, &sp) == nil {
				agents = append(agents, map[string]interface{}{"id": id, "name": name})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels, "agents": agents, "humans": []string{},
	})
}

func (h *ChannelHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "channel name is required")
		return
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "channel name must be 100 characters or less")
		return
	}

	// Check for duplicate name
	var exists bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channels
			WHERE name = $1 AND type = 'channel' AND is_archived = false
		)`, name,
	).Scan(&exists)
	if err != nil {
		slog.Error("failed to check channel name", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "a channel with this name already exists")
		return
	}

	// Create channel in a transaction to ensure atomicity
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}
	defer tx.Rollback(r.Context())

	var channelID string
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(r.Context(),
		`INSERT INTO channels (name, description, type, created_by)
		 VALUES ($1, $2, 'channel', $3)
		 RETURNING id, created_at, updated_at`,
		name, req.Description, userID,
	).Scan(&channelID, &createdAt, &updatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a channel with this name already exists")
			return
		}
		slog.Error("failed to create channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}

	// Add creator as owner member
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'user', $2, 'owner')`,
		channelID, userID,
	)
	if err != nil {
		slog.Error("failed to add creator as channel member", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}

	slog.Info("channel created", "channel_id", channelID, "name", name, "created_by", userID)

	writeJSON(w, http.StatusCreated, ChannelResponse{
		ID:          channelID,
		Name:        name,
		Description: req.Description,
		Type:        "channel",
		CreatedBy:   userID,
		IsArchived:  false,
		CreatedAt:   createdAt.Format(time.RFC3339),
		UpdatedAt:   updatedAt.Format(time.RFC3339),
	})
}

// List handles GET /api/v1/channels
func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT c.id, c.name, COALESCE(c.description, ''), c.type, c.created_by, c.is_archived, c.created_at, c.updated_at
		 FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user' AND cm.member_id = $1 AND c.is_archived = false AND c.type = 'channel'
		 ORDER BY c.created_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query channels", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list channels")
		return
	}
	defer rows.Close()

	channels := make([]ChannelResponse, 0)
	for rows.Next() {
		var ch ChannelResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy, &ch.IsArchived, &createdAt, &updatedAt)
		if err != nil {
			slog.Error("failed to scan channel row", "error", err)
			continue
		}
		ch.CreatedAt = createdAt.Format(time.RFC3339)
		ch.UpdatedAt = updatedAt.Format(time.RFC3339)
		channels = append(channels, ch)
	}

	writeJSON(w, http.StatusOK, channels)
}

// Get handles GET /api/v1/channels/{id}
func (h *ChannelHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	// Check user is a member
	var isMember bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("failed to check membership", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isMember {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	var ch ChannelResponse
	var createdAt, updatedAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description, ''), type, created_by, is_archived, created_at, updated_at
		 FROM channels WHERE id = $1 AND is_archived = false`,
		channelID,
	).Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy, &ch.IsArchived, &createdAt, &updatedAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		slog.Error("failed to query channel", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ch.CreatedAt = createdAt.Format(time.RFC3339)
	ch.UpdatedAt = updatedAt.Format(time.RFC3339)

	writeJSON(w, http.StatusOK, ch)
}

// Update handles PATCH /api/v1/channels/{id}
func (h *ChannelHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	// Check user is a member (any role can update for MVP; could restrict to owner later)
	var isMember bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("failed to check membership", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isMember {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	var req UpdateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate name if provided
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "channel name cannot be empty")
			return
		}
		if len(name) > 100 {
			writeError(w, http.StatusBadRequest, "channel name must be 100 characters or less")
			return
		}

		// Check name uniqueness (exclude current channel)
		var exists bool
		err := h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(
				SELECT 1 FROM channels
				WHERE name = $1 AND type = 'channel' AND is_archived = false AND id != $2
			)`, name, channelID,
		).Scan(&exists)
		if err != nil {
			slog.Error("failed to check channel name", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update channel")
			return
		}
		if exists {
			writeError(w, http.StatusConflict, "a channel with this name already exists")
			return
		}
	}

	// Build dynamic update query
	var ch ChannelResponse
	var createdAt, updatedAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`UPDATE channels SET
			name = COALESCE($1, name),
			description = COALESCE($2, description),
			updated_at = now()
		 WHERE id = $3 AND is_archived = false
		 RETURNING id, name, COALESCE(description, ''), type, created_by, is_archived, created_at, updated_at`,
		req.Name, req.Description, channelID,
	).Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy, &ch.IsArchived, &createdAt, &updatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a channel with this name already exists")
			return
		}
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		slog.Error("failed to update channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update channel")
		return
	}

	ch.CreatedAt = createdAt.Format(time.RFC3339)
	ch.UpdatedAt = updatedAt.Format(time.RFC3339)

	writeJSON(w, http.StatusOK, ch)
}

// Delete handles DELETE /api/v1/channels/{id} (soft-delete: sets is_archived=true)
func (h *ChannelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	// Check user is a member
	var isMember bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("failed to check membership", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isMember {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	result, err := h.pool.Exec(r.Context(),
		`UPDATE channels SET is_archived = true, updated_at = now()
		 WHERE id = $1 AND is_archived = false`,
		channelID,
	)
	if err != nil {
		slog.Error("failed to archive channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	slog.Info("channel archived", "channel_id", channelID, "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "channel deleted"})
}
