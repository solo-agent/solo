package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupChiRouterForThread(h *ThreadHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/channels/{channelID}/messages/{messageID}/thread", h.ListThreadMessages)
	r.Post("/api/v1/channels/{channelID}/messages/{messageID}/thread", h.CreateThreadReply)
	return r
}

func TestThreadHandler_CreateReply_Validation(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

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
		{
			name:       "empty body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/messages/msg-1/thread", bytes.NewBufferString(tt.body))
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

func TestThreadHandler_List_MissingAuth(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages/msg-1/thread", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestThreadHandler_CreateReply_TooLong(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

	longContent := make([]byte, 10001)
	for i := range longContent {
		longContent[i] = 'a'
	}
	body := `{"content":"` + string(longContent) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/messages/msg-1/thread", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long content, got %d", rr.Code)
	}
}

func TestThreadHandler_List_InvalidCursorFormat(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

	// Non-UUID "before" parameter should be rejected before any DB interaction.
	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages/msg-1/thread?before=not-a-uuid", nil)
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

func TestThreadHandler_List_ValidCursorFormat(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

	// Valid UUID cursor should pass format validation.
	// It will fail on membership check (nil pool), but does NOT return 400 for cursor.
	req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/messages/msg-1/thread?before=550e8400-e29b-41d4-a716-446655440000", nil)
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

func TestThreadHandler_List_LimitBoundaries(t *testing.T) {
	h := &ThreadHandler{}
	r := setupChiRouterForThread(h)

	tests := []struct {
		name        string
		limit       string
		expectError bool
	}{
		{"no limit (default to 50)", "", false},
		{"limit=1 minimum", "1", false},
		{"limit=50 default", "50", false},
		{"limit=100 maximum", "100", false},
		{"limit=0 clamped", "0", false},
		{"limit=-1 clamped", "-1", false},
		{"limit=101 clamped", "101", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/channels/ch-1/messages/msg-1/thread"
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

			if rr.Code >= 500 {
				t.Errorf("unexpected 5xx for limit=%q: %d", tt.limit, rr.Code)
			}
		})
	}
}

func TestThreadHandler_ResponseFormat(t *testing.T) {
	resp := ThreadReplyResponse{
		ID:          "reply-1",
		ChannelID:   "ch-1",
		ThreadID:    "thread-1",
		SenderType:  "user",
		SenderID:    "user-1",
		SenderName:  "Test User",
		Content:     "This is a thread reply!",
		ContentType: "text",
		CreatedAt:   "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ThreadReplyResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "reply-1" {
		t.Errorf("expected id reply-1, got %s", decoded.ID)
	}
	if decoded.ThreadID != "thread-1" {
		t.Errorf("expected thread_id thread-1, got %s", decoded.ThreadID)
	}
	if decoded.Content != "This is a thread reply!" {
		t.Errorf("expected content 'This is a thread reply!', got %s", decoded.Content)
	}
}

func TestThreadHandler_ThreadMessageListResponseFormat(t *testing.T) {
	resp := ThreadMessageListResponse{
		Messages: []ThreadReplyResponse{
			{ID: "reply-1", ThreadID: "thread-1", Content: "First reply", CreatedAt: "2025-01-01T00:00:00Z"},
			{ID: "reply-2", ThreadID: "thread-1", Content: "Second reply", CreatedAt: "2025-01-01T00:00:01Z"},
		},
		HasMore: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ThreadMessageListResponse
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

func TestThreadHandler_ThreadResponseFormat(t *testing.T) {
	resp := ThreadResponse{
		ID:            "thread-1",
		ChannelID:     "ch-1",
		RootMessageID: "msg-1",
		ReplyCount:    5,
		LastReplyAt:   "2025-01-01T00:00:00Z",
		CreatedAt:     "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ThreadResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "thread-1" {
		t.Errorf("expected id thread-1, got %s", decoded.ID)
	}
	if decoded.ReplyCount != 5 {
		t.Errorf("expected reply_count 5, got %d", decoded.ReplyCount)
	}
	if decoded.RootMessageID != "msg-1" {
		t.Errorf("expected root_message_id msg-1, got %s", decoded.RootMessageID)
	}
}
