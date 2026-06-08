package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/i18n"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

// DMHandler handles DM-related HTTP requests.
type DMHandler struct {
	pool       *pgxpool.Pool
	hub        *ws.Hub
	agentSvc   *service.AgentService
	mentionSvc *service.MentionService
	taskSvc    *service.TaskService
}

// NewDMHandler creates a new DMHandler.
func NewDMHandler(pool *pgxpool.Pool, hub *ws.Hub, agentSvc *service.AgentService, taskSvc *service.TaskService) *DMHandler {
	return &DMHandler{
		pool:       pool,
		hub:        hub,
		agentSvc:   agentSvc,
		mentionSvc: service.NewMentionService(pool),
		taskSvc:    taskSvc,
	}
}

// --- Request/Response types ---

type CreateDMRequest struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
}

type DMChannelResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	Type            string  `json:"type"`
	CreatedBy       string  `json:"created_by"`
	OtherMemberType string  `json:"other_member_type"`
	OtherMemberID   string  `json:"other_member_id"`
	OtherMemberName   string `json:"other_member_name"`
	OtherMemberActive bool   `json:"other_member_active"`
	IsArchived        bool   `json:"is_archived"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	LastMessage     string  `json:"last_message,omitempty"`
	LastMessageAt   *string `json:"last_message_at,omitempty"`
}

type DMListResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	OtherMemberType  string  `json:"other_member_type"`
	OtherMemberID    string  `json:"other_member_id"`
	OtherMemberName  string  `json:"other_member_name"`
	OtherMemberActive bool   `json:"other_member_active"`
	LastMessage      string  `json:"last_message,omitempty"`
	LastMessageAt    *string `json:"last_message_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// CreateOrGetDM handles POST /api/v1/dm
// Creates a new DM channel or returns an existing one.
// Supports user<->user and user<->agent.
// Bidirectional dedup: A->B and B->A are treated as the same DM.
func (h *DMHandler) CreateOrGetDM(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.MemberType != "user" && req.MemberType != "agent" {
		writeError(w, http.StatusBadRequest, "member_type must be 'user' or 'agent'")
		return
	}
	if req.MemberID == "" {
		writeError(w, http.StatusBadRequest, "member_id is required")
		return
	}

	// Prevent DM with self
	if req.MemberType == "user" && req.MemberID == userID {
		writeError(w, http.StatusBadRequest, "cannot create DM with yourself")
		return
	}

	// Verify target member exists
	var targetName string
	switch req.MemberType {
	case "user":
		err := h.pool.QueryRow(r.Context(),
			`SELECT display_name FROM users WHERE id = $1 AND is_active = true`,
			req.MemberID,
		).Scan(&targetName)
		if err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			slog.Error("failed to find user", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

	case "agent":
		// Verify agent exists and belongs to the requesting user
		var agentName, ownerID string
		err := h.pool.QueryRow(r.Context(),
			`SELECT name, owner_id FROM agents WHERE id = $1 AND is_active = true`,
			req.MemberID,
		).Scan(&agentName, &ownerID)
		if err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "agent not found")
				return
			}
			slog.Error("failed to find agent", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		targetName = agentName

		// Verify the agent belongs to the requesting user
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "you can only DM your own agents")
			return
		}
	}

	// Check for existing DM channel (bidirectional dedup)
	var existingChannelID string
	var existingCreatedAt, existingUpdatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT dm1.channel_id, c.created_at, c.updated_at
		 FROM dm_members dm1
		 JOIN dm_members dm2 ON dm1.channel_id = dm2.channel_id
		 JOIN channels c ON c.id = dm1.channel_id AND c.type = 'dm' AND c.is_archived = false
		 WHERE dm1.member_type = 'user' AND dm1.member_id = $1
		   AND dm2.member_type = $2 AND dm2.member_id = $3
		 LIMIT 1`,
		userID, req.MemberType, req.MemberID,
	).Scan(&existingChannelID, &existingCreatedAt, &existingUpdatedAt)
	if err == nil && existingChannelID != "" {
		// Existing DM found — return it with other member info
		slog.Info("existing DM found", "dm_id", existingChannelID, "user_id", userID, "target_id", req.MemberID)

		writeJSON(w, http.StatusOK, DMChannelResponse{
			ID:              existingChannelID,
			Name:            targetName,
			Type:            "dm",
			CreatedBy:       userID,
			OtherMemberType: req.MemberType,
			OtherMemberID:   req.MemberID,
			OtherMemberName: targetName,
			OtherMemberActive: true, // target was verified active above
			IsArchived:      false,
			CreatedAt:       existingCreatedAt.Format(time.RFC3339),
			UpdatedAt:       existingUpdatedAt.Format(time.RFC3339),
		})
		return
	}

	// Create new DM channel in a transaction
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}
	defer tx.Rollback(r.Context())

	var channelID string
	var createdAt, updatedAt time.Time

	// Use target name as channel name for display
	channelName := targetName
	if channelName == "" {
		channelName = "DM"
	}

	err = tx.QueryRow(r.Context(),
		`INSERT INTO channels (name, description, type, created_by)
		 VALUES ($1, $2, 'dm', $3)
		 RETURNING id, created_at, updated_at`,
		channelName, "Direct Message", userID,
	).Scan(&channelID, &createdAt, &updatedAt)
	if err != nil {
		slog.Error("failed to create DM channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}

	// Add requester as owner member in channel_members
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'user', $2, 'owner')`,
		channelID, userID,
	)
	if err != nil {
		slog.Error("failed to add requester to channel_members", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}

	// Add target as member in channel_members
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, $2, $3, 'member')`,
		channelID, req.MemberType, req.MemberID,
	)
	if err != nil {
		slog.Error("failed to add target to channel_members", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}

	// Add both parties to dm_members
	_, err = tx.Exec(r.Context(),
		`INSERT INTO dm_members (channel_id, member_type, member_id)
		 VALUES ($1, 'user', $2), ($1, $3, $4)`,
		channelID, userID, req.MemberType, req.MemberID,
	)
	if err != nil {
		slog.Error("failed to add dm_members", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("failed to commit DM creation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create DM")
		return
	}

	slog.Info("DM created",
		"dm_id", channelID,
		"user_id", userID,
		"target_type", req.MemberType,
		"target_id", req.MemberID,
	)

	writeJSON(w, http.StatusCreated, DMChannelResponse{
		ID:              channelID,
		Name:            targetName,
		Type:            "dm",
		CreatedBy:       userID,
		OtherMemberType: req.MemberType,
		OtherMemberID:   req.MemberID,
		OtherMemberName: targetName,
		OtherMemberActive: true, // target was verified active above
		IsArchived:      false,
		CreatedAt:       createdAt.Format(time.RFC3339),
		UpdatedAt:       updatedAt.Format(time.RFC3339),
	})
}

// ListDMs handles GET /api/v1/dm
// Returns the user's DM channels sorted by last message time (descending).
// Each DM includes the other party's info and last message preview (truncated to 50 chars).
func (h *DMHandler) ListDMs(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Query DM channels where the user is a participant, ordered by last message time
	rows, err := h.pool.Query(r.Context(),
		`SELECT c.id, c.name,
		        dm_other.member_type AS other_type, dm_other.member_id AS other_id,
		        COALESCE(u.display_name, a.name, '') AS other_name,
		        COALESCE(a.is_active, true) AS other_active,
		        COALESCE(msg.content, '') AS last_content,
		        msg.created_at AS last_at,
		        c.created_at
		 FROM channels c
		 INNER JOIN dm_members dm_self ON dm_self.channel_id = c.id
		     AND dm_self.member_type = 'user' AND dm_self.member_id = $1
		 INNER JOIN dm_members dm_other ON dm_other.channel_id = c.id
		     AND NOT (dm_other.member_type = 'user' AND dm_other.member_id = $1)
		 LEFT JOIN users u ON dm_other.member_type = 'user' AND dm_other.member_id = u.id
		 LEFT JOIN agents a ON dm_other.member_type = 'agent' AND dm_other.member_id = a.id
		 LEFT JOIN LATERAL (
		     SELECT content, created_at FROM messages
		     WHERE channel_id = c.id AND thread_id IS NULL
		     ORDER BY created_at DESC LIMIT 1
		 ) msg ON true
		 WHERE c.type = 'dm' AND c.is_archived = false
		 ORDER BY COALESCE(msg.created_at, c.created_at) DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query DM list", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list DMs")
		return
	}
	defer rows.Close()

	dms := make([]DMListResponse, 0)
	for rows.Next() {
		var d DMListResponse
		var lastContent string
		var lastAt *time.Time
		var createdAt time.Time

		err := rows.Scan(&d.ID, &d.Name,
			&d.OtherMemberType, &d.OtherMemberID, &d.OtherMemberName,
			&d.OtherMemberActive,
			&lastContent, &lastAt, &createdAt)
		if err != nil {
			slog.Error("failed to scan DM row", "error", err)
			continue
		}

		d.CreatedAt = createdAt.Format(time.RFC3339)

		// Truncate last message to 50 chars for preview
		if lastContent != "" {
			preview := truncateString(lastContent, 50)
			d.LastMessage = preview
			if lastAt != nil {
				lastAtStr := lastAt.Format(time.RFC3339)
				d.LastMessageAt = &lastAtStr
			}
		}

		dms = append(dms, d)
	}

	writeJSON(w, http.StatusOK, dms)
}

// GetDM handles GET /api/v1/dm/{dmID}
// Returns details of a specific DM channel.
func (h *DMHandler) GetDM(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dmID := chi.URLParam(r, "dmID")
	if dmID == "" {
		writeError(w, http.StatusBadRequest, "DM ID is required")
		return
	}

	// Check user is a DM participant
	var (
		channelID    string
		channelName  string
		description  string
		createdBy    string
		isArchived   bool
		createdAt, updatedAt time.Time
		otherType    string
		otherID      string
		otherName    string
		otherActive  bool
	)
	err := h.pool.QueryRow(r.Context(),
		`SELECT c.id, c.name, COALESCE(c.description, ''), c.created_by, c.is_archived,
		        c.created_at, c.updated_at,
		        dm_other.member_type, dm_other.member_id,
		        COALESCE(u.display_name, a.name, '') AS other_name,
		        COALESCE(a.is_active, true) AS other_active
		 FROM channels c
		 INNER JOIN dm_members dm_self ON dm_self.channel_id = c.id
		     AND dm_self.member_type = 'user' AND dm_self.member_id = $1
		 INNER JOIN dm_members dm_other ON dm_other.channel_id = c.id
		     AND NOT (dm_other.member_type = 'user' AND dm_other.member_id = $1)
		 LEFT JOIN users u ON dm_other.member_type = 'user' AND dm_other.member_id = u.id
		 LEFT JOIN agents a ON dm_other.member_type = 'agent' AND dm_other.member_id = a.id
		 WHERE c.id = $2 AND c.type = 'dm' AND c.is_archived = false`,
		userID, dmID,
	).Scan(&channelID, &channelName, &description, &createdBy, &isArchived,
		&createdAt, &updatedAt,
		&otherType, &otherID, &otherName, &otherActive)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "DM not found")
			return
		}
		slog.Error("failed to query DM", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, DMChannelResponse{
		ID:              channelID,
		Name:            channelName,
		Description:     description,
		Type:            "dm",
		CreatedBy:       createdBy,
		OtherMemberType: otherType,
		OtherMemberID:   otherID,
		OtherMemberName: otherName,
		OtherMemberActive: otherActive,
		IsArchived:      isArchived,
		CreatedAt:       createdAt.Format(time.RFC3339),
		UpdatedAt:       updatedAt.Format(time.RFC3339),
	})
}

// ListMessages handles GET /api/v1/dm/{dmID}/messages
// Returns DM message history with cursor-based pagination (reuses channel message logic).
func (h *DMHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dmID := chi.URLParam(r, "dmID")
	if dmID == "" {
		writeError(w, http.StatusBadRequest, "DM ID is required")
		return
	}

	// Parse and validate query params
	limit := defaultMessageLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxMessageLimit {
			limit = parsed
		}
	}

	before := r.URL.Query().Get("before")
	if before != "" {
		if _, err := uuid.Parse(before); err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor: must be a valid message ID")
			return
		}
	}

	// Check user is a DM participant
	var isParticipant bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM dm_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, dmID, userID,
	).Scan(&isParticipant)
	if err != nil {
		slog.Error("failed to check DM participation", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isParticipant {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Check DM is not archived
	var isArchived bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_archived FROM channels WHERE id = $1`, dmID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		writeError(w, http.StatusGone, "DM channel is archived")
		return
	}

	// Build message query with cursor pagination (same pattern as MessageHandler.List)
	query := `SELECT m.id, m.channel_id, m.sender_type, m.sender_id,
	                 CASE WHEN m.sender_type = 'system' THEN 'Solo' ELSE COALESCE(u.display_name, a.name, m.sender_id::text) END as sender_name,
	                 COALESCE(a.is_active, false) AS sender_active,
	                 m.content, m.content_type, COALESCE(m.attachment_ids, '{}') as attachment_ids,
                 COALESCE(t.task_number, 0) AS task_number,
                 COALESCE(t.status, '') AS task_status,
                 COALESCE(u_claimer.display_name, a_claimer.name, '') as task_claimer_name,
                 COALESCE(th.reply_count, 0) AS reply_count,
                 m.created_at
	          FROM messages m
	          LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
	          LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
	          LEFT JOIN tasks t ON t.message_id = m.id
	          LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id
	          LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id
		          LEFT JOIN threads th ON m.id = th.root_message_id
	          WHERE m.channel_id = $1 AND m.thread_id IS NULL`

	args := []any{dmID}

	if before != "" {
		query += ` AND (m.created_at, m.id) < (SELECT c.created_at, c.id FROM messages c WHERE c.id = $2)`
		args = append(args, before)
	}

	query += ` ORDER BY m.created_at DESC, m.id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("failed to query DM messages", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	defer rows.Close()

	messages := make([]MessageResponse, 0, limit)
	for rows.Next() {
		var msg MessageResponse
		var createdAt time.Time
		err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.SenderType, &msg.SenderID,
			&msg.SenderName, &msg.SenderActive, &msg.Content, &msg.ContentType, &msg.AttachmentIDs,
&msg.TaskNumber, &msg.TaskStatus, &msg.TaskClaimerName, &msg.ReplyCount, &createdAt)
		if err != nil {
			slog.Error("failed to scan message row", "error", err)
			continue
		}
		msg.CreatedAt = createdAt.Format(time.RFC3339)
		messages = append(messages, msg)
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// Batch query attachment metadata for DM messages
	if len(messages) > 0 {
		allIDs := collectAttachmentIDs(messages)
		if len(allIDs) > 0 {
			metaMap, err := queryAttachmentMap(r.Context(), h.pool, allIDs)
			if err == nil {
				for i := range messages {
					if len(messages[i].AttachmentIDs) > 0 {
						atts := make([]AttachmentMeta, 0, len(messages[i].AttachmentIDs))
						for _, id := range messages[i].AttachmentIDs {
							if m, ok := metaMap[id]; ok {
								atts = append(atts, m)
							}
						}
						messages[i].Attachments = atts
					}
				}
			}
		}
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	writeJSON(w, http.StatusOK, MessageListResponse{
		Messages: messages,
		HasMore:  hasMore,
	})
}

// SendMessage handles POST /api/v1/dm/{dmID}/messages
// Sends a message in a DM channel and triggers agent auto-response if applicable.
func (h *DMHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dmID := chi.URLParam(r, "dmID")
	if dmID == "" {
		writeError(w, http.StatusBadRequest, "DM ID is required")
		return
	}

	var req CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "message content is required")
		return
	}
	if len(content) > 10000 {
		writeError(w, http.StatusBadRequest, "message content exceeds maximum length of 10000 characters")
		return
	}

	// Verify user is a DM participant
	var isParticipant bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM dm_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, dmID, userID,
	).Scan(&isParticipant)
	if err != nil {
		slog.Error("failed to check DM participation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	if !isParticipant {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Verify channel is a DM and not archived
	var channelType string
	var isArchived bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT type, is_archived FROM channels WHERE id = $1`,
		dmID,
	).Scan(&channelType, &isArchived)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "DM not found")
			return
		}
		slog.Error("failed to check channel type", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	if channelType != "dm" {
		writeError(w, http.StatusBadRequest, "not a DM channel")
		return
	}
	if isArchived {
		writeError(w, http.StatusGone, "DM channel is archived")
		return
	}

	// Validate attachment ownership
	attachmentIDs := req.AttachmentIDs
	if len(attachmentIDs) > 0 {
		var ownedCount int
		err := h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM attachments WHERE id = ANY($1::uuid[]) AND user_id = $2`,
			formatUUIDArray(attachmentIDs), userID,
		).Scan(&ownedCount)
		if err != nil || ownedCount != len(attachmentIDs) {
			writeError(w, http.StatusBadRequest, "one or more attachment IDs are invalid")
			return
		}
	}

	// Determine sender type (agent vs user) so the message stores the correct type.
	var isAgent bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, userID,
	).Scan(&isAgent)
	senderType := "user"
	if isAgent {
		senderType = "agent"
	}

	// Persist message
	now := time.Now()
	messageID := uuid.New().String()

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, attachment_ids, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6::uuid[], $7, $7)`,
		messageID, dmID, senderType, userID, content, formatUUIDArray(attachmentIDs), now,
	)
	if err != nil {
		slog.Error("failed to persist DM message", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	// Get sender name (resolved from both users and agents tables)
	var displayName string
	err = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT display_name FROM users WHERE id = $1),
			(SELECT name FROM agents WHERE id = $1),
			'Unknown'
		)`, userID,
	).Scan(&displayName)
	if err != nil {
		displayName = "Unknown"
	}

	slog.Info("DM message sent",
		"message_id", messageID,
		"dm_id", dmID,
		"user_id", userID,
	)

	// If as_task, convert to task and return task response (align with channel behavior)
	if req.AsTask && h.taskSvc != nil {
		// Broadcast message.new first so other DM participants see the message
		if h.hub != nil {
			msgPayload := ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
				ID:          messageID,
				ChannelID:   dmID,
				SenderType:  senderType,
				SenderID:    userID,
				SenderName:  displayName,
				Content:     content,
				ContentType: "text",
				CreatedAt:   now.Format(time.RFC3339),
			})
			h.hub.BroadcastToChannel(dmID, msgPayload)
		}

		task, convertErr := h.taskSvc.ConvertMessageToTask(r.Context(), dmID, messageID, userID)
		if convertErr != nil {
			slog.Error("failed to convert DM message to task", "error", convertErr, "message_id", messageID)
			writeError(w, http.StatusInternalServerError, "failed to create task")
			return
		}
		taskResp := toTaskResponse(task)
		ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
			ID:               task.ID,
			TaskNumber:       task.TaskNumber,
			ChannelID:        task.ChannelID,
			CreatorID:        task.CreatorID,
			CreatorName:      task.CreatorName,
			Title:            task.Title,
			Description:      task.Description,
			Status:           task.Status,
			ClaimerID:        task.ClaimerID,
			ClaimerName:      task.ClaimerName,
			Priority:         task.Priority,
			MessageID:        task.MessageID,
			CreatedAt:        taskResp.CreatedAt,
			UpdatedAt:        taskResp.UpdatedAt,
			SubtaskCount:     task.SubtaskCount,
			DoneSubtaskCount: task.DoneSubtaskCount,
		})
		threadSvc := service.NewThreadService(h.pool)
		_, _, _ = threadSvc.GetOrCreateThread(r.Context(), dmID, task.MessageID)
		if h.agentSvc != nil {
			go h.agentSvc.TriggerAllAgentsForTask(context.Background(), dmID, task.ID, task.TaskNumber, task.Title, nil, nil)
		}
		writeJSON(w, http.StatusCreated, TaskResponse{
			ID:          taskResp.ID,
			TaskNumber:  taskResp.TaskNumber,
			ChannelID:   taskResp.ChannelID,
			CreatorID:   taskResp.CreatorID,
			CreatorName: taskResp.CreatorName,
			Title:       taskResp.Title,
			Description: taskResp.Description,
			Status:      taskResp.Status,
			ClaimerID:   taskResp.ClaimerID,
			ClaimerName: taskResp.ClaimerName,
			Priority:    taskResp.Priority,
			DueDate:     taskResp.DueDate,
			MessageID:   taskResp.MessageID,
			CreatedAt:   taskResp.CreatedAt,
			UpdatedAt:   taskResp.UpdatedAt,
		})
		return
	}

	// Fetch attachment metadata for the response
	var attachments []AttachmentMeta
	if len(attachmentIDs) > 0 {
		attachments, _ = queryAttachments(r.Context(), h.pool, attachmentIDs)
	}

	// Broadcast message.new to DM channel subscribers
	msgPayload := ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
		ID:          messageID,
		ChannelID:   dmID,
		SenderType:  senderType,
		SenderID:    userID,
		SenderName:  displayName,
		Content:     content,
		ContentType: "text",
		CreatedAt:   now.Format(time.RFC3339),
	})
	h.hub.BroadcastToChannel(dmID, msgPayload)

	// Also broadcast dm.message.new for DM-specific event listeners
	dmMsgPayload := ws.Envelope(ws.EventDMMessageNew, ws.DMMessageNewPayload{
		DMID:          dmID,
		ID:            messageID,
		ChannelID:     dmID,
		SenderType:    senderType,
		SenderID:      userID,
		SenderName:    displayName,
		Content:       content,
		ContentType:   "text",
		AttachmentIDs: attachmentIDs,
		CreatedAt:     now.Format(time.RFC3339),
	})
	h.hub.BroadcastToChannel(dmID, dmMsgPayload)

	// Broadcast inbox.updated to other DM participants (v1.5).
	if h.hub != nil {
		go h.notifyInboxForDMParticipants(context.Background(), dmID, userID)
	}

		// Resolve user @mentions and broadcast inbox.updated to mentioned users (v1.5).
		if h.mentionSvc != nil && h.hub != nil {
			go func() {
				mentionedUsers, err := h.mentionSvc.ResolveUserMentions(context.Background(), content, messageID)
				if err != nil {
					slog.Warn("failed to resolve user mentions in DM", "error", err)
					return
				}
				for _, uid := range mentionedUsers {
					ws.BroadcastInboxUpdated(h.hub, uid)
				}
			}()
		}

	// Trigger agent auto-response for DM (SOLO-58-B)
	// In DM, the agent responds to all messages (no @mention needed)
	if h.agentSvc != nil {
		go h.agentSvc.TriggerAgentResponse(
			context.Background(),
			dmID,
			messageID,
			"user",
			userID,
			nil,  // no @mentions in DM
			false, // hasMentions = false
			nil,   // agentChain: fresh trigger from human
		)
	}

	writeJSON(w, http.StatusCreated, MessageResponse{
		ID:            messageID,
		ChannelID:     dmID,
		SenderType:    senderType,
		SenderID:      userID,
		SenderName:    displayName,
		Content:       content,
		ContentType:   "text",
		AttachmentIDs: attachmentIDs,
		Attachments:   attachments,
		CreatedAt:     now.Format(time.RFC3339),
	})
}

// UpdateMessage handles PATCH /api/v1/dm/{dmID}/messages/{messageID}
// Edits a DM message. Only the original sender can edit. Sets is_edited = true.
func (h *DMHandler) UpdateMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dmID := chi.URLParam(r, "dmID")
	messageID := chi.URLParam(r, "messageID")
	if dmID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and message ID are required")
		return
	}

	var req UpdateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "message content is required")
		return
	}
	if len(content) > 10000 {
		writeError(w, http.StatusBadRequest, "message content exceeds maximum length of 10000 characters")
		return
	}

	// Verify user is a DM participant
	var isParticipant bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM dm_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, dmID, userID,
	).Scan(&isParticipant)
	if err != nil {
		slog.Error("failed to check DM participation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to edit message")
		return
	}
	if !isParticipant {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Verify channel is a DM and not archived
	var channelType string
	var isArchived bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT type, is_archived FROM channels WHERE id = $1`,
		dmID,
	).Scan(&channelType, &isArchived)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "DM not found")
			return
		}
		slog.Error("failed to check channel type", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to edit message")
		return
	}
	if channelType != "dm" {
		writeError(w, http.StatusBadRequest, "not a DM channel")
		return
	}
	if isArchived {
		writeError(w, http.StatusGone, "DM channel is archived")
		return
	}

	// Verify message ownership and state
	var currentSenderType, currentSenderID string
	var isDeleted bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT sender_type, sender_id, COALESCE(is_deleted, false) FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, dmID,
	).Scan(&currentSenderType, &currentSenderID, &isDeleted)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		slog.Error("failed to query DM message for edit", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to edit message")
		return
	}

	if isDeleted {
		writeError(w, http.StatusGone, "message has been deleted")
		return
	}
	if currentSenderType != "user" || currentSenderID != userID {
		writeError(w, http.StatusForbidden, "you can only edit your own messages")
		return
	}

	// Update the message
	now := time.Now()
	_, err = h.pool.Exec(r.Context(),
		`UPDATE messages SET content = $1, is_edited = true, updated_at = $2 WHERE id = $3`,
		content, now, messageID,
	)
	if err != nil {
		slog.Error("failed to update DM message", "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to edit message")
		return
	}

	slog.Info("DM message updated", "message_id", messageID, "dm_id", dmID, "user_id", userID)

	// Broadcast message.updated event
	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageUpdated, ws.MessageUpdatedPayload{
			ID:          messageID,
			ChannelID:   dmID,
			SenderType:  "user",
			SenderID:    userID,
			Content:     content,
			ContentType: "text",
			IsEdited:    true,
			UpdatedAt:   now.Format(time.RFC3339),
		})
		h.hub.BroadcastToChannel(dmID, msgPayload)
	}

	writeJSON(w, http.StatusOK, MessageResponse{
		ID:          messageID,
		ChannelID:   dmID,
		SenderType:  "user",
		SenderID:    userID,
		Content:     content,
		ContentType: "text",
		CreatedAt:   now.Format(time.RFC3339),
	})
}

// DeleteMessage handles DELETE /api/v1/dm/{dmID}/messages/{messageID}
// Soft-deletes a DM message. Only the original sender can delete.
func (h *DMHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dmID := chi.URLParam(r, "dmID")
	messageID := chi.URLParam(r, "messageID")
	if dmID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and message ID are required")
		return
	}

	// Verify user is a DM participant
	var isParticipant bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM dm_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, dmID, userID,
	).Scan(&isParticipant)
	if err != nil {
		slog.Error("failed to check DM participation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete message")
		return
	}
	if !isParticipant {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Verify message ownership and state
	var currentSenderType, currentSenderID string
	var isDeleted bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT sender_type, sender_id, COALESCE(is_deleted, false) FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, dmID,
	).Scan(&currentSenderType, &currentSenderID, &isDeleted)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		slog.Error("failed to query DM message for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete message")
		return
	}

	if isDeleted {
		writeError(w, http.StatusGone, "message has already been deleted")
		return
	}
	if currentSenderType != "user" || currentSenderID != userID {
		writeError(w, http.StatusForbidden, "you can only delete your own messages")
		return
	}

	// Soft delete
	now := time.Now()
	_, err = h.pool.Exec(r.Context(),
		`UPDATE messages SET is_deleted = true, updated_at = $1 WHERE id = $2`,
		now, messageID,
	)
	if err != nil {
		slog.Error("failed to soft-delete DM message", "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete message")
		return
	}

	slog.Info("DM message deleted", "message_id", messageID, "dm_id", dmID, "user_id", userID)

	// Broadcast message.deleted event
	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageDeleted, ws.MessageDeletedPayload{
			ID:        messageID,
			ChannelID: dmID,
		})
		h.hub.BroadcastToChannel(dmID, msgPayload)
	}

	w.WriteHeader(http.StatusNoContent)
}

// ConvertMessageToTask handles POST /api/v1/dm/{dmID}/messages/{messageID}/convert-to-task
// Creates a task from a DM message (asTask).
func (h *DMHandler) ConvertMessageToTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	messageID := chi.URLParam(r, "messageID")
	if dmID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and message ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	task, err := h.taskSvc.ConvertMessageToTask(r.Context(), dmID, messageID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		default:
			slog.Error("failed to convert DM message to task", "error", err, "message_id", messageID)
			if err.Error() == "message not found" {
				writeError(w, http.StatusNotFound, "message not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to convert message to task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusCreated, resp)

	// Broadcast task.created event
	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
		ID:               task.ID,
		TaskNumber:       task.TaskNumber,
		ChannelID:        task.ChannelID,
		CreatorID:        task.CreatorID,
		CreatorName:      task.CreatorName,
		Title:            task.Title,
		Description:      task.Description,
		Status:           task.Status,
		ClaimerID:        task.ClaimerID,
		ClaimerName:      task.ClaimerName,
		Priority:         task.Priority,
		DueDate:          dueDate,
		MessageID:        task.MessageID,
		CreatedAt:        resp.CreatedAt,
		UpdatedAt:        resp.UpdatedAt,
		SubtaskCount:     task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	// Broadcast system message
	// Broadcast message.updated so the original message gets task badge fields
	if task.MessageID != "" {
	}

	// Trigger DM agent participant for auto-claim
	if h.agentSvc != nil {
		go h.agentSvc.TriggerAllAgentsForTask(context.Background(), dmID, task.ID, task.TaskNumber, task.Title, nil, nil)
	}
	}

// --- DM Task handlers (SOLO v1.2 Phase 2) ---

// CreateTask handles POST /api/v1/dm/{dmID}/tasks
func (h *DMHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	if dmID == "" {
		writeError(w, http.StatusBadRequest, "DM ID is required")
		return
	}

	// Validate request body early (before DB calls).
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	if len(req.Title) > 500 {
		writeError(w, http.StatusBadRequest, "task title exceeds maximum length of 500 characters")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Insert a user message first (align with channel CreateGlobal)
	now := time.Now()
	msgID := uuid.New().String()

	senderType := "user"
	var isAgent bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, userID).Scan(&isAgent)
	if isAgent {
		senderType = "agent"
	}

	senderName := userID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT display_name FROM users WHERE id = $1),
			(SELECT name FROM agents WHERE id = $1),
			$1
		)`,
		userID,
	).Scan(&senderName); err != nil {
		slog.Warn("failed to resolve sender name for DM task message",
			"user_id", userID,
			"error", err,
		)
	}

	_, msgErr := h.pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 'text', $6, $6)`,
		msgID, dmID, senderType, userID, req.Title, now,
	)
	if msgErr != nil {
		slog.Error("failed to insert DM task user message", "error", msgErr)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
			ID:          msgID,
			ChannelID:   dmID,
			SenderType:  senderType,
			SenderID:    userID,
			SenderName:  senderName,
			Content:     req.Title,
			ContentType: "text",
			CreatedAt:   now.Format(time.RFC3339),
		})
		h.hub.BroadcastToChannel(dmID, msgPayload)
	}

	task, err := h.taskSvc.ConvertMessageToTask(r.Context(), dmID, msgID, userID)
	if err != nil {
		slog.Error("failed to convert DM message to task", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}


	// Create thread for the task message (align with channel task.go Create)
	threadSvc := service.NewThreadService(h.pool)
	_, _, threadErr := threadSvc.GetOrCreateThread(r.Context(), dmID, task.MessageID)
	if threadErr != nil {
		slog.Error("failed to create thread for DM task", "task_id", task.ID, "error", threadErr)
	}

	taskResp := toTaskResponse(task)
	ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
		ID:               task.ID,
		TaskNumber:       task.TaskNumber,
		ChannelID:        task.ChannelID,
		CreatorID:        task.CreatorID,
		CreatorName:      task.CreatorName,
		Title:            task.Title,
		Description:      task.Description,
		Status:           task.Status,
		ClaimerID:        task.ClaimerID,
		ClaimerName:      task.ClaimerName,
		Priority:         task.Priority,
		MessageID:        task.MessageID,
		CreatedAt:        taskResp.CreatedAt,
		UpdatedAt:        taskResp.UpdatedAt,
		SubtaskCount:     task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	// Trigger DM agent participant for auto-claim
	if h.agentSvc != nil {
		go h.agentSvc.TriggerAllAgentsForTask(context.Background(), dmID, task.ID, task.TaskNumber, task.Title, nil, nil)
	}

	writeJSON(w, http.StatusCreated, TaskResponse{
		ID:          taskResp.ID,
		TaskNumber:  taskResp.TaskNumber,
		ChannelID:   taskResp.ChannelID,
		CreatorID:   taskResp.CreatorID,
		CreatorName: taskResp.CreatorName,
		Title:       taskResp.Title,
		Description: taskResp.Description,
		Status:      taskResp.Status,
		ClaimerID:   taskResp.ClaimerID,
		ClaimerName: taskResp.ClaimerName,
		Priority:    taskResp.Priority,
		DueDate:     taskResp.DueDate,
		MessageID:   taskResp.MessageID,
		CreatedAt:   taskResp.CreatedAt,
		UpdatedAt:   taskResp.UpdatedAt,
	})
	return
}

// ListTasks handles GET /api/v1/dm/{dmID}/tasks
func (h *DMHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	if dmID == "" {
		writeError(w, http.StatusBadRequest, "DM ID is required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	filter := service.TaskFilter{
		Status:    r.URL.Query().Get("status"),
		ClaimerID: r.URL.Query().Get("claimer_id"),
	}

	tasks, err := h.taskSvc.ListTasks(r.Context(), dmID, userID, filter)
	if err != nil {
		slog.Error("failed to list DM tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponseList(tasks))
}

// GetTask handles GET /api/v1/dm/{dmID}/tasks/{taskID}
func (h *DMHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	taskID := chi.URLParam(r, "taskID")
	if dmID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and task ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	task, err := h.taskSvc.GetTask(r.Context(), dmID, taskID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		default:
			slog.Error("failed to get DM task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get task")
		}
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

// UpdateTask handles PATCH /api/v1/dm/{dmID}/tasks/{taskID}
func (h *DMHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	taskID := chi.URLParam(r, "taskID")
	if dmID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and task ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svcReq := service.TaskUpdateRequest{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
	}

	task, err := h.taskSvc.UpdateTask(r.Context(), dmID, taskID, userID, svcReq)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		case err == service.ErrTaskInvalidStatus || err == service.ErrTaskInvalidTransition:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("failed to update DM task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	// Broadcast task.updated event
	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:               task.ID,
		TaskNumber:       task.TaskNumber,
		ChannelID:        task.ChannelID,
		Title:            task.Title,
		Description:      task.Description,
		Status:           task.Status,
		ClaimerID:        task.ClaimerID,
		ClaimerName:      task.ClaimerName,
		Priority:         task.Priority,
		DueDate:          dueDateStr,
		MessageID:        task.MessageID,
		UpdatedAt:        task.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:     task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

		// Broadcast message.updated for task badge (align with channel behavior)
}

// ClaimTask handles POST /api/v1/dm/{dmID}/tasks/{taskID}/claim
func (h *DMHandler) ClaimTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	taskID := chi.URLParam(r, "taskID")
	if dmID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and task ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Resolve task_number to UUID
	t, err := h.taskSvc.GetTask(r.Context(), dmID, taskID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	task, err := h.taskSvc.ClaimTask(r.Context(), dmID, t.ID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		case err == service.ErrTaskAlreadyClaimed:
			writeError(w, http.StatusConflict, "task is already claimed")
		case err == service.ErrTaskInTerminalState:
			writeError(w, http.StatusConflict, "task is in a terminal state")
		case err == service.ErrTaskNotClaimable:
			writeError(w, http.StatusConflict, "task status does not allow claiming")
		default:
			slog.Error("failed to claim DM task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to claim task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	// Broadcast task.updated event
	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:               task.ID,
		TaskNumber:       task.TaskNumber,
		ChannelID:        task.ChannelID,
		Title:            task.Title,
		Description:      task.Description,
		Status:           task.Status,
		ClaimerID:        task.ClaimerID,
		ClaimerName:      task.ClaimerName,
		Priority:         task.Priority,
		DueDate:          dueDateStr,
		MessageID:        task.MessageID,
		UpdatedAt:        task.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:     task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	// Broadcast message.updated for task badge

	// Persist claim system message to the task's thread.
	if task.MessageID != "" {
		threadSvc := service.NewThreadService(h.pool)
		threadID, _, tErr := threadSvc.GetOrCreateThread(r.Context(), dmID, task.MessageID)
		if tErr == nil {
			claimMsgID := uuid.New().String()
			claimNow := time.Now()
			claimContent := fmt.Sprintf("📋 <@%s> claimed #%d %s", userID, task.TaskNumber, task.Title)
			_, _ = h.pool.Exec(r.Context(),
				`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, thread_id, created_at, updated_at)
				 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $5, $5)`,
				claimMsgID, dmID, claimContent, threadID, claimNow,
			)
			// Read reply_count for thread.message.new (align with task.go Claim)
			var replyCount int
			h.pool.QueryRow(r.Context(),
				`SELECT reply_count FROM threads WHERE id = $1`, threadID,
			).Scan(&replyCount)
			// Broadcast to thread subscribers so ThreadPanel updates in real-time
			threadMsgPayload := ws.ThreadMessageNewPayload{
				Message: ws.ThreadMessageItem{
					ID:          claimMsgID,
					ChannelID:   dmID,
					ThreadID:    threadID,
					SenderType:  "system",
					SenderID:    "system",
					SenderName:  "Solo",
					Content:     claimContent,
					ContentType: "system",
					CreatedAt:   claimNow.UTC().Format(time.RFC3339),
				},
				Thread: ws.ThreadMetadataItem{
					ThreadID:    threadID,
					ReplyCount:  replyCount,
					LastReplyAt: claimNow.UTC().Format(time.RFC3339),
				},
			}
			h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsgPayload))
					}
	}
}

// UnclaimTask handles DELETE /api/v1/dm/{dmID}/tasks/{taskID}/claim
func (h *DMHandler) UnclaimTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	taskID := chi.URLParam(r, "taskID")
	if dmID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and task ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Resolve task_number to UUID
	t, err := h.taskSvc.GetTask(r.Context(), dmID, taskID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	task, err := h.taskSvc.UnclaimTask(r.Context(), dmID, t.ID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		case err == service.ErrTaskNotClaimer:
			writeError(w, http.StatusForbidden, "you are not the claimer of this task")
		case err == service.ErrTaskInTerminalState:
			writeError(w, http.StatusConflict, "task is in a terminal state")
		default:
			slog.Error("failed to unclaim DM task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to unclaim task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	// Broadcast task.updated event
	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:               task.ID,
		TaskNumber:       task.TaskNumber,
		ChannelID:        task.ChannelID,
		Title:            task.Title,
		Description:      task.Description,
		Status:           task.Status,
		ClaimerID:        "",
		ClaimerName:      task.ClaimerName,
		Priority:         task.Priority,
		DueDate:          dueDateStr,
		MessageID:        task.MessageID,
		UpdatedAt:        task.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:     task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	// Broadcast message.updated for task badge (align with task.go Unclaim)

	// Persist unclaim system message to the task's thread (align with ClaimTask).
	if task.MessageID != "" {
		threadSvc := service.NewThreadService(h.pool)
		threadID, _, tErr := threadSvc.GetOrCreateThread(r.Context(), dmID, task.MessageID)
		if tErr == nil {
			unclaimMsgID := uuid.New().String()
			unclaimNow := time.Now()
			unclaimContent := fmt.Sprintf("📋 <@%s> released #%d %s", userID, task.TaskNumber, task.Title)
			_, _ = h.pool.Exec(r.Context(),
				`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, thread_id, created_at, updated_at)
				 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $5, $5)`,
				unclaimMsgID, dmID, unclaimContent, threadID, unclaimNow,
			)
			var replyCount int
			h.pool.QueryRow(r.Context(),
				`SELECT reply_count FROM threads WHERE id = $1`, threadID,
			).Scan(&replyCount)
			// Broadcast to thread subscribers
			threadMsgPayload := ws.ThreadMessageNewPayload{
				Message: ws.ThreadMessageItem{
					ID:          unclaimMsgID,
					ChannelID:   dmID,
					ThreadID:    threadID,
					SenderType:  "system",
					SenderID:    "system",
					SenderName:  "Solo",
					Content:     unclaimContent,
					ContentType: "system",
					CreatedAt:   unclaimNow.UTC().Format(time.RFC3339),
				},
				Thread: ws.ThreadMetadataItem{
					ThreadID:    threadID,
					ReplyCount:  replyCount,
					LastReplyAt: unclaimNow.UTC().Format(time.RFC3339),
				},
			}
			h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsgPayload))
					}
	}
}

// --- DM task helpers ---

// isDMParticipant checks if the user is a participant of the given DM channel.
func (h *DMHandler) isDMParticipant(ctx context.Context, dmID, userID string) bool {
	var exists bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM dm_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, dmID, userID,
	).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

// notifyInboxForDMParticipants sends inbox.updated to all DM participants except the sender.
func (h *DMHandler) notifyInboxForDMParticipants(ctx context.Context, dmID, senderID string) {
	if h.hub == nil || h.pool == nil {
		return
	}
	rows, err := h.pool.Query(ctx,
		`SELECT member_id FROM dm_members
		 WHERE channel_id = $1 AND member_type = 'user' AND member_id != $2`,
		dmID, senderID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		ws.BroadcastInboxUpdated(h.hub, userID)
	}
}

// broadcastDMTaskSystemMessage sends a system message to the DM channel via WebSocket.
func (h *DMHandler) broadcastDMTaskSystemMessage(dmID string, taskNumber int, title, action string) {
	if h.hub == nil {
		return
	}
	msg := ws.MessageNewPayload{
		ID:          uuid.New().String(),
		ChannelID:   dmID,
		SenderType:  "system",
		SenderID:    "system",
		SenderName:  "Solo",
		Content:     fmt.Sprintf("📋 Task #%d %s: %s", taskNumber, action, title),
		ContentType: "system",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	h.hub.BroadcastToChannel(dmID, ws.Envelope(ws.EventMessageNew, msg))
}

// DeleteTask handles DELETE /api/v1/dm/{dmID}/tasks/{taskID}
func (h *DMHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dmID := chi.URLParam(r, "dmID")
	taskID := chi.URLParam(r, "taskID")
	if dmID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "DM ID and task ID are required")
		return
	}

	if !h.isDMParticipant(r.Context(), dmID, userID) {
		writeError(w, http.StatusNotFound, "DM not found")
		return
	}

	// Get task before deleting for system message and broadcast
	task, getErr := h.taskSvc.GetTask(r.Context(), dmID, taskID, userID)
	if getErr != nil {
		switch {
		case getErr == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case getErr == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		default:
			slog.Error("failed to get DM task for delete", "error", getErr)
			writeError(w, http.StatusInternalServerError, "failed to delete task")
		}
		return
	}

	if err := h.taskSvc.DeleteTask(r.Context(), dmID, taskID, userID); err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a DM participant")
		default:
			slog.Error("failed to delete DM task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete task")
		}
		return
	}

	// Broadcast system message
	h.broadcastDMTaskSystemMessage(dmID, task.TaskNumber, task.Title, i18n.Active.SysTaskDeleted)

	// Broadcast task.deleted event
	ws.BroadcastTaskDeleted(h.hub, ws.TaskDeletedPayload{
		ID:         taskID,
		ChannelID:  dmID,
		TaskNumber: task.TaskNumber,
	})

	w.WriteHeader(http.StatusNoContent)
}

// truncateString truncates a string to the given length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
