package ws

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/pkg/metrics"
)

// formatUUIDArray formats a []string of UUIDs as a PostgreSQL array literal.
func formatUUIDArray(ids []string) string {
	if len(ids) == 0 {
		return "{}"
	}
	return "{" + strings.Join(ids, ",") + "}"
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for MVP; restrict in production
	},
}

// Hub manages WebSocket connections and scope-based subscriptions.
// It implements the realtime.Broadcaster interface.
type Hub struct {
	// Registered clients grouped by channel ID: channelID -> set of clients
	channels map[string]map[*Client]bool

	// Registered clients grouped by thread ID: threadID -> set of clients
	threads map[string]map[*Client]bool

	// Register/unregister channels for connection lifecycle
	register   chan *Client
	unregister chan *Client

	// Broadcast channel for outbound messages (scope -> message)
	broadcast chan *broadcastMsg

	mu sync.RWMutex

	pool *pgxpool.Pool

	// Client ID tracking for duplicate detection
	clients map[*Client]bool

	// Agent service for triggering auto-responses
	agentSvc *service.AgentService

	// Mention service for @mention resolution
	mentionSvc *service.MentionService

	// Metrics
	clientCount atomic.Int32
}

// broadcastMsg carries a message to be broadcast with its scope.
type broadcastMsg struct {
	scopeType string
	scopeID   string
	data      []byte
}

// NewHub creates a new Hub.
// agentSvc can be nil initially and set later via SetAgentService
// (needed to avoid circular dependency during initialization).
func NewHub(pool *pgxpool.Pool, agentSvc *service.AgentService) *Hub {
	return &Hub{
		channels:    make(map[string]map[*Client]bool),
		threads:     make(map[string]map[*Client]bool),
		register:    make(chan *Client, 256),
		unregister:  make(chan *Client, 256),
		broadcast:   make(chan *broadcastMsg, 256),
		clients:     make(map[*Client]bool),
		pool:        pool,
		agentSvc:    agentSvc,
		mentionSvc:  service.NewMentionService(pool),
	}
}

// Run starts the Hub's event loop. Must be called as a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := h.clientCount.Add(1)
			metrics.Global.SetWSConnections(int64(count))
			h.mu.Unlock()
			slog.Info("ws: client connected", "user_id", client.userID, "total_clients", count)

		case client := <-h.unregister:
			h.mu.Lock()
			// Clean up channel and thread subscriptions.
			// Always do this regardless of whether the client was registered
			// via the register channel — a client can be subscribed via
			// Subscribe without being formally registered.
			for channelID := range client.channels {
				if subs, ok := h.channels[channelID]; ok {
					delete(subs, client)
					if len(subs) == 0 {
						delete(h.channels, channelID)
					}
				}
			}
			for threadID := range client.threads {
				if subs, ok := h.threads[threadID]; ok {
					delete(subs, client)
					if len(subs) == 0 {
						delete(h.threads, threadID)
					}
				}
			}
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				count := h.clientCount.Add(-1)
				metrics.Global.SetWSConnections(int64(count))
			}
			// Always close send — the caller (WritePump) needs this to stop.
			close(client.send)
			h.mu.Unlock()
			slog.Info("ws: client disconnected", "user_id", client.userID, "total_clients", h.clientCount.Load())

		case msg := <-h.broadcast:
			h.mu.RLock()
			switch msg.scopeType {
			case realtime.ScopeChannel:
				if subs, ok := h.channels[msg.scopeID]; ok {
					for client := range subs {
						select {
						case client.send <- msg.data:
						default:
							// Client's send buffer is full; drop message
							slog.Debug("ws: dropping message, client buffer full",
								"user_id", client.userID, "channel_id", msg.scopeID)
						}
					}
				}
			case realtime.ScopeThread:
				if subs, ok := h.threads[msg.scopeID]; ok {
					for client := range subs {
						select {
						case client.send <- msg.data:
						default:
							slog.Debug("ws: dropping thread message, client buffer full",
								"user_id", client.userID, "thread_id", msg.scopeID)
						}
					}
				}
			case realtime.ScopeUser:
				for client := range h.clients {
					if client.userID == msg.scopeID {
						select {
						case client.send <- msg.data:
						default:
						}
					}
				}
			default:
				// Global broadcast
				for client := range h.clients {
					select {
					case client.send <- msg.data:
					default:
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// --- Broadcaster interface ---

// BroadcastToScope fans a message out to all connections subscribed to a given scope.
func (h *Hub) BroadcastToScope(scopeType, scopeID string, message []byte) {
	h.broadcast <- &broadcastMsg{
		scopeType: scopeType,
		scopeID:   scopeID,
		data:      message,
	}
}

// BroadcastToChannel is a shortcut for BroadcastToScope("channel", channelID, message).
func (h *Hub) BroadcastToChannel(channelID string, message []byte) {
	h.mu.RLock()
	count := len(h.channels[channelID])
	h.mu.RUnlock()
	slog.Debug("ws: BroadcastToChannel", "channel_id", channelID[:8], "subscribers", count)
	h.BroadcastToScope(realtime.ScopeChannel, channelID, message)
}

// BroadcastToThread is a shortcut for BroadcastToScope("thread", threadID, message).
func (h *Hub) BroadcastToThread(threadID string, message []byte) {
	h.BroadcastToScope(realtime.ScopeThread, threadID, message)
}

// SendToUser delivers a message to every connection belonging to userID.
func (h *Hub) SendToUser(userID string, message []byte) {
	h.BroadcastToScope(realtime.ScopeUser, userID, message)
}

// Broadcast fans a message out to every connection on this node.
func (h *Hub) Broadcast(message []byte) {
	h.broadcast <- &broadcastMsg{
		scopeType: "",
		scopeID:   "",
		data:      message,
	}
}

// --- Channel subscription management ---

// Subscribe adds a client to a channel's subscriber set.
func (h *Hub) Subscribe(client *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[channelID] == nil {
		h.channels[channelID] = make(map[*Client]bool)
	}
	h.channels[channelID][client] = true
	client.channels[channelID] = true
}

// Unsubscribe removes a client from a channel's subscriber set.
func (h *Hub) Unsubscribe(client *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.channels[channelID]; ok {
		delete(subs, client)
		if len(subs) == 0 {
			delete(h.channels, channelID)
		}
	}
	delete(client.channels, channelID)
}

// --- Thread subscription management ---

// SubscribeThread adds a client to a thread's subscriber set.
func (h *Hub) SubscribeThread(client *Client, threadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.threads[threadID] == nil {
		h.threads[threadID] = make(map[*Client]bool)
	}
	h.threads[threadID][client] = true
	client.threads[threadID] = true
}

// UnsubscribeThread removes a client from a thread's subscriber set.
func (h *Hub) UnsubscribeThread(client *Client, threadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.threads[threadID]; ok {
		delete(subs, client)
		if len(subs) == 0 {
			delete(h.threads, threadID)
		}
	}
	delete(client.threads, threadID)
}

// SetAgentService sets the agent service reference on the hub.
// Called after initialization to break circular dependency.
func (h *Hub) SetAgentService(agentSvc *service.AgentService) {
	h.agentSvc = agentSvc
}

// ServeWS handles the WebSocket upgrade and connection lifecycle.
// Call this from the HTTP router: GET /ws?token=<jwt>
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Extract JWT from query parameter
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
		return
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		slog.Debug("ws: invalid token", "error", err)
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "error", err)
		return
	}

	client := NewClient(h, conn, claims.Subject)
	h.register <- client

	go client.WritePump()
	go client.ReadPump()
}

// --- Event handlers ---

// handleMessageSend processes a message.send event from a client.
func (h *Hub) handleMessageSend(client *Client, payload MessageSendPayload) {
	// Validate
	if payload.ChannelID == "" {
		client.sendError("INVALID_PAYLOAD", "channel_id is required")
		return
	}
	if payload.Content == "" {
		client.sendError("INVALID_PAYLOAD", "content is required")
		return
	}
	if len(payload.Content) > 10000 {
		client.sendError("INVALID_PAYLOAD", "content exceeds maximum length of 10000 characters")
		return
	}

	// Verify sender is a member of the channel and channel is not archived
	var isMember bool
	var isArchived bool
	err := h.pool.QueryRow(context.Background(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`,
		payload.ChannelID, client.userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("ws: failed to check channel membership", "error", err, "user_id", client.userID)
		client.sendError("INTERNAL_ERROR", "failed to send message")
		return
	}
	if !isMember {
		client.sendError("FORBIDDEN", "you are not a member of this channel")
		return
	}

	err = h.pool.QueryRow(context.Background(),
		`SELECT is_archived FROM channels WHERE id = $1`, payload.ChannelID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		client.sendError("FORBIDDEN", "channel is archived")
		return
	}

	// Resolve @mentions (SOLO-52-B)
	mentionedAgentIDs, hasMentions, err := h.mentionSvc.ResolveMentions(context.Background(), payload.Content, payload.ChannelID)
	if err != nil {
		slog.Error("ws: failed to resolve mentions", "error", err, "user_id", client.userID)
		// Non-fatal: continue without mentions
		mentionedAgentIDs = nil
		hasMentions = false
	}

	// Validate attachment ownership
	attachmentIDs := payload.AttachmentIDs
	if len(attachmentIDs) > 0 {
		var ownedCount int
		err := h.pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM attachments WHERE id = ANY($1::uuid[]) AND user_id = $2`,
			formatUUIDArray(attachmentIDs), client.userID,
		).Scan(&ownedCount)
		if err != nil || ownedCount != len(attachmentIDs) {
			client.sendError("INVALID_PAYLOAD", "one or more attachment IDs are invalid")
			return
		}
	}

	// Insert message with mentioned_agent_ids and attachment_ids
		// Determine sender type — agents table lookup for correct JOIN resolution.
		var isAgent bool
		_ = h.pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, client.userID,
		).Scan(&isAgent)
		senderType := "user"
		if isAgent {
			senderType = "agent"
		}

	now := time.Now()
	messageID := uuid.New().String()

	_, err = h.pool.Exec(context.Background(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, mentioned_agent_ids, attachment_ids, created_at, updated_at)
		 VALUES (\$1, \$2, \$3, \$4, \$5, \$6::uuid[], \$7::uuid[], \$8, \$8)`,
		messageID, payload.ChannelID, senderType, client.userID, payload.Content, formatUUIDArray(mentionedAgentIDs), formatUUIDArray(attachmentIDs), now,
	)
	if err != nil {
		slog.Error("ws: failed to persist message", "error", err, "user_id", client.userID)
		client.sendError("INTERNAL_ERROR", "failed to send message")
		return
	}

	// Get user's display name for the broadcast
	var displayName string
	err = h.pool.QueryRow(context.Background(),
		`SELECT COALESCE(
		(SELECT display_name FROM users WHERE id = $1),
		(SELECT name FROM agents WHERE id = $1),
		'Unknown'
	)`, client.userID,
	).Scan(&displayName)
	if err != nil {
		displayName = "Unknown"
	}

	// Broadcast to channel
	msgNewPayload := MessageNewPayload{
		ID:            messageID,
		ChannelID:     payload.ChannelID,
		SenderType:    "user",
		SenderID:      client.userID,
		SenderName:    displayName,
		Content:       payload.Content,
		ContentType:   "text",
		AttachmentIDs: attachmentIDs,
		CreatedAt:     now.Format(time.RFC3339),
	}

	h.BroadcastToChannel(payload.ChannelID, Envelope(EventMessageNew, msgNewPayload))

	slog.Debug("ws: message broadcast",
		"message_id", messageID,
		"channel_id", payload.ChannelID,
		"user_id", client.userID,
		"content_length", len(payload.Content),
		"mentioned_agents", mentionedAgentIDs,
	)

	// Also broadcast dm.message.new if the channel is a DM (SOLO-57-B)
	go h.broadcastDMMessageIfNeeded(payload.ChannelID, msgNewPayload)

		// Resolve user @mentions and broadcast inbox.updated to mentioned users (v1.5).
		if h.mentionSvc != nil {
			go func() {
				mentionedUsers, err := h.mentionSvc.ResolveUserMentions(context.Background(), payload.Content, messageID)
				if err != nil {
					slog.Warn("failed to resolve user mentions in WS", "error", err)
					return
				}
				for _, uid := range mentionedUsers {
					env := Envelope(EventInboxUpdated, struct{}{})
					h.SendToUser(uid, env)
				}
			}()
		}

	// Trigger agent auto-response with @mention support
	if h.agentSvc != nil {
		go h.agentSvc.TriggerAgentResponse(
			context.Background(),
			payload.ChannelID,
			messageID,
			senderType,
			client.userID,
			mentionedAgentIDs,
			hasMentions,
				nil,
		)
	}
}

// broadcastDMMessageIfNeeded checks if the channel is a DM and broadcasts dm.message.new.
func (h *Hub) broadcastDMMessageIfNeeded(channelID string, msg MessageNewPayload) {
	var channelType string
	err := h.pool.QueryRow(context.Background(),
		`SELECT type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil {
		return
	}
	if channelType != "dm" {
		return
	}

	dmPayload := DMMessageNewPayload{
		DMID:          channelID,
		ID:            msg.ID,
		ChannelID:     msg.ChannelID,
		SenderType:    msg.SenderType,
		SenderID:      msg.SenderID,
		SenderName:    msg.SenderName,
		Content:       msg.Content,
		ContentType:   msg.ContentType,
		AttachmentIDs: msg.AttachmentIDs,
		CreatedAt:     msg.CreatedAt,
	}
	h.BroadcastToChannel(channelID, Envelope(EventDMMessageNew, dmPayload))
}

// handleTyping broadcasts a typing indicator to a channel.
func (h *Hub) handleTyping(client *Client, payload TypingPayload) {
	if payload.ChannelID == "" {
		return
	}
	// Broadcast typing indicator to all other channel members
	// (excluding the sender)
	typingData := map[string]string{
		"channel_id": payload.ChannelID,
		"user_id":    client.userID,
	}
	h.BroadcastToChannel(payload.ChannelID, Envelope(EventTyping, typingData))
}

// handleThreadReply processes a thread.reply event from a client.
func (h *Hub) handleThreadReply(client *Client, payload ThreadReplyPayload) {
	// Validate
	if payload.ChannelID == "" {
		client.sendError("INVALID_PAYLOAD", "channel_id is required")
		return
	}
	if payload.ThreadID == "" {
		client.sendError("INVALID_PAYLOAD", "thread_id is required")
		return
	}
	if payload.Content == "" {
		client.sendError("INVALID_PAYLOAD", "content is required")
		return
	}
	if len(payload.Content) > 10000 {
		client.sendError("INVALID_PAYLOAD", "content exceeds maximum length of 10000 characters")
		return
	}

	// Verify sender is a member of the channel
	var isMember bool
	err := h.pool.QueryRow(context.Background(),
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`,
		payload.ChannelID, client.userID,
	).Scan(&isMember)
	if err != nil {
		slog.Error("ws: failed to check channel membership", "error", err, "user_id", client.userID)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}
	if !isMember {
		client.sendError("FORBIDDEN", "you are not a member of this channel")
		return
	}

	// Check channel is not archived
	var isArchived bool
	err = h.pool.QueryRow(context.Background(),
		`SELECT is_archived FROM channels WHERE id = $1`, payload.ChannelID,
	).Scan(&isArchived)
	if err == nil && isArchived {
		client.sendError("FORBIDDEN", "channel is archived")
		return
	}

	// Resolve @mentions in thread replies
	mentionedAgentIDs, hasMentions, err := h.mentionSvc.ResolveMentions(context.Background(), payload.Content, payload.ChannelID)
	if err != nil {
		slog.Error("ws: failed to resolve mentions", "error", err, "user_id", client.userID)
		mentionedAgentIDs = nil
		hasMentions = false
	}

	// Validate attachment ownership
	attachmentIDs := payload.AttachmentIDs
	if len(attachmentIDs) > 0 {
		var ownedCount int
		err := h.pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM attachments WHERE id = ANY($1::uuid[]) AND user_id = $2`,
			formatUUIDArray(attachmentIDs), client.userID,
		).Scan(&ownedCount)
		if err != nil || ownedCount != len(attachmentIDs) {
			client.sendError("INVALID_PAYLOAD", "one or more attachment IDs are invalid")
			return
		}
	}

	// Use a transaction to atomically insert message and update thread
	tx, err := h.pool.Begin(context.Background())
	if err != nil {
		slog.Error("ws: failed to begin transaction", "error", err)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}
	defer tx.Rollback(context.Background())

	// Verify thread exists and belongs to this channel
	var rootMessageID string
	err = tx.QueryRow(context.Background(),
		`SELECT root_message_id FROM threads
		 WHERE id = $1 AND channel_id = $2`,
		payload.ThreadID, payload.ChannelID,
	).Scan(&rootMessageID)
	if err != nil {
		client.sendError("NOT_FOUND", "thread not found")
		return
	}

	// Insert reply message with mentioned_agent_ids and attachment_ids
		// Determine sender type — agents table lookup for correct JOIN resolution.
		var isAgentReply bool
		_ = h.pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, client.userID,
		).Scan(&isAgentReply)
		senderType := "user"
		if isAgentReply {
			senderType = "agent"
		}

	now := time.Now()
	messageID := uuid.New().String()

	_, err = tx.Exec(context.Background(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, mentioned_agent_ids, attachment_ids, created_at, updated_at)
		 VALUES (\$1, \$2, \$3, \$4, \$5, \$6, \$7::uuid[], \$8::uuid[], \$9, \$9)`,
		messageID, payload.ChannelID, senderType, client.userID, payload.Content, payload.ThreadID, formatUUIDArray(mentionedAgentIDs), formatUUIDArray(attachmentIDs), now,
	)
	if err != nil {
		slog.Error("ws: failed to persist thread reply", "error", err, "user_id", client.userID)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}

	// Update thread reply count and last_reply_at
	_, err = tx.Exec(context.Background(),
		`UPDATE threads SET reply_count = reply_count + 1, last_reply_at = $1
		 WHERE id = $2`,
		now, payload.ThreadID,
	)
	if err != nil {
		slog.Error("ws: failed to update thread reply count", "error", err)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}

	// Get current reply count for notification
	var replyCount int
	err = tx.QueryRow(context.Background(),
		`SELECT reply_count FROM threads WHERE id = $1`, payload.ThreadID,
	).Scan(&replyCount)
	if err != nil {
		slog.Error("ws: failed to get reply count", "error", err)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}

	if err := tx.Commit(context.Background()); err != nil {
		slog.Error("ws: failed to commit thread reply transaction", "error", err)
		client.sendError("INTERNAL_ERROR", "failed to send reply")
		return
	}

	// Get user's display name
	var displayName string
	err = h.pool.QueryRow(context.Background(),
		`SELECT COALESCE(
		(SELECT display_name FROM users WHERE id = $1),
		(SELECT name FROM agents WHERE id = $1),
		'Unknown'
	)`, client.userID,
	).Scan(&displayName)
	if err != nil {
		displayName = "Unknown"
	}

	// Broadcast thread.message.new to thread subscribers
	threadMsgPayload := ThreadMessageNewPayload{
		Message: ThreadMessageItem{
			ID:            messageID,
			ChannelID:     payload.ChannelID,
			ThreadID:      payload.ThreadID,
			SenderType:    senderType,
			SenderID:      client.userID,
			SenderName:    displayName,
			Content:       payload.Content,
			ContentType:   "text",
			AttachmentIDs: attachmentIDs,
			CreatedAt:     now.Format(time.RFC3339),
		},
		Thread: ThreadMetadataItem{
			ThreadID:    payload.ThreadID,
			ReplyCount:  replyCount,
			LastReplyAt: now.Format(time.RFC3339),
		},
	}
	h.BroadcastToThread(payload.ThreadID, Envelope(EventThreadMessageNew, threadMsgPayload))

	// Broadcast thread.reply notification to channel subscribers
	notifyPayload := ThreadReplyNotifyPayload{
		ChannelID:     payload.ChannelID,
		ThreadID:      payload.ThreadID,
		RootMessageID: rootMessageID,
		ReplyCount:    replyCount,
		LastReplyAt:   now.Format(time.RFC3339),
		LatestReply: &LatestReplyItem{
			ID:         messageID,
			SenderType: senderType,
			SenderID:   client.userID,
			SenderName: displayName,
			Content:    payload.Content,
			CreatedAt:  now.Format(time.RFC3339),
		},
	}
	h.BroadcastToChannel(payload.ChannelID, Envelope(EventThreadReplyNotify, notifyPayload))

	// Broadcast inbox.updated to all user participants of this thread (v1.5).
	go h.notifyInboxForThread(context.Background(), payload.ThreadID, payload.ChannelID, client.userID)

		// Resolve user @mentions and broadcast inbox.updated to mentioned users (v1.5).
		if h.mentionSvc != nil {
			go func() {
				mentionedUsers, err := h.mentionSvc.ResolveUserMentions(context.Background(), payload.Content, messageID)
				if err != nil {
					slog.Warn("failed to resolve user mentions in thread reply", "error", err)
					return
				}
				for _, uid := range mentionedUsers {
					env := Envelope(EventInboxUpdated, struct{}{})
					h.SendToUser(uid, env)
				}
			}()
		}

	// Trigger agent auto-response in thread with @mention support
	if h.agentSvc != nil {
		go h.agentSvc.TriggerAgentResponseInThread(
			context.Background(),
			payload.ChannelID,
			payload.ThreadID,
			senderType,
			client.userID,
			mentionedAgentIDs,
			hasMentions,
			nil,
		)
	}
}

// notifyInboxForThread sends an inbox.updated event to every user participant
// of a thread, except the message sender. Called after a new thread reply is
// persisted, both via WS and REST paths.
func (h *Hub) notifyInboxForThread(ctx context.Context, threadID, channelID, senderID string) {
	if h.pool == nil {
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
		h.SendToUser(userID, Envelope(EventInboxUpdated, struct{}{}))
	}
}
