package httpgw

import (
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/channels"
)

func TestEventHubSubscribeAndPublish(t *testing.T) {
	hub := NewEventHub()

	ch := hub.Subscribe("user-1", "thread-a")
	defer hub.Unsubscribe("user-1", "thread-a", ch)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.Publish(channels.Event{
			Type:     channels.EventAgentResponse,
			UserID:   "user-1",
			ThreadID: "thread-a",
			Payload:  "hello",
		})
	}()

	select {
	case ev := <-ch:
		if ev.Payload != "hello" {
			t.Fatalf("expected payload hello, got %s", ev.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventHubFilterByUser(t *testing.T) {
	hub := NewEventHub()

	ch := hub.Subscribe("user-1", "")
	defer hub.Unsubscribe("user-1", "", ch)

	hub.Publish(channels.Event{Type: channels.EventAgentResponse, UserID: "user-2", ThreadID: "thread-a", Payload: "x"})
	hub.Publish(channels.Event{Type: channels.EventAgentResponse, UserID: "user-1", ThreadID: "thread-b", Payload: "y"})

	select {
	case ev := <-ch:
		if ev.Payload != "y" {
			t.Fatalf("expected payload y, got %s", ev.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
