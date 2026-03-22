package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTool_ExistingFile(t *testing.T) {
	// Setup: Create a temporary file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": testFile})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}

	// Check that all 5 lines are in the output
	if !strings.Contains(result.Content, "line 1") {
		t.Errorf("Expected 'line 1' in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line 5") {
		t.Errorf("Expected 'line 5' in output, got: %s", result.Content)
	}

	// Check line numbering (should be 1-indexed: " 1", " 2", etc.)
	lines := strings.Split(result.Content, "\n")
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 lines in output, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "1") || !strings.Contains(lines[0], "line 1") {
		t.Errorf("Expected first line to have line number 1, got: %s", lines[0])
	}
}

func TestReadTool_NonexistentFile(t *testing.T) {
	// Execute read tool on non-existent file
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": "/nonexistent/path/to/file.txt"})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if !result.IsError {
		t.Fatal("Expected IsError=true for non-existent file")
	}
	if !strings.Contains(result.Content, "Error") {
		t.Errorf("Expected 'Error' in content, got: %s", result.Content)
	}
}

func TestReadTool_WithOffset(t *testing.T) {
	// Setup: Create a temporary file with multiple lines
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool with offset=2 (skip first 2 lines)
	tool := readTool()
	result, err := tool.Execute(map[string]any{
		"file_path": testFile,
		"offset":    2.0,
	})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}

	// Should start from line 3
	if !strings.Contains(result.Content, "line 3") {
		t.Errorf("Expected 'line 3' in output, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "line 1") {
		t.Errorf("Did not expect 'line 1' in output (offset=2), got: %s", result.Content)
	}
	if strings.Contains(result.Content, "line 2") {
		t.Errorf("Did not expect 'line 2' in output (offset=2), got: %s", result.Content)
	}
}

func TestReadTool_WithLimit(t *testing.T) {
	// Setup: Create a temporary file with multiple lines
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool with limit=2 (read only 2 lines)
	tool := readTool()
	result, err := tool.Execute(map[string]any{
		"file_path": testFile,
		"limit":     2.0,
	})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}

	// Should only contain first 2 lines
	lines := strings.Split(result.Content, "\n")
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 lines, got %d", len(lines))
	}

	if !strings.Contains(result.Content, "line 1") || !strings.Contains(result.Content, "line 2") {
		t.Errorf("Expected 'line 1' and 'line 2', got: %s", result.Content)
	}
	if strings.Contains(result.Content, "line 3") {
		t.Errorf("Did not expect 'line 3' in output (limit=2), got: %s", result.Content)
	}
}

func TestReadTool_WithOffsetAndLimit(t *testing.T) {
	// Setup: Create a temporary file with multiple lines
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool with offset=1, limit=3 (read 3 lines starting from line 2)
	tool := readTool()
	result, err := tool.Execute(map[string]any{
		"file_path": testFile,
		"offset":    1.0,
		"limit":     3.0,
	})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}

	// Should contain lines 2, 3, 4
	if !strings.Contains(result.Content, "line 2") {
		t.Errorf("Expected 'line 2' in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line 3") {
		t.Errorf("Expected 'line 3' in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line 4") {
		t.Errorf("Expected 'line 4' in output, got: %s", result.Content)
	}

	// Should not contain lines 1 and 5
	if strings.Contains(result.Content, "line 1") {
		t.Errorf("Did not expect 'line 1' in output, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "line 5") {
		t.Errorf("Did not expect 'line 5' in output, got: %s", result.Content)
	}
}

func TestReadTool_EmptyFile(t *testing.T) {
	// Setup: Create an empty temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty test file: %v", err)
	}

	// Execute read tool
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": testFile})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error for empty file, but IsError=true. Content: %s", result.Content)
	}
	// Empty file should produce minimal output (just empty lines or newlines)
}

func TestReadTool_OffsetBeyondFileLength(t *testing.T) {
	// Setup: Create a temporary file with 3 lines
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool with offset beyond file length
	tool := readTool()
	result, err := tool.Execute(map[string]any{
		"file_path": testFile,
		"offset":    10.0, // Beyond the 3 lines
	})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}
	// Should return empty content or minimal output
}

func TestReadTool_LineNumbering(t *testing.T) {
	// Setup: Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "first\nsecond\nthird"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": testFile})

	// Verify line numbers are 1-indexed
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	lines := strings.Split(result.Content, "\n")

	// Check first line starts with " 1 " (formatted as 6-digit number)
	if !strings.HasPrefix(lines[0], "     1") {
		t.Errorf("Expected first line to start with '     1', got: %s", lines[0])
	}

	// Check second line starts with " 2 "
	if !strings.HasPrefix(lines[1], "     2") {
		t.Errorf("Expected second line to start with '     2', got: %s", lines[1])
	}
}

func TestReadTool_FileWithSpecialCharacters(t *testing.T) {
	// Setup: Create a file with special characters
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "special.txt")
	content := "line with\ttab\nline with 'quotes'\nline with \"double quotes\"\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Execute read tool
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": testFile})

	// Verify
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.IsError {
		t.Fatalf("Expected no error, but IsError=true. Content: %s", result.Content)
	}

	// Check that special characters are preserved
	if !strings.Contains(result.Content, "tab") {
		t.Errorf("Expected tab content to be preserved, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "'quotes'") {
		t.Errorf("Expected single quotes to be preserved, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, `"double quotes"`) {
		t.Errorf("Expected double quotes to be preserved, got: %s", result.Content)
	}
}

func TestReadTool_Permissions(t *testing.T) {
	// Setup: Create a file and remove read permissions
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "noperm.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remove read permissions
	if err := os.Chmod(testFile, 0000); err != nil {
		t.Fatalf("Failed to change permissions: %v", err)
	}

	// Cleanup: restore permissions for cleanup
	defer os.Chmod(testFile, 0644)

	// Execute read tool (should fail)
	tool := readTool()
	result, err := tool.Execute(map[string]any{"file_path": testFile})

	// Verify error handling
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if !result.IsError {
		t.Fatal("Expected IsError=true for permission denied")
	}
	if !strings.Contains(result.Content, "Error") {
		t.Errorf("Expected 'Error' in content, got: %s", result.Content)
	}
}
