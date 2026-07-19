package handler

import (
	"context"
	"encoding/json"
	"errors"
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

// Maximum number of messages that can be fetched in a single page.
const maxMessageLimit = 100

// defaultMessageLimit is the default page size when no limit is specified.
const defaultMessageLimit = 50

// formatUUIDArray formats a []string of UUIDs as a PostgreSQL array literal.
func formatUUIDArray(ids []string) string {
	if len(ids) == 0 {
		return "{}"
	}
	return "{" + strings.Join(ids, ",") + "}"
}

func nullableUUIDString(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func canPostToThinkingNode(senderType, senderID, nodeAgentID string) bool {
	return senderType != "agent" || senderID == nodeAgentID
}

var errThinkingNodeScopeConflict = errors.New("requested Thinking node conflicts with executing Agent run")

func reconcileThinkingNodeScope(ctx context.Context, pool *pgxpool.Pool, agentID, channelID, requestedNodeID string) (string, error) {
	activeNodeID, err := service.NewAgentRunService(pool).ResolveExecutingThinkingNode(ctx, agentID, channelID)
	if err != nil || activeNodeID == "" {
		return requestedNodeID, err
	}
	if requestedNodeID != "" && requestedNodeID != activeNodeID {
		return "", errThinkingNodeScopeConflict
	}
	return activeNodeID, nil
}

func writeThinkingScopeError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrAmbiguousAgentRunScope) || errors.Is(err, errThinkingNodeScopeConflict) {
		writeError(w, http.StatusConflict, "ambiguous Thinking runtime route")
		return
	}
	writeError(w, http.StatusInternalServerError, "failed to resolve Thinking runtime route")
}

// MessageHandler handles message-related HTTP requests.
type MessageHandler struct {
	pool       *pgxpool.Pool
	hub        *ws.Hub
	mentionSvc *service.MentionService
	agentSvc   *service.AgentService
	taskSvc    *service.TaskService
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(pool *pgxpool.Pool, hub *ws.Hub, agentSvc *service.AgentService, taskSvc *service.TaskService) *MessageHandler {
	return &MessageHandler{
		pool:       pool,
		hub:        hub,
		mentionSvc: service.NewMentionService(pool),
		agentSvc:   agentSvc,
		taskSvc:    taskSvc,
	}
}

// --- Request/Response types ---

type CreateMessageRequest struct {
	Content        string   `json:"content"`
	AsTask         bool     `json:"as_task,omitempty"`
	AttachmentIDs  []string `json:"attachment_ids,omitempty"`
	ThreadID       string   `json:"thread_id,omitempty"`
	ThinkingNodeID string   `json:"thinking_node_id,omitempty"`
}

// AttachmentMeta is the attachment metadata included in message responses.
type AttachmentMeta struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type MessageResponse struct {
	ID                 string           `json:"id"`
	TaskNumber         int              `json:"task_number,omitempty"`
	ChannelID          string           `json:"channel_id"`
	SenderType         string           `json:"sender_type"`
	SenderID           string           `json:"sender_id"`
	SenderName         string           `json:"sender_name,omitempty"`
	SenderActive       bool             `json:"sender_active"`
	Content            string           `json:"content"`
	ContentType        string           `json:"content_type"`
	MentionedAgentIDs  []string         `json:"mentioned_agent_ids,omitempty"`
	AttachmentIDs      []string         `json:"attachment_ids,omitempty"`
	Attachments        []AttachmentMeta `json:"attachments,omitempty"`
	CreatedAt          string           `json:"created_at"`
	ReplyCount         int              `json:"reply_count,omitempty"`
	TaskStatus         string           `json:"task_status,omitempty"`
	TaskClaimerName    string           `json:"task_claimer_name,omitempty"`
	TaskClaimerDeleted bool             `json:"task_claimer_deleted"`
	HasUnreadThread    bool             `json:"has_unread_thread,omitempty"`
	ThinkingNodeID     string           `json:"thinking_node_id,omitempty"`
}

type MessageListResponse struct {
	Messages []MessageResponse `json:"messages"`
	HasMore  bool              `json:"has_more"`
}

// Create handles POST /api/v1/channels/{channelID}/messages
func (h *MessageHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	attachmentIDs := req.AttachmentIDs
	if !hasMessageBody(content, attachmentIDs) {
		writeError(w, http.StatusBadRequest, "message content or attachment is required")
		return
	}
	if req.AsTask && content == "" {
		writeError(w, http.StatusBadRequest, "task content is required")
		return
	}
	if len(content) > 10000 {
		writeError(w, http.StatusBadRequest, "message content exceeds maximum length of 10000 characters")
		return
	}
	thinkingNodeID := strings.TrimSpace(req.ThinkingNodeID)
	if thinkingNodeID != "" {
		if req.ThreadID != "" || req.AsTask {
			writeError(w, http.StatusBadRequest, "thinking node messages cannot also be threads or tasks")
			return
		}
		if _, err := uuid.Parse(thinkingNodeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid thinking node ID")
			return
		}
	}

	// Verify sender is a member of the channel and channel is not archived
	var isMember bool
	var isArchived bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("failed to check channel membership", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "you are not a member of this channel")
		return
	}

	// Check channel is not archived
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_archived FROM channels WHERE id = $1`, channelID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		writeError(w, http.StatusGone, "channel is archived")
		return
	}

	// Determine sender type before resolving runtime-owned node scope. A
	// legacy Agent CLI may omit thinking_node_id, but the executing Agent Run
	// still provides an authoritative route.
	var isAgent bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, userID,
	).Scan(&isAgent)
	senderType := "user"
	if isAgent {
		senderType = "agent"
		thinkingNodeID, err = reconcileThinkingNodeScope(r.Context(), h.pool, userID, channelID, thinkingNodeID)
		if err != nil {
			writeThinkingScopeError(w, err)
			return
		}
	}
	if thinkingNodeID != "" && (req.ThreadID != "" || req.AsTask) {
		writeError(w, http.StatusBadRequest, "thinking node messages cannot also be threads or tasks")
		return
	}

	// Resolve @mentions (SOLO-52-B)
	mentionedAgentIDs, _, err := h.mentionSvc.ResolveMentions(r.Context(), content, channelID)
	if err != nil {
		slog.Error("failed to resolve mentions", "error", err)
		mentionedAgentIDs = nil
	}

	// Resolve thread_id to a valid threads.id.
	// Accepts: short message ID prefix, full message UUID, or existing thread UUID.
	threadID := req.ThreadID
	if threadID != "" {
		threadSvc := service.NewThreadService(h.pool)
		// Check if already a valid thread UUID.
		var threadExists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM threads WHERE id = $1)`, threadID,
		).Scan(&threadExists)
		if threadExists {
			// Already valid — use as-is.
		} else if _, uuidErr := uuid.Parse(threadID); uuidErr != nil {
			// Short ID — resolve via LIKE to find message, then get/create thread.
			var msgID, tid string
			err := h.pool.QueryRow(r.Context(),
				`SELECT m.id, COALESCE(t.id::text, m.thread_id::text, '') FROM messages m
				 LEFT JOIN threads t ON t.root_message_id = m.id AND t.channel_id = m.channel_id
				 WHERE m.id::text LIKE $1 AND m.channel_id = $2
				 ORDER BY m.created_at DESC LIMIT 1`,
				threadID+"%", channelID,
			).Scan(&msgID, &tid)
			if err != nil || tid == "" {
				tid, _, err = threadSvc.GetOrCreateThread(r.Context(), channelID, msgID)
				if err != nil {
					slog.Warn("failed to resolve thread from short ID", "short_id", threadID, "error", err)
					threadID = ""
				}
			}
			if tid != "" {
				threadID = tid
			}
		} else {
			// Full UUID not a thread — check if message is already in a thread first.
			var existingThreadID string
			_ = h.pool.QueryRow(r.Context(),
				`SELECT COALESCE(t.id::text, m.thread_id::text, '') FROM messages m
				 LEFT JOIN threads t ON t.root_message_id = m.id
				 WHERE m.id = $1`, threadID,
			).Scan(&existingThreadID)
			if existingThreadID != "" {
				threadID = existingThreadID
			} else {
				tid, _, err := threadSvc.GetOrCreateThread(r.Context(), channelID, threadID)
				if err != nil {
					slog.Warn("failed to get-or-create thread for message", "message_id", threadID, "error", err)
					threadID = ""
				} else {
					threadID = tid
				}
			}
		}
	}
	// Validate attachment ownership
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

	autoSplitTitles := []string{}
	var handoffProtocol *service.ThinkingHandoffProtocol
	if thinkingNodeID != "" {
		thinkingSvc := service.NewThinkingService(h.pool)
		node, err := thinkingSvc.GetNodeForChannel(r.Context(), channelID, thinkingNodeID)
		if err != nil {
			writeError(w, http.StatusNotFound, "thinking node not found")
			return
		}
		if node.ReturnedAt != nil {
			writeError(w, http.StatusConflict, service.ErrThinkingReturned.Error())
			return
		}
		if node.ForkHandoffPending {
			writeError(w, http.StatusConflict, service.ErrThinkingPreparing.Error())
			return
		}
		if node.ReturningAt != nil && senderType != "agent" {
			writeError(w, http.StatusConflict, service.ErrThinkingReturning.Error())
			return
		}
		if senderType == "agent" {
			if !canPostToThinkingNode(senderType, userID, node.AgentID) {
				writeError(w, http.StatusForbidden, "agent is not assigned to this thinking node")
				return
			}
			if protocol, ok := service.ParseThinkingHandoffProtocol(content); ok {
				handoffProtocol = &protocol
				content = protocol.Content
			}
			if node.ReturningAt != nil && (handoffProtocol == nil || handoffProtocol.Kind != "return") {
				writeError(w, http.StatusConflict, "thinking return requires the handoff protocol")
				return
			}
			if handoffProtocol != nil && handoffProtocol.Kind == "return" && node.ReturningAt == nil {
				writeError(w, http.StatusConflict, "thinking node is not returning")
				return
			}
			if node.ReturningAt == nil {
				if handoffProtocol == nil {
					content, autoSplitTitles = service.ExtractThinkingSplits(content)
					if content == "" && len(autoSplitTitles) > 0 {
						content = "Split into: " + strings.Join(autoSplitTitles, ", ")
					}
				}
			}
		}
	}

	if handoffProtocol != nil {
		thinkingSvc := service.NewThinkingService(h.pool)
		updatedNodeID := thinkingNodeID
		switch handoffProtocol.Kind {
		case "checkpoint":
			node, err := thinkingSvc.SaveCheckpointHandoff(r.Context(), channelID, thinkingNodeID, userID, handoffProtocol.Content)
			if err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			updatedNodeID = node.ID
		case "fork":
			node, err := thinkingSvc.CompleteForkHandoff(r.Context(), channelID, thinkingNodeID, handoffProtocol.TargetID, userID, handoffProtocol.Content)
			if err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			updatedNodeID = node.ID
		case "return":
			node, returnedMessage, err := thinkingSvc.CompleteReturn(r.Context(), channelID, thinkingNodeID, userID, handoffProtocol.Content)
			if err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			updatedNodeID = node.ID
			if h.hub != nil {
				h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
					ID: returnedMessage.ID, ChannelID: channelID, SenderType: "system", SenderID: "system",
					SenderName: "Solo", Content: returnedMessage.Content, ContentType: "thinking_handoff",
					ThinkingNodeID: node.ParentID, CreatedAt: returnedMessage.CreatedAt.Format(time.RFC3339),
				}))
			}
		}
		if h.hub != nil {
			h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{
				"channel_id": channelID, "node_id": updatedNodeID,
			}))
		}
		writeJSON(w, http.StatusCreated, MessageResponse{
			ID: uuid.NewString(), ChannelID: channelID, SenderType: "agent", SenderID: userID,
			ContentType: "thinking_handoff_protocol", ThinkingNodeID: thinkingNodeID,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Insert message with mentioned_agent_ids and attachment_ids
	now := time.Now()
	messageID := uuid.New().String()

	var nullableThreadID interface{}
	if threadID != "" {
		nullableThreadID = threadID
	}
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, thread_id, thinking_node_id, sender_type, sender_id, content, mentioned_agent_ids, attachment_ids, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::uuid[], $9::uuid[], $10, $10)`,
		messageID, channelID, nullableThreadID, nullableUUIDString(thinkingNodeID), senderType, userID, content, formatUUIDArray(mentionedAgentIDs), formatUUIDArray(attachmentIDs), now,
	)
	if err != nil {
		slog.Error("failed to persist message", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	if thinkingNodeID != "" {
		thinkingSvc := service.NewThinkingService(h.pool)
		topologyChanged := false
		for _, title := range autoSplitTitles {
			child, err := thinkingSvc.CreateChild(r.Context(), channelID, thinkingNodeID, title, userID, "auto")
			if err != nil {
				slog.Warn("failed to create automatic thinking branch", "node_id", thinkingNodeID, "title", title, "error", err)
			} else {
				topologyChanged = true
				if child.ForkHandoffPending && h.agentSvc != nil {
					if err := h.agentSvc.TriggerThinkingForkHandoff(r.Context(), channelID, thinkingNodeID, child.ID, child.Title); err != nil {
						slog.Warn("failed to prepare automatic fork handoff", "node_id", child.ID, "error", err)
					}
				}
			}
		}
		if topologyChanged && h.hub != nil {
			h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{"channel_id": channelID, "node_id": thinkingNodeID}))
		}
	}
	// Update thread reply_count and store threadRootMsgID for later broadcast
	var threadRootMsgID string
	var threadReplyCount int
	if threadID != "" {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE threads SET reply_count = reply_count + 1, last_reply_at = $1
			 WHERE id = $2`, now, threadID)

		_ = h.pool.QueryRow(r.Context(),
			`SELECT t.root_message_id, t.reply_count FROM threads t WHERE t.id = $1`,
			threadID,
		).Scan(&threadRootMsgID, &threadReplyCount)
	}

	// Get sender name
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

	// Fetch attachment metadata for the response and realtime payloads.
	var attachments []AttachmentMeta
	if len(attachmentIDs) > 0 {
		attachments, _ = queryAttachments(r.Context(), h.pool, attachmentIDs)
	}

	// Broadcast thread reply_count update and thread.message.new
	if threadRootMsgID != "" && h.hub != nil {
		msgUpdated := ws.Envelope(ws.EventMessageUpdated, ws.MessageUpdatedPayload{
			ID:         threadRootMsgID,
			ChannelID:  channelID,
			ReplyCount: threadReplyCount,
		})
		h.hub.BroadcastToChannel(channelID, msgUpdated)

		threadMsg := ws.ThreadMessageNewPayload{
			Message: ws.ThreadMessageItem{
				ID:            messageID,
				ChannelID:     channelID,
				ThreadID:      threadID,
				SenderType:    senderType,
				SenderID:      userID,
				SenderName:    displayName,
				Content:       content,
				ContentType:   "text",
				AttachmentIDs: attachmentIDs,
				Attachments:   toWSAttachmentMeta(attachments),
				CreatedAt:     now.UTC().Format(time.RFC3339),
			},
			Thread: ws.ThreadMetadataItem{
				ThreadID:    threadID,
				ReplyCount:  threadReplyCount,
				LastReplyAt: now.UTC().Format(time.RFC3339),
			},
		}
		h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsg))
	}

	slog.Info("message created (REST)", "message_id", messageID, "channel_id", channelID, "user_id", userID, "mentions", mentionedAgentIDs)

	// If as_task, convert to task FIRST so task metadata is available
	// for the initial message.new broadcast (one step, no timing gap).
	var taskResp *TaskResponse
	var taskNumber int
	var taskStatus string
	if req.AsTask && h.taskSvc != nil {
		task, err := h.taskSvc.ConvertMessageToTask(r.Context(), channelID, messageID, userID)
		if err != nil {
			slog.Error("failed to auto-create task from message", "error", err, "message_id", messageID)
		} else {
			tr := toTaskResponse(task)
			taskResp = &tr
			taskNumber = task.TaskNumber
			taskStatus = task.Status

			// Broadcast task.created event to channel subscribers
			dueDate := ""
			if task.DueDate != nil {
				dueDate = task.DueDate.Format(time.RFC3339)
			}
			ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
				ID:          task.ID,
				TaskNumber:  task.TaskNumber,
				ChannelID:   task.ChannelID,
				CreatorID:   task.CreatorID,
				CreatorName: task.CreatorName,
				Title:       task.Title,
				Description: task.Description,
				Status:      task.Status,
				ClaimerID:   task.ClaimerID,
				ClaimerName: task.ClaimerName,
				Priority:    task.Priority,
				DueDate:     dueDate,
				MessageID:   task.MessageID,
				CreatedAt:   task.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
			})

			// Trigger all channel agents for this task (auto-claim)
			if h.agentSvc != nil {
				go h.agentSvc.TriggerAllAgentsForTask(context.Background(), channelID, task.ID, task.TaskNumber, task.Title, mentionedAgentIDs, nil)
			}
		}
	}

	// Broadcast message.new via WebSocket with task metadata baked in.
	// No separate message.updated needed — single event, no timing gap.
	if h.hub != nil {
		msgData := ws.MessageNewPayload{
			ID:                messageID,
			ChannelID:         channelID,
			SenderType:        senderType,
			SenderID:          userID,
			SenderName:        displayName,
			Content:           content,
			ContentType:       "text",
			ThreadID:          threadID,
			ThinkingNodeID:    thinkingNodeID,
			MentionedAgentIDs: mentionedAgentIDs,
			AttachmentIDs:     attachmentIDs,
			Attachments:       toWSAttachmentMeta(attachments),
			TaskNumber:        taskNumber,
			TaskStatus:        taskStatus,
			CreatedAt:         now.Format(time.RFC3339),
		}
		msgPayload := ws.Envelope(ws.EventMessageNew, msgData)
		h.hub.BroadcastToChannel(channelID, msgPayload)

		// Also broadcast dm.message.new if this channel is a DM
		go h.broadcastDMIfNeeded(channelID, msgData)
	}

	// Broadcast inbox.updated to thread participants (v1.5).
	if threadID != "" && h.hub != nil {
		go h.notifyInboxForThread(context.Background(), threadID, channelID, userID)

		// Resolve user @mentions and broadcast inbox.updated to mentioned users (v1.5).
		if h.mentionSvc != nil && h.hub != nil && !req.AsTask {
			go func() {
				mentionedUsers, err := h.mentionSvc.ResolveUserMentions(context.Background(), content, messageID)
				if err != nil {
					slog.Warn("failed to resolve user mentions", "request_id", "unknown", "error", err)
					return
				}
				for _, uid := range mentionedUsers {
					ws.BroadcastInboxUpdated(h.hub, uid)
				}
			}()
		}
	}

	// Trigger agent auto-response (skip if asTask — agents triggered by task creation above).
	// P25-05-B: Route to thread-scoped or channel-scoped trigger based on threadID.
	// Thread messages must use TriggerAgentResponseInThread so agents receive
	// thread context and know to reply in the thread. Channel messages use
	// TriggerAgentResponse (existing behavior).
	if h.agentSvc != nil && !req.AsTask {
		hasMentions := len(mentionedAgentIDs) > 0
		if thinkingNodeID != "" {
			go h.agentSvc.TriggerAgentResponseInNode(context.Background(), channelID, thinkingNodeID, messageID, senderType, userID, mentionedAgentIDs, hasMentions, nil)
		} else if threadID != "" {
			go h.agentSvc.TriggerAgentResponseInThread(context.Background(), channelID, threadID, senderType, userID, mentionedAgentIDs, hasMentions, nil)
		} else {
			go h.agentSvc.TriggerAgentResponse(context.Background(), channelID, messageID, senderType, userID, mentionedAgentIDs, hasMentions, nil)
		}
	}

	resp := MessageResponse{
		ID:                messageID,
		ChannelID:         channelID,
		SenderType:        "user",
		SenderID:          userID,
		SenderName:        displayName,
		Content:           content,
		ContentType:       "text",
		MentionedAgentIDs: mentionedAgentIDs,
		AttachmentIDs:     attachmentIDs,
		Attachments:       attachments,
		ThinkingNodeID:    thinkingNodeID,
		CreatedAt:         now.Format(time.RFC3339),
	}

	// If as_task, return the created task response instead of the message response.
	if taskResp != nil {
		writeJSON(w, http.StatusCreated, taskResp)
	} else {
		writeJSON(w, http.StatusCreated, resp)
	}
}

// List handles GET /api/v1/channels/{channelID}/messages
// Supports cursor-based pagination with ?before=<message_id>&limit=<n>
//
// Query plan for large channels (5000+ messages):
// The query uses the (channel_id, created_at DESC, id DESC) composite index:
//   - WHERE channel_id = $1 → index seek on channel_id
//   - (created_at, id) < (cursor) → index range scan using the composite index
//   - ORDER BY created_at DESC, id DESC → index-order scan (no separate sort)
//   - LIMIT → early-stop after fetching limit+1 rows
//
// Expected plan for a channel with 5000 messages:
//
//	Index Scan Backward using idx_messages_channel on messages
//	  Index Cond: (channel_id = $1)
//	  Filter: (ROW(created_at, id) < ROW($2, $3))
//	(Stops after limit rows due to LIMIT)
func (h *MessageHandler) List(w http.ResponseWriter, r *http.Request) {
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

	// Parse and validate query params before any DB interaction (fail fast).
	limit := defaultMessageLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxMessageLimit {
			limit = parsed
		}
	}

	before := r.URL.Query().Get("before")
	if before != "" {
		// Validate cursor is a well-formed UUID before hitting the DB.
		if _, err := uuid.Parse(before); err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor: must be a valid message ID")
			return
		}
	}
	thinkingNodeID := strings.TrimSpace(r.URL.Query().Get("thinking_node_id"))
	if thinkingNodeID != "" {
		if _, err := uuid.Parse(thinkingNodeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid thinking node ID")
			return
		}
	}

	// Check user is a member and channel is not archived
	var isMember bool
	var isArchived bool

	// Guard against nil pool (tests may construct handler without DB).
	// Production handlers always have a pool.
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
	thinkingNodeID, err = reconcileThinkingNodeScope(r.Context(), h.pool, userID, channelID, thinkingNodeID)
	if err != nil {
		writeThinkingScopeError(w, err)
		return
	}
	if thinkingNodeID != "" {
		if _, err := service.NewThinkingService(h.pool).GetNodeForChannel(r.Context(), channelID, thinkingNodeID); err != nil {
			writeError(w, http.StatusNotFound, "thinking node not found")
			return
		}
	}

	// Build query with cursor using tuple comparison for deterministic pagination.
	query := `SELECT m.id, m.channel_id, m.sender_type, m.sender_id,
	                 CASE
		                   WHEN m.sender_type = 'system' THEN 'Solo'
		                   ELSE COALESCE(u.display_name, a.name, m.sender_id::text)
		                 END as sender_name,
		                 COALESCE(a.is_active, false) AS sender_active,
		                 COALESCE(m.thinking_node_id::text, '') AS thinking_node_id,
	                 m.content, m.content_type, COALESCE(m.attachment_ids, '{}') as attachment_ids, m.created_at,
		                 COALESCE(t.reply_count, 0) AS reply_count,
		                 COALESCE(tasks.task_number, 0) AS task_number,
		                 COALESCE(tasks.status, '') AS task_status,
		                 COALESCE(u_claimer.display_name, a_claimer.name, '') AS task_claimer_name,
		                 (NOT COALESCE(a_claimer.is_active, true)) AS task_claimer_deleted,
		                 CASE WHEN t.id IS NOT NULL AND (t.last_reply_at > tr.last_read_at OR tr.last_read_at IS NULL) THEN true ELSE false END AS has_unread_thread
	          FROM messages m
	          LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
	          LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		          LEFT JOIN threads t ON m.id = t.root_message_id
		          LEFT JOIN thread_reads tr ON t.id = tr.thread_id AND tr.user_id = $2
		          LEFT JOIN tasks ON tasks.message_id = m.id
		          LEFT JOIN users u_claimer ON tasks.claimer_id = u_claimer.id
		          LEFT JOIN agents a_claimer ON tasks.claimer_id = a_claimer.id
		          WHERE m.channel_id = $1 AND m.thread_id IS NULL`

	args := []any{channelID, userID}
	if thinkingNodeID == "" {
		query += ` AND m.thinking_node_id IS NULL`
	} else {
		query += ` AND m.thinking_node_id = $` + strconv.Itoa(len(args)+1)
		args = append(args, thinkingNodeID)
	}

	if before != "" {
		query += ` AND (m.created_at, m.id) < (SELECT c.created_at, c.id FROM messages c WHERE c.id = $` + strconv.Itoa(len(args)+1) + `)`
		args = append(args, before)
	}

	query += ` ORDER BY m.created_at DESC, m.id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1) // Fetch one extra to determine has_more

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("failed to query messages", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	defer rows.Close()

	messages := make([]MessageResponse, 0, limit)
	for rows.Next() {
		var msg MessageResponse
		var createdAt time.Time
		err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.SenderType, &msg.SenderID,
			&msg.SenderName, &msg.SenderActive, &msg.ThinkingNodeID, &msg.Content, &msg.ContentType, &msg.AttachmentIDs, &createdAt,
			&msg.ReplyCount, &msg.TaskNumber, &msg.TaskStatus, &msg.TaskClaimerName, &msg.TaskClaimerDeleted, &msg.HasUnreadThread)
		if err != nil {
			slog.Error("failed to scan message row", "error", err)
			continue
		}
		msg.CreatedAt = createdAt.Format(time.RFC3339)
		messages = append(messages, msg)
	}

	// Determine has_more and trim to limit
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// Batch query attachment metadata for all messages
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

	// The messages are in DESC order (newest first). Reverse to return ASC (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	writeJSON(w, http.StatusOK, MessageListResponse{
		Messages: messages,
		HasMore:  hasMore,
	})
}

// --- Message types for edit/delete ---

type UpdateMessageRequest struct {
	Content string `json:"content"`
}

// Update handles PATCH /api/v1/channels/{channelID}/messages/{messageID}
// Edits a message's content. Only the original sender can edit. Sets is_edited = true.
func (h *MessageHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	// Verify message exists, belongs to user, and is not deleted
	var currentSenderType, currentSenderID string
	var isDeleted bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT sender_type, sender_id, COALESCE(is_deleted, false) FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&currentSenderType, &currentSenderID, &isDeleted)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		slog.Error("failed to query message for edit", "error", err)
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
		slog.Error("failed to update message", "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to edit message")
		return
	}

	slog.Info("message updated", "message_id", messageID, "channel_id", channelID, "user_id", userID)

	// Broadcast message.updated event
	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageUpdated, ws.MessageUpdatedPayload{
			ID:          messageID,
			ChannelID:   channelID,
			SenderType:  "user",
			SenderID:    userID,
			Content:     content,
			ContentType: "text",
			IsEdited:    true,
			UpdatedAt:   now.Format(time.RFC3339),
		})
		h.hub.BroadcastToChannel(channelID, msgPayload)
	}

	writeJSON(w, http.StatusOK, MessageResponse{
		ID:          messageID,
		ChannelID:   channelID,
		SenderType:  "user",
		SenderID:    userID,
		Content:     content,
		ContentType: "text",
		CreatedAt:   now.Format(time.RFC3339),
	})
}

// Delete handles DELETE /api/v1/channels/{channelID}/messages/{messageID}
// Soft-deletes a message. Only the original sender can delete. Sets is_deleted = true.
func (h *MessageHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	// Verify message exists, belongs to user, and is not already deleted
	var currentSenderType, currentSenderID string
	var isDeleted bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT sender_type, sender_id, COALESCE(is_deleted, false) FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&currentSenderType, &currentSenderID, &isDeleted)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		slog.Error("failed to query message for delete", "error", err)
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
		slog.Error("failed to soft-delete message", "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete message")
		return
	}

	slog.Info("message deleted", "message_id", messageID, "channel_id", channelID, "user_id", userID)

	// Broadcast message.deleted event
	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageDeleted, ws.MessageDeletedPayload{
			ID:        messageID,
			ChannelID: channelID,
		})
		h.hub.BroadcastToChannel(channelID, msgPayload)
	}

	w.WriteHeader(http.StatusNoContent)
}

// queryAttachments fetches attachment metadata for the given IDs (ordered).
func queryAttachments(ctx context.Context, pool *pgxpool.Pool, ids []string) ([]AttachmentMeta, error) {
	metaMap, err := queryAttachmentMap(ctx, pool, ids)
	if err != nil {
		return nil, err
	}
	result := make([]AttachmentMeta, 0, len(ids))
	for _, id := range ids {
		if m, ok := metaMap[id]; ok {
			result = append(result, m)
		}
	}
	return result, nil
}

// queryAttachmentMap fetches attachment metadata and returns a map keyed by attachment ID.
func queryAttachmentMap(ctx context.Context, pool *pgxpool.Pool, ids []string) (map[string]AttachmentMeta, error) {
	metaMap := make(map[string]AttachmentMeta, len(ids))
	if len(ids) == 0 {
		return metaMap, nil
	}

	rows, err := pool.Query(ctx,
		`SELECT id, filename, mime_type, size, storage_path, COALESCE(thumbnail_path, ''), created_at
		 FROM attachments WHERE id = ANY($1::uuid[])`,
		formatUUIDArray(ids),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var a AttachmentMeta
		var storagePath, thumbnailPath string
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.Filename, &a.MimeType, &a.Size, &storagePath, &thumbnailPath, &createdAt); err != nil {
			continue
		}
		a.CreatedAt = createdAt.Format(time.RFC3339)
		a.URL = "/api/v1/attachments/" + a.ID
		if thumbnailPath != "" {
			a.ThumbnailURL = "/api/v1/attachments/" + a.ID + "/thumbnail"
		}
		metaMap[a.ID] = a
	}
	return metaMap, nil
}

func toWSAttachmentMeta(attachments []AttachmentMeta) []ws.AttachmentMeta {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]ws.AttachmentMeta, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, ws.AttachmentMeta{
			ID:           a.ID,
			Filename:     a.Filename,
			MimeType:     a.MimeType,
			Size:         a.Size,
			URL:          a.URL,
			ThumbnailURL: a.ThumbnailURL,
			CreatedAt:    a.CreatedAt,
		})
	}
	return out
}

// broadcastDMIfNeeded checks if the channel is a DM and broadcasts dm.message.new.
func (h *MessageHandler) broadcastDMIfNeeded(channelID string, msg ws.MessageNewPayload) {
	var channelType string
	err := h.pool.QueryRow(context.Background(),
		`SELECT type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil || channelType != "dm" {
		return
	}

	dmPayload := ws.DMMessageNewPayload{
		DMID:          channelID,
		ID:            msg.ID,
		ChannelID:     msg.ChannelID,
		SenderType:    msg.SenderType,
		SenderID:      msg.SenderID,
		SenderName:    msg.SenderName,
		Content:       msg.Content,
		ContentType:   msg.ContentType,
		AttachmentIDs: msg.AttachmentIDs,
		Attachments:   msg.Attachments,
		ThreadID:      msg.ThreadID,
		CreatedAt:     msg.CreatedAt,
	}
	h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventDMMessageNew, dmPayload))
}

// notifyInboxForThread sends inbox.updated to all user participants of a thread
// except the message sender. This is called after a new thread reply is created.
func (h *MessageHandler) notifyInboxForThread(ctx context.Context, threadID, channelID, senderID string) {
	if h.hub == nil || h.pool == nil {
		return
	}
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT m.sender_id
		 FROM messages m
		 WHERE m.thread_id = $1
		   AND m.sender_type = 'user'
		   AND m.sender_id != $2`,
		threadID, senderID,
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

// collectAttachmentIDs gathers all attachment IDs from a slice of message responses.
func collectAttachmentIDs(messages []MessageResponse) []string {
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

func hasMessageBody(content string, attachmentIDs []string) bool {
	return strings.TrimSpace(content) != "" || len(attachmentIDs) > 0
}

// Check handles GET /api/v1/messages/check — a lightweight polling endpoint for
// agents to pull pending messages. It supports an optional ?channel_id= query param.
func (h *MessageHandler) Check(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	thinkingNodeID := strings.TrimSpace(r.URL.Query().Get("thinking_node_id"))
	if thinkingNodeID != "" {
		if _, err := uuid.Parse(thinkingNodeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid thinking node ID")
			return
		}
	}

	limit := defaultMessageLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxMessageLimit {
			limit = parsed
		}
	}

	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	var isMember bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("message check: membership check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isMember {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	thinkingNodeID, err = reconcileThinkingNodeScope(r.Context(), h.pool, userID, channelID, thinkingNodeID)
	if err != nil {
		writeThinkingScopeError(w, err)
		return
	}
	if thinkingNodeID != "" {
		if _, err := service.NewThinkingService(h.pool).GetNodeForChannel(r.Context(), channelID, thinkingNodeID); err != nil {
			writeError(w, http.StatusNotFound, "thinking node not found")
			return
		}
	}

	query := `SELECT m.id, m.channel_id, m.sender_type, m.sender_id,
				m.content, m.reply_to, m.created_at, m.updated_at
		 FROM messages m
		 WHERE m.channel_id = $1 AND COALESCE(m.is_deleted, false) = false`
	args := []any{channelID}
	if thinkingNodeID == "" {
		query += ` AND m.thinking_node_id IS NULL`
	} else {
		query += ` AND m.thinking_node_id = $2`
		args = append(args, thinkingNodeID)
	}
	query += ` ORDER BY m.created_at DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("message check: query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query messages")
		return
	}
	defer rows.Close()

	type msg struct {
		ID         string  `json:"id"`
		ChannelID  string  `json:"channel_id"`
		SenderType string  `json:"sender_type"`
		SenderID   string  `json:"sender_id"`
		Content    string  `json:"content"`
		ReplyTo    *string `json:"reply_to,omitempty"`
		CreatedAt  string  `json:"created_at"`
		UpdatedAt  string  `json:"updated_at"`
	}
	messages := make([]msg, 0, limit)
	for rows.Next() {
		var m msg
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.SenderType, &m.SenderID,
			&m.Content, &m.ReplyTo, &createdAt, &updatedAt); err != nil {
			slog.Error("message check: scan failed", "error", err)
			continue
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		m.UpdatedAt = updatedAt.Format(time.RFC3339)
		messages = append(messages, m)
	}

	writeJSON(w, http.StatusOK, messages)
}
