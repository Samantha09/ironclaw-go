# 技术栈

## 语言运行时

| 组件 | 语言 | 版本 | 原因 |
|-----------|----------|---------|--------|
| 编排器 | Go | 1.22+ | 编译快，标准库优秀，并发简单 |
| WASM 运行时 | Rust | 1.78+ | wasmtime 是唯一生产级的 Component Model 主机 |

## 通信

| 技术 | 角色 | 传输 |
|------------|------|-----------|
| gRPC | Go ↔ Rust IPC | Unix 域套接字（本地回退） |
| Protocol Buffers | 模式定义 | `proto/wasm_runtime.proto` |

## Go 依赖

| 库 | 版本 | 用途 |
|---------|---------|--------|
| `github.com/google/uuid` | `v1.6.0` | UUID 生成 |

**策略**：最小化外部依赖。HTTP、JSON、SQL 和并发优先使用标准库。

## Rust 依赖

| 库 | 版本 | 用途 |
|---------|---------|--------|
| `tokio` | `1.x` | 异步运行时 |
| `tonic` | `0.12` | gRPC 服务器 |
| `prost` | `0.13` | Protobuf 代码生成 |
| `wasmtime` | `31` | 带 Component Model 的 WASM 引擎 |
| `wasmtime-wasi` | `31` | WASI 主机实现 |
| `tracing` | `0.1` | 结构化日志 |
| `anyhow` | `1.x` | 错误处理 |

## 构建工具

| 工具 | 用途 |
|------|---------|
| `make` | 统一构建编排 |
| `protoc` | Protocol Buffers 编译器 |
| `go` / `cargo` | 原生构建 |

## 计划内 (MVP 之后)

| 组件 | 候选方案 | 时机 |
|-----------|-----------|------|
| PostgreSQL 驱动 | `github.com/jackc/pgx/v5` | 添加 PG 后端时 |
| libSQL 驱动 | `github.com/tursodatabase/go-libsql` | 添加 libSQL 后端时 |
| Web 框架 | `net/http` + `gorilla/mux` 或 `chi` | Web 网关 |
| WebSocket | `github.com/gorilla/websocket` | 网关实时通信 |
| CLI 框架 | `github.com/spf13/cobra` 或 std `flag` | 丰富的 CLI 子命令 |
| 配置加载器 | `github.com/spf13/viper` 或自研 | TOML + 热重载 |
| 测试 | `github.com/stretchr/testify` | 已在使用 |
