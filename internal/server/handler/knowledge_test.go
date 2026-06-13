package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupKnowledgeRouter(h *KnowledgeHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/knowledge", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/search", h.Search)
		r.Post("/import-decisions", h.ImportDecisions)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Patch("/", h.Update)
			r.Delete("/", h.Delete)
		})
	})
	return r
}

func TestKnowledgeHandler_Create_Validation(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{name: "missing auth", body: `{"channel_id":"ch-1","title":"Test","content":"Content"}`, auth: "", wantStatus: http.StatusUnauthorized},
		{name: "missing title", body: `{"channel_id":"ch-1","content":"Content"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "missing content", body: `{"channel_id":"ch-1","title":"Test"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "missing channel_id", body: `{"title":"Test","content":"Content"}`, auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "empty body", body: `{}`, auth: "user-1", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/knowledge", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != "" {
				req.Header.Set("X-User-ID", tt.auth)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestKnowledgeHandler_Get_Validation(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/knowledge/test-id", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})
}

func TestKnowledgeHandler_Delete_Validation(t *testing.T) {
	h := &KnowledgeHandler{}

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/knowledge/test-id", nil)
		rr := httptest.NewRecorder()
		h.Delete(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestKnowledgeHandler_Search_Validation(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	tests := []struct {
		name       string
		query      string
		channelID  string
		auth       string
		wantStatus int
	}{
		{name: "missing auth", query: "test", channelID: "ch-1", auth: "", wantStatus: http.StatusUnauthorized},
		{name: "missing query", query: "", channelID: "ch-1", auth: "user-1", wantStatus: http.StatusBadRequest},
		{name: "missing channel_id", query: "test", channelID: "", auth: "user-1", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/knowledge/search"
			appendQ := func(k, v string) {
				if v == "" {
					return
				}
				if url == "/api/v1/knowledge/search" {
					url += "?"
				} else {
					url += "&"
				}
				url += k + "=" + v
			}
			appendQ("q", tt.query)
			appendQ("channel_id", tt.channelID)
			req := httptest.NewRequest("GET", url, nil)
			if tt.auth != "" {
				req.Header.Set("X-User-ID", tt.auth)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestKnowledgeHandler_List_Validation(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	t.Run("missing channel_id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/knowledge", nil)
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing channel_id, got %d", rr.Code)
		}
	})
}

func TestKnowledgeHandler_ImportDecisions_Validation(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	t.Run("missing channel_id", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/knowledge/import-decisions", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing channel_id, got %d", rr.Code)
		}
	})

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/knowledge/import-decisions", bytes.NewBufferString(`{"channel_id":"ch-1"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestKnowledgeHandler_ResponseShape(t *testing.T) {
	entry := knowledgeResponse{
		ID:        "test-id",
		ChannelID: "ch-1",
		Title:     "Test Title",
		Content:   "Test Content",
		Tags:      []string{"tag1"},
		Source:    "manual",
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-01-01T00:00:00Z",
	}

	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal knowledge response: %v", err)
	}
	var decoded knowledgeResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("failed to unmarshal knowledge response: %v", err)
	}
	if decoded.ID != "test-id" {
		t.Errorf("expected test-id, got %s", decoded.ID)
	}
}
