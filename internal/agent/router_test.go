package agent

import (
	"testing"
)

func TestRouterSystemCommand(t *testing.T) {
	r := NewRouter()

	cases := []struct {
		input       string
		wantType    IntentType
		wantCommand string
		wantArgs    []string
	}{
		{"/help", IntentSystemCmd, "help", nil},
		{"/threads", IntentSystemCmd, "threads", nil},
		{"/switch abc123", IntentSystemCmd, "switch", []string{"abc123"}},
		{"/new", IntentSystemCmd, "new", nil},
		{"/t", IntentSystemCmd, "t", nil},
		{"/s id1 id2", IntentSystemCmd, "s", []string{"id1", "id2"}},
	}

	for _, tc := range cases {
		intent := r.Route(tc.input)
		if intent.Type != tc.wantType {
			t.Errorf("Route(%q).Type = %v, want %v", tc.input, intent.Type, tc.wantType)
		}
		if intent.Command != tc.wantCommand {
			t.Errorf("Route(%q).Command = %q, want %q", tc.input, intent.Command, tc.wantCommand)
		}
		if len(intent.Args) != len(tc.wantArgs) {
			t.Errorf("Route(%q).Args = %v, want %v", tc.input, intent.Args, tc.wantArgs)
		}
	}
}

func TestRouterToolCall(t *testing.T) {
	r := NewRouter()

	intent := r.Route("tool:echo hello world")
	if intent.Type != IntentToolCall {
		t.Errorf("type = %v, want tool_call", intent.Type)
	}
	if intent.ToolName != "echo" {
		t.Errorf("tool name = %q, want echo", intent.ToolName)
	}
	if intent.ToolParams != "hello world" {
		t.Errorf("params = %q, want 'hello world'", intent.ToolParams)
	}

	// tool call without params
	intent = r.Route("tool:time")
	if intent.ToolName != "time" {
		t.Errorf("tool name = %q, want time", intent.ToolName)
	}
	if intent.ToolParams != "" {
		t.Errorf("params = %q, want empty", intent.ToolParams)
	}
}

func TestRouterLLMQuery(t *testing.T) {
	r := NewRouter()

	intent := r.Route("?what is the weather today")
	if intent.Type != IntentLLMQuery {
		t.Errorf("type = %v, want llm_query", intent.Type)
	}
	if intent.Content != "what is the weather today" {
		t.Errorf("content = %q", intent.Content)
	}
}

func TestRouterChat(t *testing.T) {
	r := NewRouter()

	intent := r.Route("hello there")
	if intent.Type != IntentChat {
		t.Errorf("type = %v, want chat", intent.Type)
	}
	if intent.Content != "hello there" {
		t.Errorf("content = %q", intent.Content)
	}
}

func TestRouterTrimSpace(t *testing.T) {
	r := NewRouter()

	intent := r.Route("  /help  ")
	if intent.Command != "help" {
		t.Errorf("command = %q, want help", intent.Command)
	}
}
