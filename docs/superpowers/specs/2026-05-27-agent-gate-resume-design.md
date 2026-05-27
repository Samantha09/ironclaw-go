# Agent 异步暂停-恢复与审核策略优化设计

## 背景

当前 IronClaw 的审批门控（gate）在交互模式下触发 `Pause` 时，并未真正暂停 Agent 执行。`Dispatcher.Dispatch` 创建 `PendingGate` 后返回错误字符串，Agent 的 `handleLLMToolCalls` 将该错误当作普通工具失败结果丢给 LLM 总结。用户看到"工具调用被拒绝/需要审批"的消息，需在侧边栏点"通过"后再次发送同一请求才能执行。

这带来两个问题：
1. **用户体验破碎**：gate 暂停后没有真正的"等待-恢复"流程，用户需要手动重发消息
2. **审核策略死板**：`DefaultToolRequirement` 是硬编码规则，无法根据用户历史行为自适应，导致不必要的打断

## 目标

1. 实现 Agent 的异步暂停-恢复机制：gate Pause 后保存执行上下文，用户审批后从断点继续
2. 优化审核策略：引入风险分级 + 信任偏好 + 学习模式，减少不必要的审批打断

---

## 1. 审核策略优化

### 1.1 风险分级

将工具操作按风险分为三级：

| 风险等级 | 操作示例 | 默认行为 |
|---------|---------|---------|
| **Low** | `file read`, `http GET`, `memory get`, `echo`, `time`, `json` | 永不审核（`Never`） |
| **Medium** | `file write`（工作目录内）, `memory set`, `http POST`（已知域名） | 首次审核，后续自动通过（学习模式） |
| **High** | `shell` 任意命令, `file delete`, `file mkdir`, `http POST`（外部域名） | 始终审核（`Always`） |

### 1.2 信任偏好

用户可配置的信任级别：

```go
type TrustLevel int

const (
    TrustCautious   TrustLevel = iota // 所有 Medium/High 都审
    TrustBalanced                      // Medium 首次审，High 始终审（默认）
    TrustPermissive                    // 只有 High 审，Medium 自动过
)
```

### 1.3 学习模式（AutoApprove 名单）

`TrustBalanced` 的核心机制：

- 系统维护一个 `learnedAutoApproves` 集合，键格式为 `"toolName:action:targetPattern"`
- 用户首次批准某个组合后，系统自动将其加入名单
- 后续同类操作直接 `Allow`，不再 Pause
- 名单按用户隔离（`userID` 维度）
- TTL：学习到的自动审批项 7 天未使用后过期

```go
type AutoApproveEntry struct {
    Key       string    // "file:write:/workspace/*"
    UserID    string
    ApprovedAt time.Time
    LastUsed  time.Time
}
```

### 1.4 策略评估器

替换硬编码的 `DefaultToolRequirement`，引入 `RiskBasedEvaluator`：

```go
type RiskBasedEvaluator struct {
    trustLevel          TrustLevel
    learnedAutoApproves map[string]*AutoApproveEntry
    mu                  sync.RWMutex
}

func (e *RiskBasedEvaluator) Evaluate(toolName string, params map[string]any, userID string) gate.ApprovalRequirement {
    risk := e.assessRisk(toolName, params)
    switch risk {
    case RiskLow:
        return gate.Never
    case RiskHigh:
        return gate.Always
    case RiskMedium:
        switch e.trustLevel {
        case TrustPermissive:
            return gate.Never
        case TrustCautious:
            return gate.UnlessAutoApproved
        case TrustBalanced:
            if e.isLearnedApproved(toolName, params, userID) {
                return gate.Never
            }
            return gate.UnlessAutoApproved
        }
    }
    return gate.UnlessAutoApproved
}
```

---

## 2. 暂停-恢复架构

### 2.1 核心流程

**正常路径（无 Pause）：**
```
runLLM → handleLLMToolCalls → 顺序执行所有 tool calls → 汇总结果 → LLM 总结 → 返回响应
```

**Pause 路径：**
```
runLLM → handleLLMToolCalls
    → 执行 toolCalls[0] ✓
    → 执行 toolCalls[1] → gate.Pause!
        → 保存 PendingExecution（NextIndex = 1）
        → 返回 {status: "pending_gate", gate: {...}}
```

**恢复路径（用户审批后）：**
```
/api/gates/approve
    → PendingStore.Resolve() 取出 gate
    → 查找 PendingExecution
    → Agent.Resume(ctx, pe)
        → 从 toolCalls[NextIndex] 继续顺序执行
        → 完成后汇总 → LLM 总结 → 返回响应
```

### 2.2 数据结构

#### PendingExecution（新增）

保存 Agent 执行到一半的上下文：

```go
package agent

import (
    "time"
    "github.com/nearai/ironclaw-go/internal/channels"
    "github.com/nearai/ironclaw-go/internal/llm"
)

// PendingExecution 保存 Agent 被 gate Pause 中断时的执行状态
type PendingExecution struct {
    RequestID       string
    UserID          string
    ThreadID        string
    Messages        []llm.Message      // 含用户消息和 assistant tool-call 请求
    ToolCalls       []llm.ToolCall     // LLM 返回的完整工具调用列表
    NextIndex       int                // 下一个待执行的索引（Pause 发生在此索引）
    OriginalContent string             // 用户原始输入（用于 resume 后的 LLM 总结）
    CreatedAt       time.Time
    ExpiresAt       time.Time          // 默认 10 分钟 TTL
}

func (pe *PendingExecution) IsExpired() bool {
    return time.Now().After(pe.ExpiresAt)
}
```

#### SessionManager 扩展

```go
type SessionManager struct {
    mu                sync.RWMutex
    active            map[string]*Thread
    history           map[string][]*Thread
    pendingExecutions map[string]*PendingExecution // key: userID + "/" + threadID
}

func (sm *SessionManager) SavePendingExecution(pe *PendingExecution) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    key := pe.UserID + "/" + pe.ThreadID
    sm.pendingExecutions[key] = pe
}

func (sm *SessionManager) GetPendingExecution(userID, threadID string) *PendingExecution {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    key := userID + "/" + threadID
    pe, ok := sm.pendingExecutions[key]
    if !ok || pe.IsExpired() {
        return nil
    }
    return pe
}

func (sm *SessionManager) ClearPendingExecution(userID, threadID string) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    delete(sm.pendingExecutions, userID+"/"+threadID)
}
```

### 2.3 Agent 改造

#### handleLLMToolCalls 的 Pause 检测

```go
func (a *Agent) handleLLMToolCalls(
    ctx context.Context,
    userID, threadID, originalContent string,
    messages []llm.Message,
    resp llm.CompletionResponse,
) (channels.OutgoingResponse, error) {
    var toolResults []llm.Message

    for i, call := range resp.ToolCalls {
        params := parseToolCallParams(call)

        out, err := a.deps.Dispatcher.Dispatch(ctx, call.Function.Name, params, &tools.JobContext{
            UserID:   userID,
            ThreadID: threadID,
        })

        if err != nil {
            // 检测是否是 gate Pause 错误
            if isGatePauseError(err) {
                // 保存断点状态
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

                // 返回特殊 pending 响应
                return channels.OutgoingResponse{
                    Status:   "pending_gate",
                    ThreadID: threadID,
                }, nil
            }
            toolResults = append(toolResults, llm.Message{Role: "tool", Content: fmt.Sprintf("错误: %v", err)})
            continue
        }

        toolResults = append(toolResults, llm.Message{Role: "tool", Content: out.Content})
    }

    // 所有工具执行完毕，汇总给 LLM
    return a.finalizeToolResults(ctx, messages, toolResults, originalContent)
}
```

> **注意**：`isGatePauseError` 的实现需要调整 Dispatcher 和 gate 的交互方式。当前 Pause 是通过返回一个普通 `error` 传递的，Resume 场景下无法区分 Pause 和普通错误。建议引入一个特殊的错误类型 `gate.PauseError`。

#### gate.PauseError（新增）

```go
package gate

type PauseError struct {
    ToolName    string
    RequestID   string
    Description string
}

func (e *PauseError) Error() string {
    return fmt.Sprintf("tool '%s' requires approval (request %s): %s", e.ToolName, e.RequestID, e.Description)
}

func IsPauseError(err error) bool {
    var pe *PauseError
    return errors.As(err, &pe)
}
```

Dispatcher 中 Pause 时返回 `*PauseError`：

```go
case gate.Pause:
    pg := d.pendingStore.Create(jobCtx.UserID, jobCtx.ThreadID, toolName, params, jobCtx.Channel)
    return tools.ToolOutput{}, &gate.PauseError{
        ToolName:    toolName,
        RequestID:   pg.RequestID,
        Description: pg.Description,
    }
```

#### Resume 方法（新增）

```go
func (a *Agent) Resume(ctx context.Context, userID, threadID string) (channels.OutgoingResponse, error) {
    pe := a.sessionManager.GetPendingExecution(userID, threadID)
    if pe == nil {
        return channels.OutgoingResponse{}, fmt.Errorf("no pending execution for user %s thread %s", userID, threadID)
    }

    var toolResults []llm.Message

    for i := pe.NextIndex; i < len(pe.ToolCalls); i++ {
        call := pe.ToolCalls[i]
        params := parseToolCallParams(call)

        out, err := a.deps.Dispatcher.Dispatch(ctx, call.Function.Name, params, &tools.JobContext{
            UserID:   userID,
            ThreadID: threadID,
        })

        result := out.Content
        if err != nil {
            if gate.IsPauseError(err) {
                // 恢复过程中再次遇到 Pause（理论上不应发生，因为已批准的 gate 已清理）
                // 防御性处理：更新状态并重新保存，不清理
                pe.NextIndex = i
                pe.CreatedAt = time.Now()
                pe.ExpiresAt = time.Now().Add(10 * time.Minute)
                a.sessionManager.SavePendingExecution(pe)
                return channels.OutgoingResponse{Status: "pending_gate", ThreadID: threadID}, nil
            }
            result = fmt.Sprintf("错误: %v", err)
        }

        toolResults = append(toolResults, llm.Message{Role: "tool", Content: result})
    }

    // 全部完成，清理 pending execution
    a.sessionManager.ClearPendingExecution(userID, threadID)

    return a.finalizeToolResults(ctx, pe.Messages, toolResults, pe.OriginalContent)
}
```

### 2.4 HTTP 网关改造

#### 响应格式扩展

`OutgoingResponse` 新增 `Status` 字段：

```go
package channels

type OutgoingResponse struct {
    Content  string `json:"content,omitempty"`
    ThreadID string `json:"thread_id,omitempty"`
    Status   string `json:"status,omitempty"`    // "ok" | "pending_gate" | "error"
}
```

#### /api/chat 处理 Pause

`handleChat` 中检测 Agent 返回的 `Status == "pending_gate"`：

```go
resp, err := agent.Process(msg)
if err != nil {
    writeError(w, err)
    return
}

if resp.Status == "pending_gate" {
    // 获取当前 pending gate 信息返回给前端
    gates := pendingStore.ListForUser(userID, threadID)
    var gateInfo map[string]any
    if len(gates) > 0 {
        g := gates[len(gates)-1]
        gateInfo = map[string]any{
            "request_id":  g.RequestID,
            "tool_name":   g.ToolName,
            "description": g.Description,
        }
    }
    _ = json.NewEncoder(w).Encode(map[string]any{
        "status":    "pending_gate",
        "thread_id": resp.ThreadID,
        "gate":      gateInfo,
    })
    return
}

_ = json.NewEncoder(w).Encode(resp)
```

#### /api/gates/approve 修订

当前 `handleGateApprove` 从 `PendingStore` 删除 gate 记录后直接返回。它**不直接触发 Agent Resume**，因为：

1. Gateway 与 Agent 通过 `msgChan` 解耦，Gateway 不直接持有 Agent 实例
2. Resume 可能调用 LLM，耗时不可控，同步 HTTP 响应容易超时

修订后的 `handleGateApprove` 只负责删除 gate 记录：

```go
func (g *Gateway) handleGateApprove(w http.ResponseWriter, r *http.Request) {
    // ... 解析请求参数 ...

    pg, err := g.pendingStore.Resolve(req.UserID, req.ThreadID, req.RequestID)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusNotFound)
        return
    }

    _ = json.NewEncoder(w).Encode(map[string]any{
        "approved":   true,
        "request_id": req.RequestID,
        "tool_name":  pg.ToolName,
    })
}
```

#### 恢复触发流程（推荐）

```
用户点击"通过"
    → POST /api/gates/approve
        → PendingStore.Resolve() 删除 gate 记录
        → 返回 {approved: true}

前端收到 approved 后，自动重发用户原消息（或一个空的 resume 信号）
    → POST /api/chat {user_id, thread_id, content: "__resume__"}
        → Agent.runLLM() 检测到 thread 有 PendingExecution
        → 检查 PendingStore：该 thread 已无任何 pending gate
        → 调用 Agent.Resume()
        → Resume 完成后返回最终结果
```

Agent `runLLM` 中新增检测：

```go
func (a *Agent) runLLM(ctx context.Context, msg channels.IncomingMessage) (channels.OutgoingResponse, error) {
    // 1. 检查是否有 pending execution
    if pe := a.sessionManager.GetPendingExecution(msg.UserID, msg.ThreadID); pe != nil {
        // 检查是否还有未解决的 gate
        if a.deps.PendingStore.HasPending(msg.UserID, msg.ThreadID) {
            // 仍有 pending gate，返回等待状态
            return channels.OutgoingResponse{Status: "pending_gate", ThreadID: msg.ThreadID}, nil
        }
        // Gate 已清除，恢复执行
        return a.Resume(ctx, msg.UserID, msg.ThreadID)
    }

    // 2. 正常 LLM 调用流程 ...
}
```

### 2.5 前端配合

- 收到 `pending_gate` 响应时，在消息列表插入一个"等待审批"的特殊消息卡片
- 禁用输入框，提示用户去侧边栏审批
- `loadGates()` 刷新后如果发现当前 thread 的 pending gate 已消失，自动触发一次 `send()`（发送 `"__resume__"` 或空消息）
- Resume 完成后，Agent 返回最终结果，前端正常渲染

---

## 3. 边界情况

| 场景 | 处理 |
|-----|-----|
| PendingExecution 过期（10 分钟） | `GetPendingExecution` 返回 nil，Agent 走正常流程，LLM 重新生成 tool calls |
| 用户拒绝 gate | PendingStore.Deny() 删除 gate，前端不重发消息；用户可手动再次发送原请求 |
| Resume 时再次 Pause | 防御性处理：重新保存 PendingExecution，返回新的 pending_gate |
| 多工具调用中第 2 个 Pause | `NextIndex = 1`，Resume 时从第 2 个继续，前面已完成的结果已在 `pe.Messages` 中 |
| 用户发送新消息时存在 PendingExecution | 正常流程：先 Resume 完成上次的，然后再处理新消息？或者直接丢弃旧 execution？**决策：丢弃旧的，按新消息处理**。因为用户主动发新消息意味着上一次的上下文已经不重要了。 |

---

## 4. 测试策略

1. **单元测试**：`RiskBasedEvaluator` 的各种风险组合和信任级别
2. **单元测试**：`SessionManager` 的 Save/Get/Clear PendingExecution，包括过期逻辑
3. **单元测试**：`Agent.handleLLMToolCalls` 的 Pause 检测和 Resume 恢复
4. **集成测试**：完整流程——发送消息 → gate Pause → 前端收到 pending → 审批 → Resume → 收到最终结果
5. **边界测试**：Resume 时再次 Pause、PendingExecution 过期、用户拒绝 gate

---

## 5. 任务拆分

本次实现可拆分为两个独立阶段：

**阶段 1：审核策略优化**（可独立发布）
- 新增 `gate.RiskLevel`、`gate.RiskBasedEvaluator`
- 新增 `gate.AutoApproveEntry` 和 LearnedAutoApprove 存储
- 替换 `DefaultToolRequirement` 为 `RiskBasedEvaluator`
- 更新 `gate_test.go`

**阶段 2：暂停-恢复机制**（依赖阶段 1）
- 新增 `gate.PauseError`
- 新增 `agent.PendingExecution`
- 扩展 `SessionManager`
- 改造 `Agent.handleLLMToolCalls` 和新增 `Agent.Resume`
- 改造 HTTP 网关 `/api/chat` 和 `/api/gates/approve`
- 前端适配 pending_gate 状态

建议按顺序实现，阶段 1 完成后即可验证审核策略是否减少了不必要的打断。
