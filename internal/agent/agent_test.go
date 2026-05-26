package agent

import (
	"context"
	"testing"

	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/llm"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// mockLLM 是可编程的 LLM 模拟器。
type mockLLM struct {
	respondWith    string
	respondTools   []llm.ToolCall
	respondErr     error
	streamRespond  []llm.StreamChunk
	streamRespondErr error
}

func (m *mockLLM) Complete(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition) (llm.CompletionResponse, error) {
	if m.respondErr != nil {
		return llm.CompletionResponse{}, m.respondErr
	}
	return llm.CompletionResponse{Content: m.respondWith, ToolCalls: m.respondTools}, nil
}

func (m *mockLLM) StreamComplete(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	if m.streamRespondErr != nil {
		return nil, m.streamRespondErr
	}
	ch := make(chan llm.StreamChunk, len(m.streamRespond))
	for _, c := range m.streamRespond {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (m *mockLLM) ModelName() string { return "mock" }

// mockTool 是可编程的工具模拟器。
type mockTool struct {
	name          string
	description   string
	output        string
	execErr       error
	approvalReq   gate.ApprovalRequirement
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) ParameterSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (m *mockTool) Execute(_ context.Context, _ map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	return tools.ToolOutput{Content: m.output}, m.execErr
}
func (m *mockTool) RequiresApproval(_ map[string]any) gate.ApprovalRequirement {
	return m.approvalReq
}

func setupTestAgent() (*Agent, *tools.Registry, *tools.Dispatcher) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{name: "echo", description: "echo", output: "echoed", approvalReq: gate.Never})

	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	// Disable gates for simple agent tests
	dispatcher.WithGates()

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:        &mockLLM{respondWith: "hello from llm"},
		Tools:      registry,
		Dispatcher: dispatcher,
	})
	return ag, registry, dispatcher
}

func TestAgentProcessMessageSystemCommand(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	// /help
	msg := channels.IncomingMessage{UserID: "user1", Content: "/help"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty help response")
	}

	// /new
	msg = channels.IncomingMessage{UserID: "user1", Content: "/new"}
	resp, err = ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty new thread response")
	}

	// /threads
	msg = channels.IncomingMessage{UserID: "user1", Content: "/threads"}
	resp, err = ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty threads response")
	}
}

func TestAgentProcessMessageToolCall(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "tool:echo hello"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content != "echoed" {
		t.Errorf("content = %q, want echoed", resp.Content)
	}
}

func TestAgentProcessMessageToolCallMissing(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "tool:missing_tool"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected error response for missing tool")
	}
}

func TestAgentProcessMessageChatWithLLM(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "how are you"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content != "hello from llm" {
		t.Errorf("content = %q, want 'hello from llm'", resp.Content)
	}
}

func TestAgentProcessMessageChatNoLLM(t *testing.T) {
	ag := New(Config{Name: "TestAgent"}, Deps{})
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "hello"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected echo fallback response")
	}
}

func TestAgentProcessMessageLLMQuery(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "?what is 2+2"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content != "hello from llm" {
		t.Errorf("content = %q, want 'hello from llm'", resp.Content)
	}
}

func TestAgentHandleSystemCommandSwitch(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	// Create a thread first
	ag.sessionManager.GetOrCreateThread("user1", "repl")

	// Switch to it
	msg := channels.IncomingMessage{UserID: "user1", Content: "/switch " + ag.sessionManager.GetOrCreateThread("user1", "repl").ID}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected switch response")
	}
}

func TestAgentHandleSystemCommandSwitchMissing(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ctx := context.Background()

	msg := channels.IncomingMessage{UserID: "user1", Content: "/switch nonexistent"}
	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected not-found response")
	}
}

func TestAgentBuildLLMMessages(t *testing.T) {
	ag, _, _ := setupTestAgent()
	ag.sessionManager.GetOrCreateThread("user1", "repl")
	ag.sessionManager.AddTurn("user1", Turn{UserMsg: "hi", AgentResp: "hello"})

	msgs := ag.buildLLMMessages("user1", "how are you")
	if len(msgs) < 3 {
		t.Fatalf("messages = %d, want at least 3", len(msgs))
	}
	if msgs[0].Role != llm.RoleSystem {
		t.Errorf("first role = %v, want system", msgs[0].Role)
	}
	if msgs[len(msgs)-1].Role != llm.RoleUser {
		t.Errorf("last role = %v, want user", msgs[len(msgs)-1].Role)
	}
	if msgs[len(msgs)-1].Content != "how are you" {
		t.Errorf("last content = %q", msgs[len(msgs)-1].Content)
	}
}

func TestAgentBuildLLMTools(t *testing.T) {
	ag, _, _ := setupTestAgent()
	defs := ag.buildLLMTools()
	if len(defs) != 1 {
		t.Errorf("tools = %d, want 1", len(defs))
	}
	if len(defs) > 0 && defs[0].Function.Name != "echo" {
		t.Errorf("tool name = %q, want echo", defs[0].Function.Name)
	}
}

func TestAgentHandleLLMToolCalls(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{name: "echo", description: "echo", output: "tool result", approvalReq: gate.Never})
	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	dispatcher.WithGates()

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:        &mockLLM{respondWith: "summarized"},
		Tools:      registry,
		Dispatcher: dispatcher,
	})
	ctx := context.Background()

	resp := llm.CompletionResponse{
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Function: llm.FunctionCall{Name: "echo", Arguments: `{"message":"test"}`}},
		},
	}

	result, err := ag.handleLLMToolCalls(ctx, "user1", "thread1", "original", resp)
	if err != nil {
		t.Fatalf("handleLLMToolCalls: %v", err)
	}
	if result.Content == "" {
		t.Error("expected non-empty result after tool call")
	}
}

func TestAgentEchoReply(t *testing.T) {
	ag := New(Config{Name: "TestBot"}, Deps{})
	resp := ag.echoReply("user1", "hello")
	if resp.Content == "" {
		t.Error("expected non-empty echo reply")
	}
}

func TestAgentPersistIfNeeded(t *testing.T) {
	// No database — should not panic
	ag := New(Config{Name: "Test"}, Deps{})
	thread := ag.sessionManager.GetOrCreateThread("user1", "repl")
	ag.persistIfNeeded(context.Background(), thread)
}
