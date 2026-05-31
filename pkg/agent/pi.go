package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PiBackend implements Backend by spawning the Pi CLI in non-interactive
// JSON mode (`pi -p --mode json --session <path>`) and parsing its event
// stream on stdout.
type PiBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewPiBackend creates a new PiBackend.
// If executablePath is empty it defaults to "pi".
// If logger is nil, slog.Default() is used.
func NewPiBackend(executablePath string, logger *slog.Logger) *PiBackend {
	if executablePath == "" {
		executablePath = "pi"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PiBackend{executablePath: executablePath, logger: logger}
}

// Name returns "pi".
func (b *PiBackend) Name() string { return "pi" }

// Execute launches the pi CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *PiBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("pi executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	sessionPath, err := newPiSessionPath()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("pi session path: %w", err)
	}
	if err := ensurePiSessionFile(sessionPath); err != nil {
		cancel()
		return nil, fmt.Errorf("pi session file: %w", err)
	}

	args := buildPiArgs(prompt, sessionPath, opts)
	b.logger.Info("pi: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("pi: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[pi:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("pi: start: %w", err)
	}
	b.logger.Info("pi: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

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
		var output strings.Builder
		finalStatus := "completed"
		var finalError string
		usage := make(map[string]TokenUsage)

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

			var evt piStreamEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}

			switch evt.Type {
			case "message_update":
				if evt.AssistantMessageEvent != nil {
					delta := evt.AssistantMessageEvent.Delta
					if delta != "" {
						output.WriteString(delta)
						trySend(msgCh, OutputChunk{Type: string(MessageText), Content: delta})
					}
				}

			case "tool_execution_start":
				var input map[string]any
				if evt.Args != nil {
					_ = json.Unmarshal(evt.Args, &input)
				}
				trySend(msgCh, OutputChunk{
					Type: string(MessageToolUse),
					Tool: &ToolInfo{Name: evt.ToolName, CallID: evt.ToolCallID, Input: input},
				})

			case "tool_execution_end":
				resultContent := decodePiResult(evt.Result)
				trySend(msgCh, OutputChunk{
					Type: string(MessageToolResult),
					Tool: &ToolInfo{CallID: evt.ToolCallID, Output: resultContent},
				})

			case "error":
				errMsg := decodePiString(evt.Message)
				if errMsg != "" {
					trySend(msgCh, OutputChunk{Type: string(MessageError), Content: errMsg})
					if finalStatus == "completed" {
						finalStatus = "failed"
						finalError = errMsg
					}
				}

			case "turn_end":
				if m := decodePiMessage(evt.Message); m != nil {
					if m.Usage != nil {
						model := m.Model
						if model == "" {
							model = opts.Model
						}
						if model == "" {
							model = "unknown"
						}
						u := usage[model]
						u.InputTokens += m.Usage.Input
						u.OutputTokens += m.Usage.Output
						u.CacheReadTokens += m.Usage.CacheRead
						u.CacheWriteTokens += m.Usage.CacheWrite
						usage[model] = u
					}
				}

			case "auto_retry_end":
				if !evt.Success && evt.FinalError != "" {
					if finalStatus == "completed" {
						finalStatus = "failed"
						finalError = evt.FinalError
					}
				}
			}
		}

		exitErr := cmd.Wait()
		duration := time.Since(startTime)

		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			finalStatus = "timeout"
			finalError = fmt.Sprintf("pi timed out after %s", timeout)
		} else if errors.Is(runCtx.Err(), context.Canceled) {
			finalStatus = "cancelled"
			finalError = "execution cancelled"
		} else if exitErr != nil && finalStatus == "completed" {
			finalStatus = "failed"
			finalError = fmt.Sprintf("pi exited with error: %v", exitErr)
		}

		b.logger.Info("pi: finished",
			"status", finalStatus,
			"duration", duration.Round(time.Millisecond).String(),
		)

		resCh <- &Result{
			Status:     finalStatus,
			Output:     output.String(),
			Error:      finalError,
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

// ── Pi event types ──

type piStreamEvent struct {
	Type string `json:"type"`

	AssistantMessageEvent *piAssistantMessageEvent `json:"assistantMessageEvent,omitempty"`

	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	IsError    bool            `json:"isError,omitempty"`

	Message json.RawMessage `json:"message,omitempty"`

	Success    bool   `json:"success,omitempty"`
	FinalError string `json:"finalError,omitempty"`
}

type piAssistantMessageEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
}

type piMessage struct {
	Role    string   `json:"role,omitempty"`
	Model   string   `json:"model,omitempty"`
	Usage   *piUsage `json:"usage,omitempty"`
}

type piUsage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cacheRead"`
	CacheWrite  int64 `json:"cacheWrite"`
	TotalTokens int64 `json:"totalTokens"`
}

func decodePiMessage(raw json.RawMessage) *piMessage {
	if len(raw) == 0 {
		return nil
	}
	var m piMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return &m
}

func decodePiString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

func decodePiResult(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// ── CLI argument construction ──

var piBlockedArgs = map[string]blockedArgMode{
	"-p":        blockedStandalone,
	"--print":   blockedStandalone,
	"--mode":    blockedWithValue,
	"--session": blockedWithValue,
}

func buildPiArgs(prompt, sessionPath string, opts *ExecuteOptions) []string {
	args := []string{
		"-p",
		"--mode", "json",
	}
	if sessionPath != "" {
		args = append(args, "--session", sessionPath)
	}
	if opts.Model != "" {
		provider, model := splitPiModel(opts.Model)
		if provider != "" {
			args = append(args, "--provider", provider)
		}
		if model != "" {
			args = append(args, "--model", model)
		}
	}
	args = append(args, "--tools", "read,bash,edit,write,grep,find,ls")
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	args = append(args, filterCustomArgs(opts.CustomArgs, piBlockedArgs)...)
	args = append(args, prompt)
	return args
}

func splitPiModel(s string) (provider, model string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "/"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return "", s
}

// ── Session path ──

func piSessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".solo", "pi-sessions"), nil
}

func newPiSessionPath() (string, error) {
	dir, err := piSessionDir()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.jsonl", time.Now().UTC().Format("20060102T150405.000000000"))
	return filepath.Join(dir, name), nil
}

func ensurePiSessionFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
