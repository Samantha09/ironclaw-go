package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/agent"
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/hooks"
	"github.com/nearai/ironclaw-go/internal/safety"
	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/tools/builtin"
)

// mockChannel is a test channel that feeds messages and captures responses.
type mockChannel struct {
	name    string
	msgs    chan channels.IncomingMessage
	resp    []channels.OutgoingResponse
}

func newMockChannel(name string) *mockChannel {
	return &mockChannel{
		name: name,
		msgs: make(chan channels.IncomingMessage, 8),
	}
}

func (m *mockChannel) Name() string { return m.name }

func (m *mockChannel) Messages() <-chan channels.IncomingMessage {
	return m.msgs
}

func (m *mockChannel) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	m.resp = append(m.resp, msg)
	return nil
}

func (m *mockChannel) Shutdown(_ context.Context) error {
	close(m.msgs)
	return nil
}

func (m *mockChannel) inject(msg channels.IncomingMessage) {
	m.msgs <- msg
}

func setupAgent(t *testing.T) (*agent.Agent, *mockChannel, *channels.Manager) {
	t.Helper()

	registry := tools.NewRegistry()
	registry.Register(builtin.NewEchoTool())
	registry.Register(builtin.NewTimeTool())
	registry.Register(builtin.NewShellTool())
	registry.Register(builtin.NewMemoryTool())

	safetyLayer := safety.NewLayer()
	dispatcher := tools.NewDispatcher(registry, safetyLayer, nil)

	deps := agent.Deps{
		OwnerID:    "test_user",
		Database:   db.NewMemoryDB(),
		Tools:      registry,
		Dispatcher: dispatcher,
		Hooks:      hooks.NewRegistry(),
	}

	ag := agent.New(agent.Config{
		Name:             "TestAgent",
		MaxParallelJobs:  2,
		AutoApproveTools: true,
	}, deps)

	mockCh := newMockChannel("mock")
	mgr := channels.NewManager()
	mgr.Add(mockCh)

	return ag, mockCh, mgr
}

func TestAgent_EchoTool(t *testing.T) {
	ag, mockCh, mgr := setupAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mgr.Start(ctx)

	go func() {
		_ = ag.Run(ctx, mgr)
	}()

	mockCh.inject(channels.IncomingMessage{
		ID:      "1",
		Channel: "mock",
		UserID:  "test_user",
		Content: `tool:echo {"message":"hello"}`,
	})

	time.Sleep(200 * time.Millisecond)
	cancel()

	if len(mockCh.resp) == 0 {
		t.Fatal("expected at least one response")
	}
	if mockCh.resp[0].Content != "hello" {
		t.Fatalf("expected 'hello', got %q", mockCh.resp[0].Content)
	}
}

func TestAgent_SystemCommand(t *testing.T) {
	ag, mockCh, mgr := setupAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mgr.Start(ctx)

	go func() {
		_ = ag.Run(ctx, mgr)
	}()

	mockCh.inject(channels.IncomingMessage{
		ID:      "1",
		Channel: "mock",
		UserID:  "test_user",
		Content: "/help",
	})

	time.Sleep(200 * time.Millisecond)
	cancel()

	if len(mockCh.resp) == 0 {
		t.Fatal("expected at least one response")
	}
	if !contains(mockCh.resp[0].Content, "可用命令") {
		t.Fatalf("expected help text, got %q", mockCh.resp[0].Content)
	}
}

func TestAgent_SessionManager(t *testing.T) {
	ag, mockCh, mgr := setupAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mgr.Start(ctx)

	go func() {
		_ = ag.Run(ctx, mgr)
	}()

	// Create a new thread
	mockCh.inject(channels.IncomingMessage{
		ID:      "1",
		Channel: "mock",
		UserID:  "test_user",
		Content: "/new",
	})

	time.Sleep(100 * time.Millisecond)

	// List threads
	mockCh.inject(channels.IncomingMessage{
		ID:      "2",
		Channel: "mock",
		UserID:  "test_user",
		Content: "/threads",
	})

	time.Sleep(500 * time.Millisecond)
	cancel()

	if len(mockCh.resp) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(mockCh.resp))
	}
	if !contains(mockCh.resp[1].Content, "对话线程列表") {
		t.Fatalf("expected thread list, got %q", mockCh.resp[1].Content)
	}
}

func TestAgent_MemoryTool(t *testing.T) {
	ag, mockCh, mgr := setupAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mgr.Start(ctx)

	go func() {
		_ = ag.Run(ctx, mgr)
	}()

	// Set a value
	mockCh.inject(channels.IncomingMessage{
		ID:      "1",
		Channel: "mock",
		UserID:  "test_user",
		Content: `tool:memory {"action":"set","key":"test","value":"42"}`,
	})

	time.Sleep(100 * time.Millisecond)

	// Get the value
	mockCh.inject(channels.IncomingMessage{
		ID:      "2",
		Channel: "mock",
		UserID:  "test_user",
		Content: `tool:memory {"action":"get","key":"test"}`,
	})

	time.Sleep(500 * time.Millisecond)
	cancel()

	if len(mockCh.resp) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(mockCh.resp))
	}
	if !contains(mockCh.resp[1].Content, "42") {
		t.Fatalf("expected memory value '42', got %q", mockCh.resp[1].Content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
