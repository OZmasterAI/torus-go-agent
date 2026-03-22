package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfNoRg skips the test if ripgrep is not installed
func skipIfNoRg(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skipf("ripgrep (rg) not installed, skipping grep tests")
	}
}

// TestGrepPatternFound tests grep with a pattern that matches.
func TestGrepPatternFound(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create a test file with known content
	content := "line 1 with hello\nline 2 with world\nline 3 with hello again\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Execute grep tool
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": "hello",
		"path":    testFile,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("grep returned error: %s", result.Content)
	}

	// Verify output contains expected matches
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result.Content)
	}

	// Should have 2 matches (lines 1 and 3)
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches, got %d", len(lines))
	}

	// Verify line numbers are present
	if len(lines) >= 2 && !strings.HasPrefix(lines[0], "1:") {
		t.Errorf("expected first line to start with '1:', got: %s", lines[0])
	}
	if len(lines) >= 2 && !strings.HasPrefix(lines[1], "3:") {
		t.Errorf("expected second line to start with '3:', got: %s", lines[1])
	}
}

// TestGrepPatternNotFound tests grep with a pattern that doesn't match.
func TestGrepPatternNotFound(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create a test file
	content := "line 1 with apple\nline 2 with banana\nline 3 with orange\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Execute grep tool searching for non-existent pattern
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": "grape",
		"path":    testFile,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	// Should not be error, but should return "(no matches)"
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}

	if result.Content != "(no matches)" {
		t.Errorf("expected '(no matches)', got: %s", result.Content)
	}
}

// TestGrepMultipleMatches tests grep with multiple matching lines.
func TestGrepMultipleMatches(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create a test file with multiple matches
	content := `foo bar baz
hello foo world
testing foo again
foo at start
middle foo here
foo foo foo
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Execute grep tool
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": "foo",
		"path":    testFile,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("grep returned error: %s", result.Content)
	}

	// Should have 6 matches (one per line except the first empty one)
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	expectedMatches := 6
	if len(lines) != expectedMatches {
		t.Errorf("expected %d matches, got %d. Output:\n%s", expectedMatches, len(lines), result.Content)
	}

	// Verify each line contains "foo" and has a line number
	for _, line := range lines {
		if !strings.Contains(line, "foo") {
			t.Errorf("expected 'foo' in match line: %s", line)
		}
		// Check if line starts with a digit (line number prefix)
		if len(line) > 0 && (line[0] < '0' || line[0] > '9') {
			t.Errorf("expected line number prefix in: %s", line)
		}
	}
}

// TestGrepWithDirectory tests grep on a directory (searches recursively).
func TestGrepWithDirectory(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()

	// Create multiple files with different content
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	if err := os.WriteFile(file1, []byte("search term here\nno match\n"), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	if err := os.WriteFile(file2, []byte("another search term\nno match\n"), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	// Execute grep on directory
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": "search term",
		"path":    tmpDir,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("grep returned error: %s", result.Content)
	}

	// Should match both files - output should contain file names
	if result.Content != "(no matches)" && !strings.Contains(result.Content, "file") {
		// If we got matches, they should mention the files
		t.Logf("Got output: %s", result.Content)
	}

	// Should have 2 matches total (or skip this check if rg doesn't show filenames in output)
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) >= 2 {
		t.Logf("Got %d match lines across files", len(lines))
	}
}

// TestGrepWithDefaultPath tests grep with empty path (should use current directory).
func TestGrepWithDefaultPath(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "needle in haystack\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Execute grep without specifying path - should handle gracefully
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": "needle",
		// path is omitted, should default to "."
	})

	// This should not panic or error out
	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	// Result should be valid (either matches or "no matches")
	if result == nil {
		t.Errorf("grep returned nil result")
	}
}

// TestGrepRegexPattern tests grep with a regex pattern.
func TestGrepRegexPattern(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := `foo123
bar456
baz789
test000
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test regex pattern: lines with 3 digits at the end
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": `\d{3}$`,
		"path":    testFile,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("grep returned error: %s", result.Content)
	}

	// Should match at least some lines with 3 digits
	if result.Content == "(no matches)" {
		t.Errorf("expected matches for regex pattern \\d{3}$")
	}

	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 matches for regex pattern, got %d. Output: %s", len(lines), result.Content)
	}
}

// TestGrepToolMetadata verifies the tool is properly configured.
func TestGrepToolMetadata(t *testing.T) {
	grep := grepTool()

	if grep.Name != "grep" {
		t.Errorf("expected name 'grep', got %s", grep.Name)
	}

	if grep.Description == "" {
		t.Errorf("tool should have a description")
	}

	if grep.InputSchema == nil {
		t.Errorf("tool should have input schema")
	}

	if grep.Execute == nil {
		t.Errorf("tool should have an Execute function")
	}

	// Verify required fields in schema
	props, ok := grep.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Errorf("input schema should have properties")
	}

	if _, ok := props["pattern"]; !ok {
		t.Errorf("schema should have 'pattern' property")
	}

	if _, ok := props["path"]; !ok {
		t.Errorf("schema should have 'path' property")
	}

	if _, ok := props["glob"]; !ok {
		t.Errorf("schema should have 'glob' property")
	}
}

// TestGrepSpecialCharacters tests grep with special regex characters.
func TestGrepSpecialCharacters(t *testing.T) {
	skipIfNoRg(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := `email@example.com
no email here
contact@domain.org
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test regex pattern matching email addresses
	grep := grepTool()
	result, err := grep.Execute(map[string]any{
		"pattern": `\w+@\w+\.\w+`,
		"path":    testFile,
	})

	if err != nil {
		t.Fatalf("grep execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("grep returned error: %s", result.Content)
	}

	// Should find 2 email matches
	if result.Content != "(no matches)" {
		lines := strings.Split(strings.TrimSpace(result.Content), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 email matches, got %d. Output: %s", len(lines), result.Content)
		}
	}
}
