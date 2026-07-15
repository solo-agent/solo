package agent

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// OpenCodeBackend implements Backend by spawning `opencode acp --pure`
// and communicating through ACP over stdin/stdout.
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
		Messages:  msgCh,
		Result:    resCh,
		Stop:      func() error { stopOnce.Do(func() { b.Close(ps) }); return nil },
		SessionID: ps.SessionID,
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
	turns     acpTurnController
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
	state := &opencodePersistentState{runner: runner}
	turn, err := state.turns.begin(opts.Model)
	if err != nil {
		_ = runner.close()
		return nil, fmt.Errorf("opencode: begin initial turn: %w", err)
	}

	cl := &acpClient{
		logger:  b.logger,
		stdin:   runner.stdin,
		pending: make(map[int]*pendingRPC),
	}
	cl.setCallbacks(state.turns.emit, state.turns.recordPromptDone)
	state.client = cl

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
		cl.closeAllPending(fmt.Errorf("opencode process exited"))
		state.turns.failActive("opencode process exited unexpectedly")
	}()

	handleError := func(errMsg string) {
		state.turns.finish(turn, acpPromptErrorStatus(ctx), errMsg)
	}

	if _, err := cl.request(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "solo-agent-sdk", "version": "1.0.0"},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		handleError(fmt.Sprintf("opencode initialize failed: %v", err))
		_ = runner.close()
		return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
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
			handleError(fmt.Sprintf("opencode session/new failed: %v", err))
			_ = runner.close()
			return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
		}
		sessionID = extractACPSessionID(result)
		if sessionID == "" {
			handleError("opencode session/new returned no session ID")
			_ = runner.close()
			return &PersistentSession{Messages: turn.msgCh, Result: turn.resCh, state: state}, nil
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

	state.turns.emit(OutputChunk{Type: string(MessageStatus), Content: "running", SessionID: sessionID})

	startACPInitialPromptTurn(acpInitialPromptTurn{
		ctx:          ctx,
		provider:     "opencode",
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

func (b *OpenCodeBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("opencode: invalid session state")
	}
	if !state.runner.isAlive() {
		return nil, fmt.Errorf("opencode: session process has exited")
	}

	prompt := buildPromptFromMessages(messages)
	turn, err := state.turns.begin("")
	if err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}

	if _, err = state.client.request(ctx, "session/prompt", map[string]any{
		"sessionId": state.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt, "role": "user"},
		},
	}); err != nil {
		state.turns.finish(turn, acpPromptErrorStatus(ctx), err.Error())
		return nil, fmt.Errorf("opencode persistent session/prompt: %w", err)
	}

	state.turns.finish(turn, "completed", "")

	b.logger.Info("opencode: persistent turn completed via Send",
		"session_id", state.sessionID,
		"duration", time.Since(turn.startedAt).Round(time.Millisecond).String(),
	)

	var stopOnce2 sync.Once
	stop := func() error { stopOnce2.Do(func() {}); return nil }

	return &PersistentSession{
		Messages:  turn.msgCh,
		Result:    turn.resCh,
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

// ForceClose immediately kills the OpenCode subprocess without graceful exit.
func (b *OpenCodeBackend) ForceClose(ps *PersistentSession) error {
	state, ok := ps.state.(*opencodePersistentState)
	if !ok || state == nil {
		return fmt.Errorf("opencode: invalid session state")
	}
	return state.runner.forceClose()
}
