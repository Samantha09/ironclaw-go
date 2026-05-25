package channels

import "context"

// IncomingMessage — a message from an external channel.
type IncomingMessage struct {
	ID      string
	Channel string
	UserID  string
	Content string
}

// OutgoingResponse — a message sent back to a channel.
type OutgoingResponse struct {
	Content string
}

// Channel — a source of user input and a sink for agent output.
type Channel interface {
	Name() string
	Messages() <-chan IncomingMessage
	SendMessage(ctx context.Context, msg OutgoingResponse) error
}
