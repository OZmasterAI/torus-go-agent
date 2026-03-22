package tools

import (
	"os"
	"path/filepath"
	"testing"
	"torus_go_agent/internal/types"
)

// TestEditTool_ReplaceString tests successful string replacement in a file.
func TestEditTool_ReplaceString(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with initial content
	initialContent := "Hello world\nThis is a test\nHello world"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Call edit tool to replace first occurrence
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path":   filePath,
		"old_str":     "Hello world",
		"new_str":     "Goodbye world",
		"replace_all": false,
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Edit tool returned error result: %s", result.Content)
	}
	if result.Content != "Edited "+filePath {
		t.Errorf("Expected success message, got: %s", result.Content)
	}

	// Verify file content was updated (only first occurrence replaced)
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}

	expected := "Goodbye world\nThis is a test\nHello world"
	if string(content) != expected {
		t.Errorf("File content mismatch.\nExpected: %q\nGot: %q", expected, string(content))
	}
}

// TestEditTool_ReplaceAll tests replacing all occurrences.
func TestEditTool_ReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with repeated content
	initialContent := "foo bar\nfoo baz\nfoo qux"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Call edit tool with replace_all=true
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path":   filePath,
		"old_str":     "foo",
		"new_str":     "FOO",
		"replace_all": true,
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Edit tool returned error result: %s", result.Content)
	}

	// Verify all occurrences were replaced
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}

	expected := "FOO bar\nFOO baz\nFOO qux"
	if string(content) != expected {
		t.Errorf("File content mismatch.\nExpected: %q\nGot: %q", expected, string(content))
	}
}

// TestEditTool_StringNotFound tests behavior when old_str is not in the file.
func TestEditTool_StringNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with specific content
	initialContent := "Hello world"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Try to replace a string that doesn't exist
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"old_str":   "nonexistent",
		"new_str":   "something",
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("Expected error result for non-existent string, got success")
	}
	if result.Content != "Error: old_str not found" {
		t.Errorf("Expected 'old_str not found' error message, got: %s", result.Content)
	}

	// Verify file content was NOT modified
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != initialContent {
		t.Errorf("File was modified when it should not have been.\nExpected: %q\nGot: %q", initialContent, string(content))
	}
}

// TestEditTool_EmptyOldString tests behavior with an empty old_str.
func TestEditTool_EmptyOldString(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with content
	initialContent := "Hello world"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Try to replace with an empty old_str
	// Empty string is technically "found" in any string, so this should succeed
	// and insert new_str at the beginning (first replace of empty string)
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path":   filePath,
		"old_str":     "",
		"new_str":     "PREFIX_",
		"replace_all": false,
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	// Empty string matches, so this should succeed
	if result.IsError {
		t.Fatalf("Edit tool returned error result: %s", result.Content)
	}

	// Verify file content: empty string matches at the beginning
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}
	expected := "PREFIX_Hello world"
	if string(content) != expected {
		t.Errorf("File content mismatch.\nExpected: %q\nGot: %q", expected, string(content))
	}
}

// TestEditTool_MultilineString tests replacing a multi-line string.
func TestEditTool_MultilineString(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with multi-line content
	initialContent := "line1\nline2\nline3\nline4"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Replace a multi-line string
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"old_str":   "line2\nline3",
		"new_str":   "REPLACED",
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Edit tool returned error result: %s", result.Content)
	}

	// Verify file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}

	expected := "line1\nREPLACED\nline4"
	if string(content) != expected {
		t.Errorf("File content mismatch.\nExpected: %q\nGot: %q", expected, string(content))
	}
}

// TestEditTool_FileNotFound tests behavior when the file doesn't exist.
func TestEditTool_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent.txt")

	// Execute: Try to edit a file that doesn't exist
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"old_str":   "test",
		"new_str":   "replacement",
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("Expected error result for non-existent file")
	}
	if result.Content == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

// TestEditTool_ToolStructure verifies the tool is properly configured.
func TestEditTool_ToolStructure(t *testing.T) {
	tool := editTool()

	// Verify basic structure
	if tool.Name != "edit" {
		t.Errorf("Expected tool name 'edit', got: %s", tool.Name)
	}
	if tool.Description == "" {
		t.Errorf("Expected non-empty description")
	}
	if tool.Execute == nil {
		t.Errorf("Expected non-nil Execute function")
	}

	// Verify input schema structure
	schema := tool.InputSchema
	if schema["type"] != "object" {
		t.Errorf("Expected schema type 'object'")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Errorf("Expected properties to be a map")
	}

	// Check required fields
	required, ok := schema["required"].([]string)
	if !ok {
		t.Errorf("Expected required to be []string")
	}

	expectedRequired := []string{"file_path", "old_str", "new_str"}
	if len(required) != len(expectedRequired) {
		t.Errorf("Expected %d required fields, got %d", len(expectedRequired), len(required))
	}

	// Verify all required fields are in properties
	for _, field := range expectedRequired {
		if _, exists := props[field]; !exists {
			t.Errorf("Expected required field %q in properties", field)
		}
	}

	// Verify optional field exists
	if _, exists := props["replace_all"]; !exists {
		t.Errorf("Expected optional field 'replace_all' in properties")
	}
}

// TestEditTool_NoReplaceAllDefault tests that replace_all defaults to false.
func TestEditTool_NoReplaceAllDefault(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Setup: Create a file with repeated content
	initialContent := "foo foo foo"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute: Call edit tool without replace_all parameter (defaults to false)
	tool := editTool()
	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"old_str":   "foo",
		"new_str":   "bar",
	})

	// Assert
	if err != nil {
		t.Fatalf("Edit tool returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Edit tool returned error result: %s", result.Content)
	}

	// Verify only first occurrence was replaced
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}

	expected := "bar foo foo"
	if string(content) != expected {
		t.Errorf("File content mismatch.\nExpected: %q\nGot: %q", expected, string(content))
	}
}

// TestBuildDefaultTools verifies editTool is included in default tools.
func TestBuildDefaultTools(t *testing.T) {
	tools := BuildDefaultTools()

	if len(tools) == 0 {
		t.Fatalf("Expected non-empty tools list")
	}

	// Find the edit tool
	var editToolFound types.Tool
	for _, tool := range tools {
		if tool.Name == "edit" {
			editToolFound = tool
			break
		}
	}

	if editToolFound.Name == "" {
		t.Fatalf("Expected 'edit' tool in default tools")
	}

	// Verify it's the same structure as editTool()
	if editToolFound.Name != editTool().Name {
		t.Errorf("Tool name mismatch")
	}
}
