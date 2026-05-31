package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements the Provider interface for Anthropic Claude API.
type AnthropicProvider struct {
	apiKey  string
	client  *http.Client
	baseURL string
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
		baseURL: anthropicAPIURL,
	}
}

type anthropicRequest struct {
	Model       string                `json:"model"`
	Messages    []anthropicMessage    `json:"messages"`
	System      string                `json:"system,omitempty"`
	Temperature float64               `json:"temperature"`
	MaxTokens   int                   `json:"max_tokens"`
	Stream      bool                  `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete sends a non-streaming completion request to Anthropic.
func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	body := anthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		System:      req.SystemPrompt,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return nil, fmt.Errorf("anthropic: api error: %s - %s", anthropicResp.Error.Type, anthropicResp.Error.Message)
	}

	// Concatenate all content blocks
	var fullContent string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			fullContent += block.Text
		}
	}

	return &CompletionResponse{
		Content: fullContent,
		Usage: Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

// CompleteStream sends a streaming completion request to Anthropic.
// Not fully implemented for Week 5 — returns a single-chunk channel.
func (p *AnthropicProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)

	resp, err := p.Complete(ctx, req)
	if err != nil {
		ch <- StreamChunk{Error: err}
		close(ch)
		return ch, nil
	}

	ch <- StreamChunk{Content: resp.Content}
	ch <- StreamChunk{Done: true}
	close(ch)
	return ch, nil
}
