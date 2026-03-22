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
	bar := s.renderStatusBar(80, "test-model", 100, 50, 0.01, false)
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
