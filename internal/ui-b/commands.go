package uib

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

// isCommand checks if input starts with "/".
func isCommand(input string) bool {
	return strings.HasPrefix(input, "/")
}

// executeCommand dispatches a slash command or palette selection.
func (m Model) executeCommand(cmd string) (tea.Model, tea.Cmd) {
	switch {
	case cmd == "/exit" || cmd == "/quit":
		return m, tea.Quit
	case cmd == "/new":
		return m.cmdNew()
	case cmd == "/clear":
		return m.cmdClear()
	case cmd == "/compact":
		return m.cmdCompact()
	case cmd == "/skills":
		return m.cmdSkills()
	case strings.HasPrefix(cmd, "/fork"):
		return m.cmdFork(strings.TrimPrefix(cmd, "/fork"))
	case strings.HasPrefix(cmd, "/switch"):
		return m.cmdSwitch(strings.TrimPrefix(cmd, "/switch"))
	case strings.HasPrefix(cmd, "/steering"):
		return m.cmdSteering(strings.TrimPrefix(cmd, "/steering"))
	case cmd == "/branches":
		return m.cmdBranches()
	case strings.HasPrefix(cmd, "/alias"):
		return m.cmdAlias(strings.TrimPrefix(cmd, "/alias"))
	case strings.HasPrefix(cmd, "/messages"):
		return m.cmdMessages(strings.TrimPrefix(cmd, "/messages"))
	case cmd == "/stats":
		return m.cmdStats()
	case cmd == "/agents":
		return m.cmdAgents()
	case cmd == "/mcp-tools":
		return m.cmdMCPTools()
	case cmd == "/sequential", cmd == "/parallel", cmd == "/loop":
		return m.cmdWorkflow(strings.TrimPrefix(cmd, "/"))

	// Internal commands from overlay.
	case cmd == "::sessions":
		m.openSessions()
		return m, nil
	case cmd == "::help":
		m.overlay.Open("help", nil)
		return m, nil
	case strings.HasPrefix(cmd, "::switch:"):
		branchID := strings.TrimPrefix(cmd, "::switch:")
		return m.cmdSwitchByID(branchID)
	case cmd == "::run_workflow":
		return m.executeWorkflow()

	default:
		// Skill dispatch.
		return m.cmdSkillDispatch(cmd)
	}
}

func (m Model) cmdNew() (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	_, err := commands.New(m.agent.DAG(), m.agent.Hooks())
	if err != nil {
		m.chat.AddMessage("error", fmt.Sprintf("new branch: %v", err))
	} else {
		m.chat.messages = m.chat.messages[:0]
		m.chat.AddMessage("assistant", "New conversation started (previous preserved on old branch).")
		m.status.totalTokensIn, m.status.totalTokensOut, m.status.totalCost = 0, 0, 0
		m.sidebar.toolEvents = nil
		m.sidebar.modifiedFiles = make(map[string]int)
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdClear() (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	if err := commands.Clear(m.agent.DAG(), m.agent.Hooks()); err != nil {
		m.chat.AddMessage("error", fmt.Sprintf("clear: %v", err))
	} else {
		m.chat.messages = m.chat.messages[:0]
		m.chat.AddMessage("assistant", "Context cleared on current branch.")
		m.status.totalTokensIn, m.status.totalTokensOut, m.status.totalCost = 0, 0, 0
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdCompact() (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	result, err := commands.Compact(m.agent.DAG(), m.agent.Hooks(), m.agent.GetCompaction(), m.agent.Summarize)
	if err != nil {
		m.chat.AddMessage("error", err.Error())
	} else {
		m.chat.AddMessage("assistant", result)
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdSkills() (tea.Model, tea.Cmd) {
	if m.skills == nil {
		m.chat.AddMessage("assistant", "No skills registry loaded.")
		m.input.Clear()
		return m, nil
	}
	list := m.skills.List()
	if len(list) == 0 {
		m.chat.AddMessage("assistant", "No skills found in skills directory.")
	} else {
		var sb strings.Builder
		sb.WriteString("**Available skills:**\n\n")
		for _, s := range list {
			sb.WriteString(fmt.Sprintf("- `/%s` \u2014 %s\n", s.Name, s.Description))
		}
		m.chat.AddMessage("assistant", sb.String())
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdFork(args string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
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
		return m.cmdBranches()
	}
	if err != nil {
		m.chat.AddMessage("error", fmt.Sprintf("fork: %v", err))
	} else {
		m.chat.messages = m.chat.messages[:0]
		m.chat.AddMessage("assistant", fmt.Sprintf("Forked to new branch: %s", branchID))
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdSwitch(args string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	mode, value := commands.ParseSwitchArgs(args)
	switch mode {
	case "list":
		return m.cmdBranches()
	case "index":
		n := 0
		fmt.Sscanf(value, "%d", &n)
		name, err := commands.SwitchByIndex(m.agent.DAG(), n)
		if err != nil {
			m.chat.AddMessage("error", err.Error())
		} else {
			m.chat.messages = m.chat.messages[:0]
			m.chat.AddMessage("assistant", fmt.Sprintf("Switched to: %s", name))
		}
	case "id":
		name, err := commands.Switch(m.agent.DAG(), value)
		if err != nil {
			m.chat.AddMessage("error", err.Error())
		} else {
			m.chat.messages = m.chat.messages[:0]
			m.chat.AddMessage("assistant", fmt.Sprintf("Switched to: %s", name))
		}
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdSwitchByID(branchID string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		return m, nil
	}
	m.agent.DAG().SwitchBranch(branchID)
	m.chat.messages = m.chat.messages[:0]
	m.chat.AddMessage("assistant", fmt.Sprintf("Switched to branch: %s", branchID))
	return m, nil
}

func (m Model) cmdSteering(args string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	result := commands.Steering(m.agent, args)
	m.chat.AddMessage("assistant", result)
	m.input.Clear()
	return m, nil
}

func (m Model) cmdBranches() (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	branches, err := commands.ListBranches(m.agent.DAG())
	if err != nil {
		m.chat.AddMessage("error", err.Error())
	} else {
		m.chat.AddMessage("assistant", "**Branches:**\n```\n"+commands.FormatBranchList(branches)+"```")
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdAlias(args string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	parts := strings.Fields(strings.TrimSpace(args))
	var nodeID, name string
	switch len(parts) {
	case 0:
		m.chat.AddMessage("error", "Usage: /alias <name> [node-id]")
		m.input.Clear()
		return m, nil
	case 1:
		name = parts[0]
	default:
		name, nodeID = parts[0], parts[1]
	}
	result, err := commands.Alias(m.agent.DAG(), nodeID, name)
	if err != nil {
		m.chat.AddMessage("error", fmt.Sprintf("alias: %v", err))
	} else {
		m.chat.AddMessage("assistant", result)
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdMessages(args string) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		return m, nil
	}
	branchID := strings.TrimSpace(args)
	if branchID == "" {
		branchID = m.agent.DAG().CurrentBranchID()
	}
	msgs, err := commands.ListMessages(m.agent.DAG(), branchID)
	if err != nil {
		m.chat.AddMessage("error", err.Error())
	} else {
		m.chat.AddMessage("assistant", "**Messages on "+branchID+":**\n```\n"+commands.FormatMessageList(msgs)+"```")
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdStats() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("**Session Stats:**\n\n")
	sb.WriteString(fmt.Sprintf("- Tokens in: %d\n", m.status.totalTokensIn))
	sb.WriteString(fmt.Sprintf("- Tokens out: %d\n", m.status.totalTokensOut))
	if m.status.totalCost > 0 {
		sb.WriteString(fmt.Sprintf("- Cost: $%.4f\n", m.status.totalCost))
	}
	if m.agent != nil {
		sb.WriteString(fmt.Sprintf("- Branch: %s\n", m.agent.DAG().CurrentBranchID()))
	}
	if m.telemetry != nil {
		sb.WriteString(fmt.Sprintf("- Telemetry: %s\n", m.telemetry.Summary()))
	}
	m.chat.AddMessage("assistant", sb.String())
	m.input.Clear()
	return m, nil
}

func (m Model) cmdAgents() (tea.Model, tea.Cmd) {
	if m.subMgr == nil {
		m.chat.AddMessage("assistant", "Sub-agent manager not available.")
	} else {
		running := m.subMgr.ListRunning()
		if len(running) == 0 {
			m.chat.AddMessage("assistant", "No sub-agents currently running.")
		} else {
			var sb strings.Builder
			sb.WriteString("**Running sub-agents:**\n\n")
			for _, id := range running {
				sb.WriteString(fmt.Sprintf("- `%s`\n", id))
			}
			m.chat.AddMessage("assistant", sb.String())
		}
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdMCPTools() (tea.Model, tea.Cmd) {
	if m.mcpClient == nil {
		m.chat.AddMessage("assistant", "No MCP servers configured.")
	} else {
		tools := m.mcpClient.ListTools()
		if len(tools) == 0 {
			m.chat.AddMessage("assistant", "MCP connected but no tools available.")
		} else {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("**MCP Tools (%d):**\n\n", len(tools)))
			for _, t := range tools {
				sb.WriteString(fmt.Sprintf("- `%s` \u2014 %s\n", t.Name, t.Description))
			}
			m.chat.AddMessage("assistant", sb.String())
		}
	}
	m.input.Clear()
	return m, nil
}

func (m Model) cmdWorkflow(mode string) (tea.Model, tea.Cmd) {
	m.overlay.Open("workflow", nil)
	m.overlay.workflow = workflowState{
		mode:          mode,
		typeIdx:       0,
		maxIterations: "5",
	}
	m.input.Clear()
	return m, nil
}

func (m Model) executeWorkflow() (tea.Model, tea.Cmd) {
	if m.subMgr == nil {
		m.chat.AddMessage("error", "Sub-agent manager not available.")
		m.overlay.Close()
		return m, nil
	}
	if len(m.overlay.workflow.agents) == 0 {
		return m, nil
	}
	if m.agent == nil {
		m.chat.AddMessage("error", "No agent available")
		m.overlay.Close()
		return m, nil
	}

	prov := m.agent.Provider()
	soul := m.agent.SystemPrompt()
	dag := m.agent.DAG()
	mode := m.overlay.workflow.mode
	subMgr := m.subMgr
	agent := m.agent

	configs := make([]features.SubAgentConfig, 0, len(m.overlay.workflow.agents))
	for _, wa := range m.overlay.workflow.agents {
		configs = append(configs, features.SubAgentConfig{
			Task:      wa.Task,
			AgentType: wa.AgentType,
			Tools:     features.DefaultToolsForType(wa.AgentType),
			MaxTurns:  types.SubAgentMaxTurns,
		})
	}

	maxIter := 5
	if m.overlay.workflow.maxIterations != "" {
		if n, err := strconv.Atoi(m.overlay.workflow.maxIterations); err == nil && n > 0 {
			maxIter = n
		}
	}
	stopPhrase := m.overlay.workflow.stopPhrase

	m.overlay.Close()
	m.chat.AddMessage("assistant", fmt.Sprintf("Running **%s** workflow with %d agent(s)...", mode, len(configs)))

	return m, func() tea.Msg {
		var resultText string
		var err error
		switch mode {
		case "sequential":
			resultText, err = features.RunSequential(context.Background(), dag, prov, soul, configs, subMgr, agent)
		case "parallel":
			results, e := features.RunParallel(context.Background(), dag, prov, soul, configs, subMgr, agent)
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
			resultText, err = features.RunLoop(context.Background(), dag, prov, soul, cfg, subMgr, agent, shouldStop, maxIter)
		}
		return WorkflowDoneMsg{Text: resultText, Err: err}
	}
}

func (m Model) cmdSkillDispatch(cmd string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(cmd, "/") && m.skills != nil {
		// Try as skill.
		m.input.SetValue(cmd)
	}
	return m, nil
}
