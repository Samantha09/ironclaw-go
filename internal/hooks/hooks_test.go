package hooks

import (
	"context"
	"errors"
	"testing"
)

func TestRegistry(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry()

	t.Run("register_and_trigger", func(t *testing.T) {
		var called bool
		reg.Register(EventBeforeMessage, func(_ context.Context, ev Event) error {
			called = true
			if ev.Type != EventBeforeMessage {
				t.Errorf("expected %s, got %s", EventBeforeMessage, ev.Type)
			}
			if ev.UserID != "user1" {
				t.Errorf("expected user1, got %s", ev.UserID)
			}
			return nil
		})

		err := reg.Trigger(ctx, Event{Type: EventBeforeMessage, UserID: "user1"})
		if err != nil {
			t.Fatalf("Trigger failed: %v", err)
		}
		if !called {
			t.Error("handler was not called")
		}
	})

	t.Run("multiple_handlers", func(t *testing.T) {
		var count int
		reg.Register(EventAfterMessage, func(_ context.Context, _ Event) error {
			count++
			return nil
		})
		reg.Register(EventAfterMessage, func(_ context.Context, _ Event) error {
			count++
			return nil
		})

		err := reg.Trigger(ctx, Event{Type: EventAfterMessage})
		if err != nil {
			t.Fatalf("Trigger failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 handlers called, got %d", count)
		}
	})

	t.Run("error_stops_chain", func(t *testing.T) {
		reg := NewRegistry() // 新注册表避免干扰
		var secondCalled bool
		reg.Register(EventBeforeToolCall, func(_ context.Context, _ Event) error {
			return errors.New("first handler error")
		})
		reg.Register(EventBeforeToolCall, func(_ context.Context, _ Event) error {
			secondCalled = true
			return nil
		})

		err := reg.Trigger(ctx, Event{Type: EventBeforeToolCall})
		if err == nil {
			t.Fatal("expected error")
		}
		if secondCalled {
			t.Error("second handler should not have been called")
		}
	})

	t.Run("no_handlers", func(t *testing.T) {
		reg := NewRegistry()
		err := reg.Trigger(ctx, Event{Type: EventBeforeResponse})
		if err != nil {
			t.Fatalf("Trigger with no handlers should not error: %v", err)
		}
	})

	t.Run("has_handlers", func(t *testing.T) {
		reg := NewRegistry()
		if reg.HasHandlers(EventBeforeMessage) {
			t.Error("expected no handlers")
		}
		reg.Register(EventBeforeMessage, func(_ context.Context, _ Event) error { return nil })
		if !reg.HasHandlers(EventBeforeMessage) {
			t.Error("expected handlers")
		}
	})
}
