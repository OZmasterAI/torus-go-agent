package uib

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

// Layout constants.
const (
	headerLines = 2
	statusLines = 1
	inputLines  = 4 // border + input + border + status
)

// TUIExtras holds optional dependencies for slash commands.
type TUIExtras struct {
	Telemetry *features.TelemetryCollector
	SubMgr    *features.SubAgentManager
	MCPClient *features.MCPClient
}

// Model is the top-level Bubble Tea model, composing all sub-models.
type Model struct {
	// Sub-models.
	chat    chatModel
	input   inputModel
	overlay overlayModel
	sidebar sidebarModel
	status  statusModel

	// Startup screen (shown before main chat when enabled).
	startup      startupModel
	startupPhase bool // true while the startup screen is active

	// Dependencies.
	agent     *core.Agent
	modelName string
	agentCfg  config.AgentConfig
	skills    *features.SkillRegistry
	telemetry *features.TelemetryCollector
	subMgr    *features.SubAgentManager
	mcpClient *features.MCPClient

	// Streaming.
	eventCh chan StreamEventMsg

	// Session tracking.
	sessionStart    time.Time // When the TUI was created (for elapsed time display).
	turnCount       int       // Number of user submissions sent to the agent.
	lastInputTokens int       // Input tokens from most recent agent run (for CTX%).

	// Terminal.
	width  int
	height int
	ready  bool
	err    error
}

// NewModel creates a new Model with all sub-models wired up.
func NewModel(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry, extras *TUIExtras) Model {
	theme := DefaultTheme()

	var tel *features.TelemetryCollector
	var sub *features.SubAgentManager
	var mcp *features.MCPClient
	if extras != nil {
		tel = extras.Telemetry
		sub = extras.SubMgr
		mcp = extras.MCPClient
	}

	m := Model{
		chat:      newChatModel(theme, 80, 20),
		input:     newInputModel(theme, 80),
		overlay:   newOverlayModel(theme),
		sidebar:   newSidebarModel(theme, cfg),
		status:    newStatusModel(theme),
		agent:     agent,
		modelName: modelName,
		agentCfg:  cfg,
		skills:    skills,
		telemetry: tel,
		subMgr:    sub,
		mcpClient:    mcp,
		sessionStart: time.Now(),
	}

	// Load existing branch history from DAG.
	if agent != nil {
		m.loadHistory()
	}

	if len(m.chat.messages) == 0 {
		m.chat.messages = []DisplayMsg{
			{Role: "assistant", Text: "Type a message. Ctrl+D or /exit to quit. /skills to list skills."},
		}
	}

	return m
}

// NewModelWithStartup creates a new Model that shows the startup screen first.
// The startup screen will be shown before the main chat. When the user finishes
// setup, the StartupDoneMsg is sent and the main chat begins.
func NewModelWithStartup(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry, extras *TUIExtras) Model {
	m := NewModel(agent, modelName, cfg, skills, extras)
	su := newStartupModel()
	su.savedConfig = &cfg
	m.startup = su
	m.startupPhase = true
	return m
}

// loadHistory populates chat messages from the DAG branch history.
func (m *Model) loadHistory() {
	head, _ := m.agent.DAG().GetHead()
	if head == "" {
		return
	}
	ancestors, _ := m.agent.DAG().GetAncestors(head)
	for _, node := range ancestors {
		if node.Role == "system" || node.Role == "tool" {
			continue
		}
		text := ""
		for _, b := range node.Content {
			if b.Text != "" {
				text = b.Text
				break
			}
		}
		if text == "" {
			continue
		}
		m.chat.messages = append(m.chat.messages, DisplayMsg{Role: node.Role, Text: text, Ts: time.UnixMilli(node.Timestamp)})
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	if m.startupPhase {
		return startupTickCmd()
	}
	return nil
}

// chatDimensions returns the chat viewport size accounting for sidebar.
func (m Model) chatDimensions() (int, int) {
	w := m.chatWidth()
	extraLines := inputLines
	if m.status.processing {
		extraLines += 2
	}
	if m.input.acMode {
		n := len(m.input.acList)
		if n > acMaxResults {
			n = acMaxResults
		}
		extraLines += n + 1
	}
	h := m.height - headerLines - statusLines - extraLines
	if h < 1 {
		h = 1
	}
	return w, h
}

// chatWidth returns the width available for the chat viewport.
func (m Model) chatWidth() int {
	w := m.width
	if m.sidebar.show {
		w -= sidebarWidth + 1
	}
	if w < 40 {
		w = 40
	}
	return w
}

// chatHeight returns the height of the chat viewport.
func (m Model) chatHeight() int {
	_, h := m.chatDimensions()
	return h
}
