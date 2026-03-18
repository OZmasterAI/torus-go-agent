package tui

import (
	"go_sdk_agent/internal/channels"
	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
	"go_sdk_agent/internal/ui"
)

func init() { channels.Register(&tuiChannel{}) }

type tuiChannel struct{}

func (t *tuiChannel) Name() string { return "tui" }

func (t *tuiChannel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
	return ui.StartTUI(agent, cfg.Agent.Model, skills)
}
