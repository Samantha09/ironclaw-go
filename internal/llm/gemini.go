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

// GeminiClient 调用 Google Gemini API。
type GeminiClient struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewGeminiClient 创建 Gemini 客户端。
func NewGeminiClient(model, apiKey, baseURL string) *GeminiClient {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *GeminiClient) ModelName() string { return c.model }

func (c *GeminiClient) Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResponse, error) {
	reqBody := c.buildRequestBody(messages, tools)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	return c.parseResponse(geminiResp), nil
}

func (c *GeminiClient) StreamComplete(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	// MVP：先降级为非流式，然后一次性输出
	resp, err := c.Complete(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Content: resp.Content, ToolCalls: resp.ToolCalls, Done: true}
	close(ch)
	return ch, nil
}

// --- Gemini API types ---

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
	Tools    []geminiTool    `json:"tools,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string              `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (c *GeminiClient) buildRequestBody(messages []Message, tools []ToolDefinition) geminiRequest {
	req := geminiRequest{}

	for _, m := range messages {
		role := "user"
		switch m.Role {
		case RoleSystem:
			// Gemini 不支持 system 角色，降级为 user
			role = "user"
		case RoleUser:
			role = "user"
		case RoleAssistant:
			role = "model"
		case RoleTool:
			// Gemini 的 tool 结果作为 user 消息中的 functionResponse 部分
			// MVP：将 tool 结果作为 user 文本消息处理
			role = "user"
		}

		content := geminiContent{Role: role, Parts: []geminiPart{{Text: m.Content}}}
		req.Contents = append(req.Contents, content)
	}

	if len(tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(tools))
		for _, t := range tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		req.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	return req
}

func (c *GeminiClient) parseResponse(resp geminiResponse) CompletionResponse {
	if len(resp.Candidates) == 0 {
		return CompletionResponse{}
	}

	candidate := resp.Candidates[0]
	var content string
	var toolCalls []ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			content += part.Text
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, ToolCall{
				ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
				Type: "function",
				Function: FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}

	return CompletionResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		},
		Model: c.model,
	}
}
