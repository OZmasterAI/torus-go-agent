package uib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

// newSkillTestModel builds a Model with a temp-dir skill registry containing
// a "brainstorm" skill, ready for dispatch tests.
func newSkillTestModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	content := "# Brainstorm helper\nYou are a brainstorm assistant.\n"
	if err := os.WriteFile(filepath.Join(dir, "brainstorm.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
	sr := features.NewSkillRegistry(dir)
	m := NewModel(nil, "test", config.AgentConfig{}, sr, nil)
	m.width, m.height, m.ready = 80, 24, true
	return m
}

func TestCommandParsing(t *testing.T) {
	t.Parallel()
	if !isCommand("/new") {
		t.Fatal("/new should be a command")
	}
	if !isCommand("/exit") {
		t.Fatal("/exit should be a command")
	}
	if isCommand("hello") {
		t.Fatal("hello should not be a command")
	}
	if isCommand("") {
		t.Fatal("empty string should not be a command")
	}
}

func TestAllSlashCommandsExist(t *testing.T) {
	t.Parallel()
	commands := []string{
		"/new", "/clear", "/compact", "/fork", "/switch",
		"/steering", "/branches", "/alias", "/messages", "/stats",
		"/agents", "/mcp-tools", "/skills", "/sequential", "/parallel", "/loop", "/exit",
	}
	for _, cmd := range commands {
		if !isCommand(cmd) {
			t.Fatalf("%s should be recognized as a command", cmd)
		}
	}
}

func TestSkillDispatchSubmitsToAgent(t *testing.T) {
	t.Parallel()
	m := newSkillTestModel(t)
	m.input.SetValue("/brainstorm pick a topic") // typed path: input holds the command

	newM, cmd := m.executeCommand("/brainstorm pick a topic")
	model := newM.(Model)

	msgs := model.chat.messages
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 chat messages (user + placeholder), got %d", len(msgs))
	}
	user := msgs[len(msgs)-2]
	if user.Role != "user" {
		t.Fatalf("second-to-last message role = %q, want user", user.Role)
	}
	if !strings.Contains(user.Text, "You are a brainstorm assistant.") {
		t.Errorf("user message missing skill content (FormatSkillPrompt did not run): %q", user.Text)
	}
	if !strings.Contains(user.Text, "/brainstorm pick a topic") {
		t.Errorf("user message missing original input: %q", user.Text)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" || last.Text != "" {
		t.Errorf("last message should be empty assistant placeholder, got role=%q text=%q", last.Role, last.Text)
	}
	if !model.status.processing {
		t.Error("status.processing should be true after skill dispatch")
	}
	if !model.chat.streaming {
		t.Error("chat.streaming should be true after skill dispatch")
	}
	if model.input.Value() != "" {
		t.Errorf("input should be cleared after submit, got %q", model.input.Value())
	}
	if model.turnCount != 1 {
		t.Errorf("turnCount = %d, want 1", model.turnCount)
	}
	if cmd == nil {
		t.Error("skill dispatch should return a non-nil tea.Cmd")
	}
}

func TestPaletteSkillDispatchPrefills(t *testing.T) {
	t.Parallel()
	m := newSkillTestModel(t)
	msgsBefore := len(m.chat.messages)

	// Palette path: input is empty, command arrives from the overlay.
	newM, cmd := m.executeCommand("/brainstorm")
	model := newM.(Model)

	if got := model.input.Value(); got != "/brainstorm" {
		t.Errorf("input value = %q, want %q (palette prefill so user can add args)", got, "/brainstorm")
	}
	if len(model.chat.messages) != msgsBefore {
		t.Errorf("palette prefill should not add chat messages: before=%d after=%d", msgsBefore, len(model.chat.messages))
	}
	if model.status.processing {
		t.Error("palette prefill should not start processing")
	}
	if cmd != nil {
		t.Error("palette prefill should return a nil tea.Cmd")
	}
}

func TestUnknownSlashCommandSentToAgent(t *testing.T) {
	t.Parallel()
	m := newSkillTestModel(t)
	m.input.SetValue("/notaskill do x")

	newM, _ := m.executeCommand("/notaskill do x")
	model := newM.(Model)

	found := false
	for _, msg := range model.chat.messages {
		if msg.Role == "user" && msg.Text == "/notaskill do x" {
			found = true
			break
		}
	}
	if !found {
		t.Error("unknown slash command should be sent to the agent verbatim as a user message")
	}
	if !model.status.processing {
		t.Error("unknown slash command should start processing (main-TUI fall-through parity)")
	}
}

func TestCmdStats_ShowsCompressionRuns(t *testing.T) {
	m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)

	v := core.CompressionRuns.Load()
	res, _ := m.cmdStats()
	got := res.(Model) // value receiver: assert on the returned model

	if len(got.chat.messages) == 0 {
		t.Fatal("cmdStats should append a stats message")
	}
	last := got.chat.messages[len(got.chat.messages)-1]
	want := fmt.Sprintf("Compression runs: %d", v)
	if !strings.Contains(last.Text, want) {
		t.Errorf("stats output should contain %q, got %q", want, last.Text)
	}
}
