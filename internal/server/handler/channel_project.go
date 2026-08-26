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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/internal/realtime"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

type ChannelProjectMapping struct {
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name"`
	ComputerID     string `json:"computer_id,omitempty"`
	ComputerName   string `json:"computer_name,omitempty"`
	LocalPath      string `json:"local_path,omitempty"`
	Version        string `json:"version,omitempty"`
	AccessMode     string `json:"access_mode,omitempty"`
	ComputerStatus string `json:"computer_status,omitempty"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason,omitempty"`
}

type ChannelProjectResponse struct {
	ChannelID       string                  `json:"channel_id"`
	Name            string                  `json:"name"`
	Source          string                  `json:"source,omitempty"`
	BaselineVersion string                  `json:"baseline_version,omitempty"`
	CanManage       bool                    `json:"can_manage"`
	Mappings        []ChannelProjectMapping `json:"mappings"`
}

func isAbsoluteProjectPath(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) ||
		(len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/'))
}

func (h *ChannelHandler) projectChangeBlocked(ctx context.Context, channelID string) (bool, error) {
	var blocked bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM agent_runs
		 WHERE channel_id=$1 AND status IN ('queued','thinking','running','streaming','waiting_input','waiting_approval')
	)`, channelID).Scan(&blocked)
	return blocked, err
}

func (h *ChannelHandler) recordProjectChange(ctx context.Context, channelID, userID, event, source, baseline string) {
	actorName := "A member"
	_ = h.pool.QueryRow(ctx, `SELECT display_name FROM users WHERE id=$1`, userID).Scan(&actorName)
	metadata := map[string]any{
		"event": event, "actor_name": actorName, "source": source, "baseline_version": baseline,
	}
	encoded, _ := json.Marshal(metadata)
	content := actorName + " changed the project used by this Channel. Future Agent work will use the new project."
	if event == "channel.project.unlinked" || event == "channel.project.folder_unlinked" {
		content = actorName + " disconnected the project used by this Channel. Future Agent work will use the Agent's private workspace."
	}
	messageID, createdAt := uuid.NewString(), time.Now().UTC()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO messages (id,channel_id,sender_type,sender_id,content,content_type,metadata,created_at,updated_at)
		VALUES ($1,$2,'system','00000000-0000-0000-0000-000000000000',$3,'system',$4::jsonb,$5,$5)`,
		messageID, channelID, content, encoded, createdAt); err != nil {
		slog.Error("failed to record channel project change", "channel_id", channelID, "error", err)
		return
	}
	if h.hub != nil {
		h.hub.BroadcastToChannel(channelID, realtime.Envelope("message.new", map[string]any{
			"id": messageID, "channel_id": channelID, "sender_type": "system", "sender_id": "system",
			"sender_name": "Solo", "content": content, "content_type": "system", "metadata": metadata,
			"created_at": createdAt.Format(time.RFC3339),
		}))
	}
}

func (h *ChannelHandler) rejectBlockedProjectChange(w http.ResponseWriter, r *http.Request, channelID string) bool {
	blocked, err := h.projectChangeBlocked(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check current Agent work")
		return true
	}
	if blocked {
		writeErrorCode(w, http.StatusConflict, "CHANNEL_PROJECT_BUSY", "finish the current Agent work before changing this project")
		return true
	}
	return false
}

func (h *ChannelHandler) channelProjectRole(r *http.Request, channelID, userID string) (string, bool) {
	var role string
	err := h.pool.QueryRow(r.Context(), `
		SELECT CASE
		         WHEN cm.role IN ('owner','admin') OR wm.role IN ('owner','admin') THEN 'admin'
		         ELSE 'member'
		       END
		  FROM channels c
		  JOIN channel_members cm ON cm.channel_id=c.id AND cm.member_type='user' AND cm.member_id=$2
		  JOIN workspace_members wm ON wm.workspace_id=c.workspace_id AND wm.user_id=$2
		 WHERE c.id=$1 AND c.workspace_id=$3 AND c.type='channel' AND c.is_archived=false`,
		channelID, userID, serverworkspace.ID(r),
	).Scan(&role)
	return role, err == nil
}

func (h *ChannelHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	role, ok := h.channelProjectRole(r, channelID, userID)
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	project := ChannelProjectResponse{ChannelID: channelID, CanManage: role == "owner" || role == "admin", Mappings: []ChannelProjectMapping{}}
	if err := h.pool.QueryRow(r.Context(), `SELECT name, project_source, project_baseline FROM channels WHERE id=$1`, channelID).
		Scan(&project.Name, &project.Source, &project.BaselineVersion); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT u.id::text, u.display_name,
		       COALESCE(m.computer_id::text,''), COALESCE(c.name,''),
		       CASE WHEN u.id=$2 THEN COALESCE(m.local_path,'') ELSE '' END,
		       COALESCE(m.version,''), COALESCE(m.access_mode,''), COALESCE(c.status,'')
		  FROM channel_members cm
		  JOIN users u ON u.id=cm.member_id
		  LEFT JOIN channel_project_mappings m ON m.channel_id=cm.channel_id AND m.user_id=u.id
		  LEFT JOIN computers c ON c.id=m.computer_id
		 WHERE cm.channel_id=$1 AND cm.member_type='user'
		 ORDER BY u.display_name, c.name`, channelID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project mappings")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var mapping ChannelProjectMapping
		if err := rows.Scan(&mapping.UserID, &mapping.UserName, &mapping.ComputerID, &mapping.ComputerName, &mapping.LocalPath, &mapping.Version, &mapping.AccessMode, &mapping.ComputerStatus); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load project mappings")
			return
		}
		switch {
		case mapping.ComputerID == "":
			mapping.Reason = "missing"
		case mapping.ComputerStatus != "online":
			mapping.Reason = "computer_offline"
		case mapping.AccessMode != "read_write":
			mapping.Reason = "read_only"
		case project.BaselineVersion != "" && mapping.Version != project.BaselineVersion:
			mapping.Reason = "version_mismatch"
		default:
			mapping.Available = true
		}
		project.Mappings = append(project.Mappings, mapping)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project mappings")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *ChannelHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	role, ok := h.channelProjectRole(r, channelID, userID)
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	var req struct {
		Source          string `json:"source"`
		BaselineVersion string `json:"baseline_version"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Source = strings.TrimSpace(req.Source)
	req.BaselineVersion = strings.TrimSpace(req.BaselineVersion)
	if len(req.Source) > 2000 || len(req.BaselineVersion) > 200 {
		writeError(w, http.StatusBadRequest, "project source or version is too long")
		return
	}
	if h.rejectBlockedProjectChange(w, r, channelID) {
		return
	}
	var previousSource, previousBaseline string
	if err := h.pool.QueryRow(r.Context(), `SELECT project_source, project_baseline FROM channels WHERE id=$1`, channelID).Scan(&previousSource, &previousBaseline); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}
	if previousSource == req.Source && previousBaseline == req.BaselineVersion {
		h.GetProject(w, r)
		return
	}
	if _, err := h.pool.Exec(r.Context(), `UPDATE channels SET project_source=$1, project_baseline=$2, updated_at=now() WHERE id=$3`, req.Source, req.BaselineVersion, channelID); err != nil {
		slog.Error("failed to update channel project", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}
	event := "channel.project.changed"
	if req.Source == "" && req.BaselineVersion == "" {
		event = "channel.project.unlinked"
	}
	h.recordProjectChange(r.Context(), channelID, userID, event, req.Source, req.BaselineVersion)
	h.GetProject(w, r)
}

func (h *ChannelHandler) PutProjectMapping(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID, computerID := chi.URLParam(r, "channelID"), chi.URLParam(r, "computerID")
	if _, ok := h.channelProjectRole(r, channelID, userID); !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var req struct {
		LocalPath  string `json:"local_path"`
		Version    string `json:"version"`
		AccessMode string `json:"access_mode"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.LocalPath, req.Version, req.AccessMode = strings.TrimSpace(req.LocalPath), strings.TrimSpace(req.Version), strings.TrimSpace(req.AccessMode)
	if req.AccessMode == "" {
		req.AccessMode = "read_write"
	}
	if !isAbsoluteProjectPath(req.LocalPath) || len(req.LocalPath) > 2048 || len(req.Version) > 200 || (req.AccessMode != "read_only" && req.AccessMode != "read_write") {
		writeError(w, http.StatusBadRequest, "invalid project mapping")
		return
	}
	var canUse bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM computers c LEFT JOIN computer_members cm ON cm.computer_id=c.id AND cm.user_id=$2 WHERE c.id=$1 AND (c.owner_id=$2 OR cm.user_id IS NOT NULL))`, computerID, userID).Scan(&canUse); err != nil || !canUse {
		writeError(w, http.StatusBadRequest, "computer is not available to this user")
		return
	}
	if h.rejectBlockedProjectChange(w, r, channelID) {
		return
	}
	var previousPath, previousVersion, previousAccess string
	previousErr := h.pool.QueryRow(r.Context(), `SELECT local_path,version,access_mode FROM channel_project_mappings WHERE channel_id=$1 AND user_id=$2 AND computer_id=$3`, channelID, userID, computerID).
		Scan(&previousPath, &previousVersion, &previousAccess)
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load project mapping")
		return
	}
	if previousErr == nil && previousPath == req.LocalPath && previousVersion == req.Version && previousAccess == req.AccessMode {
		h.GetProject(w, r)
		return
	}
	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO channel_project_mappings (channel_id,user_id,computer_id,local_path,version,access_mode)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (channel_id,user_id,computer_id) DO UPDATE
		SET local_path=EXCLUDED.local_path, version=EXCLUDED.version, access_mode=EXCLUDED.access_mode, updated_at=now()`,
		channelID, userID, computerID, req.LocalPath, req.Version, req.AccessMode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save project mapping")
		return
	}
	h.recordProjectChange(r.Context(), channelID, userID, "channel.project.folder_changed", "", req.Version)
	h.GetProject(w, r)
}

func (h *ChannelHandler) DeleteProjectMapping(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID, computerID := chi.URLParam(r, "channelID"), chi.URLParam(r, "computerID")
	if _, ok := h.channelProjectRole(r, channelID, userID); !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if h.rejectBlockedProjectChange(w, r, channelID) {
		return
	}
	result, err := h.pool.Exec(r.Context(), `DELETE FROM channel_project_mappings WHERE channel_id=$1 AND user_id=$2 AND computer_id=$3`, channelID, userID, computerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove project mapping")
		return
	}
	if result.RowsAffected() > 0 {
		h.recordProjectChange(r.Context(), channelID, userID, "channel.project.folder_unlinked", "", "")
	}
	h.GetProject(w, r)
}
