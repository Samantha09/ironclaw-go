package safety

import (
	"context"
	"fmt"

	"github.com/nearai/ironclaw-go/internal/safety/leakdetector"
	"github.com/nearai/ironclaw-go/internal/safety/policy"
	"github.com/nearai/ironclaw-go/internal/safety/sanitizer"
	"github.com/nearai/ironclaw-go/internal/safety/validator"
)

type Layer struct {
	validator    *validator.Validator
	sanitizer    *sanitizer.Sanitizer
	policy       *policy.Policy
	leakDetector *leakdetector.Detector
	rateLimiter  *RateLimiter
	config       Config
}

// NewLayer creates a safety layer with default configuration.
// Maintains backward compatibility with existing callers.
func NewLayer() *Layer {
	return NewLayerWithConfig(DefaultConfig())
}

// NewLayerWithConfig creates a safety layer with custom configuration.
func NewLayerWithConfig(config Config) *Layer {
	p := policy.New()
	p.LoadDefaults()

	return &Layer{
		validator:    validator.NewValidator(p),
		sanitizer:    sanitizer.NewSanitizer(p),
		policy:       p,
		leakDetector: leakdetector.New(),
		rateLimiter:  NewRateLimiter(config.RateMaxCalls, config.RateWindow),
		config:       config,
	}
}

func (l *Layer) ScanInbound(ctx context.Context, content string) error {
	result := l.validator.ValidateInput(ctx, content)
	if !result.IsValid {
		if len(result.Errors) > 0 {
			return fmt.Errorf("safety: %s", result.Errors[0].Message)
		}
		return fmt.Errorf("safety: input blocked by policy")
	}
	return nil
}

func (l *Layer) SanitizeToolOutput(ctx context.Context, content string) (string, error) {
	sanitized := l.sanitizer.SanitizeToolOutput(ctx, "", content, l.config.MaxOutputLength)
	cleaned, _ := l.leakDetector.ScanAndClean(sanitized.Content)
	return cleaned, nil
}

func (l *Layer) ScanCode(ctx context.Context, code string) error {
	result := l.validator.ScanCode(ctx, code)
	if !result.IsValid {
		if len(result.Errors) > 0 {
			return fmt.Errorf("safety: %s", result.Errors[0].Message)
		}
		return fmt.Errorf("safety: code blocked by policy")
	}
	return nil
}

func (l *Layer) Allow(key string) bool {
	return l.rateLimiter.Allow(key)
}
