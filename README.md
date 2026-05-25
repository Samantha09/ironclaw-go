# IronClaw Go

IronClaw 的混合 Go + Rust 架构 —— 安全的个人 AI 助手。

[![CI](https://github.com/Samantha09/ironclaw-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Samantha09/ironclaw-go/actions/workflows/ci.yml)

## 功能特性

- **多通道接入**：REPL、HTTP Gateway、Webhook 统一接入，Channel-Manager 模式聚合消息流。
- **LLM 多后端**：支持 OpenAI、Anthropic、Ollama，自动 tool-call 循环与上下文压缩。
- **工具系统**：内置工具（echo、memory、exec、http）+ WASM 扩展，统一注册表与安全管道。
- **安全层**：Prompt Injection 检测、密钥脱敏、代码危险操作扫描、速率限制。
- **密钥管理**：AES-GCM 加密存储，常量时间比较，防时序攻击。
- **分层配置**：defaults < 环境变量 < TOML < 数据库，支持动态重载。
- **WASM Sidecar**：Rust + wasmtime 运行第三方扩展，Go 永不执行不可信代码。

## 架构

```
┌─ Go Orchestrator ───────────────────────────────┐
│  cmd/ironclaw/          main entry point        │
│  internal/agent/        core reasoning loop     │
│  internal/channels/     input/output adapters   │
│  internal/tools/        tool registry + dispatch│
│  internal/db/           persistence abstraction │
│  internal/config/       env/TOML/DB config      │
│  internal/safety/       sanitization layer      │
│  internal/rpc/          gRPC client to sidecar  │
└─────────────────────────────────────────────────┘
                        │ gRPC (Unix Domain Socket)
┌─ Rust Sidecar ──────────────────────────────────┐
│  wasm-runtime/          wasmtime + WASI host    │
│  proto/                 protobuf definitions    │
└─────────────────────────────────────────────────┘
```

## 快速开始

### 前置要求

- Go 1.22+
- Rust 1.78+
- `protoc`（Protocol Buffers 编译器）

### 构建

```bash
# 同时构建两边
make build

# 或分别构建
make build-go
make build-rust
```

### 运行

```bash
# 先启动 Rust sidecar
make run-rust

# 在另一个终端中，启动 Go 编排器
make run-go
```

### 生成 gRPC 代码

```bash
make proto
```

## Docker 部署

```bash
# 一键启动（包含 PostgreSQL 与 Rust Sidecar）
docker-compose up --build

# 访问 HTTP Gateway
open http://localhost:8080
```

## 开发

```bash
make fmt      # 格式化 Go 和 Rust 代码
make test     # 运行两边的测试
make clean    # 清理构建产物
```

## 项目结构

| 路径 | 语言 | 用途 |
|------|------|------|
| `cmd/ironclaw/` | Go | 主编排器入口 |
| `internal/` | Go | 核心包（agent、channels、tools、db、config、safety） |
| `proto/` | Protobuf | Go 与 Rust 之间的 gRPC 契约 |
| `wasm-runtime/` | Rust | WASM sidecar（wasmtime + WASI） |
| `tests/e2e/` | Go | 端到端测试 |

## 设计决策

- **Go 负责编排**：编译速度快，HTTP/DB 生态完善，并发简单易用。
- **Rust 负责 WASM**：wasmtime 是唯一生产级的 Component Model 运行时。
- **gRPC over UDS**：两个进程之间强类型、低延迟的进程间通信。

详见 [docs/README.md](docs/README.md)。
