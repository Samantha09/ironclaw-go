package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nearai/ironclaw-go/internal/tools"
)

type JSONTool struct{}

func NewJSONTool() *JSONTool { return &JSONTool{} }

func (j *JSONTool) Name() string        { return "json" }
func (j *JSONTool) Description() string { return "Minify or prettify JSON." }
func (j *JSONTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
			"mode":  map[string]any{"type": "string", "enum": []string{"minify", "prettify"}},
		},
		"required": []string{"input", "mode"},
	}
}
func (j *JSONTool) Execute(_ context.Context, params map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	input, _ := params["input"].(string)
	mode, _ := params["mode"].(string)

	var raw any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return tools.ToolOutput{}, fmt.Errorf("invalid JSON: %w", err)
	}

	var out []byte
	var err error
	if strings.ToLower(mode) == "prettify" {
		out, err = json.MarshalIndent(raw, "", "  ")
	} else {
		out, err = json.Marshal(raw)
	}
	if err != nil {
		return tools.ToolOutput{}, err
	}
	return tools.ToolOutput{Content: string(out)}, nil
}
