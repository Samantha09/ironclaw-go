package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/channels/httpgw"
)

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func setupGateway(t *testing.T) (*httpgw.Gateway, int) {
	t.Helper()
	port, err := findFreePort()
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	gw := httpgw.New(port)
	gw.Start()
	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)
	return gw, port
}

// readSSEEvents reads events from an SSE response body until timeout or context cancellation.
func readSSEEvents(t *testing.T, resp *http.Response, timeout time.Duration) []map[string]string {
	t.Helper()
	var events []map[string]string
	scanner := bufio.NewScanner(resp.Body)

	done := make(chan struct{})
	go func() {
		defer close(done)
		current := make(map[string]string)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(current) > 0 {
					events = append(events, current)
					current = make(map[string]string)
				}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				current["event"] = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				current["data"] = strings.TrimPrefix(line, "data: ")
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
	return events
}

func TestSSE_ConnectionEvent(t *testing.T) {
	gw, port := setupGateway(t)
	defer gw.Shutdown(context.Background())

	url := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=test_user", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	events := readSSEEvents(t, resp, 200*time.Millisecond)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0]["event"] != "connected" {
		t.Fatalf("expected 'connected' event, got %s", events[0]["event"])
	}
}

func TestSSE_EventDelivery(t *testing.T) {
	gw, port := setupGateway(t)
	defer gw.Shutdown(context.Background())

	userID := "alice"
	threadID := "thread-42"

	url := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=%s&thread_id=%s", port, userID, threadID)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Give the subscription time to register
	time.Sleep(50 * time.Millisecond)

	// Publish events via the gateway's EventHub
	gw.PublishEvent(channels.Event{
		Type:     channels.EventToolCall,
		UserID:   userID,
		ThreadID: threadID,
		Payload:  "Executing tool: echo",
		Meta:     map[string]any{"tool_name": "echo"},
	})
	gw.PublishEvent(channels.Event{
		Type:     channels.EventToolResult,
		UserID:   userID,
		ThreadID: threadID,
		Payload:  "Tool echo success (12 ms)",
		Meta:     map[string]any{"tool_name": "echo", "status": "success"},
	})
	gw.PublishEvent(channels.Event{
		Type:     channels.EventAgentResponse,
		UserID:   userID,
		ThreadID: threadID,
		Payload:  "Hello, Alice!",
	})

	events := readSSEEvents(t, resp, 500*time.Millisecond)

	// Filter out connected and ping events
	var filtered []map[string]string
	for _, ev := range events {
		if ev["event"] != "connected" && ev["event"] != "ping" {
			filtered = append(filtered, ev)
		}
	}

	if len(filtered) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(filtered), filtered)
	}

	expectedTypes := []string{"tool_call", "tool_result", "agent_response"}
	for i, exp := range expectedTypes {
		if filtered[i]["event"] != exp {
			t.Fatalf("event %d: expected %s, got %s", i, exp, filtered[i]["event"])
		}
		// Verify data is valid JSON containing the Event struct
		var ev channels.Event
		if err := json.Unmarshal([]byte(filtered[i]["data"]), &ev); err != nil {
			t.Fatalf("event %d data not valid JSON: %v", i, err)
		}
		if ev.UserID != userID {
			t.Fatalf("event %d: expected user_id %s, got %s", i, userID, ev.UserID)
		}
		if ev.ThreadID != threadID {
			t.Fatalf("event %d: expected thread_id %s, got %s", i, threadID, ev.ThreadID)
		}
	}
}

func TestSSE_ThreadFiltering(t *testing.T) {
	gw, port := setupGateway(t)
	defer gw.Shutdown(context.Background())

	userID := "bob"
	threadA := "thread-a"
	threadB := "thread-b"

	urlA := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=%s&thread_id=%s", port, userID, threadA)
	respA, err := http.Get(urlA)
	if err != nil {
		t.Fatalf("connect to SSE endpoint A: %v", err)
	}
	defer respA.Body.Close()

	urlB := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=%s&thread_id=%s", port, userID, threadB)
	respB, err := http.Get(urlB)
	if err != nil {
		t.Fatalf("connect to SSE endpoint B: %v", err)
	}
	defer respB.Body.Close()

	time.Sleep(50 * time.Millisecond)

	// Publish event only to thread A
	gw.PublishEvent(channels.Event{
		Type:     channels.EventAgentResponse,
		UserID:   userID,
		ThreadID: threadA,
		Payload:  "Message for thread A",
	})

	// Collect events from both connections
	eventsA := readSSEEvents(t, respA, 300*time.Millisecond)
	eventsB := readSSEEvents(t, respB, 300*time.Millisecond)

	// Thread A should have connected + agent_response
	var foundA bool
	for _, ev := range eventsA {
		if ev["event"] == "agent_response" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Fatal("expected agent_response on thread A connection")
	}

	// Thread B should only have connected (no agent_response)
	for _, ev := range eventsB {
		if ev["event"] == "agent_response" {
			t.Fatal("thread B should not receive thread A's event")
		}
	}
}

func TestSSE_UserFiltering(t *testing.T) {
	gw, port := setupGateway(t)
	defer gw.Shutdown(context.Background())

	threadID := "shared-thread"

	urlAlice := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=alice&thread_id=%s", port, threadID)
	respAlice, err := http.Get(urlAlice)
	if err != nil {
		t.Fatalf("connect to SSE endpoint for alice: %v", err)
	}
	defer respAlice.Body.Close()

	urlBob := fmt.Sprintf("http://127.0.0.1:%d/api/chat/stream?user_id=bob&thread_id=%s", port, threadID)
	respBob, err := http.Get(urlBob)
	if err != nil {
		t.Fatalf("connect to SSE endpoint for bob: %v", err)
	}
	defer respBob.Body.Close()

	time.Sleep(50 * time.Millisecond)

	gw.PublishEvent(channels.Event{
		Type:     channels.EventAgentResponse,
		UserID:   "alice",
		ThreadID: threadID,
		Payload:  "Hello Alice",
	})

	eventsAlice := readSSEEvents(t, respAlice, 300*time.Millisecond)
	eventsBob := readSSEEvents(t, respBob, 300*time.Millisecond)

	var foundAlice bool
	for _, ev := range eventsAlice {
		if ev["event"] == "agent_response" {
			foundAlice = true
			break
		}
	}
	if !foundAlice {
		t.Fatal("expected agent_response on alice's connection")
	}

	for _, ev := range eventsBob {
		if ev["event"] == "agent_response" {
			t.Fatal("bob should not receive alice's event")
		}
	}
}

func TestSSE_ChatEndpointStillWorks(t *testing.T) {
	gw, port := setupGateway(t)
	defer gw.Shutdown(context.Background())

	url := fmt.Sprintf("http://127.0.0.1:%d/api/chat", port)
	body := `{"user_id":"test_user","content":"hello"}`

	// Use a short client timeout so we don't wait 30s for the server-side timeout
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		// A timeout error here is expected since no agent is consuming messages
		if strings.Contains(err.Error(), "Client.Timeout") || strings.Contains(err.Error(), "context deadline exceeded") {
			return
		}
		t.Fatalf("post to /api/chat: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 200 or 504, got %d", resp.StatusCode)
	}
}
