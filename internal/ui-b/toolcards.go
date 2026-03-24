package uib

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"torus_go_agent/internal/tui/shared"
)

// ToolCardRenderer is the interface for per-tool rendering. Each tool type
// can register its own renderer to control how its card looks in the chat.
// Render handles the verbose (level 1) card -- the original mid-level.
// RenderCompact returns a brief tree-style one-liner (level 0).
// RenderFull returns a no-truncation expanded card (level 2).
type ToolCardRenderer interface {
	Render(ev *ToolEvent, maxWidth int, theme Theme) string
	RenderCompact(ev *ToolEvent, maxWidth int, theme Theme) string
	RenderFull(ev *ToolEvent, maxWidth int, theme Theme) string
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

// Render dispatches to the appropriate renderer based on verbosity level.
// Verbosity 0 (compact): tree-style one-liners, no header/footer chrome.
// Verbosity 1 (verbose): full cards with header, truncated output (default).
// Verbosity 2 (full): full cards with no truncation limits.
func (r *ToolCardRegistry) Render(ev *ToolEvent, maxWidth int, verbosity int) string {
	renderer := r.fallback
	if rr, ok := r.renderers[ev.Name]; ok {
		renderer = rr
	}

	switch verbosity {
	case shared.VerbosityCompact:
		return r.renderCompact(ev, maxWidth, renderer)
	case shared.VerbosityFull:
		return r.renderFull(ev, maxWidth, renderer)
	default:
		return r.renderVerbose(ev, maxWidth, renderer)
	}
}

// renderCompact renders a compact tool card with no header/footer chrome.
// Just the tool-specific one-liner with optional tree-drawing preview lines.
func (r *ToolCardRegistry) renderCompact(ev *ToolEvent, maxWidth int, renderer ToolCardRenderer) string {
	cardW := maxWidth - 4
	if cardW < 20 {
		cardW = 20
	}
	return renderer.RenderCompact(ev, cardW, r.theme)
}

// renderVerbose renders the standard mid-level tool card with header, body,
// and footer separators. Output is truncated (e.g. 5-line bash, 10-line diff).
func (r *ToolCardRegistry) renderVerbose(ev *ToolEvent, maxWidth int, renderer ToolCardRenderer) string {
	var sb strings.Builder
	cardW := maxWidth - 4
	if cardW < 20 {
		cardW = 20
	}

	sb.WriteString(r.renderHeader(ev, cardW))
	sb.WriteString(renderer.Render(ev, cardW, r.theme))

	if ev.IsError {
		sb.WriteString("  " + r.theme.Error.Render("error") + "\n")
	}
	sb.WriteString("  " + r.theme.ToolSep.Render(strings.Repeat("-", cardW)) + "\n")
	return sb.String()
}

// renderFull renders an expanded tool card with no truncation limits.
func (r *ToolCardRegistry) renderFull(ev *ToolEvent, maxWidth int, renderer ToolCardRenderer) string {
	var sb strings.Builder
	cardW := maxWidth - 4
	if cardW < 20 {
		cardW = 20
	}

	sb.WriteString(r.renderHeader(ev, cardW))
	sb.WriteString(renderer.RenderFull(ev, cardW, r.theme))

	if ev.IsError {
		sb.WriteString("  " + r.theme.Error.Render("error") + "\n")
	}
	sb.WriteString("  " + r.theme.ToolSep.Render(strings.Repeat("-", cardW)) + "\n")
	return sb.String()
}

// renderHeader renders the shared header line for verbose and full modes.
func (r *ToolCardRegistry) renderHeader(ev *ToolEvent, cardW int) string {
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
	return "  " + header + headerPad + "\n"
}

// ── Built-in renderers ────────────────────────────────────────────────────────

// ── edit ──

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

func (r *editCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const prefix = "            "
	var sb strings.Builder
	path := ev.FilePath
	if path == "" {
		path = "file"
	}
	sb.WriteString(prefix + theme.ToolDim.Render("Edit "+truncPath(path, 60)) + "\n")

	oldStr, _ := ev.Args["old_str"].(string)
	newStr, _ := ev.Args["new_str"].(string)
	var editLines []string
	if oldStr != "" {
		firstOld := strings.SplitN(oldStr, "\n", 2)[0]
		editLines = append(editLines, theme.DiffDel.Render("- "+truncStr(firstOld, 52)))
	}
	if newStr != "" {
		firstNew := strings.SplitN(newStr, "\n", 2)[0]
		editLines = append(editLines, theme.DiffAdd.Render("+ "+truncStr(firstNew, 52)))
	}
	for i, l := range editLines {
		treePrefix := "             \u251c\u2500 "
		if i == len(editLines)-1 {
			treePrefix = "             \u2514\u2500 "
		}
		sb.WriteString(treePrefix + l + "\n")
	}
	return sb.String()
}

func (r *editCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	oldStr, _ := ev.Args["old_str"].(string)
	newStr, _ := ev.Args["new_str"].(string)
	if oldStr != "" || newStr != "" {
		sb.WriteString(renderDiffFull(oldStr, newStr, cardW-4, theme))
	}
	return sb.String()
}

// ── write ──

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

func (r *writeCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const prefix = "            "
	path := ev.FilePath
	if path == "" {
		path = "file"
	}
	content, _ := ev.Args["content"].(string)
	lines := strings.Count(content, "\n") + 1
	return prefix + theme.ToolDim.Render(fmt.Sprintf("Write %s (%d lines)", truncPath(path, 50), lines)) + "\n"
}

func (r *writeCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	content, _ := ev.Args["content"].(string)
	contentLines := strings.Split(content, "\n")
	show := contentLines
	if len(show) > 20 {
		show = show[:20]
	}
	for _, line := range show {
		sb.WriteString("  " + theme.ToolDim.Render(truncStr(line, cardW-2)) + "\n")
	}
	if len(contentLines) > 20 {
		sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("... +%d lines", len(contentLines)-20)) + "\n")
	}
	return sb.String()
}

// ── bash ──

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

func (r *bashCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const (
		prefix     = "            "
		maxPreview = 3
	)
	var sb strings.Builder
	cmd, _ := ev.Args["command"].(string)
	sb.WriteString(prefix + theme.ToolDim.Render("$ "+truncStr(cmd, 60)) + "\n")
	if ev.Result != "" {
		lines := splitNonEmpty(ev.Result)
		show := lines
		if len(show) > maxPreview {
			show = show[:maxPreview]
		}
		var treeLines []string
		for _, l := range show {
			treeLines = append(treeLines, l)
		}
		if len(lines) > maxPreview {
			treeLines = append(treeLines, fmt.Sprintf("... +%d lines", len(lines)-maxPreview))
		}
		sb.WriteString(renderTreeLines(treeLines, theme.ToolDim, 56))
	}
	return sb.String()
}

func (r *bashCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	cmd, _ := ev.Args["command"].(string)
	if cmd != "" {
		sb.WriteString("  " + theme.Dim.Render("$ "+truncStr(cmd, cardW-4)) + "\n")
	}
	if ev.Result != "" && !ev.IsError {
		outLines := strings.Split(ev.Result, "\n")
		for _, line := range outLines {
			sb.WriteString("  " + theme.ToolDim.Render(truncStr(line, cardW-2)) + "\n")
		}
	}
	return sb.String()
}

// ── read ──

type readCardRenderer struct{}

func (r *readCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	return sb.String()
}

func (r *readCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const (
		prefix     = "            "
		maxPreview = 3
	)
	var sb strings.Builder
	path := ev.FilePath
	if path == "" {
		path = "file"
	}
	sb.WriteString(prefix + theme.ToolDim.Render("Read "+truncPath(path, 60)) + "\n")
	if ev.Result != "" {
		lines := splitNonEmpty(ev.Result)
		show := lines
		if len(show) > maxPreview {
			show = show[:maxPreview]
		}
		var treeLines []string
		for _, l := range show {
			treeLines = append(treeLines, l)
		}
		if len(lines) > maxPreview {
			treeLines = append(treeLines, fmt.Sprintf("... +%d lines", len(lines)-maxPreview))
		}
		sb.WriteString(renderTreeLines(treeLines, theme.ToolDim, 56))
	}
	return sb.String()
}

func (r *readCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	if ev.FilePath != "" {
		sb.WriteString("  " + theme.ToolDim.Render(truncPath(ev.FilePath, cardW-2)) + "\n")
	}
	if ev.Result != "" {
		resultLines := strings.Split(ev.Result, "\n")
		show := resultLines
		if len(show) > 20 {
			show = show[:20]
		}
		for _, line := range show {
			sb.WriteString("  " + theme.ToolDim.Render(truncStr(line, cardW-2)) + "\n")
		}
		if len(resultLines) > 20 {
			sb.WriteString("  " + theme.Dim.Render(fmt.Sprintf("... +%d lines", len(resultLines)-20)) + "\n")
		}
	}
	return sb.String()
}

// ── search (glob/grep) ──

type searchCardRenderer struct{}

func (r *searchCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	pat, _ := ev.Args["pattern"].(string)
	matches := countMatches(ev.Result)
	sb.WriteString("  " + theme.ToolDim.Render(fmt.Sprintf("%s -> %d matches", pat, matches)) + "\n")
	return sb.String()
}

func (r *searchCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const (
		prefix     = "            "
		maxPreview = 3
	)
	var sb strings.Builder
	pat, _ := ev.Args["pattern"].(string)
	matches := countMatches(ev.Result)
	sb.WriteString(prefix + theme.ToolDim.Render(fmt.Sprintf("%q \u2192 %d matches", pat, matches)) + "\n")
	if ev.Result != "" {
		lines := splitNonEmpty(ev.Result)
		show := lines
		if len(show) > maxPreview {
			show = show[:maxPreview]
		}
		var treeLines []string
		for _, l := range show {
			treeLines = append(treeLines, l)
		}
		if len(lines) > maxPreview {
			treeLines = append(treeLines, fmt.Sprintf("... +%d more", len(lines)-maxPreview))
		}
		sb.WriteString(renderTreeLines(treeLines, theme.ToolDim, 56))
	}
	return sb.String()
}

func (r *searchCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	pat, _ := ev.Args["pattern"].(string)
	matches := countMatches(ev.Result)
	sb.WriteString("  " + theme.ToolDim.Render(fmt.Sprintf("%s \u2192 %d matches", pat, matches)) + "\n")
	if ev.Result != "" {
		for _, line := range splitNonEmpty(ev.Result) {
			sb.WriteString("  " + theme.ToolDim.Render(truncStr(line, cardW-2)) + "\n")
		}
	}
	return sb.String()
}

// ── default (MCP / custom tools) ──

type defaultCardRenderer struct{}

func (r *defaultCardRenderer) Render(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString("  " + theme.ToolDim.Render(truncStr(ev.Result, cardW-2)) + "\n")
	return sb.String()
}

func (r *defaultCardRenderer) RenderCompact(ev *ToolEvent, cardW int, theme Theme) string {
	const prefix = "            "
	var sb strings.Builder
	capName := strings.ToUpper(ev.Name[:1]) + ev.Name[1:]
	sb.WriteString(prefix + theme.ToolDim.Render(capName) + "\n")
	if ev.Result != "" {
		sb.WriteString(renderTreeLines([]string{truncStr(ev.Result, 56)}, theme.ToolDim, 56))
	}
	return sb.String()
}

func (r *defaultCardRenderer) RenderFull(ev *ToolEvent, cardW int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString("  " + theme.ToolDim.Render(ev.Result) + "\n")
	return sb.String()
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// renderDiff renders a diff with a 10-line cap per side (verbose level).
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

// renderDiffFull renders a diff with no line limit (full level).
func renderDiffFull(oldStr, newStr string, maxWidth int, theme Theme) string {
	var sb strings.Builder
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	for _, line := range oldLines {
		sb.WriteString("  " + theme.DiffDel.Render("- "+truncStr(line, maxWidth-4)) + "\n")
	}
	for _, line := range newLines {
		sb.WriteString("  " + theme.DiffAdd.Render("+ "+truncStr(line, maxWidth-4)) + "\n")
	}
	return sb.String()
}

// renderTreeLines renders lines with box-drawing tree prefixes (compact mode).
// Uses "├─" for middle lines and "└─" for the last line, with orange tree chars.
func renderTreeLines(lines []string, style lipgloss.Style, maxWidth int) string {
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("166"))
	var sb strings.Builder
	for i, l := range lines {
		if i == len(lines)-1 {
			sb.WriteString("             " + treeStyle.Render("\u2514\u2500") + " " + style.Render(truncStr(l, maxWidth)) + "\n")
		} else {
			sb.WriteString("             " + treeStyle.Render("\u251c\u2500") + " " + style.Render(truncStr(l, maxWidth)) + "\n")
		}
	}
	return sb.String()
}

// splitNonEmpty splits s by newline and returns only non-empty lines.
func splitNonEmpty(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// countMatches counts the number of result lines for glob/grep results.
func countMatches(result string) int {
	matches := strings.Count(result, "\n")
	if result != "" && !strings.Contains(result, "no matches") {
		matches++
	}
	return matches
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
