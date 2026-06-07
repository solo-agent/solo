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
	"sync/atomic"
	"time"
)

// kimiBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var kimiBlockedArgs = map[string]blockedArgMode{
	"acp": blockedStandalone,
}

// KimiBackend implements Backend by spawning `kimi acp` and communicating
// via the ACP (Agent Client Protocol) JSON-RPC 2.0 over stdin/stdout.
type KimiBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewKimiBackend creates a new KimiBackend.
// If executablePath is empty it defaults to "kimi".
func NewKimiBackend(executablePath string, logger *slog.Logger) *KimiBackend {
	if executablePath == "" {
		executablePath = "kimi"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &KimiBackend{executablePath: executablePath, logger: logger}
}

// Name returns "kimi".
func (b *KimiBackend) Name() string { return "kimi" }

// Execute launches the kimi CLI subprocess, sends the prompt via ACP,
// streams output events through Session.Messages, and delivers the final
// result on Session.Result.
func (b *KimiBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("kimi executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	kimiArgs := append([]string{"acp"}, filterCustomArgs(opts.CustomArgs, kimiBlockedArgs)...)

	cmd := exec.CommandContext(runCtx, execPath, kimiArgs...)
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("kimi: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("kimi: stdin pipe: %w", err)
	}

	providerErr := newACPProviderErrorSniffer("kimi")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("kimi: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start kimi: %w", err)
	}

	stderrSink := io.MultiWriter(newLogWriter(b.logger, "[kimi:stderr] "), providerErr)
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(stderrSink, stderr)
	}()

	b.logger.Info("kimi: started", "pid", cmd.Process.Pid, "cwd", opts.WorkspaceDir)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	promptDone := make(chan acpPromptResult, 1)

	c := &acpClient{
		logger:  b.logger,
		stdin:   stdin,
		pending: make(map[int]*pendingRPC),
		onChunk: func(chunk OutputChunk) {
			if chunk.Type == string(MessageToolUse) {
				chunk.Tool.Name = kimiToolNameFromTitle(chunk.Tool.Name)
			}
			if chunk.Type == string(MessageText) && chunk.Content != "" {
				outputMu.Lock()
				output.WriteString(chunk.Content)
				outputMu.Unlock()
			}
			trySend(msgCh, chunk)
		},
		onPromptDone: func(result acpPromptResult) {
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
			c.handleLine(line)
		}
		c.closeAllPending(fmt.Errorf("kimi process exited"))
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
		_, err := c.request(runCtx, "initialize", map[string]any{
			"protocolVersion": 1,
			"clientInfo": map[string]any{
				"name":    "solo-agent-sdk",
				"version": "1.0.0",
			},
			"clientCapabilities": map[string]any{},
		})
		if err != nil {
			finalStatus = "failed"
			finalError = fmt.Sprintf("kimi initialize failed: %v", err)
			resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
			return
		}

		// 2. Create or resume session.
		cwd := opts.WorkspaceDir
		if cwd == "" {
			cwd = "."
		}
		if opts.ResumeSessionID != "" {
			result, err := c.request(runCtx, "session/resume", map[string]any{
				"sessionId": opts.ResumeSessionID,
			})
			if err == nil {
				sessionID, _ = resolveResumedSessionID(opts.ResumeSessionID, result)
			}
		}
		if sessionID == "" {
			result, err := c.request(runCtx, "session/new", map[string]any{
				"cwd":        cwd,
				"mcpServers": []any{},
			})
			if err != nil {
				finalStatus = "failed"
				finalError = fmt.Sprintf("kimi session/new failed: %v", err)
				resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
				return
			}
			sessionID = extractACPSessionID(result)
			if sessionID == "" {
				finalStatus = "failed"
				finalError = "kimi session/new returned no session ID"
				resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
				return
			}
		}

		c.sessionID = sessionID
		b.logger.Info("kimi session created", "session_id", sessionID)

		// 3. Set model if specified.
		if opts.Model != "" {
			if _, err := c.request(runCtx, "session/set_model", map[string]any{
				"sessionId": sessionID,
				"modelId":   opts.Model,
			}); err != nil {
				b.logger.Warn("kimi set_session_model failed", "error", err, "requested_model", opts.Model)
				finalStatus = "failed"
				finalError = fmt.Sprintf("kimi could not switch to model %q: %v", opts.Model, err)
				resCh <- &Result{
					Status:     finalStatus,
					Error:      finalError,
					DurationMs: time.Since(startTime).Milliseconds(),
				}
				return
			}
			b.logger.Info("kimi session model set", "model", opts.Model)
		}

		// 4. Build the prompt content with system prompt prepended.
		userText := prompt
		if opts.SystemPrompt != "" {
			userText = opts.SystemPrompt + "\n\n---\n\n" + prompt
		}

		// 5. Send the prompt and wait for PromptResponse.
		_, err = c.request(runCtx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt": []map[string]any{
				{"type": "text", "text": userText},
			},
		})
		if err != nil {
			if runCtx.Err() == context.DeadlineExceeded {
				finalStatus = "timeout"
				finalError = fmt.Sprintf("kimi timed out after %s", timeout)
			} else if runCtx.Err() == context.Canceled {
				finalStatus = "cancelled"
				finalError = "execution cancelled"
			} else {
				finalStatus = "failed"
				finalError = fmt.Sprintf("kimi session/prompt failed: %v", err)
			}
		} else {
			select {
			case pr := <-promptDone:
				if pr.stopReason == "cancelled" {
					finalStatus = "cancelled"
					finalError = "kimi cancelled the prompt"
				}
				c.usageMu.Lock()
				c.usage.InputTokens += pr.usage.InputTokens
				c.usage.OutputTokens += pr.usage.OutputTokens
				c.usageMu.Unlock()
			default:
			}
		}

		duration := time.Since(startTime)
		b.logger.Info("kimi finished", "pid", cmd.Process.Pid, "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

		_ = stdin.Close()
		cancel()
		<-readerDone
		<-stderrDone

		outputMu.Lock()
		finalOutput := output.String()
		outputMu.Unlock()

		finalStatus, finalError = promoteACPResultOnProviderError(finalStatus, finalError, finalOutput, providerErr)

		c.usageMu.Lock()
		u := c.usage
		c.usageMu.Unlock()

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

// ── Persistent Backend (v1.5) ──────────────────────────────────────────────────

// kimiPersistentState holds the runtime state of a long-running Kimi ACP
// subprocess across multiple turns.
type kimiPersistentState struct {
	runner    *persistentRunner
	client    *acpClient
	sessionID string
	turnFin   atomic.Bool // guards duplicate onPromptDone calls per turn
}

// Compile-time check.
var _ SessionStater = (*kimiPersistentState)(nil)

func (s *kimiPersistentState) IsAlive() bool           { return s.runner.isAlive() }
func (s *kimiPersistentState) SessionID() string       { return s.sessionID }
func (s *kimiPersistentState) Done() <-chan struct{}   { return s.runner.done }
func (s *kimiPersistentState) Notify(msg string) error { return s.runner.write([]byte(msg)) }

// buildKimiSessionParams constructs the session/new parameters for Kimi.
func buildKimiSessionParams(cwd string) map[string]any {
	return map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	}
}

// Start creates a persistent Kimi session via ACP. The initial handshake
// and prompt are processed synchronously before Start returns.
func (b *KimiBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("kimi executable not found at %q: %w", execPath, err)
	}

	kimiArgs := append([]string{"acp"}, filterCustomArgs(opts.ExtraArgs, kimiBlockedArgs)...)
	kimiArgs = append(kimiArgs, filterCustomArgs(opts.CustomArgs, kimiBlockedArgs)...)

	extraEnv := make(map[string]string, len(opts.Env))
	for k, v := range opts.Env {
		extraEnv[k] = v
	}

	runner, err := startPersistent(ctx, execPath, kimiArgs, opts.WorkspaceDir, extraEnv, b.logger)
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
			if chunk.Type == string(MessageToolUse) {
				chunk.Tool.Name = kimiToolNameFromTitle(chunk.Tool.Name)
			}
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

	state := &kimiPersistentState{
		runner: runner,
		client: cl,
	}

	// Start reader goroutine for process lifetime.
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
		cl.closeAllPending(fmt.Errorf("kimi process exited"))

		// If a turn is still active, signal failure.
		if state.turnFin.CompareAndSwap(false, true) {
			resCh <- &Result{Status: "failed", Error: "kimi process exited unexpectedly"}
			close(msgCh)
			close(resCh)
		}
	}()

	// Drive the initial handshake and prompt synchronously.
	startTime := time.Now()
	handleError := func(errMsg string) {
		if state.turnFin.CompareAndSwap(false, true) {
			resCh <- &Result{Status: "failed", Error: errMsg, DurationMs: time.Since(startTime).Milliseconds()}
			close(msgCh)
			close(resCh)
		}
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
		runner.close()
		handleError(fmt.Sprintf("kimi initialize failed: %v", err))
		return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
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
		result, err := cl.request(ctx, "session/new", buildKimiSessionParams(cwd))
		if err != nil {
			runner.close()
			handleError(fmt.Sprintf("kimi session/new failed: %v", err))
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
		sessionID = extractACPSessionID(result)
		if sessionID == "" {
			runner.close()
			handleError("kimi session/new returned no session ID")
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
	}
	cl.sessionID = sessionID
	state.sessionID = sessionID
	b.logger.Info("kimi: persistent session created", "session_id", sessionID)

	// 3. Set model if specified.
	if opts.Model != "" {
		if _, err := cl.request(ctx, "session/set_model", map[string]any{
			"sessionId": sessionID,
			"modelId":   opts.Model,
		}); err != nil {
			b.logger.Warn("kimi: set_session_model failed", "error", err)
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

	// 5. Send the initial prompt.
	_, err = cl.request(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    promptBlocks,
	})
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			handleError(fmt.Sprintf("kimi timed out during initial prompt"))
		} else if errors.Is(ctx.Err(), context.Canceled) {
			handleError("execution cancelled")
		} else {
			handleError(fmt.Sprintf("kimi session/prompt failed: %v", err))
		}
		return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
	}

	// Collect prompt result (usage, stop reason).
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
		Usage:      buildACPUsageMap(usage, opts.Model),
	}
	close(msgCh)
	close(resCh)
	state.turnFin.Store(true)

	b.logger.Info("kimi: initial persistent turn completed",
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

// Send delivers new messages to a running persistent Kimi session on the
// existing ACP session.
func (b *KimiBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*kimiPersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("kimi: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("kimi: session process has exited")
	}

	prompt := buildPromptFromMessages(messages)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	turnDone := make(chan acpPromptResult, 1)

	// Redirect client callbacks to this turn's channels.
	state.client.onChunk = func(chunk OutputChunk) {
		if chunk.Type == string(MessageToolUse) {
			chunk.Tool.Name = kimiToolNameFromTitle(chunk.Tool.Name)
		}
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

	_, err := state.client.request(ctx, "session/prompt", map[string]any{
		"sessionId": state.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		},
	})
	if err != nil {
		state.turnFin.Store(true)
		close(msgCh)
		close(resCh)
		return nil, fmt.Errorf("kimi persistent session/prompt: %w", err)
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
		Usage:      buildACPUsageMap(usage, ""),
	}
	close(msgCh)
	close(resCh)
	state.turnFin.Store(true)

	b.logger.Info("kimi: persistent turn completed via Send",
		"session_id", state.sessionID,
		"duration", duration.Round(time.Millisecond).String(),
	)

	var stopOnce sync.Once
	stop := func() error { stopOnce.Do(func() {}); return nil }

	return &PersistentSession{
		Messages:  msgCh,
		Result:    resCh,
		Stop:      stop,
		SessionID: state.sessionID,
		state:     state,
	}, nil
}

// Close terminates the persistent Kimi session.
func (b *KimiBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*kimiPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("kimi: invalid session state")
	}
	return state.runner.close()
}

// kimiTitleExtras maps Kimi's ACP-server tool labels to the canonical
// snake_case identifiers the UI expects. Entries are matched case-
// insensitively against the (trimmed, colon-stripped) title by
// acpToolNameFromTitle.
var kimiTitleExtras = map[string]string{
	"read":              "read_file",
	"read file":         "read_file",
	"write":             "write_file",
	"write file":        "write_file",
	"edit":              "edit_file",
	"patch":             "edit_file",
	"shell":             "terminal",
	"bash":              "terminal",
	"terminal":          "terminal",
	"run command":       "terminal",
	"run shell command": "terminal",
	"search":            "search_files",
	"grep":              "search_files",
	"find":              "search_files",
	"glob":              "glob",
	"web search":        "web_search",
	"fetch":             "web_fetch",
	"web fetch":         "web_fetch",
	"todo":              "todo_write",
	"todo write":        "todo_write",
}

// kimiToolNameFromTitle normalises tool names from Kimi's ACP server
// into the snake_case identifiers that the UI expects. It is a thin
// adapter around the shared acpToolNameFromTitle that supplies Kimi's
// backend-specific title→name mapping.
func kimiToolNameFromTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	if idx := strings.Index(t, ":"); idx > 0 {
		t = strings.TrimSpace(t[:idx])
	}
	return acpToolNameFromTitle(t, "", kimiTitleExtras)
}
