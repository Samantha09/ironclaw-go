package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/channels"
)

// Repl — reads from stdin, writes to stdout.
type Repl struct {
	userID  string
	msgChan chan channels.IncomingMessage
	mu      sync.Mutex
}

func New(userID string) *Repl {
	r := &Repl{
		userID:  userID,
		msgChan: make(chan channels.IncomingMessage, 8),
	}
	go r.readLoop()
	return r
}

func (r *Repl) Name() string { return "repl" }

func (r *Repl) Messages() <-chan channels.IncomingMessage {
	return r.msgChan
}

func (r *Repl) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Printf("\n[%s] %s\n> ", r.Name(), msg.Content)
	return nil
}

func (r *Repl) readLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		line := scanner.Text()
		if line == "quit" || line == "exit" {
			close(r.msgChan)
			return
		}
		r.msgChan <- channels.IncomingMessage{
			ID:      uuid.New().String(),
			Channel: r.Name(),
			UserID:  r.userID,
			Content: line,
		}
	}
}
