package uib

import (
	"strings"
	"testing"
	"time"
)

func TestStatusModelProgressBar(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	s.processing = true
	s.startTime = time.Now().Add(-3 * time.Second)
	bar := s.renderProgressBar(80)
	if !strings.Contains(bar, "\u2501") {
		t.Fatal("progress bar should contain bar characters")
	}
}

func TestStatusModelCompletion(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	s.lastElapsed = 1200 * time.Millisecond
	view := s.renderCompletion()
	if !strings.Contains(view, "Toroidal cycle complete") {
		t.Fatal("should show completion message")
	}
}

func TestStatusModelRenderStatusBar(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:     80,
		ModelName: "test-model",
		TokIn:     100,
		TokOut:    50,
		Cost:      0.01,
		AtBottom:  false,
	})
	if !strings.Contains(bar, "test-model") {
		t.Fatal("status bar should contain model name")
	}
}

func TestStatusModelProcessingOrCompletion(t *testing.T) {
	s := newStatusModel(DefaultTheme())

	// Not processing, no elapsed -> empty
	view := s.renderProcessingOrCompletion(80)
	if view != "" {
		t.Fatalf("expected empty when idle, got %q", view)
	}

	// Processing -> should show progress bar
	s.processing = true
	s.startTime = time.Now()
	view = s.renderProcessingOrCompletion(80)
	if view == "" {
		t.Fatal("should show progress bar when processing")
	}

	// Not processing but has elapsed -> completion
	s.processing = false
	s.lastElapsed = 2 * time.Second
	view = s.renderProcessingOrCompletion(80)
	if !strings.Contains(view, "Toroidal cycle complete") {
		t.Fatal("should show completion when done")
	}
}

// ── Feature #17: lastInputTokens tracking ────────────────────────────────────

func TestStatusBarCtxPercentage(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:           120,
		ModelName:       "claude-3-opus",
		LastInputTokens: 64000,
		ContextWindow:   128000,
	})
	if !strings.Contains(bar, "CTX:") {
		t.Fatal("status bar should contain CTX: label")
	}
	if !strings.Contains(bar, "50%") {
		t.Fatalf("expected 50%% CTX, got: %s", bar)
	}
}

// ── Feature #1: CTX progress bar ─────────────────────────────────────────────

func TestRenderCtxBar(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderCtxBar(50.0)
	// Bar should be 12 characters of block elements.
	if !strings.ContainsRune(bar, '\u2588') {
		t.Fatal("CTX bar should contain filled block chars")
	}
	if !strings.ContainsRune(bar, '\u2591') {
		t.Fatal("CTX bar should contain empty block chars")
	}
}

func TestRenderCtxBarZero(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderCtxBar(0.0)
	if strings.ContainsRune(bar, '\u2588') {
		t.Fatal("0% CTX bar should not contain filled blocks")
	}
}

func TestRenderCtxBarFull(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderCtxBar(100.0)
	if strings.ContainsRune(bar, '\u2591') {
		t.Fatal("100% CTX bar should not contain empty blocks")
	}
}

// ── Feature #2: Turn count ──────────────────────────────────────────────────

func TestStatusBarTurnCount(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:     120,
		ModelName: "test",
		TokIn:     5000,
		TokOut:    1000,
		TurnCount: 3,
	})
	if !strings.Contains(bar, "3 turns") {
		t.Fatalf("expected '3 turns' in status bar, got: %s", bar)
	}
}

func TestStatusBarNoTurnCountWhenZeroTokens(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:     120,
		ModelName: "test",
		TokIn:     0,
		TokOut:    0,
		TurnCount: 3,
	})
	// When totalTok is 0, turn count should not appear
	if strings.Contains(bar, "turns") {
		t.Fatalf("should not show turn count when no tokens, got: %s", bar)
	}
}

// ── Feature #3: Session elapsed time ────────────────────────────────────────

func TestStatusBarSessionElapsed(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:        120,
		ModelName:    "test",
		SessionStart: time.Now().Add(-90 * time.Second),
	})
	// 90 seconds = 1 minute, should show "1m"
	if !strings.Contains(bar, "1m") {
		t.Fatalf("expected '1m' for 90s elapsed, got: %s", bar)
	}
}

func TestStatusBarSessionElapsedSeconds(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:        120,
		ModelName:    "test",
		SessionStart: time.Now().Add(-30 * time.Second),
	})
	// 30 seconds, should show "30s"
	if !strings.Contains(bar, "30s") {
		t.Fatalf("expected '30s' for 30s elapsed, got: %s", bar)
	}
}

// ── Feature #4: Next-prompt cost estimate ───────────────────────────────────

func TestStatusBarNextEstimate(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:        120,
		ModelName:    "test",
		Processing:   false,
		NextEstimate: 12500,
	})
	if !strings.Contains(bar, "next:") {
		t.Fatalf("expected 'next:' estimate in status bar, got: %s", bar)
	}
	if !strings.Contains(bar, "12.5k") {
		t.Fatalf("expected '12.5k' in next estimate, got: %s", bar)
	}
}

func TestStatusBarNoNextEstimateWhileProcessing(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:        120,
		ModelName:    "test",
		Processing:   true,
		NextEstimate: 12500,
	})
	if strings.Contains(bar, "next:") {
		t.Fatalf("should not show next estimate while processing, got: %s", bar)
	}
}

// ── Feature #6: Thinking verbosity indicator ────────────────────────────────

func TestStatusBarVerbosityIndicator(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:          120,
		ModelName:      "test",
		Verbosity:      1,
		VerbosityLabel: "verbose",
	})
	if !strings.Contains(bar, "verbose") {
		t.Fatalf("expected 'verbose' in status bar, got: %s", bar)
	}
}

func TestStatusBarNoVerbosityWhenCompact(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:          120,
		ModelName:      "test",
		Verbosity:      0,
		VerbosityLabel: "compact",
	})
	if strings.Contains(bar, "compact") {
		t.Fatalf("should not show verbosity when compact (default), got: %s", bar)
	}
}

func TestStatusBarFullVerbosity(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:          120,
		ModelName:      "test",
		Verbosity:      2,
		VerbosityLabel: "full",
	})
	if !strings.Contains(bar, "full") {
		t.Fatalf("expected 'full' in status bar, got: %s", bar)
	}
}

// ── Feature: Scroll hint ────────────────────────────────────────────────────

func TestStatusBarScrollHint(t *testing.T) {
	s := newStatusModel(DefaultTheme())
	bar := s.renderStatusBar(StatusBarData{
		Width:     80,
		ModelName: "test",
		AtBottom:  false,
	})
	if !strings.Contains(bar, "PgDn") {
		t.Fatalf("expected scroll hint when not at bottom, got: %s", bar)
	}

	bar = s.renderStatusBar(StatusBarData{
		Width:     80,
		ModelName: "test",
		AtBottom:  true,
	})
	if strings.Contains(bar, "PgDn") {
		t.Fatalf("should not show scroll hint when at bottom, got: %s", bar)
	}
}
