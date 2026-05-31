package agent

import (
	"fmt"
	"log/slog"
	"os"
)

// NewBackend creates a Backend for the given provider type.
//
// Supported provider types:
//   - "claude"   — Claude Code CLI (binary resolved from PATH)
//   - "local"    — same as "claude" (local CLI execution)
//   - "codex"    — Codex CLI via JSON-RPC 2.0
//   - "cursor"   — Cursor Agent CLI via stream-json
//   - "gemini"   — Google Gemini CLI via stream-json
//   - "kimi"     — Kimi CLI via ACP protocol
//   - "kiro"     — Kiro CLI via ACP protocol
//   - "copilot"  — GitHub Copilot CLI via JSONL
//   - "opencode" — OpenCode CLI via stream-json
//   - "openclaw" — OpenClaw Agent CLI via stream-json
//   - "hermes"   — Hermes CLI via ACP protocol
//   - "pi"       — Pi CLI via JSON event stream
//   - "openai"   — not yet implemented via Backend; use llm.NewProvider
//   - "anthropic" — not yet implemented via Backend; use llm.NewProvider
//
// The apiKey parameter is reserved for API-based providers (openai,
// anthropic) and is ignored for CLI-based providers.
// For CLI-based providers the binary path is resolved via PATH.
// Set PROVIDER_BIN (e.g. CODEX_BIN) to override the default binary name.
func NewBackend(providerType, apiKey string) (Backend, error) {
	switch providerType {
	case "claude", "local":
		return newClaudeBackendFromEnv(), nil
	case "codex":
		return NewCodexBackend(os.Getenv("CODEX_BIN"), slog.Default()), nil
	case "cursor":
		return NewCursorBackend(os.Getenv("CURSOR_BIN"), slog.Default()), nil
	case "gemini":
		return NewGeminiBackend(os.Getenv("GEMINI_BIN"), slog.Default()), nil
	case "kimi":
		return NewKimiBackend(os.Getenv("KIMI_BIN"), slog.Default()), nil
	case "kiro":
		return NewKiroBackend(os.Getenv("KIRO_BIN"), slog.Default()), nil
	case "copilot":
		return NewCopilotBackend(os.Getenv("COPILOT_BIN"), slog.Default()), nil
	case "opencode":
		return NewOpenCodeBackend(os.Getenv("OPENCODE_BIN"), slog.Default()), nil
	case "openclaw":
		return NewOpenClawBackend(os.Getenv("OPENCLAW_BIN"), slog.Default()), nil
	case "hermes":
		return NewHermesBackend(os.Getenv("HERMES_BIN"), slog.Default()), nil
	case "pi":
		return NewPiBackend(os.Getenv("PI_BIN"), slog.Default()), nil
	case "openai", "anthropic":
		return nil, fmt.Errorf("backend %q: not implemented via Backend interface; use llm.NewProvider instead", providerType)
	default:
		return nil, fmt.Errorf("unknown backend provider type: %q (supported: claude, local, codex, cursor, gemini, kimi, kiro, copilot, opencode, openclaw, hermes, pi, openai, anthropic)", providerType)
	}
}

// newClaudeBackendFromEnv resolves the claude executable path from
// environment variables and constructs a ClaudeBackend. Priority:
// CLAUDE_BIN > CLAUDECODE_BIN > "claude" (PATH lookup).
func newClaudeBackendFromEnv() *ClaudeBackend {
	execPath := os.Getenv("CLAUDE_BIN")
	if execPath == "" {
		execPath = os.Getenv("CLAUDECODE_BIN")
	}
	return NewClaudeBackend(execPath, slog.Default())
}

// NewPersistentBackend creates a PersistentBackend for the given provider type.
// Only "claude" and "local" support persistent sessions. Other provider types
// return an error — they should use the regular Backend interface instead.
func NewPersistentBackend(providerType string) (PersistentBackend, error) {
	switch providerType {
	case "claude", "local":
		return newClaudeBackendFromEnv(), nil
	default:
		return nil, fmt.Errorf("persistent backend not supported for provider %q (only claude/local)", providerType)
	}
}
