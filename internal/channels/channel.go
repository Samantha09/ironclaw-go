package channels

import "context"

// AttachmentKind 表示附件内容的类型。
type AttachmentKind int

const (
	AttachmentKindAudio AttachmentKind = iota
	AttachmentKindImage
	AttachmentKindDocument
)

// Attachment 是传入消息上的单个文件或媒体附件。
type Attachment struct {
	ID            string
	Kind          AttachmentKind
	MIMEType      string
	Filename      string
	SizeBytes     int64
	SourceURL     string
	StorageKey    string
	LocalPath     string
	ExtractedText string
	Data          []byte
	DurationSecs  int
}

// IncomingMessage — 来自外部通道的消息。
type IncomingMessage struct {
	ID          string
	Channel     string
	UserID      string
	Content     string
	ThreadID    string      // 可选，用于指定回复线程
	Attachments []Attachment // 可选，消息携带的附件
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
