package agent

import "github.com/nearai/ironclaw-go/internal/tools"

// Config — agent behavior.
type Config struct {
	Name             string
	MaxParallelJobs  int
	AutoApproveTools bool
}

// Deps — injected dependencies.
type Deps struct {
	OwnerID    string
	Tools      *tools.Registry
	Dispatcher *tools.Dispatcher
}
