package policy

import "regexp"

type Severity int

const (
	Low Severity = iota + 1
	Medium
	High
	Critical
)

type Action int

const (
	Flag Action = iota
	Block
	Sanitize
)

type Rule struct {
	ID          string
	Description string
	Pattern     *regexp.Regexp
	Severity    Severity
	Action      Action
}

type Violation struct {
	RuleID      string
	Description string
	Severity    Severity
	Action      Action
	MatchedText string
}
