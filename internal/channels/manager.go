package channels

import (
	"context"
	"fmt"
	"sync"
)

// Manager — 合并多个通道的消息流，支持动态增删。
type Manager struct {
	mu       sync.RWMutex
	channels map[string]Channel
	injectCh chan IncomingMessage // 用于内部任务推送消息
	recvCh   chan IncomingMessage // 合并后的消息流
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		injectCh: make(chan IncomingMessage, 64),
		recvCh:   make(chan IncomingMessage, 64),
		stopCh:   make(chan struct{}),
	}
}

// Start 启动消息合并循环（在 Run 之前调用）。
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.mergeLoop(ctx)
}

// mergeLoop 常驻合并循环。
func (m *Manager) mergeLoop(ctx context.Context) {
	defer m.wg.Done()
	defer close(m.recvCh)

	// merged 是所有通道消息的中转站
	merged := make(chan IncomingMessage, 64)
	var innerWg sync.WaitGroup
	var innerMu sync.Mutex
	active := make(map[string]bool)

	// 启动时合并已有通道
	m.mu.RLock()
	for _, ch := range m.channels {
		innerWg.Add(1)
		active[ch.Name()] = true
		go func(name string, ch <-chan IncomingMessage) {
			defer innerWg.Done()
			defer func() {
				innerMu.Lock()
				delete(active, name)
				innerMu.Unlock()
			}()
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						return
					}
					select {
					case merged <- msg:
					case <-ctx.Done():
						return
					case <-m.stopCh:
						return
					}
				case <-ctx.Done():
					return
				case <-m.stopCh:
					return
				}
			}
		}(ch.Name(), ch.Messages())
	}
	m.mu.RUnlock()

	// 也监听 injectCh
	innerWg.Add(1)
	go func() {
		defer innerWg.Done()
		for {
			select {
			case msg, ok := <-m.injectCh:
				if !ok {
					return
				}
				select {
				case merged <- msg:
				case <-ctx.Done():
					return
				case <-m.stopCh:
					return
				}
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			}
		}
	}()

	// 将 merged 中的消息转发到 recvCh
	for {
		select {
		case msg := <-merged:
			select {
			case m.recvCh <- msg:
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			}
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

// Add 动态注册一个通道。
func (m *Manager) Add(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.Name()] = ch
}

// Remove 动态注销一个通道并关闭它。
func (m *Manager) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	ch, ok := m.channels[name]
	delete(m.channels, name)
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("channel %q not found", name)
	}
	return ch.Shutdown(ctx)
}

// Get 获取指定名称的通道。
func (m *Manager) Get(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// Names 返回所有已注册通道的名称。
func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// Inject 允许内部组件向消息流中注入消息。
func (m *Manager) Inject(msg IncomingMessage) {
	select {
	case m.injectCh <- msg:
	default:
		// 通道满时丢弃，避免阻塞
	}
}

// Receive 从合并后的消息流中接收消息。
func (m *Manager) Receive(ctx context.Context) (IncomingMessage, error) {
	select {
	case msg := <-m.recvCh:
		return msg, nil
	case <-ctx.Done():
		return IncomingMessage{}, ctx.Err()
	}
}

// Send 向指定通道发送响应。
func (m *Manager) Send(ctx context.Context, channelName string, msg OutgoingResponse) error {
	m.mu.RLock()
	ch, ok := m.channels[channelName]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("channel %q not found", channelName)
	}
	return ch.SendMessage(ctx, msg)
}

// Broadcast 向所有通道广播响应。
func (m *Manager) Broadcast(ctx context.Context, msg OutgoingResponse) error {
	m.mu.RLock()
	chs := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		chs = append(chs, ch)
	}
	m.mu.RUnlock()

	var firstErr error
	for _, ch := range chs {
		if err := ch.SendMessage(ctx, msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ShutdownAll 关闭所有通道。
func (m *Manager) ShutdownAll(ctx context.Context) error {
	close(m.stopCh)

	m.mu.Lock()
	chs := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		chs = append(chs, ch)
	}
	m.channels = make(map[string]Channel)
	m.mu.Unlock()

	var firstErr error
	for _, ch := range chs {
		if err := ch.Shutdown(ctx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	close(m.injectCh)
	m.wg.Wait()
	return firstErr
}
