package config

import (
	"encoding/json"
	"net/http"
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

// RoutingEntry defines a weighted provider for multi-provider routing.
type RoutingEntry struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Weight   int    `json:"weight"`
}

// AgentConfig holds agent/model settings.
type AgentConfig struct {
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	Routing           []RoutingEntry `json:"routing,omitempty"`       // weighted multi-provider routing
	FallbackOrder     []string       `json:"fallbackOrder,omitempty"` // "provider:model" keys in fallback order
	MaxTokens         int    `json:"maxTokens"`     // max output tokens per response
	ContextWindow         int    `json:"contextWindow"`         // model's full context window size
	Compaction            string `json:"compaction"`
	CompactionModel       string `json:"compactionModel"`
	CompactionTrigger     string `json:"compactionTrigger"`     // "tokens", "messages", or "both"
	CompactionMaxMessages int    `json:"compactionMaxMessages"` // max messages before compaction (0 = disabled)
	CompactionThreshold   int    `json:"compactionThreshold"`   // % of contextWindow that triggers compaction (default 80)
	CompactionKeepLastN   int    `json:"compactionKeepLastN"`   // messages kept verbatim after compaction (default 10)
	ContinuousCompression  bool   `json:"continuousCompression"`  // enable per-turn gradual message compression
	CompressionKeepLast    int    `json:"compressionKeepLast"`    // messages always kept verbatim by continuous compression (default 10)
	CompressionMinMessages int    `json:"compressionMinMessages"` // don't compress until this many messages (0 = compress from keepLast+1)
	ZoneBudgeting         bool   `json:"zoneBudgeting"`         // enable zone-based token budget allocation
	ZoneArchivePercent    int    `json:"zoneArchivePercent"`    // % of usable budget for archive zone (default 30)
	SmartRouting          bool   `json:"smartRouting"`
	SmartRoutingModel string `json:"smartRoutingModel"`
	SteeringMode      string `json:"steeringMode,omitempty"` // "mild" (default) or "aggressive"
	PersistThinking   bool   `json:"persistThinking"`        // store thinking blocks as DAG nodes
	AzureResource    string `json:"azureResource,omitempty"`   // Azure OpenAI resource name
	AzureDeployment  string `json:"azureDeployment,omitempty"` // Azure OpenAI deployment name
	AzureAPIVersion  string `json:"azureApiVersion,omitempty"` // Azure API version (default "2024-06-01")
	VertexProject    string `json:"vertexProject,omitempty"`   // Google Cloud project ID
	VertexRegion     string `json:"vertexRegion,omitempty"`    // Google Cloud region (e.g. "us-central1")
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
	if cfg.Agent.ContextWindow == 0 {
		cfg.Agent.ContextWindow = 128000 // safe default for most models
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

// ModelInfo holds context window and max tokens for a model.
type ModelInfo struct {
	ContextWindow int `json:"contextWindow"`
	MaxTokens     int `json:"maxTokens"`
}

// LoadModels reads models.json from configDir and returns a map of model ID to ModelInfo.
func LoadModels(configDir string) map[string]ModelInfo {
	data, err := os.ReadFile(filepath.Join(configDir, "models.json"))
	if err != nil {
		return nil
	}
	var models map[string]ModelInfo
	json.Unmarshal(data, &models)
	return models
}

// ResolveModelInfo looks up model specs in this order:
// 1. models.json cache
// 2. OpenRouter API (any provider — universal model registry)
// 3. Returns zeros if not found (caller uses code defaults)
//
// When the API returns a result, it is cached to models.json for future startups.
func ResolveModelInfo(modelID, provider string, models map[string]ModelInfo, configDir string) ModelInfo {
	// 1. Local cache
	if models != nil {
		if info, ok := models[modelID]; ok {
			return info
		}
	}

	// 2. OpenRouter API (universal registry)
	if info, ok := fetchOpenRouterModelInfo(modelID, provider); ok {
		if configDir != "" {
			if models == nil {
				models = make(map[string]ModelInfo)
			}
			models[modelID] = info
			saveModelsCache(configDir, models)
		}
		return info
	}

	return ModelInfo{}
}

// openRouterPrefix maps our provider keys to OpenRouter ID prefixes.
var openRouterPrefix = map[string]string{
	"anthropic": "anthropic",
	"openai":    "openai",
	"grok":      "x-ai",
	"gemini":    "google",
	"vertex":    "google",
	"azure":     "openai",
}

// saveModelsCache writes the models map to models.json.
func saveModelsCache(configDir string, models map[string]ModelInfo) {
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(configDir, "models.json"), append(data, '\n'), 0644)
}

// fetchOpenRouterModelInfo queries OpenRouter's /api/v1/models.
// It tries the exact modelID first, then prefixed with the provider (e.g. "anthropic/claude-sonnet-4-6").
func fetchOpenRouterModelInfo(modelID, provider string) (ModelInfo, bool) {
	resp, err := http.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		return ModelInfo{}, false
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			TopProvider   struct {
				MaxCompletionTokens *int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return ModelInfo{}, false
	}

	// Build candidate IDs: exact match first, then provider-prefixed
	candidates := []string{modelID}
	if prefix, ok := openRouterPrefix[strings.ToLower(provider)]; ok {
		candidates = append(candidates, prefix+"/"+modelID)
	}

	for _, candidate := range candidates {
		for _, m := range result.Data {
			if m.ID == candidate {
				info := ModelInfo{ContextWindow: m.ContextLength}
				if m.TopProvider.MaxCompletionTokens != nil {
					info.MaxTokens = *m.TopProvider.MaxCompletionTokens
				}
				if info.MaxTokens == 0 {
					info.MaxTokens = 8192
				}
				return info, true
			}
		}
	}
	return ModelInfo{}, false
}

// LoadTorus reads the TORUS.md persona file from configDir.
func LoadTorus(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "TORUS.md"))
	if err != nil {
		return "You are an AI assistant with access to tools."
	}
	return string(data)
}

// DataDir resolves the data directory. If configured, uses that (relative to
// configDir if not absolute). Otherwise uses $XDG_DATA_HOME/torus_go_agent
// or ~/.local/share/torus_go_agent.
func (c *Config) DataDir(configDir string) string {
	d := c.Data.Dir
	if d != "" {
		if !filepath.IsAbs(d) {
			d = filepath.Join(configDir, d)
		}
		return d
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "torus_go_agent")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(configDir, "data")
	}
	return filepath.Join(home, ".local", "share", "torus_go_agent")
}

// APIKey resolves the API key from environment based on provider.
func (c *Config) APIKey() string {
	return APIKeyFor(c.Agent.Provider)
}

// APIKeyFor resolves the API key for a given provider name.
func APIKeyFor(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "nvidia":
		return os.Getenv("NVIDIA_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "grok":
		return os.Getenv("XAI_API_KEY")
	case "azure":
		return os.Getenv("AZURE_OPENAI_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	case "vertex":
		return os.Getenv("VERTEX_ACCESS_TOKEN")
	default:
		return os.Getenv("OPENROUTER_API_KEY")
	}
}

// LoadSchema reads the SCHEMA.md architecture file from configDir.
func LoadSchema(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "SCHEMA.md"))
	if err != nil {
		return ""
	}
	return string(data)
}
