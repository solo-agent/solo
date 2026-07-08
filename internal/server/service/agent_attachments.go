package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/solo-ai/solo/pkg/agent"
)

const maxAgentAttachmentTextBytes = 16 * 1024

type agentAttachmentContext struct {
	ID            string
	Filename      string
	MIMEType      string
	Size          int64
	StoragePath   string
	URL           string
	LocalPath     string
	Text          string
	TextTruncated bool
	ReadError     string
}

func (s *AgentService) enrichMessageContentWithAttachments(ctx context.Context, content string, attachmentIDs []string) string {
	content, _ = s.enrichMessageContentAndAttachments(ctx, content, attachmentIDs)
	return content
}

func (s *AgentService) enrichMessageContentAndAttachments(ctx context.Context, content string, attachmentIDs []string) (string, []agent.Attachment) {
	if len(attachmentIDs) == 0 {
		return content, nil
	}

	attachments, err := s.loadAgentAttachmentContexts(ctx, attachmentIDs)
	if err != nil {
		slog.Warn("failed to load attachment context for agent prompt", "error", err)
		return content, nil
	}
	if len(attachments) == 0 {
		return content, nil
	}
	return renderAgentAttachmentContext(content, attachments), toAgentMessageAttachments(attachments)
}

func (s *AgentService) loadAgentAttachmentContexts(ctx context.Context, ids []string) ([]agentAttachmentContext, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, filename, mime_type, size, storage_path
		   FROM attachments
		  WHERE id = ANY($1::uuid[])`,
		ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[string]agentAttachmentContext, len(ids))
	for rows.Next() {
		var a agentAttachmentContext
		if err := rows.Scan(&a.ID, &a.Filename, &a.MIMEType, &a.Size, &a.StoragePath); err != nil {
			return nil, err
		}
		a.URL = "/api/v1/attachments/" + a.ID
		a.LocalPath = agent.AttachmentLocalPath(a.ID, a.Filename)
		if isAgentTextAttachment(a.MIMEType) {
			a.Text, a.TextTruncated, a.ReadError = readAgentAttachmentText(a.StoragePath, maxAgentAttachmentTextBytes)
		}
		byID[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]agentAttachmentContext, 0, len(ids))
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			result = append(result, a)
		}
	}
	return result, nil
}

func renderAgentAttachmentContext(content string, attachments []agentAttachmentContext) string {
	var b strings.Builder
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		b.WriteString("(no text content)")
	} else {
		b.WriteString(content)
	}

	b.WriteString("\n\nAttachments visible to this message:\n")
	for i, a := range attachments {
		fmt.Fprintf(&b, "%d. %s (%s, %s) url=%s local_path=%s\n", i+1, a.Filename, emptyFallback(a.MIMEType, "unknown"), formatAgentAttachmentSize(a.Size), a.URL, a.LocalPath)
		if a.Text != "" {
			b.WriteString("   Text content preview")
			if a.TextTruncated {
				fmt.Fprintf(&b, " (first %d bytes, truncated)", maxAgentAttachmentTextBytes)
			}
			b.WriteString(":\n")
			b.WriteString(indentAgentAttachmentText(a.Text))
			if !strings.HasSuffix(a.Text, "\n") {
				b.WriteString("\n")
			}
			continue
		}
		if a.ReadError != "" {
			fmt.Fprintf(&b, "   Text content could not be read: %s\n", a.ReadError)
			continue
		}
		b.WriteString("   Content is not inlined. Use the URL above or Solo tools if you need to inspect this attachment.\n")
	}
	return b.String()
}

func toAgentMessageAttachments(attachments []agentAttachmentContext) []agent.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]agent.Attachment, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, agent.Attachment{
			ID:          a.ID,
			Filename:    a.Filename,
			MIMEType:    a.MIMEType,
			Size:        a.Size,
			URL:         a.URL,
			StoragePath: a.StoragePath,
			LocalPath:   a.LocalPath,
		})
	}
	return out
}

func readAgentAttachmentText(storagePath string, limit int64) (string, bool, string) {
	fullPath, err := resolveAgentAttachmentPath(storagePath)
	if err != nil {
		return "", false, err.Error()
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return "", false, err.Error()
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return "", false, err.Error()
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return string(data), truncated, ""
}

func resolveAgentAttachmentPath(storagePath string) (string, error) {
	if filepath.IsAbs(storagePath) {
		return "", fmt.Errorf("invalid attachment path")
	}
	root := agentAttachmentsRoot()
	fullPath := filepath.Join(root, filepath.Clean(storagePath))
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid attachment path")
	}
	return fullPath, nil
}

func agentAttachmentsRoot() string {
	if dir := os.Getenv("ATTACHMENTS_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".solo", "attachments")
	}
	return filepath.Join(".", "attachments")
}

func isAgentTextAttachment(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json",
		"application/xml",
		"application/yaml",
		"application/x-yaml",
		"application/toml":
		return true
	default:
		return false
	}
}

func indentAgentAttachmentText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "   " + line
	}
	return strings.Join(lines, "\n")
}

func formatAgentAttachmentSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
