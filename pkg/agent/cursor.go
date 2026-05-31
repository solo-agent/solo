package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// cursorBackend implements Backend by spawning the Cursor Agent CLI
// (cursor-agent) with --output-format stream-json and parsing the JSONL
// event stream.
type CursorBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewCursorBackend creates a new CursorBackend.
// If executablePath is empty it defaults to "cursor-agent".
func NewCursorBackend(executablePath string, logger *slog.Logger) *CursorBackend {
	if executablePath == "" {
		executablePath = "cursor-agent"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CursorBackend{executablePath: executablePath, logger: logger}
}

// Name returns "cursor".
func (b *CursorBackend) Name() string { return "cursor" }

// Execute launches the cursor-agent CLI subprocess, sends the prompt, streams
// output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *CursorBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execName := b.executablePath
	lookedUp, err := exec.LookPath(execName)
	if err != nil {
		return nil, fmt.Errorf("cursor-agent executable not found at %q: %w", execName, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	prompt := buildPrompt(req, opts)
	args := buildCursorArgs(prompt, opts)

	cmd := exec.CommandContext(runCtx, lookedUp, args...)
	cmd.WaitDelay = 20 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cursor: stdout pipe: %w", err)
	}
	cmd.Stderr = newLogWriter(b.logger, "[cursor:stderr] ")

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start cursor-agent: %w", err)
	}
	b.logger.Info("cursor: started", "pid", cmd.Process.Pid, "cwd", opts.WorkspaceDir, "model", opts.Model)

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

		go func() {
			<-runCtx.Done()
			_ = stdout.Close()
		}()

		startTime := time.Now()
		var output strings.Builder
		finalStatus := "completed"
		var finalError string
		stepUsage := make(map[string]TokenUsage)
		resultUsage := make(map[string]TokenUsage)
		hasResultUsage := false

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

		for scanner.Scan() {
			raw := scanner.Text()
			line := normalizeCursorStreamLine(raw)
			if line == "" {
				continue
			}

			var evt cursorStreamEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}


			switch evt.Type {
			case "system":
				if evt.Subtype == "init" {
					trySend(msgCh, OutputChunk{Type: string(MessageStatus), Content: "running"})
				}
				if evt.Subtype == "error" {
					errMsg := cursorErrorText(&evt)
					if errMsg != "" {
						trySend(msgCh, OutputChunk{Type: string(MessageError), Content: errMsg})
					}
				}

			case "assistant":
				b.handleCursorAssistant(&evt, msgCh, &output)

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

			case "result":
				if evt.IsError || evt.Subtype == "error" {
					finalStatus = "failed"
					finalError = cursorErrorText(&evt)
				}
				if evt.ResultText != "" && output.Len() == 0 {
					output.WriteString(evt.ResultText)
				}
				if evt.Usage != nil {
					hasResultUsage = true
					model := evt.Model
					if model == "" {
						model = "cursor"
					}
					u := resultUsage[model]
					u.InputTokens += evt.Usage.InputTokens
					u.OutputTokens += evt.Usage.OutputTokens
					u.CacheReadTokens += evt.Usage.CacheReadInputTokens
					resultUsage[model] = u
				}

			case "error":
				errMsg := cursorErrorText(&evt)
				if errMsg != "" {
					finalError = errMsg
				}
				trySend(msgCh, OutputChunk{Type: string(MessageError), Content: errMsg})

			case "text":
				if evt.Part != nil {
					var part cursorTextPart
					_ = json.Unmarshal(evt.Part, &part)
					if part.Text != "" {
						output.WriteString(part.Text)
						trySend(msgCh, OutputChunk{Type: string(MessageText), Content: part.Text})
					}
				}

			case "step_finish":
				if evt.Part != nil {
					var part cursorStepFinishPart
					_ = json.Unmarshal(evt.Part, &part)
					model := evt.Model
					if model == "" {
						model = "cursor"
					}
					u := stepUsage[model]
					u.InputTokens += int64(part.Tokens.Input)
					u.OutputTokens += int64(part.Tokens.Output)
					u.CacheReadTokens += int64(part.Tokens.Cache.Read)
					stepUsage[model] = u
				}
			}
		}

		if !hasResultUsage {
			resultUsage = stepUsage
		}

		exitErr := cmd.Wait()
		duration := time.Since(startTime)

		if runCtx.Err() == context.DeadlineExceeded {
			finalStatus = "timeout"
			finalError = fmt.Sprintf("cursor-agent timed out after %s", timeout)
		} else if runCtx.Err() == context.Canceled {
			finalStatus = "cancelled"
			finalError = "execution cancelled"
		} else if exitErr != nil && finalStatus == "completed" {
			finalStatus = "failed"
			finalError = fmt.Sprintf("cursor-agent exited with error: %v", exitErr)
		}

		b.logger.Info("cursor: finished", "pid", cmd.Process.Pid, "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

		resCh <- &Result{
			Status:     finalStatus,
			Output:     output.String(),
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
			Usage:      resultUsage,
		}
	}()

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

func (b *CursorBackend) handleCursorAssistant(evt *cursorStreamEvent, ch chan<- OutputChunk, output *strings.Builder) {
	if evt.Message == nil {
		return
	}

	var content cursorAssistantMessage
	if err := json.Unmarshal(evt.Message, &content); err != nil {
		return
	}

	for _, block := range content.Content {
		switch block.Type {
		case "output_text", "text":
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
				Tool: &ToolInfo{Name: block.Name, CallID: block.ID, Input: input},
			})
		}
	}
}

// ── Cursor stream-json types ──

type cursorStreamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`

	Message json.RawMessage `json:"message,omitempty"`

	ToolName   string          `json:"tool_name,omitempty"`
	ToolID     string          `json:"tool_id,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`

	Output string `json:"output,omitempty"`

	ResultText string       `json:"result,omitempty"`
	IsError    bool         `json:"is_error,omitempty"`
	Usage      *cursorUsage `json:"usage,omitempty"`

	ErrorMsg string `json:"error,omitempty"`
	Detail   string `json:"detail,omitempty"`

	Part json.RawMessage `json:"part,omitempty"`
}

type cursorUsage struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheReadInputTokens int64 `json:"cached_input_tokens"`
}

type cursorAssistantMessage struct {
	Model   string               `json:"model"`
	Content []cursorContentBlock `json:"content"`
}

type cursorContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type cursorTextPart struct {
	Text string `json:"text"`
}

type cursorStepFinishPart struct {
	Tokens struct {
		Input  int `json:"input"`
		Output int `json:"output"`
		Cache  struct {
			Read int `json:"read"`
		} `json:"cache"`
	} `json:"tokens"`
}

// ── Helpers ──

var cursorStreamPrefixRe = regexp.MustCompile(`^(?i)(stdout|stderr)\s*[:=]?\s*`)

func normalizeCursorStreamLine(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if idx := cursorStreamPrefixRe.FindStringIndex(trimmed); idx != nil {
		return strings.TrimSpace(trimmed[idx[1]:])
	}
	return trimmed
}

func cursorErrorText(evt *cursorStreamEvent) string {
	if evt.ErrorMsg != "" {
		return evt.ErrorMsg
	}
	if evt.Detail != "" {
		return evt.Detail
	}
	if evt.ResultText != "" {
		return evt.ResultText
	}
	return ""
}

// cursorBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var cursorBlockedArgs = map[string]blockedArgMode{
	"-p":              blockedStandalone,
	"--output-format": blockedWithValue,
	"--yolo":          blockedStandalone,
}

func buildCursorArgs(prompt string, opts *ExecuteOptions) []string {
	args := []string{
		"chat",
		"-p", prompt,
		"--output-format", "stream-json",
		"--yolo",
	}
	if opts.WorkspaceDir != "" {
		args = append(args, "--workspace", opts.WorkspaceDir)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, filterCustomArgs(opts.CustomArgs, cursorBlockedArgs)...)
	return args
}
