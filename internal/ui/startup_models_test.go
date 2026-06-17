package ui

import (
	"strings"
	"testing"
)

// TestModelsForAuthor verifies that native-provider model lists derived from the
// OpenRouter universal registry are filtered by author prefix, stripped to native
// IDs, carry the registry's real context/maxTokens, skip OpenRouter-only routing
// variants (":free" etc.) and the FREE MODELS bucket, dedupe, and end with the
// "Custom model ID" escape hatch.
func TestModelsForAuthor(t *testing.T) {
	t.Parallel()
	cats := []ModelCategory{
		{Name: "FREE MODELS", Models: []ModelChoice{
			{Name: "claude free route", ID: "anthropic/claude-free:free", ContextWindow: 100, MaxTokens: 10},
			{Name: "Custom model ID", ID: ""},
		}},
		{Name: "anthropic", Models: []ModelChoice{
			{Name: "Anthropic: Claude Opus 4.8", ID: "anthropic/claude-opus-4-8", ContextWindow: 200000, MaxTokens: 64000},
			{Name: "Anthropic: Claude Sonnet (free route)", ID: "anthropic/claude-sonnet-4-6:free", ContextWindow: 200000, MaxTokens: 64000},
			{Name: "Custom model ID", ID: ""},
		}},
		{Name: "x-ai", Models: []ModelChoice{
			{Name: "Grok 4", ID: "x-ai/grok-4", ContextWindow: 131072, MaxTokens: 32768},
			{Name: "Grok 4 (dup)", ID: "x-ai/grok-4", ContextWindow: 131072, MaxTokens: 32768},
			{Name: "Custom model ID", ID: ""},
		}},
	}

	got := modelsForAuthor(cats, "anthropic")
	// opus (real) + trailing custom-id. Free-bucket entry and the ":free" routing variant are skipped.
	if len(got) != 2 {
		t.Fatalf("anthropic: got %d models, want 2: %+v", len(got), got)
	}
	if got[0].ID != "claude-opus-4-8" {
		t.Errorf("native ID not stripped: got %q, want claude-opus-4-8", got[0].ID)
	}
	if got[0].ContextWindow != 200000 || got[0].MaxTokens != 64000 {
		t.Errorf("specs not carried from registry: ctx=%d max=%d, want 200000/64000", got[0].ContextWindow, got[0].MaxTokens)
	}
	for _, mc := range got {
		if strings.Contains(mc.ID, ":") {
			t.Errorf("OpenRouter routing variant leaked into native list: %q", mc.ID)
		}
		if strings.Contains(mc.ID, "/") {
			t.Errorf("author prefix not stripped: %q", mc.ID)
		}
	}
	if got[len(got)-1].ID != "" {
		t.Errorf("expected trailing Custom model ID entry, got %+v", got[len(got)-1])
	}

	// Dedup within an author.
	if xai := modelsForAuthor(cats, "x-ai"); len(xai) != 2 {
		t.Fatalf("x-ai dedup: got %d, want 2 (grok-4 + custom): %+v", len(xai), xai)
	}

	// Absent provider → nil, so the caller keeps its static fallback list.
	if none := modelsForAuthor(cats, "deepseek"); none != nil {
		t.Errorf("absent provider should return nil (keep static fallback), got %+v", none)
	}
}
