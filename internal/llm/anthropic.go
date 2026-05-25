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

// AnthropicClient 调用 Anthropic Claude API。
type AnthropicClient struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropicClient 创建 Anthropic 客户端。
func NewAnthropicClient(model, apiKey, baseURL string) *AnthropicClient {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *AnthropicClient) ModelName() string { return c.model }

func (c *AnthropicClient) Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResponse, error) {
	reqBody := c.buildRequestBody(messages, tools, false)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return c.parseResponse(result), nil
}

func (c *AnthropicClient) StreamComplete(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	reqBody := c.buildRequestBody(messages, tools, true)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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

			var chunk anthropicStreamEvent
			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					return
				}
				ch <- StreamChunk{Error: fmt.Errorf("decode stream: %w", err)}
				return
			}

			if chunk.Type == "content_block_delta" && chunk.Delta.Type == "text_delta" {
				ch <- StreamChunk{Content: chunk.Delta.Text}
			}
			if chunk.Type == "message_stop" {
				ch <- StreamChunk{Done: true}
				return
			}
		}
	}()

	return ch, nil
}

func (c *AnthropicClient) buildRequestBody(messages []Message, tools []ToolDefinition, stream bool) map[string]any {
	req := map[string]any{
		"model":    c.model,
		"messages": c.convertMessages(messages),
		"max_tokens": 4096,
		"stream":   stream,
	}
	if len(tools) > 0 {
		req["tools"] = c.convertTools(tools)
	}
	return req
}

func (c *AnthropicClient) convertMessages(msgs []Message) []map[string]any {
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		// Anthropic 不支持 system 角色在 messages 中，需要特殊处理
		if m.Role == RoleSystem {
			continue
		}
		msg := map[string]any{
			"role":    string(m.Role),
			"content": m.Content,
		}
		result = append(result, msg)
	}
	return result
}

func (c *AnthropicClient) convertTools(tools []ToolDefinition) []map[string]any {
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = map[string]any{
			"name":         t.Function.Name,
			"description":  t.Function.Description,
			"input_schema": t.Function.Parameters,
		}
	}
	return result
}

func (c *AnthropicClient) parseResponse(resp anthropicResponse) CompletionResponse {
	var content string
	var toolCalls []ToolCall

	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
		if block.Type == "tool_use" {
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}

	return CompletionResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		Model: resp.Model,
	}
}

// --- Anthropic API 结构 ---

type anthropicResponse struct {
	ID      string                  `json:"id"`
	Model   string                  `json:"model"`
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input string `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type  string              `json:"type"`
	Delta anthropicStreamDelta `json:"delta,omitempty"`
}

type anthropicStreamDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
