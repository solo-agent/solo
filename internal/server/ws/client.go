package ws

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/solo-ai/solo/internal/realtime"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 100 * 1024 // 100KB
)

// Client represents a single WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string

	// Subscribed channel IDs
	channels map[string]bool

	// Subscribed thread IDs
	threads map[string]bool
}

// NewClient creates a new Client.
func NewClient(hub *Hub, conn *websocket.Conn, userID string) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		channels: make(map[string]bool),
		threads:  make(map[string]bool),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub.
//
// A goroutine is started per connection. The application ensures
// there is at most one reader on a connection by executing all reads
// from this goroutine.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("ws: unexpected close", "user_id", c.userID, "error", err)
			}
			break
		}

		var msg realtime.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			slog.Debug("ws: invalid message", "user_id", c.userID, "error", err)
			c.sendError("INVALID_MESSAGE", "invalid message format")
			continue
		}

		c.handleMessage(msg)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
//
// A goroutine is started per connection. The application ensures
// there is at most one writer on a connection by executing all writes
// from this goroutine.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Debug("ws: write error", "user_id", c.userID, "error", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendError sends an error message to this client.
func (c *Client) sendError(code, message string) {
	payload := ErrorPayload{Code: code, Message: message}
	select {
	case c.send <- Envelope(EventError, payload):
	default:
		slog.Debug("ws: dropping error message, send buffer full", "user_id", c.userID)
	}
}

// IsSubscribed checks if the client is subscribed to a channel.
func (c *Client) IsSubscribed(channelID string) bool {
	c.hub.mu.RLock()
	defer c.hub.mu.RUnlock()
	_, ok := c.channels[channelID]
	return ok
}

// handleMessage dispatches an incoming WS message to the appropriate handler.
func (c *Client) handleMessage(msg realtime.WSMessage) {
	switch msg.Type {
	case EventSubscribe:
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid subscribe payload")
			return
		}
		if payload.ChannelID == "" {
			c.sendError("INVALID_PAYLOAD", "channel_id is required")
			return
		}
		c.hub.Subscribe(c, payload.ChannelID)

	case EventUnsubscribe:
		var payload UnsubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid unsubscribe payload")
			return
		}
		c.hub.Unsubscribe(c, payload.ChannelID)

	case EventMessageSend:
		var payload MessageSendPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid message.send payload")
			return
		}
		c.hub.handleMessageSend(c, payload)

	case EventTypingStart:
		var payload TypingPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid typing payload")
			return
		}
		c.hub.handleTyping(c, payload)

	case EventChannelJoin:
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid channel.join payload")
			return
		}
		c.hub.Subscribe(c, payload.ChannelID)

	case EventChannelLeave:
		var payload UnsubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid channel.leave payload")
			return
		}
		c.hub.Unsubscribe(c, payload.ChannelID)

	case EventThreadReply:
		var payload ThreadReplyPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid thread.reply payload")
			return
		}
		c.hub.handleThreadReply(c, payload)

	case EventThreadSubscribe:
		var payload ThreadSubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid thread.subscribe payload")
			return
		}
		if payload.ThreadID == "" {
			c.sendError("INVALID_PAYLOAD", "thread_id is required")
			return
		}
		c.hub.SubscribeThread(c, payload.ThreadID)

	case EventThreadUnsubscribe:
		var payload ThreadUnsubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid thread.unsubscribe payload")
			return
		}
		if payload.ThreadID == "" {
			c.sendError("INVALID_PAYLOAD", "thread_id is required")
			return
		}
		c.hub.UnsubscribeThread(c, payload.ThreadID)

	case EventDMSubscribe:
		var payload DMSubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid dm.subscribe payload")
			return
		}
		if payload.DMChannelID == "" {
			c.sendError("INVALID_PAYLOAD", "dm_id is required")
			return
		}
		slog.Info("ws: dm.subscribe received", "dm_id", payload.DMChannelID, "user_id", c.userID)
		c.hub.Subscribe(c, payload.DMChannelID)

	case EventDMUnsubscribe:
		var payload DMUnsubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("INVALID_PAYLOAD", "invalid dm.unsubscribe payload")
			return
		}
		if payload.DMChannelID == "" {
			c.sendError("INVALID_PAYLOAD", "dm_id is required")
			return
		}
		c.hub.Unsubscribe(c, payload.DMChannelID)

	case EventTaskCancel:
		slog.Info("ws: task cancel requested", "user_id", c.userID)
		c.sendError("TASK_CANCEL", "cancel request received")

	default:
		slog.Debug("ws: unknown event type", "user_id", c.userID, "type", msg.Type)
		c.sendError("UNKNOWN_EVENT", "unknown event type: "+msg.Type)
	}
}
