// Package channels defines the Channel interface for agent UI frontends
// (TUI, Telegram, Discord, web, etc.) and a registry to select between them.
package channels

import (
	"fmt"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

// Channel is a user-facing frontend that drives an agent.
type Channel interface {
	// Name returns the channel identifier (e.g. "tui", "telegram").
	Name() string

	// Start launches the channel. It blocks until the channel shuts down.
	Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error
}

// registry holds all registered channels.
var registry = map[string]Channel{}

// Register adds a channel to the global registry.
func Register(ch Channel) {
	registry[ch.Name()] = ch
}

// Get returns a channel by name, or an error if not found.
func Get(name string) (Channel, error) {
	ch, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown channel: %q (available: %v)", name, Names())
	}
	return ch, nil
}

// Names returns all registered channel names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
