// Package tuib provides a channel shim for the new composable TUI (ui-b).
// Import this package for side-effect registration:
//
//	_ "torus_go_agent/internal/channels/tui-b"
package tuib

import (
	"torus_go_agent/internal/channels"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	uib "torus_go_agent/internal/ui-b"
)

func init() { channels.Register(&tuiBChannel{}) }

// Extras holds optional dependencies set by cmd/main.go before channel start.
var Extras *uib.TUIExtras

type tuiBChannel struct{}

func (t *tuiBChannel) Name() string { return "tui-b" }

func (t *tuiBChannel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
	return uib.StartTUI(agent, cfg.Agent.Model, cfg.Agent, skills, Extras)
}
