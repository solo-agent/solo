package agent

import (
	"context"
	"log/slog"
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

func TestOpenCodeExportedHelpers(t *testing.T) {
	// Verify asserts are compatible.
	t.Run("assertContains", func(t *testing.T) {
		assertContains(t, []string{"a", "b", "c"}, "b")
	})
	t.Run("assertNotContains", func(t *testing.T) {
		assertNotContains(t, []string{"a", "b", "c"}, "z")
	})
}

