package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

// Maximum number of thread messages in a single page.
const maxThreadMessageLimit = 100

// defaultThreadMessageLimit is the default page size for thread messages.
const defaultThreadMessageLimit = 50

// ThreadHandler handles thread-related HTTP requests.
type ThreadHandler struct {
	pool       *pgxpool.Pool
	mentionSvc *service.MentionService
	agentSvc   *service.AgentService
	hub        *ws.Hub
}

// NewThreadHandler creates a new ThreadHandler.
func NewThreadHandler(pool *pgxpool.Pool, hub *ws.Hub, agentSvc *service.AgentService) *ThreadHandler {
	mentionSvc := service.NewMentionService(pool)
	return &ThreadHandler{pool: pool, hub: hub, agentSvc: agentSvc, mentionSvc: mentionSvc}
}

// --- Request/Response types ---

type ThreadReplyRequest struct {
	Content           string   `json:"content"`
	MentionedAgentIDs []string `json:"mentioned_agent_ids,omitempty"`
	AttachmentIDs     []string `json:"attachment_ids,omitempty"`
}

type ThreadResponse struct {
	ID            string `json:"id"`
	ChannelID     string `json:"channel_id"`
	RootMessageID string `json:"root_message_id"`
	ReplyCount    int    `json:"reply_count"`
	LastReplyAt   string `json:"last_reply_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type ThreadReplyResponse struct {
	ID            string           `json:"id"`
	ChannelID     string           `json:"channel_id"`
	ThreadID      string           `json:"thread_id"`
	SenderType    string           `json:"sender_type"`
	SenderID      string           `json:"sender_id"`
	SenderName    string           `json:"sender_name,omitempty"`
	SenderActive  bool             `json:"sender_active"`
	Content       string           `json:"content"`
	ContentType   string           `json:"content_type"`
	AttachmentIDs []string         `json:"attachment_ids,omitempty"`
	Attachments   []AttachmentMeta `json:"attachments,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

type ThreadMessageListResponse struct {
	Messages []ThreadReplyResponse `json:"messages"`
	HasMore  bool                  `json:"has_more"`
}

// CreateThreadReply handles POST /api/v1/channels/{channelID}/messages/{messageID}/thread
// It creates a new reply in the thread rooted at the given message.
// The thread record is auto-created if it does not exist.
func (h *ThreadHandler) CreateThreadReply(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	if channelID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and message ID are required")
		return
	}

	var req ThreadReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "reply content is required")
		return
	}
	if len(content) > 10000 {
		writeError(w, http.StatusBadRequest, "reply content exceeds maximum length of 10000 characters")
		return
	}

	// Verify user is a member of the channel
	var isMember bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
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

	// Verify the root message exists in this channel
	var msgExists bool
	var msgThreadID *string
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2),
		        thread_id FROM messages WHERE id = $1`, messageID, channelID,
	).Scan(&msgExists, &msgThreadID)
	if err != nil {
		slog.Error("failed to check message existence", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !msgExists {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	// Nested threads are not supported: reject replies to messages already in a thread.
	if msgThreadID != nil && *msgThreadID != "" {
		writeError(w, http.StatusBadRequest, "cannot reply to a message that is already in a thread")
		return
	}

	// Check channel is not archived
	var isArchived bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_archived FROM channels WHERE id = $1`, channelID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		writeError(w, http.StatusGone, "channel is archived")
		return
	}

	// Execute in a transaction: find or create thread, then insert reply
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(r.Context())

	// Try to find existing thread for this root message
	var threadID string
	err = tx.QueryRow(r.Context(),
		`SELECT id FROM threads WHERE root_message_id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&threadID)
	if err != nil {
		// Thread doesn't exist yet — create one
		if isNotFound(err) {
			now := time.Now()
			err = tx.QueryRow(r.Context(),
				`INSERT INTO threads (channel_id, root_message_id, last_reply_at)
				 VALUES ($1, $2, $3)
					 ON CONFLICT (root_message_id) DO UPDATE SET root_message_id = EXCLUDED.root_message_id
				 RETURNING id`,
				channelID, messageID, now,
			).Scan(&threadID)
			if err != nil {
				slog.Error("failed to create thread", "error", err)
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		} else {
			slog.Error("failed to find thread", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Resolve @mentions for agent triggering
	mentionedAgentIDs := req.MentionedAgentIDs
	var mentionHasMentions bool
	if len(mentionedAgentIDs) == 0 && h.mentionSvc != nil {
		resolved, hasRefs, err := h.mentionSvc.ResolveMentions(r.Context(), content, channelID)
		if err != nil {
			slog.Warn("failed to resolve mentions in thread reply", "error", err)
		} else {
			mentionedAgentIDs = resolved
			mentionHasMentions = hasRefs
		}
	}

	// Validate attachment ownership (before INSERT since tx is for message only)
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

	// Insert reply message
	now := time.Now()
	replyID := uuid.New().String()

	_, err = tx.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, mentioned_agent_ids, attachment_ids, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::uuid[], $8::uuid[], $9, $9)`,
		replyID, channelID, senderType, userID, content, threadID, formatUUIDArray(mentionedAgentIDs), formatUUIDArray(attachmentIDs), now,
	)
	if err != nil {
		slog.Error("failed to persist thread reply", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Update thread reply count and last_reply_at
	_, err = tx.Exec(r.Context(),
		`UPDATE threads SET reply_count = reply_count + 1, last_reply_at = $1
		 WHERE id = $2`,
		now, threadID,
	)
	if err != nil {
		slog.Error("failed to update thread reply count", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
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

	slog.Info("thread reply created",
		"reply_id", replyID,
		"thread_id", threadID,
		"channel_id", channelID,
		"root_message_id", messageID,
		"user_id", userID,
		"mentions", mentionedAgentIDs,
	)

	// Fetch attachment metadata for the response
	var attachments []AttachmentMeta
	if len(attachmentIDs) > 0 {
		attachments, _ = queryAttachments(r.Context(), h.pool, attachmentIDs)
	}

	// Trigger agent auto-response for @mentioned agents in thread
	if h.agentSvc != nil {
		// hasMentions is true if the content contained @patterns, even if none resolved.
		// This allows TriggerAgentResponseInThread to correctly fall back to auto-response
		// when @mentions were intended but failed to resolve.
		hasMentions := mentionHasMentions || len(mentionedAgentIDs) > 0
		go h.agentSvc.TriggerAgentResponseInThread(context.Background(), channelID, threadID, senderType, userID, mentionedAgentIDs, hasMentions, nil)
	}

	writeJSON(w, http.StatusCreated, ThreadReplyResponse{
		ID:            replyID,
		ChannelID:     channelID,
		ThreadID:      threadID,
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

// ListThreadMessages handles GET /api/v1/channels/{channelID}/messages/{messageID}/thread
// It returns the messages in the thread rooted at the given message, with cursor-based pagination.
func (h *ThreadHandler) ListThreadMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	if channelID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and message ID are required")
		return
	}

	// Parse and validate query params
	limit := defaultThreadMessageLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxThreadMessageLimit {
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

	// Verify user is a member of the channel and channel is not archived
	var isMember bool
	var isArchived bool

	// Guard against nil pool (tests may construct handler without DB).
	if h.pool == nil {
		writeError(w, http.StatusBadRequest, "database not available")
		return
	}

	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
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

	err = h.pool.QueryRow(r.Context(),
		`SELECT is_archived FROM channels WHERE id = $1`, channelID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		writeError(w, http.StatusGone, "channel is archived")
		return
	}

	// Resolve short message ID to full UUID if needed
	resolvedMsgID := messageID
	if _, err := uuid.Parse(messageID); err != nil {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT id FROM messages WHERE id::text LIKE $1 AND channel_id = $2 ORDER BY created_at DESC LIMIT 1`,
			messageID+"%", channelID,
		).Scan(&resolvedMsgID)
	}

	// Find the thread by root message ID
	var threadID string
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM threads WHERE root_message_id = $1 AND channel_id = $2`,
		resolvedMsgID, channelID,
	).Scan(&threadID)
	if err != nil {
		if isNotFound(err) {
			// No thread yet — return empty list
			writeJSON(w, http.StatusOK, ThreadMessageListResponse{
				Messages: []ThreadReplyResponse{},
				HasMore:  false,
			})
			return
		}
		slog.Error("failed to find thread", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build query for thread messages with cursor pagination
	// Messages are ordered DESC by created_at, id for pagination,
	// then reversed to ASC for natural reading order.
	query := `SELECT m.id, m.channel_id, m.thread_id, m.sender_type, m.sender_id,
	                 COALESCE(u.display_name, a.name, '') as sender_name,
	                 COALESCE(a.is_active, false) AS sender_active,
	                 m.content, m.content_type, COALESCE(m.attachment_ids, '{}') as attachment_ids,
                 m.created_at
	          FROM messages m
	          LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
	          LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
	          WHERE m.thread_id = $1`

	args := []any{threadID}
	argIdx := 2

	if before != "" {
		query += ` AND (m.created_at, m.id) < (SELECT c.created_at, c.id FROM messages c WHERE c.id = $2)`
		args = append(args, before)
		argIdx = 3
	}

	query += ` ORDER BY m.created_at DESC, m.id DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit+1)

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("failed to query thread messages", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list thread messages")
		return
	}
	defer rows.Close()

	messages := make([]ThreadReplyResponse, 0, limit)
	for rows.Next() {
		var msg ThreadReplyResponse
		var createdAt time.Time
		err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.ThreadID,
			&msg.SenderType, &msg.SenderID, &msg.SenderName,
			&msg.SenderActive,
			&msg.Content, &msg.ContentType, &msg.AttachmentIDs, &createdAt)
		if err != nil {
			slog.Error("failed to scan thread message row", "error", err)
			continue
		}
		msg.CreatedAt = createdAt.Format(time.RFC3339)
		messages = append(messages, msg)
	}

	// Batch query attachment metadata for thread messages
	if len(messages) > 0 {
		allIDs := collectThreadAttachmentIDs(messages)
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

	// Determine has_more and trim to limit
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// Reverse to ASC for natural reading order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	writeJSON(w, http.StatusOK, ThreadMessageListResponse{
		Messages: messages,
		HasMore:  hasMore,
	})
}

// MarkThreadRead handles POST /api/v1/threads/{threadID}/mark-read
// It records that the current user has read the thread up to now.
func (h *ThreadHandler) MarkThreadRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	threadID := chi.URLParam(r, "threadID")
	if threadID == "" {
		writeError(w, http.StatusBadRequest, "thread ID is required")
		return
	}

	// Verify thread exists and get channel_id for membership check
	var channelID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT channel_id FROM threads WHERE id = $1`,
		threadID,
	).Scan(&channelID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
		slog.Error("failed to find thread", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verify user is a member of the channel the thread belongs to
	var isMember bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("failed to check membership", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "not a member of this channel")
		return
	}

	// Upsert last_read_at
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO thread_reads (user_id, thread_id, last_read_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (user_id, thread_id) DO UPDATE SET last_read_at = now()`,
		userID, threadID,
	)
	if err != nil {
		slog.Error("failed to mark thread as read", "error", err, "thread_id", threadID, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UnfollowThread handles POST /api/v1/threads/unfollow.
// An agent calls this to stop receiving ordinary message delivery for a thread.
func (h *ThreadHandler) UnfollowThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Target == "" {
		writeError(w, http.StatusBadRequest, "target is required (e.g. '#general:abc123')")
		return
	}

	// Parse target: "#channel:shortid" or "#channel"
	parts := strings.SplitN(req.Target, ":", 2)
	channelRef := parts[0]
	threadSuffix := ""
	if len(parts) == 2 {
		threadSuffix = parts[1]
	}

	// Resolve the thread from the short ID suffix.
	var threadID string
	var channelID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT t.id, t.channel_id FROM threads t
		 JOIN messages m ON m.id = t.parent_message_id
		 WHERE m.channel_id = (SELECT id FROM channels WHERE name = $1 OR id = $1)
		 AND m.id::text LIKE $2 || '%'
		 LIMIT 1`,
		strings.TrimPrefix(channelRef, "#"), threadSuffix+"%",
	).Scan(&threadID, &channelID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
		slog.Error("failed to find thread", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Mark as unfollowed — delete any thread_reads record for this user.
	_, err = h.pool.Exec(r.Context(),
		`DELETE FROM thread_reads WHERE user_id = $1 AND thread_id = $2`,
		userID, threadID,
	)
	if err != nil {
		slog.Error("failed to unfollow thread", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	slog.Info("thread unfollowed", "user_id", userID, "thread_id", threadID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "unfollowed", "thread_id": threadID})
}
// collectThreadAttachmentIDs gathers all attachment IDs from a slice of thread reply responses.
func collectThreadAttachmentIDs(messages []ThreadReplyResponse) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, msg := range messages {
		for _, id := range msg.AttachmentIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

