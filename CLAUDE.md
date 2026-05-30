# IronClaw Go —— 开发指南

IronClaw Go 是 IronClaw 安全个人 AI 助手的混合 Go + Rust 重构实现。Go 编排器负责业务逻辑、通道和配置；Rust sidecar 托管 wasmtime WASM 运行时。

> **每次开发前，阅读 `.claude/memory/MEMORY.md` 中的记忆条目。** 其中包含参考项目路径（`/home/san/GolandProjects/ironclaw`）和功能对照基准，新模块实现需与 Rust 原版对齐。

## 架构

```
┌─ Go 编排器 ─────────────────────────────────────┐
│  cmd/ironclaw/          主入口点                │
│  internal/agent/        核心推理循环            │
│  internal/channels/     输入/输出适配器         │
│  internal/tools/        工具注册表 + 调度       │
│  internal/db/           持久化抽象层            │
│  internal/config/       环境变量/TOML/数据库配置│
│  internal/safety/       净化层                  │
│  internal/rpc/          到 sidecar 的 gRPC 客户端│
└─────────────────────────────────────────────────┘
                        │ gRPC (Unix 域套接字)
┌─ Rust Sidecar ──────────────────────────────────┐
│  wasm-runtime/          wasmtime + WASI 主机    │
│  proto/                 protobuf 定义           │
└─────────────────────────────────────────────────┘
```

## 构建与测试

```bash
# 构建两边
make build

# 分别构建
make build-go
make build-rust

# 运行测试
make test

# 格式化代码
make fmt

# 从 proto 生成 gRPC 存根
make proto
```

## 代码风格

- **要求 Go 1.22+**。尽可能使用标准库；最小化外部依赖。
- **生产代码中禁止 `panic()`**（测试中可以使用）。
- **错误包装**：在边界层始终使用 `fmt.Errorf("...: %w", err)` 进行包装。
- **Context 传播**：每个 I/O 操作和 goroutine 启动都必须接受并遵守 `context.Context`。
- **无全局状态**：注册表、数据库、配置通过构造函数注入（参见 `app/builder.go`）。
- **边界使用接口**：`Database`、`Channel`、`Tool`、`LlmProvider` 均为 Go 接口。
- **强类型优于字符串**：使用 `type UserID string`、`type ExtensionName string` 并配合验证构造函数。切勿在内部边界之间传递裸 `string`。
- **全部使用中文**：所有提交信息（commit message）、代码注释、文档、变量/函数的英文命名除外。与用户的所有交互也用中文。
- **经常提交**：每个独立功能点完成后立即 `git commit`，不要攒多个改动到一次提交。提交粒度以"一个完整的小功能或修复"为单位。

## 模块规范

| 包 | 职责 |
|---------|---------------|
| `internal/agent/` | 消息处理循环、工具调用解析、会话管理 |
| `internal/channels/` | 通道接口、管理器 fan-in、各通道实现 |
| `internal/tools/` | 工具接口、注册表、带安全管道的调度器 |
| `internal/db/` | 持久化接口；MVP 使用内存，后续支持 PostgreSQL/libSQL |
| `internal/config/` | 分层配置：默认值 < 环境变量 < TOML < 数据库 |
| `internal/safety/` | 输出净化、泄漏检测、入站扫描 |
| `internal/rpc/` | 到 Rust WASM sidecar 的 gRPC 客户端 |

## 添加新通道

1. 创建 `internal/channels/<name>/<name>.go`
2. 实现 `Channel` 接口
3. 在 `app/builder.go` 中注册

## 添加新工具

1. 创建 `internal/tools/builtin/<name>.go`
2. 实现 `Tool` 接口
3. 通过 `registry.Register(builtin.NewXxxTool())` 在 `app/builder.go` 中注册

## 错误处理

在 `internal/errors/` 中使用领域特定的错误类型。在通道边界映射为用户可见的消息 —— 切勿向用户暴露内部标识符或路径。

## 测试

- 单元测试放在源文件旁边的 `_test.go` 文件中。
- 优先使用表驱动测试。
- 单元测试使用模拟接口；集成测试使用真实的内存数据库。

## 设计原则

1. **所有操作都通过调度器**：UI 发起的变更必须使用 `ToolDispatcher.Dispatch()`，绝不能直接调用数据库/工作区。
2. **边界安全**：所有外部数据在存储或送入 LLM 之前必须经过扫描。
3. **LLM 数据永不删除**：一旦持久化，对话历史和工具追踪记录将永久保留。
4. **进程隔离**：WASM 执行仅在 Rust sidecar 中进行；Go 绝不执行不可信代码。
