package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/onboarding"
	"github.com/solo-ai/solo/internal/server/service"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

type WorkspaceHandler struct {
	pool *pgxpool.Pool
	dm   *service.DaemonManager
}

func NewWorkspaceHandler(pool *pgxpool.Pool, daemonManagers ...*service.DaemonManager) *WorkspaceHandler {
	h := &WorkspaceHandler{pool: pool}
	if len(daemonManagers) > 0 {
		h.dm = daemonManagers[0]
	}
	return h
}

type WorkspaceResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	Visibility    string `json:"visibility"`
	IsDefault     bool   `json:"is_default"`
	IsPersonal    bool   `json:"is_personal"`
	LucyChannelID string `json:"lucy_channel_id,omitempty"`
	MemberCount   int    `json:"member_count"`
	Role          string `json:"role"`
	CreatedBy     string `json:"created_by,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type WorkspaceMemberResponse struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at"`
}

type WorkspaceInvitationResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

type WorkspaceInviteLinkResponse struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	ExpiresAt string  `json:"expires_at"`
	RevokedAt *string `json:"revoked_at,omitempty"`
	UseCount  int     `json:"use_count"`
	CreatedAt string  `json:"created_at"`
	URL       string  `json:"url,omitempty"`
}

type WorkspaceInviteLinkInfoResponse struct {
	WorkspaceID       string `json:"workspace_id"`
	WorkspaceName     string `json:"workspace_name"`
	WorkspaceIcon     string `json:"workspace_icon"`
	InvitedBy         string `json:"invited_by"`
	ExpiresAt         string `json:"expires_at"`
	TargetChannelID   string `json:"target_channel_id,omitempty"`
	TargetChannelName string `json:"target_channel_name,omitempty"`
	MemberCount       int    `json:"member_count"`
}

type WorkspaceJoinRuleResponse struct {
	ID        string `json:"id"`
	RuleType  string `json:"rule_type"`
	Value     string `json:"value"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type WorkspaceEmbedChannelResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkspaceGuestTokenResponse struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	ExpiresAt string  `json:"expires_at"`
	RevokedAt *string `json:"revoked_at,omitempty"`
	CreatedAt string  `json:"created_at"`
	Token     string  `json:"token,omitempty"`
	URL       string  `json:"url,omitempty"`
}

type WorkspaceEmbedResponse struct {
	Enabled               bool                            `json:"enabled"`
	AllowAgentInvocations bool                            `json:"allow_agent_invocations"`
	Channels              []WorkspaceEmbedChannelResponse `json:"channels"`
	Tokens                []WorkspaceGuestTokenResponse   `json:"tokens"`
}

type GuestEmbedMessageResponse struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

type GuestEmbedResponse struct {
	WorkspaceID   string                          `json:"workspace_id"`
	WorkspaceName string                          `json:"workspace_name"`
	WorkspaceIcon string                          `json:"workspace_icon"`
	GuestID       string                          `json:"guest_id"`
	ExpiresAt     string                          `json:"expires_at"`
	Channels      []WorkspaceEmbedChannelResponse `json:"channels"`
}

func (h *WorkspaceHandler) EmbedSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	settings, err := h.loadEmbedSettings(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Guest/Embed settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *WorkspaceHandler) UpdateEmbedSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		Enabled    bool     `json:"enabled"`
		ChannelIDs []string `json:"channel_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	seen := make(map[string]bool, len(req.ChannelIDs))
	for _, channelID := range req.ChannelIDs {
		if _, err := uuid.Parse(channelID); err != nil || seen[channelID] {
			writeError(w, http.StatusBadRequest, "invalid Channel selection")
			return
		}
		seen[channelID] = true
	}
	if req.Enabled && len(req.ChannelIDs) == 0 {
		writeError(w, http.StatusBadRequest, "select at least one Channel before enabling Guest access")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update Guest/Embed settings")
		return
	}
	defer tx.Rollback(r.Context())
	var validCount int
	if len(req.ChannelIDs) > 0 {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM channels WHERE id=ANY($1) AND workspace_id=$2 AND type='channel' AND is_archived=false`, req.ChannelIDs, workspaceID).Scan(&validCount)
	}
	if err != nil || validCount != len(req.ChannelIDs) {
		writeError(w, http.StatusBadRequest, "all selected Channels must belong to this Workspace")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO workspace_embed_policies (workspace_id,enabled,allow_agent_invocations,updated_by)
		VALUES ($1,$2,false,$3)
		ON CONFLICT (workspace_id) DO UPDATE
		SET enabled=EXCLUDED.enabled,allow_agent_invocations=false,updated_by=EXCLUDED.updated_by,updated_at=now()`, workspaceID, req.Enabled, userID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM workspace_embed_channels WHERE workspace_id=$1`, workspaceID)
	}
	if err == nil && len(req.ChannelIDs) > 0 {
		_, err = tx.Exec(r.Context(), `INSERT INTO workspace_embed_channels(workspace_id,channel_id) SELECT $1, unnest($2::uuid[])`, workspaceID, req.ChannelIDs)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to update Guest/Embed settings")
		return
	}
	settings, err := h.loadEmbedSettings(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Guest/Embed settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *WorkspaceHandler) CreateGuestToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		Label         string `json:"label"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if len(req.Label) > 100 {
		writeError(w, http.StatusBadRequest, "label is too long")
		return
	}
	if req.ExpiresInDays == 0 {
		req.ExpiresInDays = 7
	}
	if req.ExpiresInDays < 1 || req.ExpiresInDays > 30 {
		writeError(w, http.StatusBadRequest, "Guest link expiry must be between 1 and 30 days")
		return
	}
	var enabled bool
	if err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT enabled FROM workspace_embed_policies WHERE workspace_id=$1),false)`, workspaceID).Scan(&enabled); err != nil || !enabled {
		writeError(w, http.StatusConflict, "enable Guest access before creating a link")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Guest link")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	var item WorkspaceGuestTokenResponse
	var expiresAt, createdAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO workspace_guest_tokens(workspace_id,token_hash,label,expires_at,created_by)
		VALUES($1,$2,$3,now()+($4*interval '1 day'),$5)
		RETURNING id::text,label,expires_at,created_at`, workspaceID, hash[:], req.Label, req.ExpiresInDays, userID).
		Scan(&item.ID, &item.Label, &expiresAt, &createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Guest link")
		return
	}
	item.Token = token
	item.URL = "/guest/" + token
	item.ExpiresAt = expiresAt.Format(time.RFC3339)
	item.CreatedAt = createdAt.Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, item)
}

func (h *WorkspaceHandler) RevokeGuestToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	result, err := h.pool.Exec(r.Context(), `UPDATE workspace_guest_tokens SET revoked_at=now() WHERE id=$1 AND workspace_id=$2 AND revoked_at IS NULL`, chi.URLParam(r, "tokenID"), workspaceID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Guest link not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) GuestEmbed(w http.ResponseWriter, r *http.Request) {
	guest, ok := h.authorizeGuest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Guest link is invalid, expired, or revoked")
		return
	}
	writeJSON(w, http.StatusOK, guest)
}

func (h *WorkspaceHandler) GuestMessages(w http.ResponseWriter, r *http.Request) {
	guest, ok := h.authorizeGuest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Guest link is invalid, expired, or revoked")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	allowed := false
	for _, channel := range guest.Channels {
		if channel.ID == channelID {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "Channel not found")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id::text,m.sender_type,CASE WHEN m.sender_type='system' THEN 'Solo' ELSE COALESCE(u.display_name,a.name,'Unknown') END,m.content,m.created_at
		FROM messages m
		LEFT JOIN users u ON m.sender_type='user' AND u.id=m.sender_id
		LEFT JOIN agents a ON m.sender_type='agent' AND a.id=m.sender_id
		WHERE m.channel_id=$1 AND m.thread_id IS NULL AND m.thinking_node_id IS NULL AND COALESCE(m.is_deleted,false)=false
		ORDER BY m.created_at DESC,m.id DESC LIMIT $2`, channelID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list Guest messages")
		return
	}
	defer rows.Close()
	items := make([]GuestEmbedMessageResponse, 0)
	for rows.Next() {
		var item GuestEmbedMessageResponse
		var createdAt time.Time
		if rows.Scan(&item.ID, &item.SenderType, &item.SenderName, &item.Content, &createdAt) == nil {
			item.CreatedAt = createdAt.Format(time.RFC3339)
			items = append(items, item)
		}
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": items})
}

func (h *WorkspaceHandler) loadEmbedSettings(r *http.Request, workspaceID string) (*WorkspaceEmbedResponse, error) {
	result := &WorkspaceEmbedResponse{Channels: []WorkspaceEmbedChannelResponse{}, Tokens: []WorkspaceGuestTokenResponse{}}
	if err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT enabled FROM workspace_embed_policies WHERE workspace_id=$1),false)`, workspaceID).Scan(&result.Enabled); err != nil {
		return nil, err
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.name FROM workspace_embed_channels ec JOIN channels c ON c.id=ec.channel_id WHERE ec.workspace_id=$1 ORDER BY c.name`, workspaceID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item WorkspaceEmbedChannelResponse
		if rows.Scan(&item.ID, &item.Name) == nil {
			result.Channels = append(result.Channels, item)
		}
	}
	rows.Close()
	rows, err = h.pool.Query(r.Context(), `SELECT id::text,label,expires_at,revoked_at,created_at FROM workspace_guest_tokens WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item WorkspaceGuestTokenResponse
		var expiresAt, createdAt time.Time
		var revokedAt *time.Time
		if rows.Scan(&item.ID, &item.Label, &expiresAt, &revokedAt, &createdAt) == nil {
			item.ExpiresAt, item.CreatedAt = expiresAt.Format(time.RFC3339), createdAt.Format(time.RFC3339)
			if revokedAt != nil {
				formatted := revokedAt.Format(time.RFC3339)
				item.RevokedAt = &formatted
			}
			result.Tokens = append(result.Tokens, item)
		}
	}
	return result, rows.Err()
}

func (h *WorkspaceHandler) authorizeGuest(r *http.Request) (*GuestEmbedResponse, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Guest ") {
		return nil, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Guest "))
	if token == "" {
		return nil, false
	}
	hash := sha256.Sum256([]byte(token))
	result := &GuestEmbedResponse{Channels: []WorkspaceEmbedChannelResponse{}}
	var expiresAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		SELECT gt.workspace_id::text,w.name,w.icon,gt.id::text,gt.expires_at
		FROM workspace_guest_tokens gt
		JOIN workspaces w ON w.id=gt.workspace_id AND w.deleted_at IS NULL
		JOIN workspace_embed_policies p ON p.workspace_id=gt.workspace_id AND p.enabled=true
		WHERE gt.token_hash=$1 AND gt.revoked_at IS NULL AND gt.expires_at>now()`, hash[:]).
		Scan(&result.WorkspaceID, &result.WorkspaceName, &result.WorkspaceIcon, &result.GuestID, &expiresAt)
	if err != nil {
		return nil, false
	}
	result.ExpiresAt = expiresAt.Format(time.RFC3339)
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.name FROM workspace_embed_channels ec JOIN channels c ON c.id=ec.channel_id WHERE ec.workspace_id=$1 AND c.type='channel' AND c.is_archived=false ORDER BY c.name`, result.WorkspaceID)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		var channel WorkspaceEmbedChannelResponse
		if rows.Scan(&channel.ID, &channel.Name) == nil {
			result.Channels = append(result.Channels, channel)
		}
	}
	return result, rows.Err() == nil
}

func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT ws.id::text, ws.name, ws.icon, ws.visibility, ws.is_default, ws.is_personal,
		       (SELECT count(*) FROM workspace_members members WHERE members.workspace_id=ws.id),
		       COALESCE((SELECT c.id::text FROM channels c WHERE c.workspace_id=ws.id AND c.type='lucy' AND c.is_archived=false LIMIT 1),''),
		       wm.role, COALESCE(ws.created_by::text, ''), ws.created_at, ws.updated_at
		  FROM workspace_members wm
		  JOIN workspaces ws ON ws.id = wm.workspace_id
		 WHERE wm.user_id = $1 AND ws.deleted_at IS NULL
		 ORDER BY ws.is_personal DESC, ws.is_default DESC, wm.joined_at ASC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list Workspaces")
		return
	}
	defer rows.Close()
	result := make([]WorkspaceResponse, 0)
	for rows.Next() {
		var item WorkspaceResponse
		var createdAt, updatedAt time.Time
		if rows.Scan(&item.ID, &item.Name, &item.Icon, &item.Visibility, &item.IsDefault, &item.IsPersonal, &item.MemberCount, &item.LucyChannelID, &item.Role, &item.CreatedBy, &createdAt, &updatedAt) == nil {
			item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339), updatedAt.Format(time.RFC3339)
			result = append(result, item)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, icon := strings.TrimSpace(req.Name), strings.TrimSpace(req.Icon)
	if name == "" || len(name) > 100 {
		writeError(w, http.StatusBadRequest, "Workspace name must be 1-100 characters")
		return
	}
	if icon == "" {
		icon = strings.ToUpper(string([]rune(name)[0]))
	}
	if len([]rune(icon)) > 8 {
		writeError(w, http.StatusBadRequest, "Workspace icon is too long")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Workspace")
		return
	}
	defer tx.Rollback(r.Context())
	var isPersonal bool
	if err = tx.QueryRow(r.Context(), `
		SELECT NOT EXISTS (
			SELECT 1 FROM workspaces WHERE created_by=$1 AND is_personal=true AND deleted_at IS NULL
		) FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&isPersonal); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Workspace")
		return
	}
	workspaceID, channelID, lucyChannelID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(r.Context(), `
		INSERT INTO workspaces (id, name, icon, visibility, is_personal, created_by)
		VALUES ($1, $2, $3, 'private', $4, $5)
		RETURNING created_at, updated_at`, workspaceID, name, icon, isPersonal, userID).Scan(&createdAt, &updatedAt)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, workspaceID, userID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO channels (id, workspace_id, name, description, type, created_by) VALUES ($1, $2, 'general', 'Workspace lobby', 'channel', $3)`, channelID, workspaceID, userID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO channel_members (channel_id, member_type, member_id, role) VALUES ($1, 'user', $2, 'owner')`, channelID, userID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO channels (id, workspace_id, name, description, type, created_by) VALUES ($1, $2, 'lucy', 'Your pinned Channel with Lucy, this Workspace''s steward.', 'lucy', $3)`, lucyChannelID, workspaceID, userID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO channel_members (channel_id, member_type, member_id, role) VALUES ($1, 'user', $2, 'owner')`, lucyChannelID, userID)
	}
	lucyAgentID := uuid.NewString()
	lucyCloned := false
	if err == nil && !isPersonal {
		var tag interface{ RowsAffected() int64 }
		tag, err = tx.Exec(r.Context(), `
			INSERT INTO agents (
				id,name,description,owner_id,model_provider,model_name,system_prompt,
				runtime_id,custom_env,custom_args,avatar_url,home_channel_id,kind
			)
			SELECT $1,'Lucy','Workspace steward — helps you create and manage Agent teams.',
			       $2,a.model_provider,a.model_name,a.system_prompt,a.runtime_id,
			       a.custom_env,a.custom_args,'dicebear:pixel-art:lucy',$3,'lucy'
			  FROM agents a
			  JOIN computers computer ON computer.id::text=a.runtime_id
			  LEFT JOIN computer_members computer_member
			    ON computer_member.computer_id=computer.id AND computer_member.user_id=$2
			 WHERE a.owner_id=$2 AND a.kind='lucy' AND a.is_active=true AND a.runtime_id IS NOT NULL
			   AND (computer.owner_id=$2 OR computer_member.user_id IS NOT NULL)
			   AND ((computer.credential_hash IS NOT NULL AND computer.credential_revoked_at IS NULL)
			     OR (computer.daemon_id IS NOT NULL AND computer.status='online'))
			 ORDER BY a.created_at ASC LIMIT 1`, lucyAgentID, userID, lucyChannelID)
		if err == nil {
			lucyCloned = tag.RowsAffected() == 1
		}
	}
	if err == nil && lucyCloned {
		_, err = tx.Exec(r.Context(), `INSERT INTO channel_members (channel_id,member_type,member_id,role) VALUES ($1,'agent',$2,'member')`, lucyChannelID, lucyAgentID)
	}
	if err == nil && !lucyCloned {
		var displayName string
		if queryErr := tx.QueryRow(r.Context(), `SELECT display_name FROM users WHERE id=$1`, userID).Scan(&displayName); queryErr != nil {
			err = queryErr
		} else {
			_, err = tx.Exec(r.Context(), `INSERT INTO messages (channel_id,sender_type,sender_id,content,content_type) VALUES ($1,'system','00000000-0000-0000-0000-000000000000',$2,'system')`, lucyChannelID, onboarding.WizardWelcomePrompt(displayName))
		}
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Workspace")
		return
	}
	if lucyCloned {
		var displayName, email string
		if h.pool.QueryRow(r.Context(), `SELECT display_name,email FROM users WHERE id=$1`, userID).Scan(&displayName, &email) == nil {
			go onboarding.SeedAgentKnowledge(lucyAgentID, displayName, email)
		}
	}
	writeJSON(w, http.StatusCreated, WorkspaceResponse{ID: workspaceID, Name: name, Icon: icon, Visibility: "private", IsPersonal: isPersonal, LucyChannelID: lucyChannelID, MemberCount: 1, Role: "owner", CreatedBy: userID, CreatedAt: createdAt.Format(time.RFC3339), UpdatedAt: updatedAt.Format(time.RFC3339)})
}

func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		Name *string `json:"name"`
		Icon *string `json:"icon"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if value == "" || len(value) > 100 {
			writeError(w, http.StatusBadRequest, "Workspace name must be 1-100 characters")
			return
		}
		req.Name = &value
	}
	if req.Icon != nil {
		value := strings.TrimSpace(*req.Icon)
		if value == "" || len([]rune(value)) > 8 {
			writeError(w, http.StatusBadRequest, "Workspace icon must be 1-8 characters")
			return
		}
		req.Icon = &value
	}
	var name, icon, visibility, role, createdBy string
	var isDefault, isPersonal bool
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		UPDATE workspaces SET
		 name = CASE WHEN $3::text IS NULL THEN name ELSE NULLIF(trim($3), '') END,
		 icon = CASE WHEN $4::text IS NULL THEN icon ELSE NULLIF(trim($4), '') END,
		 updated_at = now()
		WHERE id = $1 AND EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2 AND role IN ('owner','admin'))
		RETURNING name, icon, visibility, is_default, is_personal, COALESCE(created_by::text,''), created_at, updated_at`, workspaceID, userID, req.Name, req.Icon).Scan(&name, &icon, &visibility, &isDefault, &isPersonal, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid Workspace update")
		return
	}
	_ = h.pool.QueryRow(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).Scan(&role)
	var memberCount int
	_ = h.pool.QueryRow(r.Context(), `SELECT count(*) FROM workspace_members WHERE workspace_id=$1`, workspaceID).Scan(&memberCount)
	writeJSON(w, http.StatusOK, WorkspaceResponse{ID: workspaceID, Name: name, Icon: icon, Visibility: visibility, IsDefault: isDefault, IsPersonal: isPersonal, MemberCount: memberCount, Role: role, CreatedBy: createdBy, CreatedAt: createdAt.Format(time.RFC3339), UpdatedAt: updatedAt.Format(time.RFC3339)})
}

func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if workspaceID == serverworkspace.PublicID {
		writeError(w, http.StatusBadRequest, "the public Workspace cannot be deleted")
		return
	}
	var isPersonal bool
	if err := h.pool.QueryRow(r.Context(), `SELECT is_personal FROM workspaces WHERE id=$1 AND deleted_at IS NULL`, workspaceID).Scan(&isPersonal); err != nil {
		writeError(w, http.StatusNotFound, "Workspace not found")
		return
	}
	if isPersonal {
		writeError(w, http.StatusBadRequest, "the personal Workspace cannot be deleted")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete Workspace")
		return
	}
	defer tx.Rollback(r.Context())
	var allowed bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspaces ws JOIN workspace_members wm ON wm.workspace_id=ws.id WHERE ws.id=$1 AND ws.deleted_at IS NULL AND wm.user_id=$2 AND wm.role='owner')`, workspaceID, userID).Scan(&allowed); err != nil || !allowed {
		writeError(w, http.StatusForbidden, "Workspace owner required")
		return
	}
	var activeRuns int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM agent_runs r JOIN channels c ON c.id=r.channel_id WHERE c.workspace_id=$1 AND r.finished_at IS NULL`, workspaceID).Scan(&activeRuns); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate Workspace")
		return
	}
	if activeRuns > 0 {
		writeError(w, http.StatusConflict, "Workspace has active Agent Runs")
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT a.id::text FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id=$1 AND a.is_active=true FOR UPDATE OF a`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete Workspace")
		return
	}
	agentIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			agentIDs = append(agentIDs, id)
		}
	}
	rows.Close()
	if _, err = tx.Exec(r.Context(), `UPDATE agents a SET is_active=false,updated_at=now() FROM channels c WHERE a.home_channel_id=c.id AND c.workspace_id=$1`, workspaceID); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE agent_sessions s SET status='closed',last_active_at=now() FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE s.agent_id=a.id AND c.workspace_id=$1 AND s.status='active'`, workspaceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE computers SET agent_ids=array(SELECT unnest(agent_ids) EXCEPT SELECT unnest($1::uuid[])),updated_at=now() WHERE agent_ids && $1::uuid[]`, agentIDs)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE channels SET is_archived=true,updated_at=now() WHERE workspace_id=$1`, workspaceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE workspace_embed_policies SET enabled=false,updated_at=now() WHERE workspace_id=$1`, workspaceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE workspace_guest_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE workspace_id=$1`, workspaceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM workspace_invitations WHERE workspace_id=$1`, workspaceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE workspaces SET deleted_at=now(),updated_at=now() WHERE id=$1`, workspaceID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete Workspace")
		return
	}
	if h.dm != nil && len(agentIDs) > 0 {
		go func(ids []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := h.dm.CleanupAgents(ctx, ids); err != nil {
				slog.Warn("Workspace cleanup: Daemon cleanup failed", "workspace_id", workspaceID, "error", err)
			}
		}(append([]string(nil), agentIDs...))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) Members(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin", "member") {
		writeError(w, http.StatusForbidden, "Workspace access denied")
		return
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}
	rows, err := h.pool.Query(r.Context(), `SELECT u.id::text,u.email,u.display_name,COALESCE(u.avatar_url,''),wm.role,wm.joined_at FROM workspace_members wm JOIN users u ON u.id=wm.user_id WHERE wm.workspace_id=$1 ORDER BY (wm.user_id=$3::uuid) DESC, CASE wm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, wm.joined_at LIMIT NULLIF($2::int,0)`, workspaceID, limit, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	defer rows.Close()
	items := make([]WorkspaceMemberResponse, 0)
	for rows.Next() {
		var item WorkspaceMemberResponse
		var joined time.Time
		if rows.Scan(&item.UserID, &item.Email, &item.DisplayName, &item.AvatarURL, &item.Role, &joined) == nil {
			item.JoinedAt = joined.Format(time.RFC3339)
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WorkspaceHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	actorRole := h.role(r, workspaceID, adminID)
	if actorRole != "owner" && actorRole != "admin" {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "member"
	}
	if role != "member" && role != "admin" {
		writeError(w, http.StatusBadRequest, "role must be member or admin")
		return
	}
	if actorRole != "owner" && role == "admin" {
		writeError(w, http.StatusForbidden, "only the Workspace owner can add an admin")
		return
	}
	userID := strings.TrimSpace(req.UserID)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if userID == "" && email != "" {
		_ = h.pool.QueryRow(r.Context(), `SELECT id::text FROM users WHERE lower(email)=$1 AND is_active=true`, email).Scan(&userID)
	}
	if userID == "" {
		if email == "" {
			writeError(w, http.StatusBadRequest, "user_id or email is required")
			return
		}
		var item WorkspaceInvitationResponse
		var expiresAt, createdAt time.Time
		err := h.pool.QueryRow(r.Context(), `INSERT INTO workspace_invitations (workspace_id,email,role,invited_by) VALUES ($1,$2,$3,$4) ON CONFLICT (workspace_id,(lower(email))) WHERE accepted_at IS NULL DO UPDATE SET role=EXCLUDED.role,invited_by=EXCLUDED.invited_by,expires_at=now()+interval '30 days' RETURNING id::text,email,role,expires_at,created_at`, workspaceID, email, role, adminID).Scan(&item.ID, &item.Email, &item.Role, &expiresAt, &createdAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create invitation")
			return
		}
		item.ExpiresAt = expiresAt.Format(time.RFC3339)
		item.CreatedAt = createdAt.Format(time.RFC3339)
		writeJSON(w, http.StatusAccepted, item)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	defer tx.Rollback(r.Context())
	var memberTag interface{ RowsAffected() int64 }
	memberTag, err = tx.Exec(r.Context(), `
		INSERT INTO workspace_members (workspace_id,user_id,role)
		VALUES ($1,$2,$3)
		ON CONFLICT (workspace_id,user_id) DO NOTHING`, workspaceID, userID, role)
	if err == nil && memberTag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "user is already a Workspace member; change their role separately")
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO channel_members (channel_id,member_type,member_id,role)
			SELECT id,'user',$2,'member' FROM channels
			 WHERE workspace_id=$1 AND type='channel' AND is_archived=false
			ON CONFLICT DO NOTHING`, workspaceID, userID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusBadRequest, "failed to add member")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"user_id": userID, "role": role})
}

func (h *WorkspaceHandler) Invitations(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id::text,email,role,expires_at,created_at
		  FROM workspace_invitations
		 WHERE workspace_id=$1 AND accepted_at IS NULL AND expires_at>now()
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invitations")
		return
	}
	defer rows.Close()
	items := make([]WorkspaceInvitationResponse, 0)
	for rows.Next() {
		var item WorkspaceInvitationResponse
		var expiresAt, createdAt time.Time
		if rows.Scan(&item.ID, &item.Email, &item.Role, &expiresAt, &createdAt) == nil {
			item.ExpiresAt = expiresAt.Format(time.RFC3339)
			item.CreatedAt = createdAt.Format(time.RFC3339)
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WorkspaceHandler) DeleteInvitation(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	result, err := h.pool.Exec(r.Context(), `DELETE FROM workspace_invitations WHERE id=$1 AND workspace_id=$2 AND accepted_at IS NULL`, chi.URLParam(r, "invitationID"), workspaceID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "invitation not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) CreateInviteLink(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		ExpiresInDays int `json:"expires_in_days"`
	}
	if r.Body != nil && json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ExpiresInDays == 0 {
		req.ExpiresInDays = 7
	}
	if req.ExpiresInDays < 1 || req.ExpiresInDays > 30 {
		writeError(w, http.StatusBadRequest, "invite link expiry must be between 1 and 30 days")
		return
	}
	var targetChannelID string
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE((
			SELECT id::text FROM channels
			 WHERE workspace_id=$1 AND type='channel' AND is_archived=false
			 ORDER BY CASE WHEN name='general' THEN 0 ELSE 1 END, created_at
			 LIMIT 1
		), '')`, workspaceID).Scan(&targetChannelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to find invite destination")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invite link")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	var item WorkspaceInviteLinkResponse
	var expiresAt, createdAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO workspace_invite_links(workspace_id,token_hash,created_by,target_channel_id,expires_at)
		VALUES($1,$2,$3,NULLIF($4,'')::uuid,now()+($5*interval '1 day'))
		RETURNING id::text,role,expires_at,created_at,use_count`, workspaceID, hash[:], adminID, targetChannelID, req.ExpiresInDays).
		Scan(&item.ID, &item.Role, &expiresAt, &createdAt, &item.UseCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invite link")
		return
	}
	item.ExpiresAt = expiresAt.Format(time.RFC3339)
	item.CreatedAt = createdAt.Format(time.RFC3339)
	item.URL = "/invite/" + token
	writeJSON(w, http.StatusCreated, item)
}

func (h *WorkspaceHandler) InviteLinks(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id::text,role,expires_at,revoked_at,use_count,created_at
		  FROM workspace_invite_links
		 WHERE workspace_id=$1 AND revoked_at IS NULL AND expires_at>now()
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invite links")
		return
	}
	defer rows.Close()
	items := make([]WorkspaceInviteLinkResponse, 0)
	for rows.Next() {
		var item WorkspaceInviteLinkResponse
		var expiresAt, createdAt time.Time
		var revokedAt *time.Time
		if rows.Scan(&item.ID, &item.Role, &expiresAt, &revokedAt, &item.UseCount, &createdAt) == nil {
			item.ExpiresAt, item.CreatedAt = expiresAt.Format(time.RFC3339), createdAt.Format(time.RFC3339)
			if revokedAt != nil {
				formatted := revokedAt.Format(time.RFC3339)
				item.RevokedAt = &formatted
			}
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WorkspaceHandler) RevokeInviteLink(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	result, err := h.pool.Exec(r.Context(), `
		UPDATE workspace_invite_links SET revoked_at=COALESCE(revoked_at,now())
		 WHERE id=$1 AND workspace_id=$2 AND revoked_at IS NULL`, chi.URLParam(r, "linkID"), workspaceID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "invite link not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) InviteLinkInfo(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	hash := sha256.Sum256([]byte(token))
	var item WorkspaceInviteLinkInfoResponse
	var expiresAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		SELECT l.workspace_id::text,w.name,w.icon,
		       COALESCE(NULLIF(u.display_name,''),u.email),l.expires_at,
		       COALESCE(l.target_channel_id::text,''),COALESCE(tc.name,''),
		       (SELECT count(*) FROM workspace_members wm WHERE wm.workspace_id=l.workspace_id)
		  FROM workspace_invite_links l
		  JOIN workspaces w ON w.id=l.workspace_id AND w.deleted_at IS NULL
		  JOIN users u ON u.id=l.created_by
		  LEFT JOIN channels tc ON tc.id=l.target_channel_id AND tc.is_archived=false AND tc.type='channel'
		 WHERE l.token_hash=$1 AND l.revoked_at IS NULL AND l.expires_at>now()`, hash[:]).
		Scan(&item.WorkspaceID, &item.WorkspaceName, &item.WorkspaceIcon, &item.InvitedBy, &expiresAt,
			&item.TargetChannelID, &item.TargetChannelName, &item.MemberCount)
	if err != nil {
		writeError(w, http.StatusGone, "invite link is invalid or expired")
		return
	}
	item.ExpiresAt = expiresAt.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, item)
}

func (h *WorkspaceHandler) AcceptInviteLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	hash := sha256.Sum256([]byte(token))
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join Workspace")
		return
	}
	defer tx.Rollback(r.Context())
	var workspaceID, role, workspaceName, channelID, channelName string
	var alreadyMember bool
	err = tx.QueryRow(r.Context(), `
		SELECT l.workspace_id::text,l.role,w.name,
		       COALESCE(l.target_channel_id::text,''),COALESCE(tc.name,''),
		       EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=l.workspace_id AND wm.user_id=$2)
		  FROM workspace_invite_links l
		  JOIN workspaces w ON w.id=l.workspace_id AND w.deleted_at IS NULL
		  LEFT JOIN channels tc ON tc.id=l.target_channel_id AND tc.is_archived=false AND tc.type='channel'
		 WHERE l.token_hash=$1 AND l.revoked_at IS NULL AND l.expires_at>now()
		 FOR UPDATE OF l`, hash[:], userID).Scan(&workspaceID, &role, &workspaceName, &channelID, &channelName, &alreadyMember)
	if err != nil {
		writeError(w, http.StatusGone, "invite link is invalid or expired")
		return
	}
	if channelID == "" {
		_ = tx.QueryRow(r.Context(), `
			SELECT id::text,name FROM channels
			 WHERE workspace_id=$1 AND type='channel' AND is_archived=false
			 ORDER BY CASE WHEN name='general' THEN 0 ELSE 1 END, created_at
			 LIMIT 1`, workspaceID).Scan(&channelID, &channelName)
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO workspace_members(workspace_id,user_id,role)
		VALUES($1,$2,$3) ON CONFLICT (workspace_id,user_id) DO NOTHING`, workspaceID, userID, role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join Workspace")
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO channel_members(channel_id,member_type,member_id,role)
		SELECT id,'user',$2,'member' FROM channels
		 WHERE workspace_id=$1 AND type='channel' AND is_archived=false
		ON CONFLICT DO NOTHING`, workspaceID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join Workspace channels")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE users SET onboarding_completed_at=COALESCE(onboarding_completed_at,now()) WHERE id=$1`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish invited registration")
		return
	}
	if !alreadyMember {
		if _, err = tx.Exec(r.Context(), `
			UPDATE workspace_invite_links SET use_count=use_count+1,last_used_at=now() WHERE token_hash=$1`, hash[:]); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to record invite link use")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join Workspace")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": workspaceID, "workspace_name": workspaceName, "already_member": alreadyMember,
		"channel_id": channelID, "channel_name": channelName,
	})
}

func (h *WorkspaceHandler) JoinRules(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, userID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text,rule_type,value,role,created_at FROM workspace_join_rules WHERE workspace_id=$1 ORDER BY created_at`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list join rules")
		return
	}
	defer rows.Close()
	items := make([]WorkspaceJoinRuleResponse, 0)
	for rows.Next() {
		var item WorkspaceJoinRuleResponse
		var createdAt time.Time
		if rows.Scan(&item.ID, &item.RuleType, &item.Value, &item.Role, &createdAt) == nil {
			item.CreatedAt = createdAt.Format(time.RFC3339)
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WorkspaceHandler) AddJoinRule(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		RuleType string `json:"rule_type"`
		Value    string `json:"value"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.RuleType = strings.ToLower(strings.TrimSpace(req.RuleType))
	req.Value = strings.ToLower(strings.TrimSpace(req.Value))
	if req.RuleType != "email" && req.RuleType != "domain" {
		writeError(w, http.StatusBadRequest, "rule_type must be email or domain")
		return
	}
	if req.RuleType == "domain" {
		req.Value = strings.TrimPrefix(req.Value, "@")
	}
	if req.Value == "" || !strings.Contains(req.Value, ".") {
		writeError(w, http.StatusBadRequest, "a valid email or domain is required")
		return
	}
	var item WorkspaceJoinRuleResponse
	var createdAt time.Time
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO workspace_join_rules (workspace_id,rule_type,value,created_by)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (workspace_id,rule_type,value) DO UPDATE SET value=EXCLUDED.value
		RETURNING id::text,rule_type,value,role,created_at`, workspaceID, req.RuleType, req.Value, adminID).
		Scan(&item.ID, &item.RuleType, &item.Value, &item.Role, &createdAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to add join rule")
		return
	}
	item.CreatedAt = createdAt.Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, item)
}

func (h *WorkspaceHandler) DeleteJoinRule(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.hasRole(r, workspaceID, adminID, "owner", "admin") {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	result, err := h.pool.Exec(r.Context(), `DELETE FROM workspace_join_rules WHERE id=$1 AND workspace_id=$2`, chi.URLParam(r, "ruleID"), workspaceID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "join rule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	targetID := chi.URLParam(r, "userID")
	if !h.hasRole(r, workspaceID, adminID, "owner") {
		writeError(w, http.StatusForbidden, "Workspace owner required")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Role != "admin" && req.Role != "member") {
		writeError(w, http.StatusBadRequest, "role must be member or admin")
		return
	}
	result, err := h.pool.Exec(r.Context(), `UPDATE workspace_members SET role=$3 WHERE workspace_id=$1 AND user_id=$2 AND role<>'owner'`, workspaceID, targetID, req.Role)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusBadRequest, "member cannot be updated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user_id": targetID, "role": req.Role})
}

func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	targetID := chi.URLParam(r, "userID")
	if workspaceID == serverworkspace.PublicID {
		writeError(w, http.StatusBadRequest, "public Workspace members cannot be removed")
		return
	}
	actorRole := h.role(r, workspaceID, adminID)
	if actorRole != "owner" && actorRole != "admin" {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var targetRole string
	if err := h.pool.QueryRow(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, targetID).Scan(&targetRole); err != nil {
		writeError(w, http.StatusNotFound, "Workspace member not found")
		return
	}
	if targetRole == "owner" {
		writeError(w, http.StatusBadRequest, "Workspace owner cannot be removed")
		return
	}
	if actorRole == "admin" && targetRole != "member" {
		writeError(w, http.StatusForbidden, "Workspace admins can remove members only")
		return
	}
	var ownedAgentCount int
	if err := h.pool.QueryRow(r.Context(), `SELECT count(*) FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE c.workspace_id=$1 AND a.owner_id=$2 AND a.is_active=true`, workspaceID, targetID).Scan(&ownedAgentCount); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate member")
		return
	}
	if ownedAgentCount > 0 {
		writeError(w, http.StatusConflict, "member still owns Agents in this Workspace")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `DELETE FROM channel_member_mutes mute USING channels c WHERE mute.channel_id=c.id AND c.workspace_id=$1 AND mute.user_id=$2`, workspaceID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM channel_members cm USING channels c WHERE cm.channel_id=c.id AND c.workspace_id=$1 AND cm.member_type='user' AND cm.member_id=$2`, workspaceID, targetID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2 AND role<>'owner'`, workspaceID, targetID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) hasRole(r *http.Request, workspaceID, userID string, roles ...string) bool {
	role := h.role(r, workspaceID, userID)
	if role == "" {
		return false
	}
	for _, allowed := range roles {
		if role == allowed {
			return true
		}
	}
	return false
}

func (h *WorkspaceHandler) role(r *http.Request, workspaceID, userID string) string {
	var role string
	_ = h.pool.QueryRow(r.Context(), `SELECT wm.role FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id AND w.deleted_at IS NULL WHERE wm.workspace_id=$1 AND wm.user_id=$2`, workspaceID, userID).Scan(&role)
	return role
}
