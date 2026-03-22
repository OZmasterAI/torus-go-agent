package uib

import "testing"

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
