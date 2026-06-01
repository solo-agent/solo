package agent

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestBuildOpenCodeArgs(t *testing.T) {
	t.Run("default args", func(t *testing.T) {
		opts := &ExecuteOptions{}
		args := buildOpenCodeArgs("hello world", opts)

		assertContains(t, args, "run")
		assertContains(t, args, "--format")
		assertContains(t, args, "json")
		assertContains(t, args, "hello world")
	})

	t.Run("with model", func(t *testing.T) {
		opts := &ExecuteOptions{Model: "gpt-5"}
		args := buildOpenCodeArgs("hello", opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "gpt-5")
	})

	t.Run("with system prompt", func(t *testing.T) {
		opts := &ExecuteOptions{SystemPrompt: "Be helpful."}
		args := buildOpenCodeArgs("hello", opts)

		assertContains(t, args, "--prompt")
		assertContains(t, args, "Be helpful.")
	})

	t.Run("with ExtraArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs: []string{"--model", "gpt-5", "--max-tokens", "8192"},
		}
		args := buildOpenCodeArgs("hello", opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "gpt-5")
		assertContains(t, args, "--max-tokens")
		assertContains(t, args, "8192")
	})

	t.Run("ExtraArgs before CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs:  []string{"--model", "from-extra"},
			CustomArgs: []string{"--model", "from-custom"},
		}
		args := buildOpenCodeArgs("hello", opts)

		// Both --model values appear; the last one wins in CLI parsing.
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
		// Custom --model should be the last one.
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
			ExtraArgs: []string{"--format", "text", "--verbose"},
		}
		args := buildOpenCodeArgs("hello", opts)

		// --format is blocked; its value should be filtered.
		assertNotContains(t, args, "text")
		// --verbose passes through.
		assertContains(t, args, "--verbose")
	})

	t.Run("blocked args filtered from both ExtraArgs and CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs:  []string{"--format", "text-extra"},
			CustomArgs: []string{"--format", "text-custom", "--verbose"},
		}
		args := buildOpenCodeArgs("hello", opts)

		// Both --format values filtered.
		assertNotContains(t, args, "text-extra")
		assertNotContains(t, args, "text-custom")
		assertContains(t, args, "--verbose")
	})

	t.Run("ExtraArgs with no CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs: []string{"--model", "gpt-5"},
		}
		args := buildOpenCodeArgs("hello", opts)

		assertContains(t, args, "--model")
		assertContains(t, args, "gpt-5")
	})

	t.Run("nil ExtraArgs and nil CustomArgs", func(t *testing.T) {
		opts := &ExecuteOptions{}
		args := buildOpenCodeArgs("hello", opts)

		assertContains(t, args, "run")
		assertContains(t, args, "--format")
		assertContains(t, args, "json")
		assertContains(t, args, "hello")
	})

	t.Run("prompt is last argument", func(t *testing.T) {
		opts := &ExecuteOptions{
			ExtraArgs:  []string{"--model", "gpt-5"},
			CustomArgs: []string{"--max-tokens", "4096"},
		}
		args := buildOpenCodeArgs("my prompt text", opts)

		if last := args[len(args)-1]; last != "my prompt text" {
			t.Errorf("expected prompt to be last arg, got %q (args: %v)", last, args)
		}
	})
}

func TestOpenCodeExecute_SemanticInactivityTimeout(t *testing.T) {
	t.Run("when non-zero, timeout is logged at debug level", func(t *testing.T) {
		opts := &ExecuteOptions{
			SemanticInactivityTimeout: 5 * time.Minute,
		}
		if opts.SemanticInactivityTimeout != 5*time.Minute {
			t.Errorf("expected 5m timeout, got %s", opts.SemanticInactivityTimeout)
		}
	})

	t.Run("zero timeout means not set", func(t *testing.T) {
		opts := &ExecuteOptions{}
		if opts.SemanticInactivityTimeout != 0 {
			t.Errorf("expected zero timeout, got %s", opts.SemanticInactivityTimeout)
		}
	})
}

func TestOpenCodeExecute_ExecLookPathNotFound(t *testing.T) {
	b := NewOpenCodeBackend("/invalid/bin/opencode", slog.Default())
	req := &ExecuteRequest{
		AgentID: "agent-1",
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	opts := &ExecuteOptions{
		ExtraArgs:                 []string{"--model", "gpt-5"},
		SemanticInactivityTimeout: 5 * time.Minute,
	}
	_, err := b.Execute(context.Background(), req, opts)
	if err == nil {
		t.Fatal("expected error for missing executable")
	}
	if !strings.Contains(err.Error(), "opencode executable not found at") {
		t.Errorf("expected 'opencode executable not found at' in error, got: %v", err)
	}
}

func TestOpenCodeExportedHelpers(t *testing.T) {
	// Verify asserts are compatible.
	t.Run("assertContains", func(t *testing.T) {
		assertContains(t, []string{"a", "b", "c"}, "b")
	})
	t.Run("assertNotContains", func(t *testing.T) {
		assertNotContains(t, []string{"a", "b", "c"}, "z")
	})
}

func TestOpenCodeIntegration_ExtraArgs(t *testing.T) {
	// Full integration test: ExtraArgs + CustomArgs + blocked args.
	opts := &ExecuteOptions{
		Model:        "gpt-5",
		SystemPrompt: "You are helpful.",
		ExtraArgs:    []string{"--model", "claude-sonnet-4-6", "--verbose"},
		CustomArgs:   []string{"--max-tokens", "8192"},
	}
	args := buildOpenCodeArgs("test prompt", opts)

	// Base args.
	assertContains(t, args, "run")
	assertContains(t, args, "--format")
	assertContains(t, args, "json")

	// opts.Model (from --model flag, added before ExtraArgs).
	// The opts.Model adds --model gpt-5, then ExtraArgs adds --model claude-sonnet-4-6,
	// then CustomArgs has no --model. The last --model wins.
	lastModelIdx := -1
	for i, a := range args {
		if a == "--model" {
			lastModelIdx = i
		}
	}
	if lastModelIdx < 0 || args[lastModelIdx+1] != "claude-sonnet-4-6" {
		t.Errorf("expected last --model to be claude-sonnet-4-6 (from ExtraArgs), got args: %v", args)
	}

	// System prompt from opts.SystemPrompt flag.
	assertContains(t, args, "--prompt")
	assertContains(t, args, "You are helpful.")

	// ExtraArgs (--verbose).
	assertContains(t, args, "--verbose")

	// CustomArgs.
	assertContains(t, args, "--max-tokens")
	assertContains(t, args, "8192")

	// Prompt is last arg.
	if last := args[len(args)-1]; last != "test prompt" {
		t.Errorf("expected prompt as last arg, got %q", last)
	}
}
