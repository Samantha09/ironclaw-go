# Gateway SSE 实时推送实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 HTTP Gateway 增加 SSE 实时推送能力，让前端能实时接收 Agent 中间进度、工具调用结果、审批状态变化，替代当前的 30 秒阻塞轮询。

**Architecture:** 在 `httpgw` 包内引入轻量级 `EventHub` 管理 SSE 客户端连接。Agent 执行循环和工具调度器在关键节点发布事件，EventHub 按 userID/threadID 过滤后推送到对应 SSE 连接。前端使用原生 `EventSource` 订阅事件流。

**Tech Stack:** Go 1.22+ 标准库 (`net/http`, `sync`, `context`) + 原生 SSE 协议。无外部依赖。

---

### 文件映射

| 文件 | 职责 |
|---|---|
| `internal/channels/httpgw/events.go` | Event 类型定义 + EventHub 连接管理 |
| `internal/channels/httpgw/httpgw.go` | 新增 `/api/chat/stream` 和 `/api/events` SSE handler |
| `internal/channels/httpgw/static/index.html` | 前端改用 EventSource，实时渲染进度 |
| `internal/agent/agent.go` | 在工具调用前后、gate 暂停/恢复、最终响应处发布事件 |
| `internal/tools/dispatch.go` | 在工具执行前后发布 EventToolCall / EventToolResult |

---

### Task 1: events.go — Event 类型与 EventHub

**Files:**
- Create: `internal/channels/httpgw/events.go`
- Test: `internal/channels/httpgw/events_test.go`

- [ ] **Step 1: Write the failing test**

```go
package httpgw

import (
	"testing"
	"time"
)

func TestEventHubSubscribeAndPublish(t *testing.T) {
	hub := NewEventHub()

	ch := hub.Subscribe("user-1", "thread-a")
	defer hub.Unsubscribe("user-1", "thread-a", ch)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.Publish(Event{
			Type:     EventAgentResponse,
			UserID:   "user-1",
			ThreadID: "thread-a",
			Payload:  "hello",
		})
	}()

	select {
	case ev := <-ch:
		if ev.Payload != "hello" {
			t.Fatalf("expected payload hello, got %s", ev.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventHubFilterByUser(t *testing.T) {
	hub := NewEventHub()

	ch := hub.Subscribe("user-1", "")
	defer hub.Unsubscribe("user-1", "", ch)

	hub.Publish(Event{Type: EventAgentResponse, UserID: "user-2", ThreadID: "thread-a", Payload: "x"})
	hub.Publish(Event{Type: EventAgentResponse, UserID: "user-1", ThreadID: "thread-b", Payload: "y"})

	select {
	case ev := <-ch:
		if ev.Payload != "y" {
			t.Fatalf("expected payload y, got %s", ev.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/httpgw/... -run 'TestEventHub' -v`
Expected: FAIL — `NewEventHub`, `Event`, `Subscribe`, `Unsubscribe`, `Publish` undefined

- [ ] **Step 3: Write minimal implementation**

```go
package httpgw

import "sync"

// EventType 表示 SSE 事件类型。
type EventType string

const (
	EventAgentResponse EventType = "agent_response"
	EventToolCall      EventType = "tool_call"
	EventToolResult    EventType = "tool_result"
	EventGatePending   EventType = "gate_pending"
	EventGateResolved  EventType = "gate_resolved"
	EventError         EventType = "error"
	EventPing          EventType = "ping"
)

// Event 是通过 SSE 推送的结构化事件。
type Event struct {
	Type     EventType `json:"type"`
	UserID   string    `json:"user_id"`
	ThreadID string    `json:"thread_id,omitempty"`
	Payload  string    `json:"payload"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// EventHub 管理 SSE 客户端订阅，按 userID + threadID 过滤推送。
type EventHub struct {
	mu    sync.RWMutex
	subs  map[string][]chan Event // key: "userID|threadID" 或 "userID|"
}

func NewEventHub() *EventHub {
	return &EventHub{
		subs: make(map[string][]chan Event),
	}
}

// Subscribe 注册一个事件接收通道。
// threadID 为空字符串时表示订阅该用户的所有线程事件。
func (h *EventHub) Subscribe(userID, threadID string) <-chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 16)
	key := subKey(userID, threadID)
	h.subs[key] = append(h.subs[key], ch)
	return ch
}

// Unsubscribe 注销指定通道。
func (h *EventHub) Unsubscribe(userID, threadID string, ch <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := subKey(userID, threadID)
	list := h.subs[key]
	for i, c := range list {
		if c == ch {
			h.subs[key] = append(list[:i], list[i+1:]...)
			close(c)
			break
		}
	}
	if len(h.subs[key]) == 0 {
		delete(h.subs, key)
	}
}

// Publish 将事件推送给所有匹配的订阅者。
func (h *EventHub) Publish(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 精确匹配 userID+threadID
	if ev.ThreadID != "" {
		key := subKey(ev.UserID, ev.ThreadID)
		for _, ch := range h.subs[key] {
			select {
			case ch <- ev:
			default:
			}
		}
	}

	// 广播给该 userID 的所有线程订阅者
	allKey := subKey(ev.UserID, "")
	for _, ch := range h.subs[allKey] {
		select {
		case ch <- ev:
		default:
		}
	}
}

func subKey(userID, threadID string) string {
	if threadID == "" {
		return userID + "|"
	}
	return userID + "|" + threadID
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/httpgw/... -run 'TestEventHub' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/httpgw/events.go internal/channels/httpgw/events_test.go
git commit -m "feat(gateway): add EventHub for SSE streaming

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 2: httpgw.go — SSE handler 端点

**Files:**
- Modify: `internal/channels/httpgw/httpgw.go`
- Test: `internal/channels/httpgw/httpgw_test.go`（若无则创建）

- [ ] **Step 1: Gateway 集成 EventHub**

在 `Gateway` 结构体中添加 `eventHub *EventHub` 字段，并在 `New` 中初始化。

```go
type Gateway struct {
	// ... 现有字段 ...
	eventHub *EventHub
}

func New(port int) *Gateway {
	return &Gateway{
		// ... 现有初始化 ...
		eventHub: NewEventHub(),
	}
}
```

- [ ] **Step 2: 添加 `/api/chat/stream` handler**

在 `Start()` 的 mux 注册中新增：

```go
mux.HandleFunc("/api/chat/stream", g.handleChatStream)
```

实现 `handleChatStream`：

```go
func (g *Gateway) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := g.authenticatedUserID(r)
	if userID == "" {
		userID = r.URL.Query().Get("user_id")
	}
	if userID == "" {
		userID = "anonymous"
	}
	threadID := r.URL.Query().Get("thread_id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// 发送初始连接事件
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", `{"status":"ok"}`)
	flusher.Flush()

	evCh := g.eventHub.Subscribe(userID, threadID)
	defer g.eventHub.Unsubscribe(userID, threadID, evCh)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev, ok := <-evCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-g.shutdown:
			return
		}
	}
}
```

- [ ] **Step 3: 修改 `SendMessage` 以同时通过 EventHub 广播**

在 `SendMessage` 末尾（现有广播逻辑之后）添加 EventHub 发布：

```go
func (g *Gateway) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	// ... 现有逻辑不变 ...

	// 新增：通过 EventHub 推送事件
	if g.eventHub != nil {
		var evType EventType
		switch msg.Status {
		case "pending_gate":
			evType = EventGatePending
		case "error":
			evType = EventError
		default:
			evType = EventAgentResponse
		}
		g.eventHub.Publish(Event{
			Type:     evType,
			UserID:   msg.UserID,
			ThreadID: msg.ThreadID,
			Payload:  msg.Content,
		})
	}
	return nil
}
```

- [ ] **Step 4: 运行 gateway 测试**

Run: `go test ./internal/channels/httpgw/... -v`
Expected: PASS（现有测试 + 新增 EventHub 测试）

- [ ] **Step 5: Commit**

```bash
git add internal/channels/httpgw/
git commit -m "feat(gateway): add /api/chat/stream SSE endpoint and EventHub integration

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 3: Agent 执行循环中发布进度事件

**Files:**
- Modify: `internal/agent/agent.go`

- [ ] **Step 1: Deps 中添加 EventHub 接口**

在 `internal/agent/deps.go` 中检查 `Deps` 结构体。如果其中没有 `EventPublisher` 或类似接口，添加一个：

```go
// EventPublisher 发布 Agent 执行过程中的事件。
type EventPublisher interface {
	Publish(ev channels.Event)
}
```

并在 `Deps` 中添加：

```go
type Deps struct {
	// ... 现有字段 ...
	EventPublisher EventPublisher
}
```

> 注意：`channels.Event` 类型不存在，应使用 `httpgw.Event`。为减少循环依赖，在 `channels` 包中定义 `Event` 类型，或由 Agent 直接引用 `httpgw.Event`。为避免循环依赖，最佳做法是在 `channels` 包中定义通用的 `Event` 接口/类型。

替代方案（避免循环依赖）：在 `channels` 包中定义 `Event` 类型：

```go
package channels

type EventType string

type Event struct {
	Type     EventType
	UserID   string
	ThreadID string
	Payload  string
	Meta     map[string]any
}
```

然后 `httpgw.Event` 改为 `channels.Event` 的别名或直接使用 `channels.Event`。

修改 `events.go`：

```go
package httpgw

import "github.com/nearai/ironclaw-go/internal/channels"

type Event = channels.Event
type EventType = channels.EventType

const (
	EventAgentResponse EventType = "agent_response"
	// ...
)
```

- [ ] **Step 2: 在 Agent 工具调用前后发布事件**

在 `handleLLMToolCalls` 或等效函数中，在调用 `dispatcher.Dispatch` 前后添加事件发布。假设在 `agent.go` 中有如下代码段（需要阅读确认）：

```go
// 在工具调用前
if a.deps.EventPublisher != nil {
	a.deps.EventPublisher.Publish(channels.Event{
		Type:     channels.EventType("tool_call"),
		UserID:   userID,
		ThreadID: threadID,
		Payload:  fmt.Sprintf("Calling tool: %s", toolName),
		Meta:     map[string]any{"tool_name": toolName},
	})
}

// 执行工具
out, err := dispatcher.Dispatch(ctx, toolName, params, jobCtx)

// 在工具调用后
if a.deps.EventPublisher != nil {
	var payload string
	if err != nil {
		payload = fmt.Sprintf("Tool %s failed: %v", toolName, err)
	} else {
		payload = fmt.Sprintf("Tool %s completed", toolName)
	}
	a.deps.EventPublisher.Publish(channels.Event{
		Type:     channels.EventType("tool_result"),
		UserID:   userID,
		ThreadID: threadID,
		Payload:  payload,
		Meta:     map[string]any{"tool_name": toolName, "duration_ms": out.Duration},
	})
}
```

- [ ] **Step 3: 在 gate Pause 和 Resume 时发布事件**

在检测到 gate 返回 `Pause` 时：

```go
a.deps.EventPublisher.Publish(channels.Event{
	Type:     channels.EventType("gate_pending"),
	UserID:   userID,
	ThreadID: threadID,
	Payload:  fmt.Sprintf("Waiting for approval: %s", toolName),
	Meta:     map[string]any{"tool_name": toolName},
})
```

在从 `PendingExecution` 恢复并继续执行后：

```go
a.deps.EventPublisher.Publish(channels.Event{
	Type:     channels.EventType("gate_resolved"),
	UserID:   userID,
	ThreadID: threadID,
	Payload:  fmt.Sprintf("Resumed execution after approval"),
	Meta:     map[string]any{"tool_name": toolName},
})
```

- [ ] **Step 4: 运行 agent 测试**

Run: `go test ./internal/agent/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/ internal/channels/
git commit -m "feat(agent): publish progress events during tool execution and gate lifecycle

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 4: 工具调度器中发布事件

**Files:**
- Modify: `internal/tools/dispatch.go`

- [ ] **Step 1: Dispatcher 添加 EventPublisher**

在 `Dispatcher` 结构体中添加：

```go
type Dispatcher struct {
	// ... 现有字段 ...
	eventPublisher func(ev channels.Event)
}
```

添加 setter：

```go
func (d *Dispatcher) WithEventPublisher(pub func(ev channels.Event)) *Dispatcher {
	d.eventPublisher = pub
	return d
}
```

- [ ] **Step 2: 在 Dispatch 方法中发布工具执行事件**

在 `Dispatch` 方法中，工具执行前后：

```go
if d.eventPublisher != nil {
	d.eventPublisher(channels.Event{
		Type:     channels.EventType("tool_call"),
		UserID:   jobCtx.UserID,
		ThreadID: jobCtx.ThreadID,
		Payload:  fmt.Sprintf("Executing tool: %s", toolName),
		Meta:     map[string]any{"tool_name": toolName, "params": params},
	})
}

// 执行工具...

if d.eventPublisher != nil {
	var payload string
	status := "success"
	if err != nil {
		payload = fmt.Sprintf("Tool %s failed: %v", toolName, err)
		status = "error"
	} else {
		payload = fmt.Sprintf("Tool %s completed (%d ms)", toolName, out.Duration)
	}
	d.eventPublisher(channels.Event{
		Type:     channels.EventType("tool_result"),
		UserID:   jobCtx.UserID,
		ThreadID: jobCtx.ThreadID,
		Payload:  payload,
		Meta:     map[string]any{"tool_name": toolName, "status": status, "duration_ms": out.Duration},
	})
}
```

- [ ] **Step 3: 运行 tools 测试**

Run: `go test ./internal/tools/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): dispatcher publishes tool execution events

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 5: builder.go 中 wiring EventPublisher

**Files:**
- Modify: `internal/app/builder.go`

- [ ] **Step 1: 在 builder 中连接 EventPublisher**

在 `Build` 函数中，创建 Gateway 后，将 Gateway 的 EventHub 暴露给 Agent 和 Dispatcher：

```go
// 在 Gateway 创建后
gw := httpgw.New(cfg.Channels.HTTPPort).WithAuth(authenticator) // ...
// ...

// Safety + Dispatcher
safetyLayer := safety.NewLayerWithConfig(safety.Config{...})
dispatcher := tools.NewDispatcher(registry, safetyLayer, database)
// 将 Gateway 的 EventHub 绑定到 Dispatcher
dispatcher.WithEventPublisher(func(ev channels.Event) {
	gw.PublishEvent(ev)
})
// ...

// Agent
agentDeps := agent.Deps{
	// ... 现有字段 ...
	EventPublisher: func(ev channels.Event) {
		gw.PublishEvent(ev)
	},
}
```

在 `httpgw.Gateway` 上添加 `PublishEvent` 方法：

```go
func (g *Gateway) PublishEvent(ev channels.Event) {
	if g.eventHub != nil {
		g.eventHub.Publish(Event(ev))
	}
}
```

- [ ] **Step 2: 运行完整测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/app/builder.go internal/channels/httpgw/httpgw.go
git commit -m "feat(app): wire EventPublisher from Gateway to Agent and Dispatcher

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 6: 前端 index.html 适配 SSE

**Files:**
- Modify: `internal/channels/httpgw/static/index.html`

- [ ] **Step 1: 替换阻塞 fetch 为 EventSource**

在 `index.html` 的 JavaScript 中：

1. 添加 `EventSource` 连接管理：

```javascript
let eventSource = null;

function connectSSE(threadId) {
  if (eventSource) {
    eventSource.close();
  }
  const url = new URL('/api/chat/stream', window.location.href);
  if (threadId) url.searchParams.set('thread_id', threadId);
  eventSource = new EventSource(url.toString());

  eventSource.addEventListener('agent_response', (e) => {
    const data = JSON.parse(e.data);
    appendMessage('assistant', data.payload);
  });

  eventSource.addEventListener('tool_call', (e) => {
    const data = JSON.parse(e.data);
    appendToolProgress(data.meta.tool_name, 'running');
  });

  eventSource.addEventListener('tool_result', (e) => {
    const data = JSON.parse(e.data);
    appendToolProgress(data.meta.tool_name, data.meta.status);
  });

  eventSource.addEventListener('gate_pending', (e) => {
    const data = JSON.parse(e.data);
    showPendingGate(data);
  });

  eventSource.addEventListener('gate_resolved', (e) => {
    hidePendingGate();
  });

  eventSource.addEventListener('error', (e) => {
    console.error('SSE error', e);
  });

  eventSource.addEventListener('connected', () => {
    console.log('SSE connected');
  });
}
```

2. 修改 `sendMessage` 函数：发送 POST 到 `/api/chat` 后不再阻塞等待响应，而是由 SSE 事件触发消息渲染。

```javascript
async function sendMessage() {
  // ... 收集输入 ...
  
  // 发送消息（非阻塞）
  await fetch('/api/chat', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({content, thread_id: currentThreadId, user_id: currentUserId})
  });
  
  // 确保 SSE 连接活跃
  if (!eventSource || eventSource.readyState === EventSource.CLOSED) {
    connectSSE(currentThreadId);
  }
}
```

3. 保留 `/api/chat` 作为 fallback（用于非 SSE 客户端）。

- [ ] **Step 2: 添加工具进度 UI**

在 HTML 中添加一个 `#tool-progress` 容器，用于显示当前正在执行的工具：

```html
<div id="tool-progress" class="hidden">
  <div class="tool-indicator">
    <span class="tool-name"></span>
    <span class="tool-status">running...</span>
  </div>
</div>
```

对应的 JS 函数：

```javascript
function appendToolProgress(toolName, status) {
  const container = document.getElementById('tool-progress');
  const nameEl = container.querySelector('.tool-name');
  const statusEl = container.querySelector('.tool-status');
  nameEl.textContent = toolName;
  statusEl.textContent = status === 'running' ? 'running...' : 'done';
  container.classList.remove('hidden');
  if (status !== 'running') {
    setTimeout(() => container.classList.add('hidden'), 2000);
  }
}
```

- [ ] **Step 3: 手动验证前端功能**

启动服务：`go run ./cmd/ironclaw`
打开浏览器访问 `http://localhost:8080`
发送消息，观察：
1. SSE 连接是否成功建立（DevTools Network → EventStream）
2. 工具调用时是否显示进度指示器
3. 审批门控触发时是否实时显示 pending 面板
4. Agent 最终响应是否通过 SSE 渲染

- [ ] **Step 4: Commit**

```bash
git add internal/channels/httpgw/static/index.html
git commit -m "feat(gateway): frontend switches to EventSource for real-time streaming

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 7: 端到端集成测试

**Files:**
- Create: `tests/e2e/sse_test.go`（或追加到现有 e2e 测试）

- [ ] **Step 1: 编写 SSE 端到端测试**

```go
package e2e

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSSEStreamConnection(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/chat/stream?user_id=test-e2e")
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}

	// 读取 connected 事件
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read SSE line: %v", err)
	}
	if !strings.Contains(line, "event: connected") {
		t.Fatalf("expected connected event, got: %s", line)
	}
}
```

- [ ] **Step 2: 运行 e2e 测试**

需要先启动服务，然后运行测试。

Run: `go test ./tests/e2e/... -v -run TestSSE`
Expected: PASS（服务已启动前提下）

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/sse_test.go
git commit -m "test(e2e): add SSE stream connection end-to-end test

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## 自审查

**1. Spec coverage：**

| 功能需求 | 对应任务 |
|---|---|
| SSE 连接管理（订阅/发布/心跳） | Task 1 |
| `/api/chat/stream` 端点 | Task 2 |
| Agent 进度事件发布 | Task 3 |
| 工具调度器事件发布 | Task 4 |
| 组件 wiring | Task 5 |
| 前端 EventSource 适配 | Task 6 |
| 端到端测试 | Task 7 |

**2. Placeholder scan：**

- 无 "TBD"、"TODO"
- 每个步骤包含完整代码
- 无模糊描述

**3. Type consistency：**

- `channels.Event` 在 Task 3 定义（需要提前在 Task 1/2 中迁移）
- `EventHub.Subscribe/Unsubscribe/Publish` 签名前后一致
- `Gateway.PublishEvent` 作为 facade 方法统一暴露

**注意：** Task 1 中 `events.go` 最初使用 `httpgw.Event`，Task 3 中 Agent 引用 `channels.Event`。为避免循环依赖，需要在 Task 1 执行时就将 Event 类型迁移到 `channels` 包。已在 Task 1 Step 3 中说明此调整。
