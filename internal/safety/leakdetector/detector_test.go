package leakdetector

import (
	"regexp"
	"testing"
)

func TestScanAPIKey(t *testing.T) {
	d := New()
	matches := d.Scan("The key is sk-abc123def456ghi789jkl012mno345pqr678")

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Type != "api_key" {
		t.Errorf("expected type api_key, got %s", matches[0].Type)
	}
}

func TestScanPassword(t *testing.T) {
	d := New()
	matches := d.Scan("password: secret123")

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Type != "password" {
		t.Errorf("expected type password, got %s", matches[0].Type)
	}
}

func TestScanAndClean(t *testing.T) {
	d := New()
	cleaned, matches := d.ScanAndClean("password: secret123")

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if cleaned == "password: secret123" {
		t.Fatal("expected content to be cleaned")
	}
	if !regexp.MustCompile(`\[REDACTED\]`).MatchString(cleaned) {
		t.Fatalf("expected [REDACTED] in cleaned content, got %s", cleaned)
	}
}

func TestScanClean(t *testing.T) {
	d := New()
	matches := d.Scan("hello world")

	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}
