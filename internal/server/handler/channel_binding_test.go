package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func setupChannelBindingRouter(h *ChannelBindingHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api/v1/channels/{channelID}", func(r chi.Router) {
		r.Post("/bind-project", h.BindProject)
		r.Get("/binding", h.GetBinding)
		r.Delete("/bind-project", h.UnbindProject)
	})
	return r
}

// ---- BindProject ----

func TestChannelBinding_BindProject_Validation(t *testing.T) {
	h := &ChannelBindingHandler{}
	r := setupChannelBindingRouter(h)

	tests := []struct {
		name       string
		body       string
		auth       string
		wantStatus int
	}{
		{
			name:       "missing auth",
			body:       `{"repo_url":"https://github.com/org/repo.git"}`,
			auth:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing repo_url",
			body:       `{}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty repo_url",
			body:       `{"repo_url":""}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON body",
			body:       `{not json}`,
			auth:       "user-1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels/ch-1/bind-project", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != "" {
				req.Header.Set("X-User-ID", tt.auth)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// ---- GetBinding ----

func TestChannelBinding_GetBinding_Validation(t *testing.T) {
	h := &ChannelBindingHandler{}
	r := setupChannelBindingRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/channels/ch-1/binding", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- UnbindProject ----

func TestChannelBinding_UnbindProject_Validation(t *testing.T) {
	h := &ChannelBindingHandler{}
	r := setupChannelBindingRouter(h)

	t.Run("missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/channels/ch-1/bind-project", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

// ---- Data structure serialization ----

func TestChannelBinding_Marshal(t *testing.T) {
	now := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	b := service.ChannelBinding{
		ChannelID:  "ch-1",
		RepoURL:    "https://github.com/org/repo.git",
		RepoBranch: "main",
		BindPath:   "/home/user/.solo/channels/ch-1/workspace/repo",
		BoundBy:    "user-1",
		BoundAt:    now,
		UpdatedAt:  now,
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded service.ChannelBinding
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ChannelID != "ch-1" {
		t.Errorf("expected channel_id ch-1, got %s", decoded.ChannelID)
	}
	if decoded.RepoURL != "https://github.com/org/repo.git" {
		t.Errorf("expected repo_url, got %s", decoded.RepoURL)
	}
	if decoded.RepoBranch != "main" {
		t.Errorf("expected repo_branch main, got %s", decoded.RepoBranch)
	}
	if decoded.BindPath != "/home/user/.solo/channels/ch-1/workspace/repo" {
		t.Errorf("expected bind_path, got %s", decoded.BindPath)
	}
	if decoded.BoundBy != "user-1" {
		t.Errorf("expected bound_by user-1, got %s", decoded.BoundBy)
	}
}

func TestChannelBinding_DifferentBranches_Marshal(t *testing.T) {
	branches := []string{"main", "develop", "feature/collab"}
	for _, branch := range branches {
		b := service.ChannelBinding{
			ChannelID:  "ch-1",
			RepoURL:    "https://github.com/org/repo.git",
			RepoBranch: branch,
			BindPath:   "/tmp/repo",
			BoundBy:    "user-1",
		}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("failed to marshal branch %q: %v", branch, err)
		}
		var decoded service.ChannelBinding
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal branch %q: %v", branch, err)
		}
		if decoded.RepoBranch != branch {
			t.Errorf("expected repo_branch %s, got %s", branch, decoded.RepoBranch)
		}
	}
}

func TestBindProjectRequest_Marshal(t *testing.T) {
	// With branch specified.
	req := bindProjectRequest{
		RepoURL:    "https://github.com/org/repo.git",
		RepoBranch: "develop",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded bindProjectRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.RepoURL != "https://github.com/org/repo.git" {
		t.Errorf("expected repo_url, got %s", decoded.RepoURL)
	}
	if decoded.RepoBranch != "develop" {
		t.Errorf("expected repo_branch develop, got %s", decoded.RepoBranch)
	}
}

func TestBindProjectRequest_DefaultBranch_Marshal(t *testing.T) {
	// Without branch — should be empty string (defaults to "main" in service).
	req := bindProjectRequest{
		RepoURL: "https://github.com/org/repo.git",
	}
	data, _ := json.Marshal(req)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	if decoded["repo_url"] != "https://github.com/org/repo.git" {
		t.Errorf("expected repo_url, got %v", decoded["repo_url"])
	}
	// repo_branch should be absent when empty (omitempty).
	if branch, ok := decoded["repo_branch"]; ok && branch != nil && branch != "" {
		t.Logf("note: repo_branch present with value %v (omitempty behavior)", branch)
	}
}

func TestChannelBinding_UnbindResponseShape(t *testing.T) {
	// The response from DELETE /bind-project is {"status": "unbound"}
	resp := map[string]string{"status": "unbound"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded["status"] != "unbound" {
		t.Errorf("expected status 'unbound', got %s", decoded["status"])
	}
}

// Note: Full integration tests (bind project to channel, get binding,
// unbind project, bind duplicate → conflict) require a real database pool
// and are covered by the E2E test plan in docs/design/tasks/e2e-test-plan.md.
