package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Env != "development" {
		t.Errorf("expected env development, got %q", cfg.Env)
	}
	if cfg.Agent.Name != "IronClaw" {
		t.Errorf("expected agent name IronClaw, got %q", cfg.Agent.Name)
	}
	if cfg.Database.Driver != "memory" {
		t.Errorf("expected memory driver, got %q", cfg.Database.Driver)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("IRONCLAW_OWNER_ID", "test_owner")
	os.Setenv("IRONCLAW_AGENT_NAME", "TestAgent")
	os.Setenv("IRONCLAW_AGENT_MAX_PARALLEL_JOBS", "8")
	os.Setenv("IRONCLAW_DATABASE_DRIVER", "postgres")
	defer func() {
		os.Unsetenv("IRONCLAW_OWNER_ID")
		os.Unsetenv("IRONCLAW_AGENT_NAME")
		os.Unsetenv("IRONCLAW_AGENT_MAX_PARALLEL_JOBS")
		os.Unsetenv("IRONCLAW_DATABASE_DRIVER")
	}()

	cfg := DefaultConfig()
	if err := cfg.loadFromEnv(); err != nil {
		t.Fatalf("loadFromEnv failed: %v", err)
	}

	if cfg.OwnerID != "test_owner" {
		t.Errorf("expected owner_id test_owner, got %q", cfg.OwnerID)
	}
	if cfg.Agent.Name != "TestAgent" {
		t.Errorf("expected agent name TestAgent, got %q", cfg.Agent.Name)
	}
	if cfg.Agent.MaxParallelJobs != 8 {
		t.Errorf("expected max parallel jobs 8, got %d", cfg.Agent.MaxParallelJobs)
	}
	if cfg.Database.Driver != "postgres" {
		t.Errorf("expected postgres driver, got %q", cfg.Database.Driver)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid development",
			cfg: Config{
				Env:      "development",
				Database: DatabaseConfig{Driver: "memory"},
				Agent:    AgentConfig{MaxParallelJobs: 4},
			},
			wantErr: false,
		},
		{
			name: "valid production",
			cfg: Config{
				Env:      "production",
				Database: DatabaseConfig{Driver: "postgres", DSN: "postgres://localhost/db"},
				Agent:    AgentConfig{MaxParallelJobs: 4},
			},
			wantErr: false,
		},
		{
			name: "invalid env",
			cfg: Config{
				Env:      "invalid",
				Database: DatabaseConfig{Driver: "memory"},
			},
			wantErr: true,
		},
		{
			name: "invalid driver",
			cfg: Config{
				Env:      "development",
				Database: DatabaseConfig{Driver: "mysql"},
			},
			wantErr: true,
		},
		{
			name: "postgres without dsn",
			cfg: Config{
				Env:      "development",
				Database: DatabaseConfig{Driver: "postgres"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
