package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nearai/ironclaw-go/internal/tools"
)

// FileTool 执行文件读写操作。
type FileTool struct {
	allowedDirs []string // 允许访问的目录，为空时限制在当前目录
	maxSize     int64
}

func NewFileTool() *FileTool {
	return &FileTool{
		allowedDirs: []string{"."},
		maxSize:     10 * 1024 * 1024, // 10MB
	}
}

func (f *FileTool) Name() string        { return "file" }
func (f *FileTool) Description() string { return "Reads or writes files in the workspace." }
func (f *FileTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"read", "write", "list", "delete"}, "description": "File operation"},
			"path":    map[string]any{"type": "string", "description": "File path (relative)"},
			"content": map[string]any{"type": "string", "description": "Content to write (for write action)"},
		},
		"required": []string{"action", "path"},
	}
}

func (f *FileTool) Execute(_ context.Context, params map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	action, _ := params["action"].(string)
	path, _ := params["path"].(string)

	if path == "" {
		return tools.ToolOutput{}, fmt.Errorf("parameter 'path' is required")
	}

	// 路径安全检查：禁止绝对路径和路径遍历
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return tools.ToolOutput{}, fmt.Errorf("invalid path: must be relative and not contain '..'")
	}

	fullPath := filepath.Clean(path)

	switch action {
	case "read":
		info, err := os.Stat(fullPath)
		if err != nil {
			return tools.ToolOutput{}, fmt.Errorf("stat file: %w", err)
		}
		if info.Size() > f.maxSize {
			return tools.ToolOutput{}, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), f.maxSize)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return tools.ToolOutput{}, fmt.Errorf("read file: %w", err)
		}
		return tools.ToolOutput{Content: string(data)}, nil

	case "write":
		content, _ := params["content"].(string)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return tools.ToolOutput{}, fmt.Errorf("write file: %w", err)
		}
		return tools.ToolOutput{Content: fmt.Sprintf("Wrote %d bytes to %s", len(content), fullPath)}, nil

	case "list":
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return tools.ToolOutput{}, fmt.Errorf("list dir: %w", err)
		}
		var lines []string
		for _, e := range entries {
			marker := "📄"
			if e.IsDir() {
				marker = "📁"
			}
			lines = append(lines, fmt.Sprintf("%s %s", marker, e.Name()))
		}
		return tools.ToolOutput{Content: strings.Join(lines, "\n")}, nil

	case "delete":
		if err := os.Remove(fullPath); err != nil {
			return tools.ToolOutput{}, fmt.Errorf("delete file: %w", err)
		}
		return tools.ToolOutput{Content: fmt.Sprintf("Deleted %s", fullPath)}, nil

	default:
		return tools.ToolOutput{}, fmt.Errorf("unknown action: %q", action)
	}
}
