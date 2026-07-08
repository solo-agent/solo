package handler

import "testing"

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
