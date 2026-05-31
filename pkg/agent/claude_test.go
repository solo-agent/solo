package agent

import (
	"context"
	"encoding/json"
	"strings"
	"log/slog"
	"testing"
)

func TestBuildClaudeArgs(t *testing.T) {
	t.Run("default args", func(t *testing.T) {
		req := &ExecuteRequest{}
		opts := &ExecuteOptions{}
		args := buildClaudeArgs(req, opts)

		assertContains(t, args, "--output-format")
		assertContains(t, args, "stream-json")
		assertContains(t, args, "--input-format")
		assertContains(t, args, "--verbose")
		assertContains(t, args, "--permission-mode")
		assertContains(t, args, "bypassPermissions")
	})

	t.Run("with model", func(t *testing.T) {
		req := &ExecuteRequest{}
		opts := &ExecuteOptions{Model: "sonnet"}
		args := buildClaudeArgs(req, opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "sonnet")
	})

	t.Run("with system prompt", func(t *testing.T) {
		req := &ExecuteRequest{}
		opts := &ExecuteOptions{SystemPrompt: "Be helpful.", WorkspaceDir: t.TempDir()}
		args := buildClaudeArgs(req, opts)

		assertContains(t, args, "--system-prompt-file")
		// The file path contains system-prompt.md, but as part of an absolute
		// path — verify the flag value contains the filename.
		found := false
		for i, a := range args {
			if a == "--system-prompt-file" && i+1 < len(args) && strings.Contains(args[i+1], "system-prompt.md") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected --system-prompt-file pointing to system-prompt.md, got %v", args)
		}
	})

	t.Run("with custom args (filtered)", func(t *testing.T) {
		req := &ExecuteRequest{}
		opts := &ExecuteOptions{
			CustomArgs: []string{"--verbose", "--model", "opus"},
		}
		args := buildClaudeArgs(req, opts)

		// Both --verbose and --model are not blocked, so they pass through.
		assertContains(t, args, "--verbose")
		assertContains(t, args, "--model")
		assertContains(t, args, "opus")
	})
}

func TestFilterCustomArgs(t *testing.T) {
	blocked := map[string]blockedArgMode{
		"--output-format":   blockedWithValue,
		"--input-format":    blockedWithValue,
		"--permission-mode": blockedWithValue,
	}

	t.Run("empty input", func(t *testing.T) {
		result := filterCustomArgs(nil, blocked)
		if result != nil {
			t.Errorf("expected nil for nil input, got %v", result)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := filterCustomArgs([]string{}, blocked)
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %v", result)
		}
	})

	t.Run("blocked standalone flag filtered", func(t *testing.T) {
		// Add a test-specific standalone blocked flag to verify the filtering logic.
		testBlocked := map[string]blockedArgMode{
			"--test-blocked":     blockedStandalone,
			"--output-format":    blockedWithValue,
			"--input-format":     blockedWithValue,
			"--permission-mode":  blockedWithValue,
		}
		args := []string{"--test-blocked", "some-prompt"}
		result := filterCustomArgs(args, testBlocked)
		if len(result) != 1 || result[0] != "some-prompt" {
			t.Errorf("expected [some-prompt], got %v", result)
		}
	})

	t.Run("blocked with-value flag and next arg consumed", func(t *testing.T) {
		args := []string{"--output-format", "json", "--model", "sonnet"}
		result := filterCustomArgs(args, blocked)
		if len(result) != 2 || result[0] != "--model" || result[1] != "sonnet" {
			t.Errorf("expected [--model sonnet], got %v", result)
		}
	})

	t.Run("blocked with inline value filtered", func(t *testing.T) {
		args := []string{"--output-format=json", "--model", "sonnet"}
		result := filterCustomArgs(args, blocked)
		if len(result) != 2 || result[0] != "--model" || result[1] != "sonnet" {
			t.Errorf("expected [--model sonnet], got %v", result)
		}
	})

	t.Run("non-blocked args pass through", func(t *testing.T) {
		args := []string{"--model", "sonnet", "--max-tokens", "4096"}
		result := filterCustomArgs(args, blocked)
		if len(result) != 4 {
			t.Errorf("expected 4 args, got %d: %v", len(result), result)
		}
	})

	t.Run("mixed blocked and non-blocked", func(t *testing.T) {
		args := []string{
			"--model", "opus",
			"--max-tokens", "8192",
			"--output-format", "json",
			"--verbose",
			"--permission-mode", "acceptEdits",
		}
		result := filterCustomArgs(args, blocked)
		expected := []string{"--model", "opus", "--max-tokens", "8192", "--verbose"}
		if !stringSliceEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("consecutive blocked flags", func(t *testing.T) {
		args := []string{"--output-format", "json", "--input-format", "json", "--model", "sonnet"}
		result := filterCustomArgs(args, blocked)
		if len(result) != 2 || result[0] != "--model" || result[1] != "sonnet" {
			t.Errorf("expected [--model sonnet], got %v", result)
		}
	})
}

func TestBuildPrompt(t *testing.T) {
	t.Run("user message", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: RoleUser, Content: "hello"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		expected := "User: hello\n\nAssistant:"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("assistant message", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: RoleAssistant, Content: "hi there"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		expected := "Assistant: hi there\n\nAssistant:"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("system message skipped", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: RoleSystem, Content: "you are helpful"},
				{Role: RoleUser, Content: "hello"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		expected := "User: hello\n\nAssistant:"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("unknown role", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: "custom", Content: "data"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		expected := "[custom]: data\n\nAssistant:"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("multiple messages", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: RoleUser, Content: "first"},
				{Role: RoleAssistant, Content: "second"},
				{Role: RoleUser, Content: "third"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		expected := "User: first\n\nAssistant: second\n\nUser: third\n\nAssistant:"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("empty messages", func(t *testing.T) {
		req := &ExecuteRequest{}
		result := buildPrompt(req, &ExecuteOptions{})
		if result != "Assistant:" {
			t.Errorf("expected %q, got %q", "Assistant:", result)
		}
	})

	t.Run("only system messages", func(t *testing.T) {
		req := &ExecuteRequest{
			Messages: []Message{
				{Role: RoleSystem, Content: "rule1"},
				{Role: RoleSystem, Content: "rule2"},
			},
		}
		result := buildPrompt(req, &ExecuteOptions{})
		if result != "Assistant:" {
			t.Errorf("expected %q, got %q", "Assistant:", result)
		}
	})
}

func TestBuildClaudeInput(t *testing.T) {
	t.Run("produces valid JSON with trailing newline", func(t *testing.T) {
		data, err := buildClaudeInput("hello")
		if err != nil {
			t.Fatalf("buildClaudeInput failed: %v", err)
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			t.Error("expected trailing newline")
		}
		// Trim newline and verify JSON.
		var payload map[string]any
		if err := json.Unmarshal(data[:len(data)-1], &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if payload["type"] != "user" {
			t.Errorf("expected type=user, got %v", payload["type"])
		}
		msg, ok := payload["message"].(map[string]any)
		if !ok {
			t.Fatal("expected message object")
		}
		if msg["role"] != "user" {
			t.Errorf("expected message.role=user, got %v", msg["role"])
		}
	})

	t.Run("wraps prompt in text content", func(t *testing.T) {
		data, _ := buildClaudeInput("Hello, World!")
		var payload map[string]any
		_ = json.Unmarshal(data[:len(data)-1], &payload)
		msg := payload["message"].(map[string]any)
		content := msg["content"].([]any)
		block := content[0].(map[string]any)
		if block["type"] != "text" {
			t.Errorf("expected content type=text, got %v", block["type"])
		}
		if block["text"] != "Hello, World!" {
			t.Errorf("expected text 'Hello, World!', got %v", block["text"])
		}
	})
}

func TestExecLookPathNotFound(t *testing.T) {
	b := NewClaudeBackend("/invalid/bin/claude", slog.Default())
	req := &ExecuteRequest{
		AgentID: "agent-1",
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	opts := &ExecuteOptions{}
	_, err := b.Execute(context.Background(), req, opts)
	if err == nil {
		t.Fatal("expected error for missing executable")
	}
	if !strings.Contains(err.Error(), "claude executable not found at") {
		t.Errorf("expected 'claude executable not found at' in error, got: %v", err)
	}
}

// ── Helpers ──

func assertContains(t *testing.T, slice []string, target string) {
	t.Helper()
	for _, s := range slice {
		if s == target {
			return
		}
	}
	t.Errorf("expected slice to contain %q, got %v", target, slice)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
