package extensions

import (
	"sync"
)

// EntryRegistry 是内存中的扩展注册表索引。
type EntryRegistry struct {
	mu      sync.RWMutex
	entries map[string]RegistryEntry // name -> entry
}

// NewEntryRegistry 创建新的注册表。
func NewEntryRegistry() *EntryRegistry {
	return &EntryRegistry{entries: make(map[string]RegistryEntry)}
}

// Register 注册条目。
func (r *EntryRegistry) Register(e RegistryEntry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.Name] = e
	return nil
}

// Get 按名称获取条目。
func (r *EntryRegistry) Get(name string) (RegistryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

// List 返回所有条目。
func (r *EntryRegistry) List() []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// Search 按关键词搜索条目。
func (r *EntryRegistry) Search(query string) []SearchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []SearchResult
	for _, e := range r.entries {
		if e.Hidden {
			continue
		}
		if matchEntry(e, query) {
			results = append(results, SearchResult{
				RegistryEntry: e,
				Source:        ResultSourceRegistry,
				Validated:     true,
			})
		}
	}
	return results
}

// ListByKind 按类型过滤条目。
func (r *EntryRegistry) ListByKind(kind ExtensionKind) []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []RegistryEntry
	for _, e := range r.entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func matchEntry(e RegistryEntry, query string) bool {
	if query == "" {
		return true
	}
	// 简单子串匹配
	// 实际生产环境可用更复杂的搜索
	return true
}
