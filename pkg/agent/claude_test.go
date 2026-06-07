package agent

import (
	"context"
	"encoding/json"
	"strings"
	"log/slog"
	"testing"
	"time"
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

		// Must use --append-system-prompt-file (not --system-prompt-file) so
		// Claude Code's default system prompt with built-in tool descriptions
		// and safety rules is preserved (see buildClaudeArgs).
		assertContains(t, args, "--append-system-prompt-file")
		// The bare --system-prompt-file (override) flag must NOT be present.
		hasOverride := false
		for _, a := range args {
			if a == "--system-prompt-file" {
				hasOverride = true
				break
			}
		}
		if hasOverride {
			t.Errorf("expected no --system-prompt-file override flag, got %v", args)
		}
		// The file path contains system-prompt.md, but as part of an absolute
		// path — verify the flag value contains the filename.
		found := false
		for i, a := range args {
			if a == "--append-system-prompt-file" && i+1 < len(args) && strings.Contains(args[i+1], "system-prompt.md") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected --append-system-prompt-file pointing to system-prompt.md, got %v", args)
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

// ── system/init + system/api_retry handling ──

func TestClaudeInitInfoToMap(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var info *claudeInitInfo
		if got := info.toMap(); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty struct returns nil (omitempty friendly)", func(t *testing.T) {
		info := &claudeInitInfo{}
		if got := info.toMap(); got != nil {
			t.Errorf("expected nil for empty info, got %v", got)
		}
	})

	t.Run("model only", func(t *testing.T) {
		info := &claudeInitInfo{Model: "claude-sonnet-4"}
		got := info.toMap()
		if got["model"] != "claude-sonnet-4" {
			t.Errorf("expected model=claude-sonnet-4, got %v", got)
		}
	})

	t.Run("mcp servers with and without error", func(t *testing.T) {
		info := &claudeInitInfo{
			MCPServers: []claudeMCPServ{
				{Name: "filesystem", Status: "connected"},
				{Name: "github", Status: "failed", Error: "auth failed"},
			},
		}
		got := info.toMap()
		servers, ok := got["mcp_servers"].([]map[string]any)
		if !ok {
			t.Fatalf("expected []map[string]any for mcp_servers, got %T", got["mcp_servers"])
		}
		if len(servers) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(servers))
		}
		if servers[0]["name"] != "filesystem" || servers[0]["status"] != "connected" {
			t.Errorf("server 0 mismatch: %v", servers[0])
		}
		if _, hasErr := servers[0]["error"]; hasErr {
			t.Errorf("connected server should not include error field, got %v", servers[0])
		}
		if servers[1]["error"] != "auth failed" {
			t.Errorf("server 1 error mismatch: %v", servers[1])
		}
	})

	t.Run("plugin errors", func(t *testing.T) {
		info := &claudeInitInfo{
			PluginErrors: []claudePluginError{
				{Plugin: "test-plugin", Error: "load failed"},
			},
		}
		got := info.toMap()
		errs, ok := got["plugin_errors"].([]map[string]any)
		if !ok || len(errs) != 1 {
			t.Fatalf("expected 1 plugin_error, got %v", got["plugin_errors"])
		}
		if errs[0]["plugin"] != "test-plugin" || errs[0]["error"] != "load failed" {
			t.Errorf("plugin_error mismatch: %v", errs[0])
		}
	})

	t.Run("tools list", func(t *testing.T) {
		info := &claudeInitInfo{Tools: []string{"Read", "Write", "Bash"}}
		got := info.toMap()
		tools, ok := got["tools"].([]string)
		if !ok || len(tools) != 3 {
			t.Fatalf("expected 3 tools, got %v", got["tools"])
		}
		if tools[0] != "Read" || tools[1] != "Write" || tools[2] != "Bash" {
			t.Errorf("tools mismatch: %v", tools)
		}
	})
}

func TestHandleSystemInit(t *testing.T) {
	t.Run("captures all init fields", func(t *testing.T) {
		b := NewClaudeBackend("/bin/true", slog.Default())
		evt := claudeInitEvent{
			Subtype: "init",
			Model:   "claude-sonnet-4",
			Tools:   []string{"Read", "Bash"},
			MCPServers: []claudeMCPServ{
				{Name: "fs", Status: "connected"},
			},
			Plugins: []string{"plugin-a"},
		}
		info := b.handleSystemInit(evt)
		if info == nil {
			t.Fatal("expected non-nil init info")
		}
		if info.Model != "claude-sonnet-4" {
			t.Errorf("expected model captured, got %q", info.Model)
		}
		if len(info.Tools) != 2 || info.Tools[0] != "Read" {
			t.Errorf("expected tools captured, got %v", info.Tools)
		}
		if len(info.MCPServers) != 1 || info.MCPServers[0].Name != "fs" {
			t.Errorf("expected mcp servers captured, got %v", info.MCPServers)
		}
	})

	t.Run("accepts init with no MCP/plugin info", func(t *testing.T) {
		b := NewClaudeBackend("/bin/true", slog.Default())
		evt := claudeInitEvent{Subtype: "init", Model: "claude-opus-4"}
		info := b.handleSystemInit(evt)
		if info == nil {
			t.Fatal("expected non-nil init info")
		}
		if info.Model != "claude-opus-4" {
			t.Errorf("expected model captured, got %q", info.Model)
		}
		if len(info.MCPServers) != 0 || len(info.PluginErrors) != 0 {
			t.Errorf("expected empty mcp/plugin lists, got mcp=%v plugins=%v",
				info.MCPServers, info.PluginErrors)
		}
	})
}

func TestHandleSystemApiRetry(t *testing.T) {
	b := NewClaudeBackend("/bin/true", slog.Default())

	t.Run("emits retrying status chunk with progress", func(t *testing.T) {
		msgCh := make(chan OutputChunk, 1)
		evt := claudeApiRetryEvent{
			Subtype:      "api_retry",
			Attempt:      2,
			MaxRetries:   5,
			RetryDelayMs: 1000,
			ErrorStatus:  429,
			Error:        "rate limit",
		}
		b.handleSystemApiRetry(evt, msgCh)

		select {
		case chunk := <-msgCh:
			if chunk.Type != string(MessageStatus) {
				t.Errorf("expected type=status, got %q", chunk.Type)
			}
			for _, want := range []string{"retrying", "2/5", "1000ms", "429", "rate limit"} {
				if !strings.Contains(chunk.Content, want) {
					t.Errorf("expected content to contain %q, got %q", want, chunk.Content)
				}
			}
		default:
			t.Fatal("expected a status chunk to be sent")
		}
	})

	t.Run("omits max_retries suffix when unknown", func(t *testing.T) {
		msgCh := make(chan OutputChunk, 1)
		evt := claudeApiRetryEvent{
			Subtype:      "api_retry",
			Attempt:      1,
			MaxRetries:   0,
			RetryDelayMs: 500,
		}
		b.handleSystemApiRetry(evt, msgCh)
		select {
		case chunk := <-msgCh:
			if strings.Contains(chunk.Content, "/0") {
				t.Errorf("did not expect (X/0) when max_retries=0, got %q", chunk.Content)
			}
			if !strings.Contains(chunk.Content, "retrying") || !strings.Contains(chunk.Content, "500ms") {
				t.Errorf("expected retrying/500ms in content, got %q", chunk.Content)
			}
		default:
			t.Fatal("expected a status chunk to be sent")
		}
	})

	t.Run("drops chunk when channel is full (non-blocking)", func(t *testing.T) {
		// Unbuffered channel: handler must not block waiting for a consumer.
		msgCh := make(chan OutputChunk)
		evt := claudeApiRetryEvent{
			Subtype:      "api_retry",
			Attempt:      1,
			MaxRetries:   3,
			RetryDelayMs: 100,
		}
		done := make(chan struct{})
		go func() {
			b.handleSystemApiRetry(evt, msgCh)
			close(done)
		}()
		select {
		case <-done:
			// Good — handler returned without blocking.
		case <-time.After(2 * time.Second):
			t.Fatal("handleSystemApiRetry blocked on a full channel")
		}
	})
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
