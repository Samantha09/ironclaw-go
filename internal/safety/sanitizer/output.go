package sanitizer

import "github.com/nearai/ironclaw-go/internal/safety/policy"

type Warning struct {
	Pattern     string
	Severity    policy.Severity
	Location    [2]int
	Description string
}

type Output struct {
	Content     string
	Warnings    []Warning
	WasModified bool
}
