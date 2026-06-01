package agent

import (
	"fmt"
	"log/slog"
	"os"
)

// NewBackend creates a Backend for the given provider type.
// It delegates to the global BackendRegistry. Built-in adapters are
// registered by builtins.go via package init().
//
// Supported CLI backend types (registered by builtins.go):
//   - "claude"   — Claude Code CLI (binary resolved from env)
//   - "local"    — alias for claude (local CLI execution)
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
// The apiKey parameter is reserved for API-based providers and is
// forwarded through BackendConfig for future use. For CLI-based
// providers the binary path is resolved via environment variables
// (e.g. CODEX_BIN). Set the corresponding _BIN variable to override
// the default binary name.
func NewBackend(providerType, apiKey string) (Backend, error) {
	// openai and anthropic are not implemented as Backend. Their error
	// message is preserved for backward compatibility with existing callers.
	if providerType == "openai" || providerType == "anthropic" {
		return nil, fmt.Errorf("backend %q: not implemented via Backend interface; use llm.NewProvider instead", providerType)
	}

	cfg := BackendConfig{
		ProviderType: providerType,
		APIKey:       apiKey,
	}
	backend, err := GlobalRegistry().Create(providerType, cfg)
	if err != nil {
		// The registry error format is "unknown backend type: ...".
		// Wrap to the original format for backward compatibility with
		// callers that inspect the error string.
		return nil, fmt.Errorf("unknown backend provider type: %q (supported: claude, local, codex, cursor, gemini, kimi, kiro, copilot, opencode, openclaw, hermes, pi, openai, anthropic)", providerType)
	}
	return backend, nil
}

// newClaudeBackendFromEnv resolves the claude executable path from
// environment variables and constructs a ClaudeBackend. Priority:
// CLAUDE_BIN > CLAUDECODE_BIN > "claude" (PATH lookup).
//
// This function is kept for backward compatibility; new code should
// use GlobalRegistry().Create("claude", cfg) which goes through the
// claude factory registered in builtins.go.
func newClaudeBackendFromEnv() *ClaudeBackend {
	execPath := os.Getenv("CLAUDE_BIN")
	if execPath == "" {
		execPath = os.Getenv("CLAUDECODE_BIN")
	}
	return NewClaudeBackend(execPath, slog.Default())
}

// NewPersistentBackend creates a PersistentBackend for the given provider type.
// It delegates to the global BackendRegistry and checks whether the created
// Backend satisfies the PersistentBackend interface.
// Supported: claude, local, codex, opencode, openclaw, hermes.
func NewPersistentBackend(providerType string) (PersistentBackend, error) {
	backend, err := GlobalRegistry().Create(providerType, BackendConfig{ProviderType: providerType})
	if err != nil {
		return nil, err
	}
	pb, ok := backend.(PersistentBackend)
	if !ok {
		return nil, fmt.Errorf("persistent backend not supported for provider %q (supported: claude, local, codex, opencode, openclaw, hermes)", providerType)
	}
	return pb, nil
}
