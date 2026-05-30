package agent

import (
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/document"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/hooks"
	"github.com/nearai/ironclaw-go/internal/llm"
	"github.com/nearai/ironclaw-go/internal/skills"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// Config — Agent 行为配置。
type Config struct {
	Name             string
	MaxParallelJobs  int
	AutoApproveTools bool
}

// Deps — 注入的依赖。
type Deps struct {
	OwnerID            string
	Database           db.Database
	LLM                llm.LlmProvider
	Tools              *tools.Registry
	Dispatcher         *tools.Dispatcher
	PendingStore       *gate.PendingStore
	Hooks              *hooks.Registry
	Skills             *skills.Registry
	DocumentMiddleware *document.Middleware
	EventPublisher     func(ev channels.Event)
}
