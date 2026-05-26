package llm

import (
	"testing"
)

func TestGeminiBuildRequestBody(t *testing.T) {
	c := NewGeminiClient("gemini-2.0-flash", "key", "")

	msgs := []Message{
		{Role: RoleSystem, Content: "You are helpful."},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
	}

	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionSchema{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}

	req := c.buildRequestBody(msgs, tools)
	if len(req.Contents) != 3 {
		t.Errorf("contents = %d, want 3", len(req.Contents))
	}
	if len(req.Tools) != 1 {
		t.Errorf("tools = %d, want 1", len(req.Tools))
	}
	if len(req.Tools[0].FunctionDeclarations) != 1 {
		t.Errorf("decls = %d, want 1", len(req.Tools[0].FunctionDeclarations))
	}

	// system role 降级为 user
	if req.Contents[0].Role != "user" {
		t.Errorf("system role converted = %q, want user", req.Contents[0].Role)
	}
	if req.Contents[2].Role != "model" {
		t.Errorf("assistant role = %q, want model", req.Contents[2].Role)
	}
}

func TestGeminiParseResponse(t *testing.T) {
	c := NewGeminiClient("gemini-2.0-flash", "key", "")

	resp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "The weather is sunny."},
					},
				},
			},
		},
		UsageMetadata: geminiUsage{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}

	result := c.parseResponse(resp)
	if result.Content != "The weather is sunny." {
		t.Errorf("content = %q, want 'The weather is sunny.'", result.Content)
	}
	if result.Usage.PromptTokens != 10 {
		t.Errorf("prompt tokens = %d, want 10", result.Usage.PromptTokens)
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("total tokens = %d, want 15", result.Usage.TotalTokens)
	}
	if result.Model != "gemini-2.0-flash" {
		t.Errorf("model = %q, want gemini-2.0-flash", result.Model)
	}
}

func TestGeminiParseResponseWithToolCall(t *testing.T) {
	c := NewGeminiClient("gemini-2.0-flash", "key", "")

	resp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{
							FunctionCall: &geminiFunctionCall{
								Name: "get_weather",
								Args: map[string]interface{}{"city": "Beijing"},
							},
						},
					},
				},
			},
		},
	}

	result := c.parseResponse(resp)
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", result.ToolCalls[0].Function.Name)
	}
	if result.ToolCalls[0].Function.Arguments != `{"city":"Beijing"}` {
		t.Errorf("tool args = %q, want '{\"city\":\"Beijing\"}'", result.ToolCalls[0].Function.Arguments)
	}
}

func TestGeminiParseResponseEmpty(t *testing.T) {
	c := NewGeminiClient("gemini-2.0-flash", "key", "")
	result := c.parseResponse(geminiResponse{})
	if result.Content != "" {
		t.Errorf("expected empty content, got %q", result.Content)
	}
}
