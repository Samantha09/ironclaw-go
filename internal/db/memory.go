package db

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryDB 是 MVP 使用的临时内存存储。
type MemoryDB struct {
	mu            sync.RWMutex
	settings      map[string]map[string]string // user -> key -> value
	conversations map[string]*Conversation
	messages      map[string][]*Message // threadID -> messages
	jobs          map[string]*Job
	actions       map[string][]*ActionRecord // jobID -> records
}

// NewMemoryDB 创建一个新的内存数据库实例。
func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		settings:      make(map[string]map[string]string),
		conversations: make(map[string]*Conversation),
		messages:      make(map[string][]*Message),
		jobs:          make(map[string]*Job),
		actions:       make(map[string][]*ActionRecord),
	}
}

// Ping 检查数据库连接（内存数据库始终可用）。
func (m *MemoryDB) Ping(_ context.Context) error {
	return nil
}

// Close 关闭数据库连接。
func (m *MemoryDB) Close() error {
	return nil
}

// Settings

func (m *MemoryDB) GetSetting(_ context.Context, userID, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userSettings, ok := m.settings[userID]
	if !ok {
		return "", fmt.Errorf("user %s not found", userID)
	}
	val, ok := userSettings[key]
	if !ok {
		return "", fmt.Errorf("key %s not found", key)
	}
	return val, nil
}

func (m *MemoryDB) SetSetting(_ context.Context, userID, key string, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.settings[userID] == nil {
		m.settings[userID] = make(map[string]string)
	}
	m.settings[userID][key] = value
	return nil
}

func (m *MemoryDB) DeleteSetting(_ context.Context, userID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	userSettings, ok := m.settings[userID]
	if !ok {
		return nil
	}
	delete(userSettings, key)
	return nil
}

// Conversations

func (m *MemoryDB) SaveConversation(_ context.Context, conv *Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = time.Now()
	}
	conv.UpdatedAt = time.Now()
	m.conversations[conv.ID] = conv
	return nil
}

func (m *MemoryDB) GetConversation(_ context.Context, id string) (*Conversation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conv, ok := m.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation %s not found", id)
	}
	return conv, nil
}

func (m *MemoryDB) ListConversations(_ context.Context, userID string, limit, offset int) ([]*Conversation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Conversation
	for _, conv := range m.conversations {
		if conv.UserID == userID {
			result = append(result, conv)
		}
	}

	// 简单分页
	if offset >= len(result) {
		return []*Conversation{}, nil
	}
	end := offset + limit
	if end > len(result) || limit <= 0 {
		end = len(result)
	}
	return result[offset:end], nil
}

func (m *MemoryDB) DeleteConversation(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.conversations, id)
	delete(m.messages, id)
	return nil
}

// Messages

func (m *MemoryDB) SaveMessage(_ context.Context, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	m.messages[msg.ThreadID] = append(m.messages[msg.ThreadID], msg)

	// 同时更新 conversation 的 UpdatedAt
	if conv, ok := m.conversations[msg.ThreadID]; ok {
		conv.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MemoryDB) GetMessagesByThread(_ context.Context, threadID string, limit, offset int) ([]*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs := m.messages[threadID]
	if offset >= len(msgs) {
		return []*Message{}, nil
	}
	end := offset + limit
	if end > len(msgs) || limit <= 0 {
		end = len(msgs)
	}
	return msgs[offset:end], nil
}

func (m *MemoryDB) DeleteMessagesByThread(_ context.Context, threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.messages, threadID)
	return nil
}

// Jobs

func (m *MemoryDB) SaveJob(_ context.Context, job *Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.UpdatedAt = time.Now()
	m.jobs[job.ID] = job
	return nil
}

func (m *MemoryDB) GetJob(_ context.Context, id string) (*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	return job, nil
}

func (m *MemoryDB) ListJobs(_ context.Context, userID string, limit, offset int) ([]*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Job
	for _, job := range m.jobs {
		if job.UserID == userID {
			result = append(result, job)
		}
	}

	if offset >= len(result) {
		return []*Job{}, nil
	}
	end := offset + limit
	if end > len(result) || limit <= 0 {
		end = len(result)
	}
	return result[offset:end], nil
}

func (m *MemoryDB) UpdateJobStatus(_ context.Context, id, status, output, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	job.Status = status
	job.Output = output
	job.Error = errMsg
	job.UpdatedAt = time.Now()
	return nil
}

func (m *MemoryDB) DeleteJob(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.jobs, id)
	delete(m.actions, id)
	return nil
}

// Action Records

func (m *MemoryDB) SaveActionRecord(_ context.Context, rec *ActionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	m.actions[rec.JobID] = append(m.actions[rec.JobID], rec)
	return nil
}

func (m *MemoryDB) ListActionRecordsByJob(_ context.Context, jobID string) ([]*ActionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	recs := m.actions[jobID]
	if recs == nil {
		return []*ActionRecord{}, nil
	}
	return recs, nil
}
