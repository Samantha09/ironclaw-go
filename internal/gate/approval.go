package gate

import (
	"context"
	"fmt"
)

// ApprovalGate 检查工具是否需要审批，并根据执行模式返回 Allow、Deny 或 Pause。
type ApprovalGate struct {
	requirementFn func(toolName string) ApprovalRequirement
}

// NewApprovalGate 创建新的审批门控。
func NewApprovalGate(requirementFn func(toolName string) ApprovalRequirement) *ApprovalGate {
	return &ApprovalGate{requirementFn: requirementFn}
}

// Name 返回门控名称。
func (a *ApprovalGate) Name() string {
	return "approval"
}

// Evaluate 评估工具调用是否允许执行。
func (a *ApprovalGate) Evaluate(_ context.Context, ctx *GateContext) GateDecision {
	req := a.requirementFn(ctx.ToolName)

	isAutoApproved := ctx.AutoApproved[ctx.ToolName]

	switch ctx.ExecutionMode {
	case Autonomous:
		// 自主模式：无法交互审批，Always 和 UnlessAutoApproved（未自动审批）都拒绝
		switch req {
		case Never:
			return Allow
		case UnlessAutoApproved:
			if isAutoApproved {
				return Allow
			}
			return Deny
		case Always:
			return Deny
		}

	case Interactive:
		// 交互模式：可暂停等待用户确认
		switch req {
		case Never:
			return Allow
		case UnlessAutoApproved:
			if isAutoApproved {
				return Allow
			}
			return Pause
		case Always:
			return Pause
		}
	}

	return Deny
}

// DefaultToolRequirement 返回默认的工具审批需求。
// shell、file:write/delete、http:POST/PUT/DELETE/PATCH 需要审批。
func DefaultToolRequirement(toolName string, params map[string]any) ApprovalRequirement {
	switch toolName {
	case "shell":
		return UnlessAutoApproved
	case "file":
		action, _ := params["action"].(string)
		if action == "write" || action == "delete" || action == "mkdir" {
			return UnlessAutoApproved
		}
		return Never
	case "http":
		method, _ := params["method"].(string)
		if method != "GET" && method != "HEAD" {
			return UnlessAutoApproved
		}
		return Never
	case "memory":
		action, _ := params["action"].(string)
		if action == "set" || action == "delete" {
			return UnlessAutoApproved
		}
		return Never
	case "echo", "time", "json":
		return Never
	default:
		return UnlessAutoApproved
	}
}

// DescribePending 生成待审批门控的人类可读描述。
func DescribePending(toolName string, params map[string]any) string {
	switch toolName {
	case "shell":
		cmd, _ := params["command"].(string)
		return fmt.Sprintf("执行 shell 命令: %s", cmd)
	case "file":
		action, _ := params["action"].(string)
		path, _ := params["path"].(string)
		return fmt.Sprintf("文件操作 [%s]: %s", action, path)
	case "http":
		method, _ := params["method"].(string)
		url, _ := params["url"].(string)
		return fmt.Sprintf("HTTP 请求 [%s]: %s", method, url)
	default:
		return fmt.Sprintf("调用工具: %s", toolName)
	}
}
