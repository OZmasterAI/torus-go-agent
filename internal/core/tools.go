package core

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var secretPatterns = []struct {
	P *regexp.Regexp
	D string
}{
	{regexp.MustCompile(`(?i)(?:api[_\-]?key|apikey)\s*[=:]\s*["']?[A-Za-z0-9\-_]{16,}["']?`), "API key"},
	{regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`), "Secret key"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS key"},
	{regexp.MustCompile(`(?i)(?:password|passwd|secret|token)\s*[=:]\s*["'][^"'${}]{6,}["']`), "Credential"},
	{regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`), "Private key"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), "GitHub PAT"},
}

// ScanSecrets checks content for hardcoded secrets.
func ScanSecrets(content string) (string, bool) {
	for _, s := range secretPatterns {
		if s.P.MatchString(content) {
			m := s.P.FindString(content)
			if len(m) > 12 {
				m = m[:12] + "..."
			}
			return s.D + ": " + m, true
		}
	}
	return "", false
}

var dangerPatterns = []struct {
	L string
	P *regexp.Regexp
}{
	{"rm-rf-root", regexp.MustCompile(`\brm\s+-[^\s]*r[^\s]*f[^\s]*\s+/`)},
	{"no-preserve-root", regexp.MustCompile(`\brm\b[^|&;]*--no-preserve-root`)},
	{"fork-bomb", regexp.MustCompile(`:\(\)\s*\{[^}]*:\s*\|\s*:`)},
	{"mkfs", regexp.MustCompile(`\bmkfs(?:\.[a-z0-9]+)?\s`)},
	{"sysrq", regexp.MustCompile(`/proc/sysrq-trigger`)},
}

// CheckSafety returns a label and true if the command is dangerous.
func CheckSafety(cmd string) (string, bool) {
	for _, d := range dangerPatterns {
		if d.P.MatchString(cmd) {
			return d.L, true
		}
	}
	return "", false
}

// BuildDefaultTools returns the 6 standard tools.
func BuildDefaultTools() []Tool {
	return []Tool{bashTool(), readTool(), writeTool(), editTool(), globTool(), grepTool()}
}

func bashTool() Tool {
	return Tool{
		Name:        "bash",
		Description: "Run a shell command. Returns stdout+stderr. 30s timeout.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to run"},
				"cwd":     map[string]any{"type": "string", "description": "Working directory"},
			},
			"required": []string{"command"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			command, _ := args["command"].(string)
			cwd, _ := args["cwd"].(string)
			if label, bad := CheckSafety(command); bad {
				return &ToolResult{Content: "[BLOCKED] " + label, IsError: true}, nil
			}
			cmd := osexec.Command("bash", "-c", command)
			if cwd != "" {
				cmd.Dir = cwd
			}
			done := make(chan struct{})
			var out []byte
			var cmdErr error
			go func() { out, cmdErr = cmd.CombinedOutput(); close(done) }()
			select {
			case <-done:
				t := string(out)
				if t == "" {
					t = "(no output)"
				}
				return &ToolResult{Content: t, IsError: cmdErr != nil}, nil
			case <-time.After(30 * time.Second):
				cmd.Process.Kill()
				return &ToolResult{Content: "Timed out (30s)", IsError: true}, nil
			}
		},
	}
}

func readTool() Tool {
	return Tool{
		Name:        "read",
		Description: "Read file contents with optional offset and limit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Absolute file path"},
				"offset":    map[string]any{"type": "number", "description": "Line offset (0-based)"},
				"limit":     map[string]any{"type": "number", "description": "Max lines to read"},
			},
			"required": []string{"file_path"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			fp, _ := args["file_path"].(string)
			data, err := os.ReadFile(fp)
			if err != nil {
				return &ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			lines := strings.Split(string(data), "\n")
			off := int(GF(args, "offset", 0))
			lim := int(GF(args, "limit", float64(len(lines))))
			end := off + lim
			if end > len(lines) {
				end = len(lines)
			}
			var out []string
			for i := off; i < end; i++ {
				out = append(out, fmt.Sprintf("%6d %s", i+1, lines[i]))
			}
			return &ToolResult{Content: strings.Join(out, "\n")}, nil
		},
	}
}

func writeTool() Tool {
	return Tool{
		Name:        "write",
		Description: "Write content to a file. Creates parent directories.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Absolute file path"},
				"content":   map[string]any{"type": "string", "description": "File content to write"},
			},
			"required": []string{"file_path", "content"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			fp, _ := args["file_path"].(string)
			c, _ := args["content"].(string)
			os.MkdirAll(filepath.Dir(fp), 0755)
			if err := os.WriteFile(fp, []byte(c), 0644); err != nil {
				return &ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			return &ToolResult{Content: fmt.Sprintf("Wrote %d lines to %s", strings.Count(c, "\n")+1, fp)}, nil
		},
	}
}

func editTool() Tool {
	return Tool{
		Name:        "edit",
		Description: "Replace an exact string in a file.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "Absolute file path"},
				"old_str":     map[string]any{"type": "string", "description": "Text to find"},
				"new_str":     map[string]any{"type": "string", "description": "Replacement text"},
				"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences"},
			},
			"required": []string{"file_path", "old_str", "new_str"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			fp, _ := args["file_path"].(string)
			old, _ := args["old_str"].(string)
			nw, _ := args["new_str"].(string)
			all, _ := args["replace_all"].(bool)
			data, err := os.ReadFile(fp)
			if err != nil {
				return &ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			c := string(data)
			if !strings.Contains(c, old) {
				return &ToolResult{Content: "Error: old_str not found", IsError: true}, nil
			}
			if all {
				c = strings.ReplaceAll(c, old, nw)
			} else {
				c = strings.Replace(c, old, nw, 1)
			}
			os.WriteFile(fp, []byte(c), 0644)
			return &ToolResult{Content: "Edited " + fp}, nil
		},
	}
}

func globTool() Tool {
	return Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern"},
				"cwd":     map[string]any{"type": "string", "description": "Base directory"},
			},
			"required": []string{"pattern"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			p, _ := args["pattern"].(string)
			if cwd, ok := args["cwd"].(string); ok && cwd != "" {
				p = filepath.Join(cwd, p)
			}
			m, err := filepath.Glob(p)
			if err != nil {
				return &ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			if len(m) == 0 {
				return &ToolResult{Content: "(no matches)"}, nil
			}
			return &ToolResult{Content: strings.Join(m, "\n")}, nil
		},
	}
}

func grepTool() Tool {
	return Tool{
		Name:        "grep",
		Description: "Search file contents with regex via ripgrep.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Regex pattern"},
				"path":    map[string]any{"type": "string", "description": "File or directory"},
				"glob":    map[string]any{"type": "string", "description": "File glob filter"},
			},
			"required": []string{"pattern"},
		},
		Execute: func(args map[string]any) (*ToolResult, error) {
			pat, _ := args["pattern"].(string)
			path, _ := args["path"].(string)
			if path == "" {
				path = "."
			}
			cmdArgs := []string{"--no-heading", "--line-number"}
			if g, ok := args["glob"].(string); ok && g != "" {
				cmdArgs = append(cmdArgs, "--glob", g)
			}
			cmdArgs = append(cmdArgs, pat, path)
			out, err := osexec.Command("rg", cmdArgs...).CombinedOutput()
			if err != nil && len(out) == 0 {
				return &ToolResult{Content: "(no matches)"}, nil
			}
			return &ToolResult{Content: string(out)}, nil
		},
	}
}

func GF(m map[string]any, key string, def float64) float64 {
	v, ok := m[key].(float64)
	if !ok {
		return def
	}
	return v
}
