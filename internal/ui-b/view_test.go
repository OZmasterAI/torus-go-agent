package uib

import (
	"strings"
	"testing"

	"torus_go_agent/internal/config"
)

func TestViewContainsHeader(t *testing.T) {
	m := NewModel(nil, "test-model", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	view := m.View()
	if !strings.Contains(view, "Torus Agent") {
		t.Fatal("view should contain header text 'Torus Agent'")
	}
}

func TestViewShowsLoading(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	// Not ready yet
	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Fatal("view should show Loading before WindowSizeMsg")
	}
}

func TestViewWithOverlay(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.overlay.Open("help", nil)
	view := m.View()
	if !strings.Contains(view, "Keybindings") {
		t.Fatal("view should render help overlay")
	}
}
