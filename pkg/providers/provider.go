package providers

import (
	"context"
)

// ToolCallRequest represents a tool call request from the LLM.
type ToolCallRequest struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// LLMResponse represents a response from an LLM provider.
type LLMResponse struct {
	Content      string            `json:"content,omitempty"`
	ToolCalls    []ToolCallRequest `json:"tool_calls,omitempty"`
	FinishReason string            `json:"finish_reason"`
	Usage        map[string]int    `json:"usage"`
}

// HasToolCalls checks if the response contains tool calls.
func (r *LLMResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// LLMProvider is the interface for LLM providers.
type LLMProvider interface {
	Chat(ctx context.Context, messages []interface{}, tools []interface{}, model string) (*LLMResponse, error)
	Stream(ctx context.Context, messages []interface{}, tools []interface{}, model string) (<-chan LLMStreamChunk, error)
	GetDefaultModel() string
}

// LLMStreamChunk represents a chunk of the streaming response.
type LLMStreamChunk struct {
	Content      string         `json:"content,omitempty"`
	ToolCall     *ToolCallChunk `json:"tool_call,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
	Usage        map[string]int `json:"usage,omitempty"`
	Error        error          `json:"error,omitempty"`
}

type ToolCallChunk struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
