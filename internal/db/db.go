package db

import (
	"context"
	"fmt"
	"time"
)

// Message 表示对话中的一条消息。
type Message struct {
	ID        string
	ThreadID  string
	UserID    string
	Role      string // user, assistant, system, tool
	Content   string
	ToolCalls []ToolCall
	CreatedAt time.Time
}

// ToolCall 表示一次工具调用。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON
	Result    string // JSON
	Error     string
	Status    string // pending, success, failure
}

// Conversation 表示一个对话线程。
type Conversation struct {
	ID        string
	UserID    string
	Channel   string
	Title     string
	Messages  []Message
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Job 表示一个后台任务。
type Job struct {
	ID        string
	UserID    string
	Name      string
	Status    string // pending, running, completed, failed, cancelled
	Input     string
	Output    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ActionRecord 表示一次工具调用审计记录。
type ActionRecord struct {
	ID        string
	JobID     string
	ToolName  string
	Input     string
	Output    string
	Error     string
	Duration  time.Duration
	CreatedAt time.Time
}

// Database 是后端无关的持久化接口。
type Database interface {
	// Ping 检查数据库连接。
	Ping(ctx context.Context) error

	// Close 关闭数据库连接。
	Close() error

	// Settings
	GetSetting(ctx context.Context, userID, key string) (string, error)
	SetSetting(ctx context.Context, userID, key string, value string) error
	DeleteSetting(ctx context.Context, userID, key string) error

	// Conversations
	SaveConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	ListConversations(ctx context.Context, userID string, limit, offset int) ([]*Conversation, error)
	DeleteConversation(ctx context.Context, id string) error

	// Messages
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessagesByThread(ctx context.Context, threadID string, limit, offset int) ([]*Message, error)
	DeleteMessagesByThread(ctx context.Context, threadID string) error

	// Jobs
	SaveJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	ListJobs(ctx context.Context, userID string, limit, offset int) ([]*Job, error)
	UpdateJobStatus(ctx context.Context, id, status, output, errMsg string) error
	DeleteJob(ctx context.Context, id string) error

	// Action Records
	SaveActionRecord(ctx context.Context, rec *ActionRecord) error
	ListActionRecordsByJob(ctx context.Context, jobID string) ([]*ActionRecord, error)
}

// New 根据驱动创建数据库实例。
func New(driver, dsn string) (Database, error) {
	switch driver {
	case "memory":
		return NewMemoryDB(), nil
	case "postgres":
		// TODO: 实现 PostgreSQL 后端
		return nil, fmt.Errorf("postgres driver not yet implemented")
	case "libsql":
		// TODO: 实现 libSQL 后端
		return nil, fmt.Errorf("libsql driver not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported database driver: %q", driver)
	}
}
