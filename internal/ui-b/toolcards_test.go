package uib

import (
	"strings"
	"testing"

	"torus_go_agent/internal/tui/shared"
)

// ── Verbose (level 1) — original tests updated for 3-arg Render ──────────────

func TestToolCardRegistryBash(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{Name: "bash", Args: map[string]any{"command": "ls -la"}}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "bash") {
		t.Fatal("bash card should contain tool name")
	}
	if !strings.Contains(card, "ls") {
		t.Fatal("bash card should contain command")
	}
}

func TestToolCardRegistryEdit(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "edit",
		FilePath: "/tmp/test.go",
		Args:     map[string]any{"old_str": "old", "new_str": "new"},
	}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "edit") {
		t.Fatal("edit card should contain tool name")
	}
	if !strings.Contains(card, "test.go") {
		t.Fatal("edit card should contain file path")
	}
}

func TestToolCardRegistryWrite(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "write",
		FilePath: "/tmp/out.txt",
		Args:     map[string]any{"content": "line1\nline2\nline3"},
	}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "write") {
		t.Fatal("write card should contain tool name")
	}
	if !strings.Contains(card, "3 lines") {
		t.Fatal("write card should contain line count")
	}
}

func TestToolCardRegistryRead(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "read",
		FilePath: "/tmp/read.go",
	}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "read") {
		t.Fatal("read card should contain tool name")
	}
}

func TestToolCardRegistrySearch(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "grep",
		Args:   map[string]any{"pattern": "TODO"},
		Result: "file1.go\nfile2.go",
	}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "grep") {
		t.Fatal("grep card should contain tool name")
	}
}

func TestToolCardRegistryCustomRenderer(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	reg.Register("custom_tool", &testRenderer{})
	card := reg.Render(&ToolEvent{Name: "custom_tool"}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "CUSTOM") {
		t.Fatal("custom renderer should be used")
	}
}

type testRenderer struct{}

func (r *testRenderer) Render(ev *ToolEvent, maxWidth int, theme Theme) string {
	return "CUSTOM"
}

func (r *testRenderer) RenderCompact(ev *ToolEvent, maxWidth int, theme Theme) string {
	return "CUSTOM_COMPACT"
}

func (r *testRenderer) RenderFull(ev *ToolEvent, maxWidth int, theme Theme) string {
	return "CUSTOM_FULL"
}

func TestToolCardRegistryFallback(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{Name: "unknown_mcp_tool", Result: "some result"}, 80, shared.VerbosityVerbose)
	if !strings.Contains(card, "unknown_mcp_tool") {
		t.Fatal("fallback card should contain tool name")
	}
}

// ── Compact (level 0) ────────────────────────────────────────────────────────

func TestCompactBash(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "bash",
		Args:   map[string]any{"command": "ls -la"},
		Result: "file1\nfile2\nfile3\nfile4\nfile5",
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "$ ls") {
		t.Fatal("compact bash should contain command")
	}
	// Should have tree chars
	if !strings.Contains(card, "\u2514\u2500") && !strings.Contains(card, "\u251c\u2500") {
		t.Fatal("compact bash should have tree-drawing characters")
	}
}

func TestCompactEdit(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "edit",
		FilePath: "/tmp/test.go",
		Args:     map[string]any{"old_str": "old line", "new_str": "new line"},
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "Edit") {
		t.Fatal("compact edit should contain 'Edit'")
	}
	if !strings.Contains(card, "test.go") {
		t.Fatal("compact edit should contain file path")
	}
	// Should have tree chars for diff lines
	if !strings.Contains(card, "\u2514\u2500") {
		t.Fatal("compact edit should have tree-drawing chars for diff")
	}
}

func TestCompactWrite(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "write",
		FilePath: "/tmp/out.txt",
		Args:     map[string]any{"content": "a\nb\nc"},
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "Write") {
		t.Fatal("compact write should contain 'Write'")
	}
	if !strings.Contains(card, "3 lines") {
		t.Fatal("compact write should contain line count")
	}
}

func TestCompactRead(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "read",
		FilePath: "/tmp/read.go",
		Result:   "line1\nline2\nline3\nline4\nline5",
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "Read") {
		t.Fatal("compact read should contain 'Read'")
	}
	// Should have tree lines for result preview
	if !strings.Contains(card, "\u2514\u2500") {
		t.Fatal("compact read should have tree-drawing chars")
	}
}

func TestCompactReadOverflow(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:     "read",
		FilePath: "/tmp/read.go",
		Result:   "l1\nl2\nl3\nl4\nl5\nl6",
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "+") {
		t.Fatal("compact read with >3 lines should show overflow indicator")
	}
}

func TestCompactSearch(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "grep",
		Args:   map[string]any{"pattern": "TODO"},
		Result: "file1.go\nfile2.go",
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "TODO") {
		t.Fatal("compact search should contain pattern")
	}
	if !strings.Contains(card, "\u2192") {
		t.Fatal("compact search should contain arrow")
	}
}

func TestCompactDefault(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "mcp_tool",
		Result: "some result",
	}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "Mcp_tool") {
		t.Fatal("compact default should capitalize tool name")
	}
}

func TestCompactNoHeaderFooter(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name: "bash",
		Args: map[string]any{"command": "ls"},
	}, 80, shared.VerbosityCompact)
	// Compact mode should NOT have the header/footer separators.
	if strings.Contains(card, "--- bash") {
		t.Fatal("compact card should not have verbose header")
	}
}

func TestCompactCustomRenderer(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	reg.Register("custom_tool", &testRenderer{})
	card := reg.Render(&ToolEvent{Name: "custom_tool"}, 80, shared.VerbosityCompact)
	if !strings.Contains(card, "CUSTOM_COMPACT") {
		t.Fatal("compact custom renderer should be used")
	}
}

// ── Full (level 2) ──────────────────────────────────────────────────────────

func TestFullBash(t *testing.T) {
	// Build a result with 10 lines.
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "output line")
	}
	reg := NewToolCardRegistry(DefaultTheme())
	card := reg.Render(&ToolEvent{
		Name:   "bash",
		Args:   map[string]any{"command": "cat big"},
		Result: strings.Join(lines, "\n"),
	}, 80, shared.VerbosityFull)
	// Full mode should NOT truncate to 5 lines.
	if strings.Contains(card, "+5 lines") {
		t.Fatal("full bash should not truncate output")
	}
	if !strings.Contains(card, "bash") {
		t.Fatal("full bash card should contain tool name")
	}
}

func TestFullEdit(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	// Build old/new with 15 lines each.
	old := strings.Repeat("old\n", 15)
	new := strings.Repeat("new\n", 15)
	card := reg.Render(&ToolEvent{
		Name:     "edit",
		FilePath: "/tmp/test.go",
		Args:     map[string]any{"old_str": old, "new_str": new},
	}, 80, shared.VerbosityFull)
	// Full diff should not have "... +N lines" overflow.
	if strings.Contains(card, "... +") {
		t.Fatal("full edit should not truncate diff")
	}
}

func TestFullWrite(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	content := strings.Repeat("line\n", 25)
	card := reg.Render(&ToolEvent{
		Name:     "write",
		FilePath: "/tmp/out.txt",
		Args:     map[string]any{"content": content},
	}, 80, shared.VerbosityFull)
	// Full write shows up to 20 lines, then overflow.
	if !strings.Contains(card, "+") {
		t.Fatal("full write with >20 lines should show overflow")
	}
	if !strings.Contains(card, "out.txt") {
		t.Fatal("full write should contain file path")
	}
}

func TestFullRead(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	result := strings.Repeat("line\n", 25)
	card := reg.Render(&ToolEvent{
		Name:     "read",
		FilePath: "/tmp/read.go",
		Result:   result,
	}, 80, shared.VerbosityFull)
	// Full read shows up to 20 lines, then overflow.
	if !strings.Contains(card, "+") {
		t.Fatal("full read with >20 lines should show overflow")
	}
}

func TestFullSearch(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	result := "file1.go\nfile2.go\nfile3.go\nfile4.go\nfile5.go"
	card := reg.Render(&ToolEvent{
		Name:   "grep",
		Args:   map[string]any{"pattern": "TODO"},
		Result: result,
	}, 80, shared.VerbosityFull)
	// Full search should show all result lines.
	if !strings.Contains(card, "file5.go") {
		t.Fatal("full search should show all result lines")
	}
}

func TestFullCustomRenderer(t *testing.T) {
	reg := NewToolCardRegistry(DefaultTheme())
	reg.Register("custom_tool", &testRenderer{})
	card := reg.Render(&ToolEvent{Name: "custom_tool"}, 80, shared.VerbosityFull)
	if !strings.Contains(card, "CUSTOM_FULL") {
		t.Fatal("full custom renderer should be used")
	}
}

// ── Helper tests ────────────────────────────────────────────────────────────

func TestSplitNonEmpty(t *testing.T) {
	lines := splitNonEmpty("a\n\nb\nc\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 non-empty lines, got %d", len(lines))
	}
}

func TestRenderTreeLines(t *testing.T) {
	theme := DefaultTheme()
	out := renderTreeLines([]string{"one", "two", "three"}, theme.ToolDim, 56)
	if !strings.Contains(out, "\u251c\u2500") {
		t.Fatal("multi-line tree should have mid-branch chars")
	}
	if !strings.Contains(out, "\u2514\u2500") {
		t.Fatal("multi-line tree should have end-branch chars")
	}
}

func TestRenderTreeLinesSingle(t *testing.T) {
	theme := DefaultTheme()
	out := renderTreeLines([]string{"only"}, theme.ToolDim, 56)
	if !strings.Contains(out, "\u2514\u2500") {
		t.Fatal("single-line tree should use end-branch char")
	}
	if strings.Contains(out, "\u251c\u2500") {
		t.Fatal("single-line tree should not have mid-branch char")
	}
}

func TestRenderDiffFull(t *testing.T) {
	theme := DefaultTheme()
	old := strings.Repeat("old\n", 20)
	new := strings.Repeat("new\n", 20)
	out := renderDiffFull(old, new, 60, theme)
	// Should not have overflow indicator
	if strings.Contains(out, "... +") {
		t.Fatal("renderDiffFull should not truncate")
	}
}

func TestCountMatches(t *testing.T) {
	if n := countMatches("a\nb\nc"); n != 3 {
		t.Fatalf("expected 3 matches, got %d", n)
	}
	if n := countMatches("no matches found"); n != 0 {
		t.Fatalf("expected 0 matches for 'no matches', got %d", n)
	}
	if n := countMatches(""); n != 0 {
		t.Fatalf("expected 0 matches for empty, got %d", n)
	}
}
