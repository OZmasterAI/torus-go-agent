package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// MCPServerConfig defines an MCP server to connect to.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Config is the top-level application config.
type Config struct {
	Telegram   TelegramConfig             `json:"telegram"`
	Agent      AgentConfig                `json:"agent"`
	Data       DataConfig                 `json:"data"`
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	SkillsDir  string                     `json:"skillsDir"`
}

// TelegramConfig holds Telegram bot settings.
type TelegramConfig struct {
	BotToken     string  `json:"botToken"`
	AllowedUsers []int64 `json:"allowedUsers"`
}

// AgentConfig holds agent/model settings.
type AgentConfig struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	MaxTokens         int    `json:"maxTokens"`
	Compaction        string `json:"compaction"`
	CompactionModel   string `json:"compactionModel"`
	SmartRouting      bool   `json:"smartRouting"`
	SmartRoutingModel string `json:"smartRoutingModel"`
}

// DataConfig holds data directory settings.
type DataConfig struct {
	Dir string `json:"dir"`
}

// LoadConfig reads and parses a JSON config file with env var overrides.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Defaults
	if cfg.Agent.MaxTokens == 0 {
		cfg.Agent.MaxTokens = 8192
	}
	if cfg.Agent.Compaction == "" {
		cfg.Agent.Compaction = "llm"
	}
	// Env overrides
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		cfg.Agent.Model = v
	}
	if v := os.Getenv("AGENT_PROVIDER"); v != "" {
		cfg.Agent.Provider = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if cfg.Telegram.BotToken == "ENV" {
		cfg.Telegram.BotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	return &cfg, nil
}

// LoadTorus reads the TORUS.md persona file from configDir.
func LoadTorus(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "TORUS.md"))
	if err != nil {
		return "You are an AI assistant with access to tools."
	}
	return string(data)
}

// DataDir resolves the data directory path relative to configDir.
func (c *Config) DataDir(configDir string) string {
	d := c.Data.Dir
	if d == "" {
		d = "../data"
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(configDir, d)
	}
	return d
}

// APIKey resolves the API key from environment based on provider.
func (c *Config) APIKey() string {
	switch strings.ToLower(c.Agent.Provider) {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	default:
		return os.Getenv("OPENROUTER_API_KEY")
	}
}
