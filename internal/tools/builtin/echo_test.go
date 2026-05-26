package builtin

import (
	"context"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
)

func TestEchoTool(t *testing.T) {
	tool := NewEchoTool()
	if tool.Name() != "echo" {
		t.Errorf("name = %q, want echo", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]any{"message": "hello"}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Content != "hello" {
		t.Errorf("content = %q, want hello", out.Content)
	}
}

func TestEchoToolMissingParam(t *testing.T) {
	tool := NewEchoTool()
	_, err := tool.Execute(context.Background(), map[string]any{}, nil)
	if err == nil {
		t.Error("expected error for missing message")
	}
}

func TestEchoToolRequiresApproval(t *testing.T) {
	tool := NewEchoTool()
	if tool.RequiresApproval(nil) != gate.Never {
		t.Error("expected Never approval requirement")
	}
}

func TestEchoToolSchema(t *testing.T) {
	tool := NewEchoTool()
	schema := tool.ParameterSchema()
	if schema["type"] != "object" {
		t.Errorf("schema type = %v", schema["type"])
	}
}
