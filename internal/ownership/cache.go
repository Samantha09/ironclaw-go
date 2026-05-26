package ownership

import (
	"fmt"
	"sync"
)

// CacheEntry 是缓存中的所有权的条目。
type CacheEntry struct {
	UserID       *UserID
	ExternalID   string
	Channel      string
}

// Cache 是内存中的所有权查询缓存。
type Cache struct {
	mu    sync.RWMutex
	byKey map[string]*CacheEntry // key: channel+"/"+external_id
}

// NewCache 创建新的所有权缓存。
func NewCache() *Cache {
	return &Cache{
		byKey: make(map[string]*CacheEntry),
	}
}

// Get 按通道和外部 ID 查找缓存的用户。
func (c *Cache) Get(channel, externalID string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.byKey[c.key(channel, externalID)]
	return entry, ok
}

// Set 将用户与通道+外部 ID 关联存入缓存。
func (c *Cache) Set(channel, externalID string, userID *UserID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byKey[c.key(channel, externalID)] = &CacheEntry{
		UserID:     userID,
		ExternalID: externalID,
		Channel:    channel,
	}
}

// Remove 从缓存中移除条目。
func (c *Cache) Remove(channel, externalID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byKey, c.key(channel, externalID))
}

// Count 返回缓存条目数。
func (c *Cache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byKey)
}

func (c *Cache) key(channel, externalID string) string {
	return fmt.Sprintf("%s/%s", channel, externalID)
}
