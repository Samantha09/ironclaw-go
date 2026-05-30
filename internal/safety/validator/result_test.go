package validator

import "testing"

func TestNewResult(t *testing.T) {
	r := NewResult()
	if !r.IsValid {
		t.Error("expected new result to be valid")
	}
	if len(r.Errors) != 0 {
		t.Error("expected new result to have no errors")
	}
}

func TestResultWithError(t *testing.T) {
	r := NewResult().WithError(ValidationError{Field: "x", Message: "bad", Code: ErrForbiddenContent})
	if r.IsValid {
		t.Error("expected result to be invalid after adding error")
	}
	if len(r.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(r.Errors))
	}
}

func TestResultMerge(t *testing.T) {
	r1 := NewResult().WithWarning("warn1")
	r2 := NewResult().WithError(ValidationError{Field: "x", Message: "bad", Code: ErrForbiddenContent})
	merged := r1.Merge(r2)
	if merged.IsValid {
		t.Error("expected merged result to be invalid")
	}
	if len(merged.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(merged.Warnings))
	}
	if len(merged.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(merged.Errors))
	}
}
