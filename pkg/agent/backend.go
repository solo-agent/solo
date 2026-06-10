// Package agent defines the core types and interfaces for the Solo Agent
// execution layer. It provides a unified Backend interface that wraps CLI
// agent subprocesses (Claude Code, Codex, etc.) via stdin/stdout pipes,
package agent

import (
	"context"
	"time"
)

// Backend is the unified interface for executing prompts via coding agents.
// Each implementation wraps a specific CLI agent (e.g. "claude", "codex")
// launched as a subprocess with stdin/stdout pipes.
type Backend interface {
	// Execute runs an agent task and returns a Session for streaming results.
	// The caller should read from Session.Messages for streaming events and
	// wait on Session.Result for the final outcome. Session.Stop allows
	// cancellation mid-execution.
	Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error)

	// Name returns the backend provider name (e.g. "claude", "codex").
	Name() string
}

// ExecuteRequest contains the context and input for a single agent execution.
type ExecuteRequest struct {
	AgentID  string    `json:"agent_id"`
	Messages []Message `json:"messages"`
}

// ExecuteOptions configures a single execution.
type ExecuteOptions struct {
	SystemPrompt string            `json:"system_prompt,omitempty"`
	WorkspaceDir string            `json:"workspace_dir,omitempty"`
	Model        string            `json:"model,omitempty"`
	Effort           string            `json:"effort,omitempty"`             // thinking effort level (Claude Code --effort, OpenClaw --thinking)
	MaxTurns         int               `json:"max_turns,omitempty"`          // max agentic turns (Claude Code --max-turns)
	ResumeSessionID  string            `json:"resume_session_id,omitempty"`  // ACP session/resume ID for protocol-level resume
	MaxTokens        int               `json:"max_tokens,omitempty"`
	Temperature      float64           `json:"temperature,omitempty"`
	Env                       map[string]string `json:"env,omitempty"`
	CustomArgs                []string          `json:"custom_args,omitempty"`
	ExtraArgs                 []string          `json:"extra_args,omitempty"`                   // daemon-level global default args
	SemanticInactivityTimeout time.Duration     `json:"semantic_inactivity_timeout,omitempty"`  // 0 = disabled
}

// Session represents a running agent execution.
// Callers consume Messages for streaming output and wait on Result for
// the final outcome. Stop cancels the execution and terminates the subprocess.
type Session struct {
	// Messages streams events as the agent works. The channel is closed
	// when the agent finishes (before Result is sent).
	Messages <-chan OutputChunk
	// Result receives exactly one value — the final outcome — then closes.
	Result <-chan *Result
	// Stop cancels the running execution. It is safe to call multiple times.
	Stop func() error
}

// OutputChunk is a single event emitted by an agent during execution.
type OutputChunk struct {
	Type    string    `json:"type"`            // text, thinking, tool_use, tool_result, status, error
	Content string    `json:"content"`         // text content for text/thinking/error types
	Tool    *ToolInfo `json:"tool,omitempty"`  // tool call information
}

// ToolInfo represents a tool call or tool result emitted by an agent.
type ToolInfo struct {
	Name    string         `json:"name"`
	Input   map[string]any `json:"input,omitempty"`
	Output  string         `json:"output,omitempty"`
	CallID  string         `json:"call_id,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}

// MessageType identifies the kind of a streaming output event.
// These are emitted as the Type field of OutputChunk.
type MessageType string

const (
	MessageText       MessageType = "text"
	MessageThinking   MessageType = "thinking"
	MessageToolUse    MessageType = "tool_use"
	MessageToolResult MessageType = "tool_result"
	MessageStatus     MessageType = "status"
	MessageError      MessageType = "error"
)

// TriggerType identifies what triggered an agent execution.
// This is used by PromptBuilder and WorkspaceManager.InjectConfig
// to generate context-appropriate prompts and runtime configuration.
type TriggerType string

const (
	TriggerChat    TriggerType = "chat"    // Normal channel message
	TriggerMention TriggerType = "mention" // @mention trigger
	TriggerDM      TriggerType = "dm"      // Direct message
	TriggerThread  TriggerType = "thread"  // Thread reply
)

// Result is the final outcome after an agent session completes.
// It is delivered through the Session.Result channel.
type Result struct {
	Status     string                 `json:"status"`               // "completed", "failed", "aborted", "timeout", "cancelled"
	Output     string                 `json:"output"`               // accumulated text output
	Error      string                 `json:"error,omitempty"`      // error message if failed
	DurationMs int64                  `json:"duration_ms"`          // execution duration in milliseconds
	Usage      map[string]TokenUsage  `json:"usage,omitempty"`      // token usage keyed by model name
	// InitInfo carries backend-specific session initialization metadata —
	// e.g. the Claude Code `system/init` event payload (model name, loaded
	// MCP servers with their status, plugin errors). Keyed by field name.
	// Backends that do not emit init metadata leave this nil.
	InitInfo map[string]any `json:"init_info,omitempty"`
}

// TokenUsage tracks token consumption for a single LLM model invocation.
type TokenUsage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
}

// ── Persistent Backend ───────────────────────────────────────────────────────

// PersistentBackend extends Backend with session-level lifecycle methods.
// Implementations keep the subprocess alive across multiple Send() calls,
// maintaining full conversation context. agents are long-running colleagues, not one-shot text generators.
// agents are long-running colleagues, not one-shot text generators.
type PersistentBackend interface {
	Backend

	// Start creates a persistent session. The subprocess stays alive after
	// the initial prompt, waiting for additional input on stdin. Callers
	// consume Messages for streaming events and wait on Result for the turn
	// outcome. The session remains valid until Close is called.
	Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error)

	// Send delivers new messages to a running session. Returns a new
	// PersistentSession whose Messages and Result channels reflect this turn.
	// Callers must finish consuming the previous turn's Messages and Result
	// before calling Send.
	Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error)

	// Close terminates the persistent session. It closes stdin, waits for
	// the subprocess to exit, and cleans up resources.
	Close(ps *PersistentSession) error
}

// PersistentSession represents a multi-turn agent subprocess session.
// Each call to Start or Send produces a new PersistentSession with fresh
// Messages and Result channels for that turn. The Stop function cancels
// the current turn without killing the underlying subprocess.
type PersistentSession struct {
	// Messages streams events for the current turn. Closed when the turn
	// completes (before Result is sent).
	Messages <-chan OutputChunk

	// Result receives exactly one value — the turn's final outcome — then closes.
	Result <-chan *Result

	// Stop cancels the current turn. It is safe to call multiple times.
	// It does NOT kill the underlying subprocess.
	Stop func() error

	// SessionID is the Claude Code session identifier, preserved across turns.
	SessionID string

	// internal state handle — unexported, type-asserted by implementations.
	state any
}

// SessionStater exposes session metadata and control to AgentSessionManager
// without coupling to a specific backend's persistent state type.
// Any PersistentBackend that wants to participate in the session pool,
// crash recovery, and inbox notification must have its persistent state
// implement this interface.
type SessionStater interface {
	// IsAlive returns true if the subprocess is still running.
	IsAlive() bool
	// SessionID returns the CLI session identifier for --resume.
	SessionID() string
	// Done returns a channel that closes when the session is terminated.
	Done() <-chan struct{}
	// Notify writes a lightweight notification to the agent's stdin.
	Notify(msg string) error
}
