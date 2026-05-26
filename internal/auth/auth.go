// Package auth 提供身份认证与授权服务。
package auth

import (
	"context"
	"fmt"
	"sync"
)

// Authenticator 定义认证接口。
type Authenticator interface {
	// Authenticate 验证凭据并返回对应的用户 ID。
	// 若认证失败返回 error。
	Authenticate(ctx context.Context, credential string) (string, error)
}

// APIKeyAuth 是基于 API Key 的认证实现。
type APIKeyAuth struct {
	mu   sync.RWMutex
	keys map[string]string // api_key -> user_id
}

// NewAPIKeyAuth 创建新的 API Key 认证器。
func NewAPIKeyAuth(keys map[string]string) *APIKeyAuth {
	if keys == nil {
		keys = make(map[string]string)
	}
	return &APIKeyAuth{keys: keys}
}

// Authenticate 验证 API Key。
func (a *APIKeyAuth) Authenticate(_ context.Context, key string) (string, error) {
	a.mu.RLock()
	userID, ok := a.keys[key]
	a.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("invalid api key")
	}
	return userID, nil
}

// AddKey 动态添加 API Key。
func (a *APIKeyAuth) AddKey(key, userID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keys[key] = userID
}

// RemoveKey 移除 API Key。
func (a *APIKeyAuth) RemoveKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keys, key)
}

// NoAuth 是一个总是返回匿名用户的免认证实现。
type NoAuth struct{}

// NewNoAuth 创建免认证器。
func NewNoAuth() *NoAuth {
	return &NoAuth{}
}

// Authenticate 总是返回匿名用户。
func (n *NoAuth) Authenticate(_ context.Context, _ string) (string, error) {
	return "anonymous", nil
}
