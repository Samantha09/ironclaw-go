package sanitizer

import (
	"testing"

	"github.com/nearai/ironclaw-go/internal/safety/policy"
)

func TestOutputTypes(t *testing.T) {
	out := Output{
		Content:     "test",
		Warnings:    []Warning{{Pattern: "x", Severity: policy.Low, Location: [2]int{0, 1}, Description: "d"}},
		WasModified: true,
	}
	if out.Content != "test" {
		t.Error("content mismatch")
	}
	if !out.WasModified {
		t.Error("expected WasModified=true")
	}
}
