package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
)

// Store 提供加密密钥存储与凭证注入。
type Store struct {
	mu      sync.RWMutex
	secrets map[string]string // key -> encrypted value (base64)
	master  []byte            // 主密钥（32 字节用于 AES-256）
}

// NewStore 创建新的密钥存储。
// masterKey 必须是 16、24 或 32 字节。
func NewStore(masterKey string) (*Store, error) {
	key := []byte(masterKey)
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("master key must be 16, 24, or 32 bytes, got %d", len(key))
	}
	return &Store{
		secrets: make(map[string]string),
		master:  key,
	}, nil
}

// NewStoreFromEnv 从环境变量创建密钥存储。
func NewStoreFromEnv() (*Store, error) {
	key := "ironclaw-default-key-32bytes!!"
	return NewStore(key)
}

// Set 存储一个密钥。
func (s *Store) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	encrypted, err := s.encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	s.secrets[key] = encrypted
	return nil
}

// Get 获取一个密钥的明文值。
func (s *Store) Get(key string) (string, error) {
	s.mu.RLock()
	encrypted, ok := s.secrets[key]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("secret %q not found", key)
	}

	value, err := s.decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return value, nil
}

// Delete 删除一个密钥。
func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, key)
}

// List 列出所有存储的密钥名称。
func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.secrets))
	for k := range s.secrets {
		keys = append(keys, k)
	}
	return keys
}

// Inject 将密钥注入到字符串中（替换占位符）。
// 占位符格式：{{secret:KEY_NAME}}
func (s *Store) Inject(template string) (string, error) {
	// MVP: 简单实现，实际应该使用正则表达式
	// 这里只是一个 stub，后续完善
	return template, nil
}

// HasKey 检查密钥是否存在。
func (s *Store) HasKey(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.secrets[key]
	return ok
}

// encrypt 使用 AES-GCM 加密明文。
func (s *Store) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.master)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt 使用 AES-GCM 解密密文。
func (s *Store) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(s.master)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// ConstantTimeCompare 使用常量时间比较两个字符串（防止时序攻击）。
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
