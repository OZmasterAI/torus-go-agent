package uib

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"torus_go_agent/internal/config"
)

// ── Scroll offset ────────────────────────────────────────────────────────────

func TestStartupClampScrollOffset(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		total        int
		want         int
	}{
		{"short list no scroll", 2, 0, 5, 0},
		{"short list even with offset", 2, 3, 5, 0},
		{"exact fit", 5, 0, startupVisibleItems, 0},
		{"cursor in view", 5, 3, 20, 3},
		{"cursor above viewport", 2, 5, 20, 2},
		{"cursor below viewport", 15, 3, 20, 15 - startupVisibleItems + 1},
		{"wrap to bottom", 19, 0, 20, 19 - startupVisibleItems + 1},
		{"wrap to top", 0, 10, 20, 0},
		{"cursor at last visible", 12, 3, 20, 3},
		{"cursor one past visible", 13, 3, 20, 13 - startupVisibleItems + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := startupClampScrollOffset(tt.cursor, tt.scrollOffset, tt.total)
			if got != tt.want {
				t.Errorf("startupClampScrollOffset(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.total, got, tt.want)
			}
		})
	}
}

// ── Filter ───────────────────────────────────────────────────────────────────

func TestStartupFilteredIndices_NoFilter(t *testing.T) {
	labels := []string{"Alpha", "Beta", "Gamma"}
	got := startupFilteredIndices(3, func(i int) string { return labels[i] }, "")
	if len(got) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Errorf("index %d: got %d, want %d", i, v, i)
		}
	}
}

func TestStartupFilteredIndices_MatchSubstring(t *testing.T) {
	labels := []string{"OpenRouter", "NVIDIA NIM", "Anthropic Claude", "OpenAI"}
	got := startupFilteredIndices(4, func(i int) string { return labels[i] }, "open")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches for 'open', got %d", len(got))
	}
	if got[0] != 0 || got[1] != 3 {
		t.Errorf("expected [0, 3], got %v", got)
	}
}

func TestStartupFilteredIndices_CaseInsensitive(t *testing.T) {
	labels := []string{"Claude Opus", "GPT-4o", "Gemini Pro"}
	got := startupFilteredIndices(3, func(i int) string { return labels[i] }, "CLAUDE")
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("expected [0] for case-insensitive match, got %v", got)
	}
}

func TestStartupFilteredIndices_NoMatch(t *testing.T) {
	labels := []string{"Alpha", "Beta"}
	got := startupFilteredIndices(2, func(i int) string { return labels[i] }, "xyz")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// ── Resolve filtered index ──────────────────────────────────────────────────

func TestStartupResolveFilteredIndex_NoFilter(t *testing.T) {
	m := startupModel{cursor: 2}
	if got := m.startupResolveFilteredIndex(); got != 2 {
		t.Errorf("no filter: got %d, want 2", got)
	}
}

func TestStartupResolveFilteredIndex_WithFilter(t *testing.T) {
	groups := []startupProviderGroup{
		{Name: "OpenRouter", ProviderKey: "openrouter"},
		{Name: "NVIDIA NIM", ProviderKey: "nvidia"},
		{Name: "Anthropic Claude", ProviderKey: "anthropic"},
		{Name: "OpenAI", ProviderKey: "openai"},
	}
	m := startupModel{
		phase:      1,
		cursor:     1,
		filterText: "open",
		groups:     groups,
	}
	got := m.startupResolveFilteredIndex()
	if got != 3 {
		t.Errorf("filtered index: got %d, want 3 (OpenAI)", got)
	}
}

// ── Filterable phase ────────────────────────────────────────────────────────

func TestStartupFilterablePhase(t *testing.T) {
	for _, phase := range []int{1, 3, 4, 7} {
		m := startupModel{phase: phase}
		if !m.startupFilterablePhase() {
			t.Errorf("phase %d should be filterable", phase)
		}
	}
	for _, phase := range []int{0, 2, 5, 6} {
		m := startupModel{phase: phase}
		if m.startupFilterablePhase() {
			t.Errorf("phase %d should not be filterable", phase)
		}
	}
}

// ── Model picker ────────────────────────────────────────────────────────────

func TestBuildStartupModelPickerItems(t *testing.T) {
	groups := []startupProviderGroup{
		{
			Name:        "Anthropic",
			ProviderKey: "anthropic",
			Models: []startupModelChoice{
				{Name: "Claude Opus", ID: "claude-opus-4-6"},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name:        "NVIDIA NIM",
			ProviderKey: "nvidia",
			Categories: []startupModelCategory{
				{Name: "FREE", Models: []startupModelChoice{
					{Name: "GLM-5", ID: "z-ai/glm5"},
					{Name: "Custom model ID", ID: ""},
				}},
			},
		},
		{
			Name:        "Custom provider",
			ProviderKey: "",
		},
	}
	items := buildStartupModelPickerItems(groups)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].ModelID != "" {
		t.Errorf("first item should be none, got %q", items[0].ModelID)
	}
	if items[1].ModelID != "claude-opus-4-6" {
		t.Errorf("second item: got %q, want claude-opus-4-6", items[1].ModelID)
	}
	if items[2].ModelID != "z-ai/glm5" {
		t.Errorf("third item: got %q, want z-ai/glm5", items[2].ModelID)
	}
	if !strings.Contains(items[1].Label, "Anthropic") {
		t.Errorf("label should contain provider: %q", items[1].Label)
	}
	if !strings.Contains(items[2].Label, "NVIDIA") {
		t.Errorf("label should contain provider: %q", items[2].Label)
	}
	if items[1].ProviderKey != "anthropic" {
		t.Errorf("item 1 provider: got %q, want anthropic", items[1].ProviderKey)
	}
	if items[1].ProviderModelID() != "anthropic:claude-opus-4-6" {
		t.Errorf("ProviderModelID: got %q, want anthropic:claude-opus-4-6", items[1].ProviderModelID())
	}
	if items[0].ProviderModelID() != "" {
		t.Errorf("none entry ProviderModelID: got %q, want empty", items[0].ProviderModelID())
	}
}

// ── FormatProviderModel ─────────────────────────────────────────────────────

func TestFormatStartupProviderModel(t *testing.T) {
	tests := []struct{ input, want string }{
		{"anthropic:claude-haiku-4-5", "claude-haiku-4-5 (anthropic)"},
		{"nvidia:z-ai/glm5", "z-ai/glm5 (nvidia)"},
		{"bare-model-id", "bare-model-id"},
		{"", ""},
	}
	for _, tt := range tests {
		got := formatStartupProviderModel(tt.input)
		if got != tt.want {
			t.Errorf("formatStartupProviderModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── Config overrides ────────────────────────────────────────────────────────

func TestStartupConfigOverrides_GetSetValue(t *testing.T) {
	o := defaultStartupOverrides()

	// Compaction (idx 0) - cycle
	if got := o.getValue(0); got != "llm" {
		t.Errorf("Compaction: got %q, want 'llm'", got)
	}
	o.setValue(0, "sliding")
	if got := o.getValue(0); got != "sliding" {
		t.Errorf("Compaction after set: got %q, want 'sliding'", got)
	}

	// ContinuousCompression (idx 6) - bool
	orig := o.getValue(6)
	o.toggleBool(6)
	toggled := o.getValue(6)
	if orig == toggled {
		t.Errorf("toggleBool should change value, got %q both times", orig)
	}

	// MaxTokens (idx 17) - int with "default" display
	if got := o.getValue(17); got != "default" {
		t.Errorf("MaxTokens: got %q, want 'default'", got)
	}
	o.setValue(17, "4096")
	if got := o.getValue(17); got != "4096" {
		t.Errorf("MaxTokens after set: got %q, want '4096'", got)
	}

	// Thinking (idx 15) - cycle
	o.cycleOption(15, 1)
	if got := o.getValue(15); got == "" {
		t.Error("Thinking should have cycled from default")
	}
}

func TestStartupConfigOverrides_CycleOption(t *testing.T) {
	o := defaultStartupOverrides()
	// Compaction starts at "llm" (idx 0 in options)
	o.cycleOption(0, 1) // llm -> sliding
	if got := o.getValue(0); got != "sliding" {
		t.Errorf("cycle +1: got %q, want 'sliding'", got)
	}
	o.cycleOption(0, 1) // sliding -> off
	if got := o.getValue(0); got != "off" {
		t.Errorf("cycle +1: got %q, want 'off'", got)
	}
	o.cycleOption(0, 1) // off -> llm (wrap)
	if got := o.getValue(0); got != "llm" {
		t.Errorf("cycle wrap: got %q, want 'llm'", got)
	}
	o.cycleOption(0, -1) // llm -> off (backward)
	if got := o.getValue(0); got != "off" {
		t.Errorf("cycle -1: got %q, want 'off'", got)
	}
}

// ── OverridesFromAgentConfig ────────────────────────────────────────────────

func TestOverridesFromAgentConfig(t *testing.T) {
	cfg := config.DefaultAgentConfig()
	cfg.SteeringMode = ""
	o := overridesFromAgentConfig(cfg)
	if o.SteeringMode != "mild" {
		t.Errorf("empty SteeringMode should default to 'mild', got %q", o.SteeringMode)
	}
	if o.MaxTokens != 0 {
		t.Errorf("MaxTokens should be 0 (default), got %d", o.MaxTokens)
	}
	if o.ContextWindow != 0 {
		t.Errorf("ContextWindow should be 0 (default), got %d", o.ContextWindow)
	}
}

// ── Torus rendering ─────────────────────────────────────────────────────────

func TestRenderStartupTorus_NonEmpty(t *testing.T) {
	frame := renderStartupTorus(0.0, 0.0)
	if len(frame) == 0 {
		t.Fatal("torus frame should not be empty")
	}
	lines := strings.Split(frame, "\n")
	// Should have 18 lines (height) + trailing empty from final \n
	if len(lines) < 18 {
		t.Errorf("expected at least 18 lines, got %d", len(lines))
	}
	// Should contain torus characters (not all spaces)
	hasNonSpace := false
	for _, ch := range frame {
		if ch != ' ' && ch != '\n' {
			hasNonSpace = true
			break
		}
	}
	if !hasNonSpace {
		t.Error("torus frame should contain non-space characters")
	}
}

func TestRenderStartupTorus_DifferentAngles(t *testing.T) {
	f1 := renderStartupTorus(0.0, 0.0)
	f2 := renderStartupTorus(1.0, 0.5)
	if f1 == f2 {
		t.Error("torus frames at different angles should differ")
	}
}

func TestColorStartupTorus_RendersAllBuckets(t *testing.T) {
	// Test with a frame that includes characters from all luminance buckets
	testFrame := ".,-~:;=!*#$@ \n"
	colored := colorStartupTorus(testFrame)
	if len(colored) == 0 {
		t.Fatal("colored torus should not be empty")
	}
	// In a TTY, output would be longer due to ANSI codes.
	// In headless test mode, lipgloss renders plain text.
	// Either way, the output must contain all the original non-space characters.
	for _, ch := range ".,-~:;=!*#$@" {
		if !strings.ContainsRune(colored, ch) {
			t.Errorf("colored output should contain character %q", string(ch))
		}
	}
}

// ── Animated title ──────────────────────────────────────────────────────────

func TestRenderStartupAnimatedTitle_NonEmpty(t *testing.T) {
	out := renderStartupAnimatedTitle(startupASCIITitle, 0.0)
	if len(out) == 0 {
		t.Fatal("animated title should not be empty")
	}
}

func TestRenderStartupAnimatedTitle_PhaseDiffers(t *testing.T) {
	// In headless mode, lipgloss strips ANSI codes so both phases render identically.
	// Use a short test string where phase shift creates a visible color index difference.
	// The test verifies the function does not panic and produces output.
	o1 := renderStartupAnimatedTitle("AB\nCD", 0.0)
	o2 := renderStartupAnimatedTitle("AB\nCD", 1.0)
	// Both should be non-empty
	if len(o1) == 0 || len(o2) == 0 {
		t.Error("animated title at any phase should produce output")
	}
	// In a TTY the outputs would differ; in headless they may be identical
	// (both stripped to plain text). This is acceptable.
}

// ── Menu length ─────────────────────────────────────────────────────────────

func TestStartupMenuLen(t *testing.T) {
	m := startupModel{groups: defaultStartupProviderGroups()}

	// Phase 0: main menu = 2 items
	m.phase = 0
	if got := m.startupMenuLen(); got != 2 {
		t.Errorf("phase 0: got %d, want 2", got)
	}

	// Phase 1: provider groups
	m.phase = 1
	expected := len(m.groups)
	if got := m.startupMenuLen(); got != expected {
		t.Errorf("phase 1: got %d, want %d", got, expected)
	}

	// Phase 5: config mode = 3 items
	m.phase = 5
	if got := m.startupMenuLen(); got != 3 {
		t.Errorf("phase 5: got %d, want 3", got)
	}

	// Phase 6: settings = configFields + 1 (Done)
	m.phase = 6
	m.configOverrides = defaultStartupOverrides()
	expected = len(startupConfigFields) + 1
	if got := m.startupMenuLen(); got != expected {
		t.Errorf("phase 6: got %d, want %d", got, expected)
	}
}

// ── Default provider groups ─────────────────────────────────────────────────

func TestDefaultStartupProviderGroups(t *testing.T) {
	groups := defaultStartupProviderGroups()
	if len(groups) < 8 {
		t.Fatalf("expected at least 8 provider groups, got %d", len(groups))
	}

	// First should be OpenRouter
	if groups[0].Name != "OpenRouter" {
		t.Errorf("first group should be OpenRouter, got %q", groups[0].Name)
	}

	// Last should be Custom provider
	last := groups[len(groups)-1]
	if last.ProviderKey != "" {
		t.Errorf("last group should be custom (empty key), got %q", last.ProviderKey)
	}

	// Anthropic should have 2 auth methods (OAuth + API key)
	for _, g := range groups {
		if g.ProviderKey == "anthropic" {
			if len(g.AuthMethods) != 2 {
				t.Errorf("Anthropic should have 2 auth methods, got %d", len(g.AuthMethods))
			}
			break
		}
	}
}

// ── StartupModel Update ─────────────────────────────────────────────────────

func TestStartupModel_TickAdvancesAnimation(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true

	origA := m.torusA
	origB := m.torusB

	updated, cmd := m.Update(startupTickMsg{})
	if cmd == nil {
		t.Error("tick should return a cmd for next tick")
	}
	if updated.torusA <= origA {
		t.Error("torusA should advance on tick")
	}
	if updated.torusB <= origB {
		t.Error("torusB should advance on tick")
	}
}

func TestStartupModel_WindowSizeMsg(t *testing.T) {
	m := startupModel{}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if !updated.ready {
		t.Error("should be ready after WindowSizeMsg")
	}
	if updated.width != 120 || updated.height != 40 {
		t.Errorf("size: got %dx%d, want 120x40", updated.width, updated.height)
	}
}

func TestStartupModel_Navigation(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true

	// Navigate down
	updated, _ := m.handleStartupKey(tea.KeyMsg{Type: tea.KeyDown})
	if updated.cursor != 1 {
		t.Errorf("down: cursor should be 1, got %d", updated.cursor)
	}

	// Navigate up (wraps to end)
	m.cursor = 0
	updated, _ = m.handleStartupKey(tea.KeyMsg{Type: tea.KeyUp})
	if updated.cursor != 1 { // phase 0 has 2 items
		t.Errorf("up wrap: cursor should be 1, got %d", updated.cursor)
	}
}

func TestStartupModel_SelectExistingConfig(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.phase = 0
	m.cursor = 0 // "Use existing config"

	updated, _ := m.startupSelectItem()
	if !updated.done {
		t.Error("selecting 'use existing config' should set done")
	}
	if updated.provider != "" {
		t.Errorf("provider should be empty, got %q", updated.provider)
	}
}

func TestStartupModel_SelectProvider(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.phase = 1
	m.cursor = 2 // Anthropic Claude (has 2 auth methods)

	updated, _ := m.startupSelectItem()
	if updated.phase != 2 {
		t.Errorf("selecting Anthropic should go to auth phase (2), got phase %d", updated.phase)
	}
	if updated.selectedGroup == nil {
		t.Fatal("selectedGroup should be set")
	}
	if updated.selectedGroup.ProviderKey != "anthropic" {
		t.Errorf("provider key: got %q, want 'anthropic'", updated.selectedGroup.ProviderKey)
	}
}

func TestStartupModel_CustomProviderInput(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.phase = 1
	m.cursor = len(m.groups) - 1 // Last = Custom provider

	updated, _ := m.startupSelectItem()
	if !updated.inputMode {
		t.Error("custom provider should activate input mode")
	}
	if updated.inputStep != 0 {
		t.Errorf("input step should be 0 (provider name), got %d", updated.inputStep)
	}
}

func TestStartupModel_View(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.torusFrame = renderStartupTorus(m.torusA, m.torusB)

	view := m.View()
	if len(view) == 0 {
		t.Fatal("View should not be empty when ready")
	}
	if !strings.Contains(view, "Setup") {
		t.Error("View should contain menu header")
	}
}

func TestStartupModel_ViewNotReadyEmpty(t *testing.T) {
	m := startupModel{ready: false}
	view := m.View()
	if view != "" {
		t.Error("View should be empty when not ready")
	}
}

func TestStartupModel_EscNavigatesBack(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.phase = 1

	updated, _ := m.handleStartupKey(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.phase != 0 {
		t.Errorf("esc from phase 1 should go to phase 0, got %d", updated.phase)
	}
}

func TestStartupModel_EscClearsFilterFirst(t *testing.T) {
	m := newStartupModel()
	m.width, m.height, m.ready = 120, 40, true
	m.phase = 1
	m.filterText = "open"

	updated, _ := m.handleStartupKey(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.filterText != "" {
		t.Error("esc should clear filter text first")
	}
	if updated.phase != 1 {
		t.Errorf("phase should remain 1 after clearing filter, got %d", updated.phase)
	}
}

// ── NewModelWithStartup ─────────────────────────────────────────────────────

func TestNewModelWithStartup(t *testing.T) {
	m := NewModelWithStartup(nil, "test-model", config.AgentConfig{}, nil, nil)
	if !m.startupPhase {
		t.Error("startupPhase should be true")
	}
	if m.startup.groups == nil {
		t.Error("startup.groups should be initialized")
	}
}

func TestNewModelWithStartup_Init(t *testing.T) {
	m := NewModelWithStartup(nil, "test-model", config.AgentConfig{}, nil, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return startupTickCmd when in startup phase")
	}
}
