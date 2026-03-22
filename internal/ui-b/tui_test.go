package uib

import (
	"testing"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

func TestStartTUISignature(t *testing.T) {
	// Verify StartTUI compiles and has the right signature.
	// We cannot actually run it because it launches a terminal program,
	// but we can verify the function exists and accepts the right types.
	var fn func(*core.Agent, string, config.AgentConfig, *features.SkillRegistry, *TUIExtras) error
	fn = StartTUI
	_ = fn
}
