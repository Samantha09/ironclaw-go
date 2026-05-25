package channels

import "context"

// IncomingMessage — 来自外部通道的消息。
type IncomingMessage struct {
	ID       string
	Channel  string
	UserID   string
	Content  string
	ThreadID string // 可选，用于指定回复线程
}

// OutgoingResponse — 发送到通道的响应。
type OutgoingResponse struct {
	Content  string
	ThreadID string // 可选，用于路由到特定线程
}

// Channel — 用户输入源和 Agent 输出汇。
type Channel interface {
	Name() string
	Messages() <-chan IncomingMessage
	SendMessage(ctx context.Context, msg OutgoingResponse) error
	Shutdown(ctx context.Context) error
}
