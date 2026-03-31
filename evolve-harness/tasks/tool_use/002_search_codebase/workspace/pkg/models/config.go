package models

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	Port        int    `json:"port"`
	DatabaseURL string `json:"database_url"`
	LogLevel    string `json:"log_level"`
	MaxConns    int    `json:"max_connections"`
	Debug       bool   `json:"debug"`
}

func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &AppConfig{
		Port:     8080,
		LogLevel: "info",
		MaxConns: 25,
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *AppConfig) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return &ConfigError{Field: "port", Message: "must be between 1 and 65535"}
	}
	if c.MaxConns < 1 {
		return &ConfigError{Field: "max_connections", Message: "must be at least 1"}
	}
	return nil
}

type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config: " + e.Field + " " + e.Message
}
