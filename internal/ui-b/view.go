package uib

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the complete TUI.
func (m Model) View() string {
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
	sb.WriteString(m.status.renderStatusBar(
		m.width,
		m.modelName,
		m.status.totalTokensIn,
		m.status.totalTokensOut,
		m.status.totalCost,
		m.chat.viewport.AtBottom(),
	))

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
