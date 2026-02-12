package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIProvider implements the LLMProvider interface for OpenAI-compatible APIs.
type OpenAIProvider struct {
	APIKey  string
	APIBase string
	Model   string
}

// NewOpenAIProvider creates a new OpenAIProvider.
func NewOpenAIProvider(apiKey, apiBase, defaultModel string) *OpenAIProvider {
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	if defaultModel == "" {
		defaultModel = "gpt-4o"
	}
	return &OpenAIProvider{
		APIKey:  apiKey,
		APIBase: apiBase,
		Model:   defaultModel,
	}
}

// Chat sends a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []interface{}, tools []interface{}, model string) (*LLMResponse, error) {
	if model == "" {
		model = p.Model
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(p.APIBase, "/"))

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.APIKey))

	// Handle special headers for providers like OpenRouter if needed
	if strings.Contains(p.APIBase, "openrouter.ai") {
		req.Header.Set("HTTP-Referer", "https://github.com/HKUDS/nanobot")
		req.Header.Set("X-Title", "nanobot")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := response.Choices[0]
	llmResp := &LLMResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: map[string]int{
			"prompt_tokens":     response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
			"total_tokens":      response.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			// If arguments are not valid JSON, we might want to log it or handle it gracefully
			// For now, let's treat it as empty or error?
			// Some models return partial JSON or bad JSON.
			args = make(map[string]interface{})
		}

		llmResp.ToolCalls = append(llmResp.ToolCalls, ToolCallRequest{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return llmResp, nil
}

// Stream sends a chat completion request with streaming.
func (p *OpenAIProvider) Stream(ctx context.Context, messages []interface{}, tools []interface{}, model string) (<-chan LLMStreamChunk, error) {
	if model == "" {
		model = p.Model
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(p.APIBase, "/"))

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}

	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// log.Printf("Sending request to OpenAI API: %s", string(jsonBody))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.APIKey))

	if strings.Contains(p.APIBase, "openrouter.ai") {
		req.Header.Set("HTTP-Referer", "https://github.com/HKUDS/nanobot")
		req.Header.Set("X-Title", "nanobot")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	ch := make(chan LLMStreamChunk)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- LLMStreamChunk{Error: err}
				}
				return
			}

			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// Ignore parse errors for partial lines or keep going
				continue
			}
			// log.Printf("Received chunk: %s", data)

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]

				// Send content if present
				if choice.Delta.Content != "" {
					ch <- LLMStreamChunk{Content: choice.Delta.Content}
				}

				// Send tool calls if present
				for _, tc := range choice.Delta.ToolCalls {
					ch <- LLMStreamChunk{
						ToolCall: &ToolCallChunk{
							Index:     tc.Index,
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}

				if choice.FinishReason != "" {
					ch <- LLMStreamChunk{FinishReason: choice.FinishReason}
				}
			}

			// Some providers send usage in the stream
			if chunk.Usage.TotalTokens > 0 {
				ch <- LLMStreamChunk{Usage: map[string]int{
					"prompt_tokens":     chunk.Usage.PromptTokens,
					"completion_tokens": chunk.Usage.CompletionTokens,
					"total_tokens":      chunk.Usage.TotalTokens,
				}}
			}
		}
	}()

	return ch, nil
}

// GetDefaultModel returns the default model.
func (p *OpenAIProvider) GetDefaultModel() string {
	return p.Model
}
