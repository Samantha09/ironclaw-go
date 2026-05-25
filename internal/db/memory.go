package db

import (
	"context"
	"fmt"
	"sync"
)

// MemoryDB — ephemeral in-memory store for MVP.
type MemoryDB struct {
	mu            sync.RWMutex
	settings      map[string]map[string][]byte // user -> key -> value
	conversations map[string]*Conversation
}

func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		settings:      make(map[string]map[string][]byte),
		conversations: make(map[string]*Conversation),
	}
}

func (m *MemoryDB) GetSetting(_ context.Context, userID, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userSettings, ok := m.settings[userID]
	if !ok {
		return nil, fmt.Errorf("user %s not found", userID)
	}
	val, ok := userSettings[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found", key)
	}
	return val, nil
}

func (m *MemoryDB) SetSetting(_ context.Context, userID, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.settings[userID] == nil {
		m.settings[userID] = make(map[string][]byte)
	}
	m.settings[userID][key] = value
	return nil
}

func (m *MemoryDB) SaveConversation(_ context.Context, conv *Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
