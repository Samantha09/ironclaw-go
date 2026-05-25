package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient 调用本地 Ollama API。
type OllamaClient struct {
	model   string
	baseURL string
	client  *http.Client
}

// NewOllamaClient 创建 Ollama 客户端。
func NewOllamaClient(model, baseURL string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaClient{
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *OllamaClient) ModelName() string { return c.model }

func (c *OllamaClient) Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResponse, error) {
	reqBody := map[string]any{
		"model":    c.model,
		"messages": c.convertMessages(messages),
		"stream":   false,
	}
	if len(tools) > 0 {
		reqBody["tools"] = c.convertTools(tools)
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return CompletionResponse{
		Content:   result.Message.Content,
		ToolCalls: c.convertToolCalls(result.Message.ToolCalls),
		Usage: Usage{
			PromptTokens:     result.PromptEvalCount,
			CompletionTokens: result.EvalCount,
			TotalTokens:      result.PromptEvalCount + result.EvalCount,
		},
		Model: result.Model,
	}, nil
}

func (c *OllamaClient) StreamComplete(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	reqBody := map[string]any{
		"model":    c.model,
		"messages": c.convertMessages(messages),
		"stream":   true,
	}
	if len(tools) > 0 {
		reqBody["tools"] = c.convertTools(tools)
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Error: ctx.Err()}
				return
			default:
			}

			var chunk ollamaResponse
			if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
				continue
			}

			ch <- StreamChunk{
				Content: chunk.Message.Content,
				Done:    chunk.Done,
			}
		}
	}()

	return ch, nil
}

func (c *OllamaClient) convertMessages(msgs []Message) []map[string]any {
	result := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		result[i] = map[string]any{
			"role":    string(m.Role),
			"content": m.Content,
		}
	}
	return result
}

func (c *OllamaClient) convertTools(tools []ToolDefinition) []map[string]any {
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  t.Function.Parameters,
			},
		}
	}
	return result
}

func (c *OllamaClient) convertToolCalls(calls []ollamaToolCall) []ToolCall {
	if calls == nil {
		return nil
	}
	result := make([]ToolCall, len(calls))
	for i, call := range calls {
		result[i] = ToolCall{
			Function: FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		}
	}
	return result
}

// --- Ollama API 结构 ---

type ollamaResponse struct {
	Model            string           `json:"model"`
	Message          ollamaMessage    `json:"message"`
	Done             bool             `json:"done"`
	PromptEvalCount  int              `json:"prompt_eval_count"`
	EvalCount        int              `json:"eval_count"`
}

type ollamaMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
