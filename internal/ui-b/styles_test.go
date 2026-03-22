package uib

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	// Verify key styles have non-zero foreground colors.
	checks := []struct {
		name  string
		style lipgloss.Style
	}{
		{"User", theme.User},
		{"Prompt", theme.Prompt},
		{"Error", theme.Error},
		{"AssistantPrefix", theme.AssistantPrefix},
		{"HeaderBar", theme.HeaderBar},
		{"ToolHeader", theme.ToolHeader},
		{"SidebarTitle", theme.SidebarTitle},
		{"OverlayTitle", theme.OverlayTitle},
	}

	for _, c := range checks {
		fg := c.style.GetForeground()
		if fg == (lipgloss.NoColor{}) {
			t.Errorf("%s style has no foreground color", c.name)
		}
	}
}

func TestThemeGlowGradient(t *testing.T) {
	theme := DefaultTheme()

	// The glow gradient should have 6 distinct styles.
	glows := []lipgloss.Style{
		theme.GlowBright, theme.GlowMed, theme.GlowDim,
		theme.GlowFaint, theme.GlowFaint2, theme.GlowTrack,
	}
	for i, g := range glows {
		if g.GetForeground() == (lipgloss.NoColor{}) {
			t.Errorf("glow style %d has no foreground", i)
		}
	}
}
