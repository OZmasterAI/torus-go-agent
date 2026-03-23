package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Verbosity levels for Ctrl+O cycling.
const (
	VerbosityCompact = 0 // default — tree-style minimal
	VerbosityVerbose = 1 // Ctrl+O — full cards, truncated output
	VerbosityFull    = 2 // Ctrl+O again — everything expanded, no truncation
)

// Styles for thinking cards
var (
	ThinkingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true)
	ThinkingCollapsed = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// ThinkingCard is a finalized thinking block.
type ThinkingCard struct {
	Text string
	Ts   time.Time
}

// ThinkingModel manages thinking state for any TUI.
type ThinkingModel struct {
	Buf       string         // accumulates during streaming
	Cards     []ThinkingCard // finalized thinking blocks
	Verbosity int            // Ctrl+O cycles: 0=compact, 1=verbose, 2=full
}

// AppendDelta adds streaming thinking text to the buffer.
func (t *ThinkingModel) AppendDelta(delta string) {
	t.Buf += delta
}

// Collapse finalizes the current buffer into a card and clears it.
func (t *ThinkingModel) Collapse() {
	if t.Buf == "" {
		return
	}
	t.Cards = append(t.Cards, ThinkingCard{Text: t.Buf, Ts: time.Now()})
	t.Buf = ""
}

// Toggle cycles verbosity: compact -> verbose -> full -> compact.
func (t *ThinkingModel) Toggle() {
	t.Verbosity = (t.Verbosity + 1) % 3
}

// VerbosityLabel returns a short label for the current verbosity level.
func (t *ThinkingModel) VerbosityLabel() string {
	switch t.Verbosity {
	case VerbosityVerbose:
		return "verbose"
	case VerbosityFull:
		return "full"
	default:
		return "compact"
	}
}

// HasPending returns true if there's buffered thinking text.
func (t *ThinkingModel) HasPending() bool {
	return t.Buf != ""
}

// RenderCard renders a single thinking card.
func (t *ThinkingModel) RenderCard(card ThinkingCard, width int) string {
	if t.Verbosity >= VerbosityVerbose {
		header := ThinkingStyle.Render("            \u25bc thinking")
		body := ThinkingStyle.Render(indentBlock(wrapSimple(card.Text, width-14), "              "))
		return header + "\n" + body + "\n"
	}
	chars := len(card.Text)
	return ThinkingCollapsed.Render(fmt.Sprintf("            \u25b6 thinking (~%d chars)", chars)) + "\n"
}

// RenderInline returns a short summary for placing on the same line as a header.
func (t *ThinkingModel) RenderInline(card ThinkingCard) string {
	chars := len(card.Text)
	if t.Verbosity >= VerbosityVerbose {
		return ThinkingStyle.Render(fmt.Sprintf("\u25bc thinking (~%d chars)", chars))
	}
	return ThinkingCollapsed.Render(fmt.Sprintf("\u25b6 thinking (~%d chars)", chars))
}

// RenderPending renders the in-progress thinking buffer during streaming.
func (t *ThinkingModel) RenderPending(width int) string {
	if t.Buf == "" {
		return ""
	}
	header := ThinkingStyle.Render("            \u25bc thinking...")
	body := ThinkingStyle.Render(indentBlock(wrapSimple(t.Buf, width-14), "              "))
	return header + "\n" + body + "\n"
}

// indentBlock prepends a prefix to every line.
func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// wrapSimple wraps text at width (basic word wrap).
func wrapSimple(text string, width int) string {
	if width <= 0 {
		width = 80
	}
	var result strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result.WriteByte('\n')
			continue
		}
		words := strings.Fields(paragraph)
		lineLen := 0
		for i, w := range words {
			wLen := len(w)
			if i > 0 && lineLen+1+wLen > width {
				result.WriteByte('\n')
				lineLen = 0
			} else if i > 0 {
				result.WriteByte(' ')
				lineLen++
			}
			result.WriteString(w)
			lineLen += wLen
		}
		result.WriteByte('\n')
	}
	return strings.TrimRight(result.String(), "\n")
}
