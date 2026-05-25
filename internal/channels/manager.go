package channels

import (
	"context"
	"sync"
)

// Manager — merges message streams from multiple channels.
type Manager struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

func NewManager() *Manager {
	return &Manager{channels: make(map[string]Channel)}
}

func (m *Manager) Add(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.Name()] = ch
}

func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// Receive blocks until a message arrives from any channel.
func (m *Manager) Receive(ctx context.Context) (IncomingMessage, error) {
	m.mu.RLock()
	chs := make([]<-chan IncomingMessage, 0, len(m.channels))
	for _, ch := range m.channels {
		chs = append(chs, ch.Messages())
	}
	m.mu.RUnlock()

	merged := make(chan IncomingMessage)
	var wg sync.WaitGroup
	for _, c := range chs {
		wg.Add(1)
		go func(ch <-chan IncomingMessage) {
			defer wg.Done()
			for msg := range ch {
				select {
				case merged <- msg:
				case <-ctx.Done():
					return
				}
			}
		}(c)
	}
	go func() { wg.Wait(); close(merged) }()

	select {
	case msg := <-merged:
		return msg, nil
	case <-ctx.Done():
		return IncomingMessage{}, ctx.Err()
	}
}

// Broadcast sends a response to all channels.
func (m *Manager) Broadcast(ctx context.Context, msg OutgoingResponse) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.channels {
		_ = ch.SendMessage(ctx, msg)
	}
	return nil
}
