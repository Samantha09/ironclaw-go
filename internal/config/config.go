package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// DatabaseConfig 数据库连接配置。
type DatabaseConfig struct {
	Driver   string `toml:"driver"`   // memory, postgres, libsql
	DSN      string `toml:"dsn"`      // 连接字符串
	MaxConns int    `toml:"max_conns"`
	MinConns int    `toml:"min_conns"`
}

// LLMConfig LLM 提供商配置。
type LLMConfig struct {
	Provider    string            `toml:"provider"`     // openai, anthropic, ollama
	Model       string            `toml:"model"`
	APIKey      string            `toml:"api_key"`
	BaseURL     string            `toml:"base_url"`
	TimeoutSec  int               `toml:"timeout_sec"`
	MaxTokens   int               `toml:"max_tokens"`
	Temperature float64           `toml:"temperature"`
	Extra       map[string]string `toml:"extra"`
}

// ChannelConfig 通道配置。
type ChannelConfig struct {
	REPL     bool `toml:"repl"`
	HTTP     bool `toml:"http"`
	WebSocket bool `toml:"websocket"`
	HTTPPort int  `toml:"http_port"`
}

// SafetyConfig 安全配置。
type SafetyConfig struct {
	ScanInbound    bool `toml:"scan_inbound"`
	ScanOutbound   bool `toml:"scan_outbound"`
	MaxToolCallsPerMin int `toml:"max_tool_calls_per_min"`
}

// SidecarConfig WASM sidecar 配置。
type SidecarConfig struct {
	Address     string `toml:"address"`      // Unix 域套接字路径或 TCP 地址
	MaxMemoryMB int    `toml:"max_memory_mb"`
	MaxFuel     uint64 `toml:"max_fuel"`
}

// AgentConfig Agent 行为与身份配置。
type AgentConfig struct {
	Name             string   `toml:"name"`
	MaxParallelJobs  int      `toml:"max_parallel_jobs"`
	AutoApproveTools bool     `toml:"auto_approve_tools"`
	AllowedTools     []string `toml:"allowed_tools"`
}

// Config 顶层应用配置。
type Config struct {
	OwnerID   string         `toml:"owner_id"`
	Env       string         `toml:"env"`        // development, staging, production
	LogLevel  string         `toml:"log_level"`  // trace, debug, info, warn, error
	Agent     AgentConfig    `toml:"agent"`
	Database  DatabaseConfig `toml:"database"`
	LLM       LLMConfig      `toml:"llm"`
	Channels  ChannelConfig  `toml:"channels"`
	Safety    SafetyConfig   `toml:"safety"`
	Sidecar   SidecarConfig  `toml:"sidecar"`
}

// DefaultConfig 返回带有合理默认值的配置。
func DefaultConfig() Config {
	return Config{
		OwnerID:  "owner",
		Env:      "development",
		LogLevel: "info",
		Agent: AgentConfig{
			Name:             "IronClaw",
			MaxParallelJobs:  4,
			AutoApproveTools: false,
		},
		Database: DatabaseConfig{
			Driver:   "memory",
			MaxConns: 10,
			MinConns: 2,
		},
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			TimeoutSec:  60,
			MaxTokens:   4096,
			Temperature: 0.7,
		},
		Channels: ChannelConfig{
			REPL:      true,
			HTTP:      false,
			WebSocket: false,
			HTTPPort:  8080,
		},
		Safety: SafetyConfig{
			ScanInbound:        true,
			ScanOutbound:       true,
			MaxToolCallsPerMin: 60,
		},
		Sidecar: SidecarConfig{
			Address:     "/tmp/ironclaw-sidecar.sock",
			MaxMemoryMB: 128,
			MaxFuel:     10_000_000_000,
		},
	}
}

// Load 按优先级加载配置：defaults < env < TOML < DB（DB 层后续实现）。
func Load(tomlPath string) (Config, error) {
	cfg := DefaultConfig()

	// 第二层：环境变量
	if err := cfg.loadFromEnv(); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}

	// 第三层：TOML 文件
	if tomlPath != "" {
		if _, err := os.Stat(tomlPath); err == nil {
			if err := cfg.loadFromFile(tomlPath); err != nil {
				return Config{}, fmt.Errorf("load toml %q: %w", tomlPath, err)
			}
		}
	} else {
		// 尝试默认路径
		for _, p := range defaultConfigPaths() {
			if _, err := os.Stat(p); err == nil {
				if err := cfg.loadFromFile(p); err != nil {
					return Config{}, fmt.Errorf("load toml %q: %w", p, err)
				}
				break
			}
		}
	}

	// 第四层：DB 配置（后续实现）
	// cfg.loadFromDB(database)

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// MustLoad 与 Load 相同，但在出错时 panic（仅用于 main 入口）。
func MustLoad(tomlPath string) Config {
	cfg, err := Load(tomlPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}

func (c *Config) loadFromEnv() error {
	if v := os.Getenv("IRONCLAW_OWNER_ID"); v != "" {
		c.OwnerID = v
	}
	if v := os.Getenv("IRONCLAW_ENV"); v != "" {
		c.Env = v
	}
	if v := os.Getenv("IRONCLAW_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}

	// Agent
	if v := os.Getenv("IRONCLAW_AGENT_NAME"); v != "" {
		c.Agent.Name = v
	}
	if v := os.Getenv("IRONCLAW_AGENT_MAX_PARALLEL_JOBS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid IRONCLAW_AGENT_MAX_PARALLEL_JOBS: %w", err)
		}
		c.Agent.MaxParallelJobs = n
	}
	if v := os.Getenv("IRONCLAW_AGENT_AUTO_APPROVE_TOOLS"); v == "true" {
		c.Agent.AutoApproveTools = true
	}

	// Database
	if v := os.Getenv("IRONCLAW_DATABASE_DRIVER"); v != "" {
		c.Database.Driver = v
	}
	if v := os.Getenv("IRONCLAW_DATABASE_DSN"); v != "" {
		c.Database.DSN = v
	}

	// LLM
	if v := os.Getenv("IRONCLAW_LLM_PROVIDER"); v != "" {
		c.LLM.Provider = v
	}
	if v := os.Getenv("IRONCLAW_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("IRONCLAW_LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("IRONCLAW_LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}

	// Channels
	if v := os.Getenv("IRONCLAW_CHANNELS_HTTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid IRONCLAW_CHANNELS_HTTP_PORT: %w", err)
		}
		c.Channels.HTTPPort = n
	}

	// Sidecar
	if v := os.Getenv("IRONCLAW_SIDECAR_ADDRESS"); v != "" {
		c.Sidecar.Address = v
	}

	return nil
}

func (c *Config) loadFromFile(path string) error {
	meta, err := toml.DecodeFile(path, c)
	if err != nil {
		return fmt.Errorf("decode toml: %w", err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return fmt.Errorf("unknown config keys: %s", strings.Join(keys, ", "))
	}
	return nil
}

func (c *Config) validate() error {
	switch c.Env {
	case "development", "staging", "production":
		// ok
	default:
		return fmt.Errorf("invalid env: %q", c.Env)
	}

	switch c.Database.Driver {
	case "memory", "postgres", "libsql":
		// ok
	default:
		return fmt.Errorf("invalid database driver: %q", c.Database.Driver)
	}

	if c.Database.Driver != "memory" && c.Database.DSN == "" {
		return fmt.Errorf("database DSN required for driver %q", c.Database.Driver)
	}

	if c.Agent.MaxParallelJobs <= 0 {
		c.Agent.MaxParallelJobs = 1
	}

	return nil
}

func defaultConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"ironclaw.toml",
		".ironclaw.toml",
		filepath.Join(home, ".config", "ironclaw", "config.toml"),
		filepath.Join(home, ".ironclaw", "config.toml"),
	}
}
