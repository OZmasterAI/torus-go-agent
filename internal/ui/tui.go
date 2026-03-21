// Package ui provides a Bubble Tea terminal user interface for go_sdk_agent.
package ui

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"

	"go_sdk_agent/internal/commands"
	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
)

// ── Styles ────────────────────────────────────────────────────────────────────

// Orange glow colors for the bouncing progress bar (5-step gradient + track).
var (
	glowBright = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6600")) // hot orange core
	glowMed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")) // neon orange
	glowDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#cc3d00")) // warm orange
	glowFaint  = lipgloss.NewStyle().Foreground(lipgloss.Color("#993300")) // dim orange
	glowFaint2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#662200")) // between faint and track
	glowTrack  = lipgloss.NewStyle().Foreground(lipgloss.Color("#331100")) // very dim track
)

var (
	styleUser    = lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleCursor  = lipgloss.NewStyle().Reverse(true)
	stylePrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true)
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	// Header
	styleHeaderBar = lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("166")).
			Bold(true).
			Padding(0, 1)
	styleHeaderDim = lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("130"))
	styleSeparator    = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	styleInputBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01"))

	// Status bar — neon orange on dark background
	styleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a0a00")).
			Foreground(lipgloss.Color("#ff4d01"))
	styleScrollHint = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Italic(true)

	// Tool cards
	styleToolHeader = lipgloss.NewStyle().
				Foreground(lipgloss.Color("166")).
				Bold(true)
	styleToolDim = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleToolSep = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styleDiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleDiffDel = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	// Sidebar
	styleSidebarBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("130"))
	styleSidebarTitle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("166")).
				Bold(true)
	styleSidebarFile = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleSidebarCount = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// Autocomplete / Palette / Dialog
	styleACNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleACSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("214"))

	// Overlay dialog
	styleOverlayBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("130")).
				Padding(0, 1)
	styleOverlayTitle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("166")).
				Bold(true)
	styleOverlayHint = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Italic(true)
	styleKeybind = lipgloss.NewStyle().
				Foreground(lipgloss.Color("166"))
)

// Layout constants
const (
	headerLines  = 2
	statusLines  = 1
	inputLines   = 4 // border + input + border + status
	sidebarMinW  = 120 // show sidebar when terminal wider than this
	sidebarWidth = 26
	acMaxResults = 8
)

// ── Custom messages ───────────────────────────────────────────────────────────

type agentResponseMsg struct {
	text      string
	tokensIn  int
	tokensOut int
	cost      float64
	elapsed   time.Duration
}

type agentErrorMsg struct{ err error }

type streamDeltaMsg struct{ delta string }

type toolEvent struct {
	name     string
	args     map[string]any
	result   string
	isError  bool
	filePath string
	duration time.Duration
}

type toolEventMsg struct{ event toolEvent }

type statusHookMsg struct{ hook string }

type tickMsg time.Time

// ── Overlay mode ──────────────────────────────────────────────────────────────

type overlayMode int

const (
	overlayNone    overlayMode = iota
	overlayPalette             // command palette (Ctrl+K or /)
	overlaySessions            // session/branch switcher (Ctrl+B)
	overlayHelp                // keybindings help (?)
)

// paletteCommand is an entry in the command palette.
type paletteCommand struct {
	name    string // display name
	key     string // keybind hint (e.g. "Ctrl+B")
	command string // slash command to execute (e.g. "/new")
}

func defaultPaletteCommands(skills *features.SkillRegistry) []paletteCommand {
	cmds := []paletteCommand{
		{name: "New conversation", key: "", command: "/new"},
		{name: "Clear context", key: "", command: "/clear"},
		{name: "Switch session", key: "Ctrl+B", command: "::sessions"},
		{name: "Show keybindings", key: "?", command: "::help"},
		{name: "List skills", key: "", command: "/skills"},
		{name: "Exit", key: "Ctrl+D", command: "/exit"},
	}
	if skills != nil {
		for _, s := range skills.List() {
			cmds = append(cmds, paletteCommand{
				name:    s.Name + " — " + s.Description,
				command: "/" + s.Name,
			})
		}
	}
	return cmds
}

// ── displayMsg ────────────────────────────────────────────────────────────────

type displayMsg struct {
	role     string // "user", "assistant", "error", "tool"
	text     string
	isError  bool
	rendered string   // cached glamour output
	tool     *toolEvent // set when role == "tool"
}

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	agent     *core.Agent
	modelName string
	agentCfg  config.AgentConfig
	skills    *features.SkillRegistry

	// Input
	input     string
	cursor    bool
	cursorPos int // rune position within input

	// Chat
	messages []displayMsg
	viewport viewport.Model
	ready    bool

	// Glamour
	glamRenderer *glamour.TermRenderer

	// Streaming channels
	deltaCh chan string
	toolCh  chan toolEvent
	statusCh chan string
	steeringCh chan core.Message

	// Status
	statusLine   string
	statusPhrase string
	processing   bool
	streaming    bool
	barPos       int
	barDir       int
	startTime    time.Time
	ctxProgress  progress.Model

	// Usage
	totalTokensIn  int
	totalTokensOut int
	totalCost      float64
	preEstimate    int

	// Tool tracking
	toolEvents    []toolEvent
	modifiedFiles map[string]int // file path → edit count

	// Sidebar
	showSidebar bool

	// Autocomplete
	acMode    bool
	acQuery   string
	acList    []string
	acIdx     int
	acFiles   []string // cached file list
	acLoaded  bool

	// Overlay (command palette / session switcher / help)
	overlay      overlayMode
	overlayQuery string
	overlayIdx   int
	overlayItems []paletteCommand  // filtered palette commands
	branches     []core.BranchInfo // cached for session switcher

	// Terminal
	width  int
	height int
	err    error
}

func newModel(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry) Model {
	m := Model{
		agent:         agent,
		modelName:     modelName,
		agentCfg:      cfg,
		skills:        skills,
		barDir:        1,
		startTime:     time.Now(),
		statusLine:    "starting...",
		modifiedFiles: make(map[string]int),
		ctxProgress: progress.New(
			progress.WithGradient("#ff4d01", "#ff8c00"),
			progress.WithoutPercentage(),
			progress.WithWidth(12),
		),
	}

	// Load existing branch history from DAG
	head, _ := agent.DAG().GetHead()
	if head != "" {
		ancestors, _ := agent.DAG().GetAncestors(head)
		for _, node := range ancestors {
			if node.Role == "system" {
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
			role := node.Role
			if role == "tool" {
				continue // skip tool results in history view
			}
			m.messages = append(m.messages, displayMsg{role: role, text: text})
		}
	}

	if len(m.messages) == 0 {
		m.messages = []displayMsg{
			{role: "assistant", text: "Type a message. Ctrl+D or /exit to quit. /skills to list skills."},
		}
	}

	return m
}

func (m *Model) resizeViewport() {
	if !m.ready || m.height == 0 {
		return
	}
	extraLines := inputLines // border + input + border + status
	if m.processing {
		extraLines += 2 // blank line + progress bar
	}
	if m.acMode {
		extraLines += min(len(m.acList), acMaxResults) + 1
	}
	vpHeight := m.height - headerLines - statusLines - extraLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Width = m.chatWidth()
	m.viewport.Height = vpHeight
}

// ── tea.Model interface ───────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.showSidebar = m.width >= sidebarMinW

		if !m.ready {
			m.viewport = viewport.New(m.chatWidth(), 1)
			m.ready = true
		}
		m.resizeViewport()

		m.glamRenderer = newGlamourRenderer(m.chatWidth() - 4)
		for i := range m.messages {
			m.messages[i].rendered = ""
		}
		m.rebuildContent()
		return m, nil

	// ── Keyboard input ───────────────────────────────────────────────────
	case tea.KeyMsg:

		// Overlay key handling (command palette / session switcher / help)
		if m.overlay != overlayNone {
			return m.handleOverlayKey(msg)
		}

		// Autocomplete key handling
		if m.acMode {
			switch msg.Type {
			case tea.KeyTab:
				if len(m.acList) > 0 {
					m.acIdx = (m.acIdx + 1) % len(m.acList)
				}
				return m, nil
			case tea.KeyShiftTab:
				if len(m.acList) > 0 {
					m.acIdx = (m.acIdx - 1 + len(m.acList)) % len(m.acList)
				}
				return m, nil
			case tea.KeyEnter:
				if len(m.acList) > 0 {
					// Replace @query with selected file
					atPos := strings.LastIndex(m.input, "@")
					if atPos >= 0 {
						replacement := m.acList[m.acIdx] + " "
						m.input = m.input[:atPos] + replacement
						m.cursorPos = len([]rune(m.input))
					}
				}
				m.acMode = false
				return m, nil
			case tea.KeyEscape:
				m.acMode = false
				return m, nil
			case tea.KeyBackspace:
				runes := []rune(m.input)
				pos := m.cursorPos
				if pos > len(runes) {
					pos = len(runes)
				}
				if pos > 0 {
					m.input = string(runes[:pos-1]) + string(runes[pos:])
					m.cursorPos = pos - 1
					m.updateAutocomplete()
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes {
					char := string(msg.Runes)
					m.insertAtCursor(char)
					if char == " " || char == "\t" {
						m.acMode = false
					} else {
						m.updateAutocomplete()
					}
				} else if msg.Type == tea.KeySpace {
					m.insertAtCursor(" ")
					m.acMode = false
				}
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.processing {
				m.messages = append(m.messages, displayMsg{
					role: "error", text: "Agent is running — use Ctrl+D to force quit.", isError: true,
				})
				m.rebuildContent()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyCtrlD:
			return m, tea.Quit

		case tea.KeyCtrlK:
			m.openPalette()
			return m, nil

		case tea.KeyCtrlB:
			m.openSessions()
			return m, nil

		case tea.KeyCtrlW:
			// Delete word backward from cursor
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			if pos > 0 {
				newPos := pos
				// Skip trailing spaces
				for newPos > 0 && runes[newPos-1] == ' ' {
					newPos--
				}
				// Skip word characters
				for newPos > 0 && runes[newPos-1] != ' ' && runes[newPos-1] != '\n' {
					newPos--
				}
				m.input = string(runes[:newPos]) + string(runes[pos:])
				m.cursorPos = newPos
			}
			return m, nil

		case tea.KeyCtrlU:
			// Kill to start of current line
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			lineStart := pos
			for lineStart > 0 && runes[lineStart-1] != '\n' {
				lineStart--
			}
			m.input = string(runes[:lineStart]) + string(runes[pos:])
			m.cursorPos = lineStart
			return m, nil

		case tea.KeyCtrlA:
			// Move cursor to start of current line
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			for pos > 0 && runes[pos-1] != '\n' {
				pos--
			}
			m.cursorPos = pos
			return m, nil

		case tea.KeyCtrlE:
			// Move cursor to end of current line
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			for pos < len(runes) && runes[pos] != '\n' {
				pos++
			}
			m.cursorPos = pos
			return m, nil

		case tea.KeyCtrlLeft:
			// Move cursor to previous word boundary
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			if pos > 0 {
				pos--
				for pos > 0 && runes[pos] == ' ' {
					pos--
				}
				for pos > 0 && runes[pos-1] != ' ' && runes[pos-1] != '\n' {
					pos--
				}
			}
			m.cursorPos = pos
			return m, nil

		case tea.KeyCtrlRight:
			// Move cursor to next word boundary
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			if pos < len(runes) {
				pos++
				for pos < len(runes) && runes[pos] != ' ' && runes[pos] != '\n' {
					pos++
				}
				for pos < len(runes) && runes[pos] == ' ' {
					pos++
				}
			}
			m.cursorPos = pos
			return m, nil

		case tea.KeyPgUp, tea.KeyPgDown:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			if input == "" {
				return m, nil
			}
			if m.processing {
				if m.agent.Steering != nil {
					m.agent.Steering <- core.Message{Role: "user", Content: []core.ContentBlock{{Type: "text", Text: input}}}
					m.messages = append(m.messages, displayMsg{role: "user", text: "\u21aa " + input})
					m.input = ""
					m.cursorPos = 0
					m.rebuildContent()
				}
				return m, nil
			}
			if input == "/exit" || input == "/quit" {
				return m, tea.Quit
			}
			if input == "/new" {
				return m.handleNewBranch()
			}
			if input == "/clear" {
				return m.handleClear()
			}
			if input == "/skills" && m.skills != nil {
				return m.handleSkills()
			}
			// Check skills
			if m.skills != nil {
				if skillName, ok := m.skills.IsSkillCommand(input); ok {
					if skill, found := m.skills.Get(skillName); found {
						beforeSkill := &core.HookData{
							AgentID: "main",
							Meta:    map[string]any{"skill": skillName, "input": input},
						}
						m.agent.Hooks().Fire(context.Background(), core.HookBeforeSkill, beforeSkill)
						if !beforeSkill.Block {
							input = m.skills.FormatSkillPrompt(skill, input)
							m.agent.Hooks().Fire(context.Background(), core.HookAfterSkill, &core.HookData{
								AgentID: "main",
								Meta:    map[string]any{"skill": skillName, "input": input},
							})
						}
					}
				}
			}
			// Send message
			m.messages = append(m.messages, displayMsg{role: "user", text: input})
			m.messages = append(m.messages, displayMsg{role: "assistant", text: ""})
			m.input = ""
			m.cursorPos = 0
			m.processing = true
			m.streaming = true
			m.statusPhrase = "Toroidal meditation running..."
			m.startTime = time.Now()
			m.statusLine = m.buildStatus(0, 0, 0, 0)

			deltaCh := make(chan string, 64)
			toolCh := make(chan toolEvent, 32)
			statusCh := make(chan string, 16)
			m.deltaCh = deltaCh
			m.toolCh = toolCh
			m.statusCh = statusCh
			if m.agent.Steering == nil {
				m.agent.Steering = make(chan core.Message, 16)
			}
			m.resizeViewport()
			m.rebuildContent()
			return m, tea.Batch(
				runAgentStream(m.agent, input, deltaCh, toolCh, statusCh),
				waitForDelta(deltaCh),
				waitForToolEvent(toolCh),
				waitForStatus(statusCh),
				tick(),
			)

		case tea.KeyBackspace:
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			if pos > 0 {
				deleted := runes[pos-1]
				m.input = string(runes[:pos-1]) + string(runes[pos:])
				m.cursorPos = pos - 1
				if deleted == '@' {
					m.acMode = false
				}
			}
			return m, nil

		case tea.KeyDelete:
			runes := []rune(m.input)
			pos := m.cursorPos
			if pos < len(runes) {
				m.input = string(runes[:pos]) + string(runes[pos+1:])
			}
			return m, nil

		case tea.KeyLeft:
			if m.cursorPos > 0 {
				m.cursorPos--
			}
			return m, nil

		case tea.KeyRight:
			runes := []rune(m.input)
			if m.cursorPos < len(runes) {
				m.cursorPos++
			}
			return m, nil

		default:
			if msg.Type == tea.KeyRunes {
				// Handle bracketed paste: insert text without triggering shortcuts
				if msg.Paste {
					m.insertAtCursor(string(msg.Runes))
					return m, nil
				}
				char := string(msg.Runes)
				// Filter out leaked mouse escape sequences (SGR mouse: [<N;N;NM/m)
				if strings.HasPrefix(char, "[<") || strings.HasPrefix(char, "<") {
					return m, nil
				}
				if char == "/" && m.input == "" && !m.processing {
					m.openPalette()
					return m, nil
				}
				if char == "?" && m.input == "" && !m.processing {
					m.openHelp()
					return m, nil
				}
				m.insertAtCursor(char)
				if char == "@" && !m.processing {
					m.acMode = true
					m.acQuery = ""
					m.acIdx = 0
					m.ensureFileList()
					m.acList = m.acFiles
				}
			} else if msg.Type == tea.KeySpace {
				m.insertAtCursor(" ")
			}
			return m, nil
		}

	// ── Stream delta ─────────────────────────────────────────────────────
	case streamDeltaMsg:
		if m.streaming && len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == "assistant" {
				last.text += msg.delta
				last.rendered = ""
				m.statusPhrase = "Torus relaying..."
			}
		}
		m.rebuildContent()
		return m, waitForDelta(m.deltaCh)

	// ── Tool event ───────────────────────────────────────────────────────
	case toolEventMsg:
		ev := msg.event
		m.toolEvents = append(m.toolEvents, ev)
		m.statusPhrase = torusPhrase(ev.name, ev.isError)

		// Track modified files
		if ev.filePath != "" && (ev.name == "write" || ev.name == "edit") {
			m.modifiedFiles[ev.filePath]++
		}

		// Remove empty trailing placeholder
		if len(m.messages) > 0 {
			last := m.messages[len(m.messages)-1]
			if last.role == "assistant" && last.text == "" {
				m.messages = m.messages[:len(m.messages)-1]
			}
		}

		// Add tool card
		m.messages = append(m.messages, displayMsg{role: "tool", tool: &ev})
		// Add new streaming placeholder for next turn
		m.messages = append(m.messages, displayMsg{role: "assistant", text: ""})

		m.rebuildContent()
		return m, waitForToolEvent(m.toolCh)

	// ── Status hook event ────────────────────────────────────────────────
	case statusHookMsg:
		if phrase, ok := hookPhrases[msg.hook]; ok {
			m.statusPhrase = phrase
		}
		return m, waitForStatus(m.statusCh)

	// ── Tick ─────────────────────────────────────────────────────────────
	case tickMsg:
		if m.processing {
			barW := 30
			if m.width < 80 {
				barW = 20
			}
			m.barPos += m.barDir * 2
			if m.barPos >= barW-1 {
				m.barPos = barW - 1
				m.barDir = -1
			}
			if m.barPos <= 0 {
				m.barPos = 0
				m.barDir = 1
			}
			elapsed := time.Since(m.startTime)
			m.statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, elapsed)
			return m, tick()
		}
		return m, nil

	// ── Agent response ───────────────────────────────────────────────────
	case agentResponseMsg:
		m.processing = false
		m.streaming = false
		m.deltaCh = nil
		m.toolCh = nil
		m.resizeViewport()
		m.totalTokensIn += msg.tokensIn
		m.totalTokensOut += msg.tokensOut
		m.totalCost += msg.cost
		m.statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, msg.elapsed)

		// Clean up empty trailing placeholder
		if len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == "assistant" && last.text == "" {
				if msg.text != "" {
					last.text = msg.text
				} else {
					m.messages = m.messages[:len(m.messages)-1]
				}
			}
			// Force glamour re-render for final message
			if len(m.messages) > 0 {
				m.messages[len(m.messages)-1].rendered = ""
			}
		}
		m.rebuildContent()
		return m, nil

	// ── Agent error ──────────────────────────────────────────────────────
	case agentErrorMsg:
		m.processing = false
		m.streaming = false
		m.deltaCh = nil
		m.toolCh = nil
		m.resizeViewport()
		m.err = msg.err
		if len(m.messages) > 0 {
			last := m.messages[len(m.messages)-1]
			if last.role == "assistant" && last.text == "" {
				m.messages = m.messages[:len(m.messages)-1]
			}
		}
		m.messages = append(m.messages, displayMsg{
			role: "error", text: fmt.Sprintf("Error: %v", msg.err), isError: true,
		})
		m.rebuildContent()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	// Forward other messages to viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseLeft:
		if m.showSidebar && msg.X >= m.width-sidebarWidth {
			return m.handleSidebarClick(msg)
		}
	case tea.MouseWheelUp:
		m.viewport.LineUp(3)
	case tea.MouseWheelDown:
		m.viewport.LineDown(3)
	}
	// Consume all mouse events — never let raw escape codes leak
	return m, nil
}

func (m *Model) handleSidebarClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	contentY := msg.Y - headerLines - 1
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
			if m.agent.GetSteeringMode() == "aggressive" {
				m.agent.SetSteeringMode("mild")
			} else {
				m.agent.SetSteeringMode("aggressive")
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
	line++ // CTX
	if m.totalTokensIn+m.totalTokensOut > 0 {
		line++
	}
	if m.totalCost > 0 {
		line++
	}
	line++ // blank
	line++ // Files title
	if len(m.modifiedFiles) == 0 {
		line++
	} else {
		line += len(m.modifiedFiles)
	}
	line++ // blank
	line++ // Config title
	line++ // provider
	line++ // maxTok/ctxWin
	line++ // blank
	line++ // Flags title
	return line
}

// ── Slash command handlers ────────────────────────────────────────────────────

func (m *Model) handleNewBranch() (tea.Model, tea.Cmd) {
	commands.New(m.agent.DAG(), m.agent.Hooks())
	m.messages = m.messages[:0]
	m.messages = append(m.messages, displayMsg{role: "assistant", text: "New conversation started (previous preserved on old branch)."})
	m.totalTokensIn, m.totalTokensOut, m.totalCost = 0, 0, 0
	m.toolEvents = nil
	m.modifiedFiles = make(map[string]int)
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleClear() (tea.Model, tea.Cmd) {
	commands.Clear(m.agent.DAG(), m.agent.Hooks())
	m.messages = m.messages[:0]
	m.messages = append(m.messages, displayMsg{role: "assistant", text: "Context cleared on current branch."})
	m.totalTokensIn, m.totalTokensOut, m.totalCost = 0, 0, 0
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

func (m *Model) handleSkills() (tea.Model, tea.Cmd) {
	list := m.skills.List()
	if len(list) == 0 {
		m.messages = append(m.messages, displayMsg{role: "assistant", text: "No skills found in skills directory."})
	} else {
		var sb strings.Builder
		sb.WriteString("**Available skills:**\n\n")
		for _, s := range list {
			sb.WriteString(fmt.Sprintf("- `/%s` — %s\n", s.Name, s.Description))
		}
		m.messages = append(m.messages, displayMsg{role: "assistant", text: sb.String()})
	}
	m.input = ""
	m.cursorPos = 0
	m.rebuildContent()
	return m, nil
}

// ── Overlay methods ───────────────────────────────────────────────────────────

func (m *Model) openPalette() {
	m.overlay = overlayPalette
	m.overlayQuery = ""
	m.overlayIdx = 0
	m.overlayItems = defaultPaletteCommands(m.skills)
}

func (m *Model) openSessions() {
	m.overlay = overlaySessions
	m.overlayIdx = 0
	m.branches, _ = m.agent.DAG().ListBranches()
}

func (m *Model) openHelp() {
	m.overlay = overlayHelp
}

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		// Any key closes help
		m.overlay = overlayNone
		return m, nil

	case overlayPalette:
		switch msg.Type {
		case tea.KeyEscape:
			m.overlay = overlayNone
			return m, nil
		case tea.KeyUp:
			if m.overlayIdx > 0 {
				m.overlayIdx--
			}
			return m, nil
		case tea.KeyDown:
			if m.overlayIdx < len(m.overlayItems)-1 {
				m.overlayIdx++
			}
			return m, nil
		case tea.KeyTab:
			if len(m.overlayItems) > 0 {
				m.overlayIdx = (m.overlayIdx + 1) % len(m.overlayItems)
			}
			return m, nil
		case tea.KeyEnter:
			if len(m.overlayItems) > 0 {
				cmd := m.overlayItems[m.overlayIdx].command
				m.overlay = overlayNone
				return m.executePaletteCommand(cmd)
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.overlayQuery) > 0 {
				runes := []rune(m.overlayQuery)
				m.overlayQuery = string(runes[:len(runes)-1])
				m.filterPalette()
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.overlayQuery += string(msg.Runes)
				m.filterPalette()
			} else if msg.Type == tea.KeySpace {
				m.overlayQuery += " "
				m.filterPalette()
			}
			return m, nil
		}

	case overlaySessions:
		switch msg.Type {
		case tea.KeyEscape:
			m.overlay = overlayNone
			return m, nil
		case tea.KeyUp:
			if m.overlayIdx > 0 {
				m.overlayIdx--
			}
			return m, nil
		case tea.KeyDown:
			if m.overlayIdx < len(m.branches)-1 {
				m.overlayIdx++
			}
			return m, nil
		case tea.KeyTab:
			if len(m.branches) > 0 {
				m.overlayIdx = (m.overlayIdx + 1) % len(m.branches)
			}
			return m, nil
		case tea.KeyEnter:
			if len(m.branches) > 0 {
				branch := m.branches[m.overlayIdx]
				m.agent.DAG().SwitchBranch(branch.ID)
				m.overlay = overlayNone
				m.messages = m.messages[:0]
				m.messages = append(m.messages, displayMsg{
					role: "assistant",
					text: fmt.Sprintf("Switched to branch: **%s** (`%s`)", branch.Name, branch.ID),
				})
				m.rebuildContent()
			}
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) filterPalette() {
	all := defaultPaletteCommands(m.skills)
	if m.overlayQuery == "" {
		m.overlayItems = all
		m.overlayIdx = 0
		return
	}
	q := strings.ToLower(m.overlayQuery)
	var filtered []paletteCommand
	for _, cmd := range all {
		if strings.Contains(strings.ToLower(cmd.name), q) || strings.Contains(cmd.command, q) {
			filtered = append(filtered, cmd)
		}
	}
	m.overlayItems = filtered
	if m.overlayIdx >= len(filtered) {
		m.overlayIdx = 0
	}
}

func (m *Model) executePaletteCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/new":
		return m.handleNewBranch()
	case "/clear":
		return m.handleClear()
	case "/skills":
		return m.handleSkills()
	case "/exit":
		return m, tea.Quit
	case "::sessions":
		m.openSessions()
		return m, nil
	case "::help":
		m.openHelp()
		return m, nil
	default:
		// Skill commands — inject as input
		if strings.HasPrefix(cmd, "/") {
			m.input = cmd
			// Simulate Enter press by setting input and letting the user confirm
		}
		return m, nil
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if !m.ready {
		return "Loading...\n"
	}

	var sb strings.Builder

	// Header
	sb.WriteString(m.renderHeader())
	sb.WriteByte('\n')
	sep := styleSeparator.Render(strings.Repeat("─", m.width))
	sb.WriteString(sep)
	sb.WriteByte('\n')

	// Chat + optional sidebar
	chatView := m.viewport.View()
	if m.showSidebar {
		sidebar := m.renderSidebar()
		chatView = lipgloss.JoinHorizontal(lipgloss.Top, chatView, " ", sidebar)
	}
	sb.WriteString(chatView)
	sb.WriteByte('\n')

	// Input border (orange line)
	inputBorder := styleInputBorder.Render(strings.Repeat("─", m.width))

	// Overlay or normal input area
	if m.overlay != overlayNone {
		sb.WriteString(m.renderOverlay())
	} else {
		// Autocomplete dropdown (if active)
		if m.acMode && len(m.acList) > 0 {
			sb.WriteString(m.renderAutocomplete())
			sb.WriteByte('\n')
		}

		// Progress bar above the input borders (with blank line spacing)
		if m.processing {
			sb.WriteByte('\n')
			sb.WriteString(m.renderProgressBar())
			sb.WriteByte('\n')
		}

		sb.WriteString(inputBorder)
		sb.WriteByte('\n')
		sb.WriteString(m.renderInputLine())
		sb.WriteByte('\n')
		sb.WriteString(inputBorder)
		sb.WriteByte('\n')
	}

	// Status bar (below input)
	statusLine := m.statusLine
	if !m.processing {
		preEst := 0
		head, _ := m.agent.DAG().GetHead()
		if head != "" {
			msgs, _ := m.agent.DAG().PromptFrom(head)
			preEst = core.EstimatePromptCost("", msgs, nil)
			if m.input != "" {
				preEst += core.EstimateTokensForText(m.input)
			}
		}
		statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, 0)
		if preEst > 0 {
			statusLine += fmt.Sprintf(" | next: ~%s tok", fmtTok(preEst))
		}
	}
	if !m.viewport.AtBottom() {
		statusLine += " | " + styleScrollHint.Render("PgDn ↓")
	}
	padded := statusLine
	if m.width > 0 && len(statusLine) < m.width {
		padded = statusLine + strings.Repeat(" ", m.width-len(statusLine))
	}
	sb.WriteString(styleStatus.Render(padded))

	return sb.String()
}

// ── Content rendering ─────────────────────────────────────────────────────────

func (m *Model) rebuildContent() {
	if !m.ready {
		return
	}

	chatW := m.chatWidth()
	var sb strings.Builder

	for i := range m.messages {
		dm := &m.messages[i]
		switch dm.role {
		case "user":
			sb.WriteString(styleUser.Render("you ❯ "))
			sb.WriteString(wrapText(dm.text, chatW-6))
			sb.WriteString("\n\n")

		case "assistant":
			isStreaming := m.streaming && i == len(m.messages)-1
			if isStreaming || dm.text == "" {
				sb.WriteString(wrapText(dm.text, chatW-2))
				if dm.text != "" {
					sb.WriteByte('\n')
				}
			} else {
				if dm.rendered == "" {
					dm.rendered = m.glamourRender(dm.text)
				}
				sb.WriteString(dm.rendered)
				sb.WriteByte('\n')
			}

		case "tool":
			if dm.tool != nil {
				sb.WriteString(m.renderToolCard(dm.tool, chatW))
				sb.WriteByte('\n')
			}

		case "error":
			sb.WriteString(styleError.Render("error ❯ " + dm.text))
			sb.WriteString("\n\n")
		}
	}

	wasAtBottom := m.viewport.AtBottom()
	m.viewport.SetContent(sb.String())
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// ── Tool card rendering ───────────────────────────────────────────────────────

func (m Model) renderToolCard(ev *toolEvent, maxWidth int) string {
	var sb strings.Builder
	cardW := maxWidth - 4
	if cardW < 20 {
		cardW = 20
	}

	// Header line with optional duration
	headerText := fmt.Sprintf("─── %s ", ev.name)
	if ev.duration > 0 {
		headerText = fmt.Sprintf("─── %s (%s) ", ev.name, fmtDuration(ev.duration))
	}
	header := styleToolHeader.Render(headerText)
	headerPad := ""
	hLen := lipgloss.Width(header)
	if cardW > hLen {
		headerPad = styleToolSep.Render(strings.Repeat("─", cardW-hLen))
	}
	sb.WriteString("  " + header + headerPad + "\n")

	// Body based on tool type
	switch ev.name {
	case "edit":
		if ev.filePath != "" {
			sb.WriteString("  " + styleToolDim.Render(truncPath(ev.filePath, cardW-2)) + "\n")
		}
		oldStr, _ := ev.args["old_str"].(string)
		newStr, _ := ev.args["new_str"].(string)
		if oldStr != "" || newStr != "" {
			sb.WriteString(renderDiff(oldStr, newStr, cardW-4))
		}

	case "write":
		if ev.filePath != "" {
			sb.WriteString("  " + styleToolDim.Render(truncPath(ev.filePath, cardW-2)) + "\n")
		}
		content, _ := ev.args["content"].(string)
		lines := strings.Count(content, "\n") + 1
		sb.WriteString("  " + styleDim.Render(fmt.Sprintf("%d lines written", lines)) + "\n")

	case "bash":
		cmd, _ := ev.args["command"].(string)
		if cmd != "" {
			sb.WriteString("  " + styleDim.Render("$ "+truncStr(cmd, cardW-4)) + "\n")
		}
		// Show truncated output
		if ev.result != "" && !ev.isError {
			outLines := strings.Split(ev.result, "\n")
			show := outLines
			if len(show) > 5 {
				show = show[:5]
			}
			for _, line := range show {
				sb.WriteString("  " + styleToolDim.Render(truncStr(line, cardW-2)) + "\n")
			}
			if len(outLines) > 5 {
				sb.WriteString("  " + styleDim.Render(fmt.Sprintf("... +%d lines", len(outLines)-5)) + "\n")
			}
		}

	case "read":
		if ev.filePath != "" {
			sb.WriteString("  " + styleToolDim.Render(truncPath(ev.filePath, cardW-2)) + "\n")
		}

	case "glob", "grep":
		pat, _ := ev.args["pattern"].(string)
		matches := strings.Count(ev.result, "\n")
		if ev.result != "" && !strings.Contains(ev.result, "no matches") {
			matches++
		}
		sb.WriteString("  " + styleToolDim.Render(fmt.Sprintf("%s → %d matches", pat, matches)) + "\n")

	default:
		// MCP or custom tools
		sb.WriteString("  " + styleToolDim.Render(truncStr(ev.result, cardW-2)) + "\n")
	}

	if ev.isError {
		sb.WriteString("  " + styleError.Render("error") + "\n")
	}

	// Footer separator
	sb.WriteString("  " + styleToolSep.Render(strings.Repeat("─", cardW)) + "\n")

	return sb.String()
}

func renderDiff(oldStr, newStr string, maxWidth int) string {
	var sb strings.Builder
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// Cap lines shown
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
		sb.WriteString("  " + styleDiffDel.Render("- "+truncStr(line, maxWidth-4)) + "\n")
	}
	if len(oldLines) > maxLines {
		sb.WriteString("  " + styleDim.Render(fmt.Sprintf("  ... +%d lines", len(oldLines)-maxLines)) + "\n")
	}
	for _, line := range showNew {
		sb.WriteString("  " + styleDiffAdd.Render("+ "+truncStr(line, maxWidth-4)) + "\n")
	}
	if len(newLines) > maxLines {
		sb.WriteString("  " + styleDim.Render(fmt.Sprintf("  ... +%d lines", len(newLines)-maxLines)) + "\n")
	}

	return sb.String()
}

// ── Sidebar ───────────────────────────────────────────────────────────────────

func (m Model) renderSidebar() string {
	w := sidebarWidth - 4
	extraLines := inputLines
	if m.processing {
		extraLines = inputLines + 1
	}
	vpHeight := m.height - headerLines - statusLines - extraLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	innerH := vpHeight - 2
	if innerH < 1 {
		innerH = 1
	}
	var lines []string
	lines = append(lines, styleSidebarTitle.Render("Session"))
	lines = append(lines, fmt.Sprintf(" Tools: %d", len(m.toolEvents)))
	lines = append(lines, fmt.Sprintf(" Turns: %d", m.turnCount()))
	ctxPct := 0.0
	head, _ := m.agent.DAG().GetHead()
	if head != "" {
		msgs, _ := m.agent.DAG().PromptFrom(head)
		est := core.EstimateTokens(msgs)
		ctxWin := float64(m.agentCfg.ContextWindow)
		if ctxWin <= 0 {
			ctxWin = 128000
		}
		ctxPct = float64(est) / ctxWin * 100
	}
	lines = append(lines, fmt.Sprintf(" CTX: %.0f%%", ctxPct))
	totalTok := m.totalTokensIn + m.totalTokensOut
	if totalTok > 0 {
		lines = append(lines, fmt.Sprintf(" Tokens: %s", fmtTok(totalTok)))
	}
	if m.totalCost > 0 {
		lines = append(lines, fmt.Sprintf(" Cost: $%.2f", m.totalCost))
	}
	lines = append(lines, "")
	lines = append(lines, styleSidebarTitle.Render("Files"))
	if len(m.modifiedFiles) == 0 {
		lines = append(lines, styleDim.Render(" (none)"))
	} else {
		for path, count := range m.modifiedFiles {
			name := filepath.Base(path)
			if len(name) > w-6 {
				name = name[:w-6] + "…"
			}
			lines = append(lines, styleSidebarFile.Render(" "+name)+" "+styleSidebarCount.Render(fmt.Sprintf("(%d)", count)))
		}
	}
	lines = append(lines, "")
	prov := m.agentCfg.Provider
	if prov == "" {
		prov = "default"
	}
	lines = append(lines, styleSidebarTitle.Render("Config"))
	lines = append(lines, styleDim.Render(fmt.Sprintf(" %s", truncStr(prov, w-2))))
	lines = append(lines, styleDim.Render(fmt.Sprintf(" %s/%s", fmtTok(m.agentCfg.MaxTokens), fmtTok(m.agentCfg.ContextWindow))))
	lines = append(lines, "")
	lines = append(lines, styleSidebarTitle.Render("Flags"))
	lines = append(lines, flagStr("Smart", m.agentCfg.SmartRouting))
	lines = append(lines, flagStr("Compress", m.agentCfg.ContinuousCompression))
	lines = append(lines, flagStr("Zones", m.agentCfg.ZoneBudgeting))
	compact := m.agentCfg.Compaction != "" && m.agentCfg.Compaction != "none"
	lines = append(lines, flagStr("Compact", compact))
	lines = append(lines, flagStr("Steer+", m.agent.GetSteeringMode() == "aggressive"))
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	content := strings.Join(lines, "\n")
	return styleSidebarBorder.
		Width(sidebarWidth - 2).
		Height(innerH).
		Render(content)
}

func flagStr(name string, on bool) string {
	if on {
		return glowBright.Render(" ●") + " " + styleDim.Render(name)
	}
	return styleDim.Render(" ○") + " " + styleDim.Render(name)
}


// ── Autocomplete ──────────────────────────────────────────────────────────────

func (m *Model) ensureFileList() {
	if m.acLoaded {
		return
	}
	m.acFiles = loadFiles(".", 3)
	m.acLoaded = true
}

func (m *Model) updateAutocomplete() {
	atPos := strings.LastIndex(m.input, "@")
	if atPos < 0 {
		m.acMode = false
		return
	}
	m.acQuery = m.input[atPos+1:]
	m.ensureFileList()
	m.acList = filterFiles(m.acFiles, m.acQuery)
	if m.acIdx >= len(m.acList) {
		m.acIdx = 0
	}
	if len(m.acList) == 0 {
		m.acMode = false
	}
}

func (m Model) renderAutocomplete() string {
	var sb strings.Builder
	show := m.acList
	if len(show) > acMaxResults {
		show = show[:acMaxResults]
	}
	for i, path := range show {
		style := styleACNormal
		if i == m.acIdx {
			style = styleACSelected
		}
		entry := " " + truncStr(path, m.chatWidth()-4) + " "
		sb.WriteString("  " + style.Render(entry))
		if i < len(show)-1 {
			sb.WriteByte('\n')
		}
	}
	if len(m.acList) > acMaxResults {
		sb.WriteByte('\n')
		sb.WriteString("  " + styleDim.Render(fmt.Sprintf("  +%d more", len(m.acList)-acMaxResults)))
	}
	return sb.String()
}

// ── Overlay rendering ─────────────────────────────────────────────────────────

func (m Model) renderOverlay() string {
	switch m.overlay {
	case overlayPalette:
		return m.renderPalette()
	case overlaySessions:
		return m.renderSessionSwitcher()
	case overlayHelp:
		return m.renderHelp()
	}
	return ""
}

func (m Model) renderPalette() string {
	var sb strings.Builder
	sb.WriteString(styleOverlayTitle.Render("Command Palette"))
	sb.WriteString("  " + styleOverlayHint.Render("↑↓ navigate · Enter select · Esc close"))
	sb.WriteByte('\n')

	// Search input
	sb.WriteString(stylePrompt.Render("/ ") + m.overlayQuery + styleCursor.Render(" "))
	sb.WriteByte('\n')

	// Filtered commands
	keybindSelected := lipgloss.NewStyle().Foreground(lipgloss.Color("19"))
	for i, cmd := range m.overlayItems {
		style := styleACNormal
		kbStyle := styleKeybind
		if i == m.overlayIdx {
			style = styleACSelected
			kbStyle = keybindSelected
		}
		entry := " " + cmd.name + " "
		if cmd.key != "" {
			entry += kbStyle.Render("["+cmd.key+"]") + " "
		}
		sb.WriteString("  " + style.Render(entry))
		sb.WriteByte('\n')
	}
	if len(m.overlayItems) == 0 {
		sb.WriteString("  " + styleDim.Render("No matching commands"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m Model) renderSessionSwitcher() string {
	var sb strings.Builder
	sb.WriteString(styleOverlayTitle.Render("Switch Session"))
	sb.WriteString("  " + styleOverlayHint.Render("↑↓ navigate · Enter switch · Esc close"))
	sb.WriteByte('\n')

	currentID := m.agent.DAG().CurrentBranchID()

	for i, b := range m.branches {
		style := styleACNormal
		if i == m.overlayIdx {
			style = styleACSelected
		}
		marker := "  "
		if b.ID == currentID {
			marker = "● "
		}
		name := b.Name
		if len(name) > 30 {
			name = name[:27] + "…"
		}
		entry := marker + name
		sb.WriteString("  " + style.Render(entry))
		sb.WriteByte('\n')
	}
	if len(m.branches) == 0 {
		sb.WriteString("  " + styleDim.Render("No sessions found"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m Model) renderHelp() string {
	var sb strings.Builder
	sb.WriteString(styleOverlayTitle.Render("Keybindings"))
	sb.WriteString("  " + styleOverlayHint.Render("Press any key to close"))
	sb.WriteByte('\n')
	sb.WriteByte('\n')

	bindings := []struct{ key, desc string }{
		{"Ctrl+K", "Command palette"},
		{"Ctrl+B", "Switch session/branch"},
		{"/", "Command palette (empty input)"},
		{"?", "Show this help (empty input)"},
		{"@", "File autocomplete"},
		{"PgUp/PgDn", "Scroll chat history"},
		{"Ctrl+A", "Move to start of line"},
		{"Ctrl+E", "Move to end of line"},
		{"Ctrl+W", "Delete word backward"},
		{"Ctrl+U", "Kill to start of line"},
		{"Ctrl+L/R", "Word-level cursor movement"},
		{"Left/Right", "Move cursor"},
		{"Ctrl+C", "Cancel (or quit if idle)"},
		{"Ctrl+D", "Force quit"},
	}

	commands := []struct{ cmd, desc string }{
		{"/new", "Start new conversation branch"},
		{"/clear", "Clear context on current branch"},
		{"/skills", "List available skills"},
		{"/exit", "Exit the TUI"},
	}

	sb.WriteString("  " + styleKeybind.Render("Keys") + "\n")
	for _, b := range bindings {
		sb.WriteString(fmt.Sprintf("  %-14s %s\n", styleKeybind.Render(b.key), b.desc))
	}
	sb.WriteByte('\n')
	sb.WriteString("  " + styleKeybind.Render("Commands") + "\n")
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("  %-14s %s\n", styleKeybind.Render(c.cmd), c.desc))
	}
	return sb.String()
}

func loadFiles(dir string, maxDepth int) []string {
	var files []string
	baseDepth := strings.Count(filepath.Clean(dir), string(filepath.Separator))
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - baseDepth
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, rel)
		return nil
	})
	return files
}

func filterFiles(files []string, query string) []string {
	if query == "" {
		return files
	}
	q := strings.ToLower(query)
	var matches []string
	for _, f := range files {
		if strings.Contains(strings.ToLower(f), q) {
			matches = append(matches, f)
		}
	}
	return matches
}

// ── Header ────────────────────────────────────────────────────────────────────

func (m Model) renderHeader() string {
	title := styleHeaderBar.Render("◉ Torus Agent")

	branch := m.agent.DAG().CurrentBranchID()
	if len(branch) > 20 {
		branch = branch[:20] + "…"
	}

	info := styleHeaderDim.Render(fmt.Sprintf(" │ %s │ branch: %s ", m.modelName, branch))

	titleLen := lipgloss.Width(title) + lipgloss.Width(info)
	pad := ""
	if m.width > titleLen {
		pad = styleHeaderDim.Render(strings.Repeat(" ", m.width-titleLen))
	}
	return title + info + pad
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m Model) chatWidth() int {
	w := m.width
	if m.showSidebar {
		w -= sidebarWidth + 1
	}
	if w < 40 {
		w = 40
	}
	return w
}

func newGlamourRenderer(width int) *glamour.TermRenderer {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	return r
}

// osc8LinkRe matches markdown-style [text](url) links in rendered output.
var osc8LinkRe = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)

func (m Model) glamourRender(text string) string {
	if m.glamRenderer == nil {
		return text
	}
	rendered, err := m.glamRenderer.Render(text)
	if err != nil {
		return text
	}
	rendered = strings.TrimRight(rendered, "\n")
	// Convert markdown links to clickable OSC 8 hyperlinks
	rendered = osc8LinkRe.ReplaceAllString(rendered, "\033]8;;$2\033\\$1\033]8;;\033\\")
	return rendered
}

func (m Model) buildStatus(tokIn, tokOut int, cost float64, elapsed time.Duration) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", m.modelName))

	ctxPct := 0.0
	head, _ := m.agent.DAG().GetHead()
	if head != "" {
		msgs, _ := m.agent.DAG().PromptFrom(head)
		est := core.EstimateTokens(msgs)
		ctxWin := float64(m.agentCfg.ContextWindow)
		if ctxWin <= 0 {
			ctxWin = 128000
		}
		ctxPct = float64(est) / ctxWin * 100
	}
	parts = append(parts, "CTX:"+m.ctxProgress.ViewAs(ctxPct/100.0)+fmt.Sprintf(" %.0f%%", ctxPct))

	totalTok := tokIn + tokOut
	if totalTok > 0 {
		parts = append(parts, fmt.Sprintf("%s tok (%d turns)", fmtTok(totalTok), m.turnCount()))
	}

	sessionElapsed := time.Since(m.startTime)
	if !m.startTime.IsZero() && sessionElapsed > time.Second {
		mins := int(sessionElapsed.Minutes())
		if mins > 0 {
			parts = append(parts, fmt.Sprintf("%dm", mins))
		} else {
			parts = append(parts, fmt.Sprintf("%.0fs", sessionElapsed.Seconds()))
		}
	}

	if cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", cost))
	}

	return strings.Join(parts, " | ")
}

func (m Model) turnCount() int {
	count := 0
	for _, dm := range m.messages {
		if dm.role == "assistant" {
			count++
		}
	}
	return count
}

func fmtTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func truncStr(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
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
	return "…/" + dir[len(dir)-remain:] + "/" + base
}

// ansiEscRe matches ANSI escape sequences (CSI and OSC).
var ansiEscRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]|\x1b\\].*?\x1b\\\\")

// wrapText word-wraps text to maxWidth columns, preserving ANSI escape sequences.
// It splits on paragraph boundaries (\n), tokenises each paragraph into ANSI
// sequences and plain-text word/space runs, and re-emits active SGR sequences
// at the start of each wrapped line so colours are preserved.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	type token struct {
		text   string
		isAnsi bool
		width  int // visible width (0 for ANSI tokens)
	}

	tokenise := func(input string) []token {
		var tokens []token
		for len(input) > 0 {
			loc := ansiEscRe.FindStringIndex(input)
			if loc != nil && loc[0] == 0 {
				tokens = append(tokens, token{text: input[:loc[1]], isAnsi: true})
				input = input[loc[1]:]
				continue
			}
			end := len(input)
			if loc != nil {
				end = loc[0]
			}
			plain := input[:end]
			input = input[end:]
			i := 0
			for i < len(plain) {
				if plain[i] == ' ' {
					j := i
					for j < len(plain) && plain[j] == ' ' {
						j++
					}
					tokens = append(tokens, token{text: plain[i:j], width: j - i})
					i = j
				} else {
					j := i
					for j < len(plain) && plain[j] != ' ' {
						j++
					}
					word := plain[i:j]
					tokens = append(tokens, token{text: word, width: uniseg.StringWidth(word)})
					i = j
				}
			}
		}
		return tokens
	}

	isSGR := func(t token) bool {
		if !t.isAnsi {
			return false
		}
		return len(t.text) > 2 &&
			t.text[0] == 0x1b &&
			t.text[1] == '[' &&
			t.text[len(t.text)-1] == 'm'
	}

	isReset := func(t token) bool {
		return t.text == "\033[0m" || t.text == "\033[m"
	}

	var allLines []string
	for _, paragraph := range strings.Split(text, "\n") {
		tokens := tokenise(paragraph)
		if len(tokens) == 0 {
			allLines = append(allLines, "")
			continue
		}
		var lineBuf strings.Builder
		col := 0
		var activeSeqs []string

		flushLine := func() {
			allLines = append(allLines, lineBuf.String())
			lineBuf.Reset()
			col = 0
			for _, seq := range activeSeqs {
				lineBuf.WriteString(seq)
			}
		}

		for _, tok := range tokens {
			if tok.isAnsi {
				lineBuf.WriteString(tok.text)
				if isSGR(tok) {
					if isReset(tok) {
						activeSeqs = activeSeqs[:0]
					} else {
						activeSeqs = append(activeSeqs, tok.text)
					}
				}
				continue
			}
			// Space token
			if len(tok.text) > 0 && tok.text[0] == ' ' {
				if col == 0 {
					continue
				}
				if col+tok.width > maxWidth {
					flushLine()
					continue
				}
				lineBuf.WriteString(tok.text)
				col += tok.width
				continue
			}
			// Word token
			if tok.width == 0 {
				lineBuf.WriteString(tok.text)
				continue
			}
			if col+tok.width <= maxWidth {
				lineBuf.WriteString(tok.text)
				col += tok.width
				continue
			}
			if col > 0 {
				flushLine()
			}
			// Word wider than a full line: break by grapheme cluster
			if tok.width > maxWidth {
				remaining := tok.text
				for len(remaining) > 0 {
					cluster, rest, cw, _ := uniseg.FirstGraphemeClusterInString(remaining, -1)
					if col+cw > maxWidth {
						flushLine()
					}
					lineBuf.WriteString(cluster)
					col += cw
					remaining = rest
				}
				continue
			}
			lineBuf.WriteString(tok.text)
			col += tok.width
		}
		if lineBuf.Len() > 0 {
			allLines = append(allLines, lineBuf.String())
		}
	}

	result := strings.Join(allLines, "\n")
	return strings.TrimRight(result, "\n")
}

func (m Model) renderProgressBar() string {
	barW := 30
	if m.width < 80 {
		barW = 20
	}
	var bar strings.Builder
	for i := 0; i < barW; i++ {
		dist := m.barPos - i
		if dist < 0 {
			dist = -dist
		}
		switch {
		case dist == 0:
			bar.WriteString(glowBright.Render("\u2501"))
		case dist == 1:
			bar.WriteString(glowMed.Render("\u2501"))
		case dist == 2:
			bar.WriteString(glowDim.Render("\u2501"))
		case dist == 3:
			bar.WriteString(glowFaint.Render("\u2501"))
		case dist == 4:
			bar.WriteString(glowFaint2.Render("\u2501"))
		default:
			bar.WriteString(glowTrack.Render("\u2501"))
		}
	}
	elapsed := time.Since(m.startTime)
	phrase := m.statusPhrase
	if phrase == "" {
		phrase = "Toroidal meditation running..."
	}
	timeStr := styleDim.Render(fmt.Sprintf(" %.1fs", elapsed.Seconds()))
	amberStyle := amberCycle(elapsed).Italic(true)
	return bar.String() + amberStyle.Render(" "+phrase) + timeStr
}

// amberCycle returns a lipgloss style that smoothly cycles through amber/orange shades
// using true-color RGB interpolation. Completes a full cycle every 3 seconds.
func amberCycle(elapsed time.Duration) lipgloss.Style {
	// Amber gradient keypoints: bright amber → deep orange → dark amber → back
	type rgb struct{ r, g, b int }
	keys := []rgb{
		{255, 191, 0},   // bright amber
		{249, 115, 22},  // orange (#f97316)
		{194, 65, 12},   // deep orange
		{130, 50, 10},   // dark amber
		{194, 65, 12},   // deep orange (return)
		{249, 115, 22},  // orange (return)
	}
	// Smooth interpolation: 3 second full cycle
	t := math.Mod(elapsed.Seconds()*2, float64(len(keys)))
	i := int(t)
	frac := t - float64(i)
	a := keys[i%len(keys)]
	b := keys[(i+1)%len(keys)]
	r := a.r + int(float64(b.r-a.r)*frac)
	g := a.g + int(float64(b.g-a.g)*frac)
	bl := a.b + int(float64(b.b-a.b)*frac)
	color := fmt.Sprintf("#%02x%02x%02x", r, g, bl)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

// ── Torus Status Phrases ──────────────────────────────────────────────────────

func torusPhrase(toolName string, isError bool) string {
	if isError {
		return "\u26a0 \u2620 Error \u2620 \u26a0"
	}
	switch toolName {
	case "bash":
		return "executing on the surface..."
	case "read":
		return "reading through the ring..."
	case "edit":
		return "Inscribing the Torus..."
	case "write":
		return "Expanding the Torus..."
	case "glob", "grep":
		return "Toroidal scan..."
	case "spawn":
		return "spawning a loop..."
	case "delegate":
		return "delegating to inner ring..."
	case "recall_branch":
		return "exploring the Torus..."
	default:
		return "orbiting the Torus..."
	}
}

var hookPhrases = map[string]string{
	"on_user_input":        "parsing the meridian...",
	"before_context_build": "Toroidal mapping...",
	"before_llm_call":     "Toroidal meditation running...",
	"after_llm_call":      "completing the circuit...",
	"pre_compact":         "compressing the manifold...",
	"post_compact":        "Toroidal folding...",
	"on_error":            "\u26a0 \u2620 Error \u2620 \u26a0",
}

// ── Commands ──────────────────────────────────────────────────────────────────

func runAgentStream(agent *core.Agent, input string, deltaCh chan<- string, toolCh chan<- toolEvent, statusCh chan<- string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		var finalText string
		var finalErr error
		var toolStartTime time.Time
		for ev := range agent.RunStream(context.Background(), input) {
			switch ev.Type {
			case core.EventAgentTextDelta:
				deltaCh <- ev.Text
			case core.EventAgentToolStart:
				toolStartTime = time.Now()
			case core.EventAgentToolEnd:
				dur := time.Duration(0)
				if !toolStartTime.IsZero() {
					dur = time.Since(toolStartTime)
				}
				fp, _ := ev.ToolArgs["file_path"].(string)
				toolCh <- toolEvent{
					name:     ev.ToolName,
					args:     ev.ToolArgs,
					result:   ev.ToolResult.Content,
					isError:  ev.ToolResult.IsError,
					filePath: fp,
					duration: dur,
				}
				toolStartTime = time.Time{}
			case core.EventAgentDone:
				finalText = ev.Text
			case core.EventStatusUpdate:
				statusCh <- ev.StatusHook
			case core.EventAgentError:
				finalErr = ev.Error
			}
		}
		close(deltaCh)
		close(toolCh)
		close(statusCh)
		elapsed := time.Since(start)
		if finalErr != nil {
			return agentErrorMsg{err: finalErr}
		}
		return agentResponseMsg{
			text:    finalText,
			elapsed: elapsed,
		}
	}
}

func waitForDelta(ch <-chan string) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		delta, ok := <-ch
		if !ok {
			return nil
		}
		return streamDeltaMsg{delta: delta}
	}
}

func waitForToolEvent(ch <-chan toolEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return toolEventMsg{event: ev}
	}
}

func waitForStatus(ch <-chan string) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		hook, ok := <-ch
		if !ok {
			return nil
		}
		return statusHookMsg{hook: hook}
	}
}

func tick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Cursor-aware input helpers ────────────────────────────────────────────────

// insertAtCursor inserts text at the current cursor position and advances the cursor.
func (m *Model) insertAtCursor(text string) {
	runes := []rune(m.input)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:pos]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[pos:]...)
	m.input = string(newRunes)
	m.cursorPos = pos + len(inserted)
}

// renderInputLine renders the input prompt with a thin beam cursor.
func (m Model) renderInputLine() string {
	prompt := stylePrompt.Render("❯ ")
	runes := []rune(m.input)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	before := string(runes[:pos])
	after := string(runes[pos:])
	cursor := styleCursor.Render("│")
	return prompt + before + cursor + after
}

// fmtDuration formats a duration for compact display in tool cards.
func fmtDuration(d time.Duration) string {
	if d < 500*time.Millisecond {
		return "<0.5s"
	}
	if d < time.Second {
		ms := d.Milliseconds()
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

// ── Entry point ───────────────────────────────────────────────────────────────

func StartTUI(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry) error {
	m := newModel(agent, modelName, cfg, skills)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
