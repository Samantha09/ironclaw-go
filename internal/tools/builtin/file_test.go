package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/workspace"
)

func TestFileToolReadWrite(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	// Write
	out, err := tool.Execute(ctx, map[string]any{"action": "write", "path": "test.txt", "content": "hello"}, jobCtx)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out.Content, "Wrote") {
		t.Errorf("write output = %q", out.Content)
	}

	// Read
	out, err = tool.Execute(ctx, map[string]any{"action": "read", "path": "test.txt"}, jobCtx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Content != "hello" {
		t.Errorf("read output = %q, want hello", out.Content)
	}
}

func TestFileToolList(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	tool.Execute(ctx, map[string]any{"action": "write", "path": "a.txt", "content": "a"}, jobCtx)
	tool.Execute(ctx, map[string]any{"action": "mkdir", "path": "sub"}, jobCtx)

	out, err := tool.Execute(ctx, map[string]any{"action": "list", "path": "."}, jobCtx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt") || !strings.Contains(out.Content, "sub") {
		t.Errorf("list output = %q", out.Content)
	}
}

func TestFileToolDelete(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	tool.Execute(ctx, map[string]any{"action": "write", "path": "del.txt", "content": "x"}, jobCtx)
	out, err := tool.Execute(ctx, map[string]any{"action": "delete", "path": "del.txt"}, jobCtx)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out.Content, "Deleted") {
		t.Errorf("delete output = %q", out.Content)
	}
}

func TestFileToolMkdir(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	out, err := tool.Execute(ctx, map[string]any{"action": "mkdir", "path": "newdir"}, jobCtx)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !strings.Contains(out.Content, "Created directory") {
		t.Errorf("mkdir output = %q", out.Content)
	}
}

func TestFileToolPathTraversal(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	_, err := tool.Execute(ctx, map[string]any{"action": "read", "path": "../etc/passwd"}, jobCtx)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestFileToolMissingPath(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	_, err := tool.Execute(ctx, map[string]any{"action": "read", "path": ""}, jobCtx)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestFileToolUnknownAction(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	_, err := tool.Execute(ctx, map[string]any{"action": "unknown", "path": "x"}, jobCtx)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestFileToolRequiresApproval(t *testing.T) {
	ws := workspace.NewFSWorkspace(t.TempDir())
	tool := NewFileTool(ws)
	if tool.RequiresApproval(map[string]any{"action": "read"}) != gate.Never {
		t.Error("expected Never for read")
	}
	if tool.RequiresApproval(map[string]any{"action": "write"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for write")
	}
	if tool.RequiresApproval(map[string]any{"action": "delete"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for delete")
	}
	if tool.RequiresApproval(map[string]any{"action": "mkdir"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for mkdir")
	}
}
