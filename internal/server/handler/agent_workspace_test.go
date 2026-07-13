package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceNodesUseRelativePaths(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "mention-artifact.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "docs", "note.md"), []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &AgentHandler{}
	tree, err := h.buildFileTree(base, base, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Path != "." {
		t.Fatalf("root path should be relative dot, got %q", tree.Path)
	}

	paths := map[string]bool{}
	for _, child := range tree.Children {
		paths[child.Path] = true
		for _, grandchild := range child.Children {
			paths[grandchild.Path] = true
		}
	}
	for _, want := range []string{"mention-artifact.html", "docs", "docs/note.md"} {
		if !paths[want] {
			t.Fatalf("missing relative workspace path %q in %#v", want, paths)
		}
	}
}

func TestNormalizeWorkspaceRelPathAcceptsLegacyAbsolutePath(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "mention-artifact.html")

	got, ok := normalizeWorkspaceRelPath(base, file)
	if !ok || got != "mention-artifact.html" {
		t.Fatalf("absolute workspace path should normalize to relative path, got %q ok=%v", got, ok)
	}

	if got, ok := normalizeWorkspaceRelPath(base, filepath.Dir(base)); ok {
		t.Fatalf("outside absolute path should be rejected, got %q", got)
	}
}
