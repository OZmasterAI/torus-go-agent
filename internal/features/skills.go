// Package features provides optional agent capabilities: skill loading and sub-agent management.
package features

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded .md skill file that maps to a /command.
type Skill struct {
	Name        string
	Description string
	Content     string
	FilePath    string
}

// SkillRegistry loads and indexes .md skill files from a directory.
type SkillRegistry struct {
	skills map[string]*Skill
	dir    string
}

// NewSkillRegistry creates a SkillRegistry pointing at skillsDir and calls Load immediately.
// If the directory does not exist, the registry is returned empty (no error).
func NewSkillRegistry(skillsDir string) *SkillRegistry {
	sr := &SkillRegistry{
		skills: make(map[string]*Skill),
		dir:    skillsDir,
	}
	if err := sr.Load(); err != nil {
		log.Printf("[skills] warning: initial load from %q: %v", skillsDir, err)
	}
	return sr
}

// Load scans the registry directory for *.md files and populates the registry.
// Each file becomes a skill whose name is the filename without the .md extension.
// The first line starting with '#' is used as the description (stripped of leading '#' and spaces).
// The remaining content is stored verbatim as Content.
// Calling Load() again replaces all previously loaded skills.
func (sr *SkillRegistry) Load() error {
	if sr.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(sr.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skills: read dir %q: %w", sr.dir, err)
	}

	fresh := make(map[string]*Skill, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fp := filepath.Join(sr.dir, e.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			// Skip unreadable files; don't abort the whole load.
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skill := parseSkillFile(name, fp, string(data))
		fresh[name] = skill
	}

	sr.skills = fresh
	return nil
}

// parseSkillFile extracts description (first # heading) and content from raw text.
func parseSkillFile(name, filePath, raw string) *Skill {
	lines := strings.Split(raw, "\n")
	var description string
	var contentLines []string
	descFound := false

	for _, line := range lines {
		if !descFound && strings.HasPrefix(line, "#") {
			// Strip leading '#' characters and surrounding whitespace.
			description = strings.TrimSpace(strings.TrimLeft(line, "#"))
			descFound = true
			continue
		}
		contentLines = append(contentLines, line)
	}

	// Trim leading blank lines from content.
	for len(contentLines) > 0 && strings.TrimSpace(contentLines[0]) == "" {
		contentLines = contentLines[1:]
	}

	return &Skill{
		Name:        name,
		Description: description,
		Content:     strings.Join(contentLines, "\n"),
		FilePath:    filePath,
	}
}

// Get returns the skill with the given name and whether it was found.
func (sr *SkillRegistry) Get(name string) (*Skill, bool) {
	s, ok := sr.skills[name]
	return s, ok
}

// List returns a snapshot of all loaded skills in no guaranteed order.
func (sr *SkillRegistry) List() []Skill {
	out := make([]Skill, 0, len(sr.skills))
	for _, s := range sr.skills {
		out = append(out, *s)
	}
	return out
}

// IsSkillCommand checks whether input is a slash-command matching a loaded skill.
// Input like "/brainstorm" matches a skill named "brainstorm".
// The input may include arguments after the command name; only the first token is matched.
// Returns the skill name (without slash) and true on a match.
func (sr *SkillRegistry) IsSkillCommand(input string) (name string, ok bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return "", false
	}
	// Extract the command token (up to the first whitespace after the slash).
	token := strings.TrimPrefix(trimmed, "/")
	fields := strings.Fields(token)
	if len(fields) == 0 {
		return "", false
	}
	candidate := fields[0]
	if _, found := sr.skills[candidate]; found {
		return candidate, true
	}
	return "", false
}

// FormatSkillPrompt returns the skill's Content with the user's raw input appended.
// This combined string is sent as the user message to the agent, so the skill
// instructions frame the actual task the user typed.
func (sr *SkillRegistry) FormatSkillPrompt(skill *Skill, userInput string) string {
	var sb strings.Builder
	sb.WriteString(skill.Content)
	if skill.Content != "" && !strings.HasSuffix(skill.Content, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("\n---\n")
	sb.WriteString(userInput)
	return sb.String()
}
