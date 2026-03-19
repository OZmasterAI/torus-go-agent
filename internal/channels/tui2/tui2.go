package tui2

import (
	"go_sdk_agent/internal/channels"
	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
	"go_sdk_agent/internal/ui/rawtui"
)

func init() { channels.Register(&tui2Channel{}) }

type tui2Channel struct{}

func (t *tui2Channel) Name() string { return "tui2" }

func (t *tui2Channel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
	contextWindow := cfg.Agent.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 200000
	}
	app := rawtui.NewApp(agent, cfg.Agent.Model, contextWindow, skills)
	return app.Run()
}
