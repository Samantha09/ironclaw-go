// Package hooks 提供事件生命周期钩子系统。
package hooks

import (
	"context"
	"fmt"
	"sync"
)

// EventType 定义钩子事件类型。
type EventType string

const (
	// EventBeforeMessage 在 Agent 处理消息前触发。
	EventBeforeMessage EventType = "before:message"
	// EventAfterMessage 在 Agent 处理消息后触发。
	EventAfterMessage EventType = "after:message"
	// EventBeforeToolCall 在工具调用前触发。
	EventBeforeToolCall EventType = "before:tool_call"
	// EventAfterToolCall 在工具调用后触发。
	EventAfterToolCall EventType = "after:tool_call"
	// EventBeforeResponse 在响应发送到通道前触发。
	EventBeforeResponse EventType = "before:response"
)

// Event 是触发钩子的事件载体。
type Event struct {
	Type    EventType
	UserID  string
	Channel string
	Data    map[string]any
}

// Handler 是事件处理函数签名。
// 返回 error 可阻止后续处理（视事件类型而定）。
type Handler func(ctx context.Context, event Event) error

// Registry 管理事件类型到处理函数的映射。
type Registry struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

// NewRegistry 创建新的钩子注册表。
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[EventType][]Handler),
	}
}

// Register 为指定事件类型注册处理函数。
func (r *Registry) Register(eventType EventType, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[eventType] = append(r.handlers[eventType], handler)
}

// Trigger 触发指定事件，顺序调用所有已注册的处理函数。
// 如果某个处理函数返回错误，立即停止并返回该错误。
func (r *Registry) Trigger(ctx context.Context, event Event) error {
	r.mu.RLock()
	handlers := r.handlers[event.Type]
	r.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			return fmt.Errorf("hook %s: %w", event.Type, err)
		}
	}
	return nil
}

// HasHandlers 检查指定事件类型是否有已注册的处理函数。
func (r *Registry) HasHandlers(eventType EventType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[eventType]) > 0
}
