package uib

import (
	"testing"

	"torus_go_agent/internal/config"
)

func TestNewModel(t *testing.T) {
	m := NewModel(nil, "test-model", config.AgentConfig{}, nil, nil)
	if m.chat.ready {
		t.Fatal("chat should not be ready before WindowSizeMsg")
	}
	if m.ready {
		t.Fatal("model should not be ready before WindowSizeMsg")
	}
}

func TestNewModelWithExtras(t *testing.T) {
	extras := &TUIExtras{}
	m := NewModel(nil, "test-model", config.AgentConfig{}, nil, extras)
	if m.modelName != "test-model" {
		t.Fatalf("expected model name 'test-model', got %q", m.modelName)
	}
}
