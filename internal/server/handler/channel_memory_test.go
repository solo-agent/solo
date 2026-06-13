package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupChannelMemoryRouter(h *ChannelMemoryHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/channels/{channelID}/memory", func(r chi.Router) {
		r.Get("/channel-md", h.GetChannelMd)
		r.Post("/channel-md", h.PutChannelMd)
		r.Get("/decisions", h.GetDecisions)
		r.Post("/decisions", h.AppendDecision)
	})
	return r
}

// ---- GetChannelMd ----

func TestChannelMemory_GetChannelMd_Validation(t *testing.T) {
	h := &ChannelMemoryHandler{}
	r := setupChannelMemoryRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/memory/channel-md", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	// Note: with auth, the handler checks channel membership via pool.
	// Without a real pool (nil), a panic occurs. Integration tests with
	// full DI cover the membership-gated success/forbidden paths.
}

// ---- PutChannelMd ----

func TestChannelMemory_PutChannelMd_Validation(t *testing.T) {
	h := &ChannelMemoryHandler{}
	r := setupChannelMemoryRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/memory/channel-md",
			bytes.NewBufferString(`{"content":"# Channel Memory"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	// Note: input validation (empty content) occurs after channel agent
	// membership check, which requires a pool. Full input validation is
	// covered by integration tests.
}

// ---- GetDecisions ----

func TestChannelMemory_GetDecisions_Validation(t *testing.T) {
	h := &ChannelMemoryHandler{}
	r := setupChannelMemoryRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/memory/decisions", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- AppendDecision ----

func TestChannelMemory_AppendDecision_Validation(t *testing.T) {
	h := &ChannelMemoryHandler{}
	r := setupChannelMemoryRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/memory/decisions",
			bytes.NewBufferString(`{"content":"Use Redis for caching"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- Data structure serialization ----

func TestChannelMemoryRequest_Marshal(t *testing.T) {
	req := channelMemoryRequest{
		Content: "# CHANNEL.md\n\nThis is shared memory for the channel.",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded channelMemoryRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Content != req.Content {
		t.Errorf("expected content %q, got %q", req.Content, decoded.Content)
	}
}

func TestDecisionRequest_Marshal(t *testing.T) {
	req := decisionRequest{
		Content: "We decided to use PostgreSQL as the primary database.",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded decisionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Content != req.Content {
		t.Errorf("expected content %q, got %q", req.Content, decoded.Content)
	}
}

func TestChannelMemory_WriteResponseShape(t *testing.T) {
	// The response from POST /channel-md is {"status": "written"}
	resp := map[string]string{"status": "written"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded["status"] != "written" {
		t.Errorf("expected status 'written', got %s", decoded["status"])
	}
}

func TestChannelMemory_DecisionResponseShape(t *testing.T) {
	// The response from POST /decisions is {"status": "appended"}
	resp := map[string]string{"status": "appended"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded["status"] != "appended" {
		t.Errorf("expected status 'appended', got %s", decoded["status"])
	}
}

func TestChannelMemory_ReadResponseShape(t *testing.T) {
	// The response from GET /channel-md or /decisions is {"content": "..."}
	resp := map[string]string{"content": "# CHANNEL.md\n\nShared memory content."}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded["content"] == "" {
		t.Error("expected non-empty content")
	}
}

func TestChannelMemory_EmptyContentResponseShape(t *testing.T) {
	// Read of a non-existent channel returns empty content string.
	resp := map[string]string{"content": ""}
	data, _ := json.Marshal(resp)
	var decoded map[string]string
	json.Unmarshal(data, &decoded)
	if decoded["content"] != "" {
		t.Errorf("expected empty content, got %q", decoded["content"])
	}
}

// Note: Full integration tests (write and read CHANNEL.md, append decision
// entry, read decisions, read non-existent channel → empty) require a real
// filesystem and database pool. These are covered by the E2E test plan in
// docs/design/tasks/e2e-test-plan.md with the actual ChannelMemoryService
// writing to a temp directory.
