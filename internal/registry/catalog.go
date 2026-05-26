package registry

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nearai/ironclaw-go/internal/extensions"
)

// Catalog 是扩展注册表目录，提供查询和搜索功能。
type Catalog struct {
	mu        sync.RWMutex
	manifests map[string]*ExtensionManifest // name -> manifest
	bundles   map[string]BundleDefinition
}

// NewCatalog 创建新的空目录。
func NewCatalog() *Catalog {
	return &Catalog{
		manifests: make(map[string]*ExtensionManifest),
		bundles:   make(map[string]BundleDefinition),
	}
}

// LoadFromDir 从目录加载所有 manifest。
func (c *Catalog) LoadFromDir(dir string) error {
	manifests, err := DiscoverManifests(dir)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range manifests {
		c.manifests[m.Name] = m
	}
	return nil
}

// LoadBundles 加载 bundles 文件。
func (c *Catalog) LoadBundles(path string) error {
	bf, err := LoadBundles(path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, def := range bf.Bundles {
		c.bundles[name] = def
	}
	return nil
}

// Register 手动注册一个 manifest。
func (c *Catalog) Register(m *ExtensionManifest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manifests[m.Name] = m
}

// Get 按名称获取 manifest。
func (c *Catalog) Get(name string) (*ExtensionManifest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.manifests[name]
	return m, ok
}

// List 返回所有 manifest。
func (c *Catalog) List() []*ExtensionManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*ExtensionManifest, 0, len(c.manifests))
	for _, m := range c.manifests {
		out = append(out, m)
	}
	return out
}

// ListByKind 按类型过滤。
func (c *Catalog) ListByKind(kind ManifestKind) []*ExtensionManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []*ExtensionManifest
	for _, m := range c.manifests {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// Search 按名称/关键词/描述搜索。
func (c *Catalog) Search(query string) []*ExtensionManifest {
	if query == "" {
		return c.List()
	}
	q := strings.ToLower(query)
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []*ExtensionManifest
	for _, m := range c.manifests {
		if m.Hidden {
			continue
		}
		if matchManifest(m, q) {
			out = append(out, m)
		}
	}
	return out
}

func matchManifest(m *ExtensionManifest, q string) bool {
	if strings.Contains(strings.ToLower(m.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(m.DisplayName), q) {
		return true
	}
	if strings.Contains(strings.ToLower(m.Description), q) {
		return true
	}
	for _, kw := range m.Keywords {
		if strings.Contains(strings.ToLower(kw), q) {
			return true
		}
	}
	return false
}

// GetBundle 按名称获取 bundle。
func (c *Catalog) GetBundle(name string) (BundleDefinition, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.bundles[name]
	return b, ok
}

// ListBundles 返回所有 bundle 名称。
func (c *Catalog) ListBundles() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.bundles))
	for name := range c.bundles {
		names = append(names, name)
	}
	return names
}

// BundleExtensions 返回 bundle 中包含的扩展清单列表。
func (c *Catalog) BundleExtensions(bundleName string) ([]*ExtensionManifest, error) {
	bundle, ok := c.GetBundle(bundleName)
	if !ok {
		return nil, fmt.Errorf("bundle %q not found", bundleName)
	}

	var out []*ExtensionManifest
	for _, ref := range bundle.Extensions {
		// ref 格式: "tools/<name>" 或 "channels/<name>"
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[1]
		if m, ok := c.Get(name); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// AllEntries 将目录中所有 manifest 转换为 RegistryEntry。
func (c *Catalog) AllEntries() ([]extensions.RegistryEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []extensions.RegistryEntry
	for _, m := range c.manifests {
		entry, err := m.ToRegistryEntry()
		if err != nil {
			continue
		}
		out = append(out, *entry)
	}
	return out, nil
}
