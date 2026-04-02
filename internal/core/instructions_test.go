package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiscoverInstructionFiles verifies discovery order with TORUS.md at root and subdirectory.
func TestDiscoverInstructionFiles(t *testing.T) {
	root := t.TempDir()
	// Create .git so root is recognized as git root.
	os.Mkdir(filepath.Join(root, ".git"), 0755)

	// TORUS.md at root.
	os.WriteFile(filepath.Join(root, "TORUS.md"), []byte("root instructions"), 0644)

	// Subdirectory with its own TORUS.md.
	sub := filepath.Join(root, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "TORUS.md"), []byte("sub instructions"), 0644)

	files := DiscoverInstructionFiles(sub)

	// Filter to only project-type files from our temp dir (skip user-global).
	var projectFiles []InstructionFile
	for _, f := range files {
		if strings.HasPrefix(f.Path, root) {
			projectFiles = append(projectFiles, f)
		}
	}

	if len(projectFiles) < 2 {
		t.Fatalf("expected at least 2 project files, got %d", len(projectFiles))
	}

	// Root should appear before sub (lower priority first).
	rootIdx := -1
	subIdx := -1
	for i, f := range projectFiles {
		if f.Path == filepath.Join(root, "TORUS.md") {
			rootIdx = i
		}
		if f.Path == filepath.Join(sub, "TORUS.md") {
			subIdx = i
		}
	}
	if rootIdx < 0 {
		t.Fatal("root TORUS.md not found")
	}
	if subIdx < 0 {
		t.Fatal("sub TORUS.md not found")
	}
	if rootIdx >= subIdx {
		t.Fatalf("root (idx=%d) should appear before sub (idx=%d)", rootIdx, subIdx)
	}

	// Verify memory types are project.
	for _, f := range projectFiles {
		if f.MemType != MemoryTypeProject {
			t.Errorf("expected MemoryTypeProject for %s, got %s", f.Path, f.MemType)
		}
	}
}

// TestDiscoverInstructionFiles_TorusDir verifies .torus/TORUS.md and .torus/rules/*.md are found.
func TestDiscoverInstructionFiles_TorusDir(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0755)

	// Create .torus/TORUS.md
	torusDir := filepath.Join(root, ".torus")
	os.MkdirAll(torusDir, 0755)
	os.WriteFile(filepath.Join(torusDir, "TORUS.md"), []byte("torus dir instructions"), 0644)

	// Create .torus/rules/foo.md
	rulesDir := filepath.Join(torusDir, "rules")
	os.MkdirAll(rulesDir, 0755)
	os.WriteFile(filepath.Join(rulesDir, "foo.md"), []byte("foo rule"), 0644)

	files := DiscoverInstructionFiles(root)

	var found []string
	for _, f := range files {
		if strings.HasPrefix(f.Path, root) {
			found = append(found, f.Path)
		}
	}

	torusMD := filepath.Join(torusDir, "TORUS.md")
	fooMD := filepath.Join(rulesDir, "foo.md")

	hasTorusMD := false
	hasFooMD := false
	for _, p := range found {
		if p == torusMD {
			hasTorusMD = true
		}
		if p == fooMD {
			hasFooMD = true
		}
	}
	if !hasTorusMD {
		t.Errorf(".torus/TORUS.md not found in discovered files: %v", found)
	}
	if !hasFooMD {
		t.Errorf(".torus/rules/foo.md not found in discovered files: %v", found)
	}
}

// TestDiscoverInstructionFiles_LocalOverride verifies TORUS.local.md appears last with MemoryTypeLocal.
func TestDiscoverInstructionFiles_LocalOverride(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0755)
	os.WriteFile(filepath.Join(root, "TORUS.md"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(root, "TORUS.local.md"), []byte("local override"), 0644)

	files := DiscoverInstructionFiles(root)

	// Filter to files from our temp dir.
	var projectFiles []InstructionFile
	for _, f := range files {
		if strings.HasPrefix(f.Path, root) {
			projectFiles = append(projectFiles, f)
		}
	}

	if len(projectFiles) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(projectFiles))
	}

	last := projectFiles[len(projectFiles)-1]
	if last.MemType != MemoryTypeLocal {
		t.Errorf("last file should be MemoryTypeLocal, got %s", last.MemType)
	}
	if last.Content != "local override" {
		t.Errorf("last file content: got %q, want %q", last.Content, "local override")
	}
	if !strings.HasSuffix(last.Path, "TORUS.local.md") {
		t.Errorf("last file path should end with TORUS.local.md, got %s", last.Path)
	}
}

// TestParseFrontmatter_WithPaths tests content with YAML frontmatter containing paths.
func TestParseFrontmatter_WithPaths(t *testing.T) {
	content := "---\npaths:\n- *.go\n- internal/**\n---\nBody here"

	paths, body := ParseFrontmatter(content)

	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "*.go" {
		t.Errorf("paths[0]: got %q, want %q", paths[0], "*.go")
	}
	if paths[1] != "internal/**" {
		t.Errorf("paths[1]: got %q, want %q", paths[1], "internal/**")
	}
	if body != "Body here" {
		t.Errorf("body: got %q, want %q", body, "Body here")
	}
}

// TestParseFrontmatter_NoPaths tests content without frontmatter.
func TestParseFrontmatter_NoPaths(t *testing.T) {
	content := "Just some regular content\nwith multiple lines"

	paths, body := ParseFrontmatter(content)

	if paths != nil {
		t.Errorf("expected nil paths, got %v", paths)
	}
	if body != content {
		t.Errorf("body should equal original content: got %q", body)
	}
}

// TestParseFrontmatter_Empty tests empty frontmatter block.
// Standard YAML frontmatter requires content between the --- delimiters,
// so an empty block is "---\n\n---\n". A "---\n---\n" sequence (no content
// between delimiters) does not parse as valid frontmatter.
func TestParseFrontmatter_Empty(t *testing.T) {
	// Empty block with blank line between delimiters.
	content := "---\n\n---\nBody after empty frontmatter"

	paths, body := ParseFrontmatter(content)

	if paths != nil {
		t.Errorf("expected nil paths for empty frontmatter, got %v", paths)
	}
	if body != "Body after empty frontmatter" {
		t.Errorf("body: got %q, want %q", body, "Body after empty frontmatter")
	}

	// Adjacent delimiters (no content) are not parsed as frontmatter.
	contentNoGap := "---\n---\nBody here"
	paths2, body2 := ParseFrontmatter(contentNoGap)
	if paths2 != nil {
		t.Errorf("adjacent delimiters: expected nil paths, got %v", paths2)
	}
	if body2 != contentNoGap {
		t.Errorf("adjacent delimiters: body should be original content, got %q", body2)
	}
}

// TestExpandIncludes verifies basic @include expansion between two files.
func TestExpandIncludes(t *testing.T) {
	dir := t.TempDir()

	// Create the included file.
	os.WriteFile(filepath.Join(dir, "other.md"), []byte("included content"), 0644)

	// Source content referencing the other file.
	content := "before\n@other.md\nafter\n"

	result := ExpandIncludes(content, dir, nil)

	if !strings.Contains(result, "before") {
		t.Error("result missing 'before'")
	}
	if !strings.Contains(result, "included content") {
		t.Error("result missing 'included content'")
	}
	if !strings.Contains(result, "after") {
		t.Error("result missing 'after'")
	}
}

// TestExpandIncludes_Circular verifies circular references do not cause infinite loops.
func TestExpandIncludes_Circular(t *testing.T) {
	dir := t.TempDir()

	// A includes B, B includes A.
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A start\n@b.md\nA end\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B start\n@a.md\nB end\n"), 0644)

	content := "root\n@a.md\n"
	result := ExpandIncludes(content, dir, nil)

	// Should contain content from both files but not loop.
	if !strings.Contains(result, "root") {
		t.Error("result missing 'root'")
	}
	if !strings.Contains(result, "A start") {
		t.Error("result missing 'A start'")
	}
	if !strings.Contains(result, "B start") {
		t.Error("result missing 'B start'")
	}
	// The circular back-reference (@a.md in b.md) should be skipped,
	// so A start should appear only once.
	if strings.Count(result, "A start") > 1 {
		t.Error("circular reference not handled: 'A start' appears multiple times")
	}
}

// TestExpandIncludes_MissingFile verifies @nonexistent.md is silently skipped.
func TestExpandIncludes_MissingFile(t *testing.T) {
	dir := t.TempDir()

	content := "before\n@nonexistent.md\nafter\n"
	result := ExpandIncludes(content, dir, nil)

	if !strings.Contains(result, "before") {
		t.Error("result missing 'before'")
	}
	if !strings.Contains(result, "after") {
		t.Error("result missing 'after'")
	}
	if strings.Contains(result, "nonexistent") {
		t.Error("nonexistent file reference should be silently skipped")
	}
}

// TestExpandIncludes_InCodeBlock verifies @ inside code blocks is NOT expanded.
func TestExpandIncludes_InCodeBlock(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "other.md"), []byte("SHOULD NOT APPEAR"), 0644)

	content := "before\n```\n@other.md\n```\nafter\n"
	result := ExpandIncludes(content, dir, nil)

	if strings.Contains(result, "SHOULD NOT APPEAR") {
		t.Error("@ inside code block should not be expanded")
	}
	if !strings.Contains(result, "@other.md") {
		t.Error("@ line inside code block should be preserved verbatim")
	}
}

// TestBuildPrompt_AllUnconditional verifies files without Paths are always included.
func TestBuildPrompt_AllUnconditional(t *testing.T) {
	files := []InstructionFile{
		{Content: "first", MemType: MemoryTypeProject},
		{Content: "second", MemType: MemoryTypeUser},
		{Content: "third", MemType: MemoryTypeLocal},
	}

	result := BuildPrompt(files, nil)

	if !strings.Contains(result, "first") {
		t.Error("missing 'first'")
	}
	if !strings.Contains(result, "second") {
		t.Error("missing 'second'")
	}
	if !strings.Contains(result, "third") {
		t.Error("missing 'third'")
	}
}

// TestBuildPrompt_ConditionalMatches verifies conditional file included when glob matches.
func TestBuildPrompt_ConditionalMatches(t *testing.T) {
	files := []InstructionFile{
		{Content: "always present", MemType: MemoryTypeProject},
		{Content: "go rules", MemType: MemoryTypeProject, Paths: []string{"*.go"}},
	}

	result := BuildPrompt(files, []string{"main.go"})

	if !strings.Contains(result, "always present") {
		t.Error("unconditional file missing")
	}
	if !strings.Contains(result, "go rules") {
		t.Error("conditional file should be included when glob matches")
	}
}

// TestBuildPrompt_ConditionalNoMatch verifies conditional file excluded when glob doesn't match.
func TestBuildPrompt_ConditionalNoMatch(t *testing.T) {
	files := []InstructionFile{
		{Content: "always present", MemType: MemoryTypeProject},
		{Content: "rust rules", MemType: MemoryTypeProject, Paths: []string{"*.rs"}},
	}

	result := BuildPrompt(files, []string{"main.go"})

	if !strings.Contains(result, "always present") {
		t.Error("unconditional file missing")
	}
	if strings.Contains(result, "rust rules") {
		t.Error("conditional file should be excluded when glob doesn't match")
	}
}

// TestLoadAndParseAll is an end-to-end test: create temp dir with files including
// frontmatter and @includes, verify full pipeline.
func TestLoadAndParseAll(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0755)

	// Main TORUS.md with frontmatter and an include.
	mainContent := "---\npaths:\n- *.go\n---\nMain rules\n@extra.md\n"
	os.WriteFile(filepath.Join(root, "TORUS.md"), []byte(mainContent), 0644)

	// Included file.
	os.WriteFile(filepath.Join(root, "extra.md"), []byte("Extra content here"), 0644)

	files := LoadAndParseAll(root, LoadReasonSessionStart)

	// Find our file among results (skip user-global if any).
	var found *InstructionFile
	for i, f := range files {
		if f.Path == filepath.Join(root, "TORUS.md") {
			found = &files[i]
			break
		}
	}
	if found == nil {
		t.Fatal("TORUS.md not found in results")
	}

	// Check paths were parsed.
	if len(found.Paths) != 1 || found.Paths[0] != "*.go" {
		t.Errorf("expected Paths=[*.go], got %v", found.Paths)
	}

	// Check include was expanded.
	if !strings.Contains(found.Content, "Extra content here") {
		t.Error("@include not expanded: missing 'Extra content here'")
	}

	// Check frontmatter was stripped.
	if strings.Contains(found.Content, "---") {
		t.Error("frontmatter delimiters should be stripped from content")
	}

	// Check load reason.
	if found.LoadReason != LoadReasonSessionStart {
		t.Errorf("expected LoadReasonSessionStart, got %s", found.LoadReason)
	}

	// Check memory type.
	if found.MemType != MemoryTypeProject {
		t.Errorf("expected MemoryTypeProject, got %s", found.MemType)
	}
}
