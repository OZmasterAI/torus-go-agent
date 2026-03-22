package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillsEdge_MissingSkillDirectory tests behavior with a missing skills directory.
func TestSkillsEdge_MissingSkillDirectory(t *testing.T) {
	nonexistent := "/nonexistent/skills/dir/that/does/not/exist"
	sr := NewSkillRegistry(nonexistent)

	if sr == nil {
		t.Error("expected non-nil registry for missing directory")
	}

	skills := sr.List()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills for missing directory, got %d", len(skills))
	}

	skill, found := sr.Get("anything")
	if found {
		t.Error("expected Get to return false for missing directory")
	}
	if skill != nil {
		t.Error("expected skill to be nil for missing directory")
	}
}

// TestSkillsEdge_EmptySkillName tests handling of empty skill names.
func TestSkillsEdge_EmptySkillName(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	sr := NewSkillRegistry(tmpDir)

	skill, found := sr.Get("")
	if found {
		t.Error("expected empty skill name to not be found")
	}
	if skill != nil {
		t.Error("expected skill to be nil for empty name")
	}

	name, found := sr.IsSkillCommand("")
	if found {
		t.Error("expected empty command to not be found")
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

// TestSkillsEdge_SkillNameWithDots tests skill files with dots in their names.
func TestSkillsEdge_SkillNameWithDots(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create skill with dots in name (but only one .md extension)
	skillPath := filepath.Join(tmpDir, "my.special.skill.md")
	if err := os.WriteFile(skillPath, []byte("# Skill\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("my.special.skill")

	if !found {
		t.Error("expected skill with dots in name to be found")
	}
	if skill == nil {
		t.Error("expected non-nil skill")
	}
	if skill.Name != "my.special.skill" {
		t.Errorf("expected name 'my.special.skill', got %q", skill.Name)
	}
}

// TestSkillsEdge_SkillNameWithHyphens tests skill files with hyphens.
func TestSkillsEdge_SkillNameWithHyphens(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "multi-word-skill.md")
	if err := os.WriteFile(skillPath, []byte("# Multi Word Skill\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("multi-word-skill")

	if !found {
		t.Error("expected skill with hyphens to be found")
	}
	if skill.Name != "multi-word-skill" {
		t.Errorf("expected name 'multi-word-skill', got %q", skill.Name)
	}
}

// TestSkillsEdge_CaseSensitiveSkillNames tests that skill names are case-sensitive.
func TestSkillsEdge_CaseSensitiveSkillNames(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "MySkill.md")
	if err := os.WriteFile(skillPath, []byte("# My Skill\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Exact case should work
	skill, found := sr.Get("MySkill")
	if !found {
		t.Error("expected case-sensitive lookup to find 'MySkill'")
	}

	// Different case should not work
	skill, found = sr.Get("myskill")
	if found {
		t.Error("expected case-sensitive lookup to not find 'myskill'")
	}
	if skill != nil {
		t.Error("expected skill to be nil for wrong case")
	}
}

// TestSkillsEdge_SkillCommandCaseSensitive tests that IsSkillCommand is case-sensitive.
func TestSkillsEdge_SkillCommandCaseSensitive(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "BrainStorm.md")
	if err := os.WriteFile(skillPath, []byte("# Brain Storm\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Exact case should match
	name, found := sr.IsSkillCommand("/BrainStorm")
	if !found {
		t.Error("expected case-sensitive command to find /BrainStorm")
	}
	if name != "BrainStorm" {
		t.Errorf("expected 'BrainStorm', got %q", name)
	}

	// Different case should not match
	name, found = sr.IsSkillCommand("/brainstorm")
	if found {
		t.Error("expected case-sensitive command to not find /brainstorm")
	}
}

// TestSkillsEdge_DuplicateSkillFiles tests behavior when reloading with duplicate names (last one wins).
func TestSkillsEdge_DuplicateSkillFiles(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create first version of skill
	skill1Path := filepath.Join(tmpDir, "skill.md")
	if err := os.WriteFile(skill1Path, []byte("# Skill\nVersion 1"), 0644); err != nil {
		t.Fatalf("failed to write skill1: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, _ := sr.Get("skill")
	if !strings.Contains(skill.Content, "Version 1") {
		t.Error("expected Version 1 content")
	}

	// Overwrite with second version
	if err := os.WriteFile(skill1Path, []byte("# Skill\nVersion 2"), 0644); err != nil {
		t.Fatalf("failed to write skill2: %v", err)
	}

	// Reload
	sr.Load()
	skill, _ = sr.Get("skill")
	if !strings.Contains(skill.Content, "Version 2") {
		t.Error("expected Version 2 content after reload")
	}
}

// TestSkillsEdge_VeryLongSkillName tests handling of very long skill names.
func TestSkillsEdge_VeryLongSkillName(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create a skill with a very long name
	longName := strings.Repeat("very-long-skill-name-", 10)
	skillPath := filepath.Join(tmpDir, longName+".md")
	if err := os.WriteFile(skillPath, []byte("# Long Name\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get(longName)

	if !found {
		t.Error("expected long skill name to be found")
	}
	if skill == nil {
		t.Error("expected skill to be non-nil")
	}
}

// TestSkillsEdge_SkillContentWithSpecialCharacters tests skills with special characters in content.
func TestSkillsEdge_SkillContentWithSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	specialContent := `# Special Characters
This content has special chars: !@#$%^&*()
Unicode: αβγδ, 中文, 🚀
Escape sequences: \n \t \"quoted\"`

	skillPath := filepath.Join(tmpDir, "special.md")
	if err := os.WriteFile(skillPath, []byte(specialContent), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("special")

	if !found {
		t.Error("expected skill with special characters to be found")
	}
	if !strings.Contains(skill.Content, "!@#$%^&*()") {
		t.Error("expected special ASCII chars to be preserved")
	}
	if !strings.Contains(skill.Content, "αβγδ") {
		t.Error("expected unicode chars to be preserved")
	}
}

// TestSkillsEdge_VeryLargeSkillContent tests handling of very large skill files.
func TestSkillsEdge_VeryLargeSkillContent(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create a large skill file (1MB)
	largeContent := "# Large Skill\n" + strings.Repeat("x", 1024*1024)
	skillPath := filepath.Join(tmpDir, "large.md")
	if err := os.WriteFile(skillPath, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to write large skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("large")

	if !found {
		t.Error("expected large skill to be found")
	}
	if skill == nil {
		t.Error("expected skill to be non-nil")
	}
	if len(skill.Content) < 1024*1024 {
		t.Error("expected content to be large")
	}
}

// TestSkillsEdge_SkillWithOnlyHeading tests skill that is just a heading.
func TestSkillsEdge_SkillWithOnlyHeading(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "heading-only.md")
	if err := os.WriteFile(skillPath, []byte("# Just A Heading"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("heading-only")

	if !found {
		t.Error("expected heading-only skill to be found")
	}
	if skill.Description != "Just A Heading" {
		t.Errorf("expected description 'Just A Heading', got %q", skill.Description)
	}
	if skill.Content != "" {
		t.Errorf("expected empty content, got %q", skill.Content)
	}
}

// TestSkillsEdge_MultipleHeadingLevels tests handling of different heading levels.
func TestSkillsEdge_MultipleHeadingLevels(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	content := `### Level 3 Heading
Some content
#### Level 4 Heading
More content`

	skillPath := filepath.Join(tmpDir, "multi-level.md")
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("multi-level")

	if !found {
		t.Error("expected multi-level heading skill to be found")
	}
	// First # (regardless of level) becomes description
	if skill.Description != "Level 3 Heading" {
		t.Errorf("expected description 'Level 3 Heading', got %q", skill.Description)
	}
}

// TestSkillsEdge_SkillWithBlankLinesInContent tests skill with many blank lines.
func TestSkillsEdge_SkillWithBlankLinesInContent(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	content := `# Skill With Blanks
First paragraph.


Second paragraph after blanks.


Third paragraph.`

	skillPath := filepath.Join(tmpDir, "blanks.md")
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("blanks")

	if !found {
		t.Error("expected skill to be found")
	}
	// Blank lines in middle of content should be preserved
	if !strings.Contains(skill.Content, "\n\n\nSecond") {
		t.Error("expected blank lines in middle to be preserved")
	}
}

// TestSkillsEdge_IsSkillCommand_MultipleSpaces tests command parsing with multiple spaces.
func TestSkillsEdge_IsSkillCommand_MultipleSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(skillPath, []byte("# Test\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Multiple spaces between command and args
	name, found := sr.IsSkillCommand("/test    arg1    arg2    arg3")
	if !found {
		t.Error("expected command to be found with multiple spaces")
	}
	if name != "test" {
		t.Errorf("expected 'test', got %q", name)
	}
}

// TestSkillsEdge_IsSkillCommand_TabsAndSpaces tests command with mixed whitespace.
func TestSkillsEdge_IsSkillCommand_TabsAndSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "cmd.md")
	if err := os.WriteFile(skillPath, []byte("# Cmd\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Tabs and spaces
	name, found := sr.IsSkillCommand("/cmd\targ1 \t arg2")
	if !found {
		t.Error("expected command to handle tabs and spaces")
	}
	if name != "cmd" {
		t.Errorf("expected 'cmd', got %q", name)
	}
}

// TestSkillsEdge_FormatSkillPrompt_EmptyUserInput tests formatting with empty user input.
func TestSkillsEdge_FormatSkillPrompt_EmptyUserInput(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "Instruction content",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "")

	if !strings.Contains(result, "Instruction content") {
		t.Error("expected skill content in result")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected separator in result")
	}
	// Empty input should still appear (as empty string after separator)
	if !strings.HasSuffix(strings.TrimSpace(result), "---") {
		t.Error("expected result to end with separator when user input is empty")
	}
}

// TestSkillsEdge_FormatSkillPrompt_OnlyWhitespaceUserInput tests with whitespace-only input.
func TestSkillsEdge_FormatSkillPrompt_OnlyWhitespaceUserInput(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "Instruction",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "   \n  \t  ")

	if !strings.Contains(result, "Instruction") {
		t.Error("expected skill content")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected separator")
	}
}

// TestSkillsEdge_GetNilPointer tests that Get returns a pointer that can be safely used.
func TestSkillsEdge_GetNilPointer(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "valid.md")
	if err := os.WriteFile(skillPath, []byte("# Valid\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Get a valid skill and verify it's not nil
	skill, found := sr.Get("valid")
	if !found || skill == nil {
		t.Fatal("expected valid skill to be found and non-nil")
	}

	// Verify fields are accessible
	_ = skill.Name
	_ = skill.Description
	_ = skill.Content
	_ = skill.FilePath
}

// TestSkillsEdge_ListIsIndependent tests that List returns truly independent copies.
func TestSkillsEdge_ListIsIndependent(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "skill.md")
	if err := os.WriteFile(skillPath, []byte("# Skill\nOriginal"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Get first list
	list1 := sr.List()
	originalName := list1[0].Name

	// Modify the slice
	list1[0].Name = "modified"
	list1[0].Description = "modified"
	list1[0].Content = "modified"

	// Get second list and verify it's not affected
	list2 := sr.List()
	if list2[0].Name != originalName {
		t.Error("expected second list to have original values")
	}
	if list2[0].Description == "modified" {
		t.Error("expected second list description to be original")
	}
}

// TestSkillsEdge_ReloadClearsOldSkills tests that Load() completely replaces old skills.
func TestSkillsEdge_ReloadClearsOldSkills(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create initial skill
	skill1Path := filepath.Join(tmpDir, "skill1.md")
	if err := os.WriteFile(skill1Path, []byte("# Skill 1\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill1: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	if len(sr.List()) != 1 {
		t.Fatal("expected 1 skill initially")
	}

	// Delete skill1 and create skill2
	if err := os.Remove(skill1Path); err != nil {
		t.Fatalf("failed to remove skill1: %v", err)
	}
	skill2Path := filepath.Join(tmpDir, "skill2.md")
	if err := os.WriteFile(skill2Path, []byte("# Skill 2\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill2: %v", err)
	}

	// Reload
	sr.Load()

	// Verify only skill2 exists
	if len(sr.List()) != 1 {
		t.Errorf("expected 1 skill after reload, got %d", len(sr.List()))
	}

	_, found := sr.Get("skill1")
	if found {
		t.Error("expected skill1 to be gone after reload")
	}

	skill2, found := sr.Get("skill2")
	if !found {
		t.Error("expected skill2 to be found")
	}
	if skill2 == nil {
		t.Fatal("expected skill2 to be non-nil")
	}
}

// TestSkillsEdge_SkillFilepathPreserved tests that file path is correctly preserved.
func TestSkillsEdge_SkillFilepathPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "filepath-test.md")
	if err := os.WriteFile(skillPath, []byte("# Test\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("filepath-test")

	if !found {
		t.Fatal("expected skill to be found")
	}
	if skill.FilePath != skillPath {
		t.Errorf("expected FilePath %q, got %q", skillPath, skill.FilePath)
	}
}

// TestSkillsEdge_RegistryWithEmptyDirString tests registry created with empty dir string.
func TestSkillsEdge_RegistryWithEmptyDirString(t *testing.T) {
	sr := NewSkillRegistry("")

	if sr == nil {
		t.Error("expected non-nil registry")
	}
	if sr.dir != "" {
		t.Errorf("expected empty dir, got %q", sr.dir)
	}

	skills := sr.List()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}

	// Load should succeed on empty dir
	if err := sr.Load(); err != nil {
		t.Errorf("expected Load to succeed on empty dir, got error: %v", err)
	}
}

// TestSkillsEdge_MdFileWithoutExtension tests that files without .md are skipped.
func TestSkillsEdge_SkipNonMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	files := map[string]string{
		"skill.md":    "# Markdown\nContent",
		"skill.txt":   "Not markdown",
		"skill.MD":    "Wrong case",
		"skill":       "No extension",
		"skillmd":     "No dot",
		"skill.md.bak": "Backup file",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	sr := NewSkillRegistry(tmpDir)
	skills := sr.List()

	// Only skill.md should be loaded
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if len(skills) > 0 && skills[0].Name != "skill" {
		t.Errorf("expected skill name 'skill', got %q", skills[0].Name)
	}
}

// TestSkillsEdge_ParseSkillFile_OnlyNewlines tests parsing file with only newlines.
func TestSkillsEdge_ParseSkillFile_OnlyNewlines(t *testing.T) {
	name := "newlines"
	filePath := "/tmp/newlines.md"
	raw := "\n\n\n\n"

	skill := parseSkillFile(name, filePath, raw)

	if skill.Name != "newlines" {
		t.Errorf("expected name 'newlines', got %q", skill.Name)
	}
	if skill.Description != "" {
		t.Errorf("expected empty description, got %q", skill.Description)
	}
	if skill.Content != "" {
		t.Errorf("expected empty content, got %q", skill.Content)
	}
}

// TestSkillsEdge_ParseSkillFile_WindowsLineEndings tests parsing with CRLF line endings.
func TestSkillsEdge_ParseSkillFile_WindowsLineEndings(t *testing.T) {
	name := "windows"
	filePath := "/tmp/windows.md"
	raw := "# Windows Heading\r\nContent line 1\r\nContent line 2"

	skill := parseSkillFile(name, filePath, raw)

	if skill.Description != "Windows Heading" {
		t.Errorf("expected description 'Windows Heading', got %q", skill.Description)
	}
	// Content should preserve structure even with CRLF
	if !strings.Contains(skill.Content, "Content line 1") {
		t.Error("expected content to contain 'Content line 1'")
	}
}
