package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ── External interface ──

// ClaudeBackend implements Backend by spawning the Claude Code CLI
// with --output-format stream-json and communicating via stdin/stdout
// pipes using the stream-JSON protocol.
type ClaudeBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewClaudeBackend creates a new ClaudeBackend.
// If executablePath is empty it defaults to "claude".
// If logger is nil, slog.Default() is used.
func NewClaudeBackend(executablePath string, logger *slog.Logger) *ClaudeBackend {
	if executablePath == "" {
		executablePath = "claude"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ClaudeBackend{executablePath: executablePath, logger: logger}
}

// Name returns "claude".
func (b *ClaudeBackend) Name() string { return "claude" }

// Execute launches the claude CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *ClaudeBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("claude executable not found at %q: %w", execPath, err)
	}

	// Derive deadline from context or default to 20 min.
	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	args := buildClaudeArgs(req, opts)
	b.logger.Info("claude: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnvAt(opts.WorkspaceDir, opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude: stdin pipe: %w", err)
	}
	// Capture the last 64 KB of stderr so we can surface crash details.
	stderrLog := newLogWriter(b.logger, fmt.Sprintf("[Agent %s] ", req.AgentID))
	stderrTail := newStderrTail(stderrLog, 64*1024)
	cmd.Stderr = stderrTail

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		cancel()
		return nil, fmt.Errorf("claude: start: %w", err)
	}
	b.logger.Info("claude: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

	// Send the prompt via stdin.
	prompt := buildPrompt(req, opts)
	if err := writeClaudeInput(stdin, prompt); err != nil {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait()
		return nil, fmt.Errorf("claude: write input: %w", err)
	}
	_ = stdin.Close()

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

	go b.streamLoop(runCtx, cancel, cmd, stdout, stderrTail, msgCh, resCh, timeout)

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// ── Persistent session types ──────────────────────────────────────────────────

// Compile-time check: claudePersistentState implements SessionStater.
var _ SessionStater = (*claudePersistentState)(nil)

// claudePersistentState holds the runtime state of a long-running Claude Code
// subprocess. The subprocess stays alive across multiple turns (Start → Send →
// Send → ... → Close), maintaining full conversation context.
type claudePersistentState struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderrTail *stderrTail
	cancel     context.CancelFunc
	logger     *slog.Logger

	// turn points to the currently active turn's output channels.
	// Atomically swapped: set to a new turnState on each Send(),
	// set to nil when a turn completes (result received).
	turn atomic.Pointer[turnState]

	// done is closed when the session is terminated (Close called or
	// subprocess exited unexpectedly).
	done chan struct{}

	// sessionID from Claude Code's "system" init message.
	sessionID string

	totalUsage map[string]TokenUsage
	usageMu    sync.Mutex

	// exitErr captures the error from cmd.Wait() after the process exits.
	exitErr error
}

// turnState captures the per-turn streaming channels and accumulated output.
// Created fresh for each Start/Send call and stored in claudePersistentState.turn.
type turnState struct {
	id     string
	msgCh  chan OutputChunk
	resCh  chan *Result
	output strings.Builder
}

// ── SessionStater implementation ──────────────────────────────────────────────

func (s *claudePersistentState) IsAlive() bool {
	return s.cmd.ProcessState == nil || !s.cmd.ProcessState.Exited()
}

func (s *claudePersistentState) SessionID() string { return s.sessionID }

func (s *claudePersistentState) Done() <-chan struct{} { return s.done }

func (s *claudePersistentState) Notify(msg string) error {
	_, err := s.stdin.Write([]byte(msg))
	return err
}

// ── Persistent Backend: Start ────────────────────────────────────────────────

// Start creates a persistent Claude Code session. The subprocess stays alive
// after the initial prompt, waiting for additional input on stdin. Callers
// consume Messages for streaming events and wait on Result for the turn outcome.
func (b *ClaudeBackend) Start(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("claude executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithCancel(ctx)

	args := buildClaudeArgs(req, opts)
	b.logger.Info("claude: starting persistent session", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnvAt(opts.WorkspaceDir, opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude: stdin pipe: %w", err)
	}
	stderrLog := newLogWriter(b.logger, fmt.Sprintf("[Agent %s] ", req.AgentID))
	stderrTail := newStderrTail(stderrLog, 64*1024)
	cmd.Stderr = stderrTail

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		cancel()
		return nil, fmt.Errorf("claude: start: %w", err)
	}
	b.logger.Info("claude: persistent session started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

	state := &claudePersistentState{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderrTail: stderrTail,
		cancel:     cancel,
		logger:     b.logger,
		done:       make(chan struct{}),
		totalUsage: make(map[string]TokenUsage),
	}

	// Write the initial prompt. Stdin stays OPEN for subsequent Send() calls.
	prompt := buildPrompt(req, opts)
	if err := writeClaudeInput(stdin, prompt); err != nil {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait()
		return nil, fmt.Errorf("claude: write input: %w", err)
	}

	// Create the initial turn.
	turn := &turnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	go b.persistentStreamLoop(runCtx, state, timeout)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			cancel()
		})
		return nil
	}

	return &PersistentSession{
		Messages: turn.msgCh,
		Result:   turn.resCh,
		Stop:     stop,
		state:    state,
	}, nil
}

// ── Persistent Stream Loop ───────────────────────────────────────────────────

// persistentStreamLoop reads Claude Code's stdout across multiple turns.
// Each "result" message closes the current turn's channels and sets turn=nil.
// The loop continues until stdin is closed (EOF on stdout) or the context
// is cancelled.
func (b *ClaudeBackend) persistentStreamLoop(
	runCtx context.Context,
	state *claudePersistentState,
	timeout time.Duration,
) {
	defer state.cancel()
	defer close(state.done)

	startTime := time.Now()

	go func() {
		<-runCtx.Done()
		_ = state.stdout.Close()
	}()

	scanner := bufio.NewScanner(state.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg claudeSDKMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		turn := state.turn.Load()

		switch msg.Type {
		case "assistant":
			if turn != nil {
				b.handleAssistantPersistent(msg, turn, state)
			}

		case "user":
			if turn != nil {
				b.handleUser(msg, turn.msgCh)
			}

		case "system":
			if msg.SessionID != "" && state.sessionID == "" {
				state.sessionID = msg.SessionID
			}
			if turn != nil {
				trySend(turn.msgCh, OutputChunk{
					Type:    string(MessageStatus),
					Content: "running",
				})
			}

		case "result":
			if turn != nil {
				res := &Result{
					Status: "completed",
					Output: turn.output.String(),
					Usage:  b.snapshotUsage(state),
				}
				if msg.ResultText != "" && turn.output.Len() == 0 {
					res.Output = msg.ResultText
				}
				if msg.IsError {
					res.Status = "failed"
					res.Error = msg.ResultText
				}
				res.DurationMs = time.Since(startTime).Milliseconds()
				close(turn.msgCh)
				turn.resCh <- res
				close(turn.resCh)
				state.turn.Store(nil)
				b.logger.Info("claude: turn completed", "turn_id", turn.id, "session_id", state.sessionID)
			}

		case "error":
			if turn != nil {
				close(turn.msgCh)
				turn.resCh <- &Result{
					Status: "failed",
					Error:  msg.ErrorText,
					Usage:  b.snapshotUsage(state),
				}
				close(turn.resCh)
				state.turn.Store(nil)
			}
		}
	}

	// Process exited. Drain any remaining turn.
	if turn := state.turn.Load(); turn != nil {
		errMsg := "subprocess exited unexpectedly"
		if state.exitErr != nil {
			errMsg = state.exitErr.Error()
		}
		close(turn.msgCh)
		turn.resCh <- &Result{Status: "failed", Error: errMsg}
		close(turn.resCh)
	}

	state.exitErr = state.cmd.Wait()
	duration := time.Since(startTime)

	b.logger.Info("claude: persistent session ended",
		"session_id", state.sessionID,
		"duration", duration.Round(time.Millisecond).String(),
	)
}

func (b *ClaudeBackend) handleAssistantPersistent(msg claudeSDKMessage, turn *turnState, state *claudePersistentState) {
	var content claudeMessageContent
	if err := json.Unmarshal(msg.Message, &content); err != nil {
		return
	}

	state.usageMu.Lock()
	if content.Usage != nil && content.Model != "" {
		u := state.totalUsage[content.Model]
		u.InputTokens += content.Usage.InputTokens
		u.OutputTokens += content.Usage.OutputTokens
		u.CacheReadTokens += content.Usage.CacheReadInputTokens
		u.CacheWriteTokens += content.Usage.CacheCreationInputTokens
		state.totalUsage[content.Model] = u
	}
	state.usageMu.Unlock()

	for _, block := range content.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				turn.output.WriteString(block.Text)
				trySend(turn.msgCh, OutputChunk{Type: string(MessageText), Content: block.Text})
			}
		case "thinking":
			if block.Text != "" {
				trySend(turn.msgCh, OutputChunk{Type: string(MessageThinking), Content: block.Text})
			}
		case "tool_use":
			var input map[string]any
			if block.Input != nil {
				_ = json.Unmarshal(block.Input, &input)
			}
			trySend(turn.msgCh, OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{
					Name:   block.Name,
					CallID: block.ID,
					Input:  input,
				},
			})
		}
	}
}

func (b *ClaudeBackend) snapshotUsage(state *claudePersistentState) map[string]TokenUsage {
	state.usageMu.Lock()
	defer state.usageMu.Unlock()
	out := make(map[string]TokenUsage, len(state.totalUsage))
	for k, v := range state.totalUsage {
		out[k] = v
	}
	return out
}

// ── Persistent Backend: Send ─────────────────────────────────────────────────

// Send delivers new messages to a running persistent session. It creates a new
// turnState, writes the prompt to stdin, and returns a PersistentSession for
// consuming this turn's output. Callers must finish consuming the previous
// turn before calling Send.
func (b *ClaudeBackend) Send(ctx context.Context, ps *PersistentSession, messages []Message) (*PersistentSession, error) {
	state, ok := ps.state.(*claudePersistentState)
	if !ok || state == nil {
		return nil, fmt.Errorf("claude: invalid session state")
	}

	// Check if the process is still alive.
	if state.cmd.ProcessState != nil && state.cmd.ProcessState.Exited() {
		return nil, fmt.Errorf("claude: session process has exited")
	}

	// Build the prompt from messages.
	var promptBuilder strings.Builder
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			continue
		}
		switch msg.Role {
		case RoleUser:
			promptBuilder.WriteString("User: ")
		case RoleAssistant:
			promptBuilder.WriteString("Assistant: ")
		default:
			promptBuilder.WriteString(fmt.Sprintf("[%s]: ", msg.Role))
		}
		promptBuilder.WriteString(msg.Content)
		promptBuilder.WriteString("\n\n")
	}
	promptBuilder.WriteString("Assistant:")

	// Create the new turn BEFORE writing to stdin so the stream loop
	// can route output immediately.
	turn := &turnState{
		id:    uuid.New().String(),
		msgCh: make(chan OutputChunk, 256),
		resCh: make(chan *Result, 1),
	}
	state.turn.Store(turn)

	payload, err := buildClaudeInput(promptBuilder.String())
	if err != nil {
		state.turn.Store(nil)
		return nil, fmt.Errorf("claude: build send input: %w", err)
	}

	if _, err := state.stdin.Write(payload); err != nil {
		state.turn.Store(nil)
		return nil, fmt.Errorf("claude: write to session stdin: %w", err)
	}

	b.logger.Info("claude: turn started via Send", "turn_id", turn.id, "session_id", state.sessionID)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			// Per-turn stop: only affects this turn, not the full session.
		})
		return nil
	}

	return &PersistentSession{
		Messages:  turn.msgCh,
		Result:    turn.resCh,
		Stop:      stop,
		SessionID: state.sessionID,
		state:     state,
	}, nil
}

// ── Persistent Backend: Close ────────────────────────────────────────────────

// Close terminates the persistent session. It closes stdin, waits for the
// subprocess to exit, and cleans up resources.
func (b *ClaudeBackend) Close(ps *PersistentSession) error {
	state, ok := ps.state.(*claudePersistentState)
	if !ok || state == nil {
		return fmt.Errorf("claude: invalid session state")
	}

	b.logger.Info("claude: closing persistent session", "session_id", state.sessionID)

	// Close stdin to signal Claude Code to exit.
	_ = state.stdin.Close()

	// Wait for the process to exit, with a timeout.
	select {
	case <-state.done:
		// Process exited cleanly.
	case <-time.After(10 * time.Second):
		b.logger.Warn("claude: session did not exit within timeout, killing")
		state.cancel()
		_ = state.cmd.Process.Kill()
		<-state.done
	}

	b.logger.Info("claude: persistent session closed",
		"session_id", state.sessionID,
		"pid", state.cmd.Process.Pid,
	)

	return nil
}

// ── Stream loop ──

func (b *ClaudeBackend) streamLoop(
	runCtx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	stderrTail *stderrTail,
	msgCh chan<- OutputChunk,
	resCh chan<- *Result,
	timeout time.Duration,
) {
	defer cancel()
	defer close(msgCh)
	defer close(resCh)

	startTime := time.Now()
	var output strings.Builder
	var sessionID string
	finalStatus := "completed"
	var finalError string
	usage := make(map[string]TokenUsage)

	// Close stdout when the context is cancelled so scanner.Scan() unblocks.
	go func() {
		<-runCtx.Done()
		_ = stdout.Close()
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg claudeSDKMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "assistant":
			b.handleAssistant(msg, msgCh, &output, usage)
		case "user":
			b.handleUser(msg, msgCh)
		case "system":
			if msg.SessionID != "" {
				sessionID = msg.SessionID
			}
			trySend(msgCh, OutputChunk{
				Type:    string(MessageStatus),
				Content: "running",
			})
		case "result":
			if msg.ResultText != "" {
				output.Reset()
				output.WriteString(msg.ResultText)
			}
			if msg.IsError {
				finalStatus = "failed"
				finalError = msg.ResultText
			}
		case "error":
			finalStatus = "failed"
			finalError = msg.ErrorText
		}
	}

	exitErr := cmd.Wait()
	duration := time.Since(startTime)

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		finalStatus = "timeout"
		finalError = fmt.Sprintf("claude timed out after %s", timeout)
	} else if errors.Is(runCtx.Err(), context.Canceled) {
		finalStatus = "cancelled"
		finalError = "execution cancelled"
	} else if exitErr != nil && finalStatus == "completed" {
		finalStatus = "failed"
		finalError = fmt.Sprintf("claude exited with error: %v", exitErr)
	}

	// Attach stderr tail to any failure message.
	if finalError != "" {
		if tail := stderrTail.Tail(); tail != "" {
			finalError = fmt.Sprintf("%s (stderr: %s)", finalError, tail)
		}
	}

	b.logger.Info("claude: finished",
		"status", finalStatus,
		"duration", duration.Round(time.Millisecond).String(),
		"session_id", sessionID,
	)

	resCh <- &Result{
		Status:     finalStatus,
		Output:     output.String(),
		Error:      finalError,
		DurationMs: duration.Milliseconds(),
		Usage:      usage,
	}
}

// ── Event handlers ──

func (b *ClaudeBackend) handleAssistant(msg claudeSDKMessage, ch chan<- OutputChunk, output *strings.Builder, usage map[string]TokenUsage) {
	var content claudeMessageContent
	if err := json.Unmarshal(msg.Message, &content); err != nil {
		return
	}

	// Accumulate token usage per model.
	if content.Usage != nil && content.Model != "" {
		u := usage[content.Model]
		u.InputTokens += content.Usage.InputTokens
		u.OutputTokens += content.Usage.OutputTokens
		u.CacheReadTokens += content.Usage.CacheReadInputTokens
		u.CacheWriteTokens += content.Usage.CacheCreationInputTokens
		usage[content.Model] = u
	}

	for _, block := range content.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				output.WriteString(block.Text)
				trySend(ch, OutputChunk{Type: string(MessageText), Content: block.Text})
			}
		case "thinking":
			if block.Text != "" {
				trySend(ch, OutputChunk{Type: string(MessageThinking), Content: block.Text})
			}
		case "tool_use":
			var input map[string]any
			if block.Input != nil {
				_ = json.Unmarshal(block.Input, &input)
			}
			trySend(ch, OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{
					Name:   block.Name,
					CallID: block.ID,
					Input:  input,
				},
			})
		}
	}
}

func (b *ClaudeBackend) handleUser(msg claudeSDKMessage, ch chan<- OutputChunk) {
	var content claudeMessageContent
	if err := json.Unmarshal(msg.Message, &content); err != nil {
		return
	}

	for _, block := range content.Content {
		if block.Type == "tool_result" {
			resultStr := ""
			if block.Content != nil {
				resultStr = string(block.Content)
			}
			trySend(ch, OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{
					CallID: block.ToolUseID,
					Output: resultStr,
				},
			})
		}
	}
}

// ── CLI argument construction ──

// claudeBlockedArgs are flags hardcoded by the Backend that must not be
// overridden by user-configured CustomArgs. Overriding these would break
// the daemon-to-claude communication protocol.
//
// Note: "-p" is intentionally NOT blocked. We run Claude Code in interactive
// mode (without -p) so agents can autonomously execute shell commands
// (e.g. solo task claim, curl) as instructed by CLAUDE.md. Blocking only
// the flags that would break the stdin/stdout stream-json protocol.
var claudeBlockedArgs = map[string]blockedArgMode{
	"--output-format":   blockedWithValue,
	"--input-format":    blockedWithValue,
	"--permission-mode": blockedWithValue,
}

// buildClaudeArgs constructs the CLI arguments for spawning Claude Code.
// We run in interactive mode (without -p) so that Claude Code reads its
// CLAUDE.md, discovers the Task Protocol, and autonomously executes shell
// commands (e.g. solo task claim, curl API calls). The prompt is sent via
// stdin in stream-json format.
func buildClaudeArgs(req *ExecuteRequest, opts *ExecuteOptions) []string {
	args := []string{
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		// Write system prompt to .solo/system-prompt.md (Slock-aligned).
		// The file IS the system prompt — single source of truth.
		soloDir := filepath.Join(opts.WorkspaceDir, ".solo")
		os.MkdirAll(soloDir, 0755)
		promptPath := filepath.Join(soloDir, "system-prompt.md")
		os.WriteFile(promptPath, []byte(opts.SystemPrompt), 0644)
		args = append(args, "--system-prompt-file", promptPath)
	}

	args = append(args, filterCustomArgs(opts.CustomArgs, claudeBlockedArgs)...)
	return args
}

// ── Prompt construction ──

func buildPrompt(req *ExecuteRequest, opts *ExecuteOptions) string {
	var b strings.Builder

	for _, msg := range req.Messages {
		// System-prompt-level messages are handled via --append-system-prompt
		// to avoid duplication in the text prompt.
		if msg.Role == RoleSystem {
			continue
		}
		switch msg.Role {
		case RoleUser:
			b.WriteString("User: ")
		case RoleAssistant:
			b.WriteString("Assistant: ")
		default:
			b.WriteString(fmt.Sprintf("[%s]: ", msg.Role))
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}

	// Signal that it is the agent's turn to respond.
	b.WriteString("Assistant:")

	return b.String()
}

// ── Stdin input ──

func writeClaudeInput(w io.Writer, prompt string) error {
	data, err := buildClaudeInput(prompt)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func buildClaudeInput(prompt string) ([]byte, error) {
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]string{
				{
					"type": "text",
					"text": prompt,
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal claude input: %w", err)
	}
	return append(data, '\n'), nil
}

// ── Environment ──

func buildEnv(extra map[string]string) []string {
	env := mergeEnv(os.Environ(), extra)
	// Prepend CWD to PATH so the solo binary (copied to workspace root) is
	// directly accessible as "solo", matching Slock's slock wrapper pattern.
	// Claude Code sets CWD to workspace root via cmd.Dir.
	return env
}

// buildEnvAt adds the workspace directory to PATH before merging extra vars.
// The workspace dir contains the solo binary, so agents can run "solo" directly.
func buildEnvAt(workspaceDir string, extra map[string]string) []string {
	env := mergeEnv(os.Environ(), extra)
	pathEntry := "./"
	if workspaceDir != "" {
		pathEntry = workspaceDir
	}
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + pathEntry + ":" + e[5:]
			return env
		}
	}
	env = append(env, "PATH="+pathEntry)
	return env
}

func mergeEnv(base []string, extra map[string]string) []string {
	env := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if isFilteredChildEnvKey(key) {
			continue
		}
		env = append(env, entry)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func isFilteredChildEnvKey(key string) bool {
	return key == "CLAUDECODE" ||
		strings.HasPrefix(key, "CLAUDECODE_") ||
		strings.HasPrefix(key, "CLAUDE_CODE_")
}

// ── Custom args filtering ──

type blockedArgMode int

const (
	blockedWithValue  blockedArgMode = iota // flag takes a value (next arg or =value)
	blockedStandalone                       // flag is boolean, no value
)

// filterCustomArgs removes protocol-critical flags from CustomArgs so the
// daemon-to-agent communication protocol cannot be broken. Only flags that
// would break the protocol are blocked — other dangerous flags are left
// intact because workspace members are trusted with agent configuration.
func filterCustomArgs(args []string, blocked map[string]blockedArgMode) []string {
	if len(args) == 0 {
		return args
	}
	filtered := make([]string, 0, len(args))
	skip := false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		flag := arg
		hasInlineValue := false
		if idx := strings.Index(arg, "="); idx > 0 {
			flag = arg[:idx]
			hasInlineValue = true
		}
		mode, isBlocked := blocked[flag]
		if isBlocked {
			if mode == blockedWithValue && !hasInlineValue {
				skip = true
			}
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

// ── Channel helper ──

func trySend(ch chan<- OutputChunk, chunk OutputChunk) {
	select {
	case ch <- chunk:
	default:
		// Channel full — drop message. Final output is accumulated
		// separately in Result.Output, so only streaming consumers
		// are affected.
	}
}

// ── Claude SDK JSON types ──

type claudeSDKMessage struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	ResultText string         `json:"result,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	ErrorText string          `json:"error,omitempty"`
}

type claudeMessageContent struct {
	Role    string               `json:"role"`
	Model   string               `json:"model"`
	Content []claudeContentBlock `json:"content"`
	Usage   *claudeUsage         `json:"usage,omitempty"`
}

type claudeUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// ── Stderr capture ──

// stderrTail wraps an io.Writer and retains the last maxBytes written,
// accessible via Tail(). Used to capture crash diagnostics from the
// agent CLI subprocess.
type stderrTail struct {
	w        io.Writer
	buf      []byte
	maxBytes int
	mu       sync.Mutex
}

func newStderrTail(w io.Writer, maxBytes int) *stderrTail {
	return &stderrTail{
		w:        w,
		maxBytes: maxBytes,
		buf:      make([]byte, 0, maxBytes),
	}
}

func (t *stderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.buf)+len(p) > t.maxBytes {
		overflow := len(t.buf) + len(p) - t.maxBytes
		if overflow >= len(t.buf) {
			t.buf = t.buf[:0]
		} else {
			t.buf = t.buf[overflow:]
		}
	}
	t.buf = append(t.buf, p...)

	return t.w.Write(p)
}

func (t *stderrTail) Tail() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// logWriter adapts a *slog.Logger to an io.Writer for forwarding stderr
// output to the structured log at Debug level.
type logWriter struct {
	logger *slog.Logger
	prefix string
}

func newLogWriter(logger *slog.Logger, prefix string) *logWriter {
	return &logWriter{logger: logger, prefix: prefix}
}

func (w *logWriter) Write(p []byte) (int, error) {
	text := strings.TrimSpace(string(p))
	if text != "" {
		w.logger.Debug(w.prefix + text)
	}
	return len(p), nil
}
