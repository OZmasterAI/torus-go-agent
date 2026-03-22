package uib

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
)

func TestAllKeybindingsExist(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true

	keys := []tea.KeyType{
		tea.KeyCtrlK, tea.KeyCtrlB, tea.KeyCtrlW,
		tea.KeyCtrlU, tea.KeyCtrlA, tea.KeyCtrlE,
	}
	for _, k := range keys {
		_, _ = m.Update(tea.KeyMsg{Type: k})
		// Should not panic
	}
}

func TestPgUpPgDownDoNotPanic(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
}

func TestSlashOpenspalette(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model := newM.(Model)
	if !model.overlay.Active() {
		t.Fatal("/ on empty input should open command palette")
	}
}

func TestQuestionMarkOpensHelp(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model := newM.(Model)
	if !model.overlay.Active() {
		t.Fatal("? on empty input should open help")
	}
	if model.overlay.Kind() != "help" {
		t.Fatalf("expected help overlay, got %q", model.overlay.Kind())
	}
}
