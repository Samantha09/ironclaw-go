package builtin

import (
	"context"
	"time"

	"github.com/nearai/ironclaw-go/internal/tools"
)

type TimeTool struct{}

func NewTimeTool() *TimeTool { return &TimeTool{} }

func (t *TimeTool) Name() string        { return "time" }
func (t *TimeTool) Description() string { return "Returns the current UTC time." }
func (t *TimeTool) ParameterSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *TimeTool) Execute(_ context.Context, _ map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	return tools.ToolOutput{Content: time.Now().UTC().Format(time.RFC3339)}, nil
}
