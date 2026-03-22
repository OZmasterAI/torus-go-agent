package uib

import (
	"testing"
)

func TestChatModelAddMessage(t *testing.T) {
	c := newChatModel(DefaultTheme(), 80, 20)
	c.AddMessage("user", "hello")
	if len(c.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(c.messages))
	}
	if c.messages[0].Role != "user" {
		t.Fatalf("expected role 'user', got %q", c.messages[0].Role)
	}
}

func TestChatModelAppendDelta(t *testing.T) {
	c := newChatModel(DefaultTheme(), 80, 20)
	c.AddMessage("assistant", "")
	c.AppendDelta("hello ")
	c.AppendDelta("world")
	if len(c.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(c.messages))
	}
	if c.messages[0].Text != "hello world" {
		t.Fatalf("expected 'hello world', got %q", c.messages[0].Text)
	}
}

func TestChatModelAddToolCard(t *testing.T) {
	c := newChatModel(DefaultTheme(), 80, 20)
	c.AddMessage("assistant", "")
	c.AddToolCard(&ToolEvent{Name: "bash", Args: map[string]any{"command": "ls"}})
	// Should have removed empty placeholder and added tool + new placeholder
	found := false
	for _, m := range c.messages {
		if m.Role == "tool" {
			found = true
		}
	}
	if !found {
		t.Fatal("should have a tool message")
	}
}

func TestChatModelResize(t *testing.T) {
	c := newChatModel(DefaultTheme(), 80, 20)
	c.Resize(120, 30)
	if c.viewport.Width != 120 {
		t.Fatalf("expected viewport width 120, got %d", c.viewport.Width)
	}
}
