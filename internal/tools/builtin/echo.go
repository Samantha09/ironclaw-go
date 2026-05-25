package builtin

import (
	"context"
	"fmt"

	"github.com/nearai/ironclaw-go/internal/tools"
)

type EchoTool struct{}

func NewEchoTool() *EchoTool { return &EchoTool{} }

func (e *EchoTool) Name() string        { return "echo" }
func (e *EchoTool) Description() string { return "Echoes back the input message." }
func (e *EchoTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "description": "The message to echo"},
		},
		"required": []string{"message"},
	}
}
func (e *EchoTool) Execute(_ context.Context, params map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	msg, ok := params["message"].(string)
	if !ok {
		return tools.ToolOutput{}, fmt.Errorf("parameter 'message' must be a string")
	}
	return tools.ToolOutput{Content: msg}, nil
}
