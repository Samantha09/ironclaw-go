package llm

import (
	"context"
)

// MessageRole 定义消息角色。
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message 表示 LLM 对话中的一条消息。
type Message struct {
	Role      MessageRole
	Content   string
	ToolCalls []ToolCall
	ToolName  string // 用于 tool 角色的消息
	ToolID    string // 用于 tool 角色的消息
}

// ToolCall 表示 LLM 请求中的工具调用。
type ToolCall struct {
	ID       string
	Type     string
	Function FunctionCall
}

// FunctionCall 表示函数调用详情。
type FunctionCall struct {
	Name      string
	Arguments string // JSON
}

// ToolDefinition 定义一个可供 LLM 使用的工具。
type ToolDefinition struct {
	Type     string          `json:"type"`
	Function FunctionSchema  `json:"function"`
}

// FunctionSchema 定义函数的参数模式。
type FunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Usage 表示 LLM 调用的 token 消耗。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CompletionResponse 表示 LLM 完成响应。
type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
	Model     string
}

// LlmProvider 是 LLM 提供商的统一接口。
type LlmProvider interface {
	// Complete 发送非流式完成请求。
	Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResponse, error)

	// StreamComplete 发送流式完成请求。
	StreamComplete(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error)

	// ModelName 返回当前使用的模型名称。
	ModelName() string
}

// StreamChunk 表示流式响应的一个片段。
type StreamChunk struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
	Error     error
}

// New 根据提供商名称创建 LLM 客户端。
func New(provider, model, apiKey, baseURL string) (LlmProvider, error) {
	switch provider {
	case "openai", "groq":
		return NewOpenAIClient(model, apiKey, baseURL), nil
	case "anthropic":
		return NewAnthropicClient(model, apiKey, baseURL), nil
	case "ollama":
		return NewOllamaClient(model, baseURL), nil
	default:
		return nil, nil
	}
}
