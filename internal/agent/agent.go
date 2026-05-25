package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// Agent — the core reasoning and action loop.
type Agent struct {
	config Config
	deps   Deps
}

func New(config Config, deps Deps) *Agent {
	return &Agent{config: config, deps: deps}
}

// ProcessMessage handles a single user turn.
// MVP: no LLM integration — detects tool calls via simple prefix or falls back to echo.
func (a *Agent) ProcessMessage(ctx context.Context, msg channels.IncomingMessage) (channels.OutgoingResponse, error) {
	content := strings.TrimSpace(msg.Content)

	// Simple tool-call detection: "tool:echo {"message":"hi"}"
	if strings.HasPrefix(content, "tool:") {
		return a.handleToolInvocation(ctx, msg.UserID, content)
	}

	// Fallback: echo the input with agent name
	return channels.OutgoingResponse{
		Content: fmt.Sprintf("[%s] You said: %s", a.config.Name, content),
	}, nil
}

func (a *Agent) handleToolInvocation(ctx context.Context, userID, content string) (channels.OutgoingResponse, error) {
	// Parse "tool:NAME JSON"
	rest := strings.TrimPrefix(content, "tool:")
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) < 1 {
		return channels.OutgoingResponse{}, fmt.Errorf("usage: tool:<name> <json params>")
	}

	toolName := strings.TrimSpace(parts[0])
	var params map[string]any
	if len(parts) == 2 {
		// MVP: skip real JSON parsing; just pass raw string as "message"
		params = map[string]any{"message": strings.TrimSpace(parts[1])}
	}

	out, err := a.deps.Dispatcher.Dispatch(ctx, toolName, params, &tools.JobContext{
		UserID: userID,
	})
	if err != nil {
		return channels.OutgoingResponse{Content: fmt.Sprintf("Error: %v", err)}, nil
	}
	return channels.OutgoingResponse{Content: out.Content}, nil
}

// Run blocks forever, consuming messages from the channel manager.
func (a *Agent) Run(ctx context.Context, mgr *channels.Manager) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := mgr.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		resp, err := a.ProcessMessage(ctx, msg)
		if err != nil {
			resp = channels.OutgoingResponse{Content: fmt.Sprintf("Agent error: %v", err)}
		}

		_ = mgr.Broadcast(ctx, resp)
	}
}
