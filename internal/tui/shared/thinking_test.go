package shared

import "testing"

func TestAppendDelta(t *testing.T) {
	var tm ThinkingModel
	tm.AppendDelta("hello ")
	tm.AppendDelta("world")
	if tm.Buf != "hello world" {
		t.Errorf("Buf = %q, want %q", tm.Buf, "hello world")
	}
	if !tm.HasPending() {
		t.Error("HasPending should be true")
	}
}

func TestCollapse(t *testing.T) {
	var tm ThinkingModel
	tm.AppendDelta("thinking text")
	tm.Collapse()
	if tm.Buf != "" {
		t.Errorf("Buf should be empty after Collapse, got %q", tm.Buf)
	}
	if len(tm.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(tm.Cards))
	}
	if tm.Cards[0].Text != "thinking text" {
		t.Errorf("card text = %q, want %q", tm.Cards[0].Text, "thinking text")
	}
}

func TestCollapseEmpty(t *testing.T) {
	var tm ThinkingModel
	tm.Collapse()
	if len(tm.Cards) != 0 {
		t.Errorf("Collapse with empty buf should not create a card")
	}
}

func TestToggle(t *testing.T) {
	var tm ThinkingModel
	if tm.Verbosity != VerbosityCompact {
		t.Errorf("Verbosity should default to compact (0), got %d", tm.Verbosity)
	}
	tm.Toggle()
	if tm.Verbosity != VerbosityVerbose {
		t.Errorf("Verbosity should be verbose (1) after first Toggle, got %d", tm.Verbosity)
	}
	tm.Toggle()
	if tm.Verbosity != VerbosityFull {
		t.Errorf("Verbosity should be full (2) after second Toggle, got %d", tm.Verbosity)
	}
	tm.Toggle()
	if tm.Verbosity != VerbosityCompact {
		t.Errorf("Verbosity should be compact (0) after third Toggle, got %d", tm.Verbosity)
	}
}

func TestVerbosityLabel(t *testing.T) {
	var tm ThinkingModel
	if tm.VerbosityLabel() != "compact" {
		t.Errorf("label should be 'compact', got %q", tm.VerbosityLabel())
	}
	tm.Verbosity = VerbosityVerbose
	if tm.VerbosityLabel() != "verbose" {
		t.Errorf("label should be 'verbose', got %q", tm.VerbosityLabel())
	}
	tm.Verbosity = VerbosityFull
	if tm.VerbosityLabel() != "full" {
		t.Errorf("label should be 'full', got %q", tm.VerbosityLabel())
	}
}

func TestRenderCardCollapsed(t *testing.T) {
	var tm ThinkingModel
	card := ThinkingCard{Text: "some reasoning here"}
	out := tm.RenderCard(card, 80)
	if out == "" {
		t.Error("collapsed render should not be empty")
	}
	if !contains(out, "\u25b6") {
		t.Error("collapsed render should contain \u25b6")
	}
	if !contains(out, "19 chars") {
		t.Errorf("collapsed render should show char count, got: %s", out)
	}
}

func TestRenderCardExpanded(t *testing.T) {
	var tm ThinkingModel
	tm.Verbosity = VerbosityVerbose
	card := ThinkingCard{Text: "detailed reasoning"}
	out := tm.RenderCard(card, 80)
	if !contains(out, "\u25bc") {
		t.Error("expanded render should contain \u25bc")
	}
	if !contains(out, "detailed reasoning") {
		t.Error("expanded render should contain the thinking text")
	}
}

func TestRenderInlineCollapsed(t *testing.T) {
	var tm ThinkingModel
	card := ThinkingCard{Text: "some reasoning"}
	out := tm.RenderInline(card)
	if !contains(out, "\u25b6") {
		t.Error("collapsed inline should contain \u25b6")
	}
	if !contains(out, "14 chars") {
		t.Errorf("collapsed inline should show char count, got: %s", out)
	}
}

func TestRenderInlineExpanded(t *testing.T) {
	var tm ThinkingModel
	tm.Verbosity = VerbosityVerbose
	card := ThinkingCard{Text: "some reasoning"}
	out := tm.RenderInline(card)
	if !contains(out, "\u25bc") {
		t.Error("expanded inline should contain \u25bc")
	}
}

func TestRenderPending(t *testing.T) {
	var tm ThinkingModel
	out := tm.RenderPending(80)
	if out != "" {
		t.Error("empty buf should render nothing")
	}
	tm.AppendDelta("in progress...")
	out = tm.RenderPending(80)
	if !contains(out, "thinking...") {
		t.Error("pending render should show thinking...")
	}
	if !contains(out, "in progress...") {
		t.Error("pending render should contain buf text")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
