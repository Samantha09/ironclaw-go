package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFSWorkspace(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()
	ws := NewFSWorkspace(baseDir)
	userID := "test_user"

	t.Run("write_and_read", func(t *testing.T) {
		if err := ws.WriteFile(ctx, userID, "hello.txt", []byte("world")); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		data, err := ws.ReadFile(ctx, userID, "hello.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != "world" {
			t.Errorf("expected 'world', got %q", string(data))
		}
	})

	t.Run("nested_path", func(t *testing.T) {
		if err := ws.WriteFile(ctx, userID, "sub/dir/file.txt", []byte("nested")); err != nil {
			t.Fatalf("WriteFile nested failed: %v", err)
		}

		data, err := ws.ReadFile(ctx, userID, "sub/dir/file.txt")
		if err != nil {
			t.Fatalf("ReadFile nested failed: %v", err)
		}
		if string(data) != "nested" {
			t.Errorf("expected 'nested', got %q", string(data))
		}
	})

	t.Run("list_dir", func(t *testing.T) {
		entries, err := ws.ListDir(ctx, userID, "")
		if err != nil {
			t.Fatalf("ListDir failed: %v", err)
		}
		if len(entries) == 0 {
			t.Error("expected some entries")
		}
	})

	t.Run("mkdir", func(t *testing.T) {
		if err := ws.Mkdir(ctx, userID, "new_folder"); err != nil {
			t.Fatalf("Mkdir failed: %v", err)
		}
		info, err := os.Stat(filepath.Join(baseDir, userID, "new_folder"))
		if err != nil {
			t.Fatalf("stat new_folder failed: %v", err)
		}
		if !info.IsDir() {
			t.Error("new_folder is not a directory")
		}
	})

	t.Run("delete_file", func(t *testing.T) {
		if err := ws.DeleteFile(ctx, userID, "hello.txt"); err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}
		_, err := ws.ReadFile(ctx, userID, "hello.txt")
		if err == nil {
			t.Error("expected error after delete")
		}
	})

	t.Run("path_traversal_rejected", func(t *testing.T) {
		_, err := ws.ReadFile(ctx, userID, "../secret.txt")
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})

	t.Run("absolute_path_rejected", func(t *testing.T) {
		_, err := ws.ReadFile(ctx, userID, "/etc/passwd")
		if err == nil {
			t.Error("expected error for absolute path")
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		if err := ws.WriteFile(ctx, "user_a", "private.txt", []byte("secret")); err != nil {
			t.Fatalf("WriteFile user_a failed: %v", err)
		}

		_, err := ws.ReadFile(ctx, "user_b", "private.txt")
		if err == nil {
			t.Error("expected error reading other user's file")
		}
	})
}
