package features

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNewSkillRegistry_EmptyDir tests creating a registry with a non-existent directory.
func TestNewSkillRegistry_EmptyDir(t *testing.T) {
	sr := NewSkillRegistry("/nonexistent/path")
	if sr == nil {
		t.Error("expected non-nil registry")
	}
	if sr.dir != "/nonexistent/path" {
		t.Errorf("expected dir to be /nonexistent/path, got %q", sr.dir)
	}
	skills := sr.List()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// TestNewSkillRegistry_ValidDir tests creating a registry with a valid directory.
func TestNewSkillRegistry_ValidDir(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create a test skill file
	skillPath := filepath.Join(tmpDir, "test-skill.md")
	content := "# Test Skill\nThis is test content."
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	if sr == nil {
		t.Error("expected non-nil registry")
	}
	if sr.dir != tmpDir {
		t.Errorf("expected dir to be %s, got %q", tmpDir, sr.dir)
	}

	skills := sr.List()
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got %q", skills[0].Name)
	}
}

// TestLoad_MultipleSkills tests loading multiple skill files from a directory.
func TestLoad_MultipleSkills(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create several skill files
	skills := map[string]string{
		"brainstorm.md":  "# Brainstorm\nGenerate ideas for the task.",
		"implement.md":   "# Implement\nWrite code to solve the problem.",
		"review.md":      "# Review\nReview the implementation.",
		"not-markdown.txt": "This should be ignored",
	}

	for name, content := range skills {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file %s: %v", name, err)
		}
	}

	sr := NewSkillRegistry(tmpDir)
	loaded := sr.List()

	// Should load 3 .md files, ignore .txt
	if len(loaded) != 3 {
		t.Errorf("expected 3 skills, got %d", len(loaded))
	}

	// Verify expected skills are present
	skillNames := make(map[string]bool)
	for _, s := range loaded {
		skillNames[s.Name] = true
	}

	expected := []string{"brainstorm", "implement", "review"}
	for _, name := range expected {
		if !skillNames[name] {
			t.Errorf("expected skill %q not found", name)
		}
	}

	if skillNames["not-markdown"] {
		t.Error("expected .txt file to be ignored")
	}
}

// TestLoad_EmptyDirectory tests loading from an empty directory.
func TestLoad_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	sr := NewSkillRegistry(tmpDir)
	skills := sr.List()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills in empty dir, got %d", len(skills))
	}
}

// TestLoad_SubdirectoryIgnored tests that subdirectories are ignored.
func TestLoad_SubdirectoryIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create a skill file
	skillPath := filepath.Join(tmpDir, "skill.md")
	if err := os.WriteFile(skillPath, []byte("# Skill\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	// Create a subdirectory that should be ignored
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	subSkillPath := filepath.Join(subDir, "sub-skill.md")
	if err := os.WriteFile(subSkillPath, []byte("# Sub\nContent"), 0644); err != nil {
		t.Fatalf("failed to write sub skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skills := sr.List()
	if len(skills) != 1 {
		t.Errorf("expected 1 skill (subdir ignored), got %d", len(skills))
	}
	if skills[0].Name != "skill" {
		t.Errorf("expected skill name 'skill', got %q", skills[0].Name)
	}
}

// TestLoad_UnreadableFileSkipped tests that unreadable files are skipped.
func TestLoad_UnreadableFileSkipped(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create a readable skill
	skillPath := filepath.Join(tmpDir, "readable.md")
	if err := os.WriteFile(skillPath, []byte("# Readable\nContent"), 0644); err != nil {
		t.Fatalf("failed to write readable skill: %v", err)
	}

	// Create an unreadable file
	unreadablePath := filepath.Join(tmpDir, "unreadable.md")
	if err := os.WriteFile(unreadablePath, []byte("# Unreadable\nContent"), 0644); err != nil {
		t.Fatalf("failed to write unreadable skill: %v", err)
	}
	if err := os.Chmod(unreadablePath, 0000); err != nil {
		t.Fatalf("failed to chmod unreadable file: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skills := sr.List()

	// Should load the readable one, skip the unreadable one
	if len(skills) != 1 {
		t.Errorf("expected 1 skill (unreadable skipped), got %d", len(skills))
	}
	if skills[0].Name != "readable" {
		t.Errorf("expected skill name 'readable', got %q", skills[0].Name)
	}

	// Clean up
	os.Chmod(unreadablePath, 0644)
}

// TestLoad_Reload tests that calling Load() again replaces previous skills.
func TestLoad_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create initial skill
	skillPath := filepath.Join(tmpDir, "skill1.md")
	if err := os.WriteFile(skillPath, []byte("# Skill 1\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill1: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skills := sr.List()
	if len(skills) != 1 {
		t.Errorf("initial load: expected 1 skill, got %d", len(skills))
	}

	// Add a new skill and remove the old one
	os.Remove(skillPath)
	skill2Path := filepath.Join(tmpDir, "skill2.md")
	if err := os.WriteFile(skill2Path, []byte("# Skill 2\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill2: %v", err)
	}

	// Reload
	if err := sr.Load(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	skills = sr.List()
	if len(skills) != 1 {
		t.Errorf("after reload: expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "skill2" {
		t.Errorf("expected skill name 'skill2', got %q", skills[0].Name)
	}
}

// TestList_ReturnsCopy tests that List returns a snapshot, not a reference.
func TestList_ReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "skill.md")
	if err := os.WriteFile(skillPath, []byte("# Skill\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)

	// Get two lists and verify they're independent
	list1 := sr.List()
	list2 := sr.List()

	if len(list1) != len(list2) {
		t.Errorf("lists should have same length: %d vs %d", len(list1), len(list2))
	}

	// Modify list1; list2 should not be affected
	if len(list1) > 0 {
		list1[0].Name = "modified"
		if list2[0].Name == "modified" {
			t.Error("modifying list1 should not affect list2")
		}
	}
}

// TestGet_Found tests retrieving a skill that exists.
func TestGet_Found(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "brainstorm.md")
	content := "# Brainstorm\nGenerate ideas for the task."
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("brainstorm")

	if !found {
		t.Error("expected skill to be found")
	}
	if skill == nil {
		t.Error("expected skill to be non-nil")
	}
	if skill.Name != "brainstorm" {
		t.Errorf("expected name 'brainstorm', got %q", skill.Name)
	}
	if !strings.Contains(skill.Description, "Brainstorm") {
		t.Errorf("expected description to contain 'Brainstorm', got %q", skill.Description)
	}
}

// TestGet_NotFound tests retrieving a skill that doesn't exist.
func TestGet_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	sr := NewSkillRegistry(tmpDir)
	skill, found := sr.Get("nonexistent")

	if found {
		t.Error("expected skill not to be found")
	}
	if skill != nil {
		t.Error("expected skill to be nil")
	}
}

// TestParseSkillFile_WithDescription tests parsing a skill file with a description.
func TestParseSkillFile_WithDescription(t *testing.T) {
	name := "test-skill"
	filePath := "/tmp/test-skill.md"
	raw := `# Test Skill Description
This is the content of the skill.
It can span multiple lines.`

	skill := parseSkillFile(name, filePath, raw)

	if skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", skill.Name)
	}
	if skill.Description != "Test Skill Description" {
		t.Errorf("expected description 'Test Skill Description', got %q", skill.Description)
	}
	if !strings.Contains(skill.Content, "This is the content") {
		t.Errorf("expected content to contain 'This is the content', got %q", skill.Content)
	}
	if skill.FilePath != filePath {
		t.Errorf("expected filePath %q, got %q", filePath, skill.FilePath)
	}
}

// TestParseSkillFile_MultipleHeadings tests parsing when there are multiple # headings.
func TestParseSkillFile_MultipleHeadings(t *testing.T) {
	name := "multi"
	filePath := "/tmp/multi.md"
	raw := `# First Heading
Some intro.

# Second Heading
This should be in content.`

	skill := parseSkillFile(name, filePath, raw)

	// Should use first heading as description
	if skill.Description != "First Heading" {
		t.Errorf("expected description 'First Heading', got %q", skill.Description)
	}

	// Should include second heading in content
	if !strings.Contains(skill.Content, "# Second Heading") {
		t.Error("expected second heading to be in content")
	}
}

// TestParseSkillFile_NoHeading tests parsing a file without a # heading.
func TestParseSkillFile_NoHeading(t *testing.T) {
	name := "no-heading"
	filePath := "/tmp/no-heading.md"
	raw := `This is content without a heading.
More content.`

	skill := parseSkillFile(name, filePath, raw)

	if skill.Name != "no-heading" {
		t.Errorf("expected name 'no-heading', got %q", skill.Name)
	}
	if skill.Description != "" {
		t.Errorf("expected empty description, got %q", skill.Description)
	}
	if !strings.Contains(skill.Content, "This is content") {
		t.Error("expected content to contain the text")
	}
}

// TestParseSkillFile_EmptyFile tests parsing an empty file.
func TestParseSkillFile_EmptyFile(t *testing.T) {
	name := "empty"
	filePath := "/tmp/empty.md"
	raw := ""

	skill := parseSkillFile(name, filePath, raw)

	if skill.Name != "empty" {
		t.Errorf("expected name 'empty', got %q", skill.Name)
	}
	if skill.Description != "" {
		t.Errorf("expected empty description, got %q", skill.Description)
	}
	if skill.Content != "" {
		t.Errorf("expected empty content, got %q", skill.Content)
	}
}

// TestParseSkillFile_LeadingBlankLines tests that leading blank lines are trimmed from content.
func TestParseSkillFile_LeadingBlankLines(t *testing.T) {
	name := "blank-lines"
	filePath := "/tmp/blank-lines.md"
	raw := `# Heading


Real content here.`

	skill := parseSkillFile(name, filePath, raw)

	if skill.Description != "Heading" {
		t.Errorf("expected description 'Heading', got %q", skill.Description)
	}

	// Content should not have leading blank lines
	if strings.HasPrefix(skill.Content, "\n") {
		t.Error("expected leading blank lines to be trimmed")
	}
	if !strings.HasPrefix(strings.TrimSpace(skill.Content), "Real content") {
		t.Errorf("expected content to start with 'Real content', got %q", skill.Content)
	}
}

// TestParseSkillFile_WhitespaceInHeading tests heading with varying whitespace.
func TestParseSkillFile_WhitespaceInHeading(t *testing.T) {
	name := "space-heading"
	filePath := "/tmp/space-heading.md"
	raw := `###   Spaced Heading
Content here.`

	skill := parseSkillFile(name, filePath, raw)

	// Should trim whitespace around description
	if skill.Description != "Spaced Heading" {
		t.Errorf("expected description 'Spaced Heading', got %q", skill.Description)
	}
}

// TestIsSkillCommand_ValidCommand tests a valid slash command.
func TestIsSkillCommand_ValidCommand(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "brainstorm.md")
	if err := os.WriteFile(skillPath, []byte("# Brainstorm\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("/brainstorm")

	if !found {
		t.Error("expected command to be found")
	}
	if name != "brainstorm" {
		t.Errorf("expected name 'brainstorm', got %q", name)
	}
}

// TestIsSkillCommand_WithArguments tests a command with arguments.
func TestIsSkillCommand_WithArguments(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(skillPath, []byte("# Test\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("/test some arguments here")

	if !found {
		t.Error("expected command to be found")
	}
	if name != "test" {
		t.Errorf("expected name 'test', got %q", name)
	}
}

// TestIsSkillCommand_NotFound tests a command that doesn't exist.
func TestIsSkillCommand_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("/nonexistent")

	if found {
		t.Error("expected command not to be found")
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

// TestIsSkillCommand_NoSlash tests input without a slash.
func TestIsSkillCommand_NoSlash(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(skillPath, []byte("# Test\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("test")

	if found {
		t.Error("expected command not to be found without slash")
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

// TestIsSkillCommand_JustSlash tests input that's just a slash.
func TestIsSkillCommand_JustSlash(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("/")

	if found {
		t.Error("expected command not to be found for just /")
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

// TestIsSkillCommand_WithWhitespace tests a command with leading/trailing whitespace.
func TestIsSkillCommand_WithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(skillPath, []byte("# Test\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill: %v", err)
	}

	sr := NewSkillRegistry(tmpDir)
	name, found := sr.IsSkillCommand("  /test  arguments  ")

	if !found {
		t.Error("expected command to be found")
	}
	if name != "test" {
		t.Errorf("expected name 'test', got %q", name)
	}
}

// TestFormatSkillPrompt_BasicFormat tests formatting skill prompt with content and user input.
func TestFormatSkillPrompt_BasicFormat(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "This is the skill instruction.",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "user wants to do this")

	if !strings.Contains(result, "This is the skill instruction") {
		t.Error("expected result to contain skill content")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected result to contain separator")
	}
	if !strings.Contains(result, "user wants to do this") {
		t.Error("expected result to contain user input")
	}
}

// TestFormatSkillPrompt_NoTrailingNewline tests that a newline is added if content doesn't have one.
func TestFormatSkillPrompt_NoTrailingNewline(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "Content without newline",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "user input")

	// Should have newline between content and separator
	if !strings.Contains(result, "Content without newline\n") {
		t.Error("expected newline to be added after content")
	}
}

// TestFormatSkillPrompt_WithTrailingNewline tests content that already has a newline.
func TestFormatSkillPrompt_WithTrailingNewline(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "Content with newline\n",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "user input")

	// Should not duplicate newlines
	lines := strings.Split(result, "\n")
	separatorIndex := -1
	for i, line := range lines {
		if line == "---" {
			separatorIndex = i
			break
		}
	}

	if separatorIndex == -1 {
		t.Error("expected separator to be found")
	}

	// Check structure
	if !strings.HasPrefix(result, "Content with newline\n") {
		t.Error("content should be preserved")
	}
}

// TestFormatSkillPrompt_EmptyContent tests formatting with empty skill content.
func TestFormatSkillPrompt_EmptyContent(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "",
	}

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, "user input")

	if !strings.Contains(result, "---") {
		t.Error("expected separator even with empty content")
	}
	if !strings.Contains(result, "user input") {
		t.Error("expected user input in result")
	}
}

// TestFormatSkillPrompt_MultilineInput tests with multiline user input.
func TestFormatSkillPrompt_MultilineInput(t *testing.T) {
	skill := &Skill{
		Name:    "test",
		Content: "Instruction",
	}

	userInput := `Line 1
Line 2
Line 3`

	sr := &SkillRegistry{skills: make(map[string]*Skill)}
	result := sr.FormatSkillPrompt(skill, userInput)

	if !strings.Contains(result, "Line 1\nLine 2\nLine 3") {
		t.Error("expected multiline user input to be preserved")
	}
}

// TestSkillRegistry_IntegrationFlow tests a complete workflow.
func TestSkillRegistry_IntegrationFlow(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	// Create multiple skill files
	skills := map[string]string{
		"brainstorm.md": `# Brainstorm
Generate creative ideas for the given topic.`,
		"implement.md": `# Implement
Write code to implement the solution.`,
		"review.md": `# Review Code
Review the implementation for quality.`,
	}

	for name, content := range skills {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	// Create registry and load
	sr := NewSkillRegistry(tmpDir)

	// Verify all skills loaded
	loaded := sr.List()
	if len(loaded) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(loaded))
	}

	// Verify Get works
	skill, found := sr.Get("brainstorm")
	if !found {
		t.Fatal("expected brainstorm to be found")
	}
	if !strings.Contains(skill.Description, "Brainstorm") {
		t.Errorf("unexpected description: %q", skill.Description)
	}

	// Verify IsSkillCommand works
	name, found := sr.IsSkillCommand("/implement with args")
	if !found || name != "implement" {
		t.Errorf("expected /implement to match, got found=%v name=%q", found, name)
	}

	// Verify FormatSkillPrompt works
	formatted := sr.FormatSkillPrompt(skill, "brainstorm about AI safety")
	if !strings.Contains(formatted, "Generate creative ideas") {
		t.Error("expected skill content in formatted prompt")
	}
	if !strings.Contains(formatted, "brainstorm about AI safety") {
		t.Error("expected user input in formatted prompt")
	}
	if !strings.Contains(formatted, "---") {
		t.Error("expected separator in formatted prompt")
	}

	// Sort loaded skills by name for deterministic comparison
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].Name < loaded[j].Name
	})

	// Verify all expected skills are present
	expectedNames := []string{"brainstorm", "implement", "review"}
	for i, expected := range expectedNames {
		if loaded[i].Name != expected {
			t.Errorf("skill %d: expected %q, got %q", i, expected, loaded[i].Name)
		}
	}
}
