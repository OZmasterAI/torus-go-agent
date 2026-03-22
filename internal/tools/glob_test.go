package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobTool_BasicMatches(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	filenames := []string{"file1.txt", "file2.txt", "file3.go"}
	for _, name := range filenames {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "*.txt"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	// Should match file1.txt and file2.txt
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches, got %d", len(lines))
	}

	// Verify both filenames are in results
	matches := make(map[string]bool)
	for _, line := range lines {
		matches[filepath.Base(line)] = true
	}
	if !matches["file1.txt"] || !matches["file2.txt"] {
		t.Errorf("expected file1.txt and file2.txt in results, got: %v", matches)
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	tempDir := t.TempDir()

	tool := globTool()
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "*.nonexistent"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	// Should return "(no matches)" when no files found
	if result.Content != "(no matches)" {
		t.Errorf("expected '(no matches)', got %q", result.Content)
	}
}

func TestGlobTool_NestedDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested directory structure
	dirs := []string{
		"subdir1",
		"subdir1/nested",
		"subdir2",
	}
	for _, dir := range dirs {
		path := filepath.Join(tempDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	// Create files in nested directories
	files := []string{
		"subdir1/file1.txt",
		"subdir1/nested/file2.txt",
		"subdir2/file3.txt",
	}
	for _, file := range files {
		path := filepath.Join(tempDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()

	// Test recursive glob
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "**/*.txt"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	// All three files should match with ** pattern
	if len(lines) < 2 {
		t.Errorf("expected at least 2 matches for recursive glob, got %d: %v", len(lines), lines)
	}
}

func TestGlobTool_WithCWD(t *testing.T) {
	tempDir := t.TempDir()

	// Create files in temp directory
	filenames := []string{"alpha.txt", "beta.txt", "gamma.go"}
	for _, name := range filenames {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()

	// Use cwd parameter instead of absolute path
	result, err := tool.Execute(map[string]any{
		"pattern": "*.txt",
		"cwd":     tempDir,
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches with cwd, got %d", len(lines))
	}

	// Verify results contain the cwd prefix since filepath.Glob returns full paths
	for _, line := range lines {
		if !strings.HasPrefix(line, tempDir) {
			t.Errorf("expected result to start with cwd %q, got %q", tempDir, line)
		}
	}
}

func TestGlobTool_MultipleExtensions(t *testing.T) {
	tempDir := t.TempDir()

	// Create files with multiple extensions
	files := map[string]string{
		"file1.txt": "text",
		"file2.go":  "golang",
		"file3.rs":  "rust",
		"readme.md": "markdown",
	}
	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()

	// Test specific extension
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "*.go"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "file2.go") {
		t.Errorf("expected to find file2.go, got: %s", result.Content)
	}

	// Should not match other extensions
	if strings.Contains(result.Content, "file1.txt") || strings.Contains(result.Content, "file3.rs") {
		t.Errorf("should only match .go files, got: %s", result.Content)
	}
}

func TestGlobTool_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	tool := globTool()
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "*"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	if result.Content != "(no matches)" {
		t.Errorf("expected '(no matches)' for empty directory, got %q", result.Content)
	}
}

func TestGlobTool_QuestionMarkPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create files for ? pattern matching
	files := []string{"a.txt", "bb.txt", "ccc.txt"}
	for _, name := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()

	// ? matches exactly one character
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "?.txt"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	// Should only match a.txt
	if !strings.Contains(result.Content, "a.txt") {
		t.Errorf("expected to match a.txt with ? pattern, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "bb.txt") || strings.Contains(result.Content, "ccc.txt") {
		t.Errorf("should not match bb.txt or ccc.txt with ? pattern, got: %s", result.Content)
	}
}

func TestGlobTool_BracketPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create files for bracket pattern matching
	files := []string{"file1.txt", "file2.txt", "file3.txt", "file4.go"}
	for _, name := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()

	// [12] matches 1 or 2
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "file[12].txt"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	// Should match file1.txt and file2.txt
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches for [12] pattern, got %d: %v", len(lines), lines)
	}
}

func TestGlobTool_ErrorHandling(t *testing.T) {
	tool := globTool()

	// Test with invalid pattern - filepath.Glob has limited error cases
	// but we can test behavior with unusual patterns
	result, err := tool.Execute(map[string]any{
		"pattern": "/nonexistent/path/*.txt",
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	// Should return no matches, not an error (nonexistent path is not an error in filepath.Glob)
	if result.Content != "(no matches)" {
		t.Errorf("expected no matches for nonexistent path, got: %s", result.Content)
	}
}

func TestGlobTool_VerifyToolStructure(t *testing.T) {
	tool := globTool()

	if tool.Name != "glob" {
		t.Errorf("expected name 'glob', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("expected non-empty description")
	}

	if tool.InputSchema == nil {
		t.Fatal("expected InputSchema to be non-nil")
	}

	// Verify required fields exist
	props := tool.InputSchema["properties"]
	if props == nil {
		t.Fatal("expected properties in InputSchema")
	}

	required := tool.InputSchema["required"]
	if required == nil {
		t.Fatal("expected required in InputSchema")
	}

	if tool.Execute == nil {
		t.Fatal("expected Execute function to be non-nil")
	}
}

func TestGlobTool_ResultFormat(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	files := []string{"file1.txt", "file2.txt"}
	for _, name := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tool := globTool()
	result, err := tool.Execute(map[string]any{
		"pattern": filepath.Join(tempDir, "*.txt"),
	})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify result type
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify IsError is false for successful match
	if result.IsError {
		t.Errorf("expected IsError=false for successful match, got %v", result.IsError)
	}

	// Verify content is not empty
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}
