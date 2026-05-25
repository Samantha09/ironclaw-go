package config

import (
	"fmt"
	"os"
	"strconv"
)

// AgentConfig — behavior and identity.
type AgentConfig struct {
	Name             string
	MaxParallelJobs  int
	AutoApproveTools bool
}

// Config — top-level application configuration.
type Config struct {
	OwnerID string
	Agent   AgentConfig
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		OwnerID: "owner",
		Agent: AgentConfig{
			Name:             "IronClaw",
			MaxParallelJobs:  4,
			AutoApproveTools: false,
		},
	}
}

// LoadFromEnv overlays environment variables onto defaults.
func LoadFromEnv() (Config, error) {
	cfg := DefaultConfig()

	if v := os.Getenv("IRONCLAW_OWNER_ID"); v != "" {
		cfg.OwnerID = v
	}
	if v := os.Getenv("IRONCLAW_AGENT_NAME"); v != "" {
		cfg.Agent.Name = v
	}
	if v := os.Getenv("IRONCLAW_AGENT_MAX_PARALLEL_JOBS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid IRONCLAW_AGENT_MAX_PARALLEL_JOBS: %w", err)
		}
		cfg.Agent.MaxParallelJobs = n
	}
	if v := os.Getenv("IRONCLAW_AGENT_AUTO_APPROVE_TOOLS"); v == "true" {
		cfg.Agent.AutoApproveTools = true
	}

	return cfg, nil
}
