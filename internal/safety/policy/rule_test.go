package policy

import "testing"

func TestSeverityValues(t *testing.T) {
	if Low != 1 {
		t.Errorf("expected Low=1, got %d", Low)
	}
	if Critical != 4 {
		t.Errorf("expected Critical=4, got %d", Critical)
	}
}

func TestActionValues(t *testing.T) {
	if Flag != 0 {
		t.Errorf("expected Flag=0, got %d", Flag)
	}
	if Sanitize != 2 {
		t.Errorf("expected Sanitize=2, got %d", Sanitize)
	}
}
