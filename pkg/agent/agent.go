// Package agent defines the core Agent runtime interface and types used for
// communication between the Solo server and the Agent daemon.
package agent

import "context"

// StreamEventType represents the type of a streaming event from the LLM.
type StreamEventType string

const (
	StreamEventToken    StreamEventType = "token"     // A single token of generated text
	StreamEventToolCall StreamEventType = "tool_call"  // Agent is calling a tool
	StreamEventError    StreamEventType = "error"      // An error occurred during generation
	StreamEventDone     StreamEventType = "done"       // Generation completed successfully
)

// Role represents the sender role in a conversation message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Message represents a single message in the conversation context.
type Message struct {
	Role     Role   `json:"role"`
	Content  string `json:"content"`
	SenderID string `json:"sender_id,omitempty"`
}

// ModelConfig represents the LLM model configuration for an agent run.
type ModelConfig struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

// RunRequest contains all parameters needed to invoke an agent.
type RunRequest struct {
	AgentID      string       `json:"agent_id"`
	ChannelID    string       `json:"channel_id"`
	ThreadID     string       `json:"thread_id,omitempty"`
	Messages     []Message    `json:"messages"`
	SystemPrompt string       `json:"system_prompt"`
	ModelConfig  ModelConfig  `json:"model_config"`
}

// RunResponse contains the result of a non-streaming agent run.
type RunResponse struct {
	AgentID   string `json:"agent_id"`
	Content   string `json:"content"`
	Usage     Usage  `json:"usage,omitempty"`
}

// Usage contains token usage information from the LLM call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent represents a single event in a streaming agent response.
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	AgentID string          `json:"agent_id"`
	Content interface{}     `json:"content"`
	Usage   *Usage          `json:"usage,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// AgentRuntime defines the interface for executing agent tasks.
// Implementations may connect to LLM providers, manage tool execution,
// and handle streaming responses.
type AgentRuntime interface {
	// Run executes the agent in non-streaming mode and returns the full response.
	Run(ctx context.Context, req *RunRequest) (*RunResponse, error)

	// Stream executes the agent in streaming mode and returns a channel of events.
	// The caller must consume the channel until it is closed or the context is cancelled.
	Stream(ctx context.Context, req *RunRequest) (<-chan StreamEvent, error)

	// Stop cancels a running agent task by its agent ID.
	Stop(agentID string) error
}
