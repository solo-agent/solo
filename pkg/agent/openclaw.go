package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// openclawBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs. The "acp" subcommand is the
// entrypoint we use to drive OpenClaw; the rest of the CLI flags
// (--session, --token, --url, etc.) are openclaw-specific connection
// settings that users may still want to override via CustomArgs.
var openclawBlockedArgs = map[string]blockedArgMode{
	"acp": blockedStandalone,
}

// OpenClawBackend implements Backend by spawning `openclaw acp` and
// communicating via the ACP (Agent Communication Protocol) JSON-RPC 2.0
// over stdin/stdout. This is the same pattern as Hermes / Kimi / Kiro.
//
// OpenClaw's `acp` subcommand is a Gateway-backed ACP bridge: it accepts
// JSON-RPC 2.0 frames on stdin and routes prompts to a Gateway session
// (default: isolated `acp:<uuid>`). It supports the core ACP flow
// (initialize / newSession / prompt / cancel) plus partial session
// resume and tool streaming. See https://docs.openclaw.ai/zh-CN/cli/acp
// for the full compatibility matrix.
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

// Execute launches the openclaw CLI subprocess, sends the prompt via ACP,
// streams output events through Session.Messages, and delivers the final
// result on Session.Result.
//
// Execute is implemented in terms of Start + drain. The single-shot
// behavior (subprocess exits after the first turn completes) is preserved
// by closing the runner once the first turn's Result has been delivered.
func (b *OpenClawBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	ps, err := b.Start(ctx, req, opts)
	if err != nil {
		return nil, err
	}

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() { _ = b.Close(ps) })
		return nil
	}

	go func() {
		defer close(msgCh)
		defer close(resCh)
		defer b.Close(ps)

		for chunk := range ps.Messages {
			trySend(msgCh, chunk)
		}
		res, ok := <-ps.Result
		if !ok {
			return
		}
		resCh <- res
	}()

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// ── Persistent Backend ────────────────────────────────────────────────────────

// openclawPersistentState holds the runtime state of a long-running OpenClaw
// ACP subprocess across multiple turns.
type openclawPersistentState struct {
	runner    *persistentRunner
	client    *acpClient
	sessionID string
	turnFin   atomic.Bool // guards duplicate onPromptDone calls per turn
}

// Compile-time check.
var _ SessionStater = (*openclawPersistentState)(nil)

func (s *openclawPersistentState) IsAlive() bool            { return s.runner.isAlive() }
func (s *openclawPersistentState) SessionID() string        { return s.sessionID }
func (s *openclawPersistentState) Done() <-chan struct{}    { return s.runner.done }
func (s *openclawPersistentState) Notify(msg string) error  { return s.runner.write([]byte(msg)) }

// Start creates a persistent OpenClaw session via ACP. The initial
// handshake and prompt are processed synchronously before Start returns.
func (b *OpenClawBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("openclaw executable not found at %q: %w", execPath, err)
	}

	if err := checkOpenclawAcpSupport(ctx, execPath); err != nil {
		return nil, err
	}

	openclawArgs := append([]string{"acp"}, filterCustomArgs(opts.ExtraArgs, openclawBlockedArgs)...)
	openclawArgs = append(openclawArgs, filterCustomArgs(opts.CustomArgs, openclawBlockedArgs)...)

	runner, err := startPersistent(ctx, execPath, openclawArgs, opts.WorkspaceDir, opts.Env, b.logger)
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

	state := &openclawPersistentState{
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
		cl.closeAllPending(fmt.Errorf("openclaw process exited"))

		// If a turn is still active, signal failure.
		if state.turnFin.CompareAndSwap(false, true) {
			resCh <- &Result{Status: "failed", Error: "openclaw process exited unexpectedly"}
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
	if _, err := cl.request(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "solo-agent-sdk",
			"version": "1.0.0",
		},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		runner.close()
		handleError(fmt.Sprintf("openclaw initialize failed: %v", err))
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
		result, err := cl.request(ctx, "session/new", buildOpenclawSessionParams(cwd, opts.Model))
		if err != nil {
			runner.close()
			handleError(fmt.Sprintf("openclaw session/new failed: %v", err))
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
		sessionID = extractACPSessionID(result)
		if sessionID == "" {
			runner.close()
			handleError("openclaw session/new returned no session ID")
			return &PersistentSession{Messages: msgCh, Result: resCh, state: state}, nil
		}
	}
	cl.sessionID = sessionID
	state.sessionID = sessionID
	b.logger.Info("openclaw: persistent session created", "session_id", sessionID)

	// 3. Model selection: OpenClaw's bridge does not currently expose
	// modelId as an ACP session config option (only a focused subset of
	// Gateway knobs — think-level, tool verbosity, reasoning, usage
	// detail, escalation — are exposed). The model passed to
	// session/new is best-effort. We skip a separate session/set_model
	// call to avoid a guaranteed failure.

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
	if _, err := cl.request(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    promptBlocks,
	}); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			handleError("openclaw timed out during initial prompt")
		} else if errors.Is(ctx.Err(), context.Canceled) {
			handleError("execution cancelled")
		} else {
			handleError(fmt.Sprintf("openclaw session/prompt failed: %v", err))
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

	if state.turnFin.CompareAndSwap(false, true) {
		resCh <- &Result{
			Status:     "completed",
			Output:     finalOutput,
			DurationMs: duration.Milliseconds(),
			Usage:      buildACPUsageMap(usage, opts.Model),
		}
		close(msgCh)
		close(resCh)
	}

	b.logger.Info("openclaw: initial persistent turn completed",
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

// Send delivers new messages to a running persistent OpenClaw session on
// the existing ACP session.
func (b *OpenClawBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*openclawPersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("openclaw: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("openclaw: session process has exited")
	}

	prompt := buildPromptFromMessages(messages)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)

	var outputMu sync.Mutex
	var output strings.Builder
	turnDone := make(chan acpPromptResult, 1)

	// Redirect client callbacks to this turn's channels.
	state.client.setCallbacks(
		func(chunk OutputChunk) {
		if chunk.Type == string(MessageText) && chunk.Content != "" {
			outputMu.Lock()
			output.WriteString(chunk.Content)
			outputMu.Unlock()
		}
		trySend(msgCh, chunk)
	},
		func(pr acpPromptResult) {
		select {
		case turnDone <- pr:
		default:
		}
	},
	)

	startTime := time.Now()
	state.turnFin.Store(false)

	if _, err := state.client.request(ctx, "session/prompt", map[string]any{
		"sessionId": state.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		},
	}); err != nil {
		if state.turnFin.CompareAndSwap(false, true) {
			close(msgCh)
			close(resCh)
		}
		return nil, fmt.Errorf("openclaw persistent session/prompt: %w", err)
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

	if state.turnFin.CompareAndSwap(false, true) {
		resCh <- &Result{
			Status:     "completed",
			Output:     finalOutput,
			DurationMs: duration.Milliseconds(),
			Usage:      buildACPUsageMap(usage, ""),
		}
		close(msgCh)
		close(resCh)
	}

	b.logger.Info("openclaw: persistent turn completed via Send",
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

// Close terminates the persistent OpenClaw session.
func (b *OpenClawBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*openclawPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("openclaw: invalid session state")
	}
	return state.runner.close()
}

// ForceClose immediately kills the OpenClaw subprocess without graceful exit.
func (b *OpenClawBackend) ForceClose(ps *PersistentSession) error {
	state, ok := ps.state.(*openclawPersistentState)
	if !ok || state == nil {
		return fmt.Errorf("openclaw: invalid session state")
	}
	return state.runner.forceClose()
}

// ── Helpers ──

// minOpenclawAcpVersion is the minimum openclaw version known to support
// the `acp` subcommand. Older versions only support `agent --json` and
// will fail the ACP handshake with a cryptic error.
const minOpenclawAcpVersion = "2026.5.5"

var openclawVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func checkOpenclawAcpSupport(ctx context.Context, execPath string) error {
	cmd := exec.CommandContext(ctx, execPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openclaw --version failed: %w", err)
	}
	detected, ok := parseOpenclawVersion(string(out))
	if !ok {
		return fmt.Errorf("could not parse openclaw version from output: %q", strings.TrimSpace(string(out)))
	}
	if compareOpenclawVersion(detected, minOpenclawAcpVersion) < 0 {
		return fmt.Errorf("openclaw %s is below the minimum supported version %s for ACP support — run `openclaw update` to upgrade", detected, minOpenclawAcpVersion)
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

// buildOpenclawSessionParams constructs the params for ACP session/new.
//
// The OpenClaw bridge requires mcpServers to be present (even as an empty
// array). Per-session MCP server config is not supported in bridge mode,
// so we always send an empty list.
// Model is best-effort — see the comment in Start for why session/set_model
// is not invoked.
func buildOpenclawSessionParams(cwd, model string) map[string]any {
	params := map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	}
	if model != "" {
		params["model"] = model
	}
	return params
}

// Compile-time interface assertions.
var (
	_ Backend           = (*OpenClawBackend)(nil)
	_ PersistentBackend = (*OpenClawBackend)(nil)
)
