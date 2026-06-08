package agent

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/solo-ai/solo/internal/i18n"
)

// ============================================================================
// Island inference (SOLO-island PR1 + PR-fix)
//
// Pure-function tests for the three island helpers. They run with no
// dependencies and pin the contract that the daemon relies on to push
// agent.activity events and the frontend relies on to render the
// AgentIsland pill. Treat any failure here as a wire-format break.
// ============================================================================

// ---- InferIslandStatusFromChunk ----

func TestInferIslandStatusFromChunk(t *testing.T) {
	cases := []struct {
		name  string
		chunk OutputChunk
		want  IslandStatus
	}{
		{
			name:  "thinking → thinking",
			chunk: OutputChunk{Type: string(MessageThinking)},
			want:  IslandStatusThinking,
		},
		{
			name:  "text → streaming",
			chunk: OutputChunk{Type: string(MessageText)},
			want:  IslandStatusStreaming,
		},
		{
			name:  "tool_use → running",
			chunk: OutputChunk{Type: string(MessageToolUse)},
			want:  IslandStatusRunning,
		},
		{
			name:  "tool_result success → running (continue)",
			chunk: OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "Bash", IsError: false},
			},
			want: IslandStatusRunning,
		},
		{
			name:  "tool_result error → error",
			chunk: OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "Bash", IsError: true},
			},
			want: IslandStatusError,
		},
		{
			name:  "error → error",
			chunk: OutputChunk{Type: string(MessageError)},
			want:  IslandStatusError,
		},
		{
			name:  "status (no-op) → idle",
			chunk: OutputChunk{Type: string(MessageStatus)},
			want:  IslandStatusIdle,
		},
		{
			name:  "unknown type → idle",
			chunk: OutputChunk{Type: "wat"},
			want:  IslandStatusIdle,
		},
		{
			name:  "empty type → idle",
			chunk: OutputChunk{Type: ""},
			want:  IslandStatusIdle,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferIslandStatusFromChunk(tc.chunk)
			if got != tc.want {
				t.Errorf("InferIslandStatusFromChunk(%s) = %q, want %q", tc.chunk.Type, got, tc.want)
			}
		})
	}
}

// ---- InferActivityText ----

func TestInferActivityText(t *testing.T) {
	cases := []struct {
		name  string
		chunk OutputChunk
		want  string
	}{
		{
			name:  "thinking → thinking...",
			chunk: OutputChunk{Type: string(MessageThinking)},
			want:  i18n.Active.PillThinking,
		},
		{
			name:  "text → generating...",
			chunk: OutputChunk{Type: string(MessageText)},
			want:  i18n.Active.PillGenerating,
		},
		{
			name: "tool_use with name → calling <name>",
			chunk: OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "Bash"},
			},
			want: fmt.Sprintf(i18n.Active.PillCallingTool, "Bash"),
		},
		{
			name:  "tool_use without name → using tool",
			chunk: OutputChunk{Type: string(MessageToolUse)},
			want:  i18n.Active.PillUsingTool,
		},
		{
			name: "tool_result success → <name> done",
			chunk: OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "Edit", IsError: false},
			},
			want: fmt.Sprintf(i18n.Active.PillToolDone, "Edit"),
		},
		{
			name: "tool_result error → <name> failed",
			chunk: OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "Edit", IsError: true},
			},
			want: fmt.Sprintf(i18n.Active.PillToolFailed, "Edit"),
		},
		{
			name:  "tool_result without name → tool result",
			chunk: OutputChunk{Type: string(MessageToolResult)},
			want:  i18n.Active.PillToolResult,
		},
		{
			name:  "error → error",
			chunk: OutputChunk{Type: string(MessageError)},
			want:  i18n.Active.PillError,
		},
		{
			name:  "status → empty (caller skips push)",
			chunk: OutputChunk{Type: string(MessageStatus)},
			want:  "",
		},
		{
			name:  "unknown → empty (caller skips push)",
			chunk: OutputChunk{Type: "wat"},
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferActivityText(tc.chunk)
			if got != tc.want {
				t.Errorf("InferActivityText(%s) = %q, want %q", tc.chunk.Type, got, tc.want)
			}
		})
	}
}

// ---- SummarizeToolInput ----

func TestSummarizeToolInput(t *testing.T) {
	cases := []struct {
		name     string
		toolName string
		input    map[string]any
		want     string
	}{
		{
			name:     "empty tool name → empty",
			toolName: "",
			input:    map[string]any{"command": "ls"},
			want:     "",
		},
		{
			name:     "no input → just tool name",
			toolName: "Bash",
			input:    nil,
			want:     "Bash",
		},
		{
			name:     "empty input map → just tool name",
			toolName: "Bash",
			input:    map[string]any{},
			want:     "Bash",
		},
		{
			name:     "command (Bash)",
			toolName: "Bash",
			input:    map[string]any{"command": "npm test"},
			want:     "Bash: npm test",
		},
		{
			name:     "cmd alias",
			toolName: "Bash",
			input:    map[string]any{"cmd": "ls -la"},
			want:     "Bash: ls -la",
		},
		{
			name:     "file_path (Edit)",
			toolName: "Edit",
			input:    map[string]any{"file_path": "src/auth.ts"},
			want:     "Edit: src/auth.ts",
		},
		{
			name:     "filePath camelCase fallback",
			toolName: "Edit",
			input:    map[string]any{"filePath": "src/auth.ts"},
			want:     "Edit: src/auth.ts",
		},
		{
			name:     "path (Read)",
			toolName: "Read",
			input:    map[string]any{"path": "package.json"},
			want:     "Read: package.json",
		},
		{
			name:     "query (Glob)",
			toolName: "Glob",
			input:    map[string]any{"query": "**/*.go"},
			want:     "Glob: **/*.go",
		},
		{
			name:     "pattern (Grep)",
			toolName: "Grep",
			input:    map[string]any{"pattern": "TODO"},
			want:     "Grep: TODO",
		},
		{
			name:     "unknown keys → fallback to tool name",
			toolName: "CustomTool",
			input:    map[string]any{"foo": "bar"},
			want:     "CustomTool",
		},
		{
			name:     "non-string value → skip, fallback to tool name",
			toolName: "Bash",
			input:    map[string]any{"command": 42},
			want:     "Bash",
		},
		{
			name:     "empty string value → skip, fallback to tool name",
			toolName: "Bash",
			input:    map[string]any{"command": ""},
			want:     "Bash",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SummarizeToolInput(tc.toolName, tc.input)
			if got != tc.want {
				t.Errorf("SummarizeToolInput(%q, %v) = %q, want %q", tc.toolName, tc.input, got, tc.want)
			}
		})
	}
}

// ---- truncateRunes (CJK safety is the critical case here) ----

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"under limit stays unchanged", "hello", 10, "hello"},
		{"at limit stays unchanged", "hello", 5, "hello"},
		{"over limit ASCII truncates with ellipsis", "hello world", 5, "hell…"},
		{"CJK under limit", "你好世界", 4, "你好世界"},
		{"CJK over limit truncates by rune, not byte", "你好世界", 3, "你好…"},
		{"max=0 returns input unchanged", "abc", 0, "abc"},
		{"max=1 returns single rune", "你好", 1, "你"},
		{"single CJK char fits", "你", 1, "你"},
		{"empty input returns empty", "", 5, ""},
		// CJK safety: if we used byte-slicing instead of rune-aware,
		// "你好世界"[0:6] would corrupt the first char (cut in the
		// middle of a multi-byte sequence). Verify that doesn't happen.
		{"byte-unsafe slice would corrupt CJK", "你好世界", 3, "你好…"},
		// CJK char is 3 bytes in UTF-8. With byte-slicing [0:6], the
		// result would be "你" + half a rune → invalid UTF-8. Our
	// rune-aware version correctly returns "你好…".
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.input, tc.max)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
			}
			// Every result must be valid UTF-8.
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes(%q, %d) produced invalid UTF-8: %q", tc.input, tc.max, got)
			}
		})
	}
}

// ---- Integration: status + activity text derive the same status ----

// Sanity: InferIslandStatusFromChunk and InferActivityText should
// agree that a non-empty activity text corresponds to a non-idle
// status. If one of them drifts, the frontend's pill will look
// incoherent (e.g. "Bash running" with an idle status dot).
func TestInferStatusAndActivityText_Consistency(t *testing.T) {
	chunks := []OutputChunk{
		{Type: string(MessageThinking)},
		{Type: string(MessageText)},
		{Type: string(MessageToolUse), Tool: &ToolInfo{Name: "Bash"}},
		{Type: string(MessageToolUse), Tool: &ToolInfo{Name: "Edit"}},
		{Type: string(MessageToolResult), Tool: &ToolInfo{Name: "Bash", IsError: true}},
		{Type: string(MessageError)},
	}
	for _, chunk := range chunks {
		status := InferIslandStatusFromChunk(chunk)
		text := InferActivityText(chunk)
		// Every non-idle status should produce a non-empty activity text,
		// otherwise the daemon will push an activity event with status=idle
		// which the frontend will treat as a completed flash.
		if status != IslandStatusIdle && text == "" {
			t.Errorf("chunk %+v: status=%q but activity text is empty", chunk, status)
		}
	}
}

// ---- backendFamily ----

func TestBackendFamily(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"claude", familyStreamJSON},
		{"local", familyStreamJSON},
		{"opencode", familyStreamJSON},
		{"cursor", familyStreamJSON},
		{"gemini", familyStreamJSON},
		{"openclaw", familyStreamJSON},
		{"copilot", familyJSONL},
		{"pi", familyJSONL},
		{"kimi", familyACP},
		{"kiro", familyACP},
		{"hermes", familyACP},
		{"codex", familyOther},
		{"unknown-future-backend", familyOther},
		{"", familyOther},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			if got := backendFamily(tc.provider); got != tc.want {
				t.Errorf("backendFamily(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}

// ---- NormalizeToolName ----

func TestNormalizeToolName(t *testing.T) {
	cases := []struct {
		name     string
		provider  string
		raw      string
		want     string
	}{
		// Empty input — no work, no output.
		{"empty stays empty", "kimi", "", ""},

		// Stream-json family (Claude / OpenCode / Cursor / Gemini /
		// OpenClaw) — strip the "default_api:" namespace some backends
		// wrap tool calls with. Title-case is a no-op for already-
		// capitalised names.
		{"stream-json already canonical", "claude", "Bash", "Bash"},
		{"stream-json strips default_api", "kiro", "default_api:Bash", "Bash"},
		{"stream-json strips builtin_", "opencode", "builtin_WebFetch", "WebFetch"},

		// JSONL family (Copilot / Pi) — same strip rules, no case
		// change expected (these backends emit TitleCase names).
		{"jsonl already canonical", "copilot", "Read", "Read"},

		// ACP family (Kimi / Kiro / Hermes) — strip mcp__/acp__/
		// builtin_ prefixes, AND Title-case the snake_case tool
		// names that ACP backends typically emit. Each underscore-
		// separated word gets its first letter capitalised (standard
		// Title Case): run_command → Run_Command, mcp__run_shell
		// → Run_Shell.
		{"acp strips mcp__", "kimi", "mcp__Bash", "Bash"},
		{"acp strips acp__", "kiro", "acp__Read", "Read"},
		{"acp snake_case → Title_Case", "hermes", "run_command", "Run_Command"},
		{"acp snake_case with prefix", "kimi", "mcp__run_shell", "Run_Shell"},

		// Unknown backend — pass through unchanged.
		{"unknown passthrough", "codex", "apply_patch", "apply_patch"},
		{"unknown with prefix passthrough", "codex", "default_api:apply_patch", "apply_patch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeToolName(tc.provider, tc.raw); got != tc.want {
				t.Errorf("NormalizeToolName(%q, %q) = %q, want %q", tc.provider, tc.raw, got, tc.want)
			}
		})
	}
}

// ---- InferActivityTextForBackend ----

func TestInferActivityTextForBackend_StreamJSON(t *testing.T) {
	// Stream-json family (Claude, OpenCode, Cursor, Gemini, OpenClaw).
	// Behaviour mirrors InferActivityText for these backends since
	// they already emit canonical OutputChunk.
	for _, provider := range []string{"claude", "local", "opencode", "cursor", "gemini", "openclaw"} {
		t.Run(provider+"/tool_use normalises tool name", func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "default_api:Bash"},
			}
			got := InferActivityTextForBackend(provider, chunk)
			if got != fmt.Sprintf(i18n.Active.PillCallingTool, "Bash") {
				t.Errorf("got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Bash"))
			}
		})
	}
}

func TestInferActivityTextForBackend_ACP(t *testing.T) {
	for _, provider := range []string{"kimi", "kiro", "hermes"} {
		t.Run(provider+"/tool_use normalises snake_case", func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "mcp__run_shell"},
			}
			// Strip prefix: mcp__run_shell → run_shell
			// Then ACP Title-Case: run_shell → Run_Shell
			got := InferActivityTextForBackend(provider, chunk)
			if got != fmt.Sprintf(i18n.Active.PillCallingTool, "Run_Shell") {
				t.Errorf("got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Run_Shell"))
			}
		})

		t.Run(provider+"/tool_result normalises snake_case", func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "acp__Read", IsError: false},
			}
			// Strip acp__ → Read (already TitleCase)
			got := InferActivityTextForBackend(provider, chunk)
			if got != fmt.Sprintf(i18n.Active.PillToolDone, "Read") {
				t.Errorf("got %q, want %q", got, fmt.Sprintf(i18n.Active.PillToolDone, "Read"))
			}
		})

		t.Run(provider+"/tool_result error normalises snake_case", func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "run_command", IsError: true},
			}
			got := InferActivityTextForBackend(provider, chunk)
			if got != fmt.Sprintf(i18n.Active.PillToolFailed, "Run_Command") {
				t.Errorf("got %q, want %q", got, fmt.Sprintf(i18n.Active.PillToolFailed, "Run_Command"))
			}
		})
	}
}

func TestInferActivityTextForBackend_JSONL(t *testing.T) {
	for _, provider := range []string{"copilot", "pi"} {
		t.Run(provider+"/tool_use passthrough", func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "Read"},
			}
			got := InferActivityTextForBackend(provider, chunk)
			if got != fmt.Sprintf(i18n.Active.PillCallingTool, "Read") {
				t.Errorf("got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Read"))
			}
		})
	}
}

func TestInferActivityTextForBackend_Other(t *testing.T) {
	// codex + unknown — generic path.
	chunk := OutputChunk{
		Type: string(MessageToolUse),
		Tool: &ToolInfo{Name: "Bash"},
	}
	if got := InferActivityTextForBackend("codex", chunk); got != fmt.Sprintf(i18n.Active.PillCallingTool, "Bash") {
		t.Errorf("codex: got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Bash"))
	}
	if got := InferActivityTextForBackend("unknown", chunk); got != fmt.Sprintf(i18n.Active.PillCallingTool, "Bash") {
		t.Errorf("unknown: got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Bash"))
	}
	if got := InferActivityTextForBackend("", chunk); got != fmt.Sprintf(i18n.Active.PillCallingTool, "Bash") {
		t.Errorf("empty provider: got %q, want %q", got, fmt.Sprintf(i18n.Active.PillCallingTool, "Bash"))
	}
}

func TestInferActivityTextForBackend_NonToolChunks(t *testing.T) {
	// Thinking / text / error should be the same regardless of provider.
	for _, provider := range []string{"claude", "kiro", "copilot", "codex", ""} {
		t.Run(provider+"/thinking", func(t *testing.T) {
			chunk := OutputChunk{Type: string(MessageThinking)}
			if got := InferActivityTextForBackend(provider, chunk); got != i18n.Active.PillThinking {
				t.Errorf("got %q, want %q", got, i18n.Active.PillThinking)
			}
		})
		t.Run(provider+"/text", func(t *testing.T) {
			chunk := OutputChunk{Type: string(MessageText)}
			if got := InferActivityTextForBackend(provider, chunk); got != i18n.Active.PillGenerating {
				t.Errorf("got %q, want %q", got, i18n.Active.PillGenerating)
			}
		})
		t.Run(provider+"/error", func(t *testing.T) {
			chunk := OutputChunk{Type: string(MessageError)}
			if got := InferActivityTextForBackend(provider, chunk); got != i18n.Active.PillError {
				t.Errorf("got %q, want %q", got, i18n.Active.PillError)
			}
		})
	}
}

func TestInferActivityTextForBackend_EmptyToolName(t *testing.T) {
	// Tool_use with empty tool name should fall back to i18n.Active.PillUsingTool
	// across all providers.
	for _, provider := range []string{"claude", "kimi", "copilot"} {
		t.Run(provider, func(t *testing.T) {
			chunk := OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: ""},
			}
			if got := InferActivityTextForBackend(provider, chunk); got != i18n.Active.PillUsingTool {
				t.Errorf("got %q, want %q", got, i18n.Active.PillUsingTool)
			}
		})
	}
}
