# SQLite 本地持久化迁移设计文档

> **目标**：为 IronClaw Go 增加 SQLite 本地文件持久化支持，替代内存数据库实现对话历史、审计日志和配置的持久化存储。

**架构**：新增 `internal/db/sqlite.go`，实现 `Database` 接口。复用 `postgres.go` 的自动建表模式，启动时执行 `migrate()` 创建表和索引。Schema 结构与 PostgreSQL 版保持一致，仅做 SQLite 语法适配。

**技术栈**：Go 1.22+，`modernc.org/sqlite`（纯 Go，无 CGO），标准 `database/sql`。

---

## 文件变更

| 文件 | 操作 | 说明 |
|---|---|---|
| `internal/db/sqlite.go` | 新增 | SQLite 实现，含 migration 和所有 CRUD |
| `internal/db/db.go` | 修改 | `New()` 中 `case "sqlite"` 分支 |
| `go.mod` / `go.sum` | 修改 | 添加 `modernc.org/sqlite` 依赖 |
| `internal/db/sqlite_test.go` | 新增 | 使用接口契约测试，验证与 MemoryDB 行为一致 |

---

## Schema（SQLite 兼容版）

```sql
CREATE TABLE IF NOT EXISTS settings (
    user_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, key)
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_calls TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_messages_thread_id ON messages(thread_id);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    input TEXT NOT NULL DEFAULT '',
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id);

CREATE TABLE IF NOT EXISTS action_records (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    tool_name TEXT NOT NULL,
    input TEXT NOT NULL DEFAULT '',
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_action_records_job_id ON action_records(job_id);
```

与 PostgreSQL 版的差异：
- `TIMESTAMPTZ` → `TEXT`，用 `datetime('now')` 默认值，存 ISO-8601 字符串
- `JSONB` → `TEXT`，存 JSON 字符串
- `BIGINT` → `INTEGER`
- `ON CONFLICT` 语法 SQLite 3.24+ 完全支持，保留

---

## 配置切换方式

默认保持 `driver = "memory"`。用户通过以下方式启用 SQLite：

环境变量：
```bash
IRONCLAW_DATABASE_DRIVER=sqlite
IRONCLAW_DATABASE_DSN=./ironclaw.db
```

TOML：
```toml
[database]
driver = "sqlite"
dsn = "./ironclaw.db"
```

DSN 支持：
- `./ironclaw.db` → 本地文件
- `:memory:` → 内存 SQLite（测试可用）

---

## 测试策略

`sqlite_test.go` 复用 `postgres_test.go` 的模式：对每个 `Database` 接口方法做契约测试，确保 `sqlite` 和 `memory` 实现行为一致。同时添加一个端到端测试：启动完整 App 用 SQLite 驱动，验证对话和审计记录正确落盘。

---

## 自检

1. **Spec coverage**：所有 Database 接口方法均有对应实现，Schema 覆盖所有表和索引。
2. **Placeholder scan**：无 TBD/TODO，所有代码片段完整。
3. **内部一致性**：Schema 与 PostgreSQL 版结构一致，仅做 SQLite 语法适配；`db.New()` 的驱动分支逻辑与现有模式一致。
