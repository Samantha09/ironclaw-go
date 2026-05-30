package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/hooks"
	"github.com/nearai/ironclaw-go/internal/llm"
	"github.com/nearai/ironclaw-go/internal/skills"
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

func (a *Agent) publishEvent(userID, threadID string, evType channels.EventType, payload string, meta map[string]any) {
	if a.deps.EventPublisher == nil {
		return
	}
	a.deps.EventPublisher(channels.Event{
		Type:     evType,
		UserID:   userID,
		ThreadID: threadID,
		Payload:  payload,
		Meta:     meta,
	})
}

// ProcessMessage 处理单个用户输入。
func (a *Agent) ProcessMessage(ctx context.Context, msg channels.IncomingMessage) (channels.OutgoingResponse, error) {
	// 文档提取：处理消息中的文档附件
	if a.deps.DocumentMiddleware != nil {
		a.deps.DocumentMiddleware.Process(&msg)
		msg.Content = a.appendExtractedText(msg)
	}

	if a.deps.Hooks != nil {
		if err := a.deps.Hooks.Trigger(ctx, hooks.Event{
			Type:    hooks.EventBeforeMessage,
			UserID:  msg.UserID,
			Channel: msg.Channel,
			Data:    map[string]any{"content": msg.Content},
		}); err != nil {
			return channels.OutgoingResponse{}, fmt.Errorf("before:message hook rejected: %w", err)
		}
	}

	intent := a.router.Route(msg.Content)

	var resp channels.OutgoingResponse
	var err error

	switch intent.Type {
	case IntentSystemCmd:
		resp, err = a.handleSystemCommand(ctx, msg.UserID, intent)
	case IntentToolCall:
		resp, err = a.handleToolInvocation(ctx, msg.UserID, intent)
	case IntentLLMQuery:
		resp, err = a.handleLLMQuery(ctx, msg.UserID, intent)
	case IntentChat:
		resp, err = a.handleChat(ctx, msg.UserID, intent)
	default:
		resp = channels.OutgoingResponse{Content: "无法识别的意图类型"}
	}

	if err != nil {
		resp = channels.OutgoingResponse{Content: fmt.Sprintf("Agent 错误: %v", err)}
	}

	if a.deps.Hooks != nil {
		_ = a.deps.Hooks.Trigger(ctx, hooks.Event{
			Type:    hooks.EventAfterMessage,
			UserID:  msg.UserID,
			Channel: msg.Channel,
			Data:    map[string]any{"content": msg.Content, "response": resp.Content},
		})

		_ = a.deps.Hooks.Trigger(ctx, hooks.Event{
			Type:    hooks.EventBeforeResponse,
			UserID:  msg.UserID,
			Channel: msg.Channel,
			Data:    map[string]any{"response": resp.Content},
		})
	}

	// SSE: 发布最终响应事件（LLM 工具链路径已在 finalizeToolResults 中发布）
	if resp.Status != "pending_gate" && a.deps.EventPublisher != nil {
		a.publishEvent(msg.UserID, msg.ThreadID, channels.EventAgentResponse, resp.Content, nil)
	}

	return resp, nil
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

直接输入: 普通对话（如有 LLM 配置则通过 LLM 处理）
  tool:<name> <params> - 直接调用工具
  ?<query>             - LLM 查询`}, nil

	default:
		return channels.OutgoingResponse{Content: fmt.Sprintf("未知命令: /%s，输入 /help 查看帮助", intent.Command)}, nil
	}
}

// handleToolInvocation 处理工具调用。
func (a *Agent) handleToolInvocation(ctx context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	thread := a.sessionManager.GetOrCreateThread(userID, "repl")

	var params map[string]any
	if intent.ToolParams != "" {
		if err := json.Unmarshal([]byte(intent.ToolParams), &params); err != nil {
			params = map[string]any{"message": intent.ToolParams}
		}
	}

	if a.deps.Hooks != nil {
		if err := a.deps.Hooks.Trigger(ctx, hooks.Event{
			Type:   hooks.EventBeforeToolCall,
			UserID: userID,
			Data:   map[string]any{"tool": intent.ToolName, "params": params},
		}); err != nil {
			return channels.OutgoingResponse{}, fmt.Errorf("before:tool_call hook rejected: %w", err)
		}
	}

	out, err := a.deps.Dispatcher.Dispatch(ctx, intent.ToolName, params, &tools.JobContext{
		UserID:   userID,
		ThreadID: thread.ID,
	})

	if a.deps.Hooks != nil {
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		_ = a.deps.Hooks.Trigger(ctx, hooks.Event{
			Type:   hooks.EventAfterToolCall,
			UserID: userID,
			Data: map[string]any{
				"tool":     intent.ToolName,
				"output":   out.Content,
				"error":    errStr,
				"duration": out.Duration,
			},
		})
	}

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
	if a.deps.LLM == nil {
		return channels.OutgoingResponse{Content: "LLM 未配置，请先设置 LLM 提供商。"}, nil
	}
	return a.runLLM(ctx, userID, intent.Content)
}

// handleChat 处理普通对话。
func (a *Agent) handleChat(ctx context.Context, userID string, intent Intent) (channels.OutgoingResponse, error) {
	if a.deps.LLM == nil {
		// 无 LLM 时回退到 echo
		return a.echoReply(userID, intent.Content), nil
	}
	return a.runLLM(ctx, userID, intent.Content)
}

// runLLM 执行 LLM 推理循环。
func (a *Agent) runLLM(ctx context.Context, userID string, content string) (channels.OutgoingResponse, error) {
	thread := a.sessionManager.GetOrCreateThread(userID, "repl")

	// 检查是否有 pending execution
	if pe := a.sessionManager.GetPendingExecution(userID, thread.ID); pe != nil {
		if content == "__resume__" {
			// 检查是否还有未解决的 gate
			if a.deps.PendingStore != nil && a.deps.PendingStore.HasPending(userID, thread.ID) {
				return channels.OutgoingResponse{Status: "pending_gate", ThreadID: thread.ID}, nil
			}
			// Gate 已清除，恢复执行
			return a.Resume(ctx, userID, thread.ID)
		}
		// 用户发送了新消息，丢弃旧的 pending execution 和已批准的 gate
		a.sessionManager.ClearPendingExecution(userID, thread.ID)
		if a.deps.PendingStore != nil {
			a.deps.PendingStore.ClearResolved(userID, thread.ID)
		}
	}

	// 构建 LLM 消息历史
	messages := a.buildLLMMessages(userID, content)

	// 将工具注册表转换为 LLM 工具定义
	toolDefs := a.buildLLMTools()

	// 调用 LLM
	resp, err := a.deps.LLM.Complete(ctx, messages, toolDefs)
	if err != nil {
		return channels.OutgoingResponse{Content: fmt.Sprintf("LLM 错误: %v", err)}, nil
	}

	// 如果 LLM 返回工具调用，执行它们
	if len(resp.ToolCalls) > 0 {
		return a.handleLLMToolCalls(ctx, userID, thread.ID, content, messages, resp)
	}

	// 普通文本回复
	turn := Turn{
		UserMsg:   content,
		AgentResp: resp.Content,
	}
	a.sessionManager.AddTurn(userID, turn)
	a.persistIfNeeded(ctx, thread)

	return channels.OutgoingResponse{Content: resp.Content}, nil
}

// handleLLMToolCalls 处理 LLM 返回的工具调用。
func (a *Agent) handleLLMToolCalls(ctx context.Context, userID, threadID, originalContent string, messages []llm.Message, resp llm.CompletionResponse) (channels.OutgoingResponse, error) {
	var toolResults []llm.Message

	for i, call := range resp.ToolCalls {
		var params map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
			params = map[string]any{"message": call.Function.Arguments}
		}

		a.publishEvent(userID, threadID, channels.EventToolCall,
			fmt.Sprintf("Executing tool: %s", call.Function.Name),
			map[string]any{"tool_name": call.Function.Name})

		out, err := a.deps.Dispatcher.Dispatch(ctx, call.Function.Name, params, &tools.JobContext{
			UserID:   userID,
			ThreadID: threadID,
		})

		status := "success"
		if err != nil {
			status = "error"
		}
		a.publishEvent(userID, threadID, channels.EventToolResult,
			fmt.Sprintf("Tool %s %s (%d ms)", call.Function.Name, status, out.Duration),
			map[string]any{"tool_name": call.Function.Name, "status": status, "duration_ms": out.Duration})

		if err != nil {
			if gate.IsPauseError(err) {
				a.publishEvent(userID, threadID, channels.EventGatePending,
					fmt.Sprintf("Waiting for approval: %s", call.Function.Name),
					map[string]any{"tool_name": call.Function.Name})
				pe := &PendingExecution{
					RequestID:       uuid.New().String(),
					UserID:          userID,
					ThreadID:        threadID,
					Messages:        append([]llm.Message(nil), messages...),
					ToolCalls:       resp.ToolCalls,
					NextIndex:       i,
					OriginalContent: originalContent,
					CreatedAt:       time.Now(),
					ExpiresAt:       time.Now().Add(10 * time.Minute),
				}
				a.sessionManager.SavePendingExecution(pe)
				return channels.OutgoingResponse{
					Status:   "pending_gate",
					ThreadID: threadID,
				}, nil
			}
			toolResults = append(toolResults, llm.Message{
				Role:     llm.RoleTool,
				Content:  fmt.Sprintf("错误: %v", err),
				ToolName: call.Function.Name,
				ToolID:   call.ID,
			})
			continue
		}

		toolResults = append(toolResults, llm.Message{
			Role:     llm.RoleTool,
			Content:  out.Content,
			ToolName: call.Function.Name,
			ToolID:   call.ID,
		})
	}

	// 所有工具执行完毕，汇总给 LLM
	return a.finalizeToolResults(ctx, userID, threadID, messages, resp, toolResults, originalContent)
}

// buildLLMMessages 将对话历史转换为 LLM 消息格式。
func (a *Agent) buildLLMMessages(userID, currentContent string) []llm.Message {
	turns := a.sessionManager.GetTurns(userID)

	system := fmt.Sprintf("你是 %s，一个安全的个人 AI 助手。", a.config.Name)
	if a.deps.Skills != nil {
		if skillPrompt := a.deps.Skills.BuildSystemPrompt(); skillPrompt != "" {
			system += "\n\n" + skillPrompt
		}
	}
	system += "\n\n你有多个可用工具（functions）。当用户要求你执行文件操作、执行命令或调用外部工具时，你必须在 assistant 消息中输出 tool_calls 字段来调用工具，严禁在 content 中直接声称已执行。只有等工具返回结果后，你才能基于结果回复用户。"

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: system},
	}

	for _, turn := range turns {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: turn.UserMsg})
		if turn.AgentResp != "" {
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: turn.AgentResp})
		}
	}

	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: currentContent})
	return messages
}

// buildLLMTools 将工具注册表转换为 LLM 工具定义，并根据技能信任级别过滤。
func (a *Agent) buildLLMTools() []llm.ToolDefinition {
	if a.deps.Tools == nil {
		return nil
	}

	names := a.deps.Tools.List()
	defs := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool, ok := a.deps.Tools.Get(name)
		if !ok {
			continue
		}
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionSchema{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.ParameterSchema(),
			},
		})
	}

	if a.deps.Skills != nil {
		active := a.deps.Skills.All()
		result := skills.AttenuateTools(defs, active)
		return result.Tools
	}

	return defs
}

// appendExtractedText 将附件中提取的文本追加到消息内容中。
func (a *Agent) appendExtractedText(msg channels.IncomingMessage) string {
	if len(msg.Attachments) == 0 {
		return msg.Content
	}
	var sb strings.Builder
	sb.WriteString(msg.Content)
	for _, att := range msg.Attachments {
		if att.ExtractedText == "" {
			continue
		}
		sb.WriteString("\n\n[Attachment: ")
		if att.Filename != "" {
			sb.WriteString(att.Filename)
		} else {
			sb.WriteString(att.MIMEType)
		}
		sb.WriteString("]\n")
		sb.WriteString(att.ExtractedText)
	}
	return sb.String()
}

// Resume 从保存的断点恢复执行。
func (a *Agent) Resume(ctx context.Context, userID, threadID string) (channels.OutgoingResponse, error) {
	pe := a.sessionManager.GetPendingExecution(userID, threadID)
	if pe == nil {
		return channels.OutgoingResponse{}, fmt.Errorf("no pending execution for user %s thread %s", userID, threadID)
	}

	var toolResults []llm.Message

	for i := pe.NextIndex; i < len(pe.ToolCalls); i++ {
		call := pe.ToolCalls[i]
		var params map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
			params = map[string]any{"message": call.Function.Arguments}
		}

		out, err := a.deps.Dispatcher.Dispatch(ctx, call.Function.Name, params, &tools.JobContext{
			UserID:   userID,
			ThreadID: threadID,
		})

		result := out.Content
		if err != nil {
			if gate.IsPauseError(err) {
				// 恢复过程中再次遇到 Pause（理论上不应发生）
				pe.NextIndex = i
				pe.CreatedAt = time.Now()
				pe.ExpiresAt = time.Now().Add(10 * time.Minute)
				a.sessionManager.SavePendingExecution(pe)
				return channels.OutgoingResponse{Status: "pending_gate", ThreadID: threadID}, nil
			}
			result = fmt.Sprintf("错误: %v", err)
		}

		toolResults = append(toolResults, llm.Message{
			Role:     llm.RoleTool,
			Content:  result,
			ToolName: call.Function.Name,
			ToolID:   call.ID,
		})
	}

	// 全部完成，清理 pending execution
	a.sessionManager.ClearPendingExecution(userID, threadID)

	a.publishEvent(userID, threadID, channels.EventGateResolved,
		"Resumed execution after approval", nil)

	return a.finalizeToolResults(ctx, userID, threadID, pe.Messages, llm.CompletionResponse{ToolCalls: pe.ToolCalls}, toolResults, pe.OriginalContent)
}

// finalizeToolResults 将工具执行结果汇总给 LLM 并返回最终响应。
func (a *Agent) finalizeToolResults(ctx context.Context, userID, threadID string, messages []llm.Message, resp llm.CompletionResponse, toolResults []llm.Message, originalContent string) (channels.OutgoingResponse, error) {
	messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
	messages = append(messages, toolResults...)

	finalResp, err := a.deps.LLM.Complete(ctx, messages, nil)
	if err != nil {
		return channels.OutgoingResponse{Content: fmt.Sprintf("LLM 总结错误: %v", err)}, nil
	}

	turn := Turn{
		UserMsg:   originalContent,
		AgentResp: finalResp.Content,
	}
	a.sessionManager.AddTurn(userID, turn)

	a.publishEvent(userID, threadID, channels.EventAgentResponse, finalResp.Content, nil)

	return channels.OutgoingResponse{Content: finalResp.Content}, nil
}

// echoReply 无 LLM 时的回退回复。
func (a *Agent) echoReply(userID, content string) channels.OutgoingResponse {
	_ = a.sessionManager.GetOrCreateThread(userID, "repl")
	resp := fmt.Sprintf("[%s] 你说: %s", a.config.Name, content)

	turn := Turn{
		UserMsg:   content,
		AgentResp: resp,
	}
	a.sessionManager.AddTurn(userID, turn)

	const maxTurns = 50
	a.sessionManager.CompactThread(userID, maxTurns)

	return channels.OutgoingResponse{Content: resp}
}

// persistIfNeeded 在配置了数据库时持久化线程。
func (a *Agent) persistIfNeeded(ctx context.Context, thread *Thread) {
	if a.deps.Database == nil {
		return
	}
	_ = a.persistThread(ctx, thread)
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
