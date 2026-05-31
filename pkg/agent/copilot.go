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
	"time"
)

// CopilotBackend implements Backend by spawning the GitHub Copilot CLI
// with --output-format json and parsing its JSONL event stream.
//
// The v1 integration uses -p (pipe) mode which is the stable automation/CI
// channel. The prompt is passed as a CLI argument. Events arrive as
// newline-delimited JSON on stdout: { "type": "event.name", "data": {...} }
type CopilotBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewCopilotBackend creates a new CopilotBackend.
// If executablePath is empty it defaults to "copilot".
// If logger is nil, slog.Default() is used.
func NewCopilotBackend(executablePath string, logger *slog.Logger) *CopilotBackend {
	if executablePath == "" {
		executablePath = "copilot"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CopilotBackend{executablePath: executablePath, logger: logger}
}

// Name returns "copilot".
func (b *CopilotBackend) Name() string { return "copilot" }

// Execute launches the copilot CLI subprocess, sends the prompt via CLI arg,
// streams output events through Session.Messages, and delivers the final
// result on Session.Result.
func (b *CopilotBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("copilot executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	args := buildCopilotArgs(prompt, opts)
	b.logger.Info("copilot: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("copilot: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[copilot:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("copilot: start: %w", err)
	}
	b.logger.Info("copilot: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

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

	go b.streamLoop(runCtx, cancel, cmd, stdout, msgCh, resCh, timeout)

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// ── Stream loop ──

func (b *CopilotBackend) streamLoop(
	runCtx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	msgCh chan<- OutputChunk,
	resCh chan<- *Result,
	timeout time.Duration,
) {
	defer cancel()
	defer close(msgCh)
	defer close(resCh)

	startTime := time.Now()
	var output strings.Builder
	sessionID := ""
	finalStatus := "completed"
	var finalError string
	usage := make(map[string]TokenUsage)

	// Close stdout when the context is cancelled so scanner.Scan() unblocks.
	go func() {
		<-runCtx.Done()
		_ = stdout.Close()
	}()

	st := newCopilotEventState("copilot")

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var evt copilotEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			b.logger.Debug("copilot: event parse failed", "err", err)
			continue
		}

		for _, chunk := range b.handleEvent(evt, st) {
			if chunk.Type == string(MessageText) {
				output.WriteString(chunk.Content)
			}
			trySend(msgCh, chunk)
		}
	}

	exitErr := cmd.Wait()
	duration := time.Since(startTime)

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		finalStatus = "timeout"
		finalError = fmt.Sprintf("copilot timed out after %s", timeout)
	} else if errors.Is(runCtx.Err(), context.Canceled) {
		finalStatus = "cancelled"
		finalError = "execution cancelled"
	} else if exitErr != nil && st.finalStatus == "completed" {
		finalStatus = "failed"
		finalError = fmt.Sprintf("copilot exited with error: %v", exitErr)
	}

	if st.finalStatus != "completed" {
		finalStatus = st.finalStatus
	}
	if st.finalError != "" {
		finalError = st.finalError
	}
	if st.sessionID != "" {
		sessionID = st.sessionID
	}
	if len(st.usage) > 0 {
		usage = st.usage
	}

	b.logger.Info("copilot: finished",
		"status", finalStatus,
		"session_id", sessionID,
		"duration", duration.Round(time.Millisecond).String(),
	)

	resCh <- &Result{
		Status:     finalStatus,
		Output:     output.String(),
		Error:      finalError,
		DurationMs: duration.Milliseconds(),
		Usage:      usage,
	}
}

// ── Event handler ──

// copilotEventState holds mutable state accumulated while processing the
// JSONL event stream.
type copilotEventState struct {
	output      strings.Builder
	sessionID   string
	activeModel string
	finalStatus string
	finalError  string
	usage       map[string]TokenUsage
}

func newCopilotEventState(seedModel string) *copilotEventState {
	return &copilotEventState{
		activeModel: seedModel,
		finalStatus: "completed",
		usage:       make(map[string]TokenUsage),
	}
}

func (b *CopilotBackend) handleEvent(evt copilotEvent, st *copilotEventState) []OutputChunk {
	var chunks []OutputChunk

	switch evt.Type {
	case "session.start":
		var ss copilotSessionStart
		if err := json.Unmarshal(evt.Data, &ss); err == nil {
			if ss.SelectedModel != "" {
				st.activeModel = ss.SelectedModel
			}
			if ss.SessionID != "" {
				st.sessionID = ss.SessionID
			}
		}

	case "assistant.message_delta":
		var delta copilotMessageDelta
		if err := json.Unmarshal(evt.Data, &delta); err == nil && delta.DeltaContent != "" {
			st.output.WriteString(delta.DeltaContent)
			chunks = append(chunks, OutputChunk{Type: string(MessageText), Content: delta.DeltaContent})
		}

	case "assistant.message":
		var msg copilotAssistantMessage
		if err := json.Unmarshal(evt.Data, &msg); err != nil {
			return nil
		}
		if msg.Content != "" {
			trimmed := strings.TrimSuffix(st.output.String(), msg.Content)
			st.output.Reset()
			st.output.WriteString(trimmed)
			if st.output.Len() > 0 && !strings.HasSuffix(st.output.String(), "\n\n") {
				st.output.WriteString("\n\n")
			}
			st.output.WriteString(msg.Content)
		}
		if msg.ReasoningText != "" {
			chunks = append(chunks, OutputChunk{Type: string(MessageThinking), Content: msg.ReasoningText})
		}
		if msg.OutputTokens > 0 {
			u := st.usage[st.activeModel]
			u.OutputTokens += msg.OutputTokens
			st.usage[st.activeModel] = u
		}
		for _, tr := range msg.ToolRequests {
			var input map[string]any
			if tr.Arguments != nil {
				_ = json.Unmarshal(tr.Arguments, &input)
			}
			chunks = append(chunks, OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: tr.Name, CallID: tr.ToolCallID, Input: input},
			})
		}

	case "assistant.reasoning", "assistant.reasoning_delta":
		var r copilotReasoning
		if err := json.Unmarshal(evt.Data, &r); err == nil {
			text := r.Content
			if text == "" {
				text = r.DeltaContent
			}
			if text != "" {
				chunks = append(chunks, OutputChunk{Type: string(MessageThinking), Content: text})
			}
		}

	case "tool.execution_complete":
		var tc copilotToolExecComplete
		if err := json.Unmarshal(evt.Data, &tc); err != nil {
			return nil
		}
		if tc.Model != "" {
			st.activeModel = tc.Model
		}
		resultContent := ""
		if tc.Success && tc.Result != nil {
			resultContent = tc.Result.Content
		} else if !tc.Success {
			if tc.Error != nil {
				resultContent = "Error: " + tc.Error.Message
			} else if tc.Result != nil {
				resultContent = tc.Result.Content
			}
		}
		chunks = append(chunks, OutputChunk{
			Type: string(MessageToolResult),
			Tool: &ToolInfo{CallID: tc.ToolCallID, Output: resultContent},
		})

	case "assistant.turn_start":
		chunks = append(chunks, OutputChunk{Type: string(MessageStatus), Content: "running"})

	case "session.error":
		var se copilotSessionError
		if err := json.Unmarshal(evt.Data, &se); err == nil {
			st.finalStatus = "failed"
			st.finalError = se.Message
			chunks = append(chunks, OutputChunk{Type: string(MessageError), Content: se.Message})
		}

	case "session.warning":
		var sw copilotSessionWarning
		if err := json.Unmarshal(evt.Data, &sw); err == nil && sw.Message != "" {
			b.logger.Debug("copilot: warning", "message", sw.Message)
		}

	case "result":
		if evt.SessionID != "" {
			st.sessionID = evt.SessionID
		}
		if evt.ExitCode != 0 {
			st.finalStatus = "failed"
			st.finalError = fmt.Sprintf("copilot exited with code %d", evt.ExitCode)
		}
	}

	return chunks
}

// ── Copilot JSON types ──

type copilotEvent struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	ID        string          `json:"id,omitempty"`
	Timestamp string          `json:"timestamp,omitempty"`
	ParentID  string          `json:"parentId,omitempty"`
	Ephemeral bool            `json:"ephemeral,omitempty"`

	// Top-level fields on the synthetic "result" event only.
	SessionID string              `json:"sessionId,omitempty"`
	ExitCode  int                 `json:"exitCode,omitempty"`
	Usage     *copilotResultUsage `json:"usage,omitempty"`
}

type copilotSessionStart struct {
	SessionID     string `json:"sessionId"`
	SelectedModel string `json:"selectedModel"`
}

type copilotAssistantMessage struct {
	MessageID     string               `json:"messageId"`
	Content       string               `json:"content"`
	ToolRequests  []copilotToolRequest `json:"toolRequests"`
	OutputTokens  int64                `json:"outputTokens"`
	ReasoningText string               `json:"reasoningText,omitempty"`
}

type copilotToolRequest struct {
	ToolCallID       string          `json:"toolCallId"`
	Name             string          `json:"name"`
	Arguments        json.RawMessage `json:"arguments"`
	Type             string          `json:"type"`
	IntentionSummary string          `json:"intentionSummary,omitempty"`
}

type copilotMessageDelta struct {
	MessageID    string `json:"messageId"`
	DeltaContent string `json:"deltaContent"`
}

type copilotToolExecComplete struct {
	ToolCallID string             `json:"toolCallId"`
	Model      string             `json:"model"`
	Success    bool               `json:"success"`
	Result     *copilotToolResult `json:"result,omitempty"`
	Error      *copilotToolError  `json:"error,omitempty"`
}

type copilotToolResult struct {
	Content string `json:"content"`
}

type copilotToolError struct {
	Message string `json:"message"`
}

type copilotReasoning struct {
	Content      string `json:"content,omitempty"`
	DeltaContent string `json:"deltaContent,omitempty"`
}

type copilotSessionError struct {
	ErrorType string `json:"errorType"`
	Message   string `json:"message"`
}

type copilotSessionWarning struct {
	WarningType string `json:"warningType"`
	Message     string `json:"message"`
}

type copilotResultUsage struct {
	PremiumRequests    float64 `json:"premiumRequests"`
	TotalAPIDurationMs int64   `json:"totalApiDurationMs"`
	SessionDurationMs  int64   `json:"sessionDurationMs"`
}

// ── CLI argument construction ──

var copilotBlockedArgs = map[string]blockedArgMode{
	"-p":              blockedStandalone,
	"--output-format": blockedWithValue,
}

func buildCopilotArgs(prompt string, opts *ExecuteOptions) []string {
	args := []string{
		"-p",
		"--output-format", "json",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--instructions", opts.SystemPrompt)
	}
	args = append(args, filterCustomArgs(opts.CustomArgs, copilotBlockedArgs)...)
	args = append(args, prompt)
	return args
}
