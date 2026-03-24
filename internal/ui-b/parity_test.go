package uib

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/features"
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

func TestSkillsCommandInPalette(t *testing.T) {
	// Verify /skills appears in the command palette.
	items := DefaultPaletteCommands(nil)
	found := false
	for _, item := range items {
		if item.Command == "/skills" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("/skills should be in the default command palette")
	}
}

func TestSkillsInPaletteWithRegistry(t *testing.T) {
	// When skills are loaded, they should appear in the palette.
	sr := features.NewSkillRegistry("/nonexistent") // empty registry, no error
	items := DefaultPaletteCommands(sr)
	// With an empty directory, we just get the default items.
	// Verify /skills is still there.
	found := false
	for _, item := range items {
		if item.Command == "/skills" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("/skills should be in palette even with empty skill registry")
	}
}

func TestSkillsCommandRoute(t *testing.T) {
	// Verify /skills routes correctly through executeCommand.
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	newM, _ := m.executeCommand("/skills")
	model := newM.(Model)
	// Without a skills registry, it should show "No skills registry loaded."
	found := false
	for _, msg := range model.chat.messages {
		if strings.Contains(msg.Text, "No skills registry loaded") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("/skills without registry should show 'No skills registry loaded'")
	}
}

func TestSteerPlusFlagParityWithOriginal(t *testing.T) {
	// The original TUI shows 5 flags: Smart, Compress, Zones, Compact, Steer+.
	// Verify TUI-B sidebar renders all 5.
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{})
	view := s.View(30)
	expectedFlags := []string{"Smart", "Compress", "Zones", "Compact", "Steer+"}
	for _, flag := range expectedFlags {
		if !strings.Contains(view, flag) {
			t.Fatalf("sidebar missing flag %q (original TUI has it)", flag)
		}
	}
}
