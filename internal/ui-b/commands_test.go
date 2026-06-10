package uib

import (
	"fmt"
	"strings"
	"testing"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
)

func TestCommandParsing(t *testing.T) {
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
