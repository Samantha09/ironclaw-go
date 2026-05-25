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

// Repl — 从 stdin 读取，向 stdout 写入。
type Repl struct {
	userID   string
	msgChan  chan channels.IncomingMessage
	mu       sync.Mutex
	scanner  *bufio.Scanner
	done     chan struct{}
	shutdown chan struct{}
}

func New(userID string) *Repl {
	r := &Repl{
		userID:   userID,
		msgChan:  make(chan channels.IncomingMessage, 8),
		done:     make(chan struct{}),
		shutdown: make(chan struct{}),
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

func (r *Repl) Shutdown(_ context.Context) error {
	close(r.shutdown)
	return nil
}

func (r *Repl) readLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for {
		select {
		case <-r.shutdown:
			close(r.msgChan)
			return
		default:
		}

		if !scanner.Scan() {
			close(r.msgChan)
			return
		}

		line := scanner.Text()
		if line == "quit" || line == "exit" {
			close(r.msgChan)
			return
		}

		select {
		case r.msgChan <- channels.IncomingMessage{
			ID:      uuid.New().String(),
			Channel: r.Name(),
			UserID:  r.userID,
			Content: line,
		}:
		case <-r.shutdown:
			close(r.msgChan)
			return
		}
	}
}
