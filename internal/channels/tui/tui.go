package tui

import (
	"torus_go_agent/internal/channels"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	"torus_go_agent/internal/ui"
)

func init() { channels.Register(&tuiChannel{}) }

// Extras holds optional dependencies set by cmd/main.go before channel start.
var Extras *ui.TUIExtras

type tuiChannel struct{}

func (t *tuiChannel) Name() string { return "tui" }

func (t *tuiChannel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
	return ui.StartTUI(agent, cfg.Agent.Model, cfg.Agent, skills, Extras)
}
