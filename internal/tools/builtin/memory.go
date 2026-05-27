package builtin

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// MemoryTool 提供简单的键值内存存储。
type MemoryTool struct {
	mu   sync.RWMutex
	data map[string]map[string]string // namespace -> key -> value
}

func NewMemoryTool() *MemoryTool {
	return &MemoryTool{
		data: make(map[string]map[string]string),
	}
}

func (m *MemoryTool) Name() string        { return "memory" }
func (m *MemoryTool) Description() string { return "Stores and retrieves key-value data in a workspace namespace." }
func (m *MemoryTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":    map[string]any{"type": "string", "enum": []string{"get", "set", "delete", "list"}, "description": "Memory operation"},
			"namespace": map[string]any{"type": "string", "description": "Data namespace (default: 'default')"},
			"key":       map[string]any{"type": "string", "description": "Key to operate on"},
			"value":     map[string]any{"type": "string", "description": "Value to store (for set action)"},
		},
		"required": []string{"action"},
	}
}

func (m *MemoryTool) Execute(_ context.Context, params map[string]any, jobCtx *tools.JobContext) (tools.ToolOutput, error) {
	action, _ := params["action"].(string)
	namespace, _ := params["namespace"].(string)
	if namespace == "" {
		namespace = "default"
	}

	// 使用 userID 作为命名空间前缀，实现隔离
	if jobCtx != nil && jobCtx.UserID != "" {
		namespace = jobCtx.UserID + "/" + namespace
	}

	switch action {
	case "get":
		key, _ := params["key"].(string)
		if key == "" {
			return tools.ToolOutput{}, fmt.Errorf("parameter 'key' is required for get")
		}
		m.mu.RLock()
		ns, ok := m.data[namespace]
		val := ""
		if ok {
			val = ns[key]
		}
		m.mu.RUnlock()
		if !ok || val == "" {
			return tools.ToolOutput{Content: fmt.Sprintf("Key '%s' not found in namespace '%s'", key, namespace)}, nil
		}
		return tools.ToolOutput{Content: val}, nil

	case "set":
		key, _ := params["key"].(string)
		value, _ := params["value"].(string)
		if key == "" {
			return tools.ToolOutput{}, fmt.Errorf("parameter 'key' is required for set")
		}
		m.mu.Lock()
		if m.data[namespace] == nil {
			m.data[namespace] = make(map[string]string)
		}
		m.data[namespace][key] = value
		m.mu.Unlock()
		return tools.ToolOutput{Content: fmt.Sprintf("Set '%s' = '%s' in '%s'", key, value, namespace)}, nil

	case "delete":
		key, _ := params["key"].(string)
		if key == "" {
			return tools.ToolOutput{}, fmt.Errorf("parameter 'key' is required for delete")
		}
		m.mu.Lock()
		if ns, ok := m.data[namespace]; ok {
			delete(ns, key)
		}
		m.mu.Unlock()
		return tools.ToolOutput{Content: fmt.Sprintf("Deleted '%s' from '%s'", key, namespace)}, nil

	case "list":
		m.mu.RLock()
		ns, ok := m.data[namespace]
		m.mu.RUnlock()
		if !ok || len(ns) == 0 {
			return tools.ToolOutput{Content: fmt.Sprintf("Namespace '%s' is empty", namespace)}, nil
		}
		var lines []string
		for k, v := range ns {
			lines = append(lines, fmt.Sprintf("%s = %s", k, v))
		}
		return tools.ToolOutput{Content: strings.Join(lines, "\n")}, nil

	default:
		return tools.ToolOutput{}, fmt.Errorf("unknown action: %q", action)
	}
}

func (m *MemoryTool) RequiresApproval(params map[string]any) gate.ApprovalRequirement {
	if params == nil {
		return gate.UnlessAutoApproved
	}
	action, _ := params["action"].(string)
	if action == "" {
		return gate.UnlessAutoApproved
	}
	if action == "set" || action == "delete" {
		return gate.UnlessAutoApproved
	}
	return gate.Never
}
