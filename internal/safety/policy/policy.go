package policy

import "regexp"

type Policy struct {
	rules []Rule
}

func New() *Policy {
	return &Policy{}
}

func (p *Policy) AddRule(r Rule) {
	p.rules = append(p.rules, r)
}

func (p *Policy) Evaluate(content string) []Violation {
	var violations []Violation
	for _, rule := range p.rules {
		if rule.Pattern != nil && rule.Pattern.MatchString(content) {
			violations = append(violations, Violation{
				RuleID:      rule.ID,
				Description: rule.Description,
				Severity:    rule.Severity,
				Action:      rule.Action,
				MatchedText: content,
			})
		}
	}
	return violations
}

func (p *Policy) LoadDefaults() {
	p.AddRule(Rule{
		ID:          "ignore-previous",
		Description: "Attempt to override previous instructions",
		Pattern:     regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+(instructions|commands)`),
		Severity:    High,
		Action:      Block,
	})
	p.AddRule(Rule{
		ID:          "disregard-prior",
		Description: "Attempt to disregard prior instructions",
		Pattern:     regexp.MustCompile(`(?i)disregard\s+(all\s+)?(prior|previous)`),
		Severity:    Medium,
		Action:      Block,
	})
	p.AddRule(Rule{
		ID:          "forget-everything",
		Description: "Attempt to reset context",
		Pattern:     regexp.MustCompile(`(?i)forget\s+everything`),
		Severity:    High,
		Action:      Block,
	})
	p.AddRule(Rule{
		ID:          "system-prompt",
		Description: "Attempt to access system prompt",
		Pattern:     regexp.MustCompile(`(?i)system\s+prompt`),
		Severity:    High,
		Action:      Block,
	})
	p.AddRule(Rule{
		ID:          "dan-mode",
		Description: "Attempt to enable jailbreak mode",
		Pattern:     regexp.MustCompile(`(?i)DAN\s*mode|jailbreak`),
		Severity:    Critical,
		Action:      Block,
	})
	p.AddRule(Rule{
		ID:          "xml-injection",
		Description: "Potential XML tag injection",
		Pattern:     regexp.MustCompile(`(?i)<\/\s*(system|instructions)\s*>`),
		Severity:    High,
		Action:      Block,
	})
}
