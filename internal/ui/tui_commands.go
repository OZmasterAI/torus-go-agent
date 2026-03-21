package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/commands"
)

func (m *Model) handleCompact() (tea.Model, tea.Cmd) {
	result, err := commands.Compact(m.agent.DAG(), m.agent.Hooks(), m.agent.GetCompaction(), m.agent.Summarize)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: err.Error(), isError: true})
	} else {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: result})
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleFork(args string) (tea.Model, tea.Cmd) {
	action, value := commands.ParseForkArgs(args)
	var branchID string
	var err error
	switch action {
	case "head":
		branchID, err = commands.ForkFromHead(m.agent.DAG(), "")
	case "back":
		n := 1
		fmt.Sscanf(value, "%d", &n)
		branchID, err = commands.ForkBack(m.agent.DAG(), n, "")
	case "node":
		branchID, err = commands.Fork(m.agent.DAG(), value, "")
	}
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: fmt.Sprintf("fork: %v", err), isError: true})
	} else {
		m.messages = m.messages[:0]
		m.messages = append(m.messages, displayMsg{role: "assistant", text: fmt.Sprintf("Forked to new branch: %s", branchID)})
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleSwitch(args string) (tea.Model, tea.Cmd) {
	mode, value := commands.ParseSwitchArgs(args)
	switch mode {
	case "list":
		return m.handleBranches()
	case "index":
		n := 0
		fmt.Sscanf(value, "%d", &n)
		name, err := commands.SwitchByIndex(m.agent.DAG(), n)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", text: err.Error(), isError: true})
		} else {
			m.messages = m.messages[:0]
			m.messages = append(m.messages, displayMsg{role: "assistant", text: fmt.Sprintf("Switched to: %s", name)})
		}
	case "id":
		name, err := commands.Switch(m.agent.DAG(), value)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", text: err.Error(), isError: true})
		} else {
			m.messages = m.messages[:0]
			m.messages = append(m.messages, displayMsg{role: "assistant", text: fmt.Sprintf("Switched to: %s", name)})
		}
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleSteering(args string) (tea.Model, tea.Cmd) {
	result := commands.Steering(m.agent, args)
	m.messages = append(m.messages, displayMsg{role: "assistant", text: result})
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleBranches() (tea.Model, tea.Cmd) {
	branches, err := commands.ListBranches(m.agent.DAG())
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: err.Error(), isError: true})
	} else {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: "**Branches:**\n```\n" + commands.FormatBranchList(branches) + "```"})
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleMessages(args string) (tea.Model, tea.Cmd) {
	branchID := strings.TrimSpace(args)
	if branchID == "" {
		branchID = m.agent.DAG().CurrentBranchID()
	}
	msgs, err := commands.ListMessages(m.agent.DAG(), branchID)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: err.Error(), isError: true})
	} else {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: "**Messages on " + branchID + ":**\n```\n" + commands.FormatMessageList(msgs) + "```"})
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleStats() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("**Session Stats:**\n\n")
	sb.WriteString(fmt.Sprintf("- Tokens in: %d\n", m.totalTokensIn))
	sb.WriteString(fmt.Sprintf("- Tokens out: %d\n", m.totalTokensOut))
	if m.totalCost > 0 {
		sb.WriteString(fmt.Sprintf("- Cost: $%.4f\n", m.totalCost))
	}
	sb.WriteString(fmt.Sprintf("- Branch: %s\n", m.agent.DAG().CurrentBranchID()))
	if m.telemetry != nil {
		sb.WriteString(fmt.Sprintf("- Telemetry: %s\n", m.telemetry.Summary()))
	}
	m.messages = append(m.messages, displayMsg{role: "assistant", text: sb.String()})
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleAgents() (tea.Model, tea.Cmd) {
	if m.subMgr == nil {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: "Sub-agent manager not available."})
	} else {
		running := m.subMgr.ListRunning()
		if len(running) == 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", text: "No sub-agents currently running."})
		} else {
			var sb strings.Builder
			sb.WriteString("**Running sub-agents:**\n\n")
			for _, id := range running {
				sb.WriteString(fmt.Sprintf("- `%s`\n", id))
			}
			m.messages = append(m.messages, displayMsg{role: "assistant", text: sb.String()})
		}
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleMCPTools() (tea.Model, tea.Cmd) {
	if m.mcpClient == nil {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: "No MCP servers configured."})
	} else {
		tools := m.mcpClient.ListTools()
		if len(tools) == 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", text: "MCP connected but no tools available."})
		} else {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("**MCP Tools (%d):**\n\n", len(tools)))
			for _, t := range tools {
				sb.WriteString(fmt.Sprintf("- `%s` — %s\n", t.Name, t.Description))
			}
			m.messages = append(m.messages, displayMsg{role: "assistant", text: sb.String()})
		}
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}
