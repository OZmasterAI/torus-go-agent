package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/commands"
	"torus_go_agent/internal/features"
	"torus_go_agent/internal/types"
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
	case "branch":
		return m.handleBranches()
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

func (m *Model) handleAlias(args string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimSpace(args))
	var nodeID, name string
	switch len(parts) {
	case 0:
		m.messages = append(m.messages, displayMsg{role: "error", text: "Usage: /alias <name> [node-id]", isError: true})
		m.input = ""
		m.cursorPos = 0
		m.rebuildContent()
		return m, nil
	case 1:
		name = parts[0]
	default:
		name, nodeID = parts[0], parts[1]
	}
	result, err := commands.Alias(m.agent.DAG(), nodeID, name)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: fmt.Sprintf("alias: %v", err), isError: true})
	} else {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: result})
	}
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

// ── Workflow overlay key handling ─────────────────────────────────────────────

// workflowDoneMsg carries the result of an async workflow execution.
type workflowDoneMsg struct {
	text string
	err  error
}

// workflowFieldCount returns how many focusable fields the current workflow has.
func (m *Model) workflowFieldCount() int {
	if m.workflow.mode == "loop" {
		return 5 // task, type, max_iterations, stop_phrase, actions
	}
	return 3 // task, type, actions
}

// workflowActionCount returns how many action buttons are available.
func (m *Model) workflowActionCount() int {
	if m.workflow.mode == "loop" {
		if len(m.workflow.agents) > 0 {
			return 2 // Run, Remove
		}
		return 0
	}
	n := 1 // Add agent
	if len(m.workflow.agents) > 0 {
		n += 2 // Run, Remove last
	}
	return n
}

func (m *Model) handleWorkflowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.overlay = overlayNone
		return m, nil

	case tea.KeyTab:
		m.workflow.editIdx = (m.workflow.editIdx + 1) % m.workflowFieldCount()
		m.workflow.actionIdx = 0
		return m, nil

	case tea.KeyShiftTab:
		m.workflow.editIdx = (m.workflow.editIdx - 1 + m.workflowFieldCount()) % m.workflowFieldCount()
		m.workflow.actionIdx = 0
		return m, nil

	case tea.KeyUp, tea.KeyDown:
		dir := 1
		if msg.Type == tea.KeyUp {
			dir = -1
		}
		// Type selector
		if m.workflow.editIdx == 1 {
			m.workflow.typeIdx = (m.workflow.typeIdx + dir + 3) % 3
		}
		// Actions
		actionsField := m.workflowFieldCount() - 1
		if m.workflow.editIdx == actionsField {
			ac := m.workflowActionCount()
			if ac > 0 {
				m.workflow.actionIdx = (m.workflow.actionIdx + dir + ac) % ac
			}
		}
		return m, nil

	case tea.KeyEnter:
		return m.handleWorkflowEnter()

	case tea.KeyBackspace:
		switch m.workflow.editIdx {
		case 0: // task
			if len(m.workflow.taskInput) > 0 {
				r := []rune(m.workflow.taskInput)
				m.workflow.taskInput = string(r[:len(r)-1])
			}
		case 2:
			if m.workflow.mode == "loop" {
				if len(m.workflow.maxIterations) > 0 {
					r := []rune(m.workflow.maxIterations)
					m.workflow.maxIterations = string(r[:len(r)-1])
				}
			}
		case 3:
			if m.workflow.mode == "loop" {
				if len(m.workflow.stopPhrase) > 0 {
					r := []rune(m.workflow.stopPhrase)
					m.workflow.stopPhrase = string(r[:len(r)-1])
				}
			}
		}
		return m, nil

	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			ch := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				ch = " "
			}
			switch m.workflow.editIdx {
			case 0:
				m.workflow.taskInput += ch
			case 2:
				if m.workflow.mode == "loop" {
					for _, c := range ch {
						if c >= '0' && c <= '9' {
							m.workflow.maxIterations += string(c)
						}
					}
				}
			case 3:
				if m.workflow.mode == "loop" {
					m.workflow.stopPhrase += ch
				}
			}
		}
		return m, nil
	}
}

func (m *Model) handleWorkflowEnter() (tea.Model, tea.Cmd) {
	actionsField := m.workflowFieldCount() - 1

	// If not on actions, advance to next field
	if m.workflow.editIdx != actionsField {
		m.workflow.editIdx = (m.workflow.editIdx + 1) % m.workflowFieldCount()
		return m, nil
	}

	if m.workflow.mode == "loop" {
		if len(m.workflow.agents) == 0 {
			if strings.TrimSpace(m.workflow.taskInput) == "" {
				return m, nil
			}
			m.workflow.agents = append(m.workflow.agents, workflowAgent{
				Task:      strings.TrimSpace(m.workflow.taskInput),
				AgentType: workflowTypes[m.workflow.typeIdx],
			})
			m.workflow.taskInput = ""
			m.workflow.actionIdx = 0
			return m, nil
		}
		switch m.workflow.actionIdx {
		case 0: // Run
			return m.executeWorkflow()
		case 1: // Remove
			m.workflow.agents = m.workflow.agents[:0]
		}
		return m, nil
	}

	// Sequential / Parallel
	switch m.workflow.actionIdx {
	case 0: // Add agent
		task := strings.TrimSpace(m.workflow.taskInput)
		if task == "" {
			return m, nil
		}
		m.workflow.agents = append(m.workflow.agents, workflowAgent{
			Task:      task,
			AgentType: workflowTypes[m.workflow.typeIdx],
		})
		m.workflow.taskInput = ""
		m.workflow.typeIdx = 0
	case 1: // Run
		return m.executeWorkflow()
	case 2: // Remove last
		if len(m.workflow.agents) > 0 {
			m.workflow.agents = m.workflow.agents[:len(m.workflow.agents)-1]
		}
	}
	return m, nil
}

func (m *Model) executeWorkflow() (tea.Model, tea.Cmd) {
	if m.subMgr == nil {
		m.messages = append(m.messages, displayMsg{role: "error", text: "Sub-agent manager not available.", isError: true})
		m.overlay = overlayNone
		m.rebuildContent()
		return m, nil
	}
	if len(m.workflow.agents) == 0 {
		return m, nil
	}

	prov := m.agent.Provider()
	soul := m.agent.SystemPrompt()
	dag := m.agent.DAG()
	mode := m.workflow.mode

	configs := make([]features.SubAgentConfig, 0, len(m.workflow.agents))
	for _, wa := range m.workflow.agents {
		configs = append(configs, features.SubAgentConfig{
			Task:      wa.Task,
			AgentType: wa.AgentType,
			Tools:     features.DefaultToolsForType(wa.AgentType),
			MaxTurns:  types.SubAgentMaxTurns,
		})
	}

	maxIter := 5
	if m.workflow.maxIterations != "" {
		if n, err := strconv.Atoi(m.workflow.maxIterations); err == nil && n > 0 {
			maxIter = n
		}
	}
	stopPhrase := m.workflow.stopPhrase

	m.overlay = overlayNone
	m.messages = append(m.messages, displayMsg{
		role: "assistant",
		text: fmt.Sprintf("Running **%s** workflow with %d agent(s)...", mode, len(configs)),
	})
	m.rebuildContent()

	return m, func() tea.Msg {
		var resultText string
		var err error
		switch mode {
		case "sequential":
			resultText, err = features.RunSequential(context.Background(), dag, prov, soul, configs, m.subMgr, m.agent)
		case "parallel":
			results, e := features.RunParallel(context.Background(), dag, prov, soul, configs, m.subMgr, m.agent)
			err = e
			if err == nil {
				var sb strings.Builder
				for i, r := range results {
					fmt.Fprintf(&sb, "=== Agent %d ===\n", i+1)
					if r.Error != nil {
						fmt.Fprintf(&sb, "Error: %s\n", r.Error.Error())
					} else {
						fmt.Fprintf(&sb, "%s\n", r.Text)
					}
				}
				resultText = sb.String()
			}
		case "loop":
			cfg := configs[0]
			shouldStop := func(result string, iteration int) bool {
				return stopPhrase != "" && strings.Contains(result, stopPhrase)
			}
			resultText, err = features.RunLoop(context.Background(), dag, prov, soul, cfg, m.subMgr, m.agent, shouldStop, maxIter)
		}
		return workflowDoneMsg{text: resultText, err: err}
	}
}
