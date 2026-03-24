package uib

import "github.com/charmbracelet/lipgloss"

// Theme holds all lipgloss styles for the TUI. It can be swapped at runtime
// to implement alternative colour schemes.
type Theme struct {
	// Glow gradient (bouncing progress bar).
	GlowBright lipgloss.Style
	GlowMed    lipgloss.Style
	GlowDim    lipgloss.Style
	GlowFaint  lipgloss.Style
	GlowFaint2 lipgloss.Style
	GlowTrack  lipgloss.Style

	// Messages.
	User            lipgloss.Style
	AssistantPrefix lipgloss.Style
	Timestamp       lipgloss.Style
	Error           lipgloss.Style
	Cursor          lipgloss.Style
	Prompt          lipgloss.Style
	Dim             lipgloss.Style

	// Header.
	HeaderBar   lipgloss.Style
	HeaderDim   lipgloss.Style
	Separator   lipgloss.Style
	InputBorder lipgloss.Style

	// Status bar.
	Status     lipgloss.Style
	ScrollHint lipgloss.Style

	// CTX progress bar (orange gradient, 12-char mini bar).
	CtxBarFilled lipgloss.Style // Filled portion of CTX bar (#ff4d01).
	CtxBarEmpty  lipgloss.Style // Empty portion of CTX bar (dim track).
	CtxLabel     lipgloss.Style // "CTX: 42%" label next to the bar.

	// Tool cards.
	ToolHeader lipgloss.Style
	ToolDim    lipgloss.Style
	ToolSep    lipgloss.Style
	DiffAdd    lipgloss.Style
	DiffDel    lipgloss.Style

	// Sidebar.
	SidebarBorder lipgloss.Style
	SidebarTitle  lipgloss.Style
	SidebarFile   lipgloss.Style
	SidebarCount  lipgloss.Style

	// Autocomplete / Palette.
	ACNormal   lipgloss.Style
	ACSelected lipgloss.Style

	// Overlay.
	OverlayTitle lipgloss.Style
	OverlayHint  lipgloss.Style
	Keybind      lipgloss.Style

	// Completion indicator.
	Completion lipgloss.Style
	Check      lipgloss.Style
}

// DefaultTheme returns the dark-orange theme matching the existing TUI.
func DefaultTheme() Theme {
	return Theme{
		// Glow gradient.
		GlowBright: lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6600")),
		GlowMed:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")),
		GlowDim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#cc3d00")),
		GlowFaint:  lipgloss.NewStyle().Foreground(lipgloss.Color("#993300")),
		GlowFaint2: lipgloss.NewStyle().Foreground(lipgloss.Color("#662200")),
		GlowTrack:  lipgloss.NewStyle().Foreground(lipgloss.Color("#2b2a2a")),

		// Messages.
		User: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00e5ff")).
			Background(lipgloss.Color("#0a2a2f")).
			Bold(true),
		AssistantPrefix: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff6600")).
			Bold(true),
		Timestamp: lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Cursor:    lipgloss.NewStyle().Reverse(true),
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true),
		Dim:       lipgloss.NewStyle().Foreground(lipgloss.Color("242")),

		// Header.
		HeaderBar: lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("166")).
			Bold(true).
			Padding(0, 1),
		HeaderDim: lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("130")),
		Separator:   lipgloss.NewStyle().Foreground(lipgloss.Color("236")),
		InputBorder: lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")),

		// Status bar.
		Status: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a0a00")).
			Foreground(lipgloss.Color("#ff4d01")),
		ScrollHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true),

		// CTX progress bar.
		CtxBarFilled: lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8c00")).Background(lipgloss.Color("#ff4d01")),
		CtxBarEmpty:  lipgloss.NewStyle().Foreground(lipgloss.Color("#331100")),
		CtxLabel:     lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8c00")),

		// Tool cards.
		ToolHeader: lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true),
		ToolDim:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		ToolSep:    lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		DiffAdd:    lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		DiffDel:    lipgloss.NewStyle().Foreground(lipgloss.Color("9")),

		// Sidebar.
		SidebarBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("130")),
		SidebarTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true),
		SidebarFile:  lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		SidebarCount: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),

		// Autocomplete / Palette.
		ACNormal: lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		ACSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("214")),

		// Overlay.
		OverlayTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("166")).Bold(true),
		OverlayHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true),
		Keybind: lipgloss.NewStyle().Foreground(lipgloss.Color("166")),

		// Completion indicator.
		Completion: lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")),
		Check:      lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc66")),
	}
}
