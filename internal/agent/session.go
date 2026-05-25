package agent

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/db"
)

// Thread 表示一个对话线程。
type Thread struct {
	ID        string
	UserID    string
	Channel   string
	Title     string
	Turns     []Turn
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Turn 表示对话中的一个轮次（用户输入 + Agent 响应 + 工具调用）。
type Turn struct {
	ID        string
	UserMsg   string
	AgentResp string
	ToolCalls []db.ToolCall
	CreatedAt time.Time
}

// SessionManager 管理用户会话和对话线程。
type SessionManager struct {
	mu     sync.RWMutex
	active map[string]*Thread // userID -> current thread
	history map[string][]*Thread // userID -> threads
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		active:  make(map[string]*Thread),
		history: make(map[string][]*Thread),
	}
}

// GetOrCreateThread 获取或创建用户的当前线程。
func (sm *SessionManager) GetOrCreateThread(userID, channel string) *Thread {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if thread, ok := sm.active[userID]; ok {
		return thread
	}

	thread := &Thread{
		ID:        uuid.New().String(),
		UserID:    userID,
		Channel:   channel,
		Title:     "新对话",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.active[userID] = thread
	sm.history[userID] = append(sm.history[userID], thread)
	return thread
}

// GetThread 获取指定线程。
func (sm *SessionManager) GetThread(userID, threadID string) *Thread {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, t := range sm.history[userID] {
		if t.ID == threadID {
			return t
		}
	}
	return nil
}

// AddTurn 向线程添加一个轮次。
func (sm *SessionManager) AddTurn(userID string, turn Turn) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	thread, ok := sm.active[userID]
	if !ok {
		return
	}

	if turn.ID == "" {
		turn.ID = uuid.New().String()
	}
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now()
	}
	thread.Turns = append(thread.Turns, turn)
	thread.UpdatedAt = time.Now()
}

// GetTurns 获取线程的所有轮次。
func (sm *SessionManager) GetTurns(userID string) []Turn {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	thread, ok := sm.active[userID]
	if !ok {
		return nil
	}
	return thread.Turns
}

// SwitchThread 切换用户的活跃线程。
func (sm *SessionManager) SwitchThread(userID, threadID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, t := range sm.history[userID] {
		if t.ID == threadID {
			sm.active[userID] = t
			return true
		}
	}
	return false
}

// ListThreads 列出用户的所有线程。
func (sm *SessionManager) ListThreads(userID string) []*Thread {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	threads := make([]*Thread, len(sm.history[userID]))
	copy(threads, sm.history[userID])
	return threads
}

// CompactThread 压缩线程历史（当上下文过长时）。
// MVP: 简单地丢弃旧的轮次，保留最近的 N 个。
func (sm *SessionManager) CompactThread(userID string, maxTurns int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	thread, ok := sm.active[userID]
	if !ok || len(thread.Turns) <= maxTurns {
		return
	}

	// 保留最近的 maxTurns 个轮次
	thread.Turns = thread.Turns[len(thread.Turns)-maxTurns:]
}
