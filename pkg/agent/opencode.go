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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// opencodeBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var opencodeBlockedArgs = map[string]blockedArgMode{
	"--format": blockedWithValue, // json output format for daemon communication
}

// OpenCodeBackend implements Backend by spawning `opencode run --format json`
// and reading streaming JSON events from stdout.
type OpenCodeBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewOpenCodeBackend creates a new OpenCodeBackend.
// If executablePath is empty it defaults to "opencode".
// If logger is nil, slog.Default() is used.
func NewOpenCodeBackend(executablePath string, logger *slog.Logger) *OpenCodeBackend {
	if executablePath == "" {
		executablePath = "opencode"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OpenCodeBackend{executablePath: executablePath, logger: logger}
}

// Name returns "opencode".
func (b *OpenCodeBackend) Name() string { return "opencode" }

// Execute launches the opencode CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *OpenCodeBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("opencode executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	// P1: semantic inactivity timeout entry point (not yet wired to an
	// ActivityTracker for OpenCode). Reserved for future implementation.
	if opts.SemanticInactivityTimeout > 0 {
		b.logger.Debug("opencode: semantic inactivity timeout configured (not yet implemented)",
			"timeout", opts.SemanticInactivityTimeout)
	}

	prompt := buildPrompt(req, opts)
	args := buildOpenCodeArgs(prompt, opts)
	b.logger.Info("opencode: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)
	// Auto-approve all tool use.
	cmd.Env = append(cmd.Env, `OPENCODE_PERMISSION={"*":"allow"}`)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("opencode: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[opencode:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("opencode: start: %w", err)
	}
	b.logger.Info("opencode: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

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
		scanResult := b.processEvents(stdout, msgCh)

		// Close stdout when the context is cancelled so the scanner unblocks.
		go func() {
			<-runCtx.Done()
			_ = stdout.Close()
		}()

		exitErr := cmd.Wait()
		duration := time.Since(startTime)

		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			scanResult.status = "timeout"
			scanResult.errMsg = fmt.Sprintf("opencode timed out after %s", timeout)
		} else if errors.Is(runCtx.Err(), context.Canceled) {
			scanResult.status = "cancelled"
			scanResult.errMsg = "execution cancelled"
		} else if exitErr != nil && scanResult.status == "completed" {
			scanResult.status = "failed"
			scanResult.errMsg = fmt.Sprintf("opencode exited with error: %v", exitErr)
		}

		b.logger.Info("opencode: finished",
			"status", scanResult.status,
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

type opencodeEventResult struct {
	status    string
	errMsg    string
	output    string
	sessionID string
	usage     TokenUsage
}

func (b *OpenCodeBackend) processEvents(r io.Reader, ch chan<- OutputChunk) opencodeEventResult {
	var output strings.Builder
	var sessionID string
	var usage TokenUsage
	finalStatus := "completed"
	var finalError string

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event opencodeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.SessionID != "" {
			sessionID = event.SessionID
		}

		switch event.Type {
		case "text":
			b.handleText(event, ch, &output)
		case "tool_use":
			b.handleToolUse(event, ch)
		case "error":
			b.handleError(event, ch, &finalStatus, &finalError)
		case "step_start":
			trySend(ch, OutputChunk{Type: string(MessageStatus), Content: "running"})
		case "step_finish":
			if t := event.Part.Tokens; t != nil {
				usage.InputTokens += t.Input
				usage.OutputTokens += t.Output
				if t.Cache != nil {
					usage.CacheReadTokens += t.Cache.Read
					usage.CacheWriteTokens += t.Cache.Write
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		b.logger.Warn("opencode: stdout scanner error", "error", err)
		if finalStatus == "completed" {
			finalStatus = "failed"
			finalError = fmt.Sprintf("stdout read error: %v", err)
		}
	}

	return opencodeEventResult{
		status:    finalStatus,
		errMsg:    finalError,
		output:    output.String(),
		sessionID: sessionID,
		usage:     usage,
	}
}

func (b *OpenCodeBackend) handleText(event opencodeEvent, ch chan<- OutputChunk, output *strings.Builder) {
	text := event.Part.Text
	if text != "" {
		output.WriteString(text)
		trySend(ch, OutputChunk{Type: string(MessageText), Content: text})
	}
}

func (b *OpenCodeBackend) handleToolUse(event opencodeEvent, ch chan<- OutputChunk) {
	var input map[string]any
	if event.Part.State != nil && event.Part.State.Input != nil {
		_ = json.Unmarshal(event.Part.State.Input, &input)
	}

	trySend(ch, OutputChunk{
		Type: string(MessageToolUse),
		Tool: &ToolInfo{Name: event.Part.Tool, CallID: event.Part.CallID, Input: input},
	})

	if event.Part.State != nil && event.Part.State.Status == "completed" {
		outputStr := extractToolOutput(event.Part.State.Output)
		trySend(ch, OutputChunk{
			Type: string(MessageToolResult),
			Tool: &ToolInfo{Name: event.Part.Tool, CallID: event.Part.CallID, Output: outputStr},
		})
	}
}

func (b *OpenCodeBackend) handleError(event opencodeEvent, ch chan<- OutputChunk, finalStatus, finalError *string) {
	errMsg := ""
	if event.Error != nil {
		errMsg = event.Error.ErrorString()
	}
	if errMsg == "" {
		errMsg = "unknown opencode error"
	}

	b.logger.Warn("opencode: error event", "error", errMsg)
	trySend(ch, OutputChunk{Type: string(MessageError), Content: errMsg})

	*finalStatus = "failed"
	*finalError = errMsg
}

func extractToolOutput(output any) string {
	if output == nil {
		return ""
	}
	if s, ok := output.(string); ok {
		return s
	}
	data, _ := json.Marshal(output)
	return string(data)
}

// ── OpenCode JSON types ──

type opencodeEvent struct {
	Type      string            `json:"type"`
	Timestamp int64             `json:"timestamp,omitempty"`
	SessionID string            `json:"sessionID,omitempty"`
	Part      opencodeEventPart `json:"part"`
	Error     *opencodeError    `json:"error,omitempty"`
}

type opencodeEventPart struct {
	ID        string `json:"id,omitempty"`
	MessageID string `json:"messageID,omitempty"`
	SessionID string `json:"sessionID,omitempty"`
	Type      string `json:"type,omitempty"`

	Text string `json:"text,omitempty"`

	Tool   string             `json:"tool,omitempty"`
	CallID string             `json:"callID,omitempty"`
	State  *opencodeToolState `json:"state,omitempty"`

	Tokens *opencodeTokens `json:"tokens,omitempty"`
}

type opencodeToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output any             `json:"output"`
}

type opencodeTokens struct {
	Input  int64                `json:"input"`
	Output int64                `json:"output"`
	Cache  *opencodeCacheTokens `json:"cache,omitempty"`
}

type opencodeCacheTokens struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
}

type opencodeError struct {
	Name    string             `json:"name,omitempty"`
	Data    *opencodeErrorData `json:"data,omitempty"`
	Msg     string             `json:"message,omitempty"`
}

func (e *opencodeError) ErrorString() string {
	if e.Data != nil && e.Data.Message != "" {
		return e.Data.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Name != "" {
		return e.Name
	}
	return ""
}

type opencodeErrorData struct {
	Message string `json:"message,omitempty"`
}

// ── Persistent Backend (v1.4) ──────────────────────────────────────────────────

// opencodePersistentState holds the runtime state of a long-running OpenCode
// subprocess across multiple turns.
type opencodePersistentState struct {
	runner    *persistentRunner
	turn      atomic.Pointer[opencodeTurnState]
	sessionID string
}

// Compile-time check: opencodePersistentState implements SessionStater.
var _ SessionStater = (*opencodePersistentState)(nil)

func (s *opencodePersistentState) IsAlive() bool            { return s.runner.isAlive() }
func (s *opencodePersistentState) SessionID() string        { return s.sessionID }
func (s *opencodePersistentState) Done() <-chan struct{}    { return s.runner.done }
func (s *opencodePersistentState) Notify(msg string) error  { return s.runner.write([]byte(msg)) }

type opencodeTurnState struct {
	id    string
	msgCh chan OutputChunk
	resCh chan *Result
}

func buildOpenCodePersistentArgs(opts *ExecuteOptions) []string {
	args := []string{"run", "--format", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, filterCustomArgs(opts.ExtraArgs, opencodeBlockedArgs)...)
	args = append(args, filterCustomArgs(opts.CustomArgs, opencodeBlockedArgs)...)
	return args
}

// Start creates a persistent OpenCode session. The process stays alive across
// multiple turns with full conversation context preserved via stdin/stdout.
func (b *OpenCodeBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	args := buildOpenCodePersistentArgs(opts)
	b.logger.Info("opencode: starting persistent session", "args", args)

	env := buildEnv(opts.Env)
	env = append(env, `OPENCODE_PERMISSION={"*":"allow"}`)

	runner, err := startPersistent(ctx, b.executablePath, args, opts.WorkspaceDir, env, b.logger)
	if err != nil {
		return nil, err
	}

	prompt := buildPrompt(req, opts)
	if err := runner.write([]byte(prompt + "\n")); err != nil {
		runner.close()
		return nil, fmt.Errorf("opencode: write initial prompt: %w", err)
	}

	state := &opencodePersistentState{
		runner: runner,
	}

	turn := &opencodeTurnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	go b.opencodePersistentLoop(state)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() { runner.cancel() })
		return nil
	}

	return &PersistentSession{
		Messages: turn.msgCh,
		Result:   turn.resCh,
		Stop:     stop,
		state:    state,
	}, nil
}

// Send delivers new messages to a running persistent OpenCode session.
func (b *OpenCodeBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("opencode: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("opencode: session process has exited")
	}

	var promptBuilder strings.Builder
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			continue
		}
		promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	turn := &opencodeTurnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	if err := state.runner.write([]byte(promptBuilder.String() + "\n")); err != nil {
		state.turn.Store(nil)
		return nil, fmt.Errorf("opencode: write send input: %w", err)
	}

	b.logger.Info("opencode: turn started via Send", "turn_id", turn.id, "session_id", state.sessionID)

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

// Close terminates the persistent OpenCode session.
func (b *OpenCodeBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return fmt.Errorf("opencode: invalid session state")
	}
	return state.runner.close()
}

// opencodePersistentLoop reads OpenCode's stdout across multiple turns.
// Each "result" event closes the current turn's channels.
func (b *OpenCodeBackend) opencodePersistentLoop(state *opencodePersistentState) {
	defer close(state.runner.done)

	scanner := bufio.NewScanner(state.runner.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	startTime := time.Now()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event opencodeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.SessionID != "" && state.sessionID == "" {
			state.sessionID = event.SessionID
		}

		turn := state.turn.Load()
		if turn == nil {
			continue
		}

		switch event.Type {
		case "text":
			turn.msgCh <- OutputChunk{Type: string(MessageText), Content: event.Part.Text}
		case "tool_use":
			tool := &ToolInfo{Name: event.Part.Tool, CallID: event.Part.CallID}
			if event.Part.State != nil && len(event.Part.State.Input) > 0 {
				var input map[string]any
				if json.Unmarshal(event.Part.State.Input, &input) == nil {
					tool.Input = input
				}
			}
			turn.msgCh <- OutputChunk{Type: string(MessageToolUse), Tool: tool}
		case "tool_result":
			var resultContent string
			if event.Part.State != nil && event.Part.State.Output != nil {
				resultContent = fmt.Sprint(event.Part.State.Output)
			}
			turn.msgCh <- OutputChunk{
				Type:    string(MessageToolResult),
				Content: resultContent,
				Tool:    &ToolInfo{Name: event.Part.Tool, CallID: event.Part.CallID},
			}
		case "result":
			duration := time.Since(startTime)
			turn.resCh <- &Result{
				Status:     "completed",
				DurationMs: duration.Milliseconds(),
			}
			close(turn.msgCh)
			close(turn.resCh)
			state.turn.Store(nil)
		}
	}

	// Scanner error or EOF — close any remaining turn
	turn := state.turn.Load()
	if turn != nil {
		turn.resCh <- &Result{Status: "failed", Error: "opencode process exited unexpectedly"}
		close(turn.msgCh)
		close(turn.resCh)
		state.turn.Store(nil)
	}
}

// ── CLI argument construction ──

func buildOpenCodeArgs(prompt string, opts *ExecuteOptions) []string {
	args := []string{"run", "--format", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--prompt", opts.SystemPrompt)
	}
	// Daemon-level ExtraArgs first, then agent-level CustomArgs can override.
	args = append(args, filterCustomArgs(opts.ExtraArgs, opencodeBlockedArgs)...)
	args = append(args, filterCustomArgs(opts.CustomArgs, opencodeBlockedArgs)...)
	args = append(args, prompt)
	return args
}
