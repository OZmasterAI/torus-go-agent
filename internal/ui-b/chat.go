package uib

import (
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"torus_go_agent/internal/tui/shared"
)

// osc8LinkRe matches markdown-style [text](url) links in rendered output.
var osc8LinkRe = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)

// chatModel manages the chat viewport, message list, glamour rendering,
// tool card display, and thinking blocks.
type chatModel struct {
	theme        Theme
	messages     []DisplayMsg
	viewport     viewport.Model
	glamRenderer *glamour.TermRenderer
	toolCards    *ToolCardRegistry
	thinking     shared.ThinkingModel
	ready        bool
	streaming    bool
}

func newChatModel(theme Theme, width, height int) chatModel {
	vp := viewport.New(width, height)
	glamR := newGlamourRenderer(width - 4)
	return chatModel{
		theme:        theme,
		viewport:     vp,
		glamRenderer: glamR,
		toolCards:    NewToolCardRegistry(theme),
		ready:        false,
	}
}

// Update forwards window size messages to the viewport.
func (c *chatModel) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case tea.WindowSizeMsg:
		// Resize handled by the parent via Resize().
	}
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	return cmd
}

// View returns the viewport content.
func (c chatModel) View() string { return c.viewport.View() }

// AddMessage appends a new message and rebuilds the viewport.
func (c *chatModel) AddMessage(role, text string) {
	c.messages = append(c.messages, DisplayMsg{Role: role, Text: text, Ts: time.Now()})
	c.Rebuild()
}

// AddToolCard inserts a tool card message, removing any trailing empty
// assistant placeholder first. Then adds a new empty placeholder for the
// next streaming turn.
func (c *chatModel) AddToolCard(ev *ToolEvent) {
	// Remove empty trailing placeholder.
	if len(c.messages) > 0 {
		last := c.messages[len(c.messages)-1]
		if last.Role == "assistant" && last.Text == "" {
			c.messages = c.messages[:len(c.messages)-1]
		}
	}
	c.messages = append(c.messages, DisplayMsg{Role: "tool", Tool: ev, Ts: time.Now()})
	c.messages = append(c.messages, NewDisplayMsg("assistant", ""))
	c.Rebuild()
}

// AppendDelta appends streaming text to the last assistant message.
func (c *chatModel) AppendDelta(delta string) {
	if len(c.messages) > 0 {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			last.Text += delta
			last.Rendered = "" // Invalidate cache.
		}
	}
	c.Rebuild()
}

// Rebuild re-renders all messages into the viewport.
func (c *chatModel) Rebuild() {
	chatW := c.viewport.Width
	if chatW < 40 {
		chatW = 40
	}
	var sb strings.Builder

	// Render finalized thinking cards before messages.
	for _, card := range c.thinking.Cards {
		sb.WriteString(c.thinking.RenderCard(card, chatW))
	}

	verbosity := c.thinking.Verbosity

	for i := range c.messages {
		dm := &c.messages[i]
		switch dm.Role {
		case "user":
			ts := fmtTimestamp(dm.Ts)
			sb.WriteString(c.theme.Timestamp.Render(ts) + " " + c.theme.User.Render("\u25c9 <you>") + "\n")
			sb.WriteString("            " + wrapText(dm.Text, chatW-12))
			sb.WriteString("\n\n")

		case "assistant":
			isStreaming := c.streaming && i == len(c.messages)-1
			if isStreaming || dm.Text == "" {
				// Render pending thinking before streaming assistant text.
				if isStreaming {
					sb.WriteString(c.thinking.RenderPending(chatW))
				}
				sb.WriteString(indentBlock(wrapText(dm.Text, chatW-12), "          "))
				if dm.Text != "" {
					sb.WriteByte('\n')
				}
			} else {
				if !dm.Ts.IsZero() {
					ts := fmtTimestamp(dm.Ts)
					sb.WriteString(c.theme.Timestamp.Render(ts) + " " + c.theme.AssistantPrefix.Render("\u25c9 <torus>") + "\n")
				}
				if dm.Rendered == "" {
					dm.Rendered = c.glamourRender(dm.Text)
				}
				sb.WriteString(indentBlock(dm.Rendered, "          "))
				sb.WriteString("\n\n\n")
			}

		case "tool":
			if dm.Tool != nil {
				if verbosity == shared.VerbosityCompact {
					// Compact: no timestamp prefix, tool renders its own indent.
					sb.WriteString(c.toolCards.Render(dm.Tool, chatW, verbosity))
				} else {
					// Verbose/Full: timestamp + full card with header/footer.
					ts := fmtTimestamp(dm.Ts)
					sb.WriteString(c.theme.Timestamp.Render(ts) + " ")
					sb.WriteString(c.toolCards.Render(dm.Tool, chatW-10, verbosity))
				}
				sb.WriteByte('\n')
			}

		case "error":
			ts := fmtTimestamp(dm.Ts)
			sb.WriteString(c.theme.Timestamp.Render(ts) + " " + c.theme.Error.Render("\u2717 error \u276f " + dm.Text))
			sb.WriteString("\n\n")
		}
	}

	wasAtBottom := c.viewport.AtBottom()
	c.viewport.SetContent(sb.String())
	if wasAtBottom || c.streaming {
		c.viewport.GotoBottom()
	}
}

// Resize updates the viewport dimensions and re-creates the glamour renderer.
func (c *chatModel) Resize(width, height int) {
	c.viewport.Width = width
	c.viewport.Height = height
	c.glamRenderer = newGlamourRenderer(width - 4)
	// Invalidate all cached renders.
	for i := range c.messages {
		c.messages[i].Rendered = ""
	}
	c.Rebuild()
}

func (c *chatModel) glamourRender(text string) string {
	if c.glamRenderer == nil {
		return text
	}
	rendered, err := c.glamRenderer.Render(text)
	if err != nil {
		return text
	}
	rendered = strings.TrimRight(rendered, "\n")
	rendered = strings.TrimLeft(rendered, "\n")
	rendered = osc8LinkRe.ReplaceAllString(rendered, "\033]8;;$2\033\\$1\033]8;;\033\\")
	return rendered
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
