package safety

import "context"

// Layer — sanitizes and validates tool I/O.
// MVP: pass-through; future: injection detection, leak scanning.
type Layer struct{}

func NewLayer() *Layer { return &Layer{} }

// SanitizeToolOutput runs output through sanitization rules.
func (l *Layer) SanitizeToolOutput(ctx context.Context, content string) (string, error) {
	return content, nil
}

// ScanInbound runs safety checks on user input.
func (l *Layer) ScanInbound(ctx context.Context, content string) error {
	return nil
}
