package validator

import (
	"context"
	"regexp"
	"testing"

	"github.com/nearai/ironclaw-go/internal/safety/policy"
)

func TestValidateInputBlocked(t *testing.T) {
	p := policy.New()
	p.AddRule(policy.Rule{
		ID:          "test-block",
		Description: "Block this",
		Pattern:     regexp.MustCompile(`(?i)blockme`),
		Severity:    policy.High,
		Action:      policy.Block,
	})

	v := NewValidator(p)
	result := v.ValidateInput(context.Background(), "please blockme now")

	if result.IsValid {
		t.Fatal("expected validation to fail")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestValidateInputFlagged(t *testing.T) {
	p := policy.New()
	p.AddRule(policy.Rule{
		ID:          "test-flag",
		Description: "Flag this",
		Pattern:     regexp.MustCompile(`(?i)flagme`),
		Severity:    policy.Medium,
		Action:      policy.Flag,
	})

	v := NewValidator(p)
	result := v.ValidateInput(context.Background(), "please flagme now")

	if !result.IsValid {
		t.Fatal("expected validation to pass with warning")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestValidateInputEmpty(t *testing.T) {
	v := NewValidator(policy.New())
	result := v.ValidateInput(context.Background(), "   ")

	if result.IsValid {
		t.Fatal("expected validation to fail for empty content")
	}
}

func TestValidateInputClean(t *testing.T) {
	v := NewValidator(policy.New())
	result := v.ValidateInput(context.Background(), "hello world")

	if !result.IsValid {
		t.Fatal("expected validation to pass")
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(result.Errors))
	}
}

func TestScanCodeDangerous(t *testing.T) {
	v := NewValidator(policy.New())
	result := v.ScanCode(context.Background(), "eval('os.system(\"rm -rf /\")')")

	if result.IsValid {
		t.Fatal("expected code scan to fail")
	}
}

func TestScanCodeSafe(t *testing.T) {
	v := NewValidator(policy.New())
	result := v.ScanCode(context.Background(), "print('hello')")

	if !result.IsValid {
		t.Fatal("expected code scan to pass")
	}
}
