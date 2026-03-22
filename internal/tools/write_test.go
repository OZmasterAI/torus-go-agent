package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTool_WriteNewFile(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := "Hello, World!"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was written
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Expected content %q, got %q", content, string(fileContent))
	}

	// Check result message
	expectedMsg := "Wrote 1 lines to " + filePath
	if result.Content != expectedMsg {
		t.Errorf("Expected message %q, got %q", expectedMsg, result.Content)
	}
}

func TestWriteTool_OverwriteExistingFile(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Write initial content
	initialContent := "Initial content"
	os.WriteFile(filePath, []byte(initialContent), 0644)

	// Overwrite with new content
	newContent := "New content"
	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   newContent,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was overwritten
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != newContent {
		t.Errorf("Expected content %q, got %q", newContent, string(fileContent))
	}
}

func TestWriteTool_WriteToNestedDirectory(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "a", "b", "c", "test.txt")
	content := "Nested file content"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was written and directories were created
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Expected content %q, got %q", content, string(fileContent))
	}

	// Verify parent directories exist
	parentDir := filepath.Dir(filePath)
	if info, err := os.Stat(parentDir); err != nil {
		t.Fatalf("Parent directory not created: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("Parent path exists but is not a directory")
	}
}

func TestWriteTool_MultilineContent(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "multiline.txt")
	content := "Line 1\nLine 2\nLine 3\n"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify line count in response
	lineCount := strings.Count(content, "\n") + 1
	expectedMsg := "Wrote 4 lines to " + filePath
	if result.Content != expectedMsg {
		t.Errorf("Expected message %q, got %q", expectedMsg, result.Content)
	}

	// Verify file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Expected content with %d lines, got different content", lineCount)
	}
}

func TestWriteTool_EmptyContent(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.txt")
	content := ""

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify empty file was created
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Expected empty content, got %q", string(fileContent))
	}

	// Check line count (empty file = 1 line)
	expectedMsg := "Wrote 1 lines to " + filePath
	if result.Content != expectedMsg {
		t.Errorf("Expected message %q, got %q", expectedMsg, result.Content)
	}
}

func TestWriteTool_SpecialCharacters(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "special.txt")
	content := "Special chars: !@#$%^&*()_+-={}[]|:;\"'<>,.?/\\\t\r\n"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was written with special characters intact
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Special characters not preserved correctly")
	}
}

func TestWriteTool_LargeContent(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.txt")

	// Create 10KB of content
	contentLine := strings.Repeat("x", 100) + "\n"
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(contentLine)
	}
	content := sb.String()

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was written correctly
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Large content was not written correctly")
	}
}

func TestWriteTool_ToolMetadata(t *testing.T) {
	tool := writeTool()

	if tool.Name != "write" {
		t.Errorf("Expected tool name 'write', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Errorf("Expected non-empty description")
	}

	// Verify required input schema
	props := tool.InputSchema["properties"].(map[string]any)
	if props["file_path"] == nil {
		t.Errorf("Missing file_path in input schema")
	}
	if props["content"] == nil {
		t.Errorf("Missing content in input schema")
	}

	required := tool.InputSchema["required"].([]string)
	if len(required) != 2 {
		t.Errorf("Expected 2 required fields, got %d", len(required))
	}
}

func TestWriteTool_MissingFilePath(t *testing.T) {
	tool := writeTool()

	result, err := tool.Execute(map[string]any{
		"content": "test",
	})

	// The tool doesn't validate missing file_path, it just gets empty string
	// This tests actual behavior
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Errorf("Expected error when file_path is missing")
	}
}

func TestWriteTool_FilePermissions(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "perms.txt")
	content := "test"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file has correct permissions (0644)
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	expectedPerm := os.FileMode(0644)
	actualPerm := info.Mode() & os.ModePerm

	if actualPerm != expectedPerm {
		t.Errorf("Expected file permission 0644, got %o", actualPerm)
	}
}

func TestWriteTool_DirectoryPermissions(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "x", "y", "z", "test.txt")

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   "test",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify directories have correct permissions (0755)
	expectedDirPerm := os.FileMode(0755)
	parentPath := filepath.Join(tmpDir, "x")
	for {
		info, err := os.Stat(parentPath)
		if err != nil {
			t.Fatalf("Failed to stat directory: %v", err)
		}

		actualPerm := info.Mode() & os.ModePerm
		if actualPerm != expectedDirPerm {
			t.Errorf("Expected directory permission 0755 for %s, got %o", parentPath, actualPerm)
		}

		if parentPath == filepath.Join(tmpDir, "x", "y", "z") {
			break
		}
		if parentPath == filepath.Join(tmpDir, "x", "y") {
			parentPath = filepath.Join(tmpDir, "x", "y", "z")
		} else {
			parentPath = filepath.Join(tmpDir, "x", "y")
		}
	}
}

func TestWriteTool_UnicodeContent(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "unicode.txt")
	content := "Hello 世界 🌍\nCyrillic: Привет\nArabic: مرحبا\n"

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.Content)
	}

	// Verify file was written with unicode content intact
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(fileContent) != content {
		t.Errorf("Unicode content not preserved correctly")
	}
}

func TestWriteTool_ResultType(t *testing.T) {
	tool := writeTool()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	result, err := tool.Execute(map[string]any{
		"file_path": filePath,
		"content":   "test",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify result type
	if result == nil {
		t.Errorf("Expected non-nil ToolResult")
	}

	if result.Content == "" {
		t.Errorf("Expected result.Content to be non-empty")
	}
}
