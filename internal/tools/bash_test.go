package tools

import (
	"strings"
	"testing"
)

// TestBashToolSimpleEcho tests basic echo command execution.
func TestBashToolSimpleEcho(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "echo hello",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if result.IsError {
		t.Fatalf("IsError should be false, got %v", result.IsError)
	}

	if !strings.Contains(result.Content, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", result.Content)
	}
}

// TestBashToolExitCodeZero tests a command with explicit success (exit 0).
func TestBashToolExitCodeZero(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "true",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if result.IsError {
		t.Fatalf("IsError should be false for 'true' command, got %v", result.IsError)
	}
}

// TestBashToolFailure tests a command that exits with error code.
func TestBashToolFailure(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "false",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if !result.IsError {
		t.Fatalf("IsError should be true for 'false' command, got %v", result.IsError)
	}
}

// TestBashToolWithWorkingDirectory tests command execution in a specific directory.
func TestBashToolWithWorkingDirectory(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "pwd",
		"cwd":     "/tmp",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if result.IsError {
		t.Fatalf("IsError should be false, got %v", result.IsError)
	}

	if !strings.Contains(result.Content, "/tmp") {
		t.Fatalf("expected '/tmp' in output, got: %s", result.Content)
	}
}

// TestBashToolNoOutput tests a command with no output.
func TestBashToolNoOutput(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "true",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	// true command produces no output, should return "(no output)"
	if result.Content != "(no output)" {
		t.Fatalf("expected '(no output)' for no-output command, got: %s", result.Content)
	}
}

// TestBashToolMultipleCommands tests piped commands.
func TestBashToolMultipleCommands(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "echo hello | cat",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if result.IsError {
		t.Fatalf("IsError should be false, got %v", result.IsError)
	}

	if !strings.Contains(result.Content, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", result.Content)
	}
}

// TestBashToolToolType verifies the tool is correctly configured.
func TestBashToolToolType(t *testing.T) {
	tool := bashTool()

	if tool.Name != "bash" {
		t.Fatalf("expected name 'bash', got %s", tool.Name)
	}

	if tool.Description == "" {
		t.Fatal("Description should not be empty")
	}

	if tool.InputSchema == nil {
		t.Fatal("InputSchema should not be nil")
	}

	if tool.Execute == nil {
		t.Fatal("Execute function should not be nil")
	}

	// Verify command is required
	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be a string slice")
	}

	foundCommand := false
	for _, r := range required {
		if r == "command" {
			foundCommand = true
			break
		}
	}

	if !foundCommand {
		t.Fatal("command should be in required fields")
	}
}

// TestBashToolReturnType verifies the return type is correct.
func TestBashToolReturnType(t *testing.T) {
	tool := bashTool()
	args := map[string]any{
		"command": "echo test",
	}

	result, err := tool.Execute(args)

	if err != nil {
		t.Fatalf("Execute should not return error for valid command: %v", err)
	}

	// Verify result is not nil
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

// TestBashToolMissingCommand tests behavior when command argument is missing.
func TestBashToolMissingCommand(t *testing.T) {
	tool := bashTool()
	args := map[string]any{}

	// Should handle missing command gracefully
	result, err := tool.Execute(args)

	if err != nil {
		// It's OK if Execute returns an error for missing command
		t.Logf("Got expected error for missing command: %v", err)
		return
	}

	// Or it should still return a result with error flag set
	if result == nil {
		t.Fatal("result is nil even with missing command")
	}
}
