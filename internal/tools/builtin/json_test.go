package builtin

import (
	"context"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
)

func TestJSONToolPrettify(t *testing.T) {
	tool := NewJSONTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"input": `{"a":1,"b":2}`,
		"mode":  "prettify",
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": 2\n}"
	if out.Content != want {
		t.Errorf("content = %q, want %q", out.Content, want)
	}
}

func TestJSONToolMinify(t *testing.T) {
	tool := NewJSONTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"input": "{\n  \"a\": 1\n}",
		"mode":  "minify",
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := `{"a":1}`
	if out.Content != want {
		t.Errorf("content = %q, want %q", out.Content, want)
	}
}

func TestJSONToolInvalidInput(t *testing.T) {
	tool := NewJSONTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"input": "not json",
		"mode":  "prettify",
	}, nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONToolRequiresApproval(t *testing.T) {
	tool := NewJSONTool()
	if tool.RequiresApproval(nil) != gate.Never {
		t.Error("expected Never approval requirement")
	}
}
