package channels

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Manager — 合并多个通道的消息流，支持动态增删。
type Manager struct {
	mu       sync.RWMutex
	channels map[string]Channel
	addCh    chan Channel        // 通知 mergeLoop 新通道
	injectCh chan IncomingMessage // 用于内部任务推送消息
	recvCh   chan IncomingMessage // 合并后的消息流
	stopCh   chan struct{}
	started  atomic.Bool
	wg       sync.WaitGroup
}

func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		addCh:    make(chan Channel, 8),
		injectCh: make(chan IncomingMessage, 64),
		recvCh:   make(chan IncomingMessage, 64),
		stopCh:   make(chan struct{}),
	}
}

// Start 启动消息合并循环（幂等，仅能调用一次）。
func (m *Manager) Start(ctx context.Context) {
	if !m.started.CompareAndSwap(false, true) {
		return
	}
	m.wg.Add(1)
	go m.mergeLoop(ctx)
}

// mergeLoop 常驻合并循环，支持运行时动态添加通道。
func (m *Manager) mergeLoop(ctx context.Context) {
	defer m.wg.Done()
	defer close(m.recvCh)

	merged := make(chan IncomingMessage, 64)
	var innerWg sync.WaitGroup
	active := make(map[string]bool)
	var activeMu sync.Mutex

	// 启动时合并已有通道
	m.mu.RLock()
	for _, ch := range m.channels {
		innerWg.Add(1)
		active[ch.Name()] = true
		go m.forwardChannel(ctx, ch.Name(), ch.Messages(), merged, &innerWg, active, &activeMu)
	}
	m.mu.RUnlock()

	// 也监听 injectCh
	innerWg.Add(1)
	go m.forwardInject(ctx, m.injectCh, merged, &innerWg)

	// 主循环：从 merged/addCh 转发到 recvCh
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
		case ch := <-m.addCh:
			name := ch.Name()
			activeMu.Lock()
			if active[name] {
				activeMu.Unlock()
				continue
			}
			active[name] = true
			activeMu.Unlock()
			innerWg.Add(1)
			go m.forwardChannel(ctx, name, ch.Messages(), merged, &innerWg, active, &activeMu)
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) forwardChannel(
	ctx context.Context,
	name string,
	src <-chan IncomingMessage,
	dst chan<- IncomingMessage,
	wg *sync.WaitGroup,
	active map[string]bool,
	activeMu *sync.Mutex,
) {
	defer wg.Done()
	defer func() {
		activeMu.Lock()
		delete(active, name)
		activeMu.Unlock()
	}()
	for {
		select {
		case msg, ok := <-src:
			if !ok {
				return
			}
			select {
			case dst <- msg:
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

func (m *Manager) forwardInject(
	ctx context.Context,
	src <-chan IncomingMessage,
	dst chan<- IncomingMessage,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	for {
		select {
		case msg, ok := <-src:
			if !ok {
				return
			}
			select {
			case dst <- msg:
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

// Add 动态注册一个通道。若 Manager 已启动，新通道会自动进入合并循环。
func (m *Manager) Add(ch Channel) {
	m.mu.Lock()
	m.channels[ch.Name()] = ch
	m.mu.Unlock()

	if m.started.Load() {
		select {
		case m.addCh <- ch:
		default:
			// addCh 满时丢弃，避免阻塞
		}
	}
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
