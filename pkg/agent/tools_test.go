package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── ToolRegistry Tests ──

func TestToolRegistry_RegisterAndHasTool(t *testing.T) {
	r := NewToolRegistry()

	if r.HasTool("nonexistent") {
		t.Error("expected HasTool to return false for unregistered tool")
	}
	if r.ToolCount() != 0 {
		t.Errorf("expected ToolCount 0, got %d", r.ToolCount())
	}

	tool := &mockTool{name: "test_tool"}
	r.Register(tool)

	if !r.HasTool("test_tool") {
		t.Error("expected HasTool to return true after registration")
	}
	if r.ToolCount() != 1 {
		t.Errorf("expected ToolCount 1, got %d", r.ToolCount())
	}
}

func TestToolRegistry_Execute(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&mockTool{name: "echo", execFn: func(ctx context.Context, input string) (string, error) {
		return "echo: " + input, nil
	}})

	result, err := r.Execute(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "echo: hello" {
		t.Errorf("expected 'echo: hello', got %q", result)
	}
}

func TestToolRegistry_ExecuteUnknown(t *testing.T) {
	r := NewToolRegistry()
	_, err := r.Execute(context.Background(), "unknown", "input")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), `tool "unknown" not found`) {
		t.Errorf("expected 'tool not found' error, got: %v", err)
	}
}

func TestToolRegistry_GetSystemPrompt_Empty(t *testing.T) {
	r := NewToolRegistry()
	prompt := r.GetSystemPrompt()
	if prompt != "" {
		t.Errorf("expected empty prompt for empty registry, got %q", prompt)
	}
}

func TestToolRegistry_GetSystemPrompt_WithTools(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&mockTool{name: "alpha", desc: "First tool"})
	r.Register(&mockTool{name: "beta", desc: "Second tool"})

	prompt := r.GetSystemPrompt()
	if !strings.Contains(prompt, "alpha") {
		t.Error("expected 'alpha' in system prompt")
	}
	if !strings.Contains(prompt, "beta") {
		t.Error("expected 'beta' in system prompt")
	}
	if !strings.Contains(prompt, "First tool") {
		t.Error("expected 'First tool' in system prompt")
	}
	if !strings.Contains(prompt, "Second tool") {
		t.Error("expected 'Second tool' in system prompt")
	}
}

func TestToolRegistry_RegisterNil(t *testing.T) {
	r := NewToolRegistry()
	r.Register(nil) // should not panic
	if r.ToolCount() != 0 {
		t.Errorf("expected ToolCount 0 after registering nil, got %d", r.ToolCount())
	}
}

// ── ReadFileTool Tests ──

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadFileTool(dir)

	// Create a test file.
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	result, err := tool.Execute(context.Background(), "test.txt")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestReadFileTool_PathEscape(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadFileTool(dir)

	_, err := tool.Execute(context.Background(), "../outside.txt")
	if err == nil {
		t.Fatal("expected error for path escape")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("expected 'escapes workspace' error, got: %v", err)
	}
}

func TestReadFileTool_NonExistent(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadFileTool(dir)

	_, err := tool.Execute(context.Background(), "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestReadFileTool_NameAndDescription(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Name() != "read_file" {
		t.Errorf("expected name 'read_file', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// ── WriteFileTool Tests ──

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	result, err := tool.Execute(context.Background(), "newfile.txt\nhello world")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got %q", result)
	}

	// Verify file was written.
	data, err := os.ReadFile(filepath.Join(dir, "newfile.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteFileTool_InvalidInput(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	_, err := tool.Execute(context.Background(), "onlypath")
	if err == nil {
		t.Fatal("expected error for invalid input (no content)")
	}
}

func TestWriteFileTool_PathEscape(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	_, err := tool.Execute(context.Background(), "../outside.txt\ncontent")
	if err == nil {
		t.Fatal("expected error for path escape")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("expected 'escapes workspace' error, got: %v", err)
	}
}

func TestWriteFileTool_NameAndDescription(t *testing.T) {
	tool := NewWriteFileTool("/tmp")
	if tool.Name() != "write_file" {
		t.Errorf("expected name 'write_file', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// ── ListFilesTool Tests ──

func TestListFilesTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewListFilesTool(dir)

	// Create some files.
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bb"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	result, err := tool.Execute(context.Background(), ".")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "a.txt") {
		t.Error("expected 'a.txt' in listing")
	}
	if !strings.Contains(result, "b.txt") {
		t.Error("expected 'b.txt' in listing")
	}
	if !strings.Contains(result, "subdir/") {
		t.Error("expected 'subdir/' in listing")
	}
}

func TestListFilesTool_NonExistent(t *testing.T) {
	dir := t.TempDir()
	tool := NewListFilesTool(dir)

	_, err := tool.Execute(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestListFilesTool_PathEscape(t *testing.T) {
	dir := t.TempDir()
	tool := NewListFilesTool(dir)

	_, err := tool.Execute(context.Background(), "../..")
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func TestListFilesTool_NameAndDescription(t *testing.T) {
	tool := NewListFilesTool("/tmp")
	if tool.Name() != "list_files" {
		t.Errorf("expected name 'list_files', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// ── SearchFilesTool Tests ──

func TestSearchFilesTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewSearchFilesTool(dir)

	_ = os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "other.txt"), []byte("no match here"), 0o644)

	result, err := tool.Execute(context.Background(), "func main")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "test.go") {
		t.Error("expected 'test.go' in search results")
	}
	if !strings.Contains(result, "func main") {
		t.Error("expected 'func main' in search results")
	}
}

func TestSearchFilesTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	tool := NewSearchFilesTool(dir)

	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)

	result, err := tool.Execute(context.Background(), "nonexistent_pattern")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "No matches found") {
		t.Errorf("expected 'No matches found', got %q", result)
	}
}

func TestSearchFilesTool_EmptyPattern(t *testing.T) {
	dir := t.TempDir()
	tool := NewSearchFilesTool(dir)

	_, err := tool.Execute(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestSearchFilesTool_NameAndDescription(t *testing.T) {
	tool := NewSearchFilesTool("/tmp")
	if tool.Name() != "search_files" {
		t.Errorf("expected name 'search_files', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// ── Tool integration tests ──

func TestToolRegistry_Integration(t *testing.T) {
	dir := t.TempDir()
	r := NewToolRegistry()

	r.Register(NewReadFileTool(dir))
	r.Register(NewWriteFileTool(dir))
	r.Register(NewListFilesTool(dir))
	r.Register(NewSearchFilesTool(dir))

	if r.ToolCount() != 4 {
		t.Errorf("expected 4 tools, got %d", r.ToolCount())
	}

	sysPrompt := r.GetSystemPrompt()
	if !strings.Contains(sysPrompt, "read_file") {
		t.Error("expected read_file in system prompt")
	}
	if !strings.Contains(sysPrompt, "write_file") {
		t.Error("expected write_file in system prompt")
	}
	if !strings.Contains(sysPrompt, "list_files") {
		t.Error("expected list_files in system prompt")
	}
	if !strings.Contains(sysPrompt, "search_files") {
		t.Error("expected search_files in system prompt")
	}

	// Integration: write then read.
	_, err := r.Execute(context.Background(), "write_file", "test.txt\nintegration content")
	if err != nil {
		t.Fatalf("write_file integration failed: %v", err)
	}

	result, err := r.Execute(context.Background(), "read_file", "test.txt")
	if err != nil {
		t.Fatalf("read_file integration failed: %v", err)
	}
	if result != "integration content" {
		t.Errorf("expected 'integration content', got %q", result)
	}
}

func TestBaseWorkspaceTool_ValidatePath_AbsoluteInWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := &BaseWorkspaceTool{WorkspaceDir: dir}

	// Create a file and validate its absolute path.
	testFile := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(testFile, []byte("content"), 0o644)

	path, err := tool.ValidatePath(testFile)
	if err != nil {
		t.Fatalf("ValidatePath failed: %v", err)
	}
	if path != testFile {
		t.Errorf("expected path %q, got %q", testFile, path)
	}
}

func TestBaseWorkspaceTool_ValidatePath_Subdir(t *testing.T) {
	dir := t.TempDir()
	tool := &BaseWorkspaceTool{WorkspaceDir: dir}

	// Create a subdirectory.
	subDir := filepath.Join(dir, "subdir")
	_ = os.MkdirAll(subDir, 0o755)

	path, err := tool.ValidatePath("subdir")
	if err != nil {
		t.Fatalf("ValidatePath failed: %v", err)
	}
	if path != subDir {
		t.Errorf("expected path %q, got %q", subDir, path)
	}
}

// ── Mock tool for registry tests ──

type mockTool struct {
	name    string
	desc    string
	execFn  func(ctx context.Context, input string) (string, error)
}

func (m *mockTool) Name() string {
	if m.name == "" {
		return "mock"
	}
	return m.name
}

func (m *mockTool) Description() string {
	if m.desc == "" {
		return "mock tool"
	}
	return m.desc
}

func (m *mockTool) Execute(ctx context.Context, input string) (string, error) {
	if m.execFn != nil {
		return m.execFn(ctx, input)
	}
	return "mock output", nil
}
