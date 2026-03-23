package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfig_BasicParsing tests basic JSON parsing from a config file.
func TestLoadConfig_BasicParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		Data: DataConfig{
			Dir: "/tmp/data",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.Provider != "anthropic" {
		t.Errorf("Provider: got %q, want %q", loaded.Agent.Provider, "anthropic")
	}
	if loaded.Agent.Model != "claude-3-sonnet-20250219" {
		t.Errorf("Model: got %q, want %q", loaded.Agent.Model, "claude-3-sonnet-20250219")
	}
	if loaded.Data.Dir != "/tmp/data" {
		t.Errorf("Data.Dir: got %q, want %q", loaded.Data.Dir, "/tmp/data")
	}
}

// TestLoadConfig_DefaultMaxTokens tests that MaxTokens defaults to 8192 when 0.
func TestLoadConfig_DefaultMaxTokens(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
			// MaxTokens is 0 (zero value)
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.MaxTokens != 8192 {
		t.Errorf("MaxTokens default: got %d, want 8192", loaded.Agent.MaxTokens)
	}
}

// TestLoadConfig_DefaultContextWindow tests that ContextWindow defaults to 128000 when 0.
func TestLoadConfig_DefaultContextWindow(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
			// ContextWindow is 0 (zero value)
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.ContextWindow != 128000 {
		t.Errorf("ContextWindow default: got %d, want 128000", loaded.Agent.ContextWindow)
	}
}

// TestLoadConfig_DefaultCompaction tests that Compaction defaults to "llm" when empty.
func TestLoadConfig_DefaultCompaction(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
			// Compaction is empty
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.Compaction != "llm" {
		t.Errorf("Compaction default: got %q, want %q", loaded.Agent.Compaction, "llm")
	}
}

// TestLoadConfig_ExplicitValues tests that explicit values override defaults.
func TestLoadConfig_ExplicitValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:      "openai",
			Model:         "gpt-4",
			MaxTokens:     4096,
			ContextWindow: 8000,
			Compaction:    "summarize",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.MaxTokens != 4096 {
		t.Errorf("MaxTokens: got %d, want 4096", loaded.Agent.MaxTokens)
	}
	if loaded.Agent.ContextWindow != 8000 {
		t.Errorf("ContextWindow: got %d, want 8000", loaded.Agent.ContextWindow)
	}
	if loaded.Agent.Compaction != "summarize" {
		t.Errorf("Compaction: got %q, want %q", loaded.Agent.Compaction, "summarize")
	}
}

// TestLoadConfig_EnvOverride_Model tests AGENT_MODEL environment variable override.
func TestLoadConfig_EnvOverride_Model(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-haiku-20250219",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	oldModel := os.Getenv("AGENT_MODEL")
	defer os.Setenv("AGENT_MODEL", oldModel)
	os.Setenv("AGENT_MODEL", "claude-3-opus-20250219")

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.Model != "claude-3-opus-20250219" {
		t.Errorf("Model env override: got %q, want %q", loaded.Agent.Model, "claude-3-opus-20250219")
	}
}

// TestLoadConfig_EnvOverride_Provider tests AGENT_PROVIDER environment variable override.
func TestLoadConfig_EnvOverride_Provider(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	oldProvider := os.Getenv("AGENT_PROVIDER")
	defer os.Setenv("AGENT_PROVIDER", oldProvider)
	os.Setenv("AGENT_PROVIDER", "openai")

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.Provider != "openai" {
		t.Errorf("Provider env override: got %q, want %q", loaded.Agent.Provider, "openai")
	}
}

// TestLoadConfig_EnvOverride_TelegramToken tests TELEGRAM_BOT_TOKEN environment variable override.
func TestLoadConfig_EnvOverride_TelegramToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		Telegram: TelegramConfig{
			BotToken: "old-token",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	oldToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer os.Setenv("TELEGRAM_BOT_TOKEN", oldToken)
	os.Setenv("TELEGRAM_BOT_TOKEN", "new-token")

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Telegram.BotToken != "new-token" {
		t.Errorf("TelegramToken env override: got %q, want %q", loaded.Telegram.BotToken, "new-token")
	}
}

// TestLoadConfig_FileNotFound tests error handling for missing config file.
func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Errorf("LoadConfig should fail for missing file")
	}
}

// TestLoadConfig_InvalidJSON tests error handling for malformed JSON.
func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{invalid json}`), 0644)

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Errorf("LoadConfig should fail for invalid JSON")
	}
}

// TestLoadConfig_WithRoutingAndFallback tests parsing of routing and fallback settings.
func TestLoadConfig_WithRoutingAndFallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
			Routing: []RoutingEntry{
				{Provider: "anthropic", Model: "claude-3-opus-20250219", Weight: 70},
				{Provider: "openai", Model: "gpt-4", Weight: 30},
			},
			FallbackOrder: []string{"anthropic:claude-3-sonnet-20250219", "openai:gpt-4"},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.Agent.Routing) != 2 {
		t.Errorf("Routing entries: got %d, want 2", len(loaded.Agent.Routing))
	}
	if loaded.Agent.Routing[0].Provider != "anthropic" {
		t.Errorf("First routing provider: got %q, want %q", loaded.Agent.Routing[0].Provider, "anthropic")
	}
	if loaded.Agent.Routing[0].Weight != 70 {
		t.Errorf("First routing weight: got %d, want 70", loaded.Agent.Routing[0].Weight)
	}

	if len(loaded.Agent.FallbackOrder) != 2 {
		t.Errorf("Fallback entries: got %d, want 2", len(loaded.Agent.FallbackOrder))
	}
}

// TestLoadModels_Success tests successful loading of models.json.
func TestLoadModels_Success(t *testing.T) {
	tmpDir := t.TempDir()
	modelsPath := filepath.Join(tmpDir, "models.json")

	models := map[string]ModelInfo{
		"claude-3-sonnet-20250219": {ContextWindow: 200000, MaxTokens: 4096},
		"gpt-4":                    {ContextWindow: 8000, MaxTokens: 2048},
	}

	data, _ := json.Marshal(models)
	os.WriteFile(modelsPath, data, 0644)

	loaded := LoadModels(tmpDir)
	if len(loaded) != 2 {
		t.Errorf("Models loaded: got %d, want 2", len(loaded))
	}

	if info, ok := loaded["claude-3-sonnet-20250219"]; ok {
		if info.ContextWindow != 200000 {
			t.Errorf("Claude context window: got %d, want 200000", info.ContextWindow)
		}
		if info.MaxTokens != 4096 {
			t.Errorf("Claude max tokens: got %d, want 4096", info.MaxTokens)
		}
	} else {
		t.Errorf("Model 'claude-3-sonnet-20250219' not found in loaded models")
	}
}

// TestLoadModels_FileNotFound tests that missing models.json returns nil (no error).
func TestLoadModels_FileNotFound(t *testing.T) {
	loaded := LoadModels("/nonexistent/path")
	if loaded != nil {
		t.Errorf("LoadModels should return nil for missing file, got %v", loaded)
	}
}

// TestLoadModels_InvalidJSON tests error handling for malformed models.json.
func TestLoadModels_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	modelsPath := filepath.Join(tmpDir, "models.json")
	os.WriteFile(modelsPath, []byte(`{invalid json}`), 0644)

	loaded := LoadModels(tmpDir)
	// LoadModels doesn't return error, just returns nil on parse failure
	if loaded != nil {
		t.Errorf("LoadModels should return nil for invalid JSON, got %v", loaded)
	}
}

// TestResolveModelInfo_LocalLookup tests local models.json lookup.
func TestResolveModelInfo_LocalLookup(t *testing.T) {
	models := map[string]ModelInfo{
		"claude-3-sonnet-20250219": {ContextWindow: 200000, MaxTokens: 4096},
	}

	info := ResolveModelInfo("claude-3-sonnet-20250219", "anthropic", models, "")
	if info.ContextWindow != 200000 {
		t.Errorf("Context window: got %d, want 200000", info.ContextWindow)
	}
	if info.MaxTokens != 4096 {
		t.Errorf("Max tokens: got %d, want 4096", info.MaxTokens)
	}
}

// TestResolveModelInfo_NotFound tests that zero ModelInfo is returned when model not found.
func TestResolveModelInfo_NotFound(t *testing.T) {
	models := map[string]ModelInfo{
		"claude-3-sonnet-20250219": {ContextWindow: 200000, MaxTokens: 4096},
	}

	info := ResolveModelInfo("unknown-model", "anthropic", models, "")
	if info.ContextWindow != 0 || info.MaxTokens != 0 {
		t.Errorf("NotFound should return zero ModelInfo, got %+v", info)
	}
}

// TestResolveModelInfo_NilModels tests handling when models map is nil.
func TestResolveModelInfo_NilModels(t *testing.T) {
	info := ResolveModelInfo("some-model", "anthropic", nil, "")
	if info.ContextWindow != 0 || info.MaxTokens != 0 {
		t.Errorf("Nil models should return zero ModelInfo, got %+v", info)
	}
}

// TestLoadTorus_Success tests successful loading of TORUS.md.
func TestLoadTorus_Success(t *testing.T) {
	tmpDir := t.TempDir()
	torusPath := filepath.Join(tmpDir, "TORUS.md")
	torusContent := "You are a helpful AI assistant with specific personality traits."
	os.WriteFile(torusPath, []byte(torusContent), 0644)

	loaded := LoadTorus(tmpDir)
	if loaded != torusContent {
		t.Errorf("TORUS content: got %q, want %q", loaded, torusContent)
	}
}

// TestLoadTorus_DefaultFallback tests default fallback when TORUS.md not found.
func TestLoadTorus_DefaultFallback(t *testing.T) {
	tmpDir := t.TempDir()
	defaultMsg := "You are an AI assistant with access to tools."

	loaded := LoadTorus(tmpDir)
	if loaded != defaultMsg {
		t.Errorf("TORUS default: got %q, want %q", loaded, defaultMsg)
	}
}

// TestLoadSchema_Success tests successful loading of SCHEMA.md.
func TestLoadSchema_Success(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "SCHEMA.md")
	schemaContent := "# Architecture\n\nDetailed system architecture..."
	os.WriteFile(schemaPath, []byte(schemaContent), 0644)

	loaded := LoadSchema(tmpDir)
	if loaded != schemaContent {
		t.Errorf("SCHEMA content: got %q, want %q", loaded, schemaContent)
	}
}

// TestLoadSchema_FileNotFound tests default fallback (empty string) when SCHEMA.md not found.
func TestLoadSchema_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	loaded := LoadSchema(tmpDir)
	if loaded != "" {
		t.Errorf("SCHEMA default: got %q, want empty string", loaded)
	}
}

// TestConfig_DataDir_ExplicitAbsolute tests DataDir with explicit absolute path.
func TestConfig_DataDir_ExplicitAbsolute(t *testing.T) {
	cfg := &Config{
		Data: DataConfig{
			Dir: "/absolute/path/to/data",
		},
	}

	result := cfg.DataDir("/some/config/dir")
	if result != "/absolute/path/to/data" {
		t.Errorf("DataDir absolute: got %q, want %q", result, "/absolute/path/to/data")
	}
}

// TestConfig_DataDir_ExplicitRelative tests DataDir with explicit relative path.
func TestConfig_DataDir_ExplicitRelative(t *testing.T) {
	configDir := "/etc/myapp"
	cfg := &Config{
		Data: DataConfig{
			Dir: "data",
		},
	}

	result := cfg.DataDir(configDir)
	expected := filepath.Join(configDir, "data")
	if result != expected {
		t.Errorf("DataDir relative: got %q, want %q", result, expected)
	}
}

// TestConfig_DataDir_XDGDataHome tests DataDir with XDG_DATA_HOME environment variable.
func TestConfig_DataDir_XDGDataHome(t *testing.T) {
	cfg := &Config{
		Data: DataConfig{
			Dir: "",
		},
	}

	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer os.Setenv("XDG_DATA_HOME", oldXDG)
	os.Setenv("XDG_DATA_HOME", "/custom/data/home")

	result := cfg.DataDir("/some/config/dir")
	expected := filepath.Join("/custom/data/home", "torus_go_agent")
	if result != expected {
		t.Errorf("DataDir XDG: got %q, want %q", result, expected)
	}
}

// TestConfig_DataDir_UserHome tests DataDir with user home directory fallback.
func TestConfig_DataDir_UserHome(t *testing.T) {
	cfg := &Config{
		Data: DataConfig{
			Dir: "",
		},
	}

	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer os.Setenv("XDG_DATA_HOME", oldXDG)
	os.Unsetenv("XDG_DATA_HOME")

	result := cfg.DataDir("/some/config/dir")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "share", "torus_go_agent")
	if result != expected {
		t.Errorf("DataDir home: got %q, want %q", result, expected)
	}
}

// TestConfig_APIKey tests APIKey method returns correct key from Provider.
func TestConfig_APIKey(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{
			Provider: "anthropic",
		},
	}

	oldKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", oldKey)
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")

	key := cfg.APIKey()
	if key != "test-anthropic-key" {
		t.Errorf("APIKey: got %q, want %q", key, "test-anthropic-key")
	}
}

// TestAPIKeyFor_Anthropic tests APIKeyFor for anthropic provider.
func TestAPIKeyFor_Anthropic(t *testing.T) {
	oldKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", oldKey)
	os.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	key := APIKeyFor("anthropic")
	if key != "anthropic-test-key" {
		t.Errorf("APIKeyFor anthropic: got %q, want %q", key, "anthropic-test-key")
	}
}

// TestAPIKeyFor_OpenAI tests APIKeyFor for openai provider.
func TestAPIKeyFor_OpenAI(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	defer os.Setenv("OPENAI_API_KEY", oldKey)
	os.Setenv("OPENAI_API_KEY", "openai-test-key")

	key := APIKeyFor("openai")
	if key != "openai-test-key" {
		t.Errorf("APIKeyFor openai: got %q, want %q", key, "openai-test-key")
	}
}

// TestAPIKeyFor_NVIDIA tests APIKeyFor for nvidia provider.
func TestAPIKeyFor_NVIDIA(t *testing.T) {
	oldKey := os.Getenv("NVIDIA_API_KEY")
	defer os.Setenv("NVIDIA_API_KEY", oldKey)
	os.Setenv("NVIDIA_API_KEY", "nvidia-test-key")

	key := APIKeyFor("nvidia")
	if key != "nvidia-test-key" {
		t.Errorf("APIKeyFor nvidia: got %q, want %q", key, "nvidia-test-key")
	}
}

// TestAPIKeyFor_Grok tests APIKeyFor for grok provider (XAI).
func TestAPIKeyFor_Grok(t *testing.T) {
	oldKey := os.Getenv("XAI_API_KEY")
	defer os.Setenv("XAI_API_KEY", oldKey)
	os.Setenv("XAI_API_KEY", "xai-test-key")

	key := APIKeyFor("grok")
	if key != "xai-test-key" {
		t.Errorf("APIKeyFor grok: got %q, want %q", key, "xai-test-key")
	}
}

// TestAPIKeyFor_Azure tests APIKeyFor for azure provider.
func TestAPIKeyFor_Azure(t *testing.T) {
	oldKey := os.Getenv("AZURE_OPENAI_API_KEY")
	defer os.Setenv("AZURE_OPENAI_API_KEY", oldKey)
	os.Setenv("AZURE_OPENAI_API_KEY", "azure-test-key")

	key := APIKeyFor("azure")
	if key != "azure-test-key" {
		t.Errorf("APIKeyFor azure: got %q, want %q", key, "azure-test-key")
	}
}

// TestAPIKeyFor_Gemini tests APIKeyFor for gemini provider.
func TestAPIKeyFor_Gemini(t *testing.T) {
	oldKey := os.Getenv("GEMINI_API_KEY")
	defer os.Setenv("GEMINI_API_KEY", oldKey)
	os.Setenv("GEMINI_API_KEY", "gemini-test-key")

	key := APIKeyFor("gemini")
	if key != "gemini-test-key" {
		t.Errorf("APIKeyFor gemini: got %q, want %q", key, "gemini-test-key")
	}
}

// TestAPIKeyFor_Vertex tests APIKeyFor for vertex provider.
func TestAPIKeyFor_Vertex(t *testing.T) {
	oldKey := os.Getenv("VERTEX_ACCESS_TOKEN")
	defer os.Setenv("VERTEX_ACCESS_TOKEN", oldKey)
	os.Setenv("VERTEX_ACCESS_TOKEN", "vertex-test-token")

	key := APIKeyFor("vertex")
	if key != "vertex-test-token" {
		t.Errorf("APIKeyFor vertex: got %q, want %q", key, "vertex-test-token")
	}
}

// TestAPIKeyFor_OpenRouter tests APIKeyFor for openrouter provider (default).
func TestAPIKeyFor_OpenRouter(t *testing.T) {
	oldKey := os.Getenv("OPENROUTER_API_KEY")
	defer os.Setenv("OPENROUTER_API_KEY", oldKey)
	os.Setenv("OPENROUTER_API_KEY", "openrouter-test-key")

	key := APIKeyFor("openrouter")
	if key != "openrouter-test-key" {
		t.Errorf("APIKeyFor openrouter: got %q, want %q", key, "openrouter-test-key")
	}
}

// TestAPIKeyFor_UnknownProvider tests APIKeyFor with unknown provider defaults to OPENROUTER.
func TestAPIKeyFor_UnknownProvider(t *testing.T) {
	oldKey := os.Getenv("OPENROUTER_API_KEY")
	defer os.Setenv("OPENROUTER_API_KEY", oldKey)
	os.Setenv("OPENROUTER_API_KEY", "openrouter-default-key")

	key := APIKeyFor("unknown-provider")
	if key != "openrouter-default-key" {
		t.Errorf("APIKeyFor unknown: got %q, want %q", key, "openrouter-default-key")
	}
}

// TestAPIKeyFor_CaseInsensitive tests APIKeyFor is case insensitive.
func TestAPIKeyFor_CaseInsensitive(t *testing.T) {
	oldKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", oldKey)
	os.Setenv("ANTHROPIC_API_KEY", "case-test-key")

	key := APIKeyFor("ANTHROPIC")
	if key != "case-test-key" {
		t.Errorf("APIKeyFor case insensitive: got %q, want %q", key, "case-test-key")
	}
}

// TestConfig_CompactionSettings tests parsing of compaction-related settings.
func TestConfig_CompactionSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:              "anthropic",
			Model:                 "claude-3-sonnet-20250219",
			Compaction:            "token-aware",
			CompactionModel:       "claude-3-haiku-20250219",
			CompactionTrigger:     "both",
			CompactionMaxMessages: 500,
			CompactionThreshold:   80,
			CompactionKeepLastN:   15,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.CompactionModel != "claude-3-haiku-20250219" {
		t.Errorf("CompactionModel: got %q, want %q", loaded.Agent.CompactionModel, "claude-3-haiku-20250219")
	}
	if loaded.Agent.CompactionTrigger != "both" {
		t.Errorf("CompactionTrigger: got %q, want %q", loaded.Agent.CompactionTrigger, "both")
	}
	if loaded.Agent.CompactionMaxMessages != 500 {
		t.Errorf("CompactionMaxMessages: got %d, want 500", loaded.Agent.CompactionMaxMessages)
	}
	if loaded.Agent.CompactionThreshold != 80 {
		t.Errorf("CompactionThreshold: got %d, want 80", loaded.Agent.CompactionThreshold)
	}
	if loaded.Agent.CompactionKeepLastN != 15 {
		t.Errorf("CompactionKeepLastN: got %d, want 15", loaded.Agent.CompactionKeepLastN)
	}
}

// TestConfig_ContinuousCompressionSettings tests parsing of continuous compression settings.
func TestConfig_ContinuousCompressionSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:               "anthropic",
			Model:                  "claude-3-sonnet-20250219",
			ContinuousCompression:  true,
			CompressionKeepLast:    20,
			CompressionMinMessages: 100,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !loaded.Agent.ContinuousCompression {
		t.Errorf("ContinuousCompression: got false, want true")
	}
	if loaded.Agent.CompressionKeepLast != 20 {
		t.Errorf("CompressionKeepLast: got %d, want 20", loaded.Agent.CompressionKeepLast)
	}
	if loaded.Agent.CompressionMinMessages != 100 {
		t.Errorf("CompressionMinMessages: got %d, want 100", loaded.Agent.CompressionMinMessages)
	}
}

// TestConfig_ZoneBudgetingSettings tests parsing of zone budgeting settings.
func TestConfig_ZoneBudgetingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:          "anthropic",
			Model:             "claude-3-sonnet-20250219",
			ZoneBudgeting:     true,
			ZoneArchivePercent: 40,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !loaded.Agent.ZoneBudgeting {
		t.Errorf("ZoneBudgeting: got false, want true")
	}
	if loaded.Agent.ZoneArchivePercent != 40 {
		t.Errorf("ZoneArchivePercent: got %d, want 40", loaded.Agent.ZoneArchivePercent)
	}
}

// TestConfig_SmartRoutingSettings tests parsing of smart routing settings.
func TestConfig_SmartRoutingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:          "anthropic",
			Model:             "claude-3-sonnet-20250219",
			SmartRouting:      true,
			SmartRoutingModel: "claude-3-haiku-20250219",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !loaded.Agent.SmartRouting {
		t.Errorf("SmartRouting: got false, want true")
	}
	if loaded.Agent.SmartRoutingModel != "claude-3-haiku-20250219" {
		t.Errorf("SmartRoutingModel: got %q, want %q", loaded.Agent.SmartRoutingModel, "claude-3-haiku-20250219")
	}
}

// TestConfig_AzureSettings tests parsing of Azure-specific settings.
func TestConfig_AzureSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:       "azure",
			Model:          "gpt-4",
			AzureResource:  "my-resource",
			AzureDeployment: "my-deployment",
			AzureAPIVersion: "2024-08-01-preview",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.AzureResource != "my-resource" {
		t.Errorf("AzureResource: got %q, want %q", loaded.Agent.AzureResource, "my-resource")
	}
	if loaded.Agent.AzureDeployment != "my-deployment" {
		t.Errorf("AzureDeployment: got %q, want %q", loaded.Agent.AzureDeployment, "my-deployment")
	}
	if loaded.Agent.AzureAPIVersion != "2024-08-01-preview" {
		t.Errorf("AzureAPIVersion: got %q, want %q", loaded.Agent.AzureAPIVersion, "2024-08-01-preview")
	}
}

// TestConfig_VertexSettings tests parsing of Google Vertex-specific settings.
func TestConfig_VertexSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:     "vertex",
			Model:        "gemini-1.5-pro",
			VertexProject: "my-gcp-project",
			VertexRegion: "us-central1",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.VertexProject != "my-gcp-project" {
		t.Errorf("VertexProject: got %q, want %q", loaded.Agent.VertexProject, "my-gcp-project")
	}
	if loaded.Agent.VertexRegion != "us-central1" {
		t.Errorf("VertexRegion: got %q, want %q", loaded.Agent.VertexRegion, "us-central1")
	}
}

// TestConfig_SteeringMode tests parsing of steering mode.
func TestConfig_SteeringMode(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:     "anthropic",
			Model:        "claude-3-sonnet-20250219",
			SteeringMode: "aggressive",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.SteeringMode != "aggressive" {
		t.Errorf("SteeringMode: got %q, want %q", loaded.Agent.SteeringMode, "aggressive")
	}
}

// TestConfig_MCPServers tests parsing of MCP server configurations.
func TestConfig_MCPServers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		MCPServers: map[string]MCPServerConfig{
			"filesystem": {
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-filesystem", "/tmp"},
				Env: map[string]string{
					"DEBUG": "true",
				},
			},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.MCPServers) != 1 {
		t.Errorf("MCPServers count: got %d, want 1", len(loaded.MCPServers))
	}

	if fs, ok := loaded.MCPServers["filesystem"]; ok {
		if fs.Command != "npx" {
			t.Errorf("MCP command: got %q, want %q", fs.Command, "npx")
		}
		if len(fs.Args) != 2 {
			t.Errorf("MCP args count: got %d, want 2", len(fs.Args))
		}
		if fs.Env["DEBUG"] != "true" {
			t.Errorf("MCP env DEBUG: got %q, want %q", fs.Env["DEBUG"], "true")
		}
	} else {
		t.Errorf("MCPServers 'filesystem' not found")
	}
}

// TestConfig_SkillsDir tests parsing of skills directory.
func TestConfig_SkillsDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		SkillsDir: "/custom/skills/path",
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.SkillsDir != "/custom/skills/path" {
		t.Errorf("SkillsDir: got %q, want %q", loaded.SkillsDir, "/custom/skills/path")
	}
}

// TestConfig_PersistThinking tests that PersistThinking parses from JSON.
func TestConfig_PersistThinking(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:        "anthropic",
			Model:           "claude-3-sonnet-20250219",
			PersistThinking: true,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !loaded.Agent.PersistThinking {
		t.Errorf("PersistThinking: got false, want true")
	}
}

// TestConfig_PersistThinking_DefaultFalse tests that PersistThinking defaults to false.
func TestConfig_PersistThinking_DefaultFalse(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.PersistThinking {
		t.Errorf("PersistThinking default: got true, want false")
	}
}

// TestConfig_TelegramAllowedUsers tests parsing of Telegram allowed users.
func TestConfig_TelegramAllowedUsers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		Telegram: TelegramConfig{
			BotToken:     "test-token",
			AllowedUsers: []int64{123456789, 987654321},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.Telegram.AllowedUsers) != 2 {
		t.Errorf("AllowedUsers count: got %d, want 2", len(loaded.Telegram.AllowedUsers))
	}
	if loaded.Telegram.AllowedUsers[0] != 123456789 {
		t.Errorf("First user: got %d, want 123456789", loaded.Telegram.AllowedUsers[0])
	}
	if loaded.Telegram.AllowedUsers[1] != 987654321 {
		t.Errorf("Second user: got %d, want 987654321", loaded.Telegram.AllowedUsers[1])
	}
}
