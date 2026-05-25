// Package extensions 管理扩展（WASM 工具与通道）生命周期。
package extensions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ExtensionType 表示扩展类型。
type ExtensionType string

const (
	TypeTool    ExtensionType = "tool"
	TypeChannel ExtensionType = "channel"
)

// Extension 表示一个已安装的扩展。
type Extension struct {
	Name        string
	Type        ExtensionType
	Version     string
	Path        string // WASM 文件路径
	ConfigPath  string // 配置文件路径
	Active      bool
}

// Manager 管理扩展的生命周期。
type Manager struct {
	mu         sync.RWMutex
	extensions map[string]*Extension
	registryDir string
}

// NewManager 创建新的扩展管理器。
func NewManager(registryDir string) *Manager {
	return &Manager{
		extensions:  make(map[string]*Extension),
		registryDir: registryDir,
	}
}

// Discover 扫描注册表目录发现扩展。
func (m *Manager) Discover() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, extType := range []ExtensionType{TypeTool, TypeChannel} {
		dir := filepath.Join(m.registryDir, string(extType)+"s")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // 目录可能不存在
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			wasmPath := filepath.Join(dir, name, name+".wasm")
			configPath := filepath.Join(dir, name, name+".json")

			if _, err := os.Stat(wasmPath); err == nil {
				m.extensions[name] = &Extension{
					Name:       name,
					Type:       extType,
					Path:       wasmPath,
					ConfigPath: configPath,
					Active:     false,
				}
			}
		}
	}
	return nil
}

// Get 获取指定扩展。
func (m *Manager) Get(name string) (*Extension, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ext, ok := m.extensions[name]
	return ext, ok
}

// List 列出所有扩展。
func (m *Manager) List() []*Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Extension, 0, len(m.extensions))
	for _, ext := range m.extensions {
		result = append(result, ext)
	}
	return result
}

// ListByType 按类型列出扩展。
func (m *Manager) ListByType(extType ExtensionType) []*Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Extension
	for _, ext := range m.extensions {
		if ext.Type == extType {
			result = append(result, ext)
		}
	}
	return result
}

// Activate 激活扩展。
func (m *Manager) Activate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ext, ok := m.extensions[name]
	if !ok {
		return fmt.Errorf("extension %q not found", name)
	}
	ext.Active = true
	return nil
}

// Deactivate 停用扩展。
func (m *Manager) Deactivate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ext, ok := m.extensions[name]
	if !ok {
		return fmt.Errorf("extension %q not found", name)
	}
	ext.Active = false
	return nil
}

// Install 安装扩展（将 WASM 模块复制到注册表）。
func (m *Manager) Install(ctx context.Context, name string, extType ExtensionType, wasmData []byte, config []byte) error {
	_ = ctx
	destDir := filepath.Join(m.registryDir, string(extType)+"s", name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	wasmPath := filepath.Join(destDir, name+".wasm")
	if err := os.WriteFile(wasmPath, wasmData, 0644); err != nil {
		return fmt.Errorf("write wasm: %w", err)
	}

	if config != nil {
		configPath := filepath.Join(destDir, name+".json")
		if err := os.WriteFile(configPath, config, 0644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	m.mu.Lock()
	m.extensions[name] = &Extension{
		Name:       name,
		Type:       extType,
		Path:       wasmPath,
		ConfigPath: filepath.Join(destDir, name+".json"),
		Active:     false,
	}
	m.mu.Unlock()

	return nil
}

// Uninstall 卸载扩展。
func (m *Manager) Uninstall(name string) error {
	m.mu.Lock()
	ext, ok := m.extensions[name]
	delete(m.extensions, name)
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("extension %q not found", name)
	}

	destDir := filepath.Join(m.registryDir, string(ext.Type)+"s", name)
	return os.RemoveAll(destDir)
}

// ReadWASM 读取扩展的 WASM 模块内容。
func (m *Manager) ReadWASM(name string) ([]byte, error) {
	m.mu.RLock()
	ext, ok := m.extensions[name]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("extension %q not found", name)
	}
	return os.ReadFile(ext.Path)
}
