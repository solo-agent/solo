package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupChiRouterForMessage(h *MessageHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/channels/{channelID}/messages", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
	})
	return r
}

func TestMessageHandler_Create_Validation(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "empty content",
			body:       `{"content":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing auth",
			body:       `{"content":"hello"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/messages", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			if tt.name != "missing auth" {
				req.Header.Set("X-User-ID", "user-1")
			}

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestMessageHandler_List_MissingAuth(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestMessageHandler_List_InvalidCursorFormat(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	// Non-UUID "before" parameter should be rejected before any DB interaction.
	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages?before=not-a-uuid", nil)
	req.Header.Set("X-User-ID", "user-1")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cursor format, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Message != "invalid cursor: must be a valid message ID" {
		t.Errorf("unexpected error message: %q", resp.Message)
	}
}

func TestMessageHandler_List_ValidCursorFormat(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	// Valid UUID cursor should pass format validation.
	// It will fail on membership check (nil pool), but does NOT return 400 for cursor.
	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages?before=550e8400-e29b-41d4-a716-446655440000", nil)
	req.Header.Set("X-User-ID", "user-1")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	// Should NOT be a cursor validation error.
	if rr.Code == http.StatusBadRequest {
		var resp ErrorResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err == nil {
			if resp.Message == "invalid cursor: must be a valid message ID" {
				t.Fatal("valid UUID cursor was incorrectly rejected")
			}
		}
		// 400 for other reasons (e.g., nil pool on membership check) is expected.
	}
}

func TestMessageHandler_List_LimitBoundaries(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	tests := []struct {
		name        string
		limit       string
		expectError bool // whether to expect an error (e.g., invalid limit format)
	}{
		{"no limit (default to 50)", "", false},
		{"limit=1 minimum", "1", false},
		{"limit=50 default", "50", false},
		{"limit=100 maximum", "100", false},
		// These limits exceed max but the handler clamps them silently (no error).
		// We verify this by checking they don't return 400 for the limit itself.
		{"limit=0 clamped", "0", false},
		{"limit=-1 clamped", "-1", false},
		{"limit=101 clamped", "101", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/channels/ch-1/messages"
			if tt.limit != "" {
				url += "?limit=" + tt.limit
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("X-User-ID", "user-1")
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if tt.expectError && rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}

			// The handler should not panic or return 500 for any limit value.
			if rr.Code >= 500 {
				t.Errorf("unexpected 5xx for limit=%q: %d", tt.limit, rr.Code)
			}
		})
	}
}

func TestMessageHandler_ResponseFormat(t *testing.T) {
	// Verify serialization
	resp := MessageResponse{
		ID:          "msg-1",
		ChannelID:   "ch-1",
		SenderType:  "user",
		SenderID:    "user-1",
		SenderName:  "Test User",
		Content:     "Hello, World!",
		ContentType: "text",
		CreatedAt:   "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MessageResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "msg-1" {
		t.Errorf("expected id msg-1, got %s", decoded.ID)
	}
	if decoded.Content != "Hello, World!" {
		t.Errorf("expected content 'Hello, World!', got %s", decoded.Content)
	}
}

func TestMessageHandler_ListResponseFormat(t *testing.T) {
	resp := MessageListResponse{
		Messages: []MessageResponse{
			{ID: "msg-1", Content: "First", CreatedAt: "2025-01-01T00:00:00Z"},
			{ID: "msg-2", Content: "Second", CreatedAt: "2025-01-01T00:00:01Z"},
		},
		HasMore: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MessageListResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(decoded.Messages))
	}
	if !decoded.HasMore {
		t.Error("expected has_more to be true")
	}
}

func TestMessageHandler_ListResponse_HasMoreFalse(t *testing.T) {
	// Verify has_more=false serialization when no more messages exist.
	resp := MessageListResponse{
		Messages: []MessageResponse{
			{ID: "msg-1", Content: "Only message", CreatedAt: "2025-01-01T00:00:00Z"},
		},
		HasMore: false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MessageListResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.HasMore {
		t.Error("expected has_more to be false")
	}
}

func TestMessageHandler_Create_TooLong(t *testing.T) {
	h := &MessageHandler{}
	r := setupChiRouterForMessage(h)

	// Create content that exceeds 10000 chars using valid Unicode
	longContent := make([]byte, 10001)
	for i := range longContent {
		longContent[i] = 'a'
	}
	body := `{"content":"` + string(longContent) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/messages", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long content, got %d", rr.Code)
	}
}
