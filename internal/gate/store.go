package gate

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PendingStore 管理暂停等待审批的门控状态。
type PendingStore struct {
	mu       sync.Mutex
	byKey    map[string]*PendingGate // key: threadID
	byReqID  map[string]string       // requestID -> key
	resolved map[string]*PendingGate // key: threadID，已批准但尚未执行
	consumed map[string]time.Time    // requestID -> 处理时间（用于幂等）
	ttl      time.Duration
}

// NewPendingStore 创建新的待处理门控存储。
func NewPendingStore() *PendingStore {
	return &PendingStore{
		byKey:    make(map[string]*PendingGate),
		byReqID:  make(map[string]string),
		resolved: make(map[string]*PendingGate),
		consumed: make(map[string]time.Time),
		ttl:      10 * time.Minute,
	}
}

// Create 为给定的工具调用创建新的待审批门控。
func (s *PendingStore) Create(userID, threadID, toolName string, params map[string]any, sourceChannel string) *PendingGate {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清理过期的 consumed 记录
	now := time.Now()
	for rid, t := range s.consumed {
		if now.Sub(t) > s.ttl {
			delete(s.consumed, rid)
		}
	}

	key := s.key(userID, threadID)
	reqID := uuid.New().String()

	pg := &PendingGate{
		RequestID:     reqID,
		GateName:      "approval",
		UserID:        userID,
		ThreadID:      threadID,
		ToolName:      toolName,
		Params:        params,
		Description:   DescribePending(toolName, params),
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.ttl),
		SourceChannel: sourceChannel,
	}

	s.byKey[key] = pg
	s.byReqID[reqID] = key
	return pg
}

// Get 按用户和线程 ID 获取待处理门控。
func (s *PendingStore) Get(userID, threadID string) (*PendingGate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(userID, threadID)
	pg, ok := s.byKey[key]
	if !ok {
		return nil, false
	}
	if pg.IsExpired() {
		delete(s.byKey, key)
		delete(s.byReqID, pg.RequestID)
		return nil, false
	}
	return pg, true
}

// GetByRequestID 按请求 ID 获取待处理门控。
func (s *PendingStore) GetByRequestID(reqID string) (*PendingGate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, ok := s.byReqID[reqID]
	if !ok {
		return nil, false
	}
	pg, ok := s.byKey[key]
	if !ok || pg.IsExpired() {
		if ok {
			delete(s.byKey, key)
		}
		delete(s.byReqID, reqID)
		return nil, false
	}
	return pg, true
}

// Resolve 审批并将门控移入已批准队列（供 Resume 时消费）。
// 幂等：若同一 requestID 已被批准或消费过，直接返回成功。
func (s *PendingStore) Resolve(userID, threadID, requestID string) (*PendingGate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(userID, threadID)

	// 幂等：已批准尚未执行
	if pg, ok := s.resolved[key]; ok && pg.RequestID == requestID {
		return pg, nil
	}
	// 幂等：已消费（执行完毕）—— 返回 nil 表示已处理，调用方可视作成功
	if _, ok := s.consumed[requestID]; ok {
		return nil, nil
	}

	pg, ok := s.byKey[key]
	if !ok {
		return nil, &GateError{Reason: "no pending gate for this thread"}
	}
	if pg.RequestID != requestID {
		return nil, &GateError{Reason: "request ID mismatch (stale approval)"}
	}
	if pg.IsExpired() {
		delete(s.byKey, key)
		delete(s.byReqID, requestID)
		return nil, &GateError{Reason: "pending gate has expired"}
	}

	delete(s.byKey, key)
	delete(s.byReqID, requestID)
	s.resolved[key] = pg
	return pg, nil
}

// Deny 拒绝并移除待处理门控。
func (s *PendingStore) Deny(userID, threadID, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 幂等：已消费
	if _, ok := s.consumed[requestID]; ok {
		return nil
	}

	key := s.key(userID, threadID)
	pg, ok := s.byKey[key]
	if !ok {
		return &GateError{Reason: "no pending gate for this thread"}
	}
	if pg.RequestID != requestID {
		return &GateError{Reason: "request ID mismatch (stale approval)"}
	}
	if pg.IsExpired() {
		delete(s.byKey, key)
		delete(s.byReqID, requestID)
		return &GateError{Reason: "pending gate has expired"}
	}

	delete(s.byKey, key)
	delete(s.byReqID, requestID)
	s.consumed[requestID] = time.Now()
	return nil
}

// ConsumeResolved 消费指定线程的已批准门控（Resume 时使用）。
func (s *PendingStore) ConsumeResolved(userID, threadID string) *PendingGate {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(userID, threadID)
	pg, ok := s.resolved[key]
	if !ok {
		return nil
	}
	delete(s.resolved, key)
	s.consumed[pg.RequestID] = time.Now()
	return pg
}

// ClearResolved 清除指定线程的已批准门控（用户发送新消息时丢弃旧执行）。
func (s *PendingStore) ClearResolved(userID, threadID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resolved, s.key(userID, threadID))
}

// List 返回所有未过期的待处理门控。
func (s *PendingStore) List() []*PendingGate {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var out []*PendingGate
	for key, pg := range s.byKey {
		if pg.IsExpired() || now.After(pg.ExpiresAt) {
			delete(s.byKey, key)
			delete(s.byReqID, pg.RequestID)
			continue
		}
		out = append(out, pg)
	}
	return out
}

// Count 返回待处理门控数量。
func (s *PendingStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byKey)
}

// HasPending 检查指定用户和线程是否有待处理门控。
func (s *PendingStore) HasPending(userID, threadID string) bool {
	_, ok := s.Get(userID, threadID)
	return ok
}

func (s *PendingStore) key(_, threadID string) string {
	return threadID
}

// ErrNotFound 表示未找到待处理门控。
var ErrNotFound = fmt.Errorf("gate: no pending gate found")
