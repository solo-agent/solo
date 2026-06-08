package agent

import (
	"log/slog"
	"os"
)

// init registers all built-in backend adapters into the global registry
// when the agent package is imported. Each adapter carries its metadata
// (display name, binary requirements, protocols) and a factory function
// that constructs Backend instances from BackendConfig.
func init() {
	r := GlobalRegistry()

	// ── claude — Claude Code CLI via stream-json ──────────────────────
	r.Register(claudeMeta("claude", "Claude Code"), claudeFactory)

	// ── local — alias for claude (local execution) ───────────────────
	r.Register(claudeMeta("local", "Claude Code (Local)"), claudeFactory)

	// ── codex — Codex CLI via JSON-RPC ───────────────────────────────
	r.Register(AdapterMeta{
		Type:           "codex",
		DisplayName:    "Codex CLI",
		RequiresBinary: "codex",
		DetectCommand:  "--version",
		Protocols:      []string{"json-rpc"},
	}, codexFactory)

	// ── opencode — OpenCode CLI via stream-json ─────────────────────
	r.Register(AdapterMeta{
		Type:           "opencode",
		DisplayName:    "OpenCode CLI",
		RequiresBinary: "opencode",
		DetectCommand:  "--version",
		Protocols:      []string{"stream-json"},
	}, opencodeFactory)

	// ── cursor — Cursor Agent CLI via stream-json ───────────────────
	r.Register(AdapterMeta{
		Type:           "cursor",
		DisplayName:    "Cursor Agent",
		RequiresBinary: "cursor-agent",
		DetectCommand:  "--version",
		Protocols:      []string{"stream-json"},
	}, cursorFactory)

	// ── gemini — Google Gemini CLI via stream-json ──────────────────
	r.Register(AdapterMeta{
		Type:           "gemini",
		DisplayName:    "Gemini CLI",
		RequiresBinary: "gemini",
		DetectCommand:  "--version",
		Protocols:      []string{"stream-json"},
	}, geminiFactory)

	// ── kimi — Kimi CLI via ACP ─────────────────────────────────────
	r.Register(AdapterMeta{
		Type:           "kimi",
		DisplayName:    "Kimi CLI",
		RequiresBinary: "kimi",
		DetectCommand:  "--version",
		Protocols:      []string{"acp"},
	}, kimiFactory)

	// ── kiro — Kiro CLI via ACP ─────────────────────────────────────
	r.Register(AdapterMeta{
		Type:           "kiro",
		DisplayName:    "Kiro CLI",
		RequiresBinary: "kiro-cli",
		DetectCommand:  "--version",
		Protocols:      []string{"acp"},
	}, kiroFactory)

	// ── copilot — GitHub Copilot CLI via JSONL ──────────────────────
	r.Register(AdapterMeta{
		Type:           "copilot",
		DisplayName:    "GitHub Copilot",
		RequiresBinary: "copilot",
		DetectCommand:  "--version",
		Protocols:      []string{"jsonl"},
	}, copilotFactory)

	// ── openclaw — OpenClaw Agent CLI via ACP ───────────────────────
	r.Register(AdapterMeta{
		Type:           "openclaw",
		DisplayName:    "OpenClaw Agent",
		RequiresBinary: "openclaw",
		DetectCommand:  "--version",
		Protocols:      []string{"acp"},
	}, openclawFactory)

	// ── hermes — Hermes CLI via ACP ─────────────────────────────────
	r.Register(AdapterMeta{
		Type:           "hermes",
		DisplayName:    "Hermes CLI",
		RequiresBinary: "hermes",
		DetectCommand:  "--version",
		Protocols:      []string{"acp"},
	}, hermesFactory)

	// ── pi — Pi CLI via JSONL ───────────────────────────────────────
	r.Register(AdapterMeta{
		Type:           "pi",
		DisplayName:    "Pi CLI",
		RequiresBinary: "pi",
		DetectCommand:  "--version",
		Protocols:      []string{"jsonl"},
	}, piFactory)
}

// claudeMeta builds an AdapterMeta for the claude and local backends.
// They share the same metadata except for their Type and DisplayName fields.
func claudeMeta(typ, displayName string) AdapterMeta {
	return AdapterMeta{
		Type:           typ,
		DisplayName:    displayName,
		RequiresBinary: "claude",
		DetectCommand:  "--version",
		Protocols:      []string{"stream-json"},
	}
}

// ── Factory functions ────────────────────────────────────────────────────────

// logOrDefault returns logger if non-nil, otherwise slog.Default.
func logOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

// execPathOrDefault resolves the binary path from cfg.ExecPath or an
// environment variable. If both are empty the constructor will fall
// back to its own default (typically the binary name).
func execPathOrDefault(cfgExecPath, envVar string) string {
	if cfgExecPath != "" {
		return cfgExecPath
	}
	return os.Getenv(envVar)
}

func claudeFactory(cfg BackendConfig) (Backend, error) {
	execPath := cfg.ExecPath
	if execPath == "" {
		execPath = os.Getenv("CLAUDE_BIN")
	}
	if execPath == "" {
		execPath = os.Getenv("CLAUDECODE_BIN")
	}
	return NewClaudeBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func codexFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "CODEX_BIN")
	return NewCodexBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func opencodeFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "OPENCODE_BIN")
	return NewOpenCodeBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func cursorFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "CURSOR_BIN")
	return NewCursorBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func geminiFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "GEMINI_BIN")
	return NewGeminiBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func kimiFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "KIMI_BIN")
	return NewKimiBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func kiroFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "KIRO_BIN")
	return NewKiroBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func copilotFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "COPILOT_BIN")
	return NewCopilotBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func openclawFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "OPENCLAW_BIN")
	return NewOpenClawBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func hermesFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "HERMES_BIN")
	return NewHermesBackend(execPath, logOrDefault(cfg.Logger)), nil
}

func piFactory(cfg BackendConfig) (Backend, error) {
	execPath := execPathOrDefault(cfg.ExecPath, "PI_BIN")
	return NewPiBackend(execPath, logOrDefault(cfg.Logger)), nil
}
