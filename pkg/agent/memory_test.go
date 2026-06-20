package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testMemoryPath(basePath, agentID string) string {
	return filepath.Join(basePath, agentID, "workspace", "MEMORY.md")
}

func TestLoad_NonExistent(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load on non-existent file failed: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string for non-existent file, got %q", content)
	}
}

func TestLoad_EmptyID(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	_, err := mm.Load("")
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestAppend_NewFile(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Append("agent-1", "Learned that user likes Go.")
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify the file was created.
	memoryPath := testMemoryPath(basePath, "agent-1")
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		t.Fatal("MEMORY.md was not created")
	}

	// Verify content includes the header and the entry.
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("read MEMORY.md failed: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "# Agent Memory") {
		t.Error("expected header")
	}
	if !strings.Contains(content, "Last updated:") {
		t.Error("expected 'Last updated' line")
	}
	if !strings.Contains(content, "Learned that user likes Go.") {
		t.Error("expected appended content")
	}
	if !strings.Contains(content, "## Recent Conversations") {
		t.Error("expected Recent Conversations section")
	}
}

func TestAppend_Existing(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	// First append.
	if err := mm.Append("agent-1", "Entry one."); err != nil {
		t.Fatalf("first Append failed: %v", err)
	}

	// Second append.
	if err := mm.Append("agent-1", "Entry two."); err != nil {
		t.Fatalf("second Append failed: %v", err)
	}

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should contain both entries.
	if !strings.Contains(content, "Entry one.") {
		t.Error("missing first entry")
	}
	if !strings.Contains(content, "Entry two.") {
		t.Error("missing second entry")
	}
}

func TestAppend_EmptyEntry(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Append("agent-1", "")
	if err != nil {
		t.Fatalf("Append with empty entry should not error: %v", err)
	}

	// File should not exist since we appended nothing.
	memoryPath := testMemoryPath(basePath, "agent-1")
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Error("MEMORY.md should not exist after empty append")
	}
}

func TestAppend_EmptyID(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Append("", "entry")
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestAppend_MultiLineEntry(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	entry := "Main point.\nSupporting detail.\nMore context."
	err := mm.Append("agent-1", entry)
	if err != nil {
		t.Fatalf("Append multi-line failed: %v", err)
	}

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// All lines should be present.
	if !strings.Contains(content, "Main point.") {
		t.Error("missing first line")
	}
	if !strings.Contains(content, "Supporting detail.") {
		t.Error("missing second line")
	}
	if !strings.Contains(content, "More context.") {
		t.Error("missing third line")
	}
}

func TestSummarize(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	summary := "## Preferences\n- Likes Go\n- Prefers TDD\n\n## Knowledge\n- Solo uses Chi router"
	err := mm.Summarize("agent-1", summary)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load after Summarize failed: %v", err)
	}

	// Should wrap content with header when no h1 present.
	if !strings.Contains(content, "# Agent Memory") {
		t.Error("expected header to be prepended")
	}
	if !strings.Contains(content, "Likes Go") {
		t.Error("missing summary content")
	}
	if !strings.Contains(content, "Last updated:") {
		t.Error("expected 'Last updated' line")
	}
}

func TestSummarize_WithHeader(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	summary := "# Agent Memory — Agent-1\nLast updated: 2025-01-01\n\n## Knowledge\n- Something"
	err := mm.Summarize("agent-1", summary)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// The header should be preserved as-is (not wrapped with another).
	if !strings.Contains(content, "Agent-1") {
		t.Error("expected original agent name in header")
	}
	if !strings.Contains(content, "## Knowledge") {
		t.Error("missing section")
	}
}

func TestSummarize_Empty(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Summarize("agent-1", "")
	if err != nil {
		t.Fatalf("Summarize with empty summary should not error: %v", err)
	}

	memoryPath := testMemoryPath(basePath, "agent-1")
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Error("MEMORY.md should not exist after empty summarize")
	}
}

func TestSummarize_EmptyID(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Summarize("", "some summary")
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestDelete(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	// Create a memory file first.
	if err := mm.Append("agent-1", "test entry"); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify it exists.
	memoryPath := testMemoryPath(basePath, "agent-1")
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		t.Fatal("MEMORY.md should exist before delete")
	}

	// Delete.
	if err := mm.Delete("agent-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it is gone.
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Error("MEMORY.md should not exist after delete")
	}

	// Load should return empty string.
	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load after delete failed: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string after delete, got %q", content)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Delete("nonexistent")
	if err != nil {
		t.Fatalf("Delete on non-existent file should not error: %v", err)
	}
}

func TestDelete_EmptyID(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	err := mm.Delete("")
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	var wg sync.WaitGroup
	numGoroutines := 20

	for i := range numGoroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = mm.Append("agent-1", "entry")
		}(i)
	}
	wg.Wait()

	// Verify all entries were written.
	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Count entries by looking for lines starting with "- " which represent Append prefixes.
	lines := strings.Split(content, "\n")
	entryCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			entryCount++
		}
	}

	if entryCount != numGoroutines {
		t.Errorf("expected %d entries, got %d", numGoroutines, entryCount)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	// Seed some data.
	_ = mm.Append("agent-1", "initial entry")

	var wg sync.WaitGroup

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mm.Load("agent-1")
		}()
	}

	// Concurrent writers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mm.Append("agent-1", "concurrent entry")
		}()
	}

	wg.Wait()

	// Should not panic or produce inconsistent state.
	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load after concurrent access failed: %v", err)
	}
	if !strings.Contains(content, "initial entry") {
		t.Error("missing initial entry after concurrent access")
	}
}

func TestLoadReturnsContent(t *testing.T) {
	basePath := t.TempDir()
	mm := NewMemoryManager(basePath)

	// Directly write a memory file.
	memoryPath := testMemoryPath(basePath, "agent-1")
	_ = os.MkdirAll(filepath.Dir(memoryPath), 0o755)
	_ = os.WriteFile(memoryPath, []byte("Some memory content"), 0o644)

	content, err := mm.Load("agent-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if content != "Some memory content" {
		t.Errorf("expected 'Some memory content', got %q", content)
	}
}
