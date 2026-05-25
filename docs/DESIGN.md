# 系统设计

## 概述

IronClaw Go 是一个具有深度防御安全机制的多通道 AI Agent 框架。不可信的扩展（工具和通道）在由专用 Rust sidecar 进程托管的 WASM 沙箱中执行。Go 编排器管理所有业务逻辑、用户交互和持久化。

## 核心抽象

### 通道 (Channel)

`Channel` 是一个双向管道：它从用户产生 `IncomingMessage`，并消费来自 Agent 的 `OutgoingResponse`。

```
通道 ──(IncomingMessage)──→ Agent
通道 ←──(OutgoingResponse)── Agent
```

特性：
- 每个通道在自己的 goroutine 中运行。
- `ChannelManager` 通过单个 `Receive()` 调用将多个通道流 fan-in 合并。
- 通道可互换：后续添加 Telegram/Slack/Discord 不会改变 Agent 逻辑。

### 工具 (Tool)

`Tool` 是由 Agent 或面向用户的处理器调用的能力单元。

生命周期：
1. **注册**：启动时添加到 `ToolRegistry`。
2. **调度**：`ToolDispatcher` 执行安全管道（净化、速率限制、密钥脱敏）。
3. **执行**：工具实现运行（内置 = 原生 Go；外部 = 通过 gRPC 到 Rust sidecar）。
4. **审计**：结果存储在任务历史中。

### Agent 循环

Agent 是一个简单的事件循环：

```
for {
    msg := channelManager.Receive(ctx)
    response := agent.ProcessMessage(ctx, msg)
    channelManager.Broadcast(ctx, response)
}
```

MVP 行为：
- 检测 `tool:<name> <params>` 语法以直接调用工具。
- 无法识别的输入回退为 echo。
- 未来：集成 LLM 提供商以实现自然语言 → 工具调用推理。

## 数据流

```
用户输入
    │
    ▼
┌─────────────┐
│   通道      │ (REPL / Web / HTTP / WASM)
└──────┬──────┘
       │ IncomingMessage
       ▼
┌─────────────┐
│   管理器    │ ── fan-in ──→
└──────┬──────┘               │
       │                      ▼
       │               ┌─────────────┐
       │               │    Agent    │
       │               └──────┬──────┘
       │                      │
       │          ┌───────────┼───────────┐
       │          ▼           ▼           ▼
       │    ┌────────┐  ┌────────┐  ┌──────────┐
       │    │  工具  │  │  工具  │  │   LLM    │
       │    │  (Go)  │  │(WASM  │  │ (未来)   │
       │    │        │  │ sidecar│  │          │
       │    └────┬───┘  └────┬───┘  └────┬─────┘
       │         │           │           │
       │         └───────────┴───────────┘
       │                     │
       │                     ▼
       │              ┌─────────────┐
       │              │   安全层    │
       │              │             │
       │              └──────┬──────┘
       │                     │
       │                     ▼
       │              ┌─────────────┐
       │              │  出站响应   │
       │              │             │
       │              └──────┬──────┘
       │                     │
       └─────────────────────┘
                             ▼
                      ┌─────────────┐
                      │   通道      │ ──→ 用户
                      └─────────────┘
```

## 安全模型

### 信任边界

| 边界 | 执行机制 |
|----------|-------------|
| 用户 → Agent | 输入净化 (`SafetyLayer.ScanInbound`) |
| Agent → 工具 | 模式验证、速率限制、权限检查 |
| 工具 → 外部 | 网络白名单，出口处凭证注入 |
| 工具运行时 | WASM 沙箱：燃料计量、内存限制、能力剥离 |

### 密钥处理

- 密钥**绝不**直接传递给 WASM 客户代码。
- Rust sidecar 持有 `SecretsStore` 句柄。
- 来自 WASM 的 HTTP 请求被代理：密钥由主机在客户机构建请求之后、请求离开进程**之前**注入。

## 并发模型

- **每个通道一个 goroutine**：每个通道适配器在自己的 I/O（stdin、HTTP 监听器、WebSocket）上阻塞。
- **Agent 循环**：单个 goroutine 消费来自 `ChannelManager.Receive()` 的数据。
- **工具执行**：内置工具在 Agent goroutine 中运行（快速）；WASM 工具调用通过 gRPC 异步调度到 Rust sidecar。
- **数据库**：MVP 的内存数据库使用 `sync.RWMutex`；未来的 SQL 后端使用连接池。

## 持久化策略 (MVP)

- **仅内存**：使用互斥锁保护的映射的 `MemoryDB`。
- **未来**：由 PostgreSQL (`pgx`) 或 libSQL (`go-libsql`) 支持的 `Database` 接口。

## WASM Sidecar 协议

Rust sidecar 暴露了在 `proto/wasm_runtime.proto` 中定义的 gRPC 服务。

关键 RPC：
- `ExecuteTool`：加载 WASM 模块，带限制实例化，运行，返回输出。
- `LoadChannel`：加载 WASM 通道适配器，返回句柄。
- `SendChannelEvent`：将事件推入已加载的通道。
- `Health`：存活探针。

Sidecar 对已加载的通道是**有状态**的，但对工具执行是**无状态**的（每个 `ExecuteTool` 携带完整的模块字节，允许 Go 端管理缓存）。

## 扩展维度

| 维度 | 当前 | 未来 |
|-----------|---------|--------|
| 用户 | 单用户 (`owner_id`) | 多租户，按用户数据库作用域 |
| 通道 | REPL | Web 网关、HTTP webhook、Signal、WASM 通道 |
| 工具 | 仅内置 | 通过 sidecar 的 WASM 工具、MCP 服务器 |
| LLM | 无 (echo) | 可插拔提供商（OpenAI、Anthropic、本地） |
| 存储 | 内存 | PostgreSQL + libSQL 双后端 |
