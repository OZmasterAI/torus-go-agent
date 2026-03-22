package uib

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ToolCardRenderer is the interface for per-tool rendering. Each tool type
// can register its own renderer to control how its card looks in the chat.
type ToolCardRenderer interface {
	Render(ev *ToolEvent, maxWidth int, theme Theme) string
}

// ToolCardRegistry dispatches tool card rendering to per-tool renderers.
type ToolCardRegistry struct {
	renderers map[string]ToolCardRenderer
	fallback  ToolCardRenderer
	theme     Theme
}

// NewToolCardRegistry creates a registry with built-in renderers for the
// standard tools: edit, write, bash, read, glob, grep.
func NewToolCardRegistry(theme Theme) *ToolCardRegistry {
	r := &ToolCardRegistry{
		renderers: make(map[string]ToolCardRenderer),
		fallback:  &defaultCardRenderer{},
		theme:     theme,
	}
	r.renderers["edit"] = &editCardRenderer{}
	r.renderers["write"] = &writeCardRenderer{}
	r.renderers["bash"] = &bashCardRenderer{}
	r.renderers["read"] = &readCardRenderer{}
	r.renderers["glob"] = &searchCardRenderer{}
	r.renderers["grep"] = &searchCardRenderer{}
	return r
}

// Register adds or replaces a renderer for a tool name.
func (r *ToolCardRegistry) Register(name string, renderer ToolCardRenderer) {
	r.renderers[name] = renderer
}

// Render dispatches to the appropriate renderer or falls back to default.
func (r *ToolCardRegistry) Render(ev *ToolEvent, maxWidth int) string {
	var sb strings.Builder
	cardW := maxWidth - 4
	if cardW < 20 {
		cardW = 20
	}

	// Header line with optional duration.
	headerText := fmt.Sprintf("--- %s ", ev.Name)
	if ev.Duration > 0 {
		headerText = fmt.Sprintf("--- %s (%s) ", ev.Name, fmtDuration(ev.Duration))
	}
	header := r.theme.ToolHeader.Render(headerText)
	headerPad := ""
	hLen := lipglossWidth(header)
	if cardW > hLen {
		headerPad = r.theme.ToolSep.Render(strings.Repeat("-", cardW-hLen))
	}
	sb.WriteString("  " + header + headerPad + "\n")

	// Body from renderer.
	renderer := r.fallback
	if rr, ok := r.renderers[ev.Name]; ok {
		renderer = rr
	}
	sb.WriteString(renderer.Render(ev, cardW, r.theme))

	if ev.IsError {
		sb.WriteString("  " + r.theme.Error.Render("error") + "\n")
	}

	// Footer separator.
	sb.WriteString("  " + r.theme.ToolSep.Render(strings.Repeat("-", cardW)) + "\n")
	return sb.String()
}

// ── Built-in renderers ────────────────────────────────────────────────────────

type editCardRenderer struct{}

func (r *editCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	oldStr, _ := ev.Args["old_str"].(string)
	newStr, _ := ev.Args["new_str"].(string)
	if oldStr != "" || newStr != "" {
		sb.WriteString(renderDiff(oldStr, newStr, cardW-4, theme))
	}
	return sb.String()
}

type writeCardRenderer struct{}

func (r *writeCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	content, _ := ev.Args["content"].(string)
	lines := strings.Count(content, "\n") + 1
	sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("%d lines written", lines)) + "\n")
	return sb.String()
}

type bashCardRenderer struct{}

func (r *bashCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	cmd, _ := ev.Args["command"].(string)
	if cmd != "" {
		sb.WriteString("  " + theme.Dim.Render("$ "+truncStr(cmd, cardW-4)) + "\n")
	}
	if ev.Result != "" && !ev.IsError {
		outLines := strings.Split(ev.Result, "\n")
		show := outLines
		if len(show) > 5 {
			show = show[:5]
		}
		for _, line := range show {
			sb.WriteString("  " + theme.ToolDim.Render(truncStr(line, cardW-2)) + "\n")
		}
		if len(outLines) > 5 {
			sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("... +%d lines", len(outLines)-5)) + "\n")
		}
	}
	return sb.String()
}

type readCardRenderer struct{}

func (r *readCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	return sb.String()
}

type searchCardRenderer struct{}

func (r *searchCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	pat, _ := ev.Args["pattern"].(string)
	matches := strings.Count(ev.Result, "\n")
	if ev.Result != "" && !strings.Contains(ev.Result, "no matches") {
		matches++
	}
	sb.WriteString("  " + theme.ToolDim.Render(fmt.Sprintf("%s -> %d matches", pat, matches)) + "\n")
	return sb.String()
}

type defaultCardRenderer struct{}

func (r *defaultCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString("  " + theme.ToolDim.Render(truncStr(ev.Result, cardW-2)) + "\n")
	return sb.String()
}

// ── Shared helpers ────────────────────────────────────────────────────────────

func renderDiff(oldStr, newStr string, maxWidth int, theme Theme) string {
	var sb strings.Builder
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	maxLines := 10
	showOld := oldLines
	if len(showOld) > maxLines {
		showOld = showOld[:maxLines]
	}
	showNew := newLines
	if len(showNew) > maxLines {
		showNew = showNew[:maxLines]
	}

	for _, line := range showOld {
		sb.WriteString("  " + theme.DiffDel.Render("- "+truncStr(line, maxWidth-4)) + "\n")
	}
	if len(oldLines) > maxLines {
		sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("  ... +%d lines", len(oldLines)-maxLines)) + "\n")
	}
	for _, line := range showNew {
		sb.WriteString("  " + theme.DiffAdd.Render("+ "+truncStr(line, maxWidth-4)) + "\n")
	}
	if len(newLines) > maxLines {
		sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("  ... +%d lines", len(newLines)-maxLines)) + "\n")
	}
	return sb.String()
}

func truncStr(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "..."
}

func truncPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	base := filepath.Base(path)
	dir := filepath.Dir(path)
	if len(base) >= maxLen-4 {
		return truncStr(base, maxLen)
	}
	remain := maxLen - len(base) - 4
	if remain < 1 {
		return truncStr(base, maxLen)
	}
	return ".../" + dir[len(dir)-remain:] + "/" + base
}

// lipglossWidth is a helper to estimate rendered width.
// We avoid importing lipgloss at the top to keep this file simple;
// the actual lipgloss.Width call happens elsewhere.
func lipglossWidth(s string) int {
	// Count visible characters (naive but good enough for headers).
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}
