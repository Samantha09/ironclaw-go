package agent

import (
	"context"
	"testing"
	"time"

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

	messages := ag.buildLLMMessages("user1", "original")
	result, err := ag.handleLLMToolCalls(ctx, "user1", "thread1", "original", messages, resp)
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

// pauseTool 是一个模拟工具，执行时总是触发 gate Pause。
type pauseTool struct{}

func (p *pauseTool) Name() string        { return "pause_tool" }
func (p *pauseTool) Description() string { return "always pauses" }
func (p *pauseTool) ParameterSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (p *pauseTool) Execute(_ context.Context, _ map[string]any, _ *tools.JobContext) (tools.ToolOutput, error) {
	return tools.ToolOutput{Content: "should not reach"}, nil
}
func (p *pauseTool) RequiresApproval(_ map[string]any) gate.ApprovalRequirement {
	return gate.Always
}

func TestAgentHandleLLMToolCallsPause(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&pauseTool{})
	registry.Register(&mockTool{name: "safe", description: "safe", output: "ok", approvalReq: gate.Never})

	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	approvalGate := gate.NewApprovalGate(func(toolName string, params map[string]any, _ string) gate.ApprovalRequirement {
		if toolName == "pause_tool" {
			return gate.Always
		}
		return gate.Never
	})
	dispatcher.WithGates(approvalGate)

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:        &mockLLM{respondWith: "summarized"},
		Tools:      registry,
		Dispatcher: dispatcher,
	})
	ctx := context.Background()
	ag.sessionManager.GetOrCreateThread("user1", "repl")

	resp := llm.CompletionResponse{
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Function: llm.FunctionCall{Name: "safe", Arguments: `{}`}},
			{ID: "call_2", Function: llm.FunctionCall{Name: "pause_tool", Arguments: `{}`}},
		},
	}

	messages := ag.buildLLMMessages("user1", "original")
	result, err := ag.handleLLMToolCalls(ctx, "user1", "thread1", "original", messages, resp)
	if err != nil {
		t.Fatalf("handleLLMToolCalls: %v", err)
	}
	if result.Status != "pending_gate" {
		t.Errorf("status = %q, want pending_gate", result.Status)
	}

	// 验证 PendingExecution 被保存
	pe := ag.sessionManager.GetPendingExecution("user1", "thread1")
	if pe == nil {
		t.Fatal("expected pending execution to be saved")
	}
	if pe.NextIndex != 1 {
		t.Errorf("nextIndex = %d, want 1", pe.NextIndex)
	}
	if len(pe.ToolCalls) != 2 {
		t.Errorf("toolCalls = %d, want 2", len(pe.ToolCalls))
	}
}

func TestAgentResume(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{name: "echo", description: "echo", output: "echoed", approvalReq: gate.Never})

	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	dispatcher.WithGates()

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:        &mockLLM{respondWith: "final summary"},
		Tools:      registry,
		Dispatcher: dispatcher,
	})
	ctx := context.Background()
	ag.sessionManager.GetOrCreateThread("user1", "repl")

	// 手动构造 PendingExecution
	pe := &PendingExecution{
		RequestID:       "test-req",
		UserID:          "user1",
		ThreadID:        "thread1",
		Messages:        ag.buildLLMMessages("user1", "original"),
		ToolCalls:       []llm.ToolCall{{ID: "call_1", Function: llm.FunctionCall{Name: "echo", Arguments: `{}`}}},
		NextIndex:       0,
		OriginalContent: "original",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}
	ag.sessionManager.SavePendingExecution(pe)

	result, err := ag.Resume(ctx, "user1", "thread1")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if result.Content != "final summary" {
		t.Errorf("content = %q, want 'final summary'", result.Content)
	}

	// 恢复后应清除 PendingExecution
	if ag.sessionManager.GetPendingExecution("user1", "thread1") != nil {
		t.Error("expected pending execution to be cleared after resume")
	}
}

func TestAgentRunLLMResume(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{name: "echo", description: "echo", output: "echoed", approvalReq: gate.Never})

	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	dispatcher.WithGates()
	pendingStore := gate.NewPendingStore()

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:          &mockLLM{respondWith: "final summary"},
		Tools:        registry,
		Dispatcher:   dispatcher,
		PendingStore: pendingStore,
	})
	ctx := context.Background()
	thread := ag.sessionManager.GetOrCreateThread("user1", "repl")

	// 构造 PendingExecution
	pe := &PendingExecution{
		RequestID:       "test-req",
		UserID:          "user1",
		ThreadID:        thread.ID,
		Messages:        ag.buildLLMMessages("user1", "hello"),
		ToolCalls:       []llm.ToolCall{{ID: "call_1", Function: llm.FunctionCall{Name: "echo", Arguments: `{}`}}},
		NextIndex:       0,
		OriginalContent: "hello",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}
	ag.sessionManager.SavePendingExecution(pe)

	// 发送 __resume__ 应触发 Resume
	result, err := ag.runLLM(ctx, "user1", "__resume__")
	if err != nil {
		t.Fatalf("runLLM resume: %v", err)
	}
	if result.Content != "final summary" {
		t.Errorf("content = %q, want 'final summary'", result.Content)
	}
}

func TestAgentRunLLMDiscardOldPending(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{name: "echo", description: "echo", output: "echoed", approvalReq: gate.Never})

	dispatcher := tools.NewDispatcher(registry, nil, db.NewMemoryDB())
	dispatcher.WithGates()
	pendingStore := gate.NewPendingStore()

	ag := New(Config{Name: "TestAgent"}, Deps{
		LLM:          &mockLLM{respondWith: "normal response"},
		Tools:        registry,
		Dispatcher:   dispatcher,
		PendingStore: pendingStore,
	})
	ctx := context.Background()
	thread := ag.sessionManager.GetOrCreateThread("user1", "repl")

	pe := &PendingExecution{
		RequestID:       "test-req",
		UserID:          "user1",
		ThreadID:        thread.ID,
		Messages:        ag.buildLLMMessages("user1", "hello"),
		ToolCalls:       []llm.ToolCall{{ID: "call_1", Function: llm.FunctionCall{Name: "echo", Arguments: `{}`}}},
		NextIndex:       0,
		OriginalContent: "hello",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}
	ag.sessionManager.SavePendingExecution(pe)

	// 发送新消息（非 __resume__）应丢弃旧的 pending execution
	result, err := ag.runLLM(ctx, "user1", "new message")
	if err != nil {
		t.Fatalf("runLLM: %v", err)
	}
	if result.Content != "normal response" {
		t.Errorf("content = %q, want 'normal response'", result.Content)
	}
	if ag.sessionManager.GetPendingExecution("user1", thread.ID) != nil {
		t.Error("expected old pending execution to be discarded")
	}
}
