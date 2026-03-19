// Package ui provides a Bubble Tea terminal user interface for go_sdk_agent.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
)

// ── Styles ────────────────────────────────────────────────────────────────────

// Torus spinner frames
var torusFrames = []string{"◐", "◓", "◑", "◒"}

var (
	styleUser    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // bright green
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // bright red
	styleCursor  = lipgloss.NewStyle().Reverse(true)                              // block cursor
	stylePrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green prompt
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))           // dim text
	styleSpinner = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))            // cyan spinner

	// Header bar
	styleHeaderBar = lipgloss.NewStyle().
			Background(lipgloss.Color("17")).
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Padding(0, 1)
	styleHeaderDim = lipgloss.NewStyle().
			Background(lipgloss.Color("17")).
			Foreground(lipgloss.Color("243"))
	styleSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))

	// Status bar
	styleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))

	// Scroll indicator
	styleScrollHint = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Italic(true)
)

// Layout constants — lines reserved for non-chat UI
const (
	headerLines = 2 // header bar + separator
	statusLines = 1 // status bar
	inputLines  = 2 // blank + input line
)

// ── Custom messages ───────────────────────────────────────────────────────────

type agentResponseMsg struct {
	text      string
	tokensIn  int
	tokensOut int
	cost      float64
	elapsed   time.Duration
}

type agentErrorMsg struct {
	err error
}

type streamDeltaMsg struct {
	delta string
}

type tickMsg time.Time

// ── displayMsg ────────────────────────────────────────────────────────────────

type displayMsg struct {
	role     string // "user", "assistant", "error"
	text     string
	isError  bool
	rendered string // cached glamour output (assistant only)
}

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	agent     *core.Agent
	modelName string
	skills    *features.SkillRegistry

	// Input state
	input  string
	cursor bool

	// Chat history
	messages []displayMsg

	// Viewport (scrollable chat area)
	viewport viewport.Model
	ready    bool // true after first WindowSizeMsg

	// Glamour renderer (recreated on resize)
	glamRenderer *glamour.TermRenderer

	// Status
	statusLine   string
	processing   bool
	streaming    bool
	deltaCh      chan string
	spinnerFrame int
	startTime    time.Time

	// Accumulated usage
	totalTokensIn  int
	totalTokensOut int
	totalCost      float64
	preEstimate    int

	// Terminal dimensions
	width  int
	height int

	err error
}

func newModel(agent *core.Agent, modelName string, skills *features.SkillRegistry) Model {
	return Model{
		agent:      agent,
		modelName:  modelName,
		skills:     skills,
		startTime:  time.Now(),
		statusLine: "starting...",
		messages: []displayMsg{
			{role: "assistant", text: fmt.Sprintf("Type a message. Ctrl+D or /exit to quit. /skills to list skills.")},
		},
	}
}

// ── tea.Model interface ───────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		vpHeight := m.height - headerLines - statusLines - inputLines
		if vpHeight < 1 {
			vpHeight = 1
		}

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}

		// Recreate glamour renderer for new width
		m.glamRenderer = newGlamourRenderer(m.width - 4)

		// Re-render all cached messages for new width
		for i := range m.messages {
			m.messages[i].rendered = ""
		}
		m.rebuildContent()
		return m, nil

	// ── Keyboard input ───────────────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.Type {

		case tea.KeyCtrlC:
			if m.processing {
				m.messages = append(m.messages, displayMsg{
					role:    "error",
					text:    "Agent is running — use Ctrl+D to force quit.",
					isError: true,
				})
				m.rebuildContent()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyCtrlD:
			return m, tea.Quit

		case tea.KeyPgUp, tea.KeyPgDown:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			if input == "" || m.processing {
				return m, nil
			}
			if input == "/exit" || input == "/quit" {
				return m, tea.Quit
			}
			if input == "/new" {
				oldBranch := m.agent.DAG().CurrentBranchID()
				m.agent.Hooks().Fire(context.Background(), core.HookBeforeNewBranch, &core.HookData{
					AgentID: "main",
					Meta:    map[string]any{"old_branch": oldBranch},
				})
				newBranch, _ := m.agent.DAG().NewBranch(fmt.Sprintf("session-%d", time.Now().Unix()))
				m.agent.Hooks().Fire(context.Background(), core.HookAfterNewBranch, &core.HookData{
					AgentID: "main",
					Meta:    map[string]any{"old_branch": oldBranch, "new_branch": newBranch},
				})
				m.messages = m.messages[:0]
				m.messages = append(m.messages, displayMsg{role: "assistant", text: "New conversation started (previous preserved on old branch)."})
				m.totalTokensIn = 0
				m.totalTokensOut = 0
				m.totalCost = 0
				m.input = ""
				m.rebuildContent()
				return m, nil
			}
			if input == "/clear" {
				branchID := m.agent.DAG().CurrentBranchID()
				m.agent.Hooks().Fire(context.Background(), core.HookPreClear, &core.HookData{
					AgentID: "main",
					Meta:    map[string]any{"branch": branchID},
				})
				m.agent.DAG().ResetHead()
				m.agent.Hooks().Fire(context.Background(), core.HookPostClear, &core.HookData{
					AgentID: "main",
					Meta:    map[string]any{"branch": branchID},
				})
				m.messages = m.messages[:0]
				m.messages = append(m.messages, displayMsg{role: "assistant", text: "Context cleared on current branch."})
				m.totalTokensIn = 0
				m.totalTokensOut = 0
				m.totalCost = 0
				m.input = ""
				m.rebuildContent()
				return m, nil
			}
			if input == "/skills" && m.skills != nil {
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
				m.rebuildContent()
				return m, nil
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
			// Append user message, add streaming placeholder, start agent.
			m.messages = append(m.messages, displayMsg{role: "user", text: input})
			m.messages = append(m.messages, displayMsg{role: "assistant", text: ""}) // streaming placeholder
			m.input = ""
			m.processing = true
			m.streaming = true
			m.startTime = time.Now()
			m.statusLine = m.buildStatus(0, 0, 0, 0)

			deltaCh := make(chan string, 64)
			m.deltaCh = deltaCh
			m.rebuildContent()
			return m, tea.Batch(runAgentStream(m.agent, input, deltaCh), waitForDelta(deltaCh), tick())

		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.input) > 0 {
				runes := []rune(m.input)
				runes = runes[:len(runes)-1]
				m.input = string(runes)
			}
			return m, nil

		default:
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.input += " "
			}
			return m, nil
		}

	// ── Stream delta ─────────────────────────────────────────────────────
	case streamDeltaMsg:
		if m.streaming && len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == "assistant" {
				last.text += msg.delta
				last.rendered = "" // invalidate cache during streaming
			}
		}
		m.rebuildContent()
		return m, waitForDelta(m.deltaCh)

	// ── Tick ─────────────────────────────────────────────────────────────
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
		m.streaming = false
		m.deltaCh = nil
		m.totalTokensIn += msg.tokensIn
		m.totalTokensOut += msg.tokensOut
		m.totalCost += msg.cost
		m.statusLine = m.buildStatus(m.totalTokensIn, m.totalTokensOut, m.totalCost, msg.elapsed)
		// Finalize the last message — clear rendered cache so glamour kicks in.
		if len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == "assistant" {
				last.rendered = "" // force re-render with glamour
				if last.text == "" && msg.text != "" {
					last.text = msg.text
				}
			}
		}
		m.rebuildContent()
		return m, nil

	// ── Agent error ──────────────────────────────────────────────────────
	case agentErrorMsg:
		m.processing = false
		m.streaming = false
		m.deltaCh = nil
		m.err = msg.err
		if len(m.messages) > 0 {
			last := m.messages[len(m.messages)-1]
			if last.role == "assistant" && last.text == "" {
				m.messages = m.messages[:len(m.messages)-1]
			}
		}
		m.messages = append(m.messages, displayMsg{
			role:    "error",
			text:    fmt.Sprintf("Error: %v", msg.err),
			isError: true,
		})
		m.rebuildContent()
		return m, nil
	}

	// Forward any other messages to viewport (mouse wheel, etc.)
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if !m.ready {
		return "Loading...\n"
	}

	var sb strings.Builder

	// ── Header ───────────────────────────────────────────────────────────
	sb.WriteString(m.renderHeader())
	sb.WriteByte('\n')

	// Separator
	sep := styleSeparator.Render(strings.Repeat("─", m.width))
	sb.WriteString(sep)
	sb.WriteByte('\n')

	// ── Chat viewport ────────────────────────────────────────────────────
	sb.WriteString(m.viewport.View())
	sb.WriteByte('\n')

	// ── Status bar ───────────────────────────────────────────────────────
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
	// Scroll indicator
	if !m.viewport.AtBottom() {
		statusLine += " | " + styleScrollHint.Render("PgDn ↓")
	}
	padded := statusLine
	if m.width > 0 && len(statusLine) < m.width {
		padded = statusLine + strings.Repeat(" ", m.width-len(statusLine))
	}
	sb.WriteString(styleStatus.Render(padded))
	sb.WriteByte('\n')

	// ── Input line ───────────────────────────────────────────────────────
	inputLine := stylePrompt.Render("❯ ") + m.input + styleCursor.Render(" ")
	if m.processing {
		spinner := styleSpinner.Render(torusFrames[m.spinnerFrame])
		elapsed := time.Since(m.startTime)
		if m.streaming && len(m.messages) > 0 && m.messages[len(m.messages)-1].text != "" {
			inputLine = styleDim.Render(fmt.Sprintf("%s streaming... %.1fs", spinner, elapsed.Seconds()))
		} else {
			inputLine = styleDim.Render(fmt.Sprintf("%s thinking... %.1fs", spinner, elapsed.Seconds()))
		}
	}
	sb.WriteString(inputLine)

	return sb.String()
}

// ── Content rendering ─────────────────────────────────────────────────────────

// rebuildContent renders all messages and sets the viewport content.
func (m *Model) rebuildContent() {
	if !m.ready {
		return
	}

	var sb strings.Builder
	for i := range m.messages {
		dm := &m.messages[i]
		switch dm.role {
		case "user":
			sb.WriteString(styleUser.Render("you ❯ "))
			sb.WriteString(wrapText(dm.text, m.width-6))
			sb.WriteString("\n\n")

		case "assistant":
			isStreaming := m.streaming && i == len(m.messages)-1
			if isStreaming || dm.text == "" {
				// During streaming: plain text (fast)
				sb.WriteString(wrapText(dm.text, m.width-2))
				if dm.text != "" {
					sb.WriteByte('\n')
				}
			} else {
				// Completed: glamour-rendered markdown
				if dm.rendered == "" {
					dm.rendered = m.glamourRender(dm.text)
				}
				sb.WriteString(dm.rendered)
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

// renderHeader builds the header bar.
func (m Model) renderHeader() string {
	title := styleHeaderBar.Render("◉ Torus Agent")

	branch := m.agent.DAG().CurrentBranchID()
	if len(branch) > 20 {
		branch = branch[:20] + "…"
	}

	info := styleHeaderDim.Render(fmt.Sprintf(" │ %s │ branch: %s ", m.modelName, branch))

	// Pad to full width
	titleLen := lipgloss.Width(title) + lipgloss.Width(info)
	pad := ""
	if m.width > titleLen {
		pad = styleHeaderDim.Render(strings.Repeat(" ", m.width-titleLen))
	}
	return title + info + pad
}

// glamourRender renders markdown text with glamour, falling back to plain text.
func (m Model) glamourRender(text string) string {
	if m.glamRenderer == nil {
		return text
	}
	rendered, err := m.glamRenderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(rendered, "\n")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// newGlamourRenderer creates a glamour renderer for the given width.
func newGlamourRenderer(width int) *glamour.TermRenderer {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	return r
}

// ctxBar renders a progress bar for context usage.
func ctxBar(pct float64, barLen int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(barLen))
	if filled > barLen {
		filled = barLen
	}
	empty := barLen - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("%s %.0f%%", bar, pct)
}

// buildStatus formats the status bar line.
func (m Model) buildStatus(tokIn, tokOut int, cost float64, elapsed time.Duration) string {
	var parts []string

	// Model
	parts = append(parts, fmt.Sprintf("[%s]", m.modelName))

	// CTX bar
	ctxPct := 0.0
	head, _ := m.agent.DAG().GetHead()
	if head != "" {
		msgs, _ := m.agent.DAG().PromptFrom(head)
		est := core.EstimateTokens(msgs)
		ctxPct = float64(est) / 200000.0 * 100
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
			parts = append(parts, fmt.Sprintf("%dm", mins))
		} else {
			parts = append(parts, fmt.Sprintf("%.0fs", sessionElapsed.Seconds()))
		}
	}

	// Cost
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
	return strings.TrimRight(result, "\n")
}

// ── Commands ──────────────────────────────────────────────────────────────────

func runAgentStream(agent *core.Agent, input string, deltaCh chan<- string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		agent.OnStreamDelta = func(delta string) {
			deltaCh <- delta
		}
		text, err := agent.Run(context.Background(), input)
		agent.OnStreamDelta = nil
		close(deltaCh)
		elapsed := time.Since(start)
		if err != nil {
			return agentErrorMsg{err: err}
		}
		return agentResponseMsg{
			text:    text,
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

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Entry point ───────────────────────────────────────────────────────────────

func StartTUI(agent *core.Agent, modelName string, skills *features.SkillRegistry) error {
	m := newModel(agent, modelName, skills)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
