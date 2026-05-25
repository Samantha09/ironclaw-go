package tools

import "context"

// JobContext — execution context for a tool invocation.
type JobContext struct {
	UserID string
	JobID  string
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
}
