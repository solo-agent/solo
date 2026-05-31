package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Tool is the interface for tools that agents can invoke.
type Tool interface {
	// Name returns the tool's unique identifier (e.g. "read_file").
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// Execute runs the tool with the given input string and returns the output.
	Execute(ctx context.Context, input string) (string, error)
}

// ToolRegistry manages a set of registered tools and provides utilities
// for generating system prompt descriptions and executing tools by name.
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. If a tool with the same name already
// exists, it is overwritten.
func (r *ToolRegistry) Register(t Tool) {
	if t != nil {
		r.tools[t.Name()] = t
	}
}

// GetSystemPrompt generates a tool description text suitable for inclusion
// in an agent's system prompt. The output is a Markdown-formatted list of
// available tools with their descriptions.
func (r *ToolRegistry) GetSystemPrompt() string {
	if len(r.tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Tools\n\n")
	b.WriteString("You have access to the following built-in tools. Use them by outputting a tool_use event:\n\n")

	// Sort tool names for deterministic output.
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sortTools(names)

	for _, name := range names {
		t := r.tools[name]
		fmt.Fprintf(&b, "### %s\n\n", t.Name())
		fmt.Fprintf(&b, "%s\n\n", t.Description())
	}

	b.WriteString("To call a tool, emit a JSON tool_use block in your output stream.\n")
	return b.String()
}

// Execute runs a registered tool by name with the given input. Returns an
// error if the tool is not found in the registry.
func (r *ToolRegistry) Execute(ctx context.Context, name, input string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	return t.Execute(ctx, input)
}

// HasTool returns true if a tool with the given name is registered.
func (r *ToolRegistry) HasTool(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// ToolCount returns the number of registered tools.
func (r *ToolRegistry) ToolCount() int {
	return len(r.tools)
}

// sortTools sorts tool names in a stable order.
func sortTools(names []string) {
	// Simple insertion sort for small slices.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}

// ── Built-in tool implementations ──

// BaseWorkspaceTool provides common path validation for workspace-constrained
// tools. It ensures that file operations stay within the workspace directory.
type BaseWorkspaceTool struct {
	WorkspaceDir string
}

// ValidatePath checks that the given path resolves to a location within
// WorkspaceDir. Returns the cleaned absolute path if valid, or an error
// if the path escapes the workspace.
func (b *BaseWorkspaceTool) ValidatePath(target string) (string, error) {
	target = filepath.Clean(target)
	// If the target is relative, resolve it relative to WorkspaceDir.
	if !filepath.IsAbs(target) {
		target = filepath.Join(b.WorkspaceDir, target)
	}
	target = filepath.Clean(target)

	workspaceAbs, err := filepath.Abs(b.WorkspaceDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace dir: %w", err)
	}
	workspaceAbs = filepath.Clean(workspaceAbs)

	// Ensure the resolved path is within the workspace directory.
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}
	targetAbs = filepath.Clean(targetAbs)

	if !strings.HasPrefix(targetAbs, workspaceAbs+string(filepath.Separator)) && targetAbs != workspaceAbs {
		return "", fmt.Errorf("path %q escapes workspace directory %q", target, workspaceAbs)
	}

	return targetAbs, nil
}

// ReadFileTool implements Tool for reading files within the workspace.
type ReadFileTool struct {
	BaseWorkspaceTool
}

// NewReadFileTool creates a ReadFileTool constrained to the given workspace.
func NewReadFileTool(workspaceDir string) *ReadFileTool {
	return &ReadFileTool{BaseWorkspaceTool: BaseWorkspaceTool{WorkspaceDir: workspaceDir}}
}

// Name returns "read_file".
func (t *ReadFileTool) Name() string { return "read_file" }

// Description returns a description of the read_file tool.
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file within the workspace. Provide the file path as input."
}

// Execute reads a file within the workspace and returns its contents.
func (t *ReadFileTool) Execute(ctx context.Context, input string) (string, error) {
	path, err := t.ValidatePath(input)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("read_file: file %q does not exist", input)
		}
		return "", fmt.Errorf("read_file: %w", err)
	}

	return string(data), nil
}

// WriteFileTool implements Tool for writing files within the workspace.
type WriteFileTool struct {
	BaseWorkspaceTool
}

// NewWriteFileTool creates a WriteFileTool constrained to the given workspace.
func NewWriteFileTool(workspaceDir string) *WriteFileTool {
	return &WriteFileTool{BaseWorkspaceTool: BaseWorkspaceTool{WorkspaceDir: workspaceDir}}
}

// Name returns "write_file".
func (t *WriteFileTool) Name() string { return "write_file" }

// Description returns a description of the write_file tool.
func (t *WriteFileTool) Description() string {
	return "Write content to a file within the workspace. Input format: path|content (first line is the file path, rest is content)."
}

// Execute writes content to a file within the workspace.
func (t *WriteFileTool) Execute(ctx context.Context, input string) (string, error) {
	// Split input into path and content (first line = path, rest = content).
	parts := strings.SplitN(input, "\n", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("write_file: input must have path on first line, content on subsequent lines")
	}

	path, err := t.ValidatePath(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	content := parts[1]

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("write_file: create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// ListFilesTool implements Tool for listing files in a directory within the workspace.
type ListFilesTool struct {
	BaseWorkspaceTool
}

// NewListFilesTool creates a ListFilesTool constrained to the given workspace.
func NewListFilesTool(workspaceDir string) *ListFilesTool {
	return &ListFilesTool{BaseWorkspaceTool: BaseWorkspaceTool{WorkspaceDir: workspaceDir}}
}

// Name returns "list_files".
func (t *ListFilesTool) Name() string { return "list_files" }

// Description returns a description of the list_files tool.
func (t *ListFilesTool) Description() string {
	return "List files and directories in a path within the workspace. Provide the directory path as input. Use \".\" for the workspace root."
}

// Execute lists files in a directory within the workspace.
func (t *ListFilesTool) Execute(ctx context.Context, input string) (string, error) {
	if input == "" || input == "." {
		input = t.WorkspaceDir
	}

	path, err := t.ValidatePath(input)
	if err != nil {
		return "", fmt.Errorf("list_files: %w", err)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("list_files: directory %q does not exist", input)
		}
		return "", fmt.Errorf("list_files: %w", err)
	}

	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		info, err := entry.Info()
		if err == nil {
			fmt.Fprintf(&b, "%s  (%d bytes)\n", name, info.Size())
		} else {
			fmt.Fprintf(&b, "%s\n", name)
		}
	}

	return b.String(), nil
}

// SearchFilesTool implements Tool for searching file contents within the workspace.
type SearchFilesTool struct {
	BaseWorkspaceTool
}

// NewSearchFilesTool creates a SearchFilesTool constrained to the given workspace.
func NewSearchFilesTool(workspaceDir string) *SearchFilesTool {
	return &SearchFilesTool{BaseWorkspaceTool: BaseWorkspaceTool{WorkspaceDir: workspaceDir}}
}

// Name returns "search_files".
func (t *SearchFilesTool) Name() string { return "search_files" }

// Description returns a description of the search_files tool.
func (t *SearchFilesTool) Description() string {
	return "Search for a pattern in files within the workspace. Input format: pattern (grep-style text search)."
}

// Execute searches for a pattern in files within the workspace.
func (t *SearchFilesTool) Execute(ctx context.Context, input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("search_files: search pattern is required")
	}

	pattern := strings.TrimSpace(input)
	if pattern == "" {
		return "", fmt.Errorf("search_files: search pattern is required")
	}

	var b strings.Builder
	err := filepath.WalkDir(t.WorkspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if d.IsDir() {
			// Skip hidden directories and common build directories.
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and large files.
		info, err := d.Info()
		if err != nil || info.Size() > 1024*1024 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := bytes.Split(data, []byte("\n"))
		for i, line := range lines {
			if bytes.Contains(line, []byte(pattern)) {
				rel, _ := filepath.Rel(t.WorkspaceDir, path)
				fmt.Fprintf(&b, "%s:%d: %s\n", rel, i+1, strings.TrimSpace(string(line)))
			}
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search_files: %w", err)
	}

	if b.Len() == 0 {
		return fmt.Sprintf("No matches found for pattern %q", pattern), nil
	}

	return b.String(), nil
}
