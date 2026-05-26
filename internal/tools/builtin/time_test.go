package builtin

import (
	"context"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/gate"
)

func TestTimeTool(t *testing.T) {
	tool := NewTimeTool()
	if tool.Name() != "time" {
		t.Errorf("name = %q, want time", tool.Name())
	}

	out, err := tool.Execute(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Should parse as RFC3339
	_, err = time.Parse(time.RFC3339, out.Content)
	if err != nil {
		t.Errorf("output %q is not valid RFC3339: %v", out.Content, err)
	}
}

func TestTimeToolRequiresApproval(t *testing.T) {
	tool := NewTimeTool()
	if tool.RequiresApproval(nil) != gate.Never {
		t.Error("expected Never approval requirement")
	}
}
