package uib

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/core"

	"torus_go_agent/internal/types"
)

// Update is the main Bubble Tea update function. It routes messages to the
// appropriate sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.sidebar.show = m.width >= sidebarMinW

		if !m.ready {
			m.ready = true
		}
		chatW, chatH := m.chatDimensions()
		m.chat.Resize(chatW, chatH)
		m.input.Resize(m.width)
		return m, nil

	// ── Keyboard input ───────────────────────────────────────────────────
	case tea.KeyMsg:
		// Overlay intercepts all keys when active.
		if m.overlay.Active() {
			selected, cmd := m.overlay.Update(msg)
			if selected != "" {
				return m.executeCommand(selected)
			}
			return m, cmd
		}
		return m.handleKey(msg)

	// ── Stream events (unified channel) ──────────────────────────────────
	case StreamEventMsg:
		return m.handleStreamEvent(msg)

	// ── Agent done ───────────────────────────────────────────────────────
	case AgentDoneMsg:
		return m.handleAgentDone(msg)

	// ── Agent error ──────────────────────────────────────────────────────
	case AgentErrorMsg:
		return m.handleAgentError(msg)

	// ── Workflow done ────────────────────────────────────────────────────
	case WorkflowDoneMsg:
		return m.handleWorkflowDone(msg)

	// ── Tick (progress bar) ──────────────────────────────────────────────
	case TickMsg:
		m.status.Update(msg)
		if m.status.processing {
			return m, tick()
		}
		return m, nil

	// ── Mouse ────────────────────────────────────────────────────────────
	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	// Forward unhandled messages to the viewport.
	cmd := m.chat.Update(msg)
	return m, cmd
}

// handleKey dispatches non-overlay key events.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.status.processing {
			m.chat.AddMessage("error", "Agent is running \u2014 use Ctrl+D to force quit.")
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyCtrlD:
		return m, tea.Quit

	case tea.KeyCtrlK:
		m.overlay.Open("palette", DefaultPaletteCommands(m.skills))
		return m, nil

	case tea.KeyCtrlB:
		m.openSessions()
		return m, nil

	case tea.KeyCtrlO:
		m.chat.thinking.Toggle()
		m.chat.Rebuild()
		return m, nil

	case tea.KeyPgUp, tea.KeyPgDown:
		cmd := m.chat.Update(msg)
		return m, cmd

	default:
		// Check for "/" and "?" shortcuts on empty input.
		if msg.Type == tea.KeyRunes && !m.status.processing {
			char := string(msg.Runes)
			if char == "/" && m.input.Value() == "" {
				m.overlay.Open("palette", DefaultPaletteCommands(m.skills))
				return m, nil
			}
			if char == "?" && m.input.Value() == "" {
				m.overlay.Open("help", nil)
				return m, nil
			}
		}

		// Delegate to input model.
		submitted, cmd := m.input.Update(msg)
		if submitted {
			return m.handleSubmit()
		}
		return m, cmd
	}
}

// handleSubmit processes a submitted input line.
func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}

	// Steering injection while agent is running.
	if m.status.processing {
		if m.agent != nil && m.agent.Steering != nil {
			m.agent.Steering <- types.Message{
				Role:    "user",
				Content: []types.ContentBlock{{Type: "text", Text: input}},
			}
			m.chat.AddMessage("user", "\u21aa "+input)
			m.input.Clear()
		}
		return m, nil
	}

	// Slash commands.
	if isCommand(input) {
		return m.executeCommand(input)
	}

	// Skill dispatch.
	if m.skills != nil {
		if skillName, ok := m.skills.IsSkillCommand(input); ok {
			if skill, found := m.skills.Get(skillName); found {
				beforeSkill := &core.HookData{
					AgentID: "main",
					Meta:    map[string]any{"skill": skillName, "input": input},
				}
				if m.agent != nil {
					m.agent.Hooks().Fire(context.Background(), core.HookBeforeSkill, beforeSkill)
				}
				if !beforeSkill.Block {
					input = m.skills.FormatSkillPrompt(skill, input)
					if m.agent != nil {
						m.agent.Hooks().Fire(context.Background(), core.HookAfterSkill, &core.HookData{
							AgentID: "main",
							Meta:    map[string]any{"skill": skillName, "input": input},
						})
					}
				}
			}
		}
	}

	// Send message to agent.
	m.chat.AddMessage("user", input)
	m.chat.AddMessage("assistant", "") // streaming placeholder
	m.input.Clear()
	m.status.processing = true
	m.chat.streaming = true
	m.status.lastElapsed = 0
	m.status.statusPhrase = "Toroidal meditation running..."
	m.status.startTime = time.Now()

	eventCh := make(chan StreamEventMsg, 64)
	m.eventCh = eventCh
	if m.agent != nil && m.agent.Steering == nil {
		m.agent.Steering = make(chan types.Message, 16)
	}

	chatW, chatH := m.chatDimensions()
	m.chat.Resize(chatW, chatH)
	m.chat.Rebuild()

	if m.agent == nil {
		// No agent (testing mode) -- just close channel.
		close(eventCh)
		return m, tea.Batch(waitForStreamEvent(eventCh), tick())
	}

	return m, tea.Batch(
		runAgentStream(m.agent, input, eventCh),
		waitForStreamEvent(eventCh),
		tick(),
	)
}

// handleStreamEvent processes unified stream events.
func (m Model) handleStreamEvent(msg StreamEventMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case StreamTextDelta:
		m.chat.AppendDelta(msg.Delta)
		m.status.statusPhrase = "Torus relaying..."

	case StreamThinkingDelta:
		m.chat.thinking.AppendDelta(msg.Thinking)
		m.chat.Rebuild()

	case StreamToolStart:
		// No-op for now; tool start can be shown later.

	case StreamToolEnd:
		m.sidebar.TrackTool(msg.Tool)
		m.status.statusPhrase = torusPhrase(msg.Tool.Name, msg.Tool.IsError)
		m.chat.AddToolCard(&msg.Tool)

	case StreamStatusUpdate:
		if phrase, ok := hookPhrases[msg.StatusHook]; ok {
			m.status.statusPhrase = phrase
		}
	}

	return m, waitForStreamEvent(m.eventCh)
}

// handleAgentDone finalizes after the agent completes.
func (m Model) handleAgentDone(msg AgentDoneMsg) (tea.Model, tea.Cmd) {
	m.status.processing = false
	m.chat.streaming = false
	m.status.lastElapsed = msg.Elapsed
	m.eventCh = nil

	// Collapse any pending thinking into a card.
	m.chat.thinking.Collapse()

	m.status.totalTokensIn += msg.TokensIn
	m.status.totalTokensOut += msg.TokensOut
	m.status.totalCost += msg.Cost

	// Clean up empty trailing placeholder.
	if len(m.chat.messages) > 0 {
		last := &m.chat.messages[len(m.chat.messages)-1]
		if last.Role == "assistant" && last.Text == "" {
			if msg.Text != "" {
				last.Text = msg.Text
			} else {
				m.chat.messages = m.chat.messages[:len(m.chat.messages)-1]
			}
		}
		if len(m.chat.messages) > 0 {
			m.chat.messages[len(m.chat.messages)-1].Rendered = ""
		}
	}

	chatW, chatH := m.chatDimensions()
	m.chat.Resize(chatW, chatH)
	m.chat.Rebuild()
	return m, nil
}

// handleAgentError shows an error message.
func (m Model) handleAgentError(msg AgentErrorMsg) (tea.Model, tea.Cmd) {
	m.status.processing = false
	m.chat.streaming = false
	m.eventCh = nil
	m.err = msg.Err

	// Remove empty trailing placeholder.
	if len(m.chat.messages) > 0 {
		last := m.chat.messages[len(m.chat.messages)-1]
		if last.Role == "assistant" && last.Text == "" {
			m.chat.messages = m.chat.messages[:len(m.chat.messages)-1]
		}
	}

	m.chat.AddMessage("error", fmt.Sprintf("Error: %v", msg.Err))
	chatW, chatH := m.chatDimensions()
	m.chat.Resize(chatW, chatH)
	return m, nil
}

// handleWorkflowDone processes completed workflows.
func (m Model) handleWorkflowDone(msg WorkflowDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.chat.AddMessage("error", fmt.Sprintf("Workflow error: %v", msg.Err))
	} else {
		m.chat.AddMessage("assistant", msg.Text)
	}
	return m, nil
}

// handleMouse processes scroll and sidebar click events.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseLeft:
		if m.sidebar.show && msg.X >= m.width-sidebarWidth {
			return m.handleSidebarClick(msg)
		}
	case tea.MouseWheelUp:
		m.chat.viewport.LineUp(3)
	case tea.MouseWheelDown:
		m.chat.viewport.LineDown(3)
	}
	return m, nil
}

// handleSidebarClick toggles config flags by click position.
func (m Model) handleSidebarClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	contentY := msg.Y - headerLines - 1
	// Simplified: toggle flags at fixed positions.
	flagsStart := m.sidebarFlagsStartLine()
	flagIdx := contentY - flagsStart
	if flagIdx >= 0 && flagIdx < 5 {
		switch flagIdx {
		case 0:
			m.agentCfg.SmartRouting = !m.agentCfg.SmartRouting
		case 1:
			m.agentCfg.ContinuousCompression = !m.agentCfg.ContinuousCompression
		case 2:
			m.agentCfg.ZoneBudgeting = !m.agentCfg.ZoneBudgeting
		case 3:
			if m.agentCfg.Compaction == "" || m.agentCfg.Compaction == "none" {
				m.agentCfg.Compaction = "llm"
			} else {
				m.agentCfg.Compaction = "none"
			}
		case 4:
			if m.agent != nil {
				if m.agent.GetSteeringMode() == "aggressive" {
					m.agent.SetSteeringMode("mild")
				} else {
					m.agent.SetSteeringMode("aggressive")
				}
			}
		}
	}
	return m, nil
}

func (m Model) sidebarFlagsStartLine() int {
	line := 0
	line++ // Session title
	line++ // Tools
	line++ // Turns
	line++ // blank
	line++ // Files title
	if len(m.sidebar.modifiedFiles) == 0 {
		line++
	} else {
		line += len(m.sidebar.modifiedFiles)
	}
	line++ // blank
	line++ // Config title
	line++ // provider
	line++ // maxTok/ctxWin
	line++ // blank
	line++ // Flags title
	return line
}

// openSessions opens the session switcher overlay with branches from the DAG.
func (m *Model) openSessions() {
	var items []OverlayItem
	if m.agent != nil {
		branches, _ := m.agent.DAG().ListBranches()
		currentID := m.agent.DAG().CurrentBranchID()
		for _, b := range branches {
			marker := "  "
			if b.ID == currentID {
				marker = "\u25cf "
			}
			name := b.Name
			if len(name) > 30 {
				name = name[:27] + "..."
			}
			items = append(items, OverlayItem{
				Name:    marker + name,
				Command: "::switch:" + b.ID,
			})
		}
	}
	m.overlay.Open("sessions", items)
}
