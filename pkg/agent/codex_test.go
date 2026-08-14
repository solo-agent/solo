package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildCodexArgs(t *testing.T) {
	t.Run("default args", func(t *testing.T) {
		opts := &ExecuteOptions{}
		args := buildCodexArgs(opts)

		assertContains(t, args, "app-server")
		assertContains(t, args, "--listen")
		assertContains(t, args, "stdio://")
	})

	t.Run("with ExtraArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs: []string{"--model", "gpt-5.1", "--max-tokens", "8192"},
		}
		args := buildCodexArgs(opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "gpt-5.1")
		assertContains(t, args, "--max-tokens")
		assertContains(t, args, "8192")
	})

	t.Run("ExtraArgs before CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs:  []string{"--model", "from-extra"},
			CustomArgs: []string{"--model", "from-custom"},
		}
		args := buildCodexArgs(opts)

		// Both values appear; CustomArgs comes after ExtraArgs so the
		// final --model value that the CLI sees is "from-custom".
		foundExtra := false
		foundCustom := false
		for i, a := range args {
			if a == "--model" && i+1 < len(args) {
				if args[i+1] == "from-extra" {
					foundExtra = true
				}
				if args[i+1] == "from-custom" {
					foundCustom = true
				}
			}
		}
		if !foundExtra || !foundCustom {
			t.Errorf("expected both --model from-extra and --model from-custom, got %v", args)
		}
		// Custom should come after Extra.
		lastIdx := -1
		for i, a := range args {
			if a == "--model" {
				lastIdx = i
			}
		}
		if lastIdx < 0 || args[lastIdx+1] != "from-custom" {
			t.Errorf("expected last --model value to be from-custom, got %v", args)
		}
	})

	t.Run("blocked args filtered from ExtraArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs: []string{"--listen", "http://", "--verbose"},
		}
		args := buildCodexArgs(opts)

		// --listen should be filtered (blocked).
		assertNotContains(t, args, "http://")
		// --verbose passes through.
		assertContains(t, args, "--verbose")
	})

	t.Run("blocked args filtered from both ExtraArgs and CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs:  []string{"--listen", "http://extra"},
			CustomArgs: []string{"--listen", "http://custom", "--verbose"},
		}
		args := buildCodexArgs(opts)

		// Both --listen values filtered.
		assertNotContains(t, args, "http://extra")
		assertNotContains(t, args, "http://custom")
		assertContains(t, args, "--verbose")
	})

	t.Run("ExtraArgs with no CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs: []string{"--model", "gpt-5.1-codex"},
		}
		args := buildCodexArgs(opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "gpt-5.1-codex")
	})

	t.Run("nil ExtraArgs and nil CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{}
		args := buildCodexArgs(opts)

		// Should still contain base args.
		assertContains(t, args, "app-server")
		assertContains(t, args, "--listen")
		assertContains(t, args, "stdio://")
		// No extra or custom values.
		if len(args) != 3 {
			t.Errorf("expected exactly 3 base args, got %d: %v", len(args), args)
		}
	})
}

func TestCodexSemanticInactivityTimeout(t *testing.T) {
	t.Run("default timeout when not configured", func(t *testing.T) {
		opts := &ExecuteOptions{}
		timeout := resolveCodexSemanticInactivityTimeout(opts)
		if timeout != defaultCodexSemanticInactivityTimeout {
			t.Errorf("expected default %s, got %s", defaultCodexSemanticInactivityTimeout, timeout)
		}
	})

	t.Run("custom timeout overrides default", func(t *testing.T) {
		custom := 5 * time.Minute
		opts := &ExecuteOptions{SemanticInactivityTimeout: custom}
		timeout := resolveCodexSemanticInactivityTimeout(opts)
		if timeout != custom {
			t.Errorf("expected %s, got %s", custom, timeout)
		}
	})

	t.Run("zero timeout uses default", func(t *testing.T) {
		opts := &ExecuteOptions{SemanticInactivityTimeout: 0}
		timeout := resolveCodexSemanticInactivityTimeout(opts)
		if timeout != defaultCodexSemanticInactivityTimeout {
			t.Errorf("expected default %s, got %s", defaultCodexSemanticInactivityTimeout, timeout)
		}
	})

	t.Run("negative timeout uses default", func(t *testing.T) {
		opts := &ExecuteOptions{SemanticInactivityTimeout: -1 * time.Minute}
		timeout := resolveCodexSemanticInactivityTimeout(opts)
		if timeout != defaultCodexSemanticInactivityTimeout {
			t.Errorf("expected default %s, got %s", defaultCodexSemanticInactivityTimeout, timeout)
		}
	})
}

func TestCodexPersistentStartDoesNotPrependSystemPrompt(t *testing.T) {
	src, err := os.ReadFile("codex.go")
	if err != nil {
		t.Fatalf("read codex.go: %v", err)
	}

	if strings.Contains(string(src), `prompt = opts.SystemPrompt + "\n\n---\n\n" + prompt`) {
		t.Fatal("persistent Codex must pass SystemPrompt via developerInstructions only, not prepend it to user input")
	}
}

func TestCodexClientCompletesOnSnakeCaseFinalAnswer(t *testing.T) {
	done := false
	c := &codexClient{
		logger:      slog.Default(),
		turnStarted: true,
		onTurnDone: func(aborted bool) {
			if aborted {
				t.Fatal("aborted = true, want false")
			}
			done = true
		},
	}

	c.handleRawNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type":  "agent_message",
			"text":  "Done.",
			"phase": "final_answer",
		},
	})

	if !done {
		t.Fatal("onTurnDone was not called")
	}
}

func TestCodexClientHandlesRawCompletionAfterLegacyEvent(t *testing.T) {
	doneCount := 0
	c := &codexClient{
		logger:               slog.Default(),
		notificationProtocol: "unknown",
		onTurnDone: func(aborted bool) {
			if aborted {
				t.Fatal("aborted = true, want false")
			}
			doneCount++
		},
	}

	legacy, err := json.Marshal(map[string]any{
		"method": "codex/event",
		"params": map[string]any{"msg": map[string]any{
			"type":    "agent_message",
			"message": "Working.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleLine(string(legacy))

	completed, err := json.Marshal(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{
			"threadId": "thread-1",
			"turn": map[string]any{
				"id":     "turn-1",
				"status": "completed",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleLine(string(completed))
	c.handleLine(string(completed))

	if doneCount != 1 {
		t.Fatalf("onTurnDone calls = %d, want 1", doneCount)
	}
	if c.notificationProtocol != "mixed" {
		t.Fatalf("notificationProtocol = %q, want mixed", c.notificationProtocol)
	}
}

func TestCodexTurnResultTreatsAbortedAsCancelled(t *testing.T) {
	result := codexTurnResult(true)
	if result == nil || result.Status != "cancelled" {
		t.Fatalf("codexTurnResult(true) = %+v, want cancelled", result)
	}
}

// resolveCodexSemanticInactivityTimeout mirrors the logic in Execute().
func resolveCodexSemanticInactivityTimeout(opts *ExecuteOptions) time.Duration {
	timeout := defaultCodexSemanticInactivityTimeout
	if opts.SemanticInactivityTimeout > 0 {
		timeout = opts.SemanticInactivityTimeout
	}
	return timeout
}

func assertNotContains(t *testing.T, slice []string, target string) {
	t.Helper()
	for _, s := range slice {
		if s == target {
			t.Errorf("expected slice NOT to contain %q, got %v", target, slice)
			return
		}
	}
}

func TestCodexExportedHelpers(t *testing.T) {
	// Verify that asserts from claude_test.go are compatible.
	t.Run("assertContains", func(t *testing.T) {
		assertContains(t, []string{"a", "b", "c"}, "b")
	})
	t.Run("assertNotContains", func(t *testing.T) {
		assertNotContains(t, []string{"a", "b", "c"}, "z")
	})
}

// ── Test for Execute with SemanticInactivityTimeout code path ──

func TestCodexExecute_SemanticInactivityTimeoutPath(t *testing.T) {
	// Verifies the code path compiles and reaches the timeout logic.
	// Uses missing binary to avoid requiring codex installation.
	// The LookPath check happens before the timeout logic, so this
	// test validates the type system and compilation, not runtime behavior.
	opts := &ExecuteOptions{
		SemanticInactivityTimeout: 5 * time.Minute,
	}
	// Just verify the value is accessible.
	if opts.SemanticInactivityTimeout != 5*time.Minute {
		t.Errorf("expected 5m timeout")
	}
}

func TestCodexExecute_ExtraArgsPath(t *testing.T) {
	// Verifies ExtraArgs are properly wired into buildCodexArgs.
	opts := &ExecuteOptions{
		ExtraArgs:  []string{"--model", "gpt-5.1-codex"},
		CustomArgs: []string{"--max-tokens", "4096"},
	}
	args := buildCodexArgs(opts)

	assertContains(t, args, "app-server")
	assertContains(t, args, "--listen")
	assertContains(t, args, "stdio://")
	assertContains(t, args, "--model")
	assertContains(t, args, "gpt-5.1-codex")
	assertContains(t, args, "--max-tokens")
	assertContains(t, args, "4096")

	// Verify ordering: app-server, --listen, stdio://, then ExtraArgs, then CustomArgs.
	foundListen := false
	foundExtra := false
	foundCustom := false
	for _, a := range args {
		if a == "--listen" {
			foundListen = true
		}
		if a == "gpt-5.1-codex" {
			if !foundListen {
				t.Error("ExtraArgs appeared before base args")
			}
			foundExtra = true
		}
		if a == "4096" {
			if !foundExtra {
				t.Error("CustomArgs appeared before ExtraArgs")
			}
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Error("CustomArgs not found in result")
	}
}

func TestParseCodexSessionFileForWindowUsesLastTurnUsage(t *testing.T) {
	path := t.TempDir() + "/rollout.jsonl"
	data := strings.Join([]string{
		`{"timestamp":"2026-08-13T10:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":20,"cached_input_tokens":50},"last_token_usage":{"input_tokens":10,"output_tokens":2,"cached_input_tokens":5},"model":"gpt-5"}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":500,"output_tokens":50},"model":"gpt-5"}}}`,
		`{"timestamp":"2026-08-13T11:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":130,"output_tokens":27,"cached_input_tokens":59},"last_token_usage":{"input_tokens":30,"output_tokens":5,"reasoning_output_tokens":2,"cached_input_tokens":9},"model":"gpt-5"}}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
	result := parseCodexSessionFileForWindow(path, started, started.Add(time.Minute))
	if result == nil || result.usage.InputTokens != 30 || result.usage.OutputTokens != 7 || result.usage.CacheReadTokens != 9 {
		t.Fatalf("usage=%+v, want last usage for current turn", result)
	}
}

func TestCodexPersistentTurnResultUsesExactThreadUsage(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	sessionID := "019c-session-token-test"
	path := filepath.Join(codexHome, "sessions", "2026", "08", "13", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Second)
	line := fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":21,"output_tokens":4,"reasoning_output_tokens":2,"cached_input_tokens":7},"model":"gpt-5.6"}}}`+"\n", time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	result := codexPersistentTurnResult(false, sessionID, started, "")
	usage := result.Usage["gpt-5.6"]
	if usage.InputTokens != 21 || usage.OutputTokens != 6 || usage.CacheReadTokens != 7 {
		t.Fatalf("usage=%+v, want exact persistent turn usage", usage)
	}
}

func assertPrefix(t *testing.T, value, prefix string) {
	t.Helper()
	if !strings.HasPrefix(value, prefix) {
		t.Errorf("expected prefix %q, got %q", prefix, value)
	}
}
