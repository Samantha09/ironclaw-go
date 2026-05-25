package db

import "context"

// Conversation — a thread of messages.
type Conversation struct {
	ID      string
	UserID  string
	Channel string
	Title   string
}

// Database — backend-agnostic persistence.
type Database interface {
	// Settings
	GetSetting(ctx context.Context, userID, key string) ([]byte, error)
	SetSetting(ctx context.Context, userID, key string, value []byte) error

	// Conversations
	SaveConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
}
