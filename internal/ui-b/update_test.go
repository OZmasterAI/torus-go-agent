package uib

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/config"
)

func TestUpdateWindowSize(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := newM.(Model)
	if model.width != 120 {
		t.Fatalf("expected width 120, got %d", model.width)
	}
	if model.height != 40 {
		t.Fatalf("expected height 40, got %d", model.height)
	}
	if !model.ready {
		t.Fatal("model should be ready after WindowSizeMsg")
	}
}

func TestUpdateRoutesToOverlay(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.overlay.Open("help", nil)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := newM.(Model)
	if model.overlay.Active() {
		t.Fatal("overlay should be closed after Escape")
	}
}

func TestUpdateCtrlDQuits(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("Ctrl+D should produce a quit command")
	}
}

func TestUpdateTickMsg(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.status.processing = true
	oldPos := m.status.barPos
	m.Update(TickMsg{})
	// Bar position may or may not change depending on initial state,
	// but it should not panic.
	_ = oldPos
}

func TestStreamThinkingDeltaAppends(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.chat.streaming = true
	m.chat.AddMessage("assistant", "")
	eventCh := make(chan StreamEventMsg, 16)
	m.eventCh = eventCh

	newM, _ := m.Update(StreamEventMsg{Type: StreamThinkingDelta, Thinking: "let me think"})
	model := newM.(Model)
	if !model.chat.thinking.HasPending() {
		t.Fatal("thinking model should have pending text after StreamThinkingDelta")
	}
}

func TestThinkingCollapseOnAgentDone(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true
	m.chat.streaming = true
	m.status.processing = true
	m.chat.AddMessage("assistant", "hello")
	m.chat.thinking.AppendDelta("some reasoning")

	newM, _ := m.Update(AgentDoneMsg{Text: "final answer"})
	model := newM.(Model)
	if model.chat.thinking.HasPending() {
		t.Fatal("thinking should be collapsed after AgentDoneMsg")
	}
	if len(model.chat.thinking.Cards) != 1 {
		t.Fatalf("expected 1 thinking card, got %d", len(model.chat.thinking.Cards))
	}
}

func TestCtrlOToggleThinking(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
	m.width, m.height, m.ready = 80, 24, true

	if m.chat.thinking.Verbosity != 0 {
		t.Fatal("thinking should start at compact (0)")
	}

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model := newM.(Model)
	if model.chat.thinking.Verbosity != 1 {
		t.Fatalf("Ctrl+O should set verbosity to 1 (verbose), got %d", model.chat.thinking.Verbosity)
	}

	newM2, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model2 := newM2.(Model)
	if model2.chat.thinking.Verbosity != 2 {
		t.Fatalf("second Ctrl+O should set verbosity to 2 (full), got %d", model2.chat.thinking.Verbosity)
	}

	newM3, _ := model2.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model3 := newM3.(Model)
	if model3.chat.thinking.Verbosity != 0 {
		t.Fatalf("third Ctrl+O should set verbosity to 0 (compact), got %d", model3.chat.thinking.Verbosity)
	}
}
