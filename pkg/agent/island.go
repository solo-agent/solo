package agent

import (
	"fmt"
	"strings"

	"github.com/solo-ai/solo/internal/i18n"
)

// ============================================================================
// Run status inference
//
// Translates internal OutputChunk events into the canonical agent run status
// used by agent_runs.status and agent.run.* WebSocket events.
// ============================================================================

type RunStatus string

const (
	RunStatusQueued          RunStatus = "queued"
	RunStatusThinking        RunStatus = "thinking"
	RunStatusRunning         RunStatus = "running"
	RunStatusStreaming       RunStatus = "streaming"
	RunStatusWaitingInput    RunStatus = "waiting_input"
	RunStatusWaitingApproval RunStatus = "waiting_approval"
	RunStatusCompleted       RunStatus = "completed"
	RunStatusFailed          RunStatus = "failed"
	RunStatusCancelled       RunStatus = "cancelled"
	RunStatusTimeout         RunStatus = "timeout"
)

// InferRunStatusFromChunk maps an OutputChunk onto a run status. The bool is
// false for chunks that should not update run status.
func InferRunStatusFromChunk(chunk OutputChunk) (RunStatus, bool) {
	switch chunk.Type {
	case string(MessageThinking):
		return RunStatusThinking, true
	case string(MessageText):
		return RunStatusStreaming, true
	case string(MessageToolUse):
		return RunStatusRunning, true
	case string(MessageToolResult):
		return RunStatusRunning, true
	case string(MessageError):
		return RunStatusFailed, true
	}
	return "", false
}

// InferActivityText returns a short Chinese summary suitable for the island
// pill (e.g. "调用 Bash", "Bash 完成", "思考中…"). Returns "" for chunks we
// don't surface to the UI — caller should skip pushing the activity event
// in that case.
//
// This is the generic, backend-agnostic path. For per-CLI nuances
// (tool-name casing, family-specific event types) prefer
// InferActivityTextForBackend.
func InferActivityText(chunk OutputChunk) string {
	switch chunk.Type {
	case string(MessageThinking):
		return i18n.Active.PillThinking
	case string(MessageText):
		return i18n.Active.PillGenerating
	case string(MessageToolUse):
		if chunk.Tool != nil && chunk.Tool.Name != "" {
			return fmt.Sprintf(i18n.Active.PillCallingTool, chunk.Tool.Name)
		}
		return i18n.Active.PillUsingTool
	case string(MessageToolResult):
		if chunk.Tool != nil && chunk.Tool.IsError {
			return fmt.Sprintf(i18n.Active.PillToolFailed, chunk.Tool.Name)
		}
		if chunk.Tool != nil && chunk.Tool.Name != "" {
			return fmt.Sprintf(i18n.Active.PillToolDone, chunk.Tool.Name)
		}
		return i18n.Active.PillToolResult
	case string(MessageError):
		return i18n.Active.PillError
	}
	return ""
}

// ============================================================================
// Per-backend / per-family adaptation
// ============================================================================
//
// Solo currently ships 12 CLI backends grouped into 3 protocol families:
// stream-json (Claude / OpenCode / Cursor / Gemini / OpenClaw), jsonl
// (Copilot / Pi), and acp (Kimi / Kiro / Hermes). Codex is technically
// JSON-RPC but its backend implementation already emits the canonical
// OutputChunk types so it falls through to the generic path.
//
// The differences that matter to the island pill are:
//   - Tool name casing/format (e.g. ACP emits "Bash" / "bash" / "SHELL"
//     depending on the underlying CLI)
//   - Family-specific event vocabulary (e.g. ACP's `agent_thought_chunk`
//     maps to our MessageThinking but its tool_call name uses snake_case)
//
// The generic InferActivityText gets the right *type* (calling /
// completed) but the tool name might surface inconsistently. This
// function normalises that.
// ============================================================================

// Protocol-family classification, keyed by the backend type string
// registered in builtin.go. Used as the dispatch for per-family
// normalisations.
const (
	familyStreamJSON = "stream-json" // claude, local, opencode, cursor, gemini, openclaw
	familyJSONL      = "jsonl"       // copilot, pi
	familyACP        = "acp"         // kimi, kiro, hermes
	familyOther      = "other"       // codex (already emits canonical OutputChunk) + unknown
)

func backendFamily(provider string) string {
	switch provider {
	case "claude", "local", "opencode", "cursor", "gemini", "openclaw":
		return familyStreamJSON
	case "copilot", "pi":
		return familyJSONL
	case "kimi", "kiro", "hermes":
		return familyACP
	default:
		return familyOther
	}
}

// NormalizeToolName canonicalises a raw tool name from a backend so the
// island pill surfaces a consistent label across families. Currently
// handles:
//   - ACP family: trims the `mcp__` / `acp__` prefixes some backends add
//   - Stream-json family: trims "default_api:" namespace (Kiro/Codex wrap)
//   - All families: title-cases the first letter (so "bash" → "Bash")
func NormalizeToolName(provider, rawName string) string {
	if rawName == "" {
		return ""
	}
	name := rawName
	// Strip backend-specific prefixes that don't carry user-meaning.
	for _, prefix := range []string{"mcp__", "acp__", "default_api:", "builtin_"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	// ACP family uses snake_case tool names; convert to TitleCase for
	// UI consistency (run_command → Run_command — readable enough).
	if backendFamily(provider) == familyACP {
		parts := strings.Split(name, "_")
		for i, p := range parts {
			if p == "" {
				continue
			}
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
		name = strings.Join(parts, "_")
	}
	return name
}

// InferActivityTextForBackend is the per-backend variant. It routes to
// the right family normaliser first, then falls back to the generic
// InferActivityText. The structure mirrors Kanan's per-CLI dispatch
// pattern: family-specific work where it matters, generic everywhere else.
func InferActivityTextForBackend(provider string, chunk OutputChunk) string {
	family := backendFamily(provider)

	// Family-specific event vocabulary translation. Today the chunk
	// types are already canonicalised by each backend's factory, so
	// most cases fall through to the generic path. We keep the switch
	// as the seam for future per-family special events (e.g. an
	// ACP-specific "approval_requested" state) without changing the
	// caller signature.
	switch family {
	case familyStreamJSON, familyJSONL:
		// Both already produce OutputChunk with normalised Tool.Name.
		// We apply tool-name canonicalisation for display only.
		if chunk.Tool != nil && chunk.Tool.Name != "" {
			return inferActivityTextWithToolName(chunk, NormalizeToolName(provider, chunk.Tool.Name))
		}
		return InferActivityText(chunk)

	case familyACP:
		// ACP backends surface camelCase or snake_case tool names
		// inconsistently. We always normalise before display so the
		// user sees the same "Bash" pill whether the agent is running
		// on Kimi, Kiro, or Hermes.
		if chunk.Tool != nil && chunk.Tool.Name != "" {
			return inferActivityTextWithToolName(chunk, NormalizeToolName(provider, chunk.Tool.Name))
		}
		return InferActivityText(chunk)

	default:
		// Codex + unknown — fall back to the generic path.
		return InferActivityText(chunk)
	}
}

// inferActivityTextWithToolName is the shared "given a chunk + a
// pre-normalised tool name, return the right activity text" helper.
// Keeps InferActivityTextForBackend and InferActivityText in sync
// without duplicating the switch.
func inferActivityTextWithToolName(chunk OutputChunk, name string) string {
	switch chunk.Type {
	case string(MessageThinking):
		return i18n.Active.PillThinking
	case string(MessageText):
		return i18n.Active.PillGenerating
	case string(MessageToolUse):
		if name != "" {
			return fmt.Sprintf(i18n.Active.PillCallingTool, name)
		}
		return i18n.Active.PillUsingTool
	case string(MessageToolResult):
		if chunk.Tool != nil && chunk.Tool.IsError {
			return fmt.Sprintf(i18n.Active.PillToolFailed, name)
		}
		if name != "" {
			return fmt.Sprintf(i18n.Active.PillToolDone, name)
		}
		return i18n.Active.PillToolResult
	case string(MessageError):
		return i18n.Active.PillError
	}
	return ""
}

// SummarizeToolInput returns a one-line summary of a tool invocation, e.g.
// "Bash: npm test" or "Edit: src/auth.ts". Looks up the canonical key for
// each major tool category (command / file_path / path / query /
// description). Falls back to just the tool name when no input is present.
func SummarizeToolInput(toolName string, input map[string]any) string {
	if toolName == "" {
		return ""
	}
	if len(input) == 0 {
		return toolName
	}

	// Probe order matches how Solo's CLI agents populate the input object.
	for _, key := range []string{"command", "cmd"} {
		if v, ok := input[key].(string); ok && v != "" {
			return fmt.Sprintf("%s: %s", toolName, truncateRunes(v, 40))
		}
	}
	for _, key := range []string{"file_path", "filePath", "path"} {
		if v, ok := input[key].(string); ok && v != "" {
			return fmt.Sprintf("%s: %s", toolName, truncateRunes(v, 40))
		}
	}
	for _, key := range []string{"query", "pattern", "description", "url"} {
		if v, ok := input[key].(string); ok && v != "" {
			return fmt.Sprintf("%s: %s", toolName, truncateRunes(v, 40))
		}
	}
	return toolName
}

// truncateRunes cuts a string at max runes (not bytes) and appends an
// ellipsis when truncated. Safe for multi-byte CJK content.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "…"
}
