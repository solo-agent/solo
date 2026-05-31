package llm

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LocalProvider implements the Provider interface by executing a local
// CLI agent (e.g., Claude Code CLI / claude) to generate responses.
// This enables Solo to reuse locally installed AI CLI tools as agent
// backends instead of requiring direct LLM API keys.
type LocalProvider struct {
	binary string // Path or name of the CLI binary (default: "claude")
}

// NewLocalProvider creates a new LocalProvider.
// binary is the path/name of the CLI executable; if empty, it defaults
// to the CLAUDECODE_BIN env var, then "claude".
func NewLocalProvider(binary string) *LocalProvider {
	if binary == "" {
		binary = os.Getenv("CLAUDECODE_BIN")
	}
	if binary == "" {
		binary = "claude"
	}
	return &LocalProvider{binary: binary}
}

// Complete executes the local CLI agent and returns the full response.
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	prompt := p.buildPrompt(req)

	// Use a timeout context copied from the parent to ensure the CLI
	// doesn't hang indefinitely (5 min default, same as AgentService).
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	slog.Debug("local: executing CLI binary",
		"binary", p.binary,
		"prompt_length", len(prompt),
		"message_count", len(req.Messages),
	)

	cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt, "--print")
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_HEADLESS=true")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		errMsg := err.Error()
		if ok {
			errMsg = fmt.Sprintf("exit code %d: %s", exitErr.ExitCode(), stderr.String())
		}
		slog.Error("local: CLI execution failed",
			"binary", p.binary,
			"error", errMsg,
			"stderr", stderr.String(),
		)
		return nil, fmt.Errorf("local provider: %s", errMsg)
	}

	content := stdout.String()
	inputTokens := estimateTokens(prompt)
	outputTokens := estimateTokens(content)

	slog.Debug("local: CLI execution completed",
		"binary", p.binary,
		"content_length", len(content),
	)

	return &CompletionResponse{
		Content: content,
		Usage: Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}, nil
}

// CompleteStream executes the local CLI agent and returns the full
// response as a single streaming chunk (true streaming via the CLI
// pipe requires process-level buffering; future work can add it).
func (p *LocalProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 2)

	go func() {
		defer close(ch)
		resp, err := p.Complete(ctx, req)
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		ch <- StreamChunk{Content: resp.Content}
		ch <- StreamChunk{Done: true, Usage: resp.Usage}
	}()

	return ch, nil
}

// buildPrompt formats the conversation messages into a single text
// prompt suitable for the CLI agent.
func (p *LocalProvider) buildPrompt(req *CompletionRequest) string {
	var b strings.Builder

	// Add system prompt first
	if req.SystemPrompt != "" {
		b.WriteString(req.SystemPrompt)
		b.WriteString("\n\n")
	}

	// Add conversation messages
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			b.WriteString("User: ")
		case "assistant":
			b.WriteString("Assistant: ")
		case "system":
			b.WriteString("System: ")
		default:
			b.WriteString(fmt.Sprintf("[%s]: ", msg.Role))
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}

	// Always end with an assistant prompt to indicate we want a response
	b.WriteString("Assistant:")

	return b.String()
}

// estimateTokenCount provides a rough estimate of token usage by
// counting words and multiplying by a heuristic factor.
// This is a best-effort approximation for display purposes.
func estimateTokens(text string) int {
	words := len(strings.Fields(text))
	chars := len(text)
	// Rough heuristic: ~1.3 tokens per word, or ~4 chars per token
	tokens := words * 13 / 10
	if tokens2 := chars / 4; tokens2 > tokens {
		tokens = tokens2
	}
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}
