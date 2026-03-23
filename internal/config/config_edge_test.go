package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigEdge_LoadConfigEmptyFile tests loading an empty JSON file.
func TestConfigEdge_LoadConfigEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte("{}"), 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Should apply defaults even for empty config
	if loaded.Agent.MaxTokens != 8192 {
		t.Errorf("Empty config MaxTokens default: got %d, want 8192", loaded.Agent.MaxTokens)
	}
	if loaded.Agent.ContextWindow != 128000 {
		t.Errorf("Empty config ContextWindow default: got %d, want 128000", loaded.Agent.ContextWindow)
	}
	if loaded.Agent.Compaction != "llm" {
		t.Errorf("Empty config Compaction default: got %q, want %q", loaded.Agent.Compaction, "llm")
	}
}

// TestConfigEdge_LoadConfigMalformedJSON tests various malformed JSON inputs.
func TestConfigEdge_LoadConfigMalformedJSON(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"unclosed brace", `{"agent": {"provider": "anthropic"`},
		{"trailing comma", `{"agent": {"provider": "anthropic",}}`},
		{"invalid number", `{"agent": {"maxTokens": "not-a-number"}}`},
		{"unquoted key", `{agent: {"provider": "anthropic"}}`},
		{"control character", "{\n\"agent\": \"test\x00\"}\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")
			os.WriteFile(configPath, []byte(tc.content), 0644)

			_, err := LoadConfig(configPath)
			if err == nil {
				t.Errorf("LoadConfig should fail for malformed JSON: %s", tc.name)
			}
		})
	}
}

// TestConfigEdge_EnvOverrideEmptyValue tests that empty env vars don't override.
func TestConfigEdge_EnvOverrideEmptyValue(t *testing.T) {
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

	oldModel := os.Getenv("AGENT_MODEL")
	oldProvider := os.Getenv("AGENT_PROVIDER")
	oldToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer func() {
		os.Setenv("AGENT_MODEL", oldModel)
		os.Setenv("AGENT_PROVIDER", oldProvider)
		os.Setenv("TELEGRAM_BOT_TOKEN", oldToken)
	}()

	// Set empty env vars
	os.Setenv("AGENT_MODEL", "")
	os.Setenv("AGENT_PROVIDER", "")
	os.Setenv("TELEGRAM_BOT_TOKEN", "")

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Config values should be preserved since env vars are empty
	if loaded.Agent.Provider != "anthropic" {
		t.Errorf("Provider should not be overridden by empty env: got %q, want %q", loaded.Agent.Provider, "anthropic")
	}
	if loaded.Agent.Model != "claude-3-sonnet-20250219" {
		t.Errorf("Model should not be overridden by empty env: got %q, want %q", loaded.Agent.Model, "claude-3-sonnet-20250219")
	}
}

// TestConfigEdge_MCPServersParsing tests loading MCPServers configuration.
func TestConfigEdge_MCPServersParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		MCPServers: map[string]MCPServerConfig{
			"memory": {
				Command: "/usr/bin/python3",
				Args:    []string{"/path/to/memory.py"},
				Env: map[string]string{
					"MEMORY_DB": "/tmp/memory.db",
					"DEBUG":     "true",
				},
			},
			"tools": {
				Command: "/usr/bin/node",
				Args:    []string{"/path/to/tools.js", "--port", "3000"},
				Env:     map[string]string{},
			},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.MCPServers) != 2 {
		t.Errorf("MCPServers count: got %d, want 2", len(loaded.MCPServers))
	}

	memoryServer, exists := loaded.MCPServers["memory"]
	if !exists {
		t.Errorf("MCPServer 'memory' not found")
	} else {
		if memoryServer.Command != "/usr/bin/python3" {
			t.Errorf("Memory server command: got %q, want %q", memoryServer.Command, "/usr/bin/python3")
		}
		if len(memoryServer.Args) != 1 {
			t.Errorf("Memory server args count: got %d, want 1", len(memoryServer.Args))
		}
		if memoryServer.Env["MEMORY_DB"] != "/tmp/memory.db" {
			t.Errorf("Memory server env MEMORY_DB: got %q, want %q", memoryServer.Env["MEMORY_DB"], "/tmp/memory.db")
		}
	}

	toolsServer, exists := loaded.MCPServers["tools"]
	if !exists {
		t.Errorf("MCPServer 'tools' not found")
	} else {
		if len(toolsServer.Args) != 3 {
			t.Errorf("Tools server args count: got %d, want 3", len(toolsServer.Args))
		}
	}
}

// TestConfigEdge_SkillsDirParsing tests SkillsDir configuration.
func TestConfigEdge_SkillsDirParsing(t *testing.T) {
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

// TestConfigEdge_DataDirRelativePath tests relative data directory resolution.
func TestConfigEdge_DataDirRelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configDir := tmpDir

	cfg := Config{
		Data: DataConfig{
			Dir: "relative/data",
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	expectedDir := filepath.Join(configDir, "relative/data")
	actualDir := loaded.DataDir(configDir)
	if actualDir != expectedDir {
		t.Errorf("DataDir relative: got %q, want %q", actualDir, expectedDir)
	}
}

// TestConfigEdge_DataDirAbsolutePath tests absolute data directory.
func TestConfigEdge_DataDirAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configDir := tmpDir

	absolutePath := "/absolute/path/to/data"
	cfg := Config{
		Data: DataConfig{
			Dir: absolutePath,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	actualDir := loaded.DataDir(configDir)
	if actualDir != absolutePath {
		t.Errorf("DataDir absolute: got %q, want %q", actualDir, absolutePath)
	}
}

// TestConfigEdge_DataDirXDGDataHome tests XDG_DATA_HOME environment variable.
func TestConfigEdge_DataDirXDGDataHome(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configDir := tmpDir

	cfg := Config{
		Data: DataConfig{
			Dir: "", // Empty, should use XDG_DATA_HOME
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer os.Setenv("XDG_DATA_HOME", oldXDG)
	xdgPath := filepath.Join(tmpDir, "xdg-data")
	os.Setenv("XDG_DATA_HOME", xdgPath)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	expectedDir := filepath.Join(xdgPath, "torus_go_agent")
	actualDir := loaded.DataDir(configDir)
	if actualDir != expectedDir {
		t.Errorf("DataDir XDG: got %q, want %q", actualDir, expectedDir)
	}
}

// TestConfigEdge_DataDirFallbackToHome tests fallback to home directory.
func TestConfigEdge_DataDirFallbackToHome(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configDir := tmpDir

	cfg := Config{
		Data: DataConfig{
			Dir: "", // Empty, no XDG_DATA_HOME
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	// Clear XDG_DATA_HOME
	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer os.Setenv("XDG_DATA_HOME", oldXDG)
	os.Setenv("XDG_DATA_HOME", "")

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	actualDir := loaded.DataDir(configDir)
	if actualDir == "" {
		t.Errorf("DataDir fallback should not be empty")
	}

	// Should contain the expected path component
	if !filepath.HasPrefix(actualDir, os.TempDir()) && !filepath.HasPrefix(actualDir, os.Getenv("HOME")) {
		// It's OK if it's under temp dir or home dir
		if !filepath.IsAbs(actualDir) {
			t.Errorf("DataDir should be absolute path: got %q", actualDir)
		}
	}
}

// TestConfigEdge_APIKeyForAllProviders tests APIKeyFor function for all supported providers.
func TestConfigEdge_APIKeyForAllProviders(t *testing.T) {
	testCases := []struct {
		provider string
		envVar   string
	}{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"nvidia", "NVIDIA_API_KEY"},
		{"grok", "XAI_API_KEY"},
		{"azure", "AZURE_OPENAI_API_KEY"},
		{"gemini", "GEMINI_API_KEY"},
		{"vertex", "VERTEX_ACCESS_TOKEN"},
		{"openrouter", "OPENROUTER_API_KEY"},
		{"unknown", "OPENROUTER_API_KEY"}, // unknown defaults to openrouter
	}

	for _, tc := range testCases {
		t.Run(tc.provider, func(t *testing.T) {
			oldVal := os.Getenv(tc.envVar)
			defer os.Setenv(tc.envVar, oldVal)

			testKey := "test-key-" + tc.provider
			os.Setenv(tc.envVar, testKey)

			key := APIKeyFor(tc.provider)
			if key != testKey {
				t.Errorf("APIKeyFor(%q): got %q, want %q", tc.provider, key, testKey)
			}
		})
	}
}

// TestConfigEdge_APIKeyForCaseInsensitive tests APIKeyFor case insensitivity.
func TestConfigEdge_APIKeyForCaseInsensitive(t *testing.T) {
	testCases := []struct {
		provider string
		envVar   string
	}{
		{"ANTHROPIC", "ANTHROPIC_API_KEY"},
		{"OpenAI", "OPENAI_API_KEY"},
		{"NVIDIA", "NVIDIA_API_KEY"},
	}

	for _, tc := range testCases {
		t.Run(tc.provider, func(t *testing.T) {
			oldVal := os.Getenv(tc.envVar)
			defer os.Setenv(tc.envVar, oldVal)

			testKey := "test-key-case"
			os.Setenv(tc.envVar, testKey)

			key := APIKeyFor(tc.provider)
			if key != testKey {
				t.Errorf("APIKeyFor(%q) case insensitive: got %q, want %q", tc.provider, key, testKey)
			}
		})
	}
}

// TestConfigEdge_APIKeyEmpty tests APIKeyFor when env var is not set.
func TestConfigEdge_APIKeyEmpty(t *testing.T) {
	oldAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	oldNvidia := os.Getenv("NVIDIA_API_KEY")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", oldAnthropic)
		os.Setenv("OPENAI_API_KEY", oldOpenAI)
		os.Setenv("NVIDIA_API_KEY", oldNvidia)
	}()

	os.Setenv("ANTHROPIC_API_KEY", "")
	os.Setenv("OPENAI_API_KEY", "")
	os.Setenv("NVIDIA_API_KEY", "")

	key := APIKeyFor("anthropic")
	if key != "" {
		t.Errorf("APIKeyFor should return empty string when env not set: got %q", key)
	}
}

// TestConfigEdge_TelegramAllowedUsers tests parsing of allowed users list.
func TestConfigEdge_TelegramAllowedUsers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet-20250219",
		},
		Telegram: TelegramConfig{
			BotToken:     "test-token",
			AllowedUsers: []int64{123456789, 987654321, 111222333},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.Telegram.AllowedUsers) != 3 {
		t.Errorf("AllowedUsers count: got %d, want 3", len(loaded.Telegram.AllowedUsers))
	}

	expectedUsers := []int64{123456789, 987654321, 111222333}
	for i, user := range expectedUsers {
		if loaded.Telegram.AllowedUsers[i] != user {
			t.Errorf("AllowedUsers[%d]: got %d, want %d", i, loaded.Telegram.AllowedUsers[i], user)
		}
	}
}

// TestConfigEdge_CompactionSettings tests compaction-related configuration.
func TestConfigEdge_CompactionSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:              "anthropic",
			Model:                 "claude-3-sonnet-20250219",
			Compaction:            "summarize",
			CompactionModel:       "claude-3-haiku-20250219",
			CompactionTrigger:     "both",
			CompactionMaxMessages: 50,
			CompactionThreshold:   85,
			CompactionKeepLastN:   15,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.Compaction != "summarize" {
		t.Errorf("Compaction: got %q, want %q", loaded.Agent.Compaction, "summarize")
	}
	if loaded.Agent.CompactionModel != "claude-3-haiku-20250219" {
		t.Errorf("CompactionModel: got %q, want %q", loaded.Agent.CompactionModel, "claude-3-haiku-20250219")
	}
	if loaded.Agent.CompactionTrigger != "both" {
		t.Errorf("CompactionTrigger: got %q, want %q", loaded.Agent.CompactionTrigger, "both")
	}
	if loaded.Agent.CompactionMaxMessages != 50 {
		t.Errorf("CompactionMaxMessages: got %d, want 50", loaded.Agent.CompactionMaxMessages)
	}
	if loaded.Agent.CompactionThreshold != 85 {
		t.Errorf("CompactionThreshold: got %d, want 85", loaded.Agent.CompactionThreshold)
	}
	if loaded.Agent.CompactionKeepLastN != 15 {
		t.Errorf("CompactionKeepLastN: got %d, want 15", loaded.Agent.CompactionKeepLastN)
	}
}

// TestConfigEdge_ContinuousCompressionSettings tests compression-related configuration.
func TestConfigEdge_ContinuousCompressionSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:               "anthropic",
			Model:                  "claude-3-sonnet-20250219",
			ContinuousCompression:  true,
			CompressionKeepLast:    20,
			CompressionMinMessages: 30,
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
	if loaded.Agent.CompressionMinMessages != 30 {
		t.Errorf("CompressionMinMessages: got %d, want 30", loaded.Agent.CompressionMinMessages)
	}
}

// TestConfigEdge_ZoneBudgetingSettings tests zone-based budgeting configuration.
func TestConfigEdge_ZoneBudgetingSettings(t *testing.T) {
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

// TestConfigEdge_SmartRoutingSettings tests smart routing configuration.
func TestConfigEdge_SmartRoutingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:          "openrouter",
			Model:             "auto",
			SmartRouting:      true,
			SmartRoutingModel: "claude-3-sonnet-20250219",
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
	if loaded.Agent.SmartRoutingModel != "claude-3-sonnet-20250219" {
		t.Errorf("SmartRoutingModel: got %q, want %q", loaded.Agent.SmartRoutingModel, "claude-3-sonnet-20250219")
	}
}

// TestConfigEdge_SteeringModeSettings tests steering mode configuration.
func TestConfigEdge_SteeringModeSettings(t *testing.T) {
	testCases := []string{"mild", "aggressive", ""}

	for _, mode := range testCases {
		t.Run("mode_"+mode, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			cfg := Config{
				Agent: AgentConfig{
					Provider:      "anthropic",
					Model:         "claude-3-sonnet-20250219",
					SteeringMode:  mode,
				},
			}

			data, _ := json.Marshal(cfg)
			os.WriteFile(configPath, data, 0644)

			loaded, err := LoadConfig(configPath)
			if err != nil {
				t.Fatalf("LoadConfig failed: %v", err)
			}

			if loaded.Agent.SteeringMode != mode {
				t.Errorf("SteeringMode: got %q, want %q", loaded.Agent.SteeringMode, mode)
			}
		})
	}
}

// TestConfigEdge_AzureConfiguration tests Azure-specific configuration.
func TestConfigEdge_AzureConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:       "azure",
			Model:          "gpt-4",
			AzureResource:  "my-resource",
			AzureDeployment: "gpt4-deployment",
			AzureAPIVersion: "2024-06-01",
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
	if loaded.Agent.AzureDeployment != "gpt4-deployment" {
		t.Errorf("AzureDeployment: got %q, want %q", loaded.Agent.AzureDeployment, "gpt4-deployment")
	}
	if loaded.Agent.AzureAPIVersion != "2024-06-01" {
		t.Errorf("AzureAPIVersion: got %q, want %q", loaded.Agent.AzureAPIVersion, "2024-06-01")
	}
}

// TestConfigEdge_VertexConfiguration tests Google Vertex-specific configuration.
func TestConfigEdge_VertexConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:    "vertex",
			Model:       "gemini-pro",
			VertexProject: "my-gcp-project",
			VertexRegion:  "us-central1",
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

// TestConfigEdge_ConfigWithAllFields tests a comprehensive config with all fields set.
func TestConfigEdge_ConfigWithAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Telegram: TelegramConfig{
			BotToken:     "tg-token",
			AllowedUsers: []int64{123, 456},
		},
		Agent: AgentConfig{
			Provider:              "anthropic",
			Model:                 "claude-3-sonnet-20250219",
			MaxTokens:             4096,
			ContextWindow:         200000,
			Compaction:            "summarize",
			CompactionModel:       "claude-3-haiku-20250219",
			CompactionTrigger:     "both",
			CompactionMaxMessages: 50,
			CompactionThreshold:   85,
			CompactionKeepLastN:   10,
			ContinuousCompression:  true,
			CompressionKeepLast:    10,
			CompressionMinMessages: 5,
			ZoneBudgeting:         true,
			ZoneArchivePercent:    30,
			SmartRouting:          false,
			SteeringMode:          "mild",
		},
		Data: DataConfig{
			Dir: "/var/data/torus",
		},
		SkillsDir: "/var/skills",
		MCPServers: map[string]MCPServerConfig{
			"memory": {
				Command: "python3",
				Args:    []string{"-m", "memory_server"},
				Env:     map[string]string{"DEBUG": "false"},
			},
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify all fields are preserved
	if loaded.Telegram.BotToken != "tg-token" {
		t.Errorf("Telegram.BotToken mismatch")
	}
	if len(loaded.Telegram.AllowedUsers) != 2 {
		t.Errorf("Telegram.AllowedUsers count mismatch")
	}
	if loaded.Agent.Provider != "anthropic" {
		t.Errorf("Agent.Provider mismatch")
	}
	if loaded.Agent.Model != "claude-3-sonnet-20250219" {
		t.Errorf("Agent.Model mismatch")
	}
	if loaded.Data.Dir != "/var/data/torus" {
		t.Errorf("Data.Dir mismatch")
	}
	if loaded.SkillsDir != "/var/skills" {
		t.Errorf("SkillsDir mismatch")
	}
	if len(loaded.MCPServers) != 1 {
		t.Errorf("MCPServers count mismatch")
	}
}

// TestConfigEdge_LoadTorusFileNotFound tests LoadTorus returns default when file missing.
func TestConfigEdge_LoadTorusFileNotFound(t *testing.T) {
	torus := LoadTorus("/nonexistent/path")
	if torus != "You are an AI assistant with access to tools." {
		t.Errorf("LoadTorus should return default text: got %q", torus)
	}
}

// TestConfigEdge_LoadTorusSuccess tests successful TORUS.md loading.
func TestConfigEdge_LoadTorusSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	torusPath := filepath.Join(tmpDir, "TORUS.md")
	torusContent := "Custom persona for TORUS AI"
	os.WriteFile(torusPath, []byte(torusContent), 0644)

	torus := LoadTorus(tmpDir)
	if torus != torusContent {
		t.Errorf("LoadTorus: got %q, want %q", torus, torusContent)
	}
}

// TestConfigEdge_LoadSchemaFileNotFound tests LoadSchema returns empty when file missing.
func TestConfigEdge_LoadSchemaFileNotFound(t *testing.T) {
	schema := LoadSchema("/nonexistent/path")
	if schema != "" {
		t.Errorf("LoadSchema should return empty string: got %q", schema)
	}
}

// TestConfigEdge_LoadSchemaSuccess tests successful SCHEMA.md loading.
func TestConfigEdge_LoadSchemaSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "SCHEMA.md")
	schemaContent := "# System Architecture\n\nThis is the schema."
	os.WriteFile(schemaPath, []byte(schemaContent), 0644)

	schema := LoadSchema(tmpDir)
	if schema != schemaContent {
		t.Errorf("LoadSchema: got %q, want %q", schema, schemaContent)
	}
}

// TestConfigEdge_ResolveModelInfoNil tests ResolveModelInfo with nil models map.
func TestConfigEdge_ResolveModelInfoNil(t *testing.T) {
	info := ResolveModelInfo("claude-3-sonnet-20250219", "anthropic", nil, "")
	if info.ContextWindow != 0 || info.MaxTokens != 0 {
		t.Errorf("ResolveModelInfo with nil maps should return empty: got %+v", info)
	}
}

// TestConfigEdge_ResolveModelInfoEmpty tests ResolveModelInfo with empty models map.
func TestConfigEdge_ResolveModelInfoEmpty(t *testing.T) {
	models := make(map[string]ModelInfo)
	info := ResolveModelInfo("unknown-model", "anthropic", models, "")
	if info.ContextWindow != 0 || info.MaxTokens != 0 {
		t.Errorf("ResolveModelInfo with empty map should return empty: got %+v", info)
	}
}

// TestConfigEdge_NegativeAndZeroValues tests negative and zero values are preserved.
func TestConfigEdge_NegativeAndZeroValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:              "anthropic",
			Model:                 "claude-3-sonnet-20250219",
			MaxTokens:             -1,  // Negative should be preserved before defaults apply
			ContextWindow:         0,   // Zero triggers default
			CompactionMaxMessages: 0,   // Zero is valid (means disabled)
			CompactionThreshold:   0,   // Zero is valid
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify defaults are applied correctly
	if loaded.Agent.MaxTokens != -1 { // Negative value preserved
		t.Errorf("MaxTokens negative: got %d, want -1", loaded.Agent.MaxTokens)
	}
	if loaded.Agent.ContextWindow != 128000 { // Zero triggers default
		t.Errorf("ContextWindow zero: got %d, want 128000", loaded.Agent.ContextWindow)
	}
}

// TestConfigEdge_LargeNumberValues tests very large number values are preserved.
func TestConfigEdge_LargeNumberValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		Agent: AgentConfig{
			Provider:      "anthropic",
			Model:         "claude-3-sonnet-20250219",
			MaxTokens:     1000000,
			ContextWindow: 10000000,
		},
	}

	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Agent.MaxTokens != 1000000 {
		t.Errorf("MaxTokens large: got %d, want 1000000", loaded.Agent.MaxTokens)
	}
	if loaded.Agent.ContextWindow != 10000000 {
		t.Errorf("ContextWindow large: got %d, want 10000000", loaded.Agent.ContextWindow)
	}
}
