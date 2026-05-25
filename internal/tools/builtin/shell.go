package builtin

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/nearai/ironclaw-go/internal/tools"
)

// ShellTool 执行 shell 命令。
type ShellTool struct {
	allowedCommands []string // 白名单，为空时允许所有
	maxOutputLen    int
}

func NewShellTool() *ShellTool {
	return &ShellTool{
		maxOutputLen: 10000,
	}
}

func (s *ShellTool) Name() string        { return "shell" }
func (s *ShellTool) Description() string { return "Executes a shell command and returns the output." }
func (s *ShellTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "The shell command to execute"},
			"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 30)"},
		},
		"required": []string{"command"},
	}
}

func (s *ShellTool) Execute(ctx context.Context, params map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	cmdStr, ok := params["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return tools.ToolOutput{}, fmt.Errorf("parameter 'command' must be a non-empty string")
	}

	timeoutSec := 30
	if t, ok := params["timeout"].(float64); ok {
		timeoutSec = int(t)
	}

	// 安全检查：禁止危险命令
	dangerous := []string{"rm -rf /", ":(){ :|:& };:", "> /dev/sda", "dd if=/dev/zero"}
	lower := strings.ToLower(cmdStr)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return tools.ToolOutput{}, fmt.Errorf("dangerous command blocked: %s", d)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()

	content := string(out)
	if err != nil {
		content = fmt.Sprintf("Error: %v\nOutput:\n%s", err, content)
	}
	if len(content) > s.maxOutputLen {
		content = content[:s.maxOutputLen] + "\n... (truncated)"
	}

	return tools.ToolOutput{Content: content}, nil
}
