package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// GeminiBackend implements Backend by spawning the Google Gemini CLI
// with `--output-format stream-json` and parsing its NDJSON event stream.
type GeminiBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewGeminiBackend creates a new GeminiBackend.
// If executablePath is empty it defaults to "gemini".
func NewGeminiBackend(executablePath string, logger *slog.Logger) *GeminiBackend {
	if executablePath == "" {
		executablePath = "gemini"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GeminiBackend{executablePath: executablePath, logger: logger}
}

// Name returns "gemini".
func (b *GeminiBackend) Name() string { return "gemini" }

// Execute launches the gemini CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *GeminiBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("gemini executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	args := buildGeminiArgs(prompt, opts)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gemini: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[gemini:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start gemini: %w", err)
	}
	b.logger.Info("gemini: started", "pid", cmd.Process.Pid, "cwd", opts.WorkspaceDir, "model", opts.Model)

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
		<-runCtx.Done()
		_ = stdout.Close()
	}()

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)

		startTime := time.Now()
		var output strings.Builder
		finalStatus := "completed"
		var finalError string
		usage := make(map[string]TokenUsage)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var evt geminiStreamEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}

			switch evt.Type {
			case "init":
				trySend(msgCh, OutputChunk{Type: string(MessageStatus), Content: "running"})

			case "message":
				if evt.Role == "assistant" && evt.Content != "" {
					output.WriteString(evt.Content)
					trySend(msgCh, OutputChunk{Type: string(MessageText), Content: evt.Content})
				}

			case "tool_use":
				var params map[string]any
				if evt.Parameters != nil {
					_ = json.Unmarshal(evt.Parameters, &params)
				}
				trySend(msgCh, OutputChunk{
					Type: string(MessageToolUse),
					Tool: &ToolInfo{Name: evt.ToolName, CallID: evt.ToolID, Input: params},
				})

			case "tool_result":
				trySend(msgCh, OutputChunk{
					Type: string(MessageToolResult),
					Tool: &ToolInfo{CallID: evt.ToolID, Output: evt.Output},
				})

			case "error":
				trySend(msgCh, OutputChunk{Type: string(MessageError), Content: evt.Message})

			case "result":
				if evt.Status == "error" && evt.Error != nil {
					finalStatus = "failed"
					finalError = evt.Error.Message
				}
				if evt.Stats != nil {
					for model, m := range evt.Stats.Models {
						u := usage[model]
						u.InputTokens += int64(m.InputTokens)
						u.OutputTokens += int64(m.OutputTokens)
						u.CacheReadTokens += int64(m.Cached)
						usage[model] = u
					}
				}
			}
		}

		waitErr := cmd.Wait()
		duration := time.Since(startTime)

		if runCtx.Err() == context.DeadlineExceeded {
			finalStatus = "timeout"
			finalError = fmt.Sprintf("gemini timed out after %s", timeout)
		} else if runCtx.Err() == context.Canceled {
			finalStatus = "cancelled"
			finalError = "execution cancelled"
		} else if waitErr != nil && finalStatus == "completed" {
			finalStatus = "failed"
			finalError = fmt.Sprintf("gemini exited with error: %v", waitErr)
		}

		b.logger.Info("gemini: finished", "pid", cmd.Process.Pid, "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

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

// ── Gemini stream-json event types ──

type geminiStreamEvent struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`

	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Delta   bool   `json:"delta,omitempty"`

	ToolName   string          `json:"tool_name,omitempty"`
	ToolID     string          `json:"tool_id,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`

	Status string `json:"status,omitempty"`
	Output string `json:"output,omitempty"`

	Severity string            `json:"severity,omitempty"`
	Message  string            `json:"message,omitempty"`

	Error *geminiStreamError `json:"error,omitempty"`
	Stats *geminiStreamStats `json:"stats,omitempty"`
}

type geminiStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type geminiStreamStats struct {
	TotalTokens  int                         `json:"total_tokens"`
	InputTokens  int                         `json:"input_tokens"`
	OutputTokens int                         `json:"output_tokens"`
	DurationMs   int                         `json:"duration_ms"`
	ToolCalls    int                         `json:"tool_calls"`
	Models       map[string]geminiModelStats `json:"models,omitempty"`
}

type geminiModelStats struct {
	TotalTokens  int `json:"total_tokens"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	Cached       int `json:"cached"`
}

// ── Arg builder ──

var geminiBlockedArgs = map[string]blockedArgMode{
	"-p":     blockedWithValue,
	"--yolo": blockedStandalone,
	"-o":     blockedWithValue,
}

func buildGeminiArgs(prompt string, opts *ExecuteOptions) []string {
	args := []string{
		"-p", prompt,
		"--yolo",
		"-o", "stream-json",
	}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	args = append(args, filterCustomArgs(opts.CustomArgs, geminiBlockedArgs)...)
	return args
}
