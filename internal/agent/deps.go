package agent

import (
	"github.com/nearai/ironclaw-go/internal/db"
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
	OwnerID    string
	Database   db.Database
	Tools      *tools.Registry
	Dispatcher *tools.Dispatcher
}
