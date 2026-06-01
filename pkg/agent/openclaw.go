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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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

// ── Persistent Backend (v1.4) ──────────────────────────────────────────────────

// openclawPersistentState holds the runtime state of a long-running OpenClaw
// subprocess across multiple NDJSON turns.
type openclawPersistentState struct {
	runner    *persistentRunner
	turn      atomic.Pointer[openclawTurnState]
	sessionID string
}

// Compile-time check.
var _ SessionStater = (*openclawPersistentState)(nil)

func (s *openclawPersistentState) IsAlive() bool            { return s.runner.isAlive() }
func (s *openclawPersistentState) SessionID() string        { return s.sessionID }
func (s *openclawPersistentState) Done() <-chan struct{}    { return s.runner.done }
func (s *openclawPersistentState) Notify(msg string) error  { return s.runner.write([]byte(msg)) }

type openclawTurnState struct {
	id    string
	msgCh chan OutputChunk
	resCh chan *Result
}

// buildOpenClawPersistentArgs constructs CLI arguments for a persistent
// openclaw session. The initial prompt is written to stdin instead of
// being passed as a CLI flag.
func buildOpenClawPersistentArgs(sessionID string, opts *ExecuteOptions) []string {
	args := []string{"agent", "--local", "--json", "--session-id", sessionID}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, filterCustomArgs(opts.ExtraArgs, openclawBlockedArgs)...)
	args = append(args, filterCustomArgs(opts.CustomArgs, openclawBlockedArgs)...)
	return args
}

// Start creates a persistent OpenClaw session. The process stays alive across
// multiple turns with full conversation context preserved via stdin/stdout.
func (b *OpenClawBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("openclaw executable not found at %q: %w", execPath, err)
	}
	if err := checkOpenclawVersion(ctx, execPath); err != nil {
		return nil, err
	}

	sessionID := uuid.New().String()
	args := buildOpenClawPersistentArgs(sessionID, opts)
	b.logger.Info("openclaw: starting persistent session", "args", args, "session_id", sessionID)

	runner, err := startPersistent(ctx, execPath, args, opts.WorkspaceDir, buildEnv(opts.Env), b.logger)
	if err != nil {
		return nil, err
	}

	prompt := buildPrompt(req, opts)
	if err := runner.write([]byte(prompt + "\n")); err != nil {
		runner.close()
		return nil, fmt.Errorf("openclaw: write initial prompt: %w", err)
	}

	state := &openclawPersistentState{
		runner:    runner,
		sessionID: sessionID,
	}

	turn := &openclawTurnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	go b.openclawPersistentLoop(state)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() { runner.cancel() })
		return nil
	}

	return &PersistentSession{
		Messages:  turn.msgCh,
		Result:    turn.resCh,
		Stop:      stop,
		SessionID: sessionID,
		state:     state,
	}, nil
}

// Send delivers new messages to a running persistent OpenClaw session.
func (b *OpenClawBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*openclawPersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("openclaw: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("openclaw: session process has exited")
	}

	var promptBuilder strings.Builder
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			continue
		}
		promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	turn := &openclawTurnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	if err := state.runner.write([]byte(promptBuilder.String() + "\n")); err != nil {
		state.turn.Store(nil)
		return nil, fmt.Errorf("openclaw: write send input: %w", err)
	}

	b.logger.Info("openclaw: turn started via Send", "turn_id", turn.id, "session_id", state.sessionID)

	var stopOnce sync.Once
	stop := func() error { stopOnce.Do(func() {}); return nil }

	return &PersistentSession{
		Messages:  turn.msgCh,
		Result:    turn.resCh,
		Stop:      stop,
		SessionID: state.sessionID,
		state:     state,
	}, nil
}

// Close terminates the persistent OpenClaw session.
func (b *OpenClawBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*openclawPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("openclaw: invalid session state")
	}
	return state.runner.close()
}

// openclawPersistentLoop reads OpenClaw's stdout NDJSON across multiple turns.
// Each result object (non-event JSON) signals the end of the current turn.
func (b *OpenClawBackend) openclawPersistentLoop(state *openclawPersistentState) {
	defer close(state.runner.done)

	scanner := bufio.NewScanner(state.runner.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	startTime := time.Now()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		turn := state.turn.Load()

		// Try event first (streaming NDJSON).
		if event, ok := tryParseOpenclawEvent(line); ok {
			if event.SessionID != "" {
				// SessionID is already set from persistent args; keep for
				// eventual consistency with the event stream.
			}
			switch event.Type {
			case "text":
				if event.Text != "" && turn != nil {
					trySend(turn.msgCh, OutputChunk{Type: string(MessageText), Content: event.Text})
				}
			case "tool_use":
				if turn != nil {
					var input map[string]any
					if event.Input != nil {
						_ = json.Unmarshal(event.Input, &input)
					}
					trySend(turn.msgCh, OutputChunk{
						Type: string(MessageToolUse),
						Tool: &ToolInfo{Name: event.Tool, CallID: event.CallID, Input: input},
					})
				}
			case "tool_result":
				if turn != nil {
					trySend(turn.msgCh, OutputChunk{
						Type: string(MessageToolResult),
						Tool: &ToolInfo{CallID: event.CallID, Output: event.Text},
					})
				}
			case "error":
				if turn != nil {
					errMsg := event.errorMessage()
					b.logger.Warn("openclaw: error event in persistent loop", "error", errMsg)
					trySend(turn.msgCh, OutputChunk{Type: string(MessageError), Content: errMsg})
				}
			case "lifecycle":
				if turn != nil {
					phase := event.Phase
					if phase == "error" || phase == "failed" || phase == "cancelled" {
						errMsg := event.errorMessage()
						b.logger.Warn("openclaw: lifecycle failure in persistent loop", "phase", phase, "error", errMsg)
						trySend(turn.msgCh, OutputChunk{Type: string(MessageError), Content: errMsg})
					}
				}
			case "step_start":
				if turn != nil {
					trySend(turn.msgCh, OutputChunk{Type: string(MessageStatus), Content: "running"})
				}
			case "step_finish":
				// Per-step usage is tracked for informational purposes.
			}
			continue
		}

		// Try result object — signals turn completion.
		if result, ok := tryParseOpenclawResult(line); ok {
			if turn != nil {
				// Send any remaining payload text.
				for _, p := range result.Payloads {
					if p.Text != "" {
						trySend(turn.msgCh, OutputChunk{Type: string(MessageText), Content: p.Text})
					}
				}
				duration := time.Since(startTime)
				turn.resCh <- &Result{
					Status:     "completed",
					DurationMs: duration.Milliseconds(),
				}
				close(turn.msgCh)
				close(turn.resCh)
				state.turn.Store(nil)
				startTime = time.Now()
			}
			continue
		}

		b.logger.Debug("[openclaw:stdout] " + line)
	}

	// Scanner error or EOF — close any remaining turn.
	turn := state.turn.Load()
	if turn != nil {
		turn.resCh <- &Result{Status: "failed", Error: "openclaw process exited unexpectedly"}
		close(turn.msgCh)
		close(turn.resCh)
		state.turn.Store(nil)
	}
}
