package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// TokenStore 管理每作业的 bearer token（内存中）。
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string // jobID -> token
}

// NewTokenStore 创建新的 token 存储。
func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string]string),
	}
}

// Generate 为作业生成新的随机 token。
func (s *TokenStore) Generate(jobID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	s.tokens[jobID] = token
	return token
}

// Get 获取作业的 token。
func (s *TokenStore) Get(jobID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.tokens[jobID]
	return tok, ok
}

// Validate 验证作业 token 是否匹配。
func (s *TokenStore) Validate(jobID, token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expected, ok := s.tokens[jobID]
	return ok && expected == token
}

// Revoke 撤销作业的 token。
func (s *TokenStore) Revoke(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, jobID)
}
