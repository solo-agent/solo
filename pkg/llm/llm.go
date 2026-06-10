// Package llm provides interfaces and implementations for LLM providers
// (OpenAI, Anthropic, Local CLI) used by the Agent Daemon to generate responses.
package llm

import "context"

// Provider is the interface for LLM providers (OpenAI, Anthropic, Local CLI, etc.).
type Provider interface {
	// Complete executes a non-streaming completion request and returns the full response.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// CompleteStream executes a streaming completion and returns a channel of chunks.
	// The caller must consume the channel until it is closed or the context is cancelled.
	CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
}

// CompletionRequest contains all parameters for an LLM completion call.
type CompletionRequest struct {
	Model        string    `json:"model"`
	Messages     []Message `json:"messages"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
}

// CompletionResponse contains the result of a non-streaming LLM completion.
type CompletionResponse struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

// Message represents a single message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage contains token usage information from an LLM call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamChunk represents a single chunk from a streaming LLM response.
type StreamChunk struct {
	Content string
	Done    bool
	Error   error
	Usage   Usage
}

// NewProvider creates an LLM provider based on the given type.
// Supported types: "anthropic", "openai", "local".
// The "local" type executes a locally installed CLI agent (e.g.,
// the Claude Code CLI `claude` command) instead of calling an API.
func NewProvider(providerType, apiKey string) Provider {
	switch providerType {
	case "openai":
		return NewOpenAIProvider(apiKey)
	case "anthropic":
		return NewAnthropicProvider(apiKey)
	case "local":
		return NewLocalProvider("")
	default:
		return NewAnthropicProvider(apiKey)
	}
}
