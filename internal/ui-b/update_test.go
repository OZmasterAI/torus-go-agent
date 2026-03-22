package uib

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
)

func TestUpdateWindowSize(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := newM.(Model)
	if model.width != 120 {
		t.Fatalf("expected width 120, got %d", model.width)
	}
	if model.height != 40 {
		t.Fatalf("expected height 40, got %d", model.height)
	}
	if !model.ready {
		t.Fatal("model should be ready after WindowSizeMsg")
	}
}

func TestUpdateRoutesToOverlay(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.overlay.Open("help", nil)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := newM.(Model)
	if model.overlay.Active() {
		t.Fatal("overlay should be closed after Escape")
	}
}

func TestUpdateCtrlDQuits(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("Ctrl+D should produce a quit command")
	}
}

func TestUpdateTickMsg(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.status.processing = true
	oldPos := m.status.barPos
	m.Update(TickMsg{})
	// Bar position may or may not change depending on initial state,
	// but it should not panic.
	_ = oldPos
}
