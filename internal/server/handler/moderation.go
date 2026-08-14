package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/ws"
)

type ModerationHandler struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

func NewModerationHandler(pool *pgxpool.Pool, hub realtime.Broadcaster) *ModerationHandler {
	return &ModerationHandler{pool: pool, hub: hub}
}

func (h *ModerationHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	var policy, role string
	var muted bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT c.posting_policy, wm.role,
		       EXISTS(SELECT 1 FROM channel_member_mutes mute WHERE mute.channel_id=c.id AND mute.user_id=$2 AND (mute.expires_at IS NULL OR mute.expires_at>now()))
		  FROM channels c
		  JOIN workspace_members wm ON wm.workspace_id=c.workspace_id AND wm.user_id=$2
		  JOIN channel_members cm ON cm.channel_id=c.id AND cm.member_type='user' AND cm.member_id=$2
		 WHERE c.id=$1 AND c.type='channel'`, channelID, userID).Scan(&policy, &role, &muted)
	if err != nil {
		writeError(w, http.StatusNotFound, "Channel not found")
		return
	}
	canManage := role == "owner" || role == "admin"
	writeJSON(w, http.StatusOK, map[string]any{
		"posting_policy": policy, "role": role, "muted": muted,
		"can_manage": canManage,
		"can_post":   !muted && (policy == "everyone" || canManage),
	})
}

func (h *ModerationHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		PostingPolicy string `json:"posting_policy"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.PostingPolicy != "everyone" && req.PostingPolicy != "admins_only") {
		writeError(w, http.StatusBadRequest, "posting_policy must be everyone or admins_only")
		return
	}
	result, err := h.pool.Exec(r.Context(), `
		UPDATE channels c SET posting_policy=$3,updated_at=now()
		 WHERE c.id=$1 AND c.type='channel'
		   AND EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=c.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin'))`,
		chi.URLParam(r, "channelID"), userID, req.PostingPolicy)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	h.broadcast(channelID(r), "policy")
	writeJSON(w, http.StatusOK, map[string]string{"posting_policy": req.PostingPolicy})
}

func (h *ModerationHandler) ListMutes(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok || !h.canManage(r, chi.URLParam(r, "channelID"), userID) {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT mute.user_id::text,u.display_name,u.email,wm.role,COALESCE(mute.reason,''),mute.expires_at,mute.created_at
		  FROM channel_member_mutes mute
		  JOIN users u ON u.id=mute.user_id
		  JOIN channels c ON c.id=mute.channel_id
		  JOIN workspace_members wm ON wm.workspace_id=c.workspace_id AND wm.user_id=mute.user_id
		 WHERE mute.channel_id=$1 AND (mute.expires_at IS NULL OR mute.expires_at>now()) ORDER BY mute.created_at`, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list muted members")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, email, role, reason string
		var expires *time.Time
		var created time.Time
		if rows.Scan(&id, &name, &email, &role, &reason, &expires, &created) == nil {
			items = append(items, map[string]any{"user_id": id, "display_name": name, "email": email, "workspace_role": role, "reason": reason, "expires_at": expires, "created_at": created})
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ModerationHandler) Mute(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	channelID, targetID := chi.URLParam(r, "channelID"), chi.URLParam(r, "userID")
	actorRole, targetRole, allowed := h.moderationRoles(r, channelID, actorID, targetID)
	if !ok || !allowed || targetRole == "owner" || (actorRole == "admin" && targetRole != "member") {
		writeError(w, http.StatusForbidden, "cannot mute this Workspace member")
		return
	}
	var req struct {
		Reason    string     `json:"reason"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO channel_member_mutes(channel_id,user_id,muted_by,reason,expires_at)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(channel_id,user_id) DO UPDATE SET muted_by=EXCLUDED.muted_by,reason=EXCLUDED.reason,expires_at=EXCLUDED.expires_at,created_at=now()`,
		channelID, targetID, actorID, strings.TrimSpace(req.Reason), req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to mute member")
		return
	}
	h.broadcast(channelID, "mute")
	w.WriteHeader(http.StatusNoContent)
}

func (h *ModerationHandler) Unmute(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	channelID, targetID := chi.URLParam(r, "channelID"), chi.URLParam(r, "userID")
	actorRole, targetRole, allowed := h.moderationRoles(r, channelID, actorID, targetID)
	if !ok || !allowed || targetRole == "owner" || (actorRole == "admin" && targetRole != "member") {
		writeError(w, http.StatusForbidden, "cannot unmute this Workspace member")
		return
	}
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM channel_member_mutes WHERE channel_id=$1 AND user_id=$2`, channelID, targetID)
	h.broadcast(channelID, "mute")
	w.WriteHeader(http.StatusNoContent)
}

func (h *ModerationHandler) ListPins(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok || !h.isMember(r, chi.URLParam(r, "channelID"), userID) {
		writeError(w, http.StatusForbidden, "Channel membership required")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id::text,m.content,m.sender_type,COALESCE(u.display_name,a.name,'Solo'),m.created_at,p.pinned_at
		  FROM channel_message_pins p JOIN messages m ON m.id=p.message_id
		  LEFT JOIN users u ON m.sender_type='user' AND u.id::text=m.sender_id::text
		  LEFT JOIN agents a ON m.sender_type='agent' AND a.id::text=m.sender_id::text
		 WHERE p.channel_id=$1 AND COALESCE(m.is_deleted,false)=false
		 ORDER BY p.pinned_at DESC`, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pinned messages")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, content, senderType, senderName string
		var createdAt, pinnedAt time.Time
		if rows.Scan(&id, &content, &senderType, &senderName, &createdAt, &pinnedAt) == nil {
			items = append(items, map[string]any{"message_id": id, "content": content, "sender_type": senderType, "sender_name": senderName, "created_at": createdAt, "pinned_at": pinnedAt})
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ModerationHandler) Pin(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	channelID := chi.URLParam(r, "channelID")
	if !ok || !h.canManage(r, channelID, actorID) {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	result, err := h.pool.Exec(r.Context(), `
		INSERT INTO channel_message_pins(channel_id,message_id,pinned_by)
		SELECT $1,m.id,$3 FROM messages m
		 WHERE m.id=$2 AND m.channel_id=$1 AND COALESCE(m.is_deleted,false)=false
		ON CONFLICT(channel_id,message_id) DO UPDATE SET pinned_by=channel_message_pins.pinned_by`, channelID, chi.URLParam(r, "messageID"), actorID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	h.broadcast(channelID, "pin")
	w.WriteHeader(http.StatusNoContent)
}

func (h *ModerationHandler) Unpin(w http.ResponseWriter, r *http.Request) {
	actorID, ok := requireUserID(r)
	channelID := chi.URLParam(r, "channelID")
	if !ok || !h.canManage(r, channelID, actorID) {
		writeError(w, http.StatusForbidden, "Workspace admin required")
		return
	}
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM channel_message_pins WHERE channel_id=$1 AND message_id=$2`, channelID, chi.URLParam(r, "messageID"))
	h.broadcast(channelID, "pin")
	w.WriteHeader(http.StatusNoContent)
}

func channelID(r *http.Request) string { return chi.URLParam(r, "channelID") }

func (h *ModerationHandler) broadcast(channelID, change string) {
	if h.hub != nil {
		h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventChannelModerationUpdated, map[string]string{
			"channel_id": channelID,
			"change":     change,
		}))
	}
}

func (h *ModerationHandler) canManage(r *http.Request, channelID, userID string) bool {
	var allowed bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM channels c JOIN workspace_members wm ON wm.workspace_id=c.workspace_id WHERE c.id=$1 AND c.type='channel' AND wm.user_id=$2 AND wm.role IN ('owner','admin'))`, channelID, userID).Scan(&allowed)
	return allowed
}

func (h *ModerationHandler) isMember(r *http.Request, channelID, userID string) bool {
	var allowed bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM channel_members WHERE channel_id=$1 AND member_type='user' AND member_id=$2)`, channelID, userID).Scan(&allowed)
	return allowed
}

func (h *ModerationHandler) moderationRoles(r *http.Request, channelID, actorID, targetID string) (string, string, bool) {
	var actorRole, targetRole string
	err := h.pool.QueryRow(r.Context(), `
		SELECT actor.role,target.role FROM channels c
		JOIN workspace_members actor ON actor.workspace_id=c.workspace_id AND actor.user_id=$2
		JOIN workspace_members target ON target.workspace_id=c.workspace_id AND target.user_id=$3
		JOIN channel_members cm ON cm.channel_id=c.id AND cm.member_type='user' AND cm.member_id=$3
		WHERE c.id=$1 AND c.type='channel'`, channelID, actorID, targetID).Scan(&actorRole, &targetRole)
	return actorRole, targetRole, err == nil && (actorRole == "owner" || actorRole == "admin")
}
