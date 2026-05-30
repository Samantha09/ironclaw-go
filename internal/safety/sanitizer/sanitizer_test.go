package sanitizer

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/safety/policy"
)

func TestSanitizeToolOutputTruncation(t *testing.T) {
	s := NewSanitizer(policy.New())
	content := "this is a very long content string"
	result := s.SanitizeToolOutput(context.Background(), "echo", content, 10)

	if !result.WasModified {
		t.Fatal("expected content to be modified")
	}
	if !strings.Contains(result.Content, "[... truncated") {
		t.Fatal("expected truncation notice in content")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestSanitizeToolOutputNoTruncation(t *testing.T) {
	s := NewSanitizer(policy.New())
	content := "short"
	result := s.SanitizeToolOutput(context.Background(), "echo", content, 100)

	if result.WasModified {
		t.Fatal("expected content not to be modified")
	}
	if result.Content != content {
		t.Fatalf("expected %q, got %q", content, result.Content)
	}
}

func TestSanitizeToolOutputWithPolicy(t *testing.T) {
	p := policy.New()
	p.AddRule(policy.Rule{
		ID:          "test-output",
		Description: "Suspicious output",
		Pattern:     regexp.MustCompile(`(?i)secret`),
		Severity:    policy.Medium,
		Action:      policy.Flag,
	})

	s := NewSanitizer(p)
	result := s.SanitizeToolOutput(context.Background(), "echo", "this has secret", 100)

	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning from policy, got %d", len(result.Warnings))
	}
}
