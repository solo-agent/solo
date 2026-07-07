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

type WorkspaceViewDefaults struct {
	ChannelID  string `json:"channel_id"`
	Scope      string `json:"scope"`
	LeftTab    string `json:"left_tab"`
	RightPanel string `json:"right_panel"`
}

type ChannelContextResponse struct {
	ChannelID            string           `json:"channel_id"`
	Target               string           `json:"target"`
	Agenda               json.RawMessage  `json:"agenda"`
	ContextVersion       int              `json:"context_version"`
	LatestSummaryRecords []RecordResponse `json:"latest_summary_records"`
}

type RecordResponse struct {
	ID          string          `json:"id"`
	ChannelID   string          `json:"channel_id"`
	Scope       string          `json:"scope"`
	SubjectType string          `json:"subject_type,omitempty"`
	SubjectID   string          `json:"subject_id,omitempty"`
	RecordType  string          `json:"record_type"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	AuthorType  string          `json:"author_type"`
	AuthorID    string          `json:"author_id,omitempty"`
	ArtifactRef json.RawMessage `json:"artifact_ref,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

type TeamSurfaceResponse struct {
	Agents         []TeamAgentResponse        `json:"agents"`
	Relationships  []TeamRelationshipResponse `json:"relationships"`
	SummaryRecords []RecordResponse           `json:"summary_records"`
}

type TeamAgentResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
}

type TeamRelationshipResponse struct {
	ID          string `json:"id,omitempty"`
	FromAgentID string `json:"from_agent_id"`
	ToAgentID   string `json:"to_agent_id"`
	RelType     string `json:"rel_type"`
	Label       string `json:"label,omitempty"`
}

type ChannelWorkspaceResponse struct {
	Channel      ChannelResponse        `json:"channel"`
	ViewDefaults WorkspaceViewDefaults  `json:"view_defaults"`
	Context      ChannelContextResponse `json:"context"`
	Team         TeamSurfaceResponse    `json:"team"`
}

type PatchChannelContextRequest struct {
	Target *string          `json:"target,omitempty"`
	Agenda *json.RawMessage `json:"agenda,omitempty"`
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

// Workspace handles GET /api/v1/channels/{id}/workspace.
func (h *ChannelHandler) Workspace(w http.ResponseWriter, r *http.Request) {
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

	ch, ok := h.channelForMember(w, r, channelID, userID)
	if !ok {
		return
	}

	context, ok := h.channelContext(w, r, channelID)
	if !ok {
		return
	}
	context.LatestSummaryRecords = h.latestContextRecords(r, channelID, "summary", 5)
	team := h.channelTeamSurface(r, channelID)
	team.SummaryRecords = h.latestContextRecords(r, channelID, "team_summary", 5)

	writeJSON(w, http.StatusOK, ChannelWorkspaceResponse{
		Channel: ch,
		ViewDefaults: WorkspaceViewDefaults{
			ChannelID:  channelID,
			Scope:      "channel",
			LeftTab:    "conversation",
			RightPanel: "overview",
		},
		Context: context,
		Team:    team,
	})
}

func (h *ChannelHandler) channelTeamSurface(r *http.Request, channelID string) TeamSurfaceResponse {
	team := TeamSurfaceResponse{
		Agents:        []TeamAgentResponse{},
		Relationships: []TeamRelationshipResponse{},
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT a.id::text, a.name, cm.role,
		        CASE WHEN a.is_active THEN 'active' ELSE 'inactive' END
		   FROM channel_members cm
		   JOIN agents a ON a.id = cm.member_id
		  WHERE cm.channel_id = $1 AND cm.member_type = 'agent'
		  ORDER BY CASE WHEN cm.role IN ('lead', 'leader') THEN 0 ELSE 1 END, cm.joined_at ASC, a.name ASC`,
		channelID,
	)
	if err != nil {
		slog.Warn("failed to query channel team agents", "channel_id", channelID, "error", err)
		return team
	}
	defer rows.Close()
	for rows.Next() {
		var agent TeamAgentResponse
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Role, &agent.Status); err != nil {
			slog.Warn("failed to scan channel team agent", "channel_id", channelID, "error", err)
			continue
		}
		team.Agents = append(team.Agents, agent)
	}

	relRows, err := h.pool.Query(r.Context(),
		`SELECT r.id::text, r.from_agent_id::text, r.to_agent_id::text, r.rel_type, COALESCE(r.instruction, '')
		   FROM agent_relationships r
		  WHERE r.from_agent_id IN (
			SELECT member_id FROM channel_members WHERE channel_id = $1 AND member_type = 'agent'
		  )
		    AND r.to_agent_id IN (
			SELECT member_id FROM channel_members WHERE channel_id = $1 AND member_type = 'agent'
		  )
		  ORDER BY r.created_at ASC`,
		channelID,
	)
	if err != nil {
		slog.Warn("failed to query channel team relationships", "channel_id", channelID, "error", err)
		return team
	}
	defer relRows.Close()
	for relRows.Next() {
		var rel TeamRelationshipResponse
		if err := relRows.Scan(&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.Label); err != nil {
			slog.Warn("failed to scan channel team relationship", "channel_id", channelID, "error", err)
			continue
		}
		team.Relationships = append(team.Relationships, rel)
	}
	return team
}

// PatchContext handles PATCH /api/v1/channels/{id}/context.
func (h *ChannelHandler) PatchContext(w http.ResponseWriter, r *http.Request) {
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

	var req PatchChannelContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Target == nil && req.Agenda == nil {
		writeError(w, http.StatusBadRequest, "target or agenda is required")
		return
	}

	var targetArg any
	if req.Target != nil {
		target := strings.TrimSpace(*req.Target)
		if len(target) > 10000 {
			writeError(w, http.StatusBadRequest, "target must be 10000 characters or less")
			return
		}
		targetArg = target
	}

	var agendaArg any
	if req.Agenda != nil {
		agenda := strings.TrimSpace(string(*req.Agenda))
		if agenda == "" || agenda[0] != '[' || !json.Valid([]byte(agenda)) {
			writeError(w, http.StatusBadRequest, "agenda must be a JSON array")
			return
		}
		agendaArg = agenda
	}

	if _, ok := h.channelForMember(w, r, channelID, userID); !ok {
		return
	}

	var target string
	var agenda json.RawMessage
	var version int
	err := h.pool.QueryRow(r.Context(), `
		WITH incoming AS (
			SELECT $1::uuid AS channel_id, $2::text AS target, $3::jsonb AS agenda_json
		)
		INSERT INTO channel_contexts (channel_id, target, agenda_json)
		SELECT channel_id, COALESCE(target, ''), COALESCE(agenda_json, '[]'::jsonb)
		  FROM incoming
		ON CONFLICT (channel_id) DO UPDATE SET
			target = CASE WHEN $2::text IS NULL THEN channel_contexts.target ELSE EXCLUDED.target END,
			agenda_json = CASE WHEN $3::jsonb IS NULL THEN channel_contexts.agenda_json ELSE EXCLUDED.agenda_json END,
			context_version = CASE
				WHEN channel_contexts.target IS DISTINCT FROM CASE WHEN $2::text IS NULL THEN channel_contexts.target ELSE EXCLUDED.target END
				  OR channel_contexts.agenda_json IS DISTINCT FROM CASE WHEN $3::jsonb IS NULL THEN channel_contexts.agenda_json ELSE EXCLUDED.agenda_json END
				THEN channel_contexts.context_version + 1
				ELSE channel_contexts.context_version
			END,
			updated_at = CASE
				WHEN channel_contexts.target IS DISTINCT FROM CASE WHEN $2::text IS NULL THEN channel_contexts.target ELSE EXCLUDED.target END
				  OR channel_contexts.agenda_json IS DISTINCT FROM CASE WHEN $3::jsonb IS NULL THEN channel_contexts.agenda_json ELSE EXCLUDED.agenda_json END
				THEN now()
				ELSE channel_contexts.updated_at
			END
		RETURNING target, agenda_json, context_version`,
		channelID, targetArg, agendaArg,
	).Scan(&target, &agenda, &version)
	if err != nil {
		slog.Error("failed to patch channel context", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update channel context")
		return
	}

	writeJSON(w, http.StatusOK, ChannelContextResponse{
		ChannelID:            channelID,
		Target:               target,
		Agenda:               normalizeRawJSON(agenda, "[]"),
		ContextVersion:       version,
		LatestSummaryRecords: h.latestContextRecords(r, channelID, "summary", 5),
	})
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

func (h *ChannelHandler) channelForMember(w http.ResponseWriter, r *http.Request, channelID, userID string) (ChannelResponse, bool) {
	var ch ChannelResponse
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT c.id, c.name, COALESCE(c.description, ''), c.type, c.created_by, c.is_archived, c.created_at, c.updated_at
		 FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE c.id = $1 AND cm.member_type = 'user' AND cm.member_id = $2 AND c.is_archived = false`,
		channelID, userID,
	).Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Type, &ch.CreatedBy, &ch.IsArchived, &createdAt, &updatedAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "channel not found")
			return ch, false
		}
		slog.Error("failed to query channel workspace", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return ch, false
	}
	ch.CreatedAt = createdAt.Format(time.RFC3339)
	ch.UpdatedAt = updatedAt.Format(time.RFC3339)
	return ch, true
}

func (h *ChannelHandler) channelContext(w http.ResponseWriter, r *http.Request, channelID string) (ChannelContextResponse, bool) {
	context := ChannelContextResponse{
		ChannelID:      channelID,
		Agenda:         json.RawMessage("[]"),
		ContextVersion: 1,
	}
	var agenda json.RawMessage
	err := h.pool.QueryRow(r.Context(),
		`SELECT target, agenda_json, context_version FROM channel_contexts WHERE channel_id = $1`,
		channelID,
	).Scan(&context.Target, &agenda, &context.ContextVersion)
	if err != nil {
		if isNotFound(err) {
			return context, true
		}
		slog.Error("failed to query channel context", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query channel context")
		return context, false
	}
	context.Agenda = normalizeRawJSON(agenda, "[]")
	return context, true
}

func (h *ChannelHandler) latestContextRecords(r *http.Request, channelID, recordType string, limit int) []RecordResponse {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id::text, channel_id::text, scope, COALESCE(subject_type, ''), COALESCE(subject_id::text, ''),
		        record_type, title, body, author_type, COALESCE(author_id::text, ''),
		        COALESCE(artifact_ref_json::text, ''), created_at
		   FROM context_records
		  WHERE channel_id = $1 AND record_type = $2
		  ORDER BY created_at DESC
		  LIMIT $3`,
		channelID, recordType, limit,
	)
	if err != nil {
		slog.Warn("failed to query context records", "channel_id", channelID, "record_type", recordType, "error", err)
		return []RecordResponse{}
	}
	defer rows.Close()

	records := []RecordResponse{}
	for rows.Next() {
		var rec RecordResponse
		var artifactRef string
		var createdAt time.Time
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.Scope, &rec.SubjectType, &rec.SubjectID, &rec.RecordType, &rec.Title, &rec.Body, &rec.AuthorType, &rec.AuthorID, &artifactRef, &createdAt); err != nil {
			slog.Warn("failed to scan context record", "channel_id", channelID, "error", err)
			continue
		}
		if artifactRef != "" {
			rec.ArtifactRef = json.RawMessage(artifactRef)
		}
		rec.CreatedAt = createdAt.Format(time.RFC3339)
		records = append(records, rec)
	}
	return records
}

func normalizeRawJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(fallback)
	}
	return raw
}
