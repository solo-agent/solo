package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// minOpenclawVersion is the lowest openclaw version that emits its
// --json result on stdout.
const minOpenclawVersion = "2026.5.5"

// openclawVersionPattern extracts a three-segment dotted version from
// `openclaw --version` output.
var openclawVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// openclawBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var openclawBlockedArgs = map[string]blockedArgMode{
	"--local":         blockedStandalone,
	"--json":          blockedStandalone,
	"--session-id":    blockedWithValue,
	"--message":       blockedWithValue,
	"--model":         blockedWithValue,
	"--system-prompt": blockedWithValue,
}

// OpenClawBackend implements Backend by spawning `openclaw agent --json`
// and reading streaming NDJSON events from stdout.
type OpenClawBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewOpenClawBackend creates a new OpenClawBackend.
// If executablePath is empty it defaults to "openclaw".
// If logger is nil, slog.Default() is used.
func NewOpenClawBackend(executablePath string, logger *slog.Logger) *OpenClawBackend {
	if executablePath == "" {
		executablePath = "openclaw"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OpenClawBackend{executablePath: executablePath, logger: logger}
}

// Name returns "openclaw".
func (b *OpenClawBackend) Name() string { return "openclaw" }

// Execute launches the openclaw CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *OpenClawBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("openclaw executable not found at %q: %w", execPath, err)
	}

	if err := checkOpenclawVersion(ctx, execPath); err != nil {
		return nil, err
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	sessionID := fmt.Sprintf("solo-%d", time.Now().UnixNano())
	args := buildOpenClawArgs(prompt, sessionID, opts)
	b.logger.Info("openclaw: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("openclaw: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[openclaw:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("openclaw: start: %w", err)
	}
	b.logger.Info("openclaw: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			cancel()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
		return nil
	}

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)

		startTime := time.Now()
		scanResult := b.processOutput(stdout, msgCh)

		go func() {
			<-runCtx.Done()
			_ = stdout.Close()
		}()

		exitErr := cmd.Wait()
		duration := time.Since(startTime)

		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			scanResult.status = "timeout"
			scanResult.errMsg = fmt.Sprintf("openclaw timed out after %s", timeout)
		} else if errors.Is(runCtx.Err(), context.Canceled) {
			scanResult.status = "cancelled"
			scanResult.errMsg = "execution cancelled"
		} else if exitErr != nil && scanResult.status == "completed" {
			scanResult.status = "failed"
			scanResult.errMsg = fmt.Sprintf("openclaw exited with error: %v", exitErr)
		}

		b.logger.Info("openclaw: finished",
			"status", scanResult.status,
			"session_id", scanResult.sessionID,
			"duration", duration.Round(time.Millisecond).String(),
		)

		var usage map[string]TokenUsage
		u := scanResult.usage
		if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
			model := opts.Model
			if model == "" {
				model = "unknown"
			}
			usage = map[string]TokenUsage{model: u}
		}

		resCh <- &Result{
			Status:     scanResult.status,
			Output:     scanResult.output,
			Error:      scanResult.errMsg,
			DurationMs: duration.Milliseconds(),
			Usage:      usage,
		}
	}()

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// ── Event processing ──

type openclawEventResult struct {
	status    string
	errMsg    string
	output    string
	sessionID string
	usage     TokenUsage
}

func (b *OpenClawBackend) processOutput(r io.Reader, ch chan<- OutputChunk) openclawEventResult {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var output strings.Builder
	var sessionID string
	var usage TokenUsage
	finalStatus := "completed"
	var finalError string
	gotEvents := false

	var rawLines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if event, ok := tryParseOpenclawEvent(line); ok {
			gotEvents = true
			if event.SessionID != "" {
				sessionID = event.SessionID
			}
			switch event.Type {
			case "text":
				if event.Text != "" {
					output.WriteString(event.Text)
					trySend(ch, OutputChunk{Type: string(MessageText), Content: event.Text})
				}
			case "tool_use":
				var input map[string]any
				if event.Input != nil {
					_ = json.Unmarshal(event.Input, &input)
				}
				trySend(ch, OutputChunk{
					Type: string(MessageToolUse),
					Tool: &ToolInfo{Name: event.Tool, CallID: event.CallID, Input: input},
				})
			case "tool_result":
				trySend(ch, OutputChunk{
					Type: string(MessageToolResult),
					Tool: &ToolInfo{CallID: event.CallID, Output: event.Text},
				})
			case "error":
				errMsg := event.errorMessage()
				b.logger.Warn("openclaw: error event", "error", errMsg)
				trySend(ch, OutputChunk{Type: string(MessageError), Content: errMsg})
				finalStatus = "failed"
				finalError = errMsg
			case "lifecycle":
				phase := event.Phase
				if phase == "error" || phase == "failed" || phase == "cancelled" {
					errMsg := event.errorMessage()
					b.logger.Warn("openclaw: lifecycle failure", "phase", phase, "error", errMsg)
					trySend(ch, OutputChunk{Type: string(MessageError), Content: errMsg})
					finalStatus = "failed"
					finalError = errMsg
				}
			case "step_start":
				trySend(ch, OutputChunk{Type: string(MessageStatus), Content: "running"})
			case "step_finish":
				if event.Usage != nil {
					u := parseOpenclawUsage(event.Usage)
					usage.InputTokens += u.InputTokens
					usage.OutputTokens += u.OutputTokens
					usage.CacheReadTokens += u.CacheReadTokens
					usage.CacheWriteTokens += u.CacheWriteTokens
				}
			}
			continue
		}

		if result, ok := tryParseOpenclawResult(line); ok {
			gotEvents = true
			res := b.buildOpenclawResult(result, ch, &output)
			if res.sessionID != "" {
				sessionID = res.sessionID
			}
			u := res.usage
			if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
				usage = u
			}
			continue
		}

		b.logger.Debug("[openclaw:stdout] " + line)
		rawLines = append(rawLines, line)
	}

	if err := scanner.Err(); err != nil {
		return openclawEventResult{status: "failed", errMsg: fmt.Sprintf("read stdout: %v", err)}
	}

	if !gotEvents {
		trimmed := strings.TrimSpace(strings.Join(rawLines, "\n"))
		if trimmed != "" {
			if result, ok := tryParseOpenclawResult(trimmed); ok {
				return b.buildOpenclawResult(result, ch, &output)
			}
			for i, line := range rawLines {
				if len(line) > 0 && line[0] == '{' {
					candidate := strings.TrimSpace(strings.Join(rawLines[i:], "\n"))
					if result, ok := tryParseOpenclawResult(candidate); ok {
						return b.buildOpenclawResult(result, ch, &output)
					}
					break
				}
			}
			return openclawEventResult{status: "completed", output: trimmed}
		}
		return openclawEventResult{status: "failed", errMsg: "openclaw returned no parseable output"}
	}

	return openclawEventResult{
		status:    finalStatus,
		errMsg:    finalError,
		output:    output.String(),
		sessionID: sessionID,
		usage:     usage,
	}
}

func tryParseOpenclawEvent(line string) (openclawEvent, bool) {
	if len(line) == 0 || line[0] != '{' {
		return openclawEvent{}, false
	}
	var event openclawEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return openclawEvent{}, false
	}
	if event.Type == "" {
		return openclawEvent{}, false
	}
	return event, true
}

func tryParseOpenclawResult(raw string) (openclawResult, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return openclawResult{}, false
	}
	var result openclawResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return openclawResult{}, false
	}
	if result.Payloads == nil && result.Meta.DurationMs == 0 {
		return openclawResult{}, false
	}
	return result, true
}

func (b *OpenClawBackend) buildOpenclawResult(result openclawResult, ch chan<- OutputChunk, output *strings.Builder) openclawEventResult {
	for _, p := range result.Payloads {
		if p.Text != "" {
			output.WriteString(p.Text)
			trySend(ch, OutputChunk{Type: string(MessageText), Content: p.Text})
		}
	}

	var sessionID string
	var usage TokenUsage
	if result.Meta.AgentMeta != nil {
		if sid, ok := result.Meta.AgentMeta["sessionId"].(string); ok {
			sessionID = sid
		}
		if u, ok := result.Meta.AgentMeta["usage"].(map[string]any); ok {
			usage = parseOpenclawUsage(u)
		}
	}

	return openclawEventResult{
		status:    "completed",
		output:    output.String(),
		sessionID: sessionID,
		usage:     usage,
	}
}

func parseOpenclawUsage(data map[string]any) TokenUsage {
	return TokenUsage{
		InputTokens:      openclawInt64FirstOf(data, "input", "inputTokens", "input_tokens"),
		OutputTokens:     openclawInt64FirstOf(data, "output", "outputTokens", "output_tokens"),
		CacheReadTokens:  openclawInt64FirstOf(data, "cacheRead", "cachedInputTokens", "cached_input_tokens", "cache_read", "cache_read_input_tokens"),
		CacheWriteTokens: openclawInt64FirstOf(data, "cacheWrite", "cacheCreationInputTokens", "cache_creation_input_tokens", "cache_write"),
	}
}

func openclawInt64FirstOf(data map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v := openclawInt64(data, key); v != 0 {
			return v
		}
	}
	return 0
}

func openclawInt64(data map[string]any, key string) int64 {
	v, ok := data[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

// ── OpenClaw JSON types ──

type openclawEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId,omitempty"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callId,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Usage     map[string]any  `json:"usage,omitempty"`
	Phase     string          `json:"phase,omitempty"`
	Error     *openclawErrObj `json:"error,omitempty"`
	Message   string          `json:"message,omitempty"`
}

func (e openclawEvent) errorMessage() string {
	if e.Error != nil {
		if msg := e.Error.message(); msg != "" {
			return msg
		}
	}
	if e.Text != "" {
		return e.Text
	}
	if e.Message != "" {
		return e.Message
	}
	return "unknown openclaw error"
}

type openclawErrObj struct {
	Name    string              `json:"name,omitempty"`
	Data    *openclawErrorData  `json:"data,omitempty"`
	Message string              `json:"message,omitempty"`
}

func (e *openclawErrObj) message() string {
	if e.Data != nil && e.Data.Message != "" {
		return e.Data.Message
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Name != "" {
		return e.Name
	}
	return ""
}

type openclawErrorData struct {
	Message string `json:"message,omitempty"`
}

type openclawResult struct {
	Payloads []openclawPayload `json:"payloads"`
	Meta     openclawMeta      `json:"meta"`
}

type openclawPayload struct {
	Text string `json:"text"`
}

type openclawMeta struct {
	DurationMs int64          `json:"durationMs"`
	AgentMeta  map[string]any `json:"agentMeta"`
}

// ── Version check ──

func checkOpenclawVersion(ctx context.Context, execPath string) error {
	cmd := exec.CommandContext(ctx, execPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openclaw --version failed: %w", err)
	}
	detected, ok := parseOpenclawVersion(string(out))
	if !ok {
		return fmt.Errorf("could not parse openclaw version from output: %q", strings.TrimSpace(string(out)))
	}
	if compareOpenclawVersion(detected, minOpenclawVersion) < 0 {
		return fmt.Errorf("openclaw %s is below the minimum supported version %s. Run `openclaw update` to upgrade and try again.", detected, minOpenclawVersion)
	}
	return nil
}

func parseOpenclawVersion(raw string) (string, bool) {
	m := openclawVersionPattern.FindString(raw)
	if m == "" {
		return "", false
	}
	return m, true
}

func compareOpenclawVersion(a, b string) int {
	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		ai, _ := strconv.Atoi(aParts[i])
		bi, _ := strconv.Atoi(bParts[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// ── CLI argument construction ──

func buildOpenClawArgs(prompt, sessionID string, opts *ExecuteOptions) []string {
	args := []string{
		"agent",
		"--local",
		"--json",
		"--session-id", sessionID,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		prompt = opts.SystemPrompt + "\n\n" + prompt
	}
	args = append(args, "--message", prompt)
	args = append(args, filterCustomArgs(opts.CustomArgs, openclawBlockedArgs)...)
	return args
}
