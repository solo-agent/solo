package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	ps, err := b.Start(ctx, req, opts)
	if err != nil {
		return nil, err
	}
	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)
	go func() {
		defer close(msgCh)
		defer close(resCh)
		defer b.Close(ps)
		for chunk := range ps.Messages {
			msgCh <- chunk
		}
		result := <-ps.Result
		resCh <- result
	}()
	var stopOnce sync.Once
	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     func() error { stopOnce.Do(func() { b.Close(ps) }); return nil },
	}, nil
}


// ── Persistent Backend (v1.5, ACP) ────────────────────────────────────────────

var opencodeBlockedACArgs = map[string]blockedArgMode{
	"--pure": blockedStandalone,
	"--cwd":  blockedWithValue,
}

type opencodePersistentState struct {
	runner    *persistentRunner
	client    *acpClient
	sessionID string
	turnFin   atomic.Bool
}

var _ SessionStater = (*opencodePersistentState)(nil)

func (s *opencodePersistentState) IsAlive() bool         { return s.runner.isAlive() }
func (s *opencodePersistentState) SessionID() string     { return s.sessionID }
func (s *opencodePersistentState) Done() <-chan struct{} { return s.runner.done }
func (s *opencodePersistentState) Notify(msg string) error {
	return s.runner.write([]byte(msg))
}

func (b *OpenCodeBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("opencode executable not found at %q: %w", execPath, err)
	}

	acpArgs := append([]string{"acp", "--pure"}, filterCustomArgs(opts.ExtraArgs, opencodeBlockedACArgs)...)
	acpArgs = append(acpArgs, filterCustomArgs(opts.CustomArgs, opencodeBlockedACArgs)...)
	if opts.WorkspaceDir != "" {
		acpArgs = append(acpArgs, "--cwd", opts.WorkspaceDir)
	}

	runner, err := startPersistent(ctx, execPath, acpArgs, opts.WorkspaceDir, opts.Env, b.logger)
	if err != nil {
		return nil, err
	}

	prompt := buildPrompt(req, opts)
	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	turnDone := make(chan acpPromptResult, 1)

	cl := &acpClient{
		logger:  b.logger,
		stdin:   runner.stdin,
		pending: make(map[int]*pendingRPC),
		onChunk: func(chunk OutputChunk) {
			if chunk.Type == string(MessageText) && chunk.Content != "" {
				outputMu.Lock()
				output.WriteString(chunk.Content)
				outputMu.Unlock()
			}
			trySend(msgCh, chunk)
		},
		onPromptDone: func(pr acpPromptResult) {
			select {
			case turnDone <- pr:
			default:
			}
		},
	}

	state := &opencodePersistentState{
		runner: runner,
		client: cl,
	}

	go func() {
		defer close(state.runner.done)
		scanner := bufio.NewScanner(runner.stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			cl.handleLine(line)
		}
		cl.closeAllPending(fmt.Errorf("opencode process exited"))
		if state.turnFin.CompareAndSwap(false, true) {
			resCh <- &Result{Status: "failed", Error: "opencode process exited unexpectedly"}
			close(msgCh)
			close(resCh)
		}
	}()

	startTime := time.Now()
	handleError := func(errMsg string) {
		if state.turnFin.CompareAndSwap(false, true) {
			resCh <- &Result{Status: "failed", Error: errMsg, DurationMs: time.Since(startTime).Milliseconds()}
			close(msgCh)
			close(resCh)
		}
	}

	if _, err := cl.request(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo":       map[string]any{"name": "solo-agent-sdk", "version": "1.0.0"},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		runner.close()
		handleError(fmt.Sprintf("opencode initialize failed: %v", err))
		return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
	}

	cwd := opts.WorkspaceDir
	if cwd == "" {
		cwd = "."
	}
	var sessionID string
	if opts.ResumeSessionID != "" {
		result, err := cl.request(ctx, "session/resume", map[string]any{
			"sessionId": opts.ResumeSessionID,
		})
		if err == nil {
			sessionID, _ = resolveResumedSessionID(opts.ResumeSessionID, result)
		}
	}
	if sessionID == "" {
		result, err := cl.request(ctx, "session/new", map[string]any{
			"cwd":        cwd,
			"mcpServers": []any{},
		})
		if err != nil {
			runner.close()
			handleError(fmt.Sprintf("opencode session/new failed: %v", err))
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
		sessionID = extractACPSessionID(result)
		if sessionID == "" {
			runner.close()
			handleError("opencode session/new returned no session ID")
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
	}
	cl.sessionID = sessionID
	state.sessionID = sessionID
	b.logger.Info("opencode: persistent session created", "session_id", sessionID)

	if opts.Model != "" {
		if _, err := cl.request(ctx, "session/set_model", map[string]any{
			"sessionId": sessionID,
			"modelId":   opts.Model,
		}); err != nil {
			b.logger.Warn("opencode: set_session_model failed", "error", err)
		}
	}

	promptBlocks := []map[string]any{
		{"type": "text", "text": prompt, "role": "user"},
	}
	if opts.SystemPrompt != "" {
		promptBlocks = append([]map[string]any{
			{"type": "text", "text": opts.SystemPrompt, "role": "system"},
		}, promptBlocks...)
	}

	if _, err := cl.request(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    promptBlocks,
	}); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			handleError("opencode timed out during initial prompt")
		} else if errors.Is(ctx.Err(), context.Canceled) {
			handleError("execution cancelled")
		} else {
			handleError(fmt.Sprintf("opencode session/prompt failed: %v", err))
		}
		return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
	}

	var usage TokenUsage
	select {
	case pr := <-turnDone:
		usage = pr.usage
	default:
	}

	duration := time.Since(startTime)
	outputMu.Lock()
	finalOutput := output.String()
	outputMu.Unlock()

	resCh <- &Result{
		Status:     "completed",
		Output:     finalOutput,
		DurationMs: duration.Milliseconds(),
		Usage:      buildHermesUsageMap(usage, opts.Model),
	}
	close(msgCh)
	close(resCh)
	state.turnFin.Store(true)

	b.logger.Info("opencode: initial persistent turn completed",
		"session_id", sessionID,
		"duration", duration.Round(time.Millisecond).String(),
	)

	var stopOnce sync.Once
	stop := func() error { stopOnce.Do(func() { runner.cancel() }); return nil }

	return &PersistentSession{
		Messages:  msgCh,
		Result:    resCh,
		Stop:      stop,
		SessionID: sessionID,
		state:     state,
	}, nil
}

func (b *OpenCodeBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("opencode: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("opencode: session process has exited")
	}

	prompt := buildPromptFromMessages(messages)
	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	turnDone := make(chan acpPromptResult, 1)

	state.client.onChunk = func(chunk OutputChunk) {
		if chunk.Type == string(MessageText) && chunk.Content != "" {
			outputMu.Lock()
			output.WriteString(chunk.Content)
			outputMu.Unlock()
		}
		trySend(msgCh, chunk)
	}
	state.client.onPromptDone = func(pr acpPromptResult) {
		select {
		case turnDone <- pr:
		default:
		}
	}

	startTime := time.Now()
	state.turnFin.Store(false)

	if _, err := state.client.request(ctx, "session/prompt", map[string]any{
		"sessionId": state.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		},
	}); err != nil {
		state.turnFin.Store(true)
		close(msgCh)
		close(resCh)
		return nil, fmt.Errorf("opencode persistent session/prompt: %w", err)
	}

	var usage TokenUsage
	select {
	case pr := <-turnDone:
		usage = pr.usage
	default:
	}

	duration := time.Since(startTime)
	outputMu.Lock()
	finalOutput := output.String()
	outputMu.Unlock()

	resCh <- &Result{
		Status:     "completed",
		Output:     finalOutput,
		DurationMs: duration.Milliseconds(),
		Usage:      buildHermesUsageMap(usage, ""),
	}
	close(msgCh)
	close(resCh)
	state.turnFin.Store(true)

	b.logger.Info("opencode: persistent turn completed via Send",
		"session_id", state.sessionID,
		"duration", duration.Round(time.Millisecond).String(),
	)

	var stopOnce2 sync.Once
	stop := func() error { stopOnce2.Do(func() {}); return nil }

	return &PersistentSession{
		Messages:  msgCh,
		Result:    resCh,
		Stop:      stop,
		SessionID: state.sessionID,
		state:     state,
	}, nil
}

func (b *OpenCodeBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return fmt.Errorf("opencode: invalid session state")
	}
	return state.runner.close()
}

// ── CLI argument construction ──

// buildOpenCodeArgs constructs the CLI argument vector for `opencode run --format json`.
//
// The prompt is always appended as the last positional argument so it is not
// accidentally consumed by any preceding flag (mirroring `buildOpenClawArgs`
// and `buildCodexArgs`).
//
// Blocked args (currently `--format`) are filtered out of ExtraArgs and
// CustomArgs to preserve the daemon's JSON-protocol assumption. When the
// same flag appears in both ExtraArgs and CustomArgs, both occurrences are
// appended — the last one wins in CLI parsing, which matches the
// documented behaviour relied on by opencode_test.go.
func buildOpenCodeArgs(prompt string, opts *ExecuteOptions) []string {
	args := []string{"run", "--format", "json"}
	if opts != nil {
		if opts.Model != "" {
			args = append(args, "--model", opts.Model)
		}
		if opts.SystemPrompt != "" {
			args = append(args, "--prompt", opts.SystemPrompt)
		}
		args = append(args, filterCustomArgs(opts.ExtraArgs, opencodeBlockedArgs)...)
		args = append(args, filterCustomArgs(opts.CustomArgs, opencodeBlockedArgs)...)
	}
	args = append(args, prompt)
	return args
}
