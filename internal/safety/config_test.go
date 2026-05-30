package safety

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxOutputLength != 10000 {
		t.Errorf("expected MaxOutputLength=10000, got %d", cfg.MaxOutputLength)
	}
	if cfg.RateMaxCalls != 100 {
		t.Errorf("expected RateMaxCalls=100, got %d", cfg.RateMaxCalls)
	}
}
