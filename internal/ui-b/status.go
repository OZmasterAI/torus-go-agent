package uib

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statusModel manages the progress bar, spinner, completion indicator, and
// bottom status line.
type statusModel struct {
	theme      Theme
	processing bool
	barPos     int
	barDir     int
	startTime  time.Time

	lastElapsed  time.Duration
	statusPhrase string
	statusLine   string

	// Usage accumulators.
	totalTokensIn  int
	totalTokensOut int
	totalCost      float64
}

func newStatusModel(theme Theme) statusModel {
	return statusModel{
		theme:        theme,
		barDir:       1,
		statusPhrase: "Toroidal meditation running...",
	}
}

// Update advances the bouncing bar on tick messages.
func (s *statusModel) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case TickMsg:
		if s.processing {
			barW := 30
			s.barPos += s.barDir * 2
			if s.barPos >= barW-1 {
				s.barPos = barW - 1
				s.barDir = -1
			}
			if s.barPos <= 0 {
				s.barPos = 0
				s.barDir = 1
			}
		}
	}
	return nil
}

// renderProgressBar renders the bouncing glow bar with spinner and phrase.
func (s statusModel) renderProgressBar(width int) string {
	barW := 30
	if width < 80 {
		barW = 20
	}
	var bar strings.Builder
	for i := 0; i < barW; i++ {
		dist := s.barPos - i
		if dist < 0 {
			dist = -dist
		}
		switch {
		case dist == 0:
			bar.WriteString(s.theme.GlowBright.Render("\u2501"))
		case dist == 1:
			bar.WriteString(s.theme.GlowMed.Render("\u2501"))
		case dist == 2:
			bar.WriteString(s.theme.GlowDim.Render("\u2501"))
		case dist == 3:
			bar.WriteString(s.theme.GlowFaint.Render("\u2501"))
		case dist == 4:
			bar.WriteString(s.theme.GlowFaint2.Render("\u2501"))
		default:
			bar.WriteString(s.theme.GlowTrack.Render("\u2501"))
		}
	}
	elapsed := time.Since(s.startTime)
	phrase := s.statusPhrase
	if phrase == "" {
		phrase = "Toroidal meditation running..."
	}
	spinFrames := []string{"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807"}
	spinIdx := int(elapsed.Milliseconds()/100) % len(spinFrames)
	spinChar := amberCycle(elapsed).Render(spinFrames[spinIdx])
	timeStr := s.theme.Dim.Render(fmt.Sprintf(" %.1fs", elapsed.Seconds()))
	amberStyle := amberCycle(elapsed).Italic(true)
	return "  " + spinChar + amberStyle.Render(" "+phrase) + timeStr + "\n" + bar.String()
}

// renderCompletion renders the "Toroidal cycle complete" line with duration.
func (s statusModel) renderCompletion() string {
	elapsed := fmtDuration(s.lastElapsed)
	return "\n\n" +
		s.theme.Check.Render("  \u2714") +
		s.theme.Completion.Render(fmt.Sprintf(" Toroidal cycle complete | duration: %s", elapsed)) +
		"\n\n"
}

// renderProcessingOrCompletion returns progress bar, completion, or empty string.
func (s statusModel) renderProcessingOrCompletion(width int) string {
	if s.processing {
		return "\n" + s.renderProgressBar(width) + "\n"
	}
	if s.lastElapsed > 0 {
		return s.renderCompletion()
	}
	return ""
}

// renderStatusBar renders the bottom status line.
func (s statusModel) renderStatusBar(width int, modelName string, tokIn, tokOut int, cost float64, atBottom bool) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", modelName))

	totalTok := tokIn + tokOut
	if totalTok > 0 {
		parts = append(parts, fmt.Sprintf("%s tok", fmtTok(totalTok)))
	}
	if cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", cost))
	}
	if !atBottom {
		parts = append(parts, s.theme.ScrollHint.Render("PgDn \u2193"))
	}

	line := strings.Join(parts, " | ")
	padded := line
	if width > 0 && len(line) < width {
		padded = line + strings.Repeat(" ", width-len(line))
	}
	return s.theme.Status.Render(padded)
}

// ── Tick command ──────────────────────────────────────────────────────────────

func tick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// ── Torus status phrases ──────────────────────────────────────────────────────

func torusPhrase(toolName string, isError bool) string {
	if isError {
		return "\u26a0 \u2620 Error \u2620 \u26a0"
	}
	switch toolName {
	case "bash":
		return "executing on the surface..."
	case "read":
		return "reading through the ring..."
	case "edit":
		return "Inscribing the Torus..."
	case "write":
		return "Expanding the Torus..."
	case "glob", "grep":
		return "Toroidal scan..."
	case "spawn":
		return "spawning a loop..."
	case "delegate":
		return "delegating to inner ring..."
	case "recall_branch":
		return "exploring the Torus..."
	default:
		return "orbiting the Torus..."
	}
}

var hookPhrases = map[string]string{
	"on_user_input":        "parsing the meridian...",
	"before_context_build": "Toroidal mapping...",
	"before_llm_call":      "Toroidal meditation running...",
	"after_llm_call":       "completing the circuit...",
	"pre_compact":          "compressing the manifold...",
	"post_compact":         "Toroidal folding...",
	"on_error":             "\u26a0 \u2620 Error \u2620 \u26a0",
}

// ── Shared formatting helpers ─────────────────────────────────────────────────

func fmtDuration(d time.Duration) string {
	if d < 500*time.Millisecond {
		return "<0.5s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

func fmtTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func fmtTimestamp(t time.Time) string {
	if t.IsZero() {
		return "        "
	}
	return t.Format("15:04:05")
}

// amberCycle returns a lipgloss style that smoothly cycles through amber/orange
// shades using true-color RGB interpolation.
func amberCycle(elapsed time.Duration) lipgloss.Style {
	type rgb struct{ r, g, b int }
	keys := []rgb{
		{255, 191, 0},
		{249, 115, 22},
		{194, 65, 12},
		{130, 50, 10},
		{194, 65, 12},
		{249, 115, 22},
	}
	t := math.Mod(elapsed.Seconds()*2, float64(len(keys)))
	i := int(t)
	frac := t - float64(i)
	a := keys[i%len(keys)]
	b := keys[(i+1)%len(keys)]
	r := a.r + int(float64(b.r-a.r)*frac)
	g := a.g + int(float64(b.g-a.g)*frac)
	bl := a.b + int(float64(b.b-a.b)*frac)
	color := fmt.Sprintf("#%02x%02x%02x", r, g, bl)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
