package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestChannelHandler_Create_Validation(t *testing.T) {
	// Test without a real DB — we test validation logic that fails before DB calls
	h := &ChannelHandler{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "empty request body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "name too long",
			body:       `{"name":"` + string(make([]byte, 101)) + `"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels", bytes.NewBufferString(tt.body))
			req.Header.Set("X-User-ID", "user-1")
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			h.Create(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestChannelHandler_Get_MissingAuth(t *testing.T) {
	h := &ChannelHandler{}

	req := httptest.NewRequest("GET", "/api/v1/channels/test-id", nil)
	rr := httptest.NewRecorder()

	// Use chi URL params
	h.Get(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestChannelHandler_Create_EmptyName(t *testing.T) {
	h := &ChannelHandler{}

	body := `{"name":""}`
	req := httptest.NewRequest("POST", "/api/v1/channels", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "channel name is required" {
		t.Errorf("expected 'channel name is required', got %q", resp.Message)
	}
}

func TestChannelHandler_Delete_MissingAuth(t *testing.T) {
	h := &ChannelHandler{}

	req := httptest.NewRequest("DELETE", "/api/v1/channels/test-id", nil)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestChannelHandler_ResponseFormat(t *testing.T) {
	// Verify the channel response structure serializes correctly
	resp := ChannelResponse{
		ID:          "ch-1",
		Name:        "general",
		Description: "General discussion",
		Type:        "channel",
		CreatedBy:   "user-1",
		IsArchived:  false,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ChannelResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "ch-1" {
		t.Errorf("expected id ch-1, got %s", decoded.ID)
	}
	if decoded.Name != "general" {
		t.Errorf("expected name general, got %s", decoded.Name)
	}
	if decoded.Type != "channel" {
		t.Errorf("expected type channel, got %s", decoded.Type)
	}
}

func TestChannelHandler_PatchContext_Validation(t *testing.T) {
	h := &ChannelHandler{}
	r := chi.NewRouter()
	r.Patch("/api/v1/channels/{channelID}/context", h.PatchContext)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing fields", body: `{}`},
		{name: "agenda object", body: `{"agenda":{}}`},
		{name: "invalid json", body: `{"agenda":[}`},
		{name: "target too long", body: `{"target":"` + string(make([]byte, 10001)) + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PATCH", "/api/v1/channels/ch-1/context", bytes.NewBufferString(tt.body))
			req.Header.Set("X-User-ID", "user-1")
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}
