package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/service"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

// ChannelHandler handles channel-related HTTP requests.
type ChannelHandler struct {
	pool      *pgxpool.Pool
	dm        *service.DaemonManager
	hub       realtime.Broadcaster
	templates *service.TemplateService
}

// NewChannelHandler creates a new ChannelHandler.
func NewChannelHandler(pool *pgxpool.Pool, dm *service.DaemonManager, hub realtime.Broadcaster, templates ...*service.TemplateService) *ChannelHandler {
	templateSvc := service.NewTemplateService(pool)
	if len(templates) > 0 && templates[0] != nil {
		templateSvc = templates[0]
	}
	return &ChannelHandler{pool: pool, dm: dm, hub: hub, templates: templateSvc}
}

// --- Request/Response types ---

type CreateChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
	Locale      string `json:"locale,omitempty"`
}

type ApplyChannelTemplateRequest struct {
	TemplateID string `json:"template_id"`
	Locale     string `json:"locale,omitempty"`
}

type UpdateChannelRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ChannelResponse struct {
	ID               string `json:"id"`
	WorkspaceID      string `json:"workspace_id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Type             string `json:"type"`
	CreatedBy        string `json:"created_by"`
	IsArchived       bool   `json:"is_archived"`
	SourceTemplateID string `json:"source_template_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
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
	err := h.pool.QueryRow(r.Context(), `SELECT id FROM channels WHERE workspace_id = $1 AND name = $2 AND type = 'channel' AND is_archived = false`, serverworkspace.ID(r), channelName).Scan(&channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'user', $2) ON CONFLICT DO NOTHING`,
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
	// List public channels the user can see
	channels := []map[string]interface{}{}
	rows, err := h.pool.Query(r.Context(),
		`SELECT c.id, c.name, COALESCE(c.description,''), c.type,
		 EXISTS(SELECT 1 FROM channel_members WHERE channel_id=c.id AND member_id=$1) as joined
		 FROM channels c WHERE c.workspace_id=$2 AND ((c.type='channel' AND NOT c.is_archived) OR (c.type='lucy' AND NOT c.is_archived AND EXISTS(SELECT 1 FROM channel_members WHERE channel_id=c.id AND member_id=$1)) OR (c.type='dm' AND EXISTS(SELECT 1 FROM channel_members WHERE channel_id=c.id AND member_id=$1)))`, userID, serverworkspace.ID(r))
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
	rows2, err := h.pool.Query(r.Context(), `
		SELECT id, name, COALESCE(system_prompt, '')
		  FROM agents
		 WHERE is_active = true
		   AND owner_id = $1
		   AND home_channel_id IN (SELECT id FROM channels WHERE workspace_id = $2)
	`, userID, serverworkspace.ID(r))
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
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID != "" {
		result, err := h.templates.CreateChannel(r.Context(), service.TemplateProvisionRequest{
			OwnerID:     userID,
			WorkspaceID: serverworkspace.ID(r),
			TemplateID:  templateID,
			ChannelName: name,
			Description: req.Description,
			Locale:      req.Locale,
		})
		if err != nil {
			switch {
			case isUniqueViolation(err):
				writeError(w, http.StatusConflict, "a channel with this name already exists")
			case isNotFound(err):
				writeError(w, http.StatusNotFound, "template not found")
			case errors.Is(err, service.ErrTemplateRuntimeUnavailable):
				writeError(w, http.StatusUnprocessableEntity, err.Error())
			default:
				slog.Error("failed to create channel from template", "template_id", templateID, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create channel from template")
			}
			return
		}
		writeJSON(w, http.StatusCreated, ChannelResponse{
			ID:               result.ChannelID,
			WorkspaceID:      serverworkspace.ID(r),
			Name:             result.ChannelName,
			Description:      req.Description,
			Type:             "channel",
			CreatedBy:        userID,
			SourceTemplateID: result.TemplateID,
			CreatedAt:        result.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        result.CreatedAt.Format(time.RFC3339),
		})
		return
	}

	// Check for duplicate name
	var exists bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channels
			WHERE workspace_id = $1 AND name = $2 AND type = 'channel' AND is_archived = false
		)`, serverworkspace.ID(r), name,
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
		`INSERT INTO channels (workspace_id, name, description, type, created_by)
		 VALUES ($1, $2, $3, 'channel', $4)
		 RETURNING id, created_at, updated_at`,
		serverworkspace.ID(r), name, req.Description, userID,
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

	// Ordinary Channels inherit every active Workspace User. Agents, Lucy, and
	// DMs keep their explicit per-Channel memberships.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 SELECT $1, 'user', wm.user_id,
		        CASE WHEN wm.user_id = $3 THEN 'owner' ELSE 'member' END
		   FROM workspace_members wm
		   JOIN users u ON u.id = wm.user_id AND u.is_active = true
		  WHERE wm.workspace_id = $2`,
		channelID, serverworkspace.ID(r), userID,
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
		WorkspaceID: serverworkspace.ID(r),
		Name:        name,
		Description: req.Description,
		Type:        "channel",
		CreatedBy:   userID,
		IsArchived:  false,
		CreatedAt:   createdAt.Format(time.RFC3339),
		UpdatedAt:   updatedAt.Format(time.RFC3339),
	})
}

// ApplyTemplate handles POST /api/v1/channels/{channelID}/template.
func (h *ChannelHandler) ApplyTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := strings.TrimSpace(chi.URLParam(r, "channelID"))
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	var req ApplyChannelTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	if req.TemplateID == "" {
		writeError(w, http.StatusBadRequest, "template_id is required")
		return
	}

	result, err := h.templates.ApplyToChannel(r.Context(), service.TemplateProvisionRequest{
		OwnerID:         userID,
		WorkspaceID:     serverworkspace.ID(r),
		TemplateID:      req.TemplateID,
		TargetChannelID: channelID,
		Locale:          req.Locale,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTemplateTargetUnavailable):
			writeError(w, http.StatusNotFound, "channel not found")
		case errors.Is(err, service.ErrChannelTeamNotEmpty):
			writeError(w, http.StatusConflict, err.Error())
		case isNotFound(err):
			writeError(w, http.StatusNotFound, "template not found")
		case errors.Is(err, service.ErrTemplateRuntimeUnavailable):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			slog.Error("failed to apply template to channel", "channel_id", channelID, "template_id", req.TemplateID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to apply template to channel")
		}
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// List handles GET /api/v1/channels
func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT c.id, c.workspace_id::text, c.name, COALESCE(c.description, ''), c.type, c.created_by,
		        c.is_archived, COALESCE(c.source_template_id, ''), c.created_at, c.updated_at
		 FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user' AND cm.member_id = $1 AND c.workspace_id = $2 AND c.is_archived = false AND c.type = 'channel'
		 ORDER BY c.created_at DESC`,
		userID, serverworkspace.ID(r),
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
		err := rows.Scan(&ch.ID, &ch.WorkspaceID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy,
			&ch.IsArchived, &ch.SourceTemplateID, &createdAt, &updatedAt)
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

// GetLucy returns the authenticated user's pinned Lucy Channel. It is not
// included in the ordinary Channel list because the sidebar renders it first.
func (h *ChannelHandler) GetLucy(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var ch ChannelResponse
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		SELECT c.id, c.workspace_id::text, c.name, COALESCE(c.description, ''), c.type, c.created_by,
		       c.is_archived, COALESCE(c.source_template_id, ''), c.created_at, c.updated_at
		  FROM channels c
		  JOIN channel_members cm
		    ON cm.channel_id = c.id
		   AND cm.member_type = 'user'
		   AND cm.member_id = $1
	 WHERE c.type = 'lucy'
	   AND c.workspace_id = $2
	   AND c.is_archived = false
	 ORDER BY EXISTS (
	              SELECT 1
	                FROM agents lucy
	               WHERE lucy.owner_id = $1
	                 AND lucy.kind = 'lucy'
	                 AND lucy.is_active = true
	                 AND lucy.home_channel_id = c.id
	          ) DESC,
	          c.created_at ASC
	 LIMIT 1
	`, userID, serverworkspace.ID(r)).Scan(
		&ch.ID, &ch.WorkspaceID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy,
		&ch.IsArchived, &ch.SourceTemplateID, &createdAt, &updatedAt,
	)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "Lucy channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load Lucy channel")
		return
	}
	ch.CreatedAt = createdAt.Format(time.RFC3339)
	ch.UpdatedAt = updatedAt.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, ch)
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
			SELECT 1 FROM channel_members cm JOIN channels c ON c.id = cm.channel_id
			WHERE cm.channel_id = $1 AND cm.member_type = 'user' AND cm.member_id = $2 AND c.workspace_id = $3
		)`, channelID, userID, serverworkspace.ID(r),
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
		`SELECT c.id, c.workspace_id::text, c.name, COALESCE(c.description, ''), c.type, c.created_by,
		        c.is_archived, COALESCE(c.source_template_id, ''), c.created_at, c.updated_at
		 FROM channels c WHERE c.id = $1 AND c.workspace_id = $2 AND c.is_archived = false`,
		channelID, serverworkspace.ID(r),
	).Scan(&ch.ID, &ch.WorkspaceID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy,
		&ch.IsArchived, &ch.SourceTemplateID, &createdAt, &updatedAt)
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

	// Channel metadata is governed by Workspace roles, not inherited Channel membership.
	var canManage bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channels c JOIN workspace_members wm ON wm.workspace_id=c.workspace_id
			WHERE c.id=$1 AND c.workspace_id=$3 AND wm.user_id=$2 AND wm.role IN ('owner','admin')
		)`, channelID, userID, serverworkspace.ID(r),
	).Scan(&canManage)
	if err != nil {
		slog.Error("failed to check membership", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !canManage {
		writeError(w, http.StatusForbidden, "Workspace admin required")
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
					WHERE workspace_id = $1 AND name = $2 AND type = 'channel' AND is_archived = false AND id != $3
				)`, serverworkspace.ID(r), name, channelID,
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
		*req.Name = name
	}

	var ch ChannelResponse
	var createdAt, updatedAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`UPDATE channels SET
			name = COALESCE($1, name),
			description = COALESCE($2, description),
			updated_at = now()
			 WHERE id = $3 AND workspace_id = $4 AND is_archived = false
			 RETURNING id, workspace_id::text, name, COALESCE(description, ''), type, created_by,
			           is_archived, COALESCE(source_template_id, ''), created_at, updated_at`,
		req.Name, req.Description, channelID, serverworkspace.ID(r),
	).Scan(&ch.ID, &ch.WorkspaceID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy,
		&ch.IsArchived, &ch.SourceTemplateID, &createdAt, &updatedAt)
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

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	defer tx.Rollback(r.Context())

	var canDelete bool
	err = tx.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1
			  FROM channels c
			  JOIN workspace_members wm ON wm.workspace_id = c.workspace_id
			 WHERE c.id = $1
			   AND c.workspace_id = $3
			   AND c.type = 'channel'
			   AND c.is_archived = false
			   AND wm.user_id = $2
			   AND wm.role IN ('owner', 'admin')
		)
	`, channelID, userID, serverworkspace.ID(r)).Scan(&canDelete)
	if err != nil || !canDelete {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	agentRows, err := tx.Query(r.Context(), `
		SELECT id::text
		  FROM agents
		 WHERE home_channel_id = $1
		   AND kind = 'agent'
		   AND is_active = true
		 FOR UPDATE
	`, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	agentIDs := make([]string, 0)
	for agentRows.Next() {
		var agentID string
		if err := agentRows.Scan(&agentID); err != nil {
			agentRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to delete channel")
			return
		}
		agentIDs = append(agentIDs, agentID)
	}
	agentRows.Close()

	if _, err := tx.Exec(r.Context(), `
		UPDATE channels
		   SET is_archived = true, updated_at = now()
		 WHERE id = $1
	`, channelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE agents
		   SET is_active = false, updated_at = now()
		 WHERE home_channel_id = $1 AND kind = 'agent' AND is_active = true
	`, channelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE tasks
		   SET status = 'todo', claimer_id = NULL, updated_at = now()
		 WHERE channel_id = $1
		   AND status IN ('in_progress', 'in_review')
	`, channelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	cancelledRows, err := tx.Query(r.Context(), `
		UPDATE agent_runs
		   SET status = 'cancelled',
		       activity_text = 'Cancelled because the Channel was closed',
		       updated_at = now(),
		       finished_at = COALESCE(finished_at, now())
		 WHERE channel_id = $1
		   AND status IN (
		       'queued', 'thinking', 'running', 'streaming',
		       'waiting_input', 'waiting_approval'
		   )
		 RETURNING id::text
	`, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	cancelledRunIDs := make([]string, 0)
	for cancelledRows.Next() {
		var runID string
		if err := cancelledRows.Scan(&runID); err != nil {
			cancelledRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to delete channel")
			return
		}
		cancelledRunIDs = append(cancelledRunIDs, runID)
	}
	err = cancelledRows.Err()
	cancelledRows.Close()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	if err := service.NewBudgetService(h.pool).SettleTerminalRunsTx(r.Context(), tx, cancelledRunIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE agent_sessions
		   SET status = 'closed', last_active_at = now()
		 WHERE agent_id IN (
		       SELECT id FROM agents WHERE home_channel_id = $1
		   )
		   AND status = 'active'
	`, channelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	for _, agentID := range agentIDs {
		if _, err := tx.Exec(r.Context(), `
			UPDATE computers
			   SET agent_ids = array_remove(agent_ids, $1::uuid), updated_at = now()
			 WHERE $1::uuid = ANY(agent_ids)
		`, agentID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete channel")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}

	slog.Info("channel archived", "channel_id", channelID, "user_id", userID)

	if h.dm != nil && len(agentIDs) > 0 {
		go func(ids []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := h.dm.CleanupAgents(ctx, ids); err != nil {
				slog.Warn("failed to clean Agent sessions for archived channel", "channel_id", channelID, "error", err)
			}
		}(append([]string(nil), agentIDs...))
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "channel deleted"})
}
