package httpgw

import (
	"sync"

	"github.com/nearai/ironclaw-go/internal/channels"
)

// EventHub 管理 SSE 客户端订阅，按 userID + threadID 过滤推送。
type EventHub struct {
	mu   sync.RWMutex
	subs map[string][]chan channels.Event // key: "userID|threadID" 或 "userID|"
}

// NewEventHub 创建新的事件中心。
func NewEventHub() *EventHub {
	return &EventHub{
		subs: make(map[string][]chan channels.Event),
	}
}

// Subscribe 注册一个事件接收通道。
// threadID 为空字符串时表示订阅该用户的所有线程事件。
func (h *EventHub) Subscribe(userID, threadID string) <-chan channels.Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan channels.Event, 16)
	key := subKey(userID, threadID)
	h.subs[key] = append(h.subs[key], ch)
	return ch
}

// Unsubscribe 注销指定通道。
func (h *EventHub) Unsubscribe(userID, threadID string, ch <-chan channels.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := subKey(userID, threadID)
	list := h.subs[key]
	for i, c := range list {
		if c == ch {
			h.subs[key] = append(list[:i], list[i+1:]...)
			close(c)
			break
		}
	}
	if len(h.subs[key]) == 0 {
		delete(h.subs, key)
	}
}

// Publish 将事件推送给所有匹配的订阅者。
func (h *EventHub) Publish(ev channels.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 精确匹配 userID+threadID
	if ev.ThreadID != "" {
		key := subKey(ev.UserID, ev.ThreadID)
		for _, ch := range h.subs[key] {
			select {
			case ch <- ev:
			default:
			}
		}
	}

	// 广播给该 userID 的所有线程订阅者
	allKey := subKey(ev.UserID, "")
	for _, ch := range h.subs[allKey] {
		select {
		case ch <- ev:
		default:
		}
	}
}

func subKey(userID, threadID string) string {
	if threadID == "" {
		return userID + "|"
	}
	return userID + "|" + threadID
}
