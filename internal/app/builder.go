package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nearai/ironclaw-go/internal/agent"
	"github.com/nearai/ironclaw-go/internal/auth"
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/channels/httpgw"
	"github.com/nearai/ironclaw-go/internal/channels/repl"
	"github.com/nearai/ironclaw-go/internal/channels/websocket"
	"github.com/nearai/ironclaw-go/internal/config"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/document"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/history"
	"github.com/nearai/ironclaw-go/internal/hooks"
	"github.com/nearai/ironclaw-go/internal/llm"
	"github.com/nearai/ironclaw-go/internal/observability"
	"time"

	"github.com/nearai/ironclaw-go/internal/safety"
	"github.com/nearai/ironclaw-go/internal/secrets"
	"github.com/nearai/ironclaw-go/internal/skills"
	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/tools/builtin"
	"github.com/nearai/ironclaw-go/internal/webhooks"
	"github.com/nearai/ironclaw-go/internal/worker"
	"github.com/nearai/ironclaw-go/internal/workspace"
)

// App — fully wired application.
type App struct {
	Config   config.Config
	DB       db.Database
	Agent    *agent.Agent
	Channels *channels.Manager
	Logger   *observability.Logger
	Worker   *worker.Pool
}

// Build wires all components.
func Build(cfg config.Config) (*App, error) {
	// Logger
	logger := observability.NewLogger(cfg.LogLevel)

	// Database
	database, err := db.New(cfg.Database.Driver, cfg.Database.DSN, cfg.Database.MaxConns, cfg.Database.MinConns)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// Secrets
	_, _ = secrets.NewStoreFromEnv()

	// Workspace
	ws := workspace.NewFSWorkspace("./workspaces")

	// Tools
	registry := tools.NewRegistry()
	registry.Register(builtin.NewEchoTool())
	registry.Register(builtin.NewTimeTool())
	registry.Register(builtin.NewJSONTool())
	registry.Register(builtin.NewShellTool())
	registry.Register(builtin.NewFileTool(ws))
	registry.Register(builtin.NewHTTPTool())
	registry.Register(builtin.NewMemoryTool())

	// Skills
	skillRegistry := skills.NewRegistry()

	// History
	histStore := history.NewStore(database)

	// LLM
	var llmProvider llm.LlmProvider
	if cfg.LLM.APIKey != "" || cfg.LLM.Provider == "ollama" {
		p, err := llm.New(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKey, cfg.LLM.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("init llm: %w", err)
		}
		llmProvider = p
	}

	// Hooks
	hookRegistry := hooks.NewRegistry()

	// Safety + Dispatcher
	safetyLayer := safety.NewLayerWithConfig(safety.Config{
		MaxOutputLength: 10000,
		RateMaxCalls:    100,
		RateWindow:      time.Minute,
	})
	dispatcher := tools.NewDispatcher(registry, safetyLayer, database)

	// Gate: 审批门控（基于风险策略）
	riskEvaluator := gate.NewRiskBasedEvaluator(gate.TrustBalanced)
	approvalGate := gate.NewApprovalGate(func(toolName string, params map[string]any, userID string) gate.ApprovalRequirement {
		return riskEvaluator.Evaluate(toolName, params, userID)
	})
	dispatcher.WithGates(approvalGate)
	if cfg.Agent.AutoApproveTools {
		autoList := registry.List()
		dispatcher.WithAutoApproved(autoList)
		logger.Info("Auto-approve all tools enabled")
	}
	if len(cfg.Agent.AllowedTools) > 0 {
		logger.Info("Allowed tools restricted", slog.Any("tools", cfg.Agent.AllowedTools))
	}

	// Document extraction middleware
	docMiddleware := document.NewMiddleware()

	// Agent
	agentDeps := agent.Deps{
		OwnerID:            cfg.OwnerID,
		Database:           database,
		LLM:                llmProvider,
		Tools:              registry,
		Dispatcher:         dispatcher,
		PendingStore:       dispatcher.PendingStore(),
		Hooks:              hookRegistry,
		Skills:             skillRegistry,
		DocumentMiddleware: docMiddleware,
	}
	ag := agent.New(agent.Config{
		Name:             cfg.Agent.Name,
		MaxParallelJobs:  cfg.Agent.MaxParallelJobs,
		AutoApproveTools: cfg.Agent.AutoApproveTools,
	}, agentDeps)

	// Auth
	var authenticator auth.Authenticator
	if cfg.APIKey != "" {
		authenticator = auth.NewAPIKeyAuth(map[string]string{cfg.APIKey: cfg.OwnerID})
		logger.Info("API Key authentication enabled")
	} else {
		authenticator = auth.NewNoAuth()
	}

	// Channels
	mgr := channels.NewManager()
	replCh := repl.New(cfg.OwnerID)
	mgr.Add(replCh)

	if cfg.Channels.HTTP {
		gw := httpgw.New(cfg.Channels.HTTPPort).WithAuth(authenticator).WithHistory(histStore).WithVersion(cfg.Env).WithPendingStore(dispatcher.PendingStore()).WithRiskEvaluator(riskEvaluator)
		gw.RegisterHealthCheck("database", func(ctx context.Context) error {
			return database.Ping(ctx)
		})
		gw.RegisterHealthCheck("agent", func(ctx context.Context) error {
			if ag == nil {
				return fmt.Errorf("agent not initialized")
			}
			return nil
		})
		gw.Start()
		mgr.Add(gw)
		logger.Info("HTTP Gateway started", slog.Int("port", cfg.Channels.HTTPPort))
	}

	// Webhook 服务器（固定使用 HTTPPort+1）
	wh := webhooks.NewServer(cfg.Channels.HTTPPort + 1)
	wh.Start()
	mgr.Add(wh)
	logger.Info("Webhook server started", slog.Int("port", cfg.Channels.HTTPPort+1))

	if cfg.Channels.WebSocket {
		wsCh := websocket.New(cfg.Channels.WebSocketPort).WithAuth(authenticator)
		wsCh.Start()
		mgr.Add(wsCh)
		logger.Info("WebSocket server started", slog.Int("port", cfg.Channels.WebSocketPort))
	}

	mgr.Start(context.Background())

	// Worker Pool
	wp := worker.NewPool(database, dispatcher, cfg.Agent.MaxParallelJobs)
	wp.Start(context.Background())
	logger.Info("Worker pool started", slog.Int("max_parallel", cfg.Agent.MaxParallelJobs))

	return &App{
		Config:   cfg,
		DB:       database,
		Agent:    ag,
		Channels: mgr,
		Logger:   logger,
		Worker:   wp,
	}, nil
}

// Run starts the agent loop and blocks until context cancellation.
func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("IronClaw starting",
		slog.String("name", a.Config.Agent.Name),
		slog.Any("channels", a.Channels.Names()),
	)
	fmt.Println("Type 'quit' or 'exit' to stop.")
	fmt.Println()

	err := a.Agent.Run(ctx, a.Channels)
	a.Worker.Stop()
	return err
}
