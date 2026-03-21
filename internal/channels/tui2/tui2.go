package tui2

import (
	"torus_go_agent/internal/channels"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	"torus_go_agent/internal/ui/rawtui"
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
