package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/nearai/ironclaw-go/internal/safety"
)

// Dispatcher — runs the safety pipeline and executes tools.
type Dispatcher struct {
	registry *Registry
	safety   *safety.Layer
}

func NewDispatcher(registry *Registry, safety *safety.Layer) *Dispatcher {
	return &Dispatcher{registry: registry, safety: safety}
}

// Dispatch runs a tool by name with safety checks.
func (d *Dispatcher) Dispatch(ctx context.Context, toolName string, params map[string]any, jobCtx *JobContext) (ToolOutput, error) {
	tool, ok := d.registry.Get(toolName)
	if !ok {
		return ToolOutput{}, fmt.Errorf("tool '%s' not found", toolName)
	}

	start := time.Now()
	out, err := tool.Execute(ctx, params, jobCtx)
	out.Duration = time.Since(start).Milliseconds()

	if err != nil {
		return ToolOutput{}, err
	}

	sanitized, err := d.safety.SanitizeToolOutput(ctx, out.Content)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("safety check failed: %w", err)
	}
	out.Content = sanitized

	return out, nil
}
