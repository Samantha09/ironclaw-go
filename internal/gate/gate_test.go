package gate

import (
	"context"
	"testing"
	"time"
)

func TestApprovalRequirementIsRequired(t *testing.T) {
	cases := []struct {
		req  ApprovalRequirement
		want bool
	}{
		{Never, false},
		{UnlessAutoApproved, true},
		{Always, true},
	}
	for _, tc := range cases {
		if got := tc.req.IsRequired(); got != tc.want {
			t.Errorf("%v.IsRequired() = %v, want %v", tc.req, got, tc.want)
		}
	}
}

func TestApprovalGateInteractive(t *testing.T) {
	gate := NewApprovalGate(func(toolName string) ApprovalRequirement {
		if toolName == "safe" {
			return Never
		}
		if toolName == "dangerous" {
			return Always
		}
		return UnlessAutoApproved
	})

	ctx := context.Background()

	// Never -> Allow
	gctx := &GateContext{ToolName: "safe", ExecutionMode: Interactive}
	if got := gate.Evaluate(ctx, gctx); got != Allow {
		t.Errorf("safe = %v, want Allow", got)
	}

	// Always -> Pause
	gctx = &GateContext{ToolName: "dangerous", ExecutionMode: Interactive}
	if got := gate.Evaluate(ctx, gctx); got != Pause {
		t.Errorf("dangerous = %v, want Pause", got)
	}

	// UnlessAutoApproved without auto-approve -> Pause
	gctx = &GateContext{ToolName: "http", ExecutionMode: Interactive}
	if got := gate.Evaluate(ctx, gctx); got != Pause {
		t.Errorf("http = %v, want Pause", got)
	}

	// UnlessAutoApproved with auto-approve -> Allow
	gctx = &GateContext{ToolName: "http", ExecutionMode: Interactive, AutoApproved: map[string]bool{"http": true}}
	if got := gate.Evaluate(ctx, gctx); got != Allow {
		t.Errorf("http auto-approved = %v, want Allow", got)
	}
}

func TestApprovalGateAutonomous(t *testing.T) {
	gate := NewApprovalGate(func(toolName string) ApprovalRequirement {
		if toolName == "safe" {
			return Never
		}
		return UnlessAutoApproved
	})

	ctx := context.Background()

	// Never -> Allow
	gctx := &GateContext{ToolName: "safe", ExecutionMode: Autonomous}
	if got := gate.Evaluate(ctx, gctx); got != Allow {
		t.Errorf("safe autonomous = %v, want Allow", got)
	}

	// UnlessAutoApproved without auto-approve -> Deny
	gctx = &GateContext{ToolName: "shell", ExecutionMode: Autonomous}
	if got := gate.Evaluate(ctx, gctx); got != Deny {
		t.Errorf("shell autonomous = %v, want Deny", got)
	}

	// UnlessAutoApproved with auto-approve -> Allow
	gctx = &GateContext{ToolName: "shell", ExecutionMode: Autonomous, AutoApproved: map[string]bool{"shell": true}}
	if got := gate.Evaluate(ctx, gctx); got != Allow {
		t.Errorf("shell auto-approved autonomous = %v, want Allow", got)
	}

	// Always -> Deny even if auto-approved
	gateAlways := NewApprovalGate(func(_ string) ApprovalRequirement { return Always })
	gctx = &GateContext{ToolName: "x", ExecutionMode: Autonomous, AutoApproved: map[string]bool{"x": true}}
	if got := gateAlways.Evaluate(ctx, gctx); got != Deny {
		t.Errorf("Always autonomous = %v, want Deny", got)
	}
}

func TestDefaultToolRequirement(t *testing.T) {
	cases := []struct {
		tool   string
		params map[string]any
		want   ApprovalRequirement
	}{
		{"echo", nil, Never},
		{"time", nil, Never},
		{"json", nil, Never},
		{"memory", map[string]any{"action": "get"}, Never},
		{"memory", map[string]any{"action": "set"}, UnlessAutoApproved},
		{"shell", nil, UnlessAutoApproved},
		{"file", map[string]any{"action": "read"}, Never},
		{"file", map[string]any{"action": "write"}, UnlessAutoApproved},
		{"file", map[string]any{"action": "delete"}, UnlessAutoApproved},
		{"http", map[string]any{"method": "GET"}, Never},
		{"http", map[string]any{"method": "POST"}, UnlessAutoApproved},
		{"unknown", nil, UnlessAutoApproved},
	}
	for _, tc := range cases {
		got := DefaultToolRequirement(tc.tool, tc.params)
		if got != tc.want {
			t.Errorf("DefaultToolRequirement(%q, %v) = %v, want %v", tc.tool, tc.params, got, tc.want)
		}
	}
}

func TestPendingGateExpired(t *testing.T) {
	pg := &PendingGate{
		CreatedAt: time.Now().Add(-20 * time.Minute),
		ExpiresAt: time.Now().Add(-10 * time.Minute),
	}
	if !pg.IsExpired() {
		t.Error("expected expired")
	}

	pg2 := &PendingGate{
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if pg2.IsExpired() {
		t.Error("expected not expired")
	}
}

func TestGateError(t *testing.T) {
	err := &GateError{Reason: "test error"}
	if err.Error() != "gate: test error" {
		t.Errorf("error message = %q", err.Error())
	}
}
