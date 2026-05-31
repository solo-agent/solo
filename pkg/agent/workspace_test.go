package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	config := &AgentConfig{
		AgentID:      "agent-1",
		Name:         "TestBot",
		SystemPrompt: "You are a test bot.",
		Model:        "sonnet",
		Provider:     "claude",
	}

	ws, err := wm.Prepare("agent-1", config)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// Verify workspace structure.
	if ws == nil {
		t.Fatal("expected non-nil workspace")
	}
	if ws.RootDir == "" || ws.WorkDir == "" || ws.OutputDir == "" {
		t.Error("expected all workspace dirs to be non-empty")
	}

	// Verify directories exist.
	assertDirExists(t, ws.RootDir)
	assertDirExists(t, ws.WorkDir)
	assertDirExists(t, ws.OutputDir)

	// Verify root directory naming convention.
	expectedRoot := filepath.Join(basePath, "agent-1")
	if ws.RootDir != expectedRoot {
		t.Errorf("expected root %q, got %q", expectedRoot, ws.RootDir)
	}

	// Verify work directory.
	expectedWork := filepath.Join(basePath, "agent-1", "workspace")
	if ws.WorkDir != expectedWork {
		t.Errorf("expected work dir %q, got %q", expectedWork, ws.WorkDir)
	}

	// Verify output directory.
	expectedOutput := filepath.Join(basePath, "agent-1", "output")
	if ws.OutputDir != expectedOutput {
		t.Errorf("expected output dir %q, got %q", expectedOutput, ws.OutputDir)
	}

	// Verify solo-config.json exists.
	configPath := filepath.Join(ws.RootDir, "solo-config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("solo-config.json was not created")
	}

	// Verify CLAUDE.md exists.
	claudePath := filepath.Join(ws.WorkDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md was not created")
	}
}

func TestPrepare_Idempotent(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	config := &AgentConfig{
		AgentID: "agent-1",
		Name:    "TestBot",
	}

	// First call.
	ws1, err := wm.Prepare("agent-1", config)
	if err != nil {
		t.Fatalf("first Prepare failed: %v", err)
	}

	// Second call — should not error.
	ws2, err := wm.Prepare("agent-1", config)
	if err != nil {
		t.Fatalf("second Prepare failed: %v", err)
	}

	// Both should return the same paths.
	if ws1.RootDir != ws2.RootDir {
		t.Errorf("root dir changed: %q vs %q", ws1.RootDir, ws2.RootDir)
	}
	if ws1.WorkDir != ws2.WorkDir {
		t.Errorf("work dir changed: %q vs %q", ws1.WorkDir, ws2.WorkDir)
	}
	if ws1.OutputDir != ws2.OutputDir {
		t.Errorf("output dir changed: %q vs %q", ws1.OutputDir, ws2.OutputDir)
	}
}

func TestPrepare_InvalidID(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	_, err := wm.Prepare("", nil)
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestPrepare_NilConfig(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	ws, err := wm.Prepare("agent-1", nil)
	if err != nil {
		t.Fatalf("Prepare with nil config failed: %v", err)
	}

	// Directories should exist.
	assertDirExists(t, ws.WorkDir)
	assertDirExists(t, ws.OutputDir)

	// Config should not be written.
	configPath := filepath.Join(ws.RootDir, "solo-config.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("solo-config.json should not exist for nil config")
	}

	// CLAUDE.md should still be created (with minimal content).
	claudePath := filepath.Join(ws.WorkDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md should exist even with nil config")
	}
}

func TestInjectConfig(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	_, err := wm.Prepare("agent-1", &AgentConfig{
		AgentID:      "agent-1",
		Name:         "TestBot",
		SystemPrompt: "You are a helpful test bot.",
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// InjectConfig now only ensures the workspace dir exists.
	// CLAUDE.md is written separately by InjectInstructionFiles.
	channelCtx := &ChannelContext{
		ChannelID:   "chan-1",
		ChannelName: "general",
		TriggerType: TriggerChat,
	}
	err = wm.InjectConfig(context.Background(), "agent-1", channelCtx)
	if err != nil {
		t.Fatalf("InjectConfig failed: %v", err)
	}

	// Verify workspace directory exists.
	workDir := filepath.Join(basePath, "agent-1", "workspace")
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		t.Error("workspace directory should exist after InjectConfig")
	}
}

func TestInjectConfig_AllTriggerTypes(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	_, err := wm.Prepare("agent-1", &AgentConfig{
		AgentID: "agent-1",
		Name:    "TestBot",
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	for _, tt := range []TriggerType{TriggerChat, TriggerMention, TriggerDM, TriggerThread} {
		t.Run(string(tt), func(t *testing.T) {
			err := wm.InjectConfig(context.Background(), "agent-1", &ChannelContext{
				ChannelID:   "chan-1",
				ChannelName: "test",
				TriggerType: tt,
			})
			if err != nil {
				t.Fatalf("InjectConfig failed: %v", err)
			}
		})
	}
}

func TestInjectConfig_InvalidID(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	err := wm.InjectConfig(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestWorkspacePath(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	path := wm.WorkspacePath("agent-1")
	expected := filepath.Join(basePath, "agent-1")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestCleanup(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	// Prepare workspace.
	ws, err := wm.Prepare("agent-1", &AgentConfig{
		AgentID: "agent-1",
		Name:    "TestBot",
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// Cleanup.
	if err := wm.Cleanup("agent-1"); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify directories no longer exist.
	if _, err := os.Stat(ws.RootDir); !os.IsNotExist(err) {
		t.Error("RootDir should not exist after cleanup")
	}
}

func TestCleanup_InvalidID(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	err := wm.Cleanup("")
	if err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if !strings.Contains(err.Error(), "agent ID is required") {
		t.Errorf("expected 'agent ID is required' in error, got: %v", err)
	}
}

func TestCleanup_NonExistent(t *testing.T) {
	basePath := t.TempDir()
	wm := NewWorkspaceManager(basePath)

	err := wm.Cleanup("nonexistent-agent")
	if err != nil {
		t.Fatalf("Cleanup on non-existent workspace should not error: %v", err)
	}
}

// ── Helpers ──

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %q to exist: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", path)
	}
}
