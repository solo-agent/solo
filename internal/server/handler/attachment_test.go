package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/auth"
)

func TestAttachmentRejectsBearerTokenInQuery(t *testing.T) {
	h := &AttachmentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/example?access_token=secret", nil)
	recorder := httptest.NewRecorder()
	h.Serve(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAgentRunAttachmentTokenIsLimitedToRunChannel(t *testing.T) {
	pool := attachmentTestPool(t)
	ctx := context.Background()
	ownerID := uuid.NewString()
	agentID := uuid.NewString()
	computerID := uuid.NewString()
	runChannelID := uuid.NewString()
	otherChannelID := uuid.NewString()
	runID := uuid.NewString()
	attachmentID := uuid.NewString()
	email := fmt.Sprintf("attachment-%s@example.test", ownerID)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, 'Attachment Test', 'test')`, ownerID, email); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM attachments WHERE id = $1`, attachmentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id IN ($1, $2)`, runChannelID, otherChannelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO channels (id, name, created_by) VALUES ($1, $3, $2), ($4, $5, $2)`, runChannelID, ownerID, "run-"+ownerID, otherChannelID, "other-"+ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id, name, owner_id, model_name, home_channel_id, runtime_id) VALUES ($1, $2, $3, 'test', $4, $5)`, agentID, "attachment-agent-"+agentID[:8], ownerID, runChannelID, computerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO computers (id, name, owner_id) VALUES ($1, 'attachment-computer', $2)`, computerID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attachments (id, user_id, filename, mime_type, size, storage_path) VALUES ($1, $2, 'secret.txt', 'text/plain', 6, 'secret.txt')`, attachmentID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (channel_id, sender_type, sender_id, content, attachment_ids) VALUES ($1, 'user', $2, 'other', ARRAY[$3]::uuid[])`, otherChannelID, ownerID, attachmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agent_runs (id, agent_id, trigger_type, channel_id, status, computer_id) VALUES ($1, $2, 'message', $3, 'running', $4)`, runID, agentID, runChannelID, computerID); err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateAgentRunToken(agentID, "Agent", runID, computerID)
	if err != nil {
		t.Fatal(err)
	}
	authorized := func() bool {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/"+attachmentID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		route := chi.NewRouteContext()
		route.URLParams.Add("attachmentID", attachmentID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
		return (&AttachmentHandler{pool: pool}).authorize(req)
	}
	if authorized() {
		t.Fatal("Run token read an attachment from another channel")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (channel_id, sender_type, sender_id, content, attachment_ids) VALUES ($1, 'user', $2, 'run', ARRAY[$3]::uuid[])`, runChannelID, ownerID, attachmentID); err != nil {
		t.Fatal(err)
	}
	if !authorized() {
		t.Fatal("Run token could not read an attachment from its own channel")
	}
}

func attachmentTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestDetectMIMEType_CommonAttachmentFormats(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"report.pdf", "application/pdf"},
		{"notes.md", "text/markdown"},
		{"brief.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"archive.7z", "application/x-7z-compressed"},
		{"bundle.rar", "application/vnd.rar"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := detectMIMEType(tt.filename); got != tt.want {
				t.Fatalf("detectMIMEType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
			if !isAllowedMIMEType(tt.want) {
				t.Fatalf("expected MIME type %q to be allowed", tt.want)
			}
		})
	}
}

func TestNormalizeMIMEType(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		filename string
		want     string
	}{
		{
			name:     "keeps specific header",
			raw:      "application/pdf",
			filename: "report.pdf",
			want:     "application/pdf",
		},
		{
			name:     "strips parameters",
			raw:      "text/plain; charset=utf-8",
			filename: "notes.txt",
			want:     "text/plain",
		},
		{
			name:     "falls back from octet stream",
			raw:      "application/octet-stream",
			filename: "brief.docx",
			want:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:     "falls back from non-standard zip header",
			raw:      "application/x-zip-compressed",
			filename: "archive.zip",
			want:     "application/x-zip-compressed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMIMEType(tt.raw, tt.filename); got != tt.want {
				t.Fatalf("normalizeMIMEType(%q, %q) = %q, want %q", tt.raw, tt.filename, got, tt.want)
			}
		})
	}
}
