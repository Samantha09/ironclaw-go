package agent

import (
	"strings"
)

// IntentType 表示用户输入的意图类型。
type IntentType string

const (
	IntentChat       IntentType = "chat"        // 普通对话
	IntentToolCall   IntentType = "tool_call"   // 直接工具调用
	IntentSystemCmd  IntentType = "system_cmd"  // 系统命令（/threads, /switch 等）
	IntentLLMQuery   IntentType = "llm_query"   // 需要 LLM 推理的查询
)

// Intent 表示解析后的用户意图。
type Intent struct {
	Type       IntentType
	ToolName   string
	ToolParams string
	Content    string
	Command    string
	Args       []string
}

// Router 解析用户输入并确定处理路径。
type Router struct{}

func NewRouter() *Router {
	return &Router{}
}

// Route 分析用户输入并返回意图。
func (r *Router) Route(content string) Intent {
	content = strings.TrimSpace(content)

	// 系统命令：以 / 开头
	if strings.HasPrefix(content, "/") {
		parts := strings.Fields(content)
		cmd := strings.TrimPrefix(parts[0], "/")
		args := []string{}
		if len(parts) > 1 {
			args = parts[1:]
		}
		return Intent{
			Type:    IntentSystemCmd,
			Command: cmd,
			Args:    args,
			Content: content,
		}
	}

	// 工具调用：tool:NAME PARAMS
	if strings.HasPrefix(content, "tool:") {
		rest := strings.TrimPrefix(content, "tool:")
		parts := strings.SplitN(rest, " ", 2)
		intent := Intent{
			Type:     IntentToolCall,
			ToolName: strings.TrimSpace(parts[0]),
			Content:  content,
		}
		if len(parts) == 2 {
			intent.ToolParams = strings.TrimSpace(parts[1])
		}
		return intent
	}

	// LLM 查询：以 ? 开头或包含复杂推理需求的输入
	if strings.HasPrefix(content, "?") {
		return Intent{
			Type:    IntentLLMQuery,
			Content: strings.TrimPrefix(content, "?"),
		}
	}

	// 默认：普通对话（尝试通过 LLM 处理，MVP 回退到 echo）
	return Intent{
		Type:    IntentChat,
		Content: content,
	}
}
