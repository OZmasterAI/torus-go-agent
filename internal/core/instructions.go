package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadReason describes why an instruction file was loaded.
type LoadReason string

const (
	LoadReasonSessionStart LoadReason = "session_start"
	LoadReasonCompact      LoadReason = "compact"
	LoadReasonInclude      LoadReason = "include"
	LoadReasonFileChange   LoadReason = "file_change"
)

// MemoryType describes the tier of an instruction file.
type MemoryType string

const (
	MemoryTypeManaged MemoryType = "managed"
	MemoryTypeUser    MemoryType = "user"
	MemoryTypeProject MemoryType = "project"
	MemoryTypeLocal   MemoryType = "local"
)

// InstructionFile represents a loaded instruction file.
type InstructionFile struct {
	Path       string
	Content    string
	MemType    MemoryType
	Paths      []string   // frontmatter paths: globs for conditional activation
	LoadReason LoadReason
}

// DiscoverInstructionFiles walks from cwd up to the git root (or filesystem root),
// collecting instruction files in priority order (lowest priority first).
// It checks these locations at each directory level:
//   - TORUS.md
//   - .torus/TORUS.md
//   - .torus/rules/*.md
//
// Additionally checks:
//   - User-global: ~/.config/torus/TORUS.md and ~/.config/torus/rules/*.md
//
// Files closer to cwd have higher priority (appear later in the slice).
func DiscoverInstructionFiles(cwd string) []InstructionFile {
	var files []InstructionFile

	// 1. User-global instructions.
	home, err := os.UserHomeDir()
	if err == nil {
		userDir := filepath.Join(home, ".config", "torus")
		files = append(files, discoverInDir(userDir, MemoryTypeUser)...)
	}

	// 2. Walk from git root (or filesystem root) down to cwd.
	// Collect directories from cwd upward, then reverse for correct priority order.
	var dirs []string
	dir := filepath.Clean(cwd)
	for {
		dirs = append(dirs, dir)
		if isGitRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // filesystem root
		}
		dir = parent
	}
	// Reverse: root-most first (lowest priority), cwd last (highest priority).
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	for _, d := range dirs {
		files = append(files, discoverInDir(d, MemoryTypeProject)...)
	}

	// 3. Local override: TORUS.local.md in cwd only.
	localPath := filepath.Join(cwd, "TORUS.local.md")
	if content, err := os.ReadFile(localPath); err == nil {
		files = append(files, InstructionFile{
			Path:    localPath,
			Content: string(content),
			MemType: MemoryTypeLocal,
		})
	}

	return files
}

// discoverInDir finds instruction files in a single directory.
func discoverInDir(dir string, memType MemoryType) []InstructionFile {
	var files []InstructionFile

	// TORUS.md in the directory itself.
	if content, err := os.ReadFile(filepath.Join(dir, "TORUS.md")); err == nil {
		files = append(files, InstructionFile{
			Path:    filepath.Join(dir, "TORUS.md"),
			Content: string(content),
			MemType: memType,
		})
	}

	// .torus/TORUS.md
	torusDir := filepath.Join(dir, ".torus")
	if content, err := os.ReadFile(filepath.Join(torusDir, "TORUS.md")); err == nil {
		files = append(files, InstructionFile{
			Path:    filepath.Join(torusDir, "TORUS.md"),
			Content: string(content),
			MemType: memType,
		})
	}

	// .torus/rules/*.md
	rulesDir := filepath.Join(torusDir, "rules")
	entries, err := os.ReadDir(rulesDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(rulesDir, e.Name())
			if content, err := os.ReadFile(path); err == nil {
				files = append(files, InstructionFile{
					Path:    path,
					Content: string(content),
					MemType: memType,
				})
			}
		}
	}

	return files
}

// isGitRoot checks if dir contains a .git directory or file.
func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// ParseFrontmatter extracts YAML frontmatter (between --- delimiters) from content.
// Returns the parsed paths and the content with frontmatter stripped.
// Uses a simple line parser instead of a YAML library to avoid external dependencies.
func ParseFrontmatter(content string) (paths []string, body string) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil, content
	}
	fmRaw := content[4 : 4+end]
	body = strings.TrimLeft(content[4+end+4:], "\n")

	// Simple YAML list parser for paths: field only.
	for _, line := range strings.Split(fmRaw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "paths:") {
			continue // The "paths:" header line
		}
		if strings.HasPrefix(line, "- ") {
			paths = append(paths, strings.TrimSpace(line[2:]))
		}
	}
	return paths, body
}

// ExpandIncludes processes @include directives in content.
// Lines starting with @ (not inside code blocks) are treated as file references.
// Paths are resolved relative to the directory of the source file.
func ExpandIncludes(content string, sourceDir string, seen map[string]bool) string {
	if seen == nil {
		seen = make(map[string]bool)
	}
	var result strings.Builder
	inCodeBlock := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
		}
		if inCodeBlock || !strings.HasPrefix(line, "@") {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		// Parse @include path.
		ref := strings.TrimSpace(line[1:])
		if ref == "" {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		// Resolve path.
		ref = expandHome(ref)
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(sourceDir, ref)
		}
		ref = filepath.Clean(ref)
		if seen[ref] {
			// Circular reference -- skip.
			continue
		}
		seen[ref] = true
		data, err := os.ReadFile(ref)
		if err != nil {
			// Non-existent files silently ignored.
			continue
		}
		// Recursively expand includes in the included file.
		expanded := ExpandIncludes(string(data), filepath.Dir(ref), seen)
		result.WriteString(expanded)
		if !strings.HasSuffix(expanded, "\n") {
			result.WriteByte('\n')
		}
	}
	return result.String()
}

// expandHome replaces a leading ~/ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// LoadAndParseAll loads all discovered instruction files, parses frontmatter,
// expands includes, and sets the load reason.
func LoadAndParseAll(cwd string, reason LoadReason) []InstructionFile {
	raw := DiscoverInstructionFiles(cwd)
	result := make([]InstructionFile, 0, len(raw))
	for _, f := range raw {
		paths, body := ParseFrontmatter(f.Content)
		expanded := ExpandIncludes(body, filepath.Dir(f.Path), nil)
		result = append(result, InstructionFile{
			Path:       f.Path,
			Content:    expanded,
			MemType:    f.MemType,
			Paths:      paths,
			LoadReason: reason,
		})
	}
	return result
}

// BuildPrompt concatenates instruction files into a single prompt string.
// If activeFiles is non-empty, conditional rules (those with Paths set) are
// only included if at least one of their path globs matches an active file.
// Files without Paths are always included.
func BuildPrompt(files []InstructionFile, activeFiles []string) string {
	var parts []string
	for _, f := range files {
		if len(f.Paths) > 0 && len(activeFiles) > 0 {
			if !matchesAnyGlob(f.Paths, activeFiles) {
				continue
			}
		} else if len(f.Paths) > 0 && len(activeFiles) == 0 {
			// Conditional rule but no active files context -- skip.
			continue
		}
		if f.Content != "" {
			parts = append(parts, f.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// matchesAnyGlob returns true if any of the globs match any of the files.
func matchesAnyGlob(globs []string, files []string) bool {
	for _, g := range globs {
		for _, f := range files {
			if matched, _ := filepath.Match(g, f); matched {
				return true
			}
			// Also try matching just the filename.
			if matched, _ := filepath.Match(g, filepath.Base(f)); matched {
				return true
			}
		}
	}
	return false
}
