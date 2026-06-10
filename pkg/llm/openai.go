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

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider implements the Provider interface for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey  string
	client  *http.Client
	baseURL string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
		baseURL: openaiAPIURL,
	}
}

type openAIRequest struct {
	Model       string            `json:"model"`
	Messages    []openAIMessage   `json:"messages"`
	Stream      bool              `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Complete sends a non-streaming completion request to OpenAI.
func (p *OpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		messages = append(messages, openAIMessage{Role: m.Role, Content: m.Content})
	}

	body := openAIRequest{
		Model:       req.Model,
		Messages:    messages,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("openai: api error: %s - %s", openAIResp.Error.Type, openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	return &CompletionResponse{
		Content: openAIResp.Choices[0].Message.Content,
		Usage: Usage{
			InputTokens:  openAIResp.Usage.PromptTokens,
			OutputTokens: openAIResp.Usage.CompletionTokens,
		},
	}, nil
}

// CompleteStream sends a streaming completion request to OpenAI.
// Not fully implemented for Week 5 — returns a single-chunk channel.
func (p *OpenAIProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
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
