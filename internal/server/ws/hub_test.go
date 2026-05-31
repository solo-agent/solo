package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestServer creates a test HTTP server with a Hub for testing WS connections.
// It returns the server URL and a cleanup function.
func newTestServer(t *testing.T) (*Hub, string, func()) {
	t.Helper()

	hub := NewHub(nil, nil)
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)

	server := httptest.NewServer(mux)

	return hub, server.URL, func() {
		server.Close()
	}
}

// dialClient connects to the test WS server with the given token.
func dialClient(t *testing.T, serverURL, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws?token=" + token
	return websocket.DefaultDialer.Dial(u, nil)
}

func TestHubSubscriptions(t *testing.T) {
	hub, _, cleanup := newTestServer(t)
	defer cleanup()

	// Connect client using a fake JWT (just testing hub, not auth)
	// We bypass auth by calling internal methods directly
	client := NewClient(hub, nil, "user-1")
	client.send = make(chan []byte, 256)

	// Test subscribe
	hub.Subscribe(client, "channel-1")
	if !client.channels["channel-1"] {
		t.Error("expected client to be subscribed to channel-1")
	}

	// Verify hub has the subscription
	hub.mu.RLock()
	subs, ok := hub.channels["channel-1"]
	hub.mu.RUnlock()
	if !ok {
		t.Fatal("expected channel-1 to have subscribers")
	}
	if !subs[client] {
		t.Error("expected client to be in channel-1 subscribers")
	}

	// Test broadcast to channel
	msg := []byte(`{"type":"test","payload":"hello"}`)
	hub.BroadcastToChannel("channel-1", msg)

	select {
	case received := <-client.send:
		if string(received) != string(msg) {
			t.Errorf("expected message %q, got %q", string(msg), string(received))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}

	// Test unsubscribe
	hub.Unsubscribe(client, "channel-1")
	if client.channels["channel-1"] {
		t.Error("expected client to be unsubscribed from channel-1")
	}

	hub.mu.RLock()
	_, ok = hub.channels["channel-1"]
	hub.mu.RUnlock()
	if ok {
		t.Error("expected channel-1 to have no subscribers after unsubscribe")
	}
}

func TestHubMultipleChannels(t *testing.T) {
	hub, _, cleanup := newTestServer(t)
	defer cleanup()

	client := NewClient(hub, nil, "user-1")
	client.send = make(chan []byte, 256)

	// Subscribe to multiple channels
	hub.Subscribe(client, "channel-1")
	hub.Subscribe(client, "channel-2")

	if len(client.channels) != 2 {
		t.Errorf("expected 2 subscribed channels, got %d", len(client.channels))
	}

	// Broadcast to channel-1 should not appear in channel-2's subscribers
	msg2 := []byte(`{"scope":"channel-2"}`)
	hub.BroadcastToChannel("channel-2", msg2)

	select {
	case received := <-client.send:
		if string(received) != string(msg2) {
			t.Errorf("expected message, got different content")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestHubUnregisterRemovesFromChannels(t *testing.T) {
	hub, _, cleanup := newTestServer(t)
	defer cleanup()

	client := NewClient(hub, nil, "user-1")
	client.send = make(chan []byte, 256)

	hub.Subscribe(client, "channel-1")
	hub.Subscribe(client, "channel-2")

	// Simulate unregister
	hub.unregister <- client
	time.Sleep(50 * time.Millisecond)

	// Client should not be in channels anymore
	hub.mu.RLock()
	for _, ch := range []string{"channel-1", "channel-2"} {
		if subs, ok := hub.channels[ch]; ok {
			if subs[client] {
				hub.mu.RUnlock()
				t.Fatalf("client should not be in %s after unregister", ch)
			}
		}
	}
	hub.mu.RUnlock()

	// Client's send channel should be closed
	_, ok := <-client.send
	if ok {
		t.Error("expected client.send to be closed after unregister")
	}
}

func TestHubMessageEnvelope(t *testing.T) {
	payload := MessageNewPayload{
		ID:          "msg-1",
		ChannelID:   "ch-1",
		SenderType:  "user",
		SenderID:    "user-1",
		SenderName:  "Test User",
		Content:     "Hello, World!",
		ContentType: "text",
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	data := Envelope(EventMessageNew, payload)

	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	if msg.Type != EventMessageNew {
		t.Errorf("expected type %q, got %q", EventMessageNew, msg.Type)
	}

	var decoded MessageNewPayload
	if err := json.Unmarshal(msg.Payload, &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if decoded.ID != "msg-1" {
		t.Errorf("expected id %q, got %q", "msg-1", decoded.ID)
	}
	if decoded.Content != "Hello, World!" {
		t.Errorf("expected content %q, got %q", "Hello, World!", decoded.Content)
	}
}
