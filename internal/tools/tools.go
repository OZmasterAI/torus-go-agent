package tools

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	t "torus_go_agent/internal/types"
)

// BuildDefaultTools returns the 6 standard tools.
func BuildDefaultTools() []t.Tool {
	return []t.Tool{bashTool(), readTool(), writeTool(), editTool(), globTool(), grepTool()}
}

func bashTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
			command, _ := args["command"].(string)
			cwd, _ := args["cwd"].(string)
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
				s := string(out)
				if s == "" {
					s = "(no output)"
				}
				return &t.ToolResult{Content: s, IsError: cmdErr != nil}, nil
			case <-time.After(30 * time.Second):
				cmd.Process.Kill()
				return &t.ToolResult{Content: "Timed out (30s)", IsError: true}, nil
			}
		},
	}
}

func readTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
			fp, _ := args["file_path"].(string)
			data, err := os.ReadFile(fp)
			if err != nil {
				return &t.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
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
			return &t.ToolResult{Content: strings.Join(out, "\n")}, nil
		},
	}
}

func writeTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
			fp, _ := args["file_path"].(string)
			c, _ := args["content"].(string)
			os.MkdirAll(filepath.Dir(fp), 0755)
			if err := os.WriteFile(fp, []byte(c), 0644); err != nil {
				return &t.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			return &t.ToolResult{Content: fmt.Sprintf("Wrote %d lines to %s", strings.Count(c, "\n")+1, fp)}, nil
		},
	}
}

func editTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
			fp, _ := args["file_path"].(string)
			old, _ := args["old_str"].(string)
			nw, _ := args["new_str"].(string)
			all, _ := args["replace_all"].(bool)
			data, err := os.ReadFile(fp)
			if err != nil {
				return &t.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			c := string(data)
			if !strings.Contains(c, old) {
				return &t.ToolResult{Content: "Error: old_str not found", IsError: true}, nil
			}
			if all {
				c = strings.ReplaceAll(c, old, nw)
			} else {
				c = strings.Replace(c, old, nw, 1)
			}
			os.WriteFile(fp, []byte(c), 0644)
			return &t.ToolResult{Content: "Edited " + fp}, nil
		},
	}
}

func globTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
			p, _ := args["pattern"].(string)
			if cwd, ok := args["cwd"].(string); ok && cwd != "" {
				p = filepath.Join(cwd, p)
			}
			m, err := filepath.Glob(p)
			if err != nil {
				return &t.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			if len(m) == 0 {
				return &t.ToolResult{Content: "(no matches)"}, nil
			}
			return &t.ToolResult{Content: strings.Join(m, "\n")}, nil
		},
	}
}

func grepTool() t.Tool {
	return t.Tool{
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
		Execute: func(args map[string]any) (*t.ToolResult, error) {
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
				return &t.ToolResult{Content: "(no matches)"}, nil
			}
			return &t.ToolResult{Content: string(out)}, nil
		},
	}
}

// GF extracts a float64 from a map with a default value.
func GF(m map[string]any, key string, def float64) float64 {
	v, ok := m[key].(float64)
	if !ok {
		return def
	}
	return v
}
