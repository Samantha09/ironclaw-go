package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
)

func TestShellToolEcho(t *testing.T) {
	tool := NewShellTool()
	out, err := tool.Execute(context.Background(), map[string]any{"command": "echo hello"}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestShellToolInvalidParam(t *testing.T) {
	tool := NewShellTool()
	_, err := tool.Execute(context.Background(), map[string]any{}, nil)
	if err == nil {
		t.Error("expected error for missing command")
	}
}

func TestShellToolDangerousBlocked(t *testing.T) {
	tool := NewShellTool()
	cases := []string{
		"rm -rf /",
		":(){ :|:& };:",
		"dd if=/dev/zero of=/dev/sda",
	}
	for _, cmd := range cases {
		_, err := tool.Execute(context.Background(), map[string]any{"command": cmd}, nil)
		if err == nil {
			t.Errorf("expected dangerous command %q to be blocked", cmd)
		}
	}
}

func TestShellToolTimeout(t *testing.T) {
	tool := NewShellTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 5",
		"timeout": 1,
	}, nil)
	if err == nil && !strings.Contains(out.Content, "Error") {
		t.Errorf("expected timeout error, got %q", out.Content)
	}
}

func TestShellToolRequiresApproval(t *testing.T) {
	tool := NewShellTool()
	if tool.RequiresApproval(nil) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved")
	}
}

func TestShellToolTruncation(t *testing.T) {
	tool := NewShellTool()
	// Generate output longer than maxOutputLen (10000)
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "python3 -c \"print('x'*20000)\"",
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.Content, "truncated") {
		t.Errorf("expected truncated output")
	}
}

func TestShellToolJobContext(t *testing.T) {
	tool := NewShellTool()
	out, err := tool.Execute(context.Background(), map[string]any{"command": "pwd"}, &tools.JobContext{UserID: "u1"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Content == "" {
		t.Error("expected non-empty output")
	}
}
