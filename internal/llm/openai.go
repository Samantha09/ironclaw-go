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

// OpenAIClient 调用 OpenAI 兼容 API。
type OpenAIClient struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIClient 创建 OpenAI 客户端。
func NewOpenAIClient(model, apiKey, baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *OpenAIClient) ModelName() string { return c.model }

func (c *OpenAIClient) Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResponse, error) {
	reqBody := c.buildRequestBody(messages, tools, false)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result openAICompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("no choices in response")
	}

	choice := result.Choices[0]
	return CompletionResponse{
		Content:   choice.Message.Content,
		ToolCalls: c.convertToolCalls(choice.Message.ToolCalls),
		Usage: Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
		Model: result.Model,
	}, nil
}

func (c *OpenAIClient) StreamComplete(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	reqBody := c.buildRequestBody(messages, tools, true)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Error: ctx.Err()}
				return
			default:
			}

			var chunk openAIStreamChunk
			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					return
				}
				ch <- StreamChunk{Error: fmt.Errorf("decode stream: %w", err)}
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta
			ch <- StreamChunk{
				Content:   delta.Content,
				ToolCalls: c.convertToolCalls(delta.ToolCalls),
				Done:      chunk.Choices[0].FinishReason != "",
			}
		}
	}()

	return ch, nil
}

func (c *OpenAIClient) buildRequestBody(messages []Message, tools []ToolDefinition, stream bool) map[string]any {
	req := map[string]any{
		"model":    c.model,
		"messages": c.convertMessages(messages),
		"stream":   stream,
	}
	if len(tools) > 0 {
		req["tools"] = tools
	}
	return req
}

func (c *OpenAIClient) convertMessages(msgs []Message) []map[string]any {
	result := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		msg := map[string]any{
			"role":    string(m.Role),
			"content": m.Content,
		}
		if m.Role == RoleTool {
			msg["tool_call_id"] = m.ToolID
			msg["name"] = m.ToolName
		}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = c.convertToOpenAIToolCalls(m.ToolCalls)
		}
		result[i] = msg
	}
	return result
}

func (c *OpenAIClient) convertToOpenAIToolCalls(calls []ToolCall) []map[string]any {
	result := make([]map[string]any, len(calls))
	for i, call := range calls {
		result[i] = map[string]any{
			"id":   call.ID,
			"type": call.Type,
			"function": map[string]any{
				"name":      call.Function.Name,
				"arguments": call.Function.Arguments,
			},
		}
	}
	return result
}

func (c *OpenAIClient) convertToolCalls(calls []openAIToolCall) []ToolCall {
	if calls == nil {
		return nil
	}
	result := make([]ToolCall, len(calls))
	for i, call := range calls {
		result[i] = ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		}
	}
	return result
}

// --- OpenAI API 结构 ---

type openAICompletionResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Choices []openAIChoice  `json:"choices"`
	Usage   openAIUsage     `json:"usage"`
}

type openAIChoice struct {
	Index        int              `json:"index"`
	Message      openAIMessage    `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type openAIMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function openAIFunctionCall   `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChunk struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Index        int           `json:"index"`
	Delta        openAIMessage `json:"delta"`
	FinishReason string        `json:"finish_reason"`
}
