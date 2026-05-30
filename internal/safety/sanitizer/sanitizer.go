package sanitizer

import (
	"context"
	"fmt"

	"github.com/nearai/ironclaw-go/internal/safety/policy"
)

type Sanitizer struct {
	policy *policy.Policy
}

func NewSanitizer(p *policy.Policy) *Sanitizer {
	return &Sanitizer{policy: p}
}

func (s *Sanitizer) SanitizeToolOutput(ctx context.Context, toolName, content string, maxLen int) Output {
	var warnings []Warning
	wasModified := false

	truncatedContent := content
	if maxLen > 0 && len(content) > maxLen {
		truncatedContent = content[:maxLen]
		truncatedContent += fmt.Sprintf("\n\n[... truncated: showing %d/%d bytes. Use the json tool with source_tool_call_id to query the full output.]", maxLen, len(content))
		wasModified = true
		warnings = append(warnings, Warning{
			Pattern:     "output_too_large",
			Severity:    policy.Low,
			Location:    [2]int{maxLen, len(content)},
			Description: fmt.Sprintf("Output from tool '%s' was truncated due to size", toolName),
		})
	}

	violations := s.policy.Evaluate(truncatedContent)
	for _, v := range violations {
		warnings = append(warnings, Warning{
			Pattern:     v.RuleID,
			Severity:    v.Severity,
			Location:    [2]int{0, len(truncatedContent)},
			Description: v.Description,
		})
	}

	return Output{
		Content:     truncatedContent,
		Warnings:    warnings,
		WasModified: wasModified,
	}
}
