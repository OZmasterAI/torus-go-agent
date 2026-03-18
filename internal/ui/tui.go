// Package ui provides a Bubble Tea terminal user interface for go_sdk_agent.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
)

// ── Styles ────────────────────────────────────────────────────────────────────

// Torus spinner frames
var torusFrames = []string{"◐", "◓", "◑", "◒"}

var (
	styleUser      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)  // bright green
	styleAssistant = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))             // white
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)   // bright red
	styleStatus    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))  // grey bar
	styleCursor    = lipgloss.NewStyle().Reverse(true)                                // block cursor
	stylePrompt    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)  // green prompt
	styleHeader    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)  // cyan header
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))            // dim text
	styleSpinner   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))             // cyan spinner
)

// ── Custom messages ───────────────────────────────────────────────────────────

// agentResponseMsg carries a successful agent reply back to the Bubble Tea loop.
type agentResponseMsg struct {
	text     string
	tokensIn int
	tokensOut int
	cost     float64
	elapsed  time.Duration
}

// agentErrorMsg carries an agent error back to the Bubble Tea loop.
type agentErrorMsg struct {
	err error
}

// tickMsg drives the elapsed-time display while the agent is processing.
type tickMsg time.Time

// ── displayMsg ────────────────────────────────────────────────────────────────

// displayMsg is a single rendered chat entry.
type displayMsg struct {
	role    string // "user", "assistant", "error"
	text    string
	isError bool
}

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the Bubble Tea model for the chat TUI.
type Model struct {
	agent      *core.Agent
	modelName  string
	skills     *features.SkillRegistry

	// Input state
	input  string // current typed text
	cursor bool   // blink state (unused – always shown)

	// Chat history
	messages []displayMsg

	// Status
	statusLine string
	processing   bool
	spinnerFrame int
	startTime  time.Time

	// Accumulated usage across the session
	totalTokensIn  int
	totalTokensOut int
	totalCost      float64

	// Pre-send token estimate (updated on each render when not processing)
	preEstimate int

	// Terminal dimensions
	width  int
	height int

	// Last error (non-fatal)
	err error
}

// newModel builds the initial model.
func newModel(agent *core.Agent, modelName string, skills *features.SkillRegistry) Model {
	return Model{
		agent:     agent,
		modelName: modelName,
		skills:    skills,
		startTime:  time.Now(),
		statusLine: "starting...",
		messages: []displayMsg{
			{role: "assistant", text: fmt.Sprintf("◉ Torus Agent [%s]\n  Type a message. Ctrl+D or /exit to quit. /skills to list skills.", modelName)},
		},
	}
}

// ── tea.Model interface ───────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	// ── Keyboard input ───────────────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.Type {

		case tea.KeyCtrlC:
			if m.processing {
				// No cancellation mechanism in this agent version; just inform.
				m.messages = append(m.messages, displayMsg{
					role:    "error",
					text:    "Agent is running — use Ctrl+D to force quit.",
					isError: true,
				})
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyCtrlD:
			return m, tea.Quit

		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			if input == "" || m.processing {
				return m, nil
			}
			if input == "/exit" || input == "/quit" {
				return m, tea.Quit
			}
			if input == "/new" {
				// Start fresh branch
				head, _ := m.agent.DAG().GetHead()
				if head != "" {
					m.agent.DAG().Branch(head, fmt.Sprintf("session-%d", time.Now().Unix()))
				}
				m.messages = m.messages[:0]
				m.messages = append(m.messages, displayMsg{role: "assistant", text: "New conversation started (previous preserved on old branch)."})
				m.totalTokensIn = 0
				m.totalTokensOut = 0
				m.totalCost = 0
				m.input = ""
				return m, nil
			}
			if input == "/skills" && m.skills != nil {
				list := m.skills.List()
				if len(list) == 0 {
					m.messages = append(m.messages, displayMsg{role: "assistant", text: "No skills found in skills directory."})
				} else {
					var sb strings.Builder
					sb.WriteString("Available skills:\n")
					for _, s := range list {
						sb.WriteString(fmt.Sprintf("  /%s — %s\n", s.Name, s.Description))
					}
					m.messages = append(m.messages, displayMsg{role: "assistant", text: sb.String()})
				}
				m.input = ""
				return m, nil
			}
			// Check skills
			if m.skills != nil {
				if skillName, ok := m.skills.IsSkillCommand(input); ok {
					if skill, found := m.skills.Get(skillName); found {
						input = m.skills.FormatSkillPrompt(skill, input)
					}
				}
			}
			// Append user message to display, clear input, start agent.
			m.messages = append(m.messages, displayMsg{role: "user", text: input})
			m.input = ""
			m.processing = true
			m.startTime = time.Now()
			m.statusLine = m.buildStatus(0, 0, 0, 0)
			return m, tea.Batch(runAgent(m.agent, input), tick())

		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.input) > 0 {
				// Remove last rune (safe for ASCII; rune-aware below).
				runes := []rune(m.input)
				runes = runes[:len(runes)-1]
				m.input = string(runes)
			}
			return m, nil

		default:
			// Append printable characters.
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.input += " "
			}
			return m, nil
		}

	// ── Tick (elapsed timer while processing) ────────────────────────────
	case tickMsg:
		if m.processing {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(torusFrames)
			elapsed := time.Since(m.startTime)
			m.statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, elapsed)
			return m, tick()
		}
		return m, nil

	// ── Agent response ───────────────────────────────────────────────────
	case agentResponseMsg:
		m.processing = false
		m.totalTokensIn += msg.tokensIn
		m.totalTokensOut += msg.tokensOut
		m.totalCost += msg.cost
		m.statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, msg.elapsed)
		m.messages = append(m.messages, displayMsg{role: "assistant", text: msg.text})
		return m, nil

	// ── Agent error ──────────────────────────────────────────────────────
	case agentErrorMsg:
		m.processing = false
		m.err = msg.err
		m.messages = append(m.messages, displayMsg{
			role:    "error",
			text:    fmt.Sprintf("Error: %v", msg.err),
			isError: true,
		})
		return m, nil
	}

	return m, nil
}

// View renders the full TUI frame.
func (m Model) View() string {
	if m.width == 0 {
		// Not yet sized — return minimal placeholder.
		return "Loading...\n"
	}

	var sb strings.Builder

	// ── Chat history ─────────────────────────────────────────────────────
	// Reserve lines: 1 status bar + 1 blank + 1 input line + 1 blank = 4 lines.
	const reservedLines = 4
	historyHeight := m.height - reservedLines
	if historyHeight < 1 {
		historyHeight = 1
	}

	// Build all history lines, then take the last historyHeight.
	var histLines []string
	for _, dm := range m.messages {
		var prefix, body string
		switch dm.role {
		case "user":
			prefix = styleUser.Render("you> ")
			body = wrapText(dm.text, m.width-5)
			lines := strings.Split(body, "\n")
			for i, line := range lines {
				if i == 0 {
					histLines = append(histLines, prefix+line)
				} else {
					histLines = append(histLines, "     "+line)
				}
			}
		case "assistant":
			prefix = styleAssistant.Render("assistant> ")
			body = wrapText(dm.text, m.width-11)
			lines := strings.Split(body, "\n")
			for i, line := range lines {
				if i == 0 {
					histLines = append(histLines, prefix+line)
				} else {
					histLines = append(histLines, "           "+line)
				}
			}
		case "error":
			body = styleError.Render("error> " + dm.text)
			histLines = append(histLines, body)
		}
		histLines = append(histLines, "") // blank line between messages
	}

	// Trim to visible window (scroll to bottom).
	if len(histLines) > historyHeight {
		histLines = histLines[len(histLines)-historyHeight:]
	}

	// Pad to fill historyHeight so the layout is stable.
	for len(histLines) < historyHeight {
		histLines = append([]string{""}, histLines...)
	}

	sb.WriteString(strings.Join(histLines, "\n"))
	sb.WriteByte('\n')

	// ── Status bar (recomputed each frame) ──────────────────────────────
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
	padded := statusLine
	if m.width > 0 && len(statusLine) < m.width {
		padded = statusLine + strings.Repeat(" ", m.width-len(statusLine))
	}
	sb.WriteString(styleStatus.Render(padded))
	sb.WriteByte('\n')

	// ── Blank separator ──────────────────────────────────────────────────
	sb.WriteByte('\n')

	// ── Input line ───────────────────────────────────────────────────────
	inputLine := stylePrompt.Render("you> ") + m.input + styleCursor.Render(" ")
	if m.processing {
		spinner := styleSpinner.Render(torusFrames[m.spinnerFrame])
		elapsed := time.Since(m.startTime)
		inputLine = styleDim.Render(fmt.Sprintf("%s thinking... %.1fs", spinner, elapsed.Seconds()))
	}
	sb.WriteString(inputLine)

	return sb.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// ctxBar renders a progress bar for context usage.
func ctxBar(pct float64, barLen int) string {
	if pct < 0 { pct = 0 }
	if pct > 100 { pct = 100 }
	filled := int(pct / 100 * float64(barLen))
	if filled > barLen { filled = barLen }
	empty := barLen - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("%s %.1f%%", bar, pct)
}

// buildStatus formats the rich status bar line.
func (m Model) buildStatus(tokIn, tokOut int, cost float64, elapsed time.Duration) string {
	var parts []string

	// Model
	parts = append(parts, fmt.Sprintf("[%s]", m.modelName))

	// CTX bar
	ctxPct := 0.0
	{ // CTX bar
		head, _ := m.agent.DAG().GetHead()
		if head != "" {
			msgs, _ := m.agent.DAG().PromptFrom(head)
			est := core.EstimateTokens(msgs)
			ctxPct = float64(est) / 200000.0 * 100 // assume 200k context
		}
	}
	parts = append(parts, "CTX:"+ctxBar(ctxPct, 8))

	// Tokens
	totalTok := tokIn + tokOut
	if totalTok > 0 {
		parts = append(parts, fmt.Sprintf("%s tok (%d turns)", fmtTok(totalTok), m.turnCount()))
	}

	// Timer
	sessionElapsed := time.Since(m.startTime)
	if !m.startTime.IsZero() && sessionElapsed > time.Second {
		mins := int(sessionElapsed.Minutes())
		if mins > 0 {
			parts = append(parts, fmt.Sprintf("⏱️ %dm", mins))
		} else {
			parts = append(parts, fmt.Sprintf("⏱️ %.0fs", sessionElapsed.Seconds()))
		}
	}

	// Cost
	if cost > 0 {
		parts = append(parts, fmt.Sprintf("💰$%.2f", cost))
	}

	return strings.Join(parts, " | ")
}

func (m Model) turnCount() int {
	count := 0
	for _, dm := range m.messages {
		if dm.role == "assistant" { count++ }
	}
	return count
}

// fmtTok formats a token count as "1.2k", "12.4k", "1.2M", or raw number.
func fmtTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// wrapText does a simple word-wrap at maxWidth columns.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var out strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out.WriteByte('\n')
			continue
		}
		col := 0
		for i, w := range words {
			wlen := len([]rune(w))
			if i == 0 {
				out.WriteString(w)
				col = wlen
			} else if col+1+wlen > maxWidth {
				out.WriteByte('\n')
				out.WriteString(w)
				col = wlen
			} else {
				out.WriteByte(' ')
				out.WriteString(w)
				col += 1 + wlen
			}
		}
		out.WriteByte('\n')
	}
	result := out.String()
	// Trim trailing newline added above.
	return strings.TrimRight(result, "\n")
}

// ── Commands ──────────────────────────────────────────────────────────────────

// runAgent returns a tea.Cmd that executes agent.Run in a goroutine.
func runAgent(agent *core.Agent, input string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		text, err := agent.Run(context.Background(), input)
		elapsed := time.Since(start)
		if err != nil {
			return agentErrorMsg{err: err}
		}
		// Token/cost data not yet plumbed through agent.Run return value;
		// leave as zero — status bar will show elapsed time.
		return agentResponseMsg{
			text:    text,
			elapsed: elapsed,
		}
	}
}

// tick returns a command that fires a tickMsg after one second.
func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Entry point ───────────────────────────────────────────────────────────────

// StartTUI launches the Bubble Tea program in alt-screen mode.
// It blocks until the user quits.
func StartTUI(agent *core.Agent, modelName string, skills *features.SkillRegistry) error {
	m := newModel(agent, modelName, skills)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
