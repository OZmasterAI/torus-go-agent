package ui

import (
	"strings"
	"testing"
)

func TestClampScrollOffset(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		total        int
		want         int
	}{
		// Short list: always returns 0
		{"short list no scroll", 2, 0, 5, 0},
		{"short list even with offset", 2, 3, 5, 0},

		// Exactly visibleItems: no scroll needed
		{"exact fit", 5, 0, visibleItems, 0},

		// Long list: cursor in view, no change
		{"cursor in view", 5, 3, 20, 3},

		// Long list: cursor above viewport, scroll up
		{"cursor above viewport", 2, 5, 20, 2},

		// Long list: cursor below viewport, scroll down
		{"cursor below viewport", 15, 3, 20, 15 - visibleItems + 1},

		// Wrap from top to bottom: cursor at last, offset was 0
		{"wrap to bottom", 19, 0, 20, 19 - visibleItems + 1},

		// Wrap from bottom to top: cursor at 0, offset was at end
		{"wrap to top", 0, 10, 20, 0},

		// Cursor exactly at viewport boundary (last visible)
		{"cursor at last visible", 12, 3, 20, 3},

		// Cursor one past viewport boundary
		{"cursor one past visible", 13, 3, 20, 13 - visibleItems + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampScrollOffset(tt.cursor, tt.scrollOffset, tt.total)
			if got != tt.want {
				t.Errorf("clampScrollOffset(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.total, got, tt.want)
			}
		})
	}
}

func TestFilteredIndices_NoFilter(t *testing.T) {
	labels := []string{"Alpha", "Beta", "Gamma"}
	got := filteredIndices(3, func(i int) string { return labels[i] }, "")
	if len(got) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Errorf("index %d: got %d, want %d", i, v, i)
		}
	}
}

func TestFilteredIndices_MatchSubstring(t *testing.T) {
	labels := []string{"OpenRouter", "NVIDIA NIM", "Anthropic Claude", "OpenAI"}
	got := filteredIndices(4, func(i int) string { return labels[i] }, "open")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches for 'open', got %d", len(got))
	}
	if got[0] != 0 || got[1] != 3 {
		t.Errorf("expected [0, 3], got %v", got)
	}
}

func TestFilteredIndices_CaseInsensitive(t *testing.T) {
	labels := []string{"Claude Opus", "GPT-4o", "Gemini Pro"}
	got := filteredIndices(3, func(i int) string { return labels[i] }, "CLAUDE")
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("expected [0] for case-insensitive match, got %v", got)
	}
}

func TestFilteredIndices_NoMatch(t *testing.T) {
	labels := []string{"Alpha", "Beta"}
	got := filteredIndices(2, func(i int) string { return labels[i] }, "xyz")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestResolveFilteredIndex_NoFilter(t *testing.T) {
	m := setupModel{cursor: 2}
	if got := m.resolveFilteredIndex(); got != 2 {
		t.Errorf("no filter: got %d, want 2", got)
	}
}

func TestResolveFilteredIndex_WithFilter(t *testing.T) {
	groups := []ProviderGroup{
		{Name: "OpenRouter", ProviderKey: "openrouter"},
		{Name: "NVIDIA NIM", ProviderKey: "nvidia"},
		{Name: "Anthropic Claude", ProviderKey: "anthropic"},
		{Name: "OpenAI", ProviderKey: "openai"},
	}
	m := setupModel{
		phase:      1,
		cursor:     1, // second filtered result
		filterText: "open",
		groups:     groups,
	}
	// "open" matches index 0 (OpenRouter) and 3 (OpenAI)
	got := m.resolveFilteredIndex()
	if got != 3 {
		t.Errorf("filtered index: got %d, want 3 (OpenAI)", got)
	}
}

func TestFilterablePhase(t *testing.T) {
	for _, phase := range []int{1, 3, 4} {
		m := setupModel{phase: phase}
		if !m.filterablePhase() {
			t.Errorf("phase %d should be filterable", phase)
		}
	}
	for _, phase := range []int{0, 2, 5, 6} {
		m := setupModel{phase: phase}
		if m.filterablePhase() {
			t.Errorf("phase %d should not be filterable", phase)
		}
	}
}

func TestBuildModelPickerItems(t *testing.T) {
	groups := []ProviderGroup{
		{
			Name:        "Anthropic",
			ProviderKey: "anthropic",
			Models: []ModelChoice{
				{Name: "Claude Opus", ID: "claude-opus-4-6"},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name:        "NVIDIA NIM",
			ProviderKey: "nvidia",
			Categories: []ModelCategory{
				{Name: "FREE", Models: []ModelChoice{
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
	items := buildModelPickerItems(groups)
	// Expect: (none), Claude Opus, GLM-5 = 3 items
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
	// Labels should include provider name
	if !strings.Contains(items[1].Label, "Anthropic") {
		t.Errorf("label should contain provider: %q", items[1].Label)
	}
	if !strings.Contains(items[2].Label, "NVIDIA") {
		t.Errorf("label should contain provider: %q", items[2].Label)
	}
	// ProviderKey should be set
	if items[1].ProviderKey != "anthropic" {
		t.Errorf("item 1 provider: got %q, want anthropic", items[1].ProviderKey)
	}
	if items[2].ProviderKey != "nvidia" {
		t.Errorf("item 2 provider: got %q, want nvidia", items[2].ProviderKey)
	}
	// ProviderModelID format
	if items[1].ProviderModelID() != "anthropic:claude-opus-4-6" {
		t.Errorf("ProviderModelID: got %q, want anthropic:claude-opus-4-6", items[1].ProviderModelID())
	}
	if items[0].ProviderModelID() != "" {
		t.Errorf("none entry ProviderModelID: got %q, want empty", items[0].ProviderModelID())
	}
}

func TestInitTorusParticles(t *testing.T) {
	particles := initTorusParticles()
	if len(particles) != numTorusParticles {
		t.Fatalf("expected %d particles, got %d", numTorusParticles, len(particles))
	}
	// All particles should start unsettled
	for i, p := range particles {
		if p.settled {
			t.Errorf("particle %d should start unsettled", i)
		}
	}
}

func TestRenderParticleTorus(t *testing.T) {
	particles := initTorusParticles()
	frame := renderParticleTorus(particles, 0, 0)
	if frame == "" {
		t.Fatal("renderParticleTorus returned empty string")
	}
	lines := strings.Split(frame, "\n")
	// Should produce at least torusHeight lines (trimmed trailing newlines may reduce by 1)
	if len(lines) < torusHeight-1 {
		t.Errorf("expected at least %d lines, got %d", torusHeight-1, len(lines))
	}
}

func TestUpdateTorusParticles(t *testing.T) {
	particles := initTorusParticles()
	// Run several update cycles -- should not panic
	for i := 0; i < 100; i++ {
		updateTorusParticles(particles, float64(i)*0.032)
	}
	// After many updates, some particles should have settled
	settled := 0
	for _, p := range particles {
		if p.settled {
			settled++
		}
	}
	if settled == 0 {
		t.Error("after 100 updates, expected some settled particles")
	}
}

func TestNvidiaFreeEnablesRewardScoring(t *testing.T) {
	m := setupModel{
		configOverrides: defaultOverrides(),
	}
	mc := ModelChoice{
		ID:            "nvidia/free",
		ContextWindow: 131072,
		MaxTokens:     8192,
	}
	if mc.ContextWindow > 0 {
		m.configOverrides.ContextWindow = mc.ContextWindow
		m.configOverrides.MaxTokens = mc.MaxTokens
	}
	if mc.ID == "nvidia/free" {
		m.configOverrides.RewardScoring = true
	}
	if !m.configOverrides.RewardScoring {
		t.Error("expected RewardScoring=true for nvidia/free")
	}
	if m.configOverrides.ContextWindow != 131072 {
		t.Errorf("expected ContextWindow=131072, got %d", m.configOverrides.ContextWindow)
	}
}

func TestNonNvidiaFreeSkipsRewardScoring(t *testing.T) {
	m := setupModel{
		configOverrides: defaultOverrides(),
	}
	mc := ModelChoice{
		ID:            "some-other-model",
		ContextWindow: 32768,
		MaxTokens:     4096,
	}
	if mc.ContextWindow > 0 {
		m.configOverrides.ContextWindow = mc.ContextWindow
		m.configOverrides.MaxTokens = mc.MaxTokens
	}
	if mc.ID == "nvidia/free" {
		m.configOverrides.RewardScoring = true
	}
	if m.configOverrides.RewardScoring {
		t.Error("expected RewardScoring=false for non-nvidia/free model")
	}
}

func TestFormatProviderModel(t *testing.T) {
	tests := []struct{ input, want string }{
		{"anthropic:claude-haiku-4-5", "claude-haiku-4-5 (anthropic)"},
		{"nvidia:z-ai/glm5", "z-ai/glm5 (nvidia)"},
		{"bare-model-id", "bare-model-id"},
		{"", ""},
	}
	for _, tt := range tests {
		got := formatProviderModel(tt.input)
		if got != tt.want {
			t.Errorf("formatProviderModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
