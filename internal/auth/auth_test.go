package auth

import (
	"context"
	"testing"
)

func TestAPIKeyAuth(t *testing.T) {
	ctx := context.Background()
	auth := NewAPIKeyAuth(map[string]string{
		"key1": "user1",
		"key2": "user2",
	})

	t.Run("valid_key", func(t *testing.T) {
		userID, err := auth.Authenticate(ctx, "key1")
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if userID != "user1" {
			t.Errorf("expected user1, got %q", userID)
		}
	})

	t.Run("invalid_key", func(t *testing.T) {
		_, err := auth.Authenticate(ctx, "bad_key")
		if err == nil {
			t.Error("expected error for invalid key")
		}
	})

	t.Run("add_and_remove", func(t *testing.T) {
		auth.AddKey("key3", "user3")
		userID, err := auth.Authenticate(ctx, "key3")
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if userID != "user3" {
			t.Errorf("expected user3, got %q", userID)
		}

		auth.RemoveKey("key3")
		_, err = auth.Authenticate(ctx, "key3")
		if err == nil {
			t.Error("expected error after removal")
		}
	})
}

func TestNoAuth(t *testing.T) {
	ctx := context.Background()
	auth := NewNoAuth()

	userID, err := auth.Authenticate(ctx, "anything")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if userID != "anonymous" {
		t.Errorf("expected anonymous, got %q", userID)
	}
}
