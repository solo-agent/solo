package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// hermesBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var hermesBlockedArgs = map[string]blockedArgMode{
	"acp": blockedStandalone,
}

// HermesBackend implements Backend by spawning `hermes acp` and communicating
// via the ACP (Agent Communication Protocol) JSON-RPC 2.0 over stdin/stdout.
// This is the same pattern as Kimi and Kiro backends.
type HermesBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewHermesBackend creates a new HermesBackend.
// If executablePath is empty it defaults to "hermes".
// If logger is nil, slog.Default() is used.
func NewHermesBackend(executablePath string, logger *slog.Logger) *HermesBackend {
	if executablePath == "" {
		executablePath = "hermes"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HermesBackend{executablePath: executablePath, logger: logger}
}

// Name returns "hermes".
func (b *HermesBackend) Name() string { return "hermes" }

// Execute launches the hermes CLI subprocess, sends the prompt via ACP,
// streams output events through Session.Messages, and delivers the final
// result on Session.Result.
func (b *HermesBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("hermes executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	hermesArgs := append([]string{"acp"}, filterCustomArgs(opts.CustomArgs, hermesBlockedArgs)...)

	cmd := exec.CommandContext(runCtx, execPath, hermesArgs...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnvAt(opts.WorkspaceDir, opts.Env)
	cmd.Env = append(cmd.Env, "HERMES_YOLO_MODE=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("hermes: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("hermes: stdin pipe: %w", err)
	}

	providerErr := newACPProviderErrorSniffer("hermes")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("hermes: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("hermes: start: %w", err)
	}

	stderrSink := io.MultiWriter(newLogWriter(b.logger, "[hermes:stderr] "), providerErr)
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(stderrSink, stderr)
	}()

	b.logger.Info("hermes: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	var streamingCurrentTurn bool
	promptDone := make(chan acpPromptResult, 1)

	cl := &acpClient{
		logger:  b.logger,
		stdin:   stdin,
		pending: make(map[int]*pendingRPC),
		onChunk: func(chunk OutputChunk) {
			if !streamingCurrentTurn {
				return
			}
			if chunk.Type == string(MessageText) && chunk.Content != "" {
				outputMu.Lock()
				output.WriteString(chunk.Content)
				outputMu.Unlock()
			}
			trySend(msgCh, chunk)
		},
		onPromptDone: func(result acpPromptResult) {
			if !streamingCurrentTurn {
				return
			}
			select {
			case promptDone <- result:
			default:
			}
		},
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			cl.handleLine(line)
		}
		cl.closeAllPending(fmt.Errorf("hermes process exited"))
	}()

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
		defer func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		}()

		startTime := time.Now()
		finalStatus := "completed"
		var finalError string
		var sessionID string

		// 1. Initialize handshake.
		_, err := cl.request(runCtx, "initialize", map[string]any{
			"protocolVersion": 1,
			"clientInfo": map[string]any{
				"name":    "solo-agent-sdk",
				"version": "1.0.0",
			},
			"clientCapabilities": map[string]any{},
		})
		if err != nil {
			finalStatus = "failed"
			finalError = fmt.Sprintf("hermes initialize failed: %v", err)
			resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
			return
		}

		// 2. Create or resume session.
		cwd := opts.WorkspaceDir
		if cwd == "" {
			cwd = "."
		}
		if opts.ResumeSessionID != "" {
			result, err := cl.request(runCtx, "session/resume", map[string]any{
				"sessionId": opts.ResumeSessionID,
			})
			if err == nil {
				sessionID, _ = resolveResumedSessionID(opts.ResumeSessionID, result)
			}
		}
		if sessionID == "" {
			result, err := cl.request(runCtx, "session/new", buildHermesSessionParams(cwd, opts.Model))
			if err != nil {
				finalStatus = "failed"
				finalError = fmt.Sprintf("hermes session/new failed: %v", err)
				resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
				return
			}
			sessionID = extractACPSessionID(result)
			if sessionID == "" {
				finalStatus = "failed"
				finalError = "hermes session/new returned no session ID"
				resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
				return
			}
		}

		cl.sessionID = sessionID
		b.logger.Info("hermes: session created", "session_id", sessionID)
		trySend(msgCh, OutputChunk{Type: string(MessageStatus), Content: "running", SessionID: sessionID})

		// 3. Set model if specified.
		if opts.Model != "" {
			if _, err := cl.request(runCtx, "session/set_model", map[string]any{
				"sessionId": sessionID,
				"modelId":   opts.Model,
			}); err != nil {
				b.logger.Warn("hermes: set_session_model failed", "error", err, "requested_model", opts.Model)
				finalStatus = "failed"
				finalError = fmt.Sprintf("hermes could not switch to model %q: %v", opts.Model, err)
				resCh <- &Result{
					Status:     finalStatus,
					Error:      finalError,
					DurationMs: time.Since(startTime).Milliseconds(),
				}
				return
			}
			b.logger.Info("hermes: session model set", "model", opts.Model)
		}

		// 4. Build prompt blocks with role-based format so the agent
		// treats system instructions as authoritative.
		promptBlocks := []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		}
		if opts.SystemPrompt != "" {
			promptBlocks = append([]map[string]any{
				{"type": "text", "text": opts.SystemPrompt, "role": "system"},
			}, promptBlocks...)
		}

		// 5. Send the prompt and wait for PromptResponse.
		streamingCurrentTurn = true
		_, err = cl.request(runCtx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    promptBlocks,
		})
		if err != nil {
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				finalStatus = "timeout"
				finalError = fmt.Sprintf("hermes timed out after %s", timeout)
			} else if errors.Is(runCtx.Err(), context.Canceled) {
				finalStatus = "cancelled"
				finalError = "execution cancelled"
			} else {
				finalStatus = "failed"
				finalError = fmt.Sprintf("hermes session/prompt failed: %v", err)
			}
		} else {
			select {
			case pr := <-promptDone:
				if pr.stopReason == "cancelled" {
					finalStatus = "cancelled"
					finalError = "hermes cancelled the prompt"
				}
				cl.usageMu.Lock()
				cl.usage.InputTokens += pr.usage.InputTokens
				cl.usage.OutputTokens += pr.usage.OutputTokens
				cl.usageMu.Unlock()
			default:
			}
		}

		duration := time.Since(startTime)
		b.logger.Info("hermes: finished",
			"status", finalStatus,
			"session_id", sessionID,
			"duration", duration.Round(time.Millisecond).String(),
		)

		_ = stdin.Close()
		cancel()
		<-readerDone
		<-stderrDone

		outputMu.Lock()
		finalOutput := output.String()
		outputMu.Unlock()

		finalStatus, finalError = promoteACPResultOnProviderError(finalStatus, finalError, finalOutput, providerErr)

		cl.usageMu.Lock()
		u := cl.usage
		cl.usageMu.Unlock()

		var usageMap map[string]TokenUsage
		if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 {
			usageMap = map[string]TokenUsage{opts.Model: u}
		}

		resCh <- &Result{
			Status:     finalStatus,
			Output:     finalOutput,
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
			Usage:      usageMap,
		}
	}()

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// ── Helpers ──

func buildHermesSessionParams(cwd, model string) map[string]any {
	params := map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	}
	if model != "" {
		params["model"] = model
	}
	return params
}

// ── Persistent Backend (v1.4) ──────────────────────────────────────────────────

// hermesPersistentState holds the runtime state of a long-running Hermes ACP
// subprocess across multiple turns.
type hermesPersistentState struct {
	runner    *persistentRunner
	client    *acpClient
	sessionID string
	turns     acpTurnController
}

// Compile-time check.
var _ SessionStater = (*hermesPersistentState)(nil)

func (s *hermesPersistentState) IsAlive() bool           { return s.runner.isAlive() }
func (s *hermesPersistentState) SessionID() string       { return s.sessionID }
func (s *hermesPersistentState) Done() <-chan struct{}   { return s.runner.done }
func (s *hermesPersistentState) Notify(msg string) error { return s.runner.write([]byte(msg)) }

// Start creates a persistent Hermes session via ACP. The handshake completes
// before Start returns; the initial prompt continues asynchronously.
func (b *HermesBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("hermes executable not found at %q: %w", execPath, err)
	}

	hermesArgs := append([]string{"acp"}, filterCustomArgs(opts.ExtraArgs, hermesBlockedArgs)...)
	hermesArgs = append(hermesArgs, filterCustomArgs(opts.CustomArgs, hermesBlockedArgs)...)

	extraEnv := make(map[string]string, len(opts.Env)+1)
	for k, v := range opts.Env {
		extraEnv[k] = v
	}
	extraEnv["HERMES_YOLO_MODE"] = "1"

	runner, err := startPersistent(ctx, execPath, hermesArgs, opts.WorkspaceDir, extraEnv, b.logger)
	if err != nil {
		return nil, err
	}

	prompt := buildPrompt(req, opts)
	state := &hermesPersistentState{runner: runner}
	turn, turnErr := state.turns.begin(opts.Model)
	if turnErr != nil {
		_ = runner.close()
		return nil, fmt.Errorf("hermes: begin initial turn: %w", turnErr)
	}

	cl := &acpClient{
		logger:  b.logger,
		stdin:   runner.stdin,
		pending: make(map[int]*pendingRPC),
	}
	cl.setCallbacks(state.turns.emit, state.turns.recordPromptDone)
	state.client = cl

	// Start reader goroutine for process lifetime.
	go func() {
		defer state.runner.finish()
		scanner := bufio.NewScanner(runner.stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			cl.handleLine(line)
		}
		cl.closeAllPending(fmt.Errorf("hermes process exited"))
		state.turns.failActive("hermes process exited unexpectedly")
	}()

	handleError := func(errMsg string) {
		state.turns.finish(turn, acpPromptErrorStatus(ctx), errMsg)
	}

	// 1. Initialize handshake.
	_, err = cl.request(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "solo-agent-sdk",
			"version": "1.0.0",
		},
		"clientCapabilities": map[string]any{},
	})
	if err != nil {
		handleError(fmt.Sprintf("hermes initialize failed: %v", err))
		_ = runner.close()
		return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
	}

	// 2. Create or resume session.
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
		result, err := cl.request(ctx, "session/new", buildHermesSessionParams(cwd, opts.Model))
		if err != nil {
			handleError(fmt.Sprintf("hermes session/new failed: %v", err))
			_ = runner.close()
			return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
		}
		sessionID = extractACPSessionID(result)
		if sessionID == "" {
			handleError("hermes session/new returned no session ID")
			_ = runner.close()
			return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
		}
	}
	cl.sessionID = sessionID
	state.sessionID = sessionID
	b.logger.Info("hermes: persistent session created", "session_id", sessionID)

	// 3. Set model if specified (session/new already passes model, but
	// explicit set_model provides a clearer error path).
	if opts.Model != "" {
		if _, err := cl.request(ctx, "session/set_model", map[string]any{
			"sessionId": sessionID,
			"modelId":   opts.Model,
		}); err != nil {
			b.logger.Warn("hermes: set_session_model failed", "error", err)
		}
	}

	// 4. Build prompt blocks with role-based format so the agent
	// treats system instructions as authoritative.
	promptBlocks := []map[string]any{
		{"type": "text", "text": prompt, "role": "user"},
	}
	if opts.SystemPrompt != "" {
		promptBlocks = append([]map[string]any{
			{"type": "text", "text": opts.SystemPrompt, "role": "system"},
		}, promptBlocks...)
	}

	state.turns.emit(OutputChunk{Type: string(MessageStatus), Content: "running", SessionID: sessionID})

	startACPInitialPromptTurn(acpInitialPromptTurn{
		ctx:          ctx,
		provider:     "hermes",
		sessionID:    sessionID,
		promptBlocks: promptBlocks,
		client:       cl,
		turns:        &state.turns,
		turn:         turn,
	})

	var stopOnce sync.Once
	stop := func() error { stopOnce.Do(func() { runner.cancel() }); return nil }
	return &PersistentSession{
		Messages:  turn.msgCh,
		Result:    turn.resCh,
		Stop:      stop,
		SessionID: sessionID,
		state:     state,
	}, nil
}

// Send delivers new messages to a running persistent Hermes session on the
// existing ACP session.
func (b *HermesBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*hermesPersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("hermes: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("hermes: session process has exited")
	}

	prompt := buildPromptFromMessages(messages)
	turn, err := state.turns.begin("")
	if err != nil {
		return nil, fmt.Errorf("hermes: %w", err)
	}

	_, err = state.client.request(ctx, "session/prompt", map[string]any{
		"sessionId": state.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		},
	})
	if err != nil {
		state.turns.finish(turn, acpPromptErrorStatus(ctx), err.Error())
		return nil, fmt.Errorf("hermes persistent session/prompt: %w", err)
	}

	state.turns.finish(turn, "completed", "")

	b.logger.Info("hermes: persistent turn completed via Send",
		"session_id", state.sessionID,
		"duration", time.Since(turn.startedAt).Round(time.Millisecond).String(),
	)

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

// Close terminates the persistent Hermes session.
func (b *HermesBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*hermesPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("hermes: invalid session state")
	}
	return state.runner.close()
}

// ForceClose immediately kills the Hermes subprocess without graceful exit.
func (b *HermesBackend) ForceClose(ps *PersistentSession) error {
	state, ok := ps.state.(*hermesPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("hermes: invalid session state")
	}
	return state.runner.forceClose()
}
