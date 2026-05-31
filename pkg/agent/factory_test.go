package agent

import (
	"strings"
	"testing"
)

func TestNewBackend_Claude(t *testing.T) {
	b, err := NewBackend("claude", "")
	if err != nil {
		t.Fatalf("NewBackend(\"claude\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", b.Name())
	}
	if _, ok := b.(*ClaudeBackend); !ok {
		t.Error("expected *ClaudeBackend type")
	}
}

func TestNewBackend_Local(t *testing.T) {
	b, err := NewBackend("local", "")
	if err != nil {
		t.Fatalf("NewBackend(\"local\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", b.Name())
	}
	if _, ok := b.(*ClaudeBackend); !ok {
		t.Error("expected *ClaudeBackend type")
	}
}

func TestNewBackend_Codex(t *testing.T) {
	b, err := NewBackend("codex", "")
	if err != nil {
		t.Fatalf("NewBackend(\"codex\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "codex" {
		t.Errorf("expected name 'codex', got %q", b.Name())
	}
	if _, ok := b.(*CodexBackend); !ok {
		t.Error("expected *CodexBackend type")
	}
}

func TestNewBackend_Cursor(t *testing.T) {
	b, err := NewBackend("cursor", "")
	if err != nil {
		t.Fatalf("NewBackend(\"cursor\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "cursor" {
		t.Errorf("expected name 'cursor', got %q", b.Name())
	}
	if _, ok := b.(*CursorBackend); !ok {
		t.Error("expected *CursorBackend type")
	}
}

func TestNewBackend_Gemini(t *testing.T) {
	b, err := NewBackend("gemini", "")
	if err != nil {
		t.Fatalf("NewBackend(\"gemini\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "gemini" {
		t.Errorf("expected name 'gemini', got %q", b.Name())
	}
	if _, ok := b.(*GeminiBackend); !ok {
		t.Error("expected *GeminiBackend type")
	}
}

func TestNewBackend_Kimi(t *testing.T) {
	b, err := NewBackend("kimi", "")
	if err != nil {
		t.Fatalf("NewBackend(\"kimi\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "kimi" {
		t.Errorf("expected name 'kimi', got %q", b.Name())
	}
	if _, ok := b.(*KimiBackend); !ok {
		t.Error("expected *KimiBackend type")
	}
}

func TestNewBackend_Kiro(t *testing.T) {
	b, err := NewBackend("kiro", "")
	if err != nil {
		t.Fatalf("NewBackend(\"kiro\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "kiro" {
		t.Errorf("expected name 'kiro', got %q", b.Name())
	}
	if _, ok := b.(*KiroBackend); !ok {
		t.Error("expected *KiroBackend type")
	}
}

func TestNewBackend_Unknown(t *testing.T) {
	b, err := NewBackend("unknown-provider", "")
	if err == nil {
		t.Fatal("expected error for unknown provider type")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	if !strings.Contains(err.Error(), "unknown backend provider type") {
		t.Errorf("expected 'unknown backend provider type' in error, got: %v", err)
	}
}

func TestNewBackend_OpenAI(t *testing.T) {
	b, err := NewBackend("openai", "sk-...")
	if err == nil {
		t.Fatal("expected error for openai provider type")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	if !strings.Contains(err.Error(), "not implemented via Backend interface") {
		t.Errorf("expected 'not implemented via Backend interface' in error, got: %v", err)
	}
}

func TestNewBackend_Anthropic(t *testing.T) {
	b, err := NewBackend("anthropic", "sk-...")
	if err == nil {
		t.Fatal("expected error for anthropic provider type")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	if !strings.Contains(err.Error(), "not implemented via Backend interface") {
		t.Errorf("expected 'not implemented via Backend interface' in error, got: %v", err)
	}
}

func TestNewBackend_CaseSensitive(t *testing.T) {
	b, err := NewBackend("Claude", "")
	if err == nil {
		t.Fatal("expected error for capitalized 'Claude'")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
}

func TestNewBackend_Copilot(t *testing.T) {
	b, err := NewBackend("copilot", "")
	if err != nil {
		t.Fatalf("NewBackend(\"copilot\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "copilot" {
		t.Errorf("expected name 'copilot', got %q", b.Name())
	}
	if _, ok := b.(*CopilotBackend); !ok {
		t.Error("expected *CopilotBackend type")
	}
}

func TestNewBackend_OpenCode(t *testing.T) {
	b, err := NewBackend("opencode", "")
	if err != nil {
		t.Fatalf("NewBackend(\"opencode\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "opencode" {
		t.Errorf("expected name 'opencode', got %q", b.Name())
	}
	if _, ok := b.(*OpenCodeBackend); !ok {
		t.Error("expected *OpenCodeBackend type")
	}
}

func TestNewBackend_OpenClaw(t *testing.T) {
	b, err := NewBackend("openclaw", "")
	if err != nil {
		t.Fatalf("NewBackend(\"openclaw\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "openclaw" {
		t.Errorf("expected name 'openclaw', got %q", b.Name())
	}
	if _, ok := b.(*OpenClawBackend); !ok {
		t.Error("expected *OpenClawBackend type")
	}
}

func TestNewBackend_Hermes(t *testing.T) {
	b, err := NewBackend("hermes", "")
	if err != nil {
		t.Fatalf("NewBackend(\"hermes\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "hermes" {
		t.Errorf("expected name 'hermes', got %q", b.Name())
	}
	if _, ok := b.(*HermesBackend); !ok {
		t.Error("expected *HermesBackend type")
	}
}

func TestNewBackend_Pi(t *testing.T) {
	b, err := NewBackend("pi", "")
	if err != nil {
		t.Fatalf("NewBackend(\"pi\") failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "pi" {
		t.Errorf("expected name 'pi', got %q", b.Name())
	}
	if _, ok := b.(*PiBackend); !ok {
		t.Error("expected *PiBackend type")
	}
}

func TestNewClaudeBackendFromEnv_Default(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "")
	t.Setenv("CLAUDECODE_BIN", "")

	b := newClaudeBackendFromEnv()
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if b.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", b.Name())
	}
}

func TestNewClaudeBackendFromEnv_CLAUDE_BIN(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "/custom/path/claude")
	t.Setenv("CLAUDECODE_BIN", "")

	b := newClaudeBackendFromEnv()
	if b.executablePath != "/custom/path/claude" {
		t.Errorf("expected executablePath %q, got %q", "/custom/path/claude", b.executablePath)
	}
}

func TestNewClaudeBackendFromEnv_CLAUDECODE_BIN(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "")
	t.Setenv("CLAUDECODE_BIN", "/fallback/path/claude")

	b := newClaudeBackendFromEnv()
	if b.executablePath != "/fallback/path/claude" {
		t.Errorf("expected executablePath %q, got %q", "/fallback/path/claude", b.executablePath)
	}
}

func TestNewClaudeBackendFromEnv_Priority(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "/primary/claude")
	t.Setenv("CLAUDECODE_BIN", "/secondary/claude")

	b := newClaudeBackendFromEnv()
	if b.executablePath != "/primary/claude" {
		t.Errorf("expected executablePath %q, got %q", "/primary/claude", b.executablePath)
	}
}

// ── Constructor default tests ──

func TestNewCodexBackend_Defaults(t *testing.T) {
	b := NewCodexBackend("", nil)
	if b.executablePath != "codex" {
		t.Errorf("expected executablePath 'codex', got %q", b.executablePath)
	}
	if b.Name() != "codex" {
		t.Errorf("expected name 'codex', got %q", b.Name())
	}
}

func TestNewCursorBackend_Defaults(t *testing.T) {
	b := NewCursorBackend("", nil)
	if b.executablePath != "cursor-agent" {
		t.Errorf("expected executablePath 'cursor-agent', got %q", b.executablePath)
	}
	if b.Name() != "cursor" {
		t.Errorf("expected name 'cursor', got %q", b.Name())
	}
}

func TestNewGeminiBackend_Defaults(t *testing.T) {
	b := NewGeminiBackend("", nil)
	if b.executablePath != "gemini" {
		t.Errorf("expected executablePath 'gemini', got %q", b.executablePath)
	}
	if b.Name() != "gemini" {
		t.Errorf("expected name 'gemini', got %q", b.Name())
	}
}

func TestNewKimiBackend_Defaults(t *testing.T) {
	b := NewKimiBackend("", nil)
	if b.executablePath != "kimi" {
		t.Errorf("expected executablePath 'kimi', got %q", b.executablePath)
	}
	if b.Name() != "kimi" {
		t.Errorf("expected name 'kimi', got %q", b.Name())
	}
}

func TestNewKiroBackend_Defaults(t *testing.T) {
	b := NewKiroBackend("", nil)
	if b.executablePath != "kiro-cli" {
		t.Errorf("expected executablePath 'kiro-cli', got %q", b.executablePath)
	}
	if b.Name() != "kiro" {
		t.Errorf("expected name 'kiro', got %q", b.Name())
	}
}

func TestNewBackend_CustomExecPath(t *testing.T) {
	codex := NewCodexBackend("/custom/codex", nil)
	if codex.executablePath != "/custom/codex" {
		t.Errorf("expected /custom/codex, got %q", codex.executablePath)
	}

	gemini := NewGeminiBackend("/usr/local/bin/gemini-cli", nil)
	if gemini.executablePath != "/usr/local/bin/gemini-cli" {
		t.Errorf("expected /usr/local/bin/gemini-cli, got %q", gemini.executablePath)
	}
}

func TestNewCopilotBackend_Defaults(t *testing.T) {
	b := NewCopilotBackend("", nil)
	if b.executablePath != "copilot" {
		t.Errorf("expected executablePath 'copilot', got %q", b.executablePath)
	}
	if b.Name() != "copilot" {
		t.Errorf("expected name 'copilot', got %q", b.Name())
	}
}

func TestNewOpenCodeBackend_Defaults(t *testing.T) {
	b := NewOpenCodeBackend("", nil)
	if b.executablePath != "opencode" {
		t.Errorf("expected executablePath 'opencode', got %q", b.executablePath)
	}
	if b.Name() != "opencode" {
		t.Errorf("expected name 'opencode', got %q", b.Name())
	}
}

func TestNewOpenClawBackend_Defaults(t *testing.T) {
	b := NewOpenClawBackend("", nil)
	if b.executablePath != "openclaw" {
		t.Errorf("expected executablePath 'openclaw', got %q", b.executablePath)
	}
	if b.Name() != "openclaw" {
		t.Errorf("expected name 'openclaw', got %q", b.Name())
	}
}

func TestNewHermesBackend_Defaults(t *testing.T) {
	b := NewHermesBackend("", nil)
	if b.executablePath != "hermes" {
		t.Errorf("expected executablePath 'hermes', got %q", b.executablePath)
	}
	if b.Name() != "hermes" {
		t.Errorf("expected name 'hermes', got %q", b.Name())
	}
}

func TestNewPiBackend_Defaults(t *testing.T) {
	b := NewPiBackend("", nil)
	if b.executablePath != "pi" {
		t.Errorf("expected executablePath 'pi', got %q", b.executablePath)
	}
	if b.Name() != "pi" {
		t.Errorf("expected name 'pi', got %q", b.Name())
	}
}
