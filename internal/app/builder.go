package app

import (
	"context"
	"fmt"

	"github.com/nearai/ironclaw-go/internal/agent"
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/channels/repl"
	"github.com/nearai/ironclaw-go/internal/config"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/safety"
	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/tools/builtin"
)

// App — fully wired application.
type App struct {
	Config   config.Config
	DB       db.Database
	Agent    *agent.Agent
	Channels *channels.Manager
}

// Build wires all components.
func Build(cfg config.Config) (*App, error) {
	// Database
	database := db.NewMemoryDB()

	// Tools
	registry := tools.NewRegistry()
	registry.Register(builtin.NewEchoTool())
	registry.Register(builtin.NewTimeTool())
	registry.Register(builtin.NewJSONTool())
	registry.Register(builtin.NewShellTool())
	registry.Register(builtin.NewFileTool())
	registry.Register(builtin.NewHTTPTool())
	registry.Register(builtin.NewMemoryTool())

	// Safety + Dispatcher
	safetyLayer := safety.NewLayer()
	dispatcher := tools.NewDispatcher(registry, safetyLayer)

	// Agent
	agentDeps := agent.Deps{
		OwnerID:    cfg.OwnerID,
		Database:   database,
		Tools:      registry,
		Dispatcher: dispatcher,
	}
	ag := agent.New(agent.Config{
		Name:             cfg.Agent.Name,
		MaxParallelJobs:  cfg.Agent.MaxParallelJobs,
		AutoApproveTools: cfg.Agent.AutoApproveTools,
	}, agentDeps)

	// Channels
	mgr := channels.NewManager()
	replCh := repl.New(cfg.OwnerID)
	mgr.Add(replCh)
	mgr.Start(context.Background())

	return &App{
		Config:   cfg,
		DB:       database,
		Agent:    ag,
		Channels: mgr,
	}, nil
}

// Run starts the agent loop.
func (a *App) Run(ctx context.Context) error {
	fmt.Printf("IronClaw %s starting...\n", a.Config.Agent.Name)
	fmt.Printf("Channels: %v\n", a.Channels.Names())
	fmt.Println("Type 'quit' or 'exit' to stop.")
	fmt.Println()

	return a.Agent.Run(ctx, a.Channels)
}
