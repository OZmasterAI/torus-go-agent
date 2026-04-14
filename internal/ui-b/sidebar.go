package uib

import (
	"fmt"
	"path/filepath"
	"strings"

	"torus_go_agent/internal/config"
)

// Layout constants.
const (
	sidebarMinW  = 120
	sidebarWidth = 26
)

// sidebarModel renders session stats, modified files, config, and flags.
type sidebarModel struct {
	theme           Theme
	modifiedFiles   map[string]int
	toolEvents      []ToolEvent
	agentCfg        config.AgentConfig
	steerAggressive bool // true when agent steering mode is "aggressive"
	turnCount       int  // actual turn count from parent model
	totalTokensIn   int
	totalTokensOut  int
	totalCost       float64
	lastInputTokens int // from most recent API call (for CTX%)
	show            bool
}

func newSidebarModel(theme Theme, cfg config.AgentConfig) sidebarModel {
	return sidebarModel{
		theme:         theme,
		modifiedFiles: make(map[string]int),
		agentCfg:      cfg,
	}
}

// TrackTool updates modified-file counts for write/edit tools.
func (s *sidebarModel) TrackTool(ev ToolEvent) {
	s.toolEvents = append(s.toolEvents, ev)
	if ev.FilePath != "" && (ev.Name == "write" || ev.Name == "edit") {
		s.modifiedFiles[ev.FilePath]++
	}
}

// View renders the sidebar within the given height.
func (s sidebarModel) View(height int) string {
	w := sidebarWidth - 4
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}

	var lines []string
	lines = append(lines, s.theme.SidebarTitle.Render("Session"))
	lines = append(lines, fmt.Sprintf(" Tools: %d", len(s.toolEvents)))

	lines = append(lines, fmt.Sprintf(" Turns: %d", s.turnCount))

	ctxWin := float64(s.agentCfg.ContextWindow)
	if ctxWin <= 0 {
		ctxWin = 128000
	}
	ctxPct := 0.0
	if s.lastInputTokens > 0 {
		ctxPct = float64(s.lastInputTokens) / ctxWin * 100
	}
	lines = append(lines, fmt.Sprintf(" CTX: %.0f%%", ctxPct))

	totalTok := s.totalTokensIn + s.totalTokensOut
	if totalTok > 0 {
		lines = append(lines, fmt.Sprintf(" Tokens: %s", fmtTok(totalTok)))
	}
	if s.totalCost > 0 {
		lines = append(lines, fmt.Sprintf(" Cost: $%.2f", s.totalCost))
	}

	lines = append(lines, "")
	lines = append(lines, s.theme.SidebarTitle.Render("Files"))
	if len(s.modifiedFiles) == 0 {
		lines = append(lines, s.theme.Dim.Render(" (none)"))
	} else {
		for path, count := range s.modifiedFiles {
			name := filepath.Base(path)
			if len(name) > w-6 {
				name = name[:w-6] + "..."
			}
			lines = append(lines, s.theme.SidebarFile.Render(" "+name)+" "+s.theme.SidebarCount.Render(fmt.Sprintf("(%d)", count)))
		}
	}

	lines = append(lines, "")
	prov := s.agentCfg.Provider
	if prov == "" {
		prov = "default"
	}
	lines = append(lines, s.theme.SidebarTitle.Render("Config"))
	lines = append(lines, s.theme.Dim.Render(fmt.Sprintf(" %s", truncStr(prov, w-2))))
	lines = append(lines, s.theme.Dim.Render(fmt.Sprintf(" %s/%s", fmtTok(s.agentCfg.MaxTokens), fmtTok(s.agentCfg.ContextWindow))))

	lines = append(lines, "")
	lines = append(lines, s.theme.SidebarTitle.Render("Flags"))
	lines = append(lines, flagStr(s.theme, "Smart", s.agentCfg.SmartRouting))
	lines = append(lines, flagStr(s.theme, "Compress", s.agentCfg.ContinuousCompression))
	lines = append(lines, flagStr(s.theme, "Zones", s.agentCfg.ZoneBudgeting))
	compact := s.agentCfg.Compaction != "" && s.agentCfg.Compaction != "none"
	lines = append(lines, flagStr(s.theme, "Compact", compact))
	lines = append(lines, flagStr(s.theme, "Steer+", s.steerAggressive))

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	content := strings.Join(lines, "\n")
	return s.theme.SidebarBorder.
		Width(sidebarWidth - 2).
		Height(innerH).
		Render(content)
}

func flagStr(theme Theme, name string, on bool) string {
	if on {
		return theme.GlowBright.Render(" \u25cf") + " " + theme.Dim.Render(name)
	}
	return theme.Dim.Render(" \u25cb") + " " + theme.Dim.Render(name)
}
