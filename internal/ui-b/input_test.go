package uib

import (
	"strings"
	"testing"
)

func TestInputModelPlaceholder(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	view := m.View()
	if !strings.Contains(view, "Type a message...") {
		t.Fatal("empty input should show placeholder")
	}
}

func TestInputModelSetValue(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	m.SetValue("hello world")
	if m.Value() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", m.Value())
	}
	if m.cursorPos != len([]rune("hello world")) {
		t.Fatalf("cursor should be at end, got %d", m.cursorPos)
	}
}

func TestInputModelClear(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	m.SetValue("some text")
	m.Clear()
	if m.Value() != "" {
		t.Fatal("clear should empty input")
	}
	if m.cursorPos != 0 {
		t.Fatal("cursor should be at 0 after clear")
	}
}

func TestInputModelInsertAtCursor(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	m.insertAtCursor("abc")
	if m.Value() != "abc" {
		t.Fatalf("expected 'abc', got %q", m.Value())
	}
	m.cursorPos = 1
	m.insertAtCursor("X")
	if m.Value() != "aXbc" {
		t.Fatalf("expected 'aXbc', got %q", m.Value())
	}
}

func TestInputModelAutocomplete(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	m.SetValue("@ma")
	m.triggerAutocomplete([]string{"main.go", "Makefile", "README.md"})
	if len(m.acList) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.acList))
	}
}

func TestInputModelResize(t *testing.T) {
	m := newInputModel(DefaultTheme(), 80)
	m.Resize(120)
	if m.width != 120 {
		t.Fatalf("expected width 120, got %d", m.width)
	}
}
