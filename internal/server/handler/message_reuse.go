package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

type favoriteMessageResponse struct {
	Message       MessageResponse `json:"message"`
	WorkspaceID   string          `json:"workspace_id"`
	WorkspaceName string          `json:"workspace_name"`
	ChannelName   string          `json:"channel_name"`
	FavoritedAt   string          `json:"favorited_at"`
}

type forwardMessageRequest struct {
	TargetChannelID string `json:"target_channel_id"`
}

func (h *MessageHandler) requireChannelMessage(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", "", "", false
	}
	channelID, messageID := chi.URLParam(r, "channelID"), chi.URLParam(r, "messageID")
	if _, err := uuid.Parse(channelID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return "", "", "", false
	}
	if _, err := uuid.Parse(messageID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid message ID")
		return "", "", "", false
	}
	var allowed bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM messages message
			JOIN channels channel ON channel.id = message.channel_id AND channel.type = 'channel'
			JOIN channel_members member ON member.channel_id = channel.id
			 WHERE message.id = $1 AND message.channel_id = $2
			   AND message.thread_id IS NULL AND message.thinking_node_id IS NULL
			   AND COALESCE(message.is_deleted, false) = false
			   AND member.member_type = 'user' AND member.member_id = $3
		)`, messageID, channelID, userID).Scan(&allowed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check message access")
		return "", "", "", false
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "message not found")
		return "", "", "", false
	}
	return userID, channelID, messageID, true
}

func (h *MessageHandler) Favorite(w http.ResponseWriter, r *http.Request) {
	userID, _, messageID, ok := h.requireChannelMessage(w, r)
	if !ok {
		return
	}
	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO message_favorites (user_id, message_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, messageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to favorite message")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MessageHandler) Unfavorite(w http.ResponseWriter, r *http.Request) {
	userID, _, messageID, ok := h.requireChannelMessage(w, r)
	if !ok {
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM message_favorites WHERE user_id = $1 AND message_id = $2`, userID, messageID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MessageHandler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id::text, m.channel_id::text, m.sender_type, m.sender_id::text,
		       CASE WHEN m.sender_type = 'system' THEN 'Solo' WHEN m.sender_type = 'external' THEN COALESCE(m.metadata->>'external_sender_name', '飞书成员') ELSE COALESCE(u.display_name, a.name, 'Unknown') END,
		       CASE WHEN m.sender_type = 'external' THEN COALESCE(m.metadata->>'external_sender_avatar', '') ELSE COALESCE(u.avatar_url, a.avatar_url, '') END, COALESCE(a.is_active, false),
		       m.content, m.content_type, m.metadata, COALESCE(m.attachment_ids, '{}'), m.created_at,
		       w.id::text, w.name, c.name, favorite.created_at
		  FROM message_favorites favorite
		  JOIN messages m ON m.id = favorite.message_id AND COALESCE(m.is_deleted, false) = false
		  JOIN channels c ON c.id = m.channel_id
		  JOIN workspaces w ON w.id = c.workspace_id
		  JOIN channel_members member ON member.channel_id = c.id AND member.member_type = 'user' AND member.member_id = favorite.user_id
		  LEFT JOIN users u ON m.sender_type = 'user' AND u.id = m.sender_id
		  LEFT JOIN agents a ON m.sender_type = 'agent' AND a.id = m.sender_id
		 WHERE favorite.user_id = $1
		 ORDER BY favorite.created_at DESC
		 LIMIT 200`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list favorites")
		return
	}
	defer rows.Close()
	items := []favoriteMessageResponse{}
	for rows.Next() {
		var item favoriteMessageResponse
		var metadata []byte
		var createdAt, favoritedAt time.Time
		if err := rows.Scan(
			&item.Message.ID, &item.Message.ChannelID, &item.Message.SenderType, &item.Message.SenderID,
			&item.Message.SenderName, &item.Message.SenderAvatar, &item.Message.SenderActive,
			&item.Message.Content, &item.Message.ContentType, &metadata, &item.Message.AttachmentIDs, &createdAt,
			&item.WorkspaceID, &item.WorkspaceName, &item.ChannelName, &favoritedAt,
		); err != nil {
			continue
		}
		_ = json.Unmarshal(metadata, &item.Message.Metadata)
		item.Message.CreatedAt = createdAt.Format(time.RFC3339)
		item.FavoritedAt = favoritedAt.Format(time.RFC3339)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list favorites")
		return
	}
	allIDs := []string{}
	for _, item := range items {
		allIDs = append(allIDs, item.Message.AttachmentIDs...)
	}
	if attachmentMap, err := queryAttachmentMap(r.Context(), h.pool, allIDs); err == nil {
		for i := range items {
			for _, id := range items[i].Message.AttachmentIDs {
				if attachment, ok := attachmentMap[id]; ok {
					items[i].Message.Attachments = append(items[i].Message.Attachments, attachment)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *MessageHandler) Forward(w http.ResponseWriter, r *http.Request) {
	userID, sourceChannelID, messageID, ok := h.requireChannelMessage(w, r)
	if !ok {
		return
	}
	var req forwardMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := uuid.Parse(req.TargetChannelID); err != nil || req.TargetChannelID == sourceChannelID {
		writeError(w, http.StatusBadRequest, "invalid target channel")
		return
	}
	var canPost bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM channels channel
			JOIN channel_members member ON member.channel_id = channel.id
			 WHERE channel.id = $1 AND channel.type = 'channel' AND channel.is_archived = false
			   AND channel.workspace_id = (SELECT workspace_id FROM channels WHERE id = $3)
			   AND member.member_type = 'user' AND member.member_id = $2
		)`, req.TargetChannelID, userID, sourceChannelID).Scan(&canPost)
	if err != nil || !canPost {
		writeError(w, http.StatusNotFound, "target channel not found")
		return
	}
	if err := service.CheckHumanChannelPosting(r.Context(), h.pool, req.TargetChannelID, userID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var content, sourceChannelName, sourceSenderID, sourceSenderType, sourceSenderName string
	var attachmentIDs []string
	var sourceCreatedAt time.Time
	err = h.pool.QueryRow(r.Context(), `
		SELECT message.content, COALESCE(message.attachment_ids, '{}'), channel.name,
		       message.sender_id::text, message.sender_type,
		       CASE WHEN message.sender_type = 'system' THEN 'Solo' ELSE COALESCE(u.display_name, a.name, 'Unknown') END,
		       message.created_at
		  FROM messages message
		  JOIN channels channel ON channel.id = message.channel_id
		  LEFT JOIN users u ON message.sender_type = 'user' AND u.id = message.sender_id
		  LEFT JOIN agents a ON message.sender_type = 'agent' AND a.id = message.sender_id
		 WHERE message.id = $1 AND message.channel_id = $2
		   AND COALESCE(message.is_deleted, false) = false`, messageID, sourceChannelID,
	).Scan(&content, &attachmentIDs, &sourceChannelName, &sourceSenderID, &sourceSenderType, &sourceSenderName, &sourceCreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source message")
		return
	}
	metadata := map[string]any{
		"forwarded_message_id": messageID, "forwarded_channel_id": sourceChannelID,
		"forwarded_channel_name": sourceChannelName, "forwarded_sender_id": sourceSenderID,
		"forwarded_sender_type": sourceSenderType, "forwarded_sender_name": sourceSenderName,
		"forwarded_created_at": sourceCreatedAt.Format(time.RFC3339),
	}
	metadataJSON, _ := json.Marshal(metadata)
	newMessageID, now := uuid.NewString(), time.Now().UTC()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO messages
		    (id, channel_id, sender_type, sender_id, content, content_type, mentioned_agent_ids,
		     attachment_ids, metadata, created_at, updated_at)
		VALUES ($1, $2, 'user', $3, $4, 'forwarded', '{}'::uuid[], $5::uuid[], $6::jsonb, $7, $7)`,
		newMessageID, req.TargetChannelID, userID, content, formatUUIDArray(attachmentIDs), metadataJSON, now)
	if err != nil {
		slog.Error("failed to forward message", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to forward message")
		return
	}
	var senderName, senderAvatar string
	_ = h.pool.QueryRow(r.Context(), `SELECT display_name, COALESCE(avatar_url, '') FROM users WHERE id = $1`, userID).Scan(&senderName, &senderAvatar)
	attachments, _ := queryAttachments(r.Context(), h.pool, attachmentIDs)
	response := MessageResponse{
		ID: newMessageID, ChannelID: req.TargetChannelID, SenderType: "user", SenderID: userID,
		SenderName: senderName, SenderAvatar: senderAvatar, Content: content, ContentType: "forwarded",
		Metadata: metadata, AttachmentIDs: attachmentIDs, Attachments: attachments, CreatedAt: now.Format(time.RFC3339),
	}
	if h.hub != nil {
		h.hub.BroadcastToChannel(req.TargetChannelID, ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
			ID: newMessageID, ChannelID: req.TargetChannelID, SenderType: "user", SenderID: userID,
			SenderName: senderName, SenderAvatar: senderAvatar, Content: content, ContentType: "forwarded",
			Metadata: metadata, AttachmentIDs: attachmentIDs, Attachments: toWSAttachmentMeta(attachments), CreatedAt: now.Format(time.RFC3339),
		}))
	}
	writeJSON(w, http.StatusCreated, response)
}

type createThinkingFromMessageRequest struct {
	Title string `json:"title"`
}

func (h *ThinkingHandler) CreateFromMessage(w http.ResponseWriter, r *http.Request) {
	actorID, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	messageID := chi.URLParam(r, "messageID")
	if _, err := uuid.Parse(messageID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid message ID")
		return
	}
	var req createThinkingFromMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	node, err := h.svc.CreateFromMessage(r.Context(), channelID, messageID, req.Title, actorID)
	switch {
	case errors.Is(err, service.ErrThinkingNotFound):
		writeError(w, http.StatusNotFound, "message not found")
	case errors.Is(err, service.ErrThinkingLimit), errors.Is(err, service.ErrThinkingDuplicate):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		if h.hub != nil {
			h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{"channel_id": channelID, "node_id": node.ID}))
		}
		writeJSON(w, http.StatusCreated, node)
	}
}
