package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// HTTPTool 执行 HTTP 请求。
type HTTPTool struct {
	client      *http.Client
	maxBodySize int64
}

func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxBodySize: 5 * 1024 * 1024, // 5MB
	}
}

func (h *HTTPTool) Name() string        { return "http" }
func (h *HTTPTool) Description() string { return "Makes HTTP requests to external APIs." }
func (h *HTTPTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH"}, "description": "HTTP method"},
			"url":     map[string]any{"type": "string", "description": "Request URL"},
			"headers": map[string]any{"type": "object", "description": "Optional headers as key-value pairs"},
			"body":    map[string]any{"type": "string", "description": "Request body (for POST/PUT/PATCH)"},
		},
		"required": []string{"method", "url"},
	}
}

func (h *HTTPTool) Execute(ctx context.Context, params map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	method, _ := params["method"].(string)
	rawURL, _ := params["url"].(string)
	bodyStr, _ := params["body"].(string)

	if rawURL == "" {
		return tools.ToolOutput{}, fmt.Errorf("parameter 'url' is required")
	}

	// URL 验证
	u, err := url.Parse(rawURL)
	if err != nil {
		return tools.ToolOutput{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return tools.ToolOutput{}, fmt.Errorf("only http/https URLs are allowed")
	}

	var body io.Reader
	if bodyStr != "" {
		body = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return tools.ToolOutput{}, fmt.Errorf("create request: %w", err)
	}

	// 设置 headers
	if headers, ok := params["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return tools.ToolOutput{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取大小
	var reader io.Reader = resp.Body
	if resp.ContentLength > h.maxBodySize {
		reader = io.LimitReader(resp.Body, h.maxBodySize)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return tools.ToolOutput{}, fmt.Errorf("read response: %w", err)
	}

	content := fmt.Sprintf("Status: %s\n\n%s", resp.Status, string(data))
	return tools.ToolOutput{Content: content}, nil
}

func (h *HTTPTool) RequiresApproval(params map[string]any) gate.ApprovalRequirement {
	if params == nil {
		return gate.UnlessAutoApproved
	}
	method, _ := params["method"].(string)
	if method == "" {
		return gate.UnlessAutoApproved
	}
	if method != "GET" && method != "HEAD" {
		return gate.UnlessAutoApproved
	}
	return gate.Never
}
