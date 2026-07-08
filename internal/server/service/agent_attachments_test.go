package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRenderAgentAttachmentContextInlinesTextAttachment(t *testing.T) {
	got := renderAgentAttachmentContext("please inspect this", []agentAttachmentContext{
		{
			ID:       "att-1",
			Filename: "README.md",
			MIMEType: "text/markdown",
			Size:     42,
			URL:      "/api/v1/attachments/att-1",
			Text:     "# Title\nhello",
		},
	})

	assertContains(t, got, "please inspect this")
	assertContains(t, got, "README.md (text/markdown, 42 B)")
	assertContains(t, got, "url=/api/v1/attachments/att-1")
	assertContains(t, got, "local_path=")
	assertContains(t, got, "Text content preview")
	assertContains(t, got, "   # Title")
	assertContains(t, got, "   hello")
}

func TestRenderAgentAttachmentContextListsNonTextAttachment(t *testing.T) {
	got := renderAgentAttachmentContext("", []agentAttachmentContext{
		{
			ID:       "att-2",
			Filename: "diagram.png",
			MIMEType: "image/png",
			Size:     2048,
			URL:      "/api/v1/attachments/att-2",
		},
	})

	assertContains(t, got, "(no text content)")
	assertContains(t, got, "diagram.png (image/png, 2.0 KB)")
	assertContains(t, got, "Content is not inlined")
}

func TestReadAgentAttachmentTextUsesConfiguredAttachmentRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ATTACHMENTS_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, "2026-07"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "2026-07", "note.txt"), []byte("hello attachment"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, truncated, readErr := readAgentAttachmentText(filepath.Join("2026-07", "note.txt"), 1024)
	if readErr != "" {
		t.Fatalf("readErr = %q", readErr)
	}
	if truncated {
		t.Fatal("expected text not to be truncated")
	}
	if text != "hello attachment" {
		t.Fatalf("text = %q", text)
	}
}

func TestReadAgentAttachmentTextTruncates(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ATTACHMENTS_DIR", root)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, truncated, readErr := readAgentAttachmentText("long.txt", 3)
	if readErr != "" {
		t.Fatalf("readErr = %q", readErr)
	}
	if !truncated {
		t.Fatal("expected text to be truncated")
	}
	if text != "abc" {
		t.Fatalf("text = %q", text)
	}
}

func TestResolveAgentAttachmentPathRejectsTraversal(t *testing.T) {
	t.Setenv("ATTACHMENTS_DIR", t.TempDir())
	if _, err := resolveAgentAttachmentPath("../secret.txt"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if _, err := resolveAgentAttachmentPath("/tmp/secret.txt"); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestIsAgentTextAttachment(t *testing.T) {
	for _, mimeType := range []string{"text/plain", "text/markdown", "text/csv", "application/json"} {
		if !isAgentTextAttachment(mimeType) {
			t.Fatalf("expected %s to be treated as text", mimeType)
		}
	}
	if isAgentTextAttachment("image/png") {
		t.Fatal("image/png should not be treated as text")
	}
}

func TestGetRecentMessagesIncludesAttachmentContext(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("ATTACHMENTS_DIR", root)

	ownerID := agentRunUser(t, pool)
	channelID := agentRunChannel(t, pool, ownerID)
	attachmentID := uuid.NewString()
	messageID := uuid.NewString()
	storagePath := filepath.Join("2026-07", "note.txt")
	fullPath := filepath.Join(root, storagePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("agent-visible attachment content"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM attachments WHERE id = $1`, attachmentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx,
		`INSERT INTO attachments (id, user_id, filename, mime_type, size, storage_path)
		 VALUES ($1, $2, 'note.txt', 'text/plain', 32, $3)`,
		attachmentID, ownerID, storagePath,
	); err != nil {
		t.Fatalf("insert attachment: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, attachment_ids)
		 VALUES ($1, $2, 'user', $3, 'please read attached file', $4::uuid[])`,
		messageID, channelID, ownerID, "{"+attachmentID+"}",
	); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	svc := NewAgentService(pool, nil, noopBroadcaster{}, nil)
	msgs, err := svc.getRecentMessages(ctx, channelID, 1)
	if err != nil {
		t.Fatalf("getRecentMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	assertContains(t, msgs[0].Content, "please read attached file")
	assertContains(t, msgs[0].Content, "note.txt (text/plain")
	assertContains(t, msgs[0].Content, ".solo/attachments/")
	assertContains(t, msgs[0].Content, "agent-visible attachment content")
	if len(msgs[0].Attachments) != 1 {
		t.Fatalf("len(attachments) = %d, want 1", len(msgs[0].Attachments))
	}
	if msgs[0].Attachments[0].LocalPath == "" {
		t.Fatal("attachment local path is empty")
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}
