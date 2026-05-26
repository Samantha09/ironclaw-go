package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
)

func TestMemoryToolSetAndGet(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	// Set
	out, err := tool.Execute(ctx, map[string]any{"action": "set", "key": "name", "value": "Alice"}, jobCtx)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(out.Content, "Alice") {
		t.Errorf("set output = %q", out.Content)
	}

	// Get
	out, err = tool.Execute(ctx, map[string]any{"action": "get", "key": "name"}, jobCtx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Content != "Alice" {
		t.Errorf("get output = %q, want Alice", out.Content)
	}
}

func TestMemoryToolGetMissing(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	out, err := tool.Execute(ctx, map[string]any{"action": "get", "key": "missing"}, jobCtx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out.Content, "not found") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestMemoryToolDelete(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	tool.Execute(ctx, map[string]any{"action": "set", "key": "x", "value": "1"}, jobCtx)
	_, err := tool.Execute(ctx, map[string]any{"action": "delete", "key": "x"}, jobCtx)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	out, _ := tool.Execute(ctx, map[string]any{"action": "get", "key": "x"}, jobCtx)
	if !strings.Contains(out.Content, "not found") {
		t.Errorf("expected not found after delete, got %q", out.Content)
	}
}

func TestMemoryToolList(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	tool.Execute(ctx, map[string]any{"action": "set", "key": "a", "value": "1"}, jobCtx)
	tool.Execute(ctx, map[string]any{"action": "set", "key": "b", "value": "2"}, jobCtx)

	out, err := tool.Execute(ctx, map[string]any{"action": "list"}, jobCtx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.Content, "a = 1") || !strings.Contains(out.Content, "b = 2") {
		t.Errorf("list output = %q", out.Content)
	}
}

func TestMemoryToolListEmpty(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()
	jobCtx := &tools.JobContext{UserID: "user1"}

	out, err := tool.Execute(ctx, map[string]any{"action": "list"}, jobCtx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.Content, "empty") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestMemoryToolNamespaceIsolation(t *testing.T) {
	tool := NewMemoryTool()
	ctx := context.Background()

	tool.Execute(ctx, map[string]any{"action": "set", "key": "k", "value": "v1"}, &tools.JobContext{UserID: "user1"})
	tool.Execute(ctx, map[string]any{"action": "set", "key": "k", "value": "v2"}, &tools.JobContext{UserID: "user2"})

	out, _ := tool.Execute(ctx, map[string]any{"action": "get", "key": "k"}, &tools.JobContext{UserID: "user1"})
	if out.Content != "v1" {
		t.Errorf("user1 got %q, want v1", out.Content)
	}
}

func TestMemoryToolRequiresApproval(t *testing.T) {
	tool := NewMemoryTool()
	if tool.RequiresApproval(map[string]any{"action": "get"}) != gate.Never {
		t.Error("expected Never for get")
	}
	if tool.RequiresApproval(map[string]any{"action": "set"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for set")
	}
	if tool.RequiresApproval(map[string]any{"action": "delete"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for delete")
	}
}
