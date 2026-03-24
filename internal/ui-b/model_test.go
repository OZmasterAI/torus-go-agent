package uib

import (
	"testing"
	"time"

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

func TestNewModelHasWelcomeMessage(t *testing.T) {
	m := NewModel(nil, "test-model", config.AgentConfig{}, nil, nil)
	// Without an agent, NewModel creates a welcome message.
	if len(m.chat.messages) == 0 {
		t.Fatal("expected at least one welcome message")
	}
	welcome := m.chat.messages[0]
	if welcome.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", welcome.Role)
	}
}

func TestDisplayMsgTimestampField(t *testing.T) {
	// Verify DisplayMsg has a Ts field that can hold time.UnixMilli results.
	// This is a compile-time check for the timestamp preservation feature.
	ts := time.UnixMilli(1711209600000) // 2024-03-23T12:00:00Z
	msg := DisplayMsg{Role: "user", Text: "hello", Ts: ts}
	if msg.Ts.IsZero() {
		t.Fatal("Ts should not be zero after setting with UnixMilli")
	}
	if msg.Ts.Year() != 2024 {
		t.Fatalf("expected year 2024, got %d", msg.Ts.Year())
	}
}
