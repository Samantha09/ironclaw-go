package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresDB 是基于 PostgreSQL 的 Database 实现。
type PostgresDB struct {
	pool *pgxpool.Pool
}

// NewPostgresDB 创建新的 PostgreSQL 数据库实例并运行迁移。
func NewPostgresDB(dsn string, maxConns, minConns int) (*PostgresDB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	if minConns > 0 {
		cfg.MinConns = int32(minConns)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	db := &PostgresDB{pool: pool}
	if err := db.migrate(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// migrate 创建必要的表（如果不存在）。
func (p *PostgresDB) migrate(ctx context.Context) error {
	schema := `
CREATE TABLE IF NOT EXISTS settings (
    user_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, key)
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_calls JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
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
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id);

CREATE TABLE IF NOT EXISTS action_records (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    tool_name TEXT NOT NULL,
    input TEXT NOT NULL DEFAULT '',
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_action_records_job_id ON action_records(job_id);
`
	_, err := p.pool.Exec(ctx, schema)
	return err
}

// Ping 检查数据库连接。
func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Close 关闭连接池。
func (p *PostgresDB) Close() error {
	p.pool.Close()
	return nil
}

// Settings

func (p *PostgresDB) GetSetting(ctx context.Context, userID, key string) (string, error) {
	var value string
	err := p.pool.QueryRow(ctx,
		"SELECT value FROM settings WHERE user_id = $1 AND key = $2",
		userID, key,
	).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("setting not found for user %s key %s", userID, key)
	}
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}
	return value, nil
}

func (p *PostgresDB) SetSetting(ctx context.Context, userID, key string, value string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO settings (user_id, key, value, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, userID, key, value)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

func (p *PostgresDB) DeleteSetting(ctx context.Context, userID, key string) error {
	_, err := p.pool.Exec(ctx,
		"DELETE FROM settings WHERE user_id = $1 AND key = $2",
		userID, key,
	)
	if err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	return nil
}

// Conversations

func (p *PostgresDB) SaveConversation(ctx context.Context, conv *Conversation) error {
	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = time.Now()
	}
	conv.UpdatedAt = time.Now()

	_, err := p.pool.Exec(ctx, `
		INSERT INTO conversations (id, user_id, channel, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			channel = EXCLUDED.channel,
			title = EXCLUDED.title,
			updated_at = EXCLUDED.updated_at
	`, conv.ID, conv.UserID, conv.Channel, conv.Title, conv.CreatedAt, conv.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}

func (p *PostgresDB) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	var conv Conversation
	err := p.pool.QueryRow(ctx,
		"SELECT id, user_id, channel, title, created_at, updated_at FROM conversations WHERE id = $1",
		id,
	).Scan(&conv.ID, &conv.UserID, &conv.Channel, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("conversation %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return &conv, nil
}

func (p *PostgresDB) ListConversations(ctx context.Context, userID string, limit, offset int) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, user_id, channel, title, created_at, updated_at
		FROM conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	return scanConversations(rows)
}

func scanConversations(rows pgx.Rows) ([]*Conversation, error) {
	var result []*Conversation
	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.Channel, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, &conv)
	}
	return result, rows.Err()
}

func (p *PostgresDB) DeleteConversation(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM conversations WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

// Messages

func (p *PostgresDB) SaveMessage(ctx context.Context, msg *Message) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	toolCallsJSON, err := json.Marshal(msg.ToolCalls)
	if err != nil {
		return fmt.Errorf("marshal tool calls: %w", err)
	}

	_, err = p.pool.Exec(ctx, `
		INSERT INTO messages (id, thread_id, user_id, role, content, tool_calls, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, msg.ID, msg.ThreadID, msg.UserID, msg.Role, msg.Content, toolCallsJSON, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	// 更新 conversation 的 updated_at
	_, _ = p.pool.Exec(ctx,
		"UPDATE conversations SET updated_at = NOW() WHERE id = $1",
		msg.ThreadID,
	)

	return nil
}

func (p *PostgresDB) GetMessagesByThread(ctx context.Context, threadID string, limit, offset int) ([]*Message, error) {
	if limit <= 0 {
		limit = 1000
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, thread_id, user_id, role, content, tool_calls, created_at
		FROM messages
		WHERE thread_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`, threadID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

func scanMessages(rows pgx.Rows) ([]*Message, error) {
	var result []*Message
	for rows.Next() {
		var msg Message
		var toolCallsJSON []byte
		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.UserID, &msg.Role, &msg.Content, &toolCallsJSON, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if len(toolCallsJSON) > 0 {
			_ = json.Unmarshal(toolCallsJSON, &msg.ToolCalls)
		}
		result = append(result, &msg)
	}
	return result, rows.Err()
}

func (p *PostgresDB) DeleteMessagesByThread(ctx context.Context, threadID string) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM messages WHERE thread_id = $1", threadID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}

// Jobs

func (p *PostgresDB) SaveJob(ctx context.Context, job *Job) error {
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.UpdatedAt = time.Now()

	_, err := p.pool.Exec(ctx, `
		INSERT INTO jobs (id, user_id, name, status, input, output, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			input = EXCLUDED.input,
			output = EXCLUDED.output,
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
	`, job.ID, job.UserID, job.Name, job.Status, job.Input, job.Output, job.Error, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	return nil
}

func (p *PostgresDB) GetJob(ctx context.Context, id string) (*Job, error) {
	var job Job
	err := p.pool.QueryRow(ctx,
		"SELECT id, user_id, name, status, input, output, error, created_at, updated_at FROM jobs WHERE id = $1",
		id,
	).Scan(&job.ID, &job.UserID, &job.Name, &job.Status, &job.Input, &job.Output, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &job, nil
}

func (p *PostgresDB) ListJobs(ctx context.Context, userID string, limit, offset int) ([]*Job, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, user_id, name, status, input, output, error, created_at, updated_at
		FROM jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	return scanJobs(rows)
}

func scanJobs(rows pgx.Rows) ([]*Job, error) {
	var result []*Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.UserID, &job.Name, &job.Status, &job.Input, &job.Output, &job.Error, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, &job)
	}
	return result, rows.Err()
}

func (p *PostgresDB) UpdateJobStatus(ctx context.Context, id, status, output, errMsg string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, output = $2, error = $3, updated_at = NOW()
		WHERE id = $4
	`, status, output, errMsg, id)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

func (p *PostgresDB) DeleteJob(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM jobs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	return nil
}

// Action Records

func (p *PostgresDB) SaveActionRecord(ctx context.Context, rec *ActionRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}

	durationMs := rec.Duration.Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO action_records (id, job_id, tool_name, input, output, error, duration_ms, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, rec.ID, rec.JobID, rec.ToolName, rec.Input, rec.Output, rec.Error, durationMs, rec.CreatedAt)
	if err != nil {
		return fmt.Errorf("save action record: %w", err)
	}
	return nil
}

func (p *PostgresDB) ListActionRecordsByJob(ctx context.Context, jobID string) ([]*ActionRecord, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, job_id, tool_name, input, output, error, duration_ms, created_at
		FROM action_records
		WHERE job_id = $1
		ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list action records: %w", err)
	}
	defer rows.Close()

	return scanActionRecords(rows)
}

func scanActionRecords(rows pgx.Rows) ([]*ActionRecord, error) {
	var result []*ActionRecord
	for rows.Next() {
		var rec ActionRecord
		var durationMs int64
		if err := rows.Scan(&rec.ID, &rec.JobID, &rec.ToolName, &rec.Input, &rec.Output, &rec.Error, &durationMs, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Duration = time.Duration(durationMs) * time.Millisecond
		result = append(result, &rec)
	}
	return result, rows.Err()
}
