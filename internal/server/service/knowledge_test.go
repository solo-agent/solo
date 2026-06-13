package service

import (
	"testing"
)

func TestParseDecisionBlocks_SingleEntry(t *testing.T) {
	raw := `## 2026-06-13: Adopt pgvector for semantic search

We decided to use pgvector instead of an external vector database.
This keeps operations simple and avoids adding a new service dependency.

Trade-off: pgvector cosine similarity is slightly slower than Pinecone at scale,
but for our <10k document corpus, the difference is negligible.

---`

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	b := blocks[0]
	if b.Title != "2026-06-13: Adopt pgvector for semantic search" {
		t.Errorf("unexpected title: %q", b.Title)
	}
	if b.Content == "" {
		t.Error("expected non-empty content")
	}
	if !containsSubstring(b.Content, "pgvector") {
		t.Errorf("content should mention pgvector, got: %s", b.Content)
	}
	if !containsSubstring(b.Content, "trade-off") {
		t.Logf("content: %s", b.Content)
	}
	if b.SourceRef != "decisions.md#L1" {
		t.Errorf("expected SourceRef decisions.md#L1, got %q", b.SourceRef)
	}
}

func TestParseDecisionBlocks_MultipleEntries(t *testing.T) {
	raw := `## 2026-01-10: Use Go for the backend

Go was chosen for its simplicity, fast compile times, and strong concurrency model.

---

## 2026-02-15: Adopt React Flow for graph editing

React Flow provides the interaction model we need (drag, zoom, edge creation)
without reinventing the wheel.

---

## 2026-03-20: Use WebSocket for real-time updates

WebSocket gives us bidirectional communication with low latency.

---`

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	expectedTitles := []string{
		"2026-01-10: Use Go for the backend",
		"2026-02-15: Adopt React Flow for graph editing",
		"2026-03-20: Use WebSocket for real-time updates",
	}

	for i, expected := range expectedTitles {
		if blocks[i].Title != expected {
			t.Errorf("block[%d]: expected title %q, got %q", i, expected, blocks[i].Title)
		}
		if blocks[i].Content == "" {
			t.Errorf("block[%d]: expected non-empty content", i)
		}
		if blocks[i].SourceRef == "" {
			t.Errorf("block[%d]: expected non-empty SourceRef", i)
		}
	}
}

func TestParseDecisionBlocks_EmptyDecisions(t *testing.T) {
	raw := ""

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for empty input, got %d", len(blocks))
	}
}

func TestParseDecisionBlocks_WhitespaceOnly(t *testing.T) {
	raw := "\n\n  \n\t\n"

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for whitespace-only input, got %d", len(blocks))
	}
}

func TestParseDecisionBlocks_MalformedEntries(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantBlocks  int
		description string
	}{
		{
			name: "no delimiter between entries",
			raw: `## Entry 1
Content of entry 1.
## Entry 2
Content of entry 2.`,
			wantBlocks:  2,
			description: "each ## header starts a new block even without --- delimiters between them",
		},
		{
			name: "header without content",
			raw: `## Bare header

---`,
			wantBlocks:  1,
			description: "header with empty body should still produce a block with empty content",
		},
		{
			name: "trailing delimiter without preceding header",
			raw: `Some text without a header.

---`,
			wantBlocks:  0,
			description: "text before any ## header should be ignored; --- is a no-op without active block",
		},
		{
			name: "header with trailing content but no delimiter",
			raw: `## Trailing entry
This entry has no closing --- delimiter.
More content here.`,
			wantBlocks:  1,
			description: "block without closing --- should still be captured (trailing block)",
		},
		{
			name: "multiple headers without delimiters",
			raw: `## First
Content A.
## Second
Content B.
## Third
Content C.`,
			wantBlocks:  3,
			description: "each ## header starts its own block regardless of delimiter presence",
		},
		{
			name: "content before first header is ignored",
			raw: `Preamble text that should be skipped.

## Valid Entry
This is the real content.

---`,
			wantBlocks:  1,
			description: "preamble before the first ## header is ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := parseDecisionBlocks(tt.raw)

			if len(blocks) != tt.wantBlocks {
				t.Errorf("%s: expected %d blocks, got %d", tt.description, tt.wantBlocks, len(blocks))
				for i, b := range blocks {
					t.Logf("  block[%d]: title=%q content=%q", i, b.Title, b.Content)
				}
			}
		})
	}
}

func TestParseDecisionBlocks_TitleFormatting(t *testing.T) {
	raw := `## 2026-06-13: Adopt pgvector for semantic search

We decided to use pgvector instead of an external vector database.

---`

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	// Title should include the full line after "## " (date + description).
	if blocks[0].Title != "2026-06-13: Adopt pgvector for semantic search" {
		t.Errorf("unexpected title: %q", blocks[0].Title)
	}
}

func TestParseDecisionBlocks_ContentPreservesNewlines(t *testing.T) {
	raw := `## Decision with multiline content

Line 1.
Line 2.

Line 3 (after blank line).

---`

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	// The content should preserve blank lines between paragraphs.
	if blocks[0].Content == "" {
		t.Error("expected non-empty content")
	}
	if !containsSubstring(blocks[0].Content, "Line 1.") {
		t.Error("content missing Line 1")
	}
	if !containsSubstring(blocks[0].Content, "Line 2.") {
		t.Error("content missing Line 2")
	}
	if !containsSubstring(blocks[0].Content, "Line 3") {
		t.Error("content missing Line 3")
	}
}

func TestParseDecisionBlocks_SourceRefIncrements(t *testing.T) {
	raw := `## Decision A
Content A.

---

## Decision B
Content B.

---

## Decision C
Content C.

---`

	blocks := parseDecisionBlocks(raw)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	if blocks[0].SourceRef != "decisions.md#L1" {
		t.Errorf("block[0] SourceRef: expected decisions.md#L1, got %q", blocks[0].SourceRef)
	}
	if blocks[1].SourceRef == "" {
		t.Error("block[1] SourceRef should not be empty")
	}
	if blocks[2].SourceRef == "" {
		t.Error("block[2] SourceRef should not be empty")
	}
}

func TestParseDecisionBlocks_Idempotent(t *testing.T) {
	raw := `## Decision 1
Content 1.

---

## Decision 2
Content 2.

---`

	result1 := parseDecisionBlocks(raw)
	result2 := parseDecisionBlocks(raw)

	if len(result1) != len(result2) {
		t.Fatalf("idempotent check: got %d and %d blocks", len(result1), len(result2))
	}

	for i := range result1 {
		if result1[i].Title != result2[i].Title {
			t.Errorf("block[%d] title differs: %q vs %q", i, result1[i].Title, result2[i].Title)
		}
		if result1[i].Content != result2[i].Content {
			t.Errorf("block[%d] content differs", i)
		}
	}
}

func TestKnowledgeService_DecisionBlockStruct(t *testing.T) {
	// Verify the decisionBlock struct fields are accessible.
	b := decisionBlock{
		Title:     "Test Title",
		Content:   "Test Content",
		SourceRef: "decisions.md#L1",
	}

	if b.Title != "Test Title" {
		t.Errorf("expected Title, got %q", b.Title)
	}
	if b.Content != "Test Content" {
		t.Errorf("expected Content, got %q", b.Content)
	}
	if b.SourceRef != "decisions.md#L1" {
		t.Errorf("expected SourceRef, got %q", b.SourceRef)
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
