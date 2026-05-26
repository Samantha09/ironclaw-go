package tools

import (
	"context"

	"github.com/nearai/ironclaw-go/internal/gate"
)

// JobContext — execution context for a tool invocation.
type JobContext struct {
	UserID   string
	JobID    string
	ThreadID string
}

// ToolOutput — result of a tool invocation.
type ToolOutput struct {
	Content  string
	Metadata map[string]any
	Duration int64 // milliseconds
}

// Tool — the unit of capability in IronClaw.
type Tool interface {
	Name() string
	Description() string
	ParameterSchema() map[string]any
	Execute(ctx context.Context, params map[string]any, jobCtx *JobContext) (ToolOutput, error)
	// RequiresApproval 返回此工具调用的审批需求级别。
	RequiresApproval(params map[string]any) gate.ApprovalRequirement
}
