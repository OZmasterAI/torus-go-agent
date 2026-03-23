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
}
