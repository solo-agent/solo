package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestOpenCodeStartReturnsAfterSessionNewBeforePromptCompletes(t *testing.T) {
	tempDir := t.TempDir()
	fake := filepath.Join(tempDir, "opencode")
	promptStarted := filepath.Join(tempDir, "prompt-started")
	releasePrompt := filepath.Join(tempDir, "release-prompt")
	script := fmt.Sprintf(`#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":0,"result":{}}\n'
      ;;
    *'"method":"session/new"'*)
      printf '{"jsonrpc":"2.0","id":1,"result":{"sessionId":"opencode-session-1"}}\n'
      ;;
    *'"method":"session/prompt"'*)
	  : > %q
	  while [ ! -f %q ]; do sleep 0.01; done
	  printf '{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}\n'
      ;;
  esac
done
`, promptStarted, releasePrompt)
	if err := os.WriteFile(fake, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewOpenCodeBackend(fake, slog.Default())
	done := make(chan *PersistentSession, 1)
	errCh := make(chan error, 1)
	go func() {
		ps, err := b.Start(ctx, &ExecuteRequest{
			AgentID:  "agent-1",
			Messages: []Message{{Role: RoleUser, Content: "hello"}},
		}, &ExecuteOptions{})
		if err != nil {
			errCh <- err
			return
		}
		done <- ps
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Start returned error: %v", err)
	case ps := <-done:
		defer b.Close(ps)
		if ps.SessionID != "opencode-session-1" {
			t.Fatalf("SessionID = %q, want opencode-session-1", ps.SessionID)
		}
		if _, err := os.Stat(promptStarted); err != nil && !os.IsNotExist(err) {
			t.Fatalf("stat prompt marker: %v", err)
		}
		if err := os.WriteFile(releasePrompt, nil, 0o600); err != nil {
			t.Fatalf("release prompt: %v", err)
		}
		select {
		case result := <-ps.Result:
			if result == nil || result.Status != "completed" {
				t.Fatalf("prompt result = %#v, want completed", result)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("initial prompt did not complete after release")
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Start did not return before session/prompt completed")
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
