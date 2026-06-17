package uib

import (
	"strings"
	"testing"
)

// TestModelsForAuthor verifies native-provider lists derived from the OpenRouter
// universal registry are filtered by author prefix, stripped to native IDs, carry
// real context/maxTokens, skip routing variants and the FREE bucket, dedupe, and
// end with the "Custom model ID" escape hatch. Mirror of the ui-package test.
func TestModelsForAuthor(t *testing.T) {
	t.Parallel()
	cats := []startupModelCategory{
		{Name: "FREE MODELS", Models: []startupModelChoice{
			{Name: "claude free route", ID: "anthropic/claude-free:free", ContextWindow: 100, MaxTokens: 10},
			{Name: "Custom model ID", ID: ""},
		}},
		{Name: "anthropic", Models: []startupModelChoice{
			{Name: "Anthropic: Claude Opus 4.8", ID: "anthropic/claude-opus-4-8", ContextWindow: 200000, MaxTokens: 64000},
			{Name: "Anthropic: Claude Sonnet (free route)", ID: "anthropic/claude-sonnet-4-6:free", ContextWindow: 200000, MaxTokens: 64000},
			{Name: "Custom model ID", ID: ""},
		}},
		{Name: "x-ai", Models: []startupModelChoice{
			{Name: "Grok 4", ID: "x-ai/grok-4", ContextWindow: 131072, MaxTokens: 32768},
			{Name: "Grok 4 (dup)", ID: "x-ai/grok-4", ContextWindow: 131072, MaxTokens: 32768},
			{Name: "Custom model ID", ID: ""},
		}},
	}

	got := modelsForAuthor(cats, "anthropic")
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

	if xai := modelsForAuthor(cats, "x-ai"); len(xai) != 2 {
		t.Fatalf("x-ai dedup: got %d, want 2 (grok-4 + custom): %+v", len(xai), xai)
	}

	if none := modelsForAuthor(cats, "deepseek"); none != nil {
		t.Errorf("absent provider should return nil (keep static fallback), got %+v", none)
	}
}
