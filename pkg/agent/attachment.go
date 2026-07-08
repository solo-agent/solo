package agent

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Attachment is message attachment metadata passed to agent runtimes.
type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	MIMEType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url,omitempty"`
	StoragePath string `json:"storage_path,omitempty"`
	LocalPath   string `json:"local_path,omitempty"`
}

// AttachmentLocalPath returns the relative path where an attachment should be
// materialized inside an agent workspace.
func AttachmentLocalPath(id, filename string) string {
	safeName := sanitizeAttachmentFilename(filename)
	prefix := id
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	if prefix == "" {
		prefix = "attachment"
	}
	return filepath.ToSlash(filepath.Join(".solo", "attachments", prefix+"-"+safeName))
}

func sanitizeAttachmentFilename(filename string) string {
	base := filepath.Base(strings.TrimSpace(filename))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "attachment"
	}

	var b strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "attachment"
	}
	return out
}
