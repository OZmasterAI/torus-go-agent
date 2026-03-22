package uib

import (
	"strings"
	"testing"
)

func TestToolCardRegistryBash(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{Name: "bash", Args: map[string]any{"command": "ls -la"}}, 80)
	if !strings.Contains(card, "bash") {
		t.Fatal("bash card should contain tool name")
	}
	if !strings.Contains(card, "ls") {
		t.Fatal("bash card should contain command")
	}
}

func TestToolCardRegistryEdit(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "edit",
		FilePath: "/tmp/test.go",
		Args:     map[string]any{"old_str": "old", "new_str": "new"},
	}, 80)
	if !strings.Contains(card, "edit") {
		t.Fatal("edit card should contain tool name")
	}
	if !strings.Contains(card, "test.go") {
		t.Fatal("edit card should contain file path")
	}
}

func TestToolCardRegistryWrite(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "write",
		FilePath: "/tmp/out.txt",
		Args:     map[string]any{"content": "line1\nline2\nline3"},
	}, 80)
	if !strings.Contains(card, "write") {
		t.Fatal("write card should contain tool name")
	}
	if !strings.Contains(card, "3 lines") {
		t.Fatal("write card should contain line count")
	}
}

func TestToolCardRegistryRead(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "read",
		FilePath: "/tmp/read.go",
	}, 80)
	if !strings.Contains(card, "read") {
		t.Fatal("read card should contain tool name")
	}
}

func TestToolCardRegistrySearch(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "grep",
		Args:   map[string]any{"pattern": "TODO"},
		Result: "file1.go\nfile2.go",
	}, 80)
	if !strings.Contains(card, "grep") {
		t.Fatal("grep card should contain tool name")
	}
}

func TestToolCardRegistryCustomRenderer(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	reg.Register("custom_tool", &testRenderer{})
	card := reg.Render(&ToolEvent{Name: "custom_tool"}, 80)
	if !strings.Contains(card, "CUSTOM") {
		t.Fatal("custom renderer should be used")
	}
}

type testRenderer struct{}

func (r *testRenderer) Render(ev *ToolEvent, maxWidth int, theme Theme) string {
	return "CUSTOM"
}

func TestToolCardRegistryFallback(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{Name: "unknown_mcp_tool", Result: "some result"}, 80)
	if !strings.Contains(card, "unknown_mcp_tool") {
		t.Fatal("fallback card should contain tool name")
	}
}
