package validator

import (
	"context"
	"strings"

	"github.com/nearai/ironclaw-go/internal/safety/policy"
)

type Validator struct {
	policy *policy.Policy
}

func NewValidator(p *policy.Policy) *Validator {
	return &Validator{policy: p}
}

func (v *Validator) ValidateInput(ctx context.Context, content string) Result {
	result := NewResult()

	if strings.TrimSpace(content) == "" {
		return result.WithError(ValidationError{
			Field:   "content",
			Message: "content is empty",
			Code:    ErrForbiddenContent,
		})
	}

	if len(content) > 100000 {
		return result.WithError(ValidationError{
			Field:   "content",
			Message: "content exceeds maximum length",
			Code:    ErrTooLong,
		})
	}

	violations := v.policy.Evaluate(content)
	for _, v := range violations {
		if v.Action == policy.Block {
			result = result.WithError(ValidationError{
				Field:   "content",
				Message: v.Description,
				Code:    ErrSuspiciousPattern,
			})
		} else {
			result = result.WithWarning(v.Description)
		}
	}

	return result
}

func (v *Validator) ScanCode(ctx context.Context, code string) Result {
	result := NewResult()

	dangerous := []string{"eval(", "exec(", "system(", "os.system", "subprocess.call"}
	lower := strings.ToLower(code)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			result = result.WithError(ValidationError{
				Field:   "code",
				Message: "dangerous code pattern detected: " + d,
				Code:    ErrForbiddenContent,
			})
		}
	}

	return result
}
