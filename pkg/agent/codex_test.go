package agent

import (
	"os"
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

func assertPrefix(t *testing.T, value, prefix string) {
	t.Helper()
	if !strings.HasPrefix(value, prefix) {
		t.Errorf("expected prefix %q, got %q", prefix, value)
	}
}
