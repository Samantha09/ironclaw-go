package secrets

import (
	"testing"
)

func TestStore(t *testing.T) {
	store, err := NewStore("my-master-key-32bytes-long!!1234")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Set and Get
	if err := store.Set("api_key", "sk-123456"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := store.Get("api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "sk-123456" {
		t.Errorf("expected sk-123456, got %q", val)
	}

	// HasKey
	if !store.HasKey("api_key") {
		t.Error("expected HasKey(api_key) = true")
	}
	if store.HasKey("missing") {
		t.Error("expected HasKey(missing) = false")
	}

	// List
	keys := store.List()
	if len(keys) != 1 || keys[0] != "api_key" {
		t.Errorf("expected [api_key], got %v", keys)
	}

	// Delete
	store.Delete("api_key")
	_, err = store.Get("api_key")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestStoreInvalidKey(t *testing.T) {
	_, err := NewStore("short")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	if !ConstantTimeCompare("abc", "abc") {
		t.Error("expected equal strings to match")
	}
	if ConstantTimeCompare("abc", "def") {
		t.Error("expected different strings to not match")
	}
}
