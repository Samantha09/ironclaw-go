package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/workspace"
)

// FileTool 执行文件读写操作。
type FileTool struct {
	ws workspace.Workspace
}

// NewFileTool 创建新的文件工具，使用提供的 Workspace 进行用户隔离的文件操作。
func NewFileTool(ws workspace.Workspace) *FileTool {
	return &FileTool{ws: ws}
}

func (f *FileTool) Name() string        { return "file" }
func (f *FileTool) Description() string { return "Reads or writes files in the workspace." }
func (f *FileTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"read", "write", "list", "delete", "mkdir"}, "description": "File operation"},
			"path":    map[string]any{"type": "string", "description": "File path (relative)"},
			"content": map[string]any{"type": "string", "description": "Content to write (for write action)"},
		},
		"required": []string{"action", "path"},
	}
}

func (f *FileTool) Execute(ctx context.Context, params map[string]any, jobCtx *tools.JobContext) (tools.ToolOutput, error) {
	action, _ := params["action"].(string)
	path, _ := params["path"].(string)

	if path == "" {
		return tools.ToolOutput{}, fmt.Errorf("parameter 'path' is required")
	}

	// 路径安全检查已在 Workspace 层实现，此处保留基本校验
	if strings.Contains(path, "..") {
		return tools.ToolOutput{}, fmt.Errorf("invalid path: must not contain '..'")
	}

	switch action {
	case "read":
		data, err := f.ws.ReadFile(ctx, jobCtx.UserID, path)
		if err != nil {
			return tools.ToolOutput{}, fmt.Errorf("read file: %w", err)
		}
		return tools.ToolOutput{Content: string(data)}, nil

	case "write":
		content, _ := params["content"].(string)
		if err := f.ws.WriteFile(ctx, jobCtx.UserID, path, []byte(content)); err != nil {
			return tools.ToolOutput{}, fmt.Errorf("write file: %w", err)
		}
		return tools.ToolOutput{Content: fmt.Sprintf("Wrote %d bytes to %s", len(content), path)}, nil

	case "list":
		entries, err := f.ws.ListDir(ctx, jobCtx.UserID, path)
		if err != nil {
			return tools.ToolOutput{}, fmt.Errorf("list dir: %w", err)
		}
		var lines []string
		for _, e := range entries {
			marker := "📄"
			if e.IsDir {
				marker = "📁"
			}
			lines = append(lines, fmt.Sprintf("%s %s", marker, e.Name))
		}
		return tools.ToolOutput{Content: strings.Join(lines, "\n")}, nil

	case "delete":
		if err := f.ws.DeleteFile(ctx, jobCtx.UserID, path); err != nil {
			return tools.ToolOutput{}, fmt.Errorf("delete file: %w", err)
		}
		return tools.ToolOutput{Content: fmt.Sprintf("Deleted %s", path)}, nil

	case "mkdir":
		if err := f.ws.Mkdir(ctx, jobCtx.UserID, path); err != nil {
			return tools.ToolOutput{}, fmt.Errorf("mkdir: %w", err)
		}
		return tools.ToolOutput{Content: fmt.Sprintf("Created directory %s", path)}, nil

	default:
		return tools.ToolOutput{}, fmt.Errorf("unknown action: %q", action)
	}
}
