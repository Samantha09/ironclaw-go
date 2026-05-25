package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// Agent 是核心推理与行动循环。
type Agent struct {
	config         Config
	deps           Deps
	router         *Router
	sessionManager *SessionManager
}

// New 创建新的 Agent 实例。
func New(config Config, deps Deps) *Agent {
	return &Agent{
		config:         config,
		deps:           deps,
		router:         NewRouter(),
		sessionManager: NewSessionManager(),
	}
}

// ProcessMessage 处理单个用户输入。
func (a *Agent) ProcessMessage(ctx context.Context, msg channels.IncomingMessage) (channels.OutgoingResponse, error) {
	intent := a.router.Route(msg.Content)

	switch intent.Type {
	case IntentSystemCmd:
		return a.handleSystemCommand(ctx, msg.UserID, intent)
	case IntentToolCall:
		return a.handleToolInvocation(ctx, msg.UserID, intent)
	case IntentLLMQuery:
		return a.handleLLMQuery(ctx, msg.UserID, intent)
	case IntentChat:
		return a.handleChat(ctx, msg.UserID, intent)
	default:
		return channels.OutgoingResponse{Content: "无法识别的意图类型"}, nil
	}
}

// handleSystemCommand 处理系统命令。
func (a *Agent) handleSystemCommand(_ context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	switch intent.Command {
	case "threads", "t":
		threads := a.sessionManager.ListThreads(userID)
		if len(threads) == 0 {
			return channels.OutgoingResponse{Content: "当前没有对话线程。"}, nil
		}
		var sb strings.Builder
		sb.WriteString("对话线程列表:\n")
		for i, t := range threads {
			mark := " "
			if active := a.sessionManager.GetThread(userID, t.ID); active != nil {
				// 简化：假设列表中的第一个是活跃的
				if i == 0 {
					mark = "*"
				}
			}
			sb.WriteString(fmt.Sprintf("  %s [%s] %s (%d 轮)\n", mark, t.ID[:8], t.Title, len(t.Turns)))
		}
		return channels.OutgoingResponse{Content: sb.String()}, nil

	case "switch", "s":
		if len(intent.Args) < 1 {
			return channels.OutgoingResponse{Content: "用法: /switch <thread-id>"}, nil
		}
		threadID := intent.Args[0]
		if ok := a.sessionManager.SwitchThread(userID, threadID); ok {
			return channels.OutgoingResponse{Content: fmt.Sprintf("已切换到线程 %s", threadID[:8])}, nil
		}
		return channels.OutgoingResponse{Content: fmt.Sprintf("未找到线程: %s", threadID)}, nil

	case "new", "n":
		thread := a.sessionManager.GetOrCreateThread(userID, "repl")
		return channels.OutgoingResponse{Content: fmt.Sprintf("已创建新线程: %s", thread.ID[:8])}, nil

	case "help", "h":
		return channels.OutgoingResponse{Content: `可用命令:
  /threads, /t    - 列出所有对话线程
  /switch, /s <id> - 切换到指定线程
  /new, /n        - 创建新线程
  /help, /h       - 显示此帮助

直接输入: 普通对话
  tool:<name> <params> - 直接调用工具
  ?<query>             - LLM 查询（需配置 LLM）`}, nil

	default:
		return channels.OutgoingResponse{Content: fmt.Sprintf("未知命令: /%s，输入 /help 查看帮助", intent.Command)}, nil
	}
}

// handleToolInvocation 处理工具调用。
func (a *Agent) handleToolInvocation(ctx context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	thread := a.sessionManager.GetOrCreateThread(userID, "repl")

	var params map[string]any
	if intent.ToolParams != "" {
		// 尝试解析 JSON，失败则当作字符串 message 传递
		if err := json.Unmarshal([]byte(intent.ToolParams), &params); err != nil {
			params = map[string]any{"message": intent.ToolParams}
		}
	}

	out, err := a.deps.Dispatcher.Dispatch(ctx, intent.ToolName, params, &tools.JobContext{
		UserID:   userID,
		ThreadID: thread.ID,
	})

	turn := Turn{
		UserMsg:   intent.Content,
		AgentResp: out.Content,
	}
	if err != nil {
		turn.AgentResp = fmt.Sprintf("错误: %v", err)
		out = tools.ToolOutput{Content: turn.AgentResp}
	}
	a.sessionManager.AddTurn(userID, turn)

	return channels.OutgoingResponse{Content: out.Content}, nil
}

// handleLLMQuery 处理 LLM 查询。
func (a *Agent) handleLLMQuery(ctx context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	// MVP: LLM 集成后续实现，当前回退到 echo
	_ = ctx
	_ = userID

	resp := fmt.Sprintf("[%s] LLM 查询（尚未集成 LLM 提供商）: %s", a.config.Name, intent.Content)

	turn := Turn{
		UserMsg:   intent.Content,
		AgentResp: resp,
	}
	a.sessionManager.AddTurn(userID, turn)

	return channels.OutgoingResponse{Content: resp}, nil
}

// handleChat 处理普通对话。
func (a *Agent) handleChat(ctx context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	_ = ctx
	thread := a.sessionManager.GetOrCreateThread(userID, "repl")

	// MVP: 无 LLM 时回退到 echo
	resp := fmt.Sprintf("[%s] 你说: %s", a.config.Name, intent.Content)

	turn := Turn{
		UserMsg:   intent.Content,
		AgentResp: resp,
	}
	a.sessionManager.AddTurn(userID, turn)

	// 上下文压缩：当轮次过多时自动压缩
	const maxTurns = 50
	a.sessionManager.CompactThread(userID, maxTurns)

	// 持久化到数据库（如果配置了）
	if a.deps.Database != nil {
		_ = a.persistThread(ctx, thread)
	}

	return channels.OutgoingResponse{Content: resp}, nil
}

// persistThread 将线程持久化到数据库。
func (a *Agent) persistThread(ctx context.Context, thread *Thread) error {
	conv := &db.Conversation{
		ID:        thread.ID,
		UserID:    thread.UserID,
		Channel:   thread.Channel,
		Title:     thread.Title,
		CreatedAt: thread.CreatedAt,
		UpdatedAt: thread.UpdatedAt,
	}
	if err := a.deps.Database.SaveConversation(ctx, conv); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}

// Run 阻塞运行，从通道管理器消费消息。
func (a *Agent) Run(ctx context.Context, mgr *channels.Manager) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := mgr.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		resp, err := a.ProcessMessage(ctx, msg)
		if err != nil {
			resp = channels.OutgoingResponse{Content: fmt.Sprintf("Agent 错误: %v", err)}
		}

		_ = mgr.Broadcast(ctx, resp)
	}
}
