// Package workspace 提供用户工作区管理与文件操作抽象。
package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirEntry 表示工作区目录中的一个条目。
type DirEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// Workspace 定义用户隔离的文件操作接口。
type Workspace interface {
	// ReadFile 读取工作区中的文件内容。
	ReadFile(ctx context.Context, userID, path string) ([]byte, error)

	// WriteFile 向工作区写入文件。
	WriteFile(ctx context.Context, userID, path string, data []byte) error

	// ListDir 列出工作区目录内容。
	ListDir(ctx context.Context, userID, path string) ([]DirEntry, error)

	// DeleteFile 删除工作区中的文件或空目录。
	DeleteFile(ctx context.Context, userID, path string) error

	// Mkdir 在工作区中创建目录。
	Mkdir(ctx context.Context, userID, path string) error
}

// FSWorkspace 是基于本地文件系统的 Workspace 实现。
// 每个用户在 baseDir 下拥有一个独立子目录。
type FSWorkspace struct {
	baseDir string
	maxSize int64
}

// NewFSWorkspace 创建新的基于文件系统的工作区管理器。
func NewFSWorkspace(baseDir string) *FSWorkspace {
	return &FSWorkspace{
		baseDir: baseDir,
		maxSize: 10 * 1024 * 1024, // 10MB
	}
}

// userDir 返回指定用户的根目录路径。
func (f *FSWorkspace) userDir(userID string) string {
	return filepath.Join(f.baseDir, userID)
}

// resolvePath 将用户相对路径解析为绝对路径，并进行安全检查。
func (f *FSWorkspace) resolvePath(userID, relPath string) (string, error) {
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("path contains forbidden '..' segment: %q", relPath)
	}

	userRoot := f.userDir(userID)
	absPath := filepath.Join(userRoot, relPath)
	absPath = filepath.Clean(absPath)

	// 确保解析后的路径仍在用户目录内
	if !strings.HasPrefix(absPath, userRoot) {
		return "", fmt.Errorf("path escapes user workspace: %q", relPath)
	}

	return absPath, nil
}

// ensureUserDir 确保用户根目录存在。
func (f *FSWorkspace) ensureUserDir(userID string) error {
	return os.MkdirAll(f.userDir(userID), 0755)
}

// ReadFile 读取工作区文件。
func (f *FSWorkspace) ReadFile(_ context.Context, userID, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := f.resolvePath(userID, path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %q", path)
	}
	if info.Size() > f.maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), f.maxSize)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// WriteFile 向工作区写入文件。
func (f *FSWorkspace) WriteFile(_ context.Context, userID, path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := f.resolvePath(userID, path)
	if err != nil {
		return err
	}

	if err := f.ensureUserDir(userID); err != nil {
		return fmt.Errorf("ensure user dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// ListDir 列出工作区目录内容。
func (f *FSWorkspace) ListDir(_ context.Context, userID, path string) ([]DirEntry, error) {
	absPath, err := f.resolvePath(userID, path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var result []DirEntry
	for _, e := range entries {
		info, err := e.Info()
		var size int64
		if err == nil {
			size = info.Size()
		}
		result = append(result, DirEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  size,
		})
	}
	return result, nil
}

// DeleteFile 删除工作区中的文件或空目录。
func (f *FSWorkspace) DeleteFile(_ context.Context, userID, path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := f.resolvePath(userID, path)
	if err != nil {
		return err
	}

	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

// Mkdir 在工作区中创建目录。
func (f *FSWorkspace) Mkdir(_ context.Context, userID, path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := f.resolvePath(userID, path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return nil
}
