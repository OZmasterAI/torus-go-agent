package uib

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStreamEventTypes(t *testing.T) {
	// Verify all event types are distinct integers.
	types := []StreamEventType{StreamTextDelta, StreamThinkingDelta, StreamToolStart, StreamToolEnd, StreamStatusUpdate}
	seen := map[StreamEventType]bool{}
	for _, st := range types {
		if seen[st] {
			t.Fatalf("duplicate StreamEventType: %d", st)
		}
		seen[st] = true
	}
}

func TestStreamThinkingDeltaField(t *testing.T) {
	msg := StreamEventMsg{Type: StreamThinkingDelta, Thinking: "let me reason"}
	if msg.Type != StreamThinkingDelta {
		t.Fatalf("expected StreamThinkingDelta, got %d", msg.Type)
	}
	if msg.Thinking != "let me reason" {
		t.Fatalf("expected thinking text, got %q", msg.Thinking)
	}
}

func TestAllMsgTypesSatisfyTeaMsg(t *testing.T) {
	// Verify each custom message type can be used as a tea.Msg.
	var msgs []tea.Msg
	msgs = append(msgs, StreamEventMsg{})
	msgs = append(msgs, AgentDoneMsg{})
	msgs = append(msgs, AgentErrorMsg{})
	msgs = append(msgs, TickMsg(time.Now()))
	msgs = append(msgs, WorkflowDoneMsg{})
	if len(msgs) != 5 {
		t.Fatalf("expected 5 msg types, got %d", len(msgs))
	}
}

func TestNewDisplayMsg(t *testing.T) {
	dm := NewDisplayMsg("user", "hello")
	if dm.Role != "user" {
		t.Fatalf("expected role=user, got %q", dm.Role)
	}
	if dm.Text != "hello" {
		t.Fatalf("expected text=hello, got %q", dm.Text)
	}
	if dm.Ts.IsZero() {
		t.Fatal("timestamp should not be zero")
	}
}

func TestToolEventFields(t *testing.T) {
	ev := ToolEvent{
		Name:     "bash",
		Args:     map[string]any{"command": "ls"},
		Result:   "file.txt",
		IsError:  false,
		FilePath: "/tmp/test",
		Duration: 42 * time.Millisecond,
	}
	if ev.Name != "bash" {
		t.Fatalf("expected name=bash, got %q", ev.Name)
	}
	if ev.Duration != 42*time.Millisecond {
		t.Fatalf("unexpected duration: %v", ev.Duration)
	}
}
