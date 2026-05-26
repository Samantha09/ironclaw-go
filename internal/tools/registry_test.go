package tools

import (
	"context"
	"testing"

	"github.com/nearai/ironclaw-go/internal/safety"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Register
	tool := &mockTool{name: "test_tool"}
	reg.Register(tool)

	if reg.Count() != 1 {
		t.Errorf("expected count 1, got %d", reg.Count())
	}

	// Get
	got, ok := reg.Get("test_tool")
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected test_tool, got %q", got.Name())
	}

	// List
	names := reg.List()
	if len(names) != 1 || names[0] != "test_tool" {
		t.Errorf("expected [test_tool], got %v", names)
	}

	// Get missing
	_, ok = reg.Get("missing")
	if ok {
		t.Error("expected missing tool to not be found")
	}
}

func TestDispatcher(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{name: "echo"})

	disp := NewDispatcher(reg, safety.NewLayer(), nil)

	ctx := context.Background()
	out, err := disp.Dispatch(ctx, "echo", map[string]any{"message": "hello"}, &JobContext{UserID: "user1"})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if out.Content != "hello" {
		t.Errorf("expected hello, got %q", out.Content)
	}

	// Missing tool
	_, err = disp.Dispatch(ctx, "missing", nil, &JobContext{UserID: "user1"})
	if err == nil {
		t.Error("expected error for missing tool")
	}
}

type mockTool struct {
	name string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "mock tool" }
func (m *mockTool) ParameterSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (m *mockTool) Execute(_ context.Context, params map[string]any, _ *JobContext) (ToolOutput, error) {
	msg, _ := params["message"].(string)
	return ToolOutput{Content: msg}, nil
}
