package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SetupResult holds the configuration choices from the setup screen.
type SetupResult struct {
	Provider string
	Model    string
	Config   *AgentConfigOverrides // nil means "use existing config.json"
}

// AgentConfigOverrides holds agent settings that can be set during setup.
type AgentConfigOverrides struct {
	MaxTokens              int
	ContextWindow          int
	Compaction             string
	CompactionTrigger      string
	CompactionThreshold    int
	CompactionMaxMessages  int
	CompactionKeepLastN    int
	ContinuousCompression  bool
	CompressionKeepLast    int
	CompressionMinMessages int
	ZoneBudgeting          bool
	ZoneArchivePercent     int
	SmartRouting           bool
	SteeringAggressive     bool
}

func defaultOverrides() *AgentConfigOverrides {
	return &AgentConfigOverrides{
		MaxTokens:              8192,
		ContextWindow:          128000,
		Compaction:             "llm",
		CompactionTrigger:      "both",
		CompactionThreshold:    75,
		CompactionMaxMessages:  50,
		CompactionKeepLastN:    20,
		ContinuousCompression:  true,
		CompressionKeepLast:    10,
		CompressionMinMessages: 0,
		ZoneBudgeting:          true,
		ZoneArchivePercent:     25,
		SmartRouting:           false,
		SteeringAggressive:     false,
	}
}

type configField struct {
	name string
	kind string // "bool", "int", "string"
}

var configFields = []configField{
	{"MaxTokens", "int"},
	{"ContextWindow", "int"},
	{"Compaction", "string"},
	{"CompactionTrigger", "string"},
	{"CompactionThreshold", "int"},
	{"CompactionMaxMessages", "int"},
	{"CompactionKeepLastN", "int"},
	{"ContinuousCompression", "bool"},
	{"CompressionKeepLast", "int"},
	{"CompressionMinMessages", "int"},
	{"ZoneBudgeting", "bool"},
	{"ZoneArchivePercent", "int"},
	{"SmartRouting", "bool"},
	{"SteeringAggressive", "bool"},
}

func (o *AgentConfigOverrides) getValue(idx int) string {
	switch idx {
	case 0:
		return fmt.Sprintf("%d", o.MaxTokens)
	case 1:
		return fmt.Sprintf("%d", o.ContextWindow)
	case 2:
		return o.Compaction
	case 3:
		return o.CompactionTrigger
	case 4:
		return fmt.Sprintf("%d", o.CompactionThreshold)
	case 5:
		return fmt.Sprintf("%d", o.CompactionMaxMessages)
	case 6:
		return fmt.Sprintf("%d", o.CompactionKeepLastN)
	case 7:
		if o.ContinuousCompression {
			return "true"
		}
		return "false"
	case 8:
		return fmt.Sprintf("%d", o.CompressionKeepLast)
	case 9:
		return fmt.Sprintf("%d", o.CompressionMinMessages)
	case 10:
		if o.ZoneBudgeting {
			return "true"
		}
		return "false"
	case 11:
		return fmt.Sprintf("%d", o.ZoneArchivePercent)
	case 12:
		if o.SmartRouting {
			return "true"
		}
		return "false"
	case 13:
		if o.SteeringAggressive {
			return "true"
		}
		return "false"
	}
	return ""
}

func (o *AgentConfigOverrides) setValue(idx int, val string) {
	n, _ := strconv.Atoi(val)
	switch idx {
	case 0:
		o.MaxTokens = n
	case 1:
		o.ContextWindow = n
	case 2:
		o.Compaction = val
	case 3:
		o.CompactionTrigger = val
	case 4:
		o.CompactionThreshold = n
	case 5:
		o.CompactionMaxMessages = n
	case 6:
		o.CompactionKeepLastN = n
	case 7:
		o.ContinuousCompression = val == "true"
	case 8:
		o.CompressionKeepLast = n
	case 9:
		o.CompressionMinMessages = n
	case 10:
		o.ZoneBudgeting = val == "true"
	case 11:
		o.ZoneArchivePercent = n
	case 12:
		o.SmartRouting = val == "true"
	case 13:
		o.SteeringAggressive = val == "true"
	}
}

func (o *AgentConfigOverrides) toggleBool(idx int) {
	switch idx {
	case 7:
		o.ContinuousCompression = !o.ContinuousCompression
	case 10:
		o.ZoneBudgeting = !o.ZoneBudgeting
	case 12:
		o.SmartRouting = !o.SmartRouting
	case 13:
		o.SteeringAggressive = !o.SteeringAggressive
	}
}

// torusFrames provides spinner characters used by the main TUI (tui.go).
var torusFrames = []string{"◐", "◓", "◑", "◒"}

// ProviderChoice holds a provider option for the startup menu.
type ProviderChoice struct {
	Name          string
	Provider      string // "openrouter", "nvidia", or "anthropic"
	Model         string
	NeedsKey      string // env var name
	ContextWindow int    // model's context window size
	MaxTokens     int    // max output tokens
}

// DefaultProviderChoices returns the standard options.
func DefaultProviderChoices() []ProviderChoice {
	return []ProviderChoice{
		{Name: "OpenRouter (hunter-alpha)", Provider: "openrouter", Model: "openrouter/hunter-alpha", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "OpenRouter (nemotron-3-super)", Provider: "openrouter", Model: "nvidia/nemotron-3-super-120b-a12b:free", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 131072, MaxTokens: 8192},
		{Name: "OpenRouter (step-3.5-flash)", Provider: "openrouter", Model: "stepfun/step-3.5-flash:free", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "NVIDIA NIM (GLM-4.7)", Provider: "nvidia", Model: "z-ai/glm4.7", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 32768, MaxTokens: 8192},
		{Name: "NVIDIA NIM (Qwen3.5-122B)", Provider: "nvidia", Model: "qwen/qwen3.5-122b-a10b", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 262144, MaxTokens: 16384},
		{Name: "NVIDIA NIM (llama-3.3-70b)", Provider: "nvidia", Model: "meta/llama-3.3-70b-instruct", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "Anthropic Claude (OAuth)", Provider: "anthropic", Model: "claude-sonnet-4-5-20250929", NeedsKey: "", ContextWindow: 200000, MaxTokens: 64000},
		{Name: "Anthropic Claude (API key)", Provider: "anthropic", Model: "claude-sonnet-4-5-20250929", NeedsKey: "ANTHROPIC_API_KEY", ContextWindow: 200000, MaxTokens: 64000},
		{Name: "Custom model", Provider: "", Model: "", NeedsKey: "", ContextWindow: 128000, MaxTokens: 8192},
	}
}

// ── Color palette (amber/orange) ──────────────────────────────────────────────

var (
	colorBrightAmber = lipgloss.Color("166")
	colorOrange      = lipgloss.Color("130")
	colorDarkOrange  = lipgloss.Color("94")
	colorDimOrange   = lipgloss.Color("88")
	colorMutedGold   = lipgloss.Color("130")
	colorDarkBg      = lipgloss.Color("52")
)

// Styles derived from the palette.
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMutedGold).
			Italic(true)

	menuPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOrange).
			Padding(1, 2).
			Background(colorDarkBg)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(colorBrightAmber).
				Bold(true).
				Padding(0, 1)

	menuHeaderStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true).
			Underline(true).
			MarginBottom(1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Italic(true)

	// styleSpinner is used by the main TUI (tui.go) for its spinner.
	styleSpinner = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	// Torus character luminance styles.
	torusDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("130"))
	torusMidStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("172"))
	torusBrightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	torusMaxStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	textInputStyle = lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Bold(true)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(colorMutedGold)
)

// ── ASCII art title ───────────────────────────────────────────────────────────

const asciiTitle = ` ████████╗  ██████╗  ██████╗  ██╗   ██╗ ███████╗
 ╚══██╔══╝ ██╔═══██╗ ██╔══██╗ ██║   ██║ ██╔════╝
    ██║    ██║   ██║ ██████╔╝ ██║   ██║ ███████╗
    ██║    ██║   ██║ ██╔══██╗ ██║   ██║ ╚════██║
    ██║    ╚██████╔╝ ██║  ██║ ╚██████╔╝ ███████║
    ╚═╝     ╚═════╝  ╚═╝  ╚═╝  ╚═════╝  ╚══════╝`

// ── Anthropic model choices ───────────────────────────────────────────────────

type anthropicModel struct {
	Name string
	ID   string
}

var anthropicModels = []anthropicModel{
	{Name: "Claude Opus 4.6", ID: "claude-opus-4-6"},
	{Name: "Claude Sonnet 4.6", ID: "claude-sonnet-4-6"},
	{Name: "Claude Sonnet 4.5", ID: "claude-sonnet-4-5-20250929"},
	{Name: "Claude Haiku 4.5", ID: "claude-haiku-4-5-20251001"},
	{Name: "Custom model ID", ID: ""},
}

// ── Tick command for startup animation ────────────────────────────────────────
// tickMsg is declared in tui.go as `type tickMsg time.Time` (same package).

func startupTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Setup model ───────────────────────────────────────────────────────────────

type setupModel struct {
	width, height int
	ready         bool

	// Torus animation
	torusA, torusB float64
	torusFrame     string

	// Menu
	phase   int // 0=main, 1=provider, 2=anthropic, 3=config mode, 4=edit settings
	cursor  int
	choices []ProviderChoice

	// Text input for custom provider/model
	textInput string
	inputMode bool // true when typing custom provider/model
	inputStep int  // 0=provider name, 1=model id

	// Config customization (phase 3 = choose config mode, phase 4 = edit settings)
	configOverrides *AgentConfigOverrides
	configCursor    int    // cursor for config editing in phase 4
	editingConfig   bool   // true when editing a numeric/string value
	editBuffer      string // text buffer for numeric/string input

	// Result
	provider string
	model    string
	done     bool
}

func newSetupModel() setupModel {
	m := setupModel{
		choices: DefaultProviderChoices(),
	}
	m.torusFrame = renderTorus(m.torusA, m.torusB)
	return m
}

func (m setupModel) Init() tea.Cmd {
	return startupTickCmd()
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case tickMsg:
		m.torusA += 0.04
		m.torusB += 0.02
		m.torusFrame = renderTorus(m.torusA, m.torusB)
		return m, startupTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m setupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Config editing mode (phase 4 inline edit) ────────────────
	if m.editingConfig {
		return m.handleConfigEdit(msg)
	}

	// ── Text input mode ──────────────────────────────────────────
	if m.inputMode {
		return m.handleTextInput(msg)
	}

	// ── Normal menu navigation ───────────────────────────────────
	switch msg.String() {

	case "ctrl+c", "q":
		m.provider = ""
		m.model = ""
		m.done = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		max := m.menuLen() - 1
		if m.cursor < max {
			m.cursor++
		}

	case "enter":
		return m.selectItem()

	case "esc":
		switch m.phase {
		case 4:
			m.phase = 3
			m.cursor = 0
		case 3:
			// Go back to where we came from (phase 1 or 0)
			if m.provider != "" {
				// We already picked a provider, go back to provider select
				m.phase = 1
			} else {
				m.phase = 0
			}
			m.cursor = 0
		default:
			if m.phase > 0 {
				m.phase--
				m.cursor = 0
			}
		}
	}

	return m, nil
}

func (m setupModel) handleConfigEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.editBuffer)
		if val != "" {
			m.configOverrides.setValue(m.cursor, val)
		}
		m.editingConfig = false
		m.editBuffer = ""
	case "esc":
		m.editingConfig = false
		m.editBuffer = ""
	case "backspace":
		if len(m.editBuffer) > 0 {
			m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.editBuffer += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "ctrl+c":
		m.provider = ""
		m.model = ""
		m.done = true
		return m, tea.Quit

	case "esc":
		m.inputMode = false
		m.textInput = ""
		m.inputStep = 0
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.textInput)
		if val == "" {
			return m, nil
		}
		// Custom provider flow: step 0 = provider name, step 1 = model ID
		if m.phase == 1 {
			if m.inputStep == 0 {
				m.provider = val
				m.textInput = ""
				m.inputStep = 1
				return m, nil
			}
			// step 1: got model ID → go to config phase
			m.model = val
			m.inputMode = false
			m.textInput = ""
			m.inputStep = 0
			m.phase = 3
			m.cursor = 0
			m.configOverrides = defaultOverrides()
			return m, nil
		}
		// Custom anthropic model (phase 2) → go to config phase
		if m.phase == 2 {
			m.provider = "anthropic"
			m.model = val
			m.inputMode = false
			m.textInput = ""
			m.phase = 3
			m.cursor = 0
			m.configOverrides = defaultOverrides()
			return m, nil
		}
		return m, nil

	case "backspace":
		if len(m.textInput) > 0 {
			m.textInput = m.textInput[:len(m.textInput)-1]
		}

	default:
		// Only accept printable single characters
		if len(msg.String()) == 1 {
			m.textInput += msg.String()
		}
	}

	return m, nil
}

func (m setupModel) menuLen() int {
	switch m.phase {
	case 0:
		return 3 // Use defaults, Use existing config, Choose provider & model
	case 1:
		return len(m.choices)
	case 2:
		return len(anthropicModels)
	case 3:
		return 3 // Use defaults, Use existing, Customize
	case 4:
		return len(configFields) + 1 // +1 for "Done" item
	}
	return 0
}

func (m setupModel) selectItem() (tea.Model, tea.Cmd) {
	switch m.phase {

	case 0: // Main menu
		switch m.cursor {
		case 0: // Use defaults → go to config phase
			m.provider = "openrouter"
			m.model = "openrouter/hunter-alpha"
			m.phase = 3
			m.cursor = 0
			m.configOverrides = defaultOverrides()
		case 1: // Use existing config → done immediately
			m.provider = ""
			m.model = ""
			m.done = true
			return m, tea.Quit
		case 2: // Choose provider & model
			m.phase = 1
			m.cursor = 0
		}

	case 1: // Provider select
		choice := m.choices[m.cursor]
		if choice.Provider == "" {
			// Custom: start text input
			m.inputMode = true
			m.inputStep = 0
			m.textInput = ""
			return m, nil
		}
		if choice.Provider == "anthropic" {
			m.phase = 2
			m.cursor = 0
			return m, nil
		}
		// Direct selection → go to config phase
		m.provider = choice.Provider
		m.model = choice.Model
		m.phase = 3
		m.cursor = 0
		m.configOverrides = defaultOverrides()
		return m, nil

	case 2: // Anthropic model
		am := anthropicModels[m.cursor]
		if am.ID == "" {
			// Custom model ID input
			m.inputMode = true
			m.textInput = ""
			return m, nil
		}
		// Direct selection → go to config phase
		m.provider = "anthropic"
		m.model = am.ID
		m.phase = 3
		m.cursor = 0
		m.configOverrides = defaultOverrides()
		return m, nil

	case 3: // Config mode
		switch m.cursor {
		case 0: // Use defaults
			m.configOverrides = defaultOverrides()
			m.done = true
			return m, tea.Quit
		case 1: // Use existing
			m.configOverrides = nil
			m.done = true
			return m, tea.Quit
		case 2: // Customize
			if m.configOverrides == nil {
				m.configOverrides = defaultOverrides()
			}
			m.phase = 4
			m.cursor = 0
			m.configCursor = 0
		}

	case 4: // Edit config
		if m.cursor >= len(configFields) {
			// "Done" selected
			m.done = true
			return m, tea.Quit
		}
		f := configFields[m.cursor]
		if f.kind == "bool" {
			m.configOverrides.toggleBool(m.cursor)
		} else {
			m.editingConfig = true
			m.editBuffer = m.configOverrides.getValue(m.cursor)
		}
	}

	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m setupModel) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder

	// ── Title block (centered) ────────────────────────────────────
	titleRendered := titleStyle.Render(asciiTitle)
	titleBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, titleRendered)
	b.WriteString(titleBlock)
	b.WriteByte('\n')

	subtitleRendered := subtitleStyle.Render("GO AGENT")
	subtitleBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, subtitleRendered)
	b.WriteString(subtitleBlock)
	b.WriteString("\n\n")

	// ── Menu panel ────────────────────────────────────────────────
	menuContent := m.renderMenu()
	menuPanel := menuPanelStyle.Render(menuContent)

	// ── Layout: torus left, menu right (or menu only if narrow) ──
	narrow := m.width < 80
	if narrow {
		// Center the menu panel only
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, menuPanel)
		b.WriteString(centered)
	} else {
		// Color the torus frame
		coloredTorus := colorTorus(m.torusFrame)

		torusPanel := lipgloss.NewStyle().
			Width(44).
			Render(coloredTorus)

		joined := lipgloss.JoinHorizontal(lipgloss.Top, torusPanel, "  ", menuPanel)
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, joined)
		b.WriteString(centered)
	}

	b.WriteString("\n\n")

	// ── Footer hints ──────────────────────────────────────────────
	var hint string
	if m.inputMode {
		hint = "enter: confirm  |  esc: cancel  |  ctrl+c: quit"
	} else {
		phaseHints := []string{
			"j/k or arrows: navigate  |  enter: select  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k: navigate  |  enter: toggle/edit  |  esc: back  |  q: quit",
		}
		if m.phase < len(phaseHints) {
			hint = phaseHints[m.phase]
		}
	}
	footer := footerStyle.Render(hint)
	footerBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer)
	b.WriteString(footerBlock)

	return b.String()
}

func (m setupModel) renderMenu() string {
	var b strings.Builder

	switch m.phase {
	case 0:
		b.WriteString(menuHeaderStyle.Render("Setup"))
		b.WriteByte('\n')

		items := []string{
			"Use defaults (OpenRouter / hunter-alpha)",
			"Use existing config",
			"Choose provider & model",
		}
		for i, item := range items {
			if i == m.cursor {
				b.WriteString(menuSelectedStyle.Render("> " + item))
			} else {
				b.WriteString(menuItemStyle.Render("  " + item))
			}
			b.WriteByte('\n')
		}

	case 1:
		b.WriteString(menuHeaderStyle.Render("Select Provider"))
		b.WriteByte('\n')

		for i, c := range m.choices {
			label := c.Name
			if i == m.cursor {
				b.WriteString(menuSelectedStyle.Render("> " + label))
			} else {
				b.WriteString(menuItemStyle.Render("  " + label))
			}
			b.WriteByte('\n')
		}

	case 2:
		b.WriteString(menuHeaderStyle.Render("Anthropic Model"))
		b.WriteByte('\n')

		for i, am := range anthropicModels {
			if i == m.cursor {
				b.WriteString(menuSelectedStyle.Render("> " + am.Name))
			} else {
				b.WriteString(menuItemStyle.Render("  " + am.Name))
			}
			b.WriteByte('\n')
		}

	case 3:
		b.WriteString(menuHeaderStyle.Render("Configuration"))
		b.WriteByte('\n')

		items := []string{
			"Use defaults",
			"Use existing config",
			"Customize settings",
		}
		for i, item := range items {
			if i == m.cursor {
				b.WriteString(menuSelectedStyle.Render("> " + item))
			} else {
				b.WriteString(menuItemStyle.Render("  " + item))
			}
			b.WriteByte('\n')
		}

	case 4:
		b.WriteString(menuHeaderStyle.Render("Settings"))
		b.WriteByte('\n')

		for i, f := range configFields {
			val := m.configOverrides.getValue(i)
			var line string
			if f.kind == "bool" {
				indicator := "○"
				if val == "true" {
					indicator = "●"
				}
				line = fmt.Sprintf("%s %s", indicator, f.name)
			} else {
				line = fmt.Sprintf("%s: %s", f.name, val)
			}

			if i == m.cursor {
				if m.editingConfig {
					// Show edit mode
					line = fmt.Sprintf("%s: %s_", f.name, m.editBuffer)
					b.WriteString(textInputStyle.Render("> " + line))
				} else {
					b.WriteString(menuSelectedStyle.Render("> " + line))
				}
			} else {
				b.WriteString(menuItemStyle.Render("  " + line))
			}
			b.WriteByte('\n')
		}
		// "Done" item
		doneIdx := len(configFields)
		if m.cursor == doneIdx {
			b.WriteString(menuSelectedStyle.Render("> Done →"))
		} else {
			b.WriteString(menuItemStyle.Render("  Done →"))
		}
		b.WriteByte('\n')
	}

	// ── Text input overlay ────────────────────────────────────────
	if m.inputMode {
		b.WriteByte('\n')
		var label string
		switch {
		case m.phase == 1 && m.inputStep == 0:
			label = "Provider (openrouter/anthropic/nvidia): "
		case m.phase == 1 && m.inputStep == 1:
			label = fmt.Sprintf("Model ID for %s: ", m.provider)
		case m.phase == 2:
			label = "Model ID: "
		}
		b.WriteString(promptLabelStyle.Render(label))
		b.WriteString(textInputStyle.Render(m.textInput))
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Blink(true).
			Render("_"))
	}

	return b.String()
}

// ── Torus rendering (horn torus from horntorus.com) ────────────────────────────

func renderTorus(a, b float64) string {
	const (
		width  = 40
		height = 20
	)

	output := make([][]byte, height)
	zbuf := make([][]float64, height)
	for i := range output {
		output[i] = make([]byte, width)
		zbuf[i] = make([]float64, width)
		for j := range output[i] {
			output[i][j] = ' '
		}
	}

	chars := ".,-~:;=!*#$@"

	for j := 0.0; j < 6.28; j += 0.07 {
		for i := 0.0; i < 6.28; i += 0.02 {
			c := math.Sin(i)
			d := math.Cos(j)
			e := math.Sin(a)
			f := math.Sin(j)
			g := math.Cos(a)
			h := d + 2
			capD := 1.0 / (c*h*e + f*g + 5)
			l := math.Cos(i)
			capM := math.Cos(b)
			n := math.Sin(b)
			t := c*h*g - f*e

			x := int(float64(width)/2 + 15*capD*(l*h*capM-t*n))
			y := int(float64(height)/2 + 8*capD*(l*h*n+t*capM))

			capN := int(8 * ((f*e-c*d*g)*capM - c*d*e - f*g - l*d*n))

			if y >= 0 && y < height && x >= 0 && x < width && capD > zbuf[y][x] {
				zbuf[y][x] = capD
				idx := capN
				if idx < 0 {
					idx = 0
				}
				if idx >= len(chars) {
					idx = len(chars) - 1
				}
				output[y][x] = chars[idx]
			}
		}
	}

	var sb strings.Builder
	for _, row := range output {
		sb.WriteString(string(row))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// colorTorus applies amber-shade lipgloss styles to each character based on
// its luminance bucket in the donut.c character ramp.
func colorTorus(frame string) string {
	var sb strings.Builder
	for _, ch := range frame {
		switch {
		case ch == '\n':
			sb.WriteByte('\n')
		case ch == ' ':
			sb.WriteByte(' ')
		case ch == '.' || ch == ',' || ch == '-':
			sb.WriteString(torusDimStyle.Render(string(ch)))
		case ch == '~' || ch == ':' || ch == ';':
			sb.WriteString(torusMidStyle.Render(string(ch)))
		case ch == '=' || ch == '!' || ch == '*':
			sb.WriteString(torusBrightStyle.Render(string(ch)))
		case ch == '#' || ch == '$' || ch == '@':
			sb.WriteString(torusMaxStyle.Render(string(ch)))
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// ── Public entry point ────────────────────────────────────────────────────────

// RunStartup shows an interactive provider/model selection menu.
// Returns a SetupResult with provider, model, and optional config overrides.
// If skipStartup is true, returns an empty SetupResult.
func RunStartup(skipStartup bool) SetupResult {
	if skipStartup {
		return SetupResult{}
	}

	m := newSetupModel()

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return SetupResult{}
	}

	result := finalModel.(setupModel)
	if !result.done {
		return SetupResult{}
	}

	return SetupResult{
		Provider: result.provider,
		Model:    result.model,
		Config:   result.configOverrides,
	}
}
