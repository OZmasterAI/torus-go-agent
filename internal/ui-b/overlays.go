package uib

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"torus_go_agent/internal/features"
)

// OverlayItem is an entry in a palette, session list, or help overlay.
type OverlayItem struct {
	Name    string // Display text.
	Key     string // Keybind hint (e.g. "Ctrl+B").
	Command string // Slash command to execute (e.g. "/new").
}

// workflowAgent is one agent entry in the workflow builder.
type workflowAgent struct {
	Task      string
	AgentType string
}

var workflowTypes = []string{"builder", "researcher", "tester"}

// workflowState tracks the interactive workflow overlay.
type workflowState struct {
	mode          string // "sequential", "parallel", "loop"
	agents        []workflowAgent
	editIdx       int
	taskInput     string
	typeIdx       int
	actionIdx     int
	maxIterations string
	stopPhrase    string
}

// overlayModel is a generic reusable overlay for command palette, session
// switcher, help, and workflow builder.
type overlayModel struct {
	theme    Theme
	active   bool
	kind     string // "palette", "sessions", "help", "workflow"
	items    []OverlayItem
	filtered []OverlayItem
	query    string
	idx      int
	workflow workflowState
}

func newOverlayModel(theme Theme) overlayModel {
	return overlayModel{theme: theme}
}

// Open activates the overlay with the given kind and items.
func (o *overlayModel) Open(kind string, items []OverlayItem) {
	o.active = true
	o.kind = kind
	o.items = items
	o.filtered = items
	o.query = ""
	o.idx = 0
}

// Close deactivates the overlay.
func (o *overlayModel) Close() { o.active = false }

// Active returns whether the overlay is visible.
func (o overlayModel) Active() bool { return o.active }

// Kind returns the type of overlay currently displayed.
func (o overlayModel) Kind() string { return o.kind }

// SetQuery filters the items by substring match on name or command.
func (o *overlayModel) SetQuery(q string) {
	o.query = q
	if q == "" {
		o.filtered = o.items
		o.idx = 0
		return
	}
	lq := strings.ToLower(q)
	var filtered []OverlayItem
	for _, item := range o.items {
		if strings.Contains(strings.ToLower(item.Name), lq) || strings.Contains(item.Command, lq) {
			filtered = append(filtered, item)
		}
	}
	o.filtered = filtered
	if o.idx >= len(filtered) {
		o.idx = 0
	}
}

// Filtered returns the currently filtered items.
func (o overlayModel) Filtered() []OverlayItem { return o.filtered }

// Selected returns the currently highlighted item.
func (o overlayModel) Selected() OverlayItem {
	if o.idx < len(o.filtered) {
		return o.filtered[o.idx]
	}
	return OverlayItem{}
}

// Update handles key events for the overlay. Returns the selected command
// string (non-empty means the user selected something) and a tea.Cmd.
func (o *overlayModel) Update(msg tea.Msg) (string, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return "", nil
	}

	switch o.kind {
	case "help":
		o.active = false
		return "", nil

	case "palette":
		return o.handlePaletteKey(keyMsg)

	case "sessions":
		return o.handleSessionsKey(keyMsg)

	case "workflow":
		return o.handleWorkflowKey(keyMsg)
	}
	return "", nil
}

func (o *overlayModel) handlePaletteKey(msg tea.KeyMsg) (string, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		o.active = false
		return "", nil
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return "", nil
	case tea.KeyDown:
		if o.idx < len(o.filtered)-1 {
			o.idx++
		}
		return "", nil
	case tea.KeyTab:
		if len(o.filtered) > 0 {
			o.idx = (o.idx + 1) % len(o.filtered)
		}
		return "", nil
	case tea.KeyEnter:
		if len(o.filtered) > 0 {
			cmd := o.filtered[o.idx].Command
			o.active = false
			return cmd, nil
		}
		return "", nil
	case tea.KeyBackspace:
		if len(o.query) > 0 {
			runes := []rune(o.query)
			o.SetQuery(string(runes[:len(runes)-1]))
		}
		return "", nil
	default:
		if msg.Type == tea.KeyRunes {
			o.SetQuery(o.query + string(msg.Runes))
		} else if msg.Type == tea.KeySpace {
			o.SetQuery(o.query + " ")
		}
		return "", nil
	}
}

func (o *overlayModel) handleSessionsKey(msg tea.KeyMsg) (string, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		o.active = false
		return "", nil
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return "", nil
	case tea.KeyDown:
		if o.idx < len(o.filtered)-1 {
			o.idx++
		}
		return "", nil
	case tea.KeyTab:
		if len(o.filtered) > 0 {
			o.idx = (o.idx + 1) % len(o.filtered)
		}
		return "", nil
	case tea.KeyEnter:
		if len(o.filtered) > 0 {
			cmd := o.filtered[o.idx].Command
			o.active = false
			return cmd, nil
		}
		return "", nil
	}
	return "", nil
}

func (o *overlayModel) handleWorkflowKey(msg tea.KeyMsg) (string, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		o.active = false
		return "", nil
	case tea.KeyTab:
		o.workflow.editIdx = (o.workflow.editIdx + 1) % o.workflowFieldCount()
		o.workflow.actionIdx = 0
		return "", nil
	case tea.KeyShiftTab:
		o.workflow.editIdx = (o.workflow.editIdx - 1 + o.workflowFieldCount()) % o.workflowFieldCount()
		o.workflow.actionIdx = 0
		return "", nil
	case tea.KeyUp, tea.KeyDown:
		dir := 1
		if msg.Type == tea.KeyUp {
			dir = -1
		}
		if o.workflow.editIdx == 1 {
			o.workflow.typeIdx = (o.workflow.typeIdx + dir + 3) % 3
		}
		actionsField := o.workflowFieldCount() - 1
		if o.workflow.editIdx == actionsField {
			ac := o.workflowActionCount()
			if ac > 0 {
				o.workflow.actionIdx = (o.workflow.actionIdx + dir + ac) % ac
			}
		}
		return "", nil
	case tea.KeyEnter:
		return o.handleWorkflowEnter()
	case tea.KeyBackspace:
		switch o.workflow.editIdx {
		case 0:
			if len(o.workflow.taskInput) > 0 {
				r := []rune(o.workflow.taskInput)
				o.workflow.taskInput = string(r[:len(r)-1])
			}
		case 2:
			if o.workflow.mode == "loop" && len(o.workflow.maxIterations) > 0 {
				r := []rune(o.workflow.maxIterations)
				o.workflow.maxIterations = string(r[:len(r)-1])
			}
		case 3:
			if o.workflow.mode == "loop" && len(o.workflow.stopPhrase) > 0 {
				r := []rune(o.workflow.stopPhrase)
				o.workflow.stopPhrase = string(r[:len(r)-1])
			}
		}
		return "", nil
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			ch := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				ch = " "
			}
			switch o.workflow.editIdx {
			case 0:
				o.workflow.taskInput += ch
			case 2:
				if o.workflow.mode == "loop" {
					for _, c := range ch {
						if c >= '0' && c <= '9' {
							o.workflow.maxIterations += string(c)
						}
					}
				}
			case 3:
				if o.workflow.mode == "loop" {
					o.workflow.stopPhrase += ch
				}
			}
		}
		return "", nil
	}
}

func (o *overlayModel) handleWorkflowEnter() (string, tea.Cmd) {
	actionsField := o.workflowFieldCount() - 1
	if o.workflow.editIdx != actionsField {
		o.workflow.editIdx = (o.workflow.editIdx + 1) % o.workflowFieldCount()
		return "", nil
	}

	if o.workflow.mode == "loop" {
		if len(o.workflow.agents) == 0 {
			if strings.TrimSpace(o.workflow.taskInput) == "" {
				return "", nil
			}
			o.workflow.agents = append(o.workflow.agents, workflowAgent{
				Task:      strings.TrimSpace(o.workflow.taskInput),
				AgentType: workflowTypes[o.workflow.typeIdx],
			})
			o.workflow.taskInput = ""
			o.workflow.actionIdx = 0
			return "", nil
		}
		switch o.workflow.actionIdx {
		case 0: // Run
			o.active = false
			return "::run_workflow", nil
		case 1: // Remove
			o.workflow.agents = o.workflow.agents[:0]
		}
		return "", nil
	}

	// Sequential / Parallel
	switch o.workflow.actionIdx {
	case 0: // Add agent
		task := strings.TrimSpace(o.workflow.taskInput)
		if task == "" {
			return "", nil
		}
		o.workflow.agents = append(o.workflow.agents, workflowAgent{
			Task:      task,
			AgentType: workflowTypes[o.workflow.typeIdx],
		})
		o.workflow.taskInput = ""
		o.workflow.typeIdx = 0
	case 1: // Run
		o.active = false
		return "::run_workflow", nil
	case 2: // Remove last
		if len(o.workflow.agents) > 0 {
			o.workflow.agents = o.workflow.agents[:len(o.workflow.agents)-1]
		}
	}
	return "", nil
}

func (o *overlayModel) workflowFieldCount() int {
	if o.workflow.mode == "loop" {
		return 5
	}
	return 3
}

func (o *overlayModel) workflowActionCount() int {
	if o.workflow.mode == "loop" {
		if len(o.workflow.agents) > 0 {
			return 2
		}
		return 0
	}
	n := 1
	if len(o.workflow.agents) > 0 {
		n += 2
	}
	return n
}

// View renders the overlay based on its kind.
func (o overlayModel) View() string {
	switch o.kind {
	case "palette":
		return o.renderPalette()
	case "sessions":
		return o.renderSessions()
	case "help":
		return o.renderHelp()
	case "workflow":
		return o.renderWorkflow()
	}
	return ""
}

func (o overlayModel) renderPalette() string {
	var sb strings.Builder
	sb.WriteString(o.theme.OverlayTitle.Render("Command Palette"))
	sb.WriteString("  " + o.theme.OverlayHint.Render("\u2191\u2193 navigate \u00b7 Enter select \u00b7 Esc close"))
	sb.WriteByte('\n')
	sb.WriteString(o.theme.Prompt.Render("/ ") + o.query + o.theme.Cursor.Render(" "))
	sb.WriteByte('\n')

	keybindSelected := lipgloss.NewStyle().Foreground(lipgloss.Color("19"))
	for i, item := range o.filtered {
		style := o.theme.ACNormal
		kbStyle := o.theme.Keybind
		if i == o.idx {
			style = o.theme.ACSelected
			kbStyle = keybindSelected
		}
		entry := " " + item.Name + " "
		if item.Key != "" {
			entry += kbStyle.Render("["+item.Key+"]") + " "
		}
		sb.WriteString("  " + style.Render(entry))
		sb.WriteByte('\n')
	}
	if len(o.filtered) == 0 {
		sb.WriteString("  " + o.theme.Dim.Render("No matching commands"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (o overlayModel) renderSessions() string {
	var sb strings.Builder
	sb.WriteString(o.theme.OverlayTitle.Render("Switch Session"))
	sb.WriteString("  " + o.theme.OverlayHint.Render("\u2191\u2193 navigate \u00b7 Enter switch \u00b7 Esc close"))
	sb.WriteByte('\n')

	for i, item := range o.filtered {
		style := o.theme.ACNormal
		if i == o.idx {
			style = o.theme.ACSelected
		}
		sb.WriteString("  " + style.Render(item.Name))
		sb.WriteByte('\n')
	}
	if len(o.filtered) == 0 {
		sb.WriteString("  " + o.theme.Dim.Render("No sessions found"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (o overlayModel) renderHelp() string {
	var sb strings.Builder
	sb.WriteString(o.theme.OverlayTitle.Render("Keybindings"))
	sb.WriteString("  " + o.theme.OverlayHint.Render("Press any key to close"))
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
		{"/compact", "Compact conversation context"},
		{"/fork", "Fork branch (head, -back N, node ID, branch)"},
		{"/switch", "Switch branch (list, index, ID)"},
		{"/branches", "List all branches"},
		{"/alias", "Name a node (/alias <name> [node-id])"},
		{"/messages", "Show message history"},
		{"/steering", "Set steering mode"},
		{"/stats", "Show session stats"},
		{"/agents", "List running agents"},
		{"/mcp-tools", "List MCP tools"},
		{"/skills", "List available skills"},
		{"/exit", "Exit the TUI"},
	}

	sb.WriteString("  " + o.theme.Keybind.Render("Keys") + "\n")
	for _, b := range bindings {
		sb.WriteString(fmt.Sprintf("  %-14s %s\n", o.theme.Keybind.Render(b.key), b.desc))
	}
	sb.WriteByte('\n')
	sb.WriteString("  " + o.theme.Keybind.Render("Commands") + "\n")
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("  %-14s %s\n", o.theme.Keybind.Render(c.cmd), c.desc))
	}
	return sb.String()
}

func (o overlayModel) renderWorkflow() string {
	var sb strings.Builder
	title := strings.ToUpper(o.workflow.mode)
	sb.WriteString(o.theme.OverlayTitle.Render("Workflow: " + title))
	sb.WriteString("  " + o.theme.OverlayHint.Render("Tab cycle  \u2191\u2193 select  Enter confirm  Esc cancel"))
	sb.WriteByte('\n')
	sb.WriteByte('\n')

	for i, a := range o.workflow.agents {
		sb.WriteString(fmt.Sprintf("  %d. %s ", i+1, a.Task))
		sb.WriteString(o.theme.Dim.Render("[" + a.AgentType + "]"))
		sb.WriteByte('\n')
	}
	if len(o.workflow.agents) > 0 {
		sb.WriteByte('\n')
	}

	focusStyle := func(idx int) lipgloss.Style {
		if o.workflow.editIdx == idx {
			return o.theme.ACSelected
		}
		return o.theme.Dim
	}

	cursor := ""
	if o.workflow.editIdx == 0 {
		cursor = "\u2588"
	}
	if o.workflow.mode == "loop" && len(o.workflow.agents) > 0 {
		sb.WriteString(o.theme.Dim.Render(fmt.Sprintf("  Task: %s", o.workflow.agents[0].Task)))
	} else {
		sb.WriteString(focusStyle(0).Render(fmt.Sprintf("  Task: %s%s", o.workflow.taskInput, cursor)))
	}
	sb.WriteByte('\n')

	sb.WriteString("  Type: ")
	for i, t := range workflowTypes {
		s := o.theme.Dim
		if i == o.workflow.typeIdx && o.workflow.editIdx == 1 {
			s = o.theme.ACSelected
		} else if i == o.workflow.typeIdx {
			s = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
		}
		sb.WriteString(s.Render(t))
		if i < len(workflowTypes)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteByte('\n')

	if o.workflow.mode == "loop" {
		iterCursor := ""
		if o.workflow.editIdx == 2 {
			iterCursor = "\u2588"
		}
		sb.WriteString(focusStyle(2).Render(fmt.Sprintf("  Max iterations: %s%s", o.workflow.maxIterations, iterCursor)))
		sb.WriteByte('\n')

		stopCursor := ""
		if o.workflow.editIdx == 3 {
			stopCursor = "\u2588"
		}
		sb.WriteString(focusStyle(3).Render(fmt.Sprintf("  Stop phrase: %s%s", o.workflow.stopPhrase, stopCursor)))
		sb.WriteByte('\n')
	}

	sb.WriteByte('\n')
	actionsField := o.workflowFieldCount() - 1
	onActions := o.workflow.editIdx == actionsField

	if o.workflow.mode == "loop" {
		if len(o.workflow.agents) == 0 {
			s := o.theme.Dim
			if onActions {
				s = o.theme.ACSelected
			}
			sb.WriteString("  " + s.Render("[ Set agent (Enter) ]"))
		} else {
			actions := []string{"Run", "Remove"}
			for i, a := range actions {
				s := o.theme.Dim
				if onActions && o.workflow.actionIdx == i {
					s = o.theme.ACSelected
				}
				sb.WriteString("  " + s.Render("[ "+a+" ]"))
			}
		}
	} else {
		actions := []string{"Add agent"}
		if len(o.workflow.agents) > 0 {
			actions = append(actions, "Run", "Remove last")
		}
		for i, a := range actions {
			s := o.theme.Dim
			if onActions && o.workflow.actionIdx == i {
				s = o.theme.ACSelected
			}
			sb.WriteString("  " + s.Render("[ "+a+" ]"))
		}
	}
	sb.WriteByte('\n')
	return sb.String()
}

// DefaultPaletteCommands returns the default command palette entries.
func DefaultPaletteCommands(skills *features.SkillRegistry) []OverlayItem {
	items := []OverlayItem{
		{Name: "New conversation", Command: "/new"},
		{Name: "Clear context", Command: "/clear"},
		{Name: "Compact context", Command: "/compact"},
		{Name: "Fork branch", Command: "/fork"},
		{Name: "Switch branch", Key: "Ctrl+B", Command: "::sessions"},
		{Name: "List branches", Command: "/branches"},
		{Name: "Show messages", Command: "/messages"},
		{Name: "Steering mode", Command: "/steering"},
		{Name: "Session stats", Command: "/stats"},
		{Name: "Running agents", Command: "/agents"},
		{Name: "MCP tools", Command: "/mcp-tools"},
		{Name: "Show keybindings", Key: "?", Command: "::help"},
		{Name: "List skills", Command: "/skills"},
		{Name: "Workflow: sequential", Command: "/sequential"},
		{Name: "Workflow: parallel", Command: "/parallel"},
		{Name: "Workflow: loop", Command: "/loop"},
		{Name: "Exit", Key: "Ctrl+D", Command: "/exit"},
	}
	if skills != nil {
		for _, s := range skills.List() {
			items = append(items, OverlayItem{
				Name:    s.Name + " \u2014 " + s.Description,
				Command: "/" + s.Name,
			})
		}
	}
	return items
}
