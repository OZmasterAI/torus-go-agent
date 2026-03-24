package uib

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"torus_go_agent/internal/core"
)

// View renders the complete TUI.
func (m Model) View() string {
	// Startup phase renders the startup screen instead of the main chat.
	if m.startupPhase {
		return m.startup.View()
	}

	if !m.ready {
		return "Loading...\n"
	}

	var sb strings.Builder

	// Header.
	sb.WriteString(m.renderHeader())
	sb.WriteByte('\n')
	sep := m.status.theme.Separator.Render(strings.Repeat("\u2500", m.width))
	sb.WriteString(sep)
	sb.WriteByte('\n')

	// Chat viewport + optional sidebar.
	chatView := m.chat.View()
	if m.sidebar.show {
		// Sync steering state from agent into sidebar sub-model.
		if m.agent != nil {
			m.sidebar.steerAggressive = m.agent.GetSteeringMode() == "aggressive"
		}
		sidebar := m.sidebar.View(m.chatHeight())
		chatView = lipgloss.JoinHorizontal(lipgloss.Top, chatView, " ", sidebar)
	}
	sb.WriteString(chatView)
	sb.WriteByte('\n')

	// Input border.
	inputBorder := m.status.theme.InputBorder.Render(strings.Repeat("\u2500", m.width))

	// Overlay or normal input area.
	if m.overlay.Active() {
		sb.WriteString(m.overlay.View())
	} else {
		// Autocomplete dropdown.
		if m.input.acMode && len(m.input.acList) > 0 {
			sb.WriteString(m.input.renderAutocomplete())
			sb.WriteByte('\n')
		}

		// Progress bar / completion indicator.
		sb.WriteString(m.status.renderProcessingOrCompletion(m.width))

		sb.WriteString(inputBorder)
		sb.WriteByte('\n')
		sb.WriteString(m.input.View())
		sb.WriteByte('\n')
		sb.WriteString(inputBorder)
		sb.WriteByte('\n')
	}

	// Status bar.
	data := StatusBarData{
		Width:           m.width,
		ModelName:       m.modelName,
		TokIn:           m.status.totalTokensIn,
		TokOut:          m.status.totalTokensOut,
		Cost:            m.status.totalCost,
		AtBottom:        m.chat.viewport.AtBottom(),
		LastInputTokens: m.lastInputTokens,
		ContextWindow:   m.agentCfg.ContextWindow,
		TurnCount:       m.turnCount,
		SessionStart:    m.sessionStart,
		Processing:      m.status.processing,
		Verbosity:       m.chat.thinking.Verbosity,
		VerbosityLabel:  m.chat.thinking.VerbosityLabel(),
	}

	// Compute next-prompt cost estimate when idle and DAG head is available.
	if !m.status.processing && m.agent != nil {
		head, _ := m.agent.DAG().GetHead()
		if head != "" {
			msgs, _ := m.agent.DAG().PromptFrom(head)
			preEst := core.EstimatePromptCost("", msgs, nil)
			if m.input.Value() != "" {
				preEst += core.EstimateTokensForText(m.input.Value())
			}
			data.NextEstimate = preEst
		}
	}

	sb.WriteString(m.status.renderStatusBar(data))

	return sb.String()
}

// renderHeader renders the top bar with agent name, model, and branch.
func (m Model) renderHeader() string {
	theme := m.status.theme
	title := theme.HeaderBar.Render("\u25c9 Torus Agent")

	branch := "unknown"
	if m.agent != nil {
		branch = m.agent.DAG().CurrentBranchID()
		if len(branch) > 20 {
			branch = branch[:20] + "..."
		}
	}

	info := theme.HeaderDim.Render(" \u2502 " + m.modelName + " \u2502 branch: " + branch + " ")

	titleLen := lipgloss.Width(title) + lipgloss.Width(info)
	pad := ""
	if m.width > titleLen {
		pad = theme.HeaderDim.Render(strings.Repeat(" ", m.width-titleLen))
	}
	return title + info + pad
}
