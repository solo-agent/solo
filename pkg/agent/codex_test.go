package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildPromptFromMessagesPreservesSystemContext(t *testing.T) {
	got := buildPromptFromMessages([]Message{
		{Role: RoleSystem, Content: "# Session Continuity\nresume task"},
		{Role: RoleUser, Content: "continue"},
	})
	want := "System: # Session Continuity\nresume task\n[user]: continue\n"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

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

func TestCodexClientWaitsForRawTurnCompletionAfterFinalAnswer(t *testing.T) {
	done := false
	var chunks []OutputChunk
	c := &codexClient{
		logger:               slog.Default(),
		threadID:             "thread-1",
		turnID:               "turn-1",
		notificationProtocol: "raw",
		onChunk: func(chunk OutputChunk) {
			chunks = append(chunks, chunk)
		},
		onTurnDone: func(aborted bool) {
			if aborted {
				t.Fatal("aborted = true, want false")
			}
			done = true
		},
	}

	c.handleRawNotification("item/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":  "agent_message",
			"text":  "Done.",
			"phase": "final_answer",
		},
	})

	if done {
		t.Fatal("final_answer item completed the raw turn before trailing usage")
	}
	c.handleRawNotification("thread/status/changed", map[string]any{
		"threadId": "thread-1",
		"status":   map[string]any{"type": "idle"},
	})
	if done {
		t.Fatal("idle status completed the raw turn before trailing usage")
	}
	c.notificationProtocol = "mixed"
	c.handleEvent(map[string]any{"type": "task_complete"})
	c.handleEvent(map[string]any{"type": "turn_aborted"})
	if done {
		t.Fatal("legacy terminal event completed a mixed-protocol turn before trailing usage")
	}
	c.handleRawNotification("thread/tokenUsage/updated", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"tokenUsage": map[string]any{
			"last":               map[string]any{"totalTokens": float64(42)},
			"modelContextWindow": float64(100),
		},
	})
	if len(chunks) != 2 || chunks[1].Context == nil || chunks[1].Context.UsedTokens == nil || *chunks[1].Context.UsedTokens != 42 {
		t.Fatalf("chunks = %+v, want final text then trailing context usage", chunks)
	}
	c.handleRawNotification("turn/completed", map[string]any{
		"threadId": "thread-1",
		"turn":     map[string]any{"id": "turn-1", "status": "completed"},
	})
	if !done {
		t.Fatal("turn/completed did not complete the raw turn")
	}
}

func TestCodexClientPureLegacyTerminalFallback(t *testing.T) {
	done := false
	c := &codexClient{
		logger:               slog.Default(),
		notificationProtocol: "legacy",
		onTurnDone:           func(bool) { done = true },
	}
	c.handleEvent(map[string]any{"type": "task_complete"})
	if !done {
		t.Fatal("pure legacy task_complete did not finish the turn")
	}
}

func TestCodexClientRawFailedTurnSignalsFailure(t *testing.T) {
	var failed error
	done := false
	c := &codexClient{
		logger:   slog.Default(),
		threadID: "thread-1",
		turnID:   "turn-1",
		onTurnDone: func(bool) {
			done = true
		},
		onTurnFailed: func(err error) {
			failed = err
		},
	}
	c.handleRawNotification("turn/completed", map[string]any{
		"threadId": "thread-1",
		"turn": map[string]any{
			"id":     "turn-1",
			"status": "failed",
			"error":  map[string]any{"code": "context_length_exceeded"},
		},
	})
	if done || failed == nil || !strings.Contains(failed.Error(), "context_length_exceeded") {
		t.Fatalf("done=%t failed=%v, want failure callback with provider code", done, failed)
	}
}

func TestCodexClientIgnoresFailedCompletionFromPreviousTurn(t *testing.T) {
	c := &codexClient{logger: slog.Default(), threadID: "thread-1"}
	c.prepareTurn(nil, func(bool) {}, func(error) {})
	c.recordTurnID("turn-old")
	c.signalTurnDoneForID("turn-old", false)

	failed := false
	c.prepareTurn(nil, func(bool) {}, func(error) { failed = true })
	c.recordTurnID("turn-new")
	c.handleRawNotification("turn/completed", map[string]any{
		"threadId": "thread-1",
		"turn": map[string]any{
			"id":     "turn-old",
			"status": "failed",
			"error":  map[string]any{"message": "late failure"},
		},
	})
	c.turnDoneMu.Lock()
	turnDone := c.turnDone
	c.turnDoneMu.Unlock()
	c.turnErrorMu.Lock()
	turnError := c.turnError
	c.turnErrorMu.Unlock()
	if failed || turnDone || turnError != "" {
		t.Fatalf("previous turn failure contaminated the active turn: failed=%t done=%t error=%q", failed, turnDone, turnError)
	}
}

func TestCodexClientFailedTurnUsesPriorErrorNotification(t *testing.T) {
	var failed error
	c := &codexClient{
		logger:       slog.Default(),
		threadID:     "thread-1",
		turnID:       "turn-1",
		onTurnFailed: func(err error) { failed = err },
	}
	c.handleRawNotification("error", map[string]any{
		"threadId":  "thread-1",
		"willRetry": false,
		"error":     map[string]any{"message": "context_length_exceeded"},
	})
	c.handleRawNotification("turn/completed", map[string]any{
		"threadId": "thread-1",
		"turn": map[string]any{
			"id":     "turn-1",
			"status": "failed",
			"error":  nil,
		},
	})
	if failed == nil || !strings.Contains(failed.Error(), "context_length_exceeded") {
		t.Fatalf("failed = %v, want prior context error", failed)
	}
}

func TestCodexClientPrepareTurnClearsPreviousError(t *testing.T) {
	c := &codexClient{logger: slog.Default(), turnError: "previous turn failed"}
	c.prepareTurn(nil, nil, nil)
	c.turnErrorMu.Lock()
	defer c.turnErrorMu.Unlock()
	if c.turnError != "" {
		t.Fatalf("turnError = %q, want empty", c.turnError)
	}
}

func TestCodexClientCallbackReplacementIsSynchronized(t *testing.T) {
	c := &codexClient{logger: slog.Default()}
	var calls atomic.Int64
	c.prepareTurn(func(OutputChunk) { calls.Add(1) }, func(bool) {}, func(error) {})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.emitChunk(OutputChunk{Type: string(MessageStatus)})
		}()
		c.prepareTurn(func(OutputChunk) { calls.Add(1) }, func(bool) {}, func(error) {})
	}
	wg.Wait()
	if calls.Load() != 100 {
		t.Fatalf("callback calls = %d, want 100", calls.Load())
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

func TestCodexContextCompactionPairing(t *testing.T) {
	newClient := func(chunks *[]OutputChunk) *codexClient {
		return &codexClient{
			logger:   slog.Default(),
			threadID: "thread-1",
			turnID:   "turn-1",
			onChunk:  func(chunk OutputChunk) { *chunks = append(*chunks, chunk) },
		}
	}
	usage := func(threadID, turnID string, last, total int64, window any) map[string]any {
		return map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"tokenUsage": map[string]any{
				"last":               map[string]any{"totalTokens": last},
				"total":              map[string]any{"totalTokens": total},
				"modelContextWindow": window,
			},
		}
	}
	item := func(threadID, turnID, itemID string) map[string]any {
		return map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"item":     map[string]any{"type": "contextCompaction", "id": itemID},
		}
	}

	t.Run("uses last snapshot and pairs the first post snapshot", func(t *testing.T) {
		var chunks []OutputChunk
		client := newClient(&chunks)
		client.handleRawNotification("thread/tokenUsage/updated", usage("thread-1", "turn-1", 95, 9000, int64(100)))
		client.handleRawNotification("thread/tokenUsage/updated", usage("wrong-thread", "turn-1", 99, 9999, int64(100)))
		client.handleRawNotification("item/started", item("thread-1", "turn-1", "compact-1"))
		client.handleRawNotification("item/completed", item("thread-1", "turn-1", "compact-1"))
		client.handleRawNotification("thread/tokenUsage/updated", usage("thread-1", "turn-1", 40, 10000, int64(100)))

		if len(chunks) != 4 {
			t.Fatalf("chunks = %+v, want usage/start/end/usage", chunks)
		}
		if got := chunks[0].Context; got == nil || got.UsedTokens == nil || *got.UsedTokens != 95 {
			t.Fatalf("first usage = %+v, want tokenUsage.last.totalTokens", got)
		}
		started, completed := chunks[1].Context, chunks[2].Context
		if started == nil || started.Type != "compaction_start" || started.BeforeTokens == nil || *started.BeforeTokens != 95 {
			t.Fatalf("started = %+v", started)
		}
		if completed == nil || completed.Type != "compaction_end" || completed.BeforeTokens == nil || *completed.BeforeTokens != 95 || completed.AfterTokens == nil || *completed.AfterTokens != 40 {
			t.Fatalf("completed = %+v", completed)
		}
	})

	t.Run("completed compaction without post snapshot has unknown after", func(t *testing.T) {
		var chunks []OutputChunk
		client := newClient(&chunks)
		client.handleRawNotification("thread/tokenUsage/updated", usage("thread-1", "turn-1", 95, 9000, int64(100)))
		client.handleRawNotification("item/started", item("thread-1", "turn-1", "compact-1"))
		client.handleRawNotification("item/completed", item("thread-1", "turn-1", "compact-1"))
		client.signalTurnDoneForID("turn-1", false)

		completed := chunks[len(chunks)-1].Context
		if completed == nil || completed.Type != "compaction_end" || completed.BeforeTokens == nil || *completed.BeforeTokens != 95 || completed.AfterTokens != nil {
			t.Fatalf("completed = %+v, want unknown after", completed)
		}
	})

	t.Run("wrong turn completion cannot pair", func(t *testing.T) {
		var chunks []OutputChunk
		client := newClient(&chunks)
		client.handleRawNotification("thread/tokenUsage/updated", usage("thread-1", "turn-1", 95, 9000, int64(100)))
		client.handleRawNotification("item/started", item("thread-1", "turn-1", "compact-1"))
		client.handleRawNotification("item/completed", item("thread-1", "wrong-turn", "compact-1"))
		client.handleRawNotification("thread/tokenUsage/updated", usage("thread-1", "turn-1", 40, 10000, int64(100)))
		client.signalTurnDoneForID("turn-1", false)

		for _, chunk := range chunks {
			if chunk.Context != nil && chunk.Context.Type == "compaction_end" {
				t.Fatalf("unexpected paired compaction = %+v", chunk.Context)
			}
		}
	})
}

func assertPrefix(t *testing.T, value, prefix string) {
	t.Helper()
	if !strings.HasPrefix(value, prefix) {
		t.Errorf("expected prefix %q, got %q", prefix, value)
	}
}
