package uib

import (
	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

// StartTUI launches the Bubble Tea TUI with alt-screen and mouse support.
// This is the entry point called by the channel shim.
func StartTUI(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry, extras *TUIExtras) error {
	m := NewModel(agent, modelName, cfg, skills, extras)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
