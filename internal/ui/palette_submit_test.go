package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPaletteSkillAutoSubmits verifies that picking a skill command from the
// command palette prefills the input AND auto-submits it (by replaying an Enter
// key), rather than only prefilling and waiting for a second keystroke. (audit #5)
func TestPaletteSkillAutoSubmits(t *testing.T) {
	t.Parallel()
	m := &Model{}
	_, cmd := m.executePaletteCommand("/myskill")

	if m.input != "/myskill" {
		t.Fatalf("input = %q, want /myskill", m.input)
	}
	if cmd == nil {
		t.Fatal("expected an auto-submit command, got nil (skill was only prefilled)")
	}
	msg := cmd()
	ke, ok := msg.(tea.KeyMsg)
	if !ok || ke.Type != tea.KeyEnter {
		t.Fatalf("expected a replayed Enter key, got %#v", msg)
	}
}
