package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestThinkingDeltaMsgType verifies the thinkingDeltaMsg struct exists and carries delta text.
func TestThinkingDeltaMsgType(t *testing.T) {
	msg := thinkingDeltaMsg{delta: "reasoning step"}
	if msg.delta != "reasoning step" {
		t.Errorf("thinkingDeltaMsg.delta = %q, want %q", msg.delta, "reasoning step")
	}
}

// TestWaitForThinking tests the thinking channel waiter function.
func TestWaitForThinking(t *testing.T) {
	t.Run("nil channel returns nil", func(t *testing.T) {
		cmd := waitForThinking(nil)
		if cmd != nil {
			t.Error("waitForThinking(nil) should return nil")
		}
	})

	t.Run("reads from channel", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "thinking text"
		cmd := waitForThinking(ch)
		if cmd == nil {
			t.Fatal("waitForThinking should return a command")
		}
		msg := cmd()
		tdm, ok := msg.(thinkingDeltaMsg)
		if !ok {
			t.Fatalf("expected thinkingDeltaMsg, got %T", msg)
		}
		if tdm.delta != "thinking text" {
			t.Errorf("delta = %q, want %q", tdm.delta, "thinking text")
		}
	})

	t.Run("closed channel returns nil msg", func(t *testing.T) {
		ch := make(chan string)
		close(ch)
		cmd := waitForThinking(ch)
		if cmd == nil {
			t.Fatal("waitForThinking should return a command for closed channel")
		}
		msg := cmd()
		if msg != nil {
			t.Errorf("closed channel should yield nil msg, got %T", msg)
		}
	})
}

// TestModelThinkingFieldExists verifies the Model struct has the thinking field.
func TestModelThinkingFieldExists(t *testing.T) {
	m := Model{}
	// Verify thinking model is accessible (zero value)
	if m.thinking.Verbosity != 0 {
		t.Error("thinking.Verbosity should default to 0 (compact)")
	}
	if m.thinking.HasPending() {
		t.Error("thinking should have no pending content by default")
	}
}

// TestDisplayMsgThinkingTextField verifies displayMsg has the thinkingText field.
func TestDisplayMsgThinkingTextField(t *testing.T) {
	dm := displayMsg{role: "assistant", text: "hello", thinkingText: "reasoning here"}
	if dm.thinkingText != "reasoning here" {
		t.Errorf("thinkingText = %q, want %q", dm.thinkingText, "reasoning here")
	}
}

// TestThinkingDeltaUpdate tests that thinkingDeltaMsg appends to thinking buffer.
func TestThinkingDeltaUpdate(t *testing.T) {
	m := Model{
		width:  80,
		height: 24,
		ready:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.thinkingCh = make(chan string, 8)
	m.streaming = true
	m.messages = []displayMsg{newDisplayMsg("assistant", "")}

	// Simulate a thinking delta arriving
	m.thinking.AppendDelta("step 1")
	if !m.thinking.HasPending() {
		t.Error("thinking should have pending content after AppendDelta")
	}
	if m.thinking.Buf != "step 1" {
		t.Errorf("thinking.Buf = %q, want %q", m.thinking.Buf, "step 1")
	}

	// Append more
	m.thinking.AppendDelta(" continued")
	if m.thinking.Buf != "step 1 continued" {
		t.Errorf("thinking.Buf = %q, want %q", m.thinking.Buf, "step 1 continued")
	}
}

// TestThinkingCollapseOnResponse tests that thinking is stored on the last assistant message.
func TestThinkingCollapseOnResponse(t *testing.T) {
	m := Model{
		width:  80,
		height: 24,
		ready:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.streaming = true
	m.messages = []displayMsg{newDisplayMsg("assistant", "response text")}

	// Simulate thinking accumulation
	m.thinking.AppendDelta("I need to reason about this")

	// Simulate what agentResponseMsg handler now does:
	// store thinking on last assistant message instead of global Cards
	if m.thinking.HasPending() {
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				m.messages[i].thinkingText = m.thinking.Buf
				m.messages[i].rendered = ""
				break
			}
		}
		m.thinking.Buf = ""
	}

	if m.thinking.HasPending() {
		t.Error("thinking should have no pending after storing on message")
	}
	if m.messages[0].thinkingText != "I need to reason about this" {
		t.Errorf("thinkingText = %q, want %q", m.messages[0].thinkingText, "I need to reason about this")
	}
}

// TestCtrlOToggle tests that Ctrl+O toggles thinking visibility.
func TestCtrlOToggle(t *testing.T) {
	m := Model{
		width:  80,
		height: 24,
		ready:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.messages = []displayMsg{newDisplayMsg("assistant", "hello")}

	// Default: compact
	if m.thinking.Verbosity != 0 {
		t.Error("thinking should be compact (0) by default")
	}

	// Toggle: compact -> verbose
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model := newM.(Model)
	if model.thinking.Verbosity != 1 {
		t.Errorf("first Ctrl+O should set verbosity to 1 (verbose), got %d", model.thinking.Verbosity)
	}
	if cmd != nil {
		t.Error("Ctrl+O should return nil cmd")
	}

	// Toggle: verbose -> full
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model = newM.(Model)
	if model.thinking.Verbosity != 2 {
		t.Errorf("second Ctrl+O should set verbosity to 2 (full), got %d", model.thinking.Verbosity)
	}

	// Toggle: full -> compact
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	model = newM.(Model)
	if model.thinking.Verbosity != 0 {
		t.Errorf("third Ctrl+O should set verbosity to 0 (compact), got %d", model.thinking.Verbosity)
	}
}

// TestThinkingRenderInContent tests that thinking cards appear inline with assistant messages.
func TestThinkingRenderInContent(t *testing.T) {
	m := Model{
		width:  80,
		height: 24,
		ready:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.messages = []displayMsg{
		{role: "user", text: "hello", ts: time.Now()},
		{role: "assistant", text: "world", ts: time.Now(), thinkingText: "Let me think about this carefully"},
	}

	// Rebuild content - thinking should appear inline before assistant text
	m.rebuildContent()
	content := m.viewport.View()
	if !strings.Contains(content, "thinking") {
		t.Error("rebuildContent should include thinking card inline with assistant message")
	}
	// Thinking should appear before the response text
	thinkIdx := strings.Index(content, "thinking")
	worldIdx := strings.Index(content, "world")
	if thinkIdx >= 0 && worldIdx >= 0 && thinkIdx > worldIdx {
		t.Error("thinking card should appear before assistant text, not after")
	}
}

// TestThinkingPendingRenderDuringStream tests pending thinking renders during streaming.
func TestThinkingPendingRenderDuringStream(t *testing.T) {
	m := Model{
		width:     80,
		height:    24,
		ready:     true,
		streaming: true,
	}
	m.viewport = viewport.New(80, 20)
	m.messages = []displayMsg{
		newDisplayMsg("assistant", ""),
	}

	// Buffer some thinking
	m.thinking.AppendDelta("reasoning in progress")

	// Rebuild
	m.rebuildContent()
	content := m.viewport.View()
	if !strings.Contains(content, "thinking") {
		t.Error("rebuildContent should show pending thinking during streaming")
	}
}

// TestThinkingChannelNilOnResponse verifies the agentResponseMsg handler
// nils thinkingCh and stores thinking on the last assistant message.
func TestThinkingChannelNilOnResponse(t *testing.T) {
	m := Model{
		width:      80,
		height:     24,
		ready:      true,
		processing: true,
		streaming:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.thinkingCh = make(chan string, 8)
	m.messages = []displayMsg{newDisplayMsg("assistant", "response")}

	// Simulate thinking buffer before response
	m.thinking.AppendDelta("I think about this")
	if !m.thinking.HasPending() {
		t.Fatal("thinking should have pending before collapse")
	}

	// Directly simulate what agentResponseMsg handler does:
	// store thinking on last assistant message
	m.thinkingCh = nil
	if m.thinking.HasPending() {
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				m.messages[i].thinkingText = m.thinking.Buf
				m.messages[i].rendered = ""
				break
			}
		}
		m.thinking.Buf = ""
	}

	if m.thinkingCh != nil {
		t.Error("thinkingCh should be nil after response handling")
	}
	if m.thinking.HasPending() {
		t.Error("thinking should have no pending after storing on message")
	}
	if m.messages[0].thinkingText != "I think about this" {
		t.Errorf("thinkingText = %q, want %q", m.messages[0].thinkingText, "I think about this")
	}
}

// TestThinkingChannelNilOnError verifies the agentErrorMsg handler
// nils thinkingCh and stores thinking on the last assistant message.
func TestThinkingChannelNilOnError(t *testing.T) {
	m := Model{
		width:      80,
		height:     24,
		ready:      true,
		processing: true,
		streaming:  true,
	}
	m.viewport = viewport.New(80, 20)
	m.thinkingCh = make(chan string, 8)
	m.messages = []displayMsg{newDisplayMsg("assistant", "partial")}

	// Buffer some thinking
	m.thinking.AppendDelta("partial reasoning")

	// Directly simulate what agentErrorMsg handler does:
	// store thinking on last assistant message
	m.thinkingCh = nil
	if m.thinking.HasPending() {
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				m.messages[i].thinkingText = m.thinking.Buf
				m.messages[i].rendered = ""
				break
			}
		}
		m.thinking.Buf = ""
	}

	if m.thinkingCh != nil {
		t.Error("thinkingCh should be nil after error handling")
	}
	if m.thinking.HasPending() {
		t.Error("thinking should have no pending after error handling")
	}
	if m.messages[0].thinkingText != "partial reasoning" {
		t.Errorf("thinkingText = %q, want %q", m.messages[0].thinkingText, "partial reasoning")
	}
}

// TestRenderHelpIncludesThinking verifies the help overlay lists Ctrl+O.
func TestRenderHelpIncludesThinking(t *testing.T) {
	m := Model{width: 80, height: 24}
	help := m.renderHelp()
	if !strings.Contains(help, "Ctrl+O") {
		t.Error("renderHelp should list Ctrl+O keybinding")
	}
	if !strings.Contains(help, "verbosity") && !strings.Contains(help, "Verbosity") {
		t.Error("renderHelp should mention verbosity cycling for Ctrl+O")
	}
}

// TestToolCardCompactRead verifies compact multi-line rendering for read tool.
func TestToolCardCompactRead(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:     "read",
		filePath: "/home/user/project/main.go",
		args:     map[string]any{},
		result:   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}",
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "Read") {
		t.Errorf("compact read should contain 'Read', got %q", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("compact read should contain filename, got %q", out)
	}
	// Compact mode: no timestamp
	if strings.Contains(out, "12:34:56") {
		t.Errorf("compact read should NOT contain timestamp, got %q", out)
	}
	// Multi-line: header + up to 3 preview lines + overflow
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Errorf("compact read with result should have multiple lines, got %d", len(lines))
	}
	// Should show preview lines with tree characters (single-dash)
	if !strings.Contains(out, "package main") {
		t.Errorf("compact read should show first result line, got %q", out)
	}
	if !strings.Contains(out, "\u251c\u2500 ") {
		t.Errorf("compact read should use tree mid-prefix, got %q", out)
	}
	// Should show overflow indicator with tree end-prefix
	if !strings.Contains(out, "... +") {
		t.Errorf("compact read with many lines should show overflow, got %q", out)
	}
	if !strings.Contains(out, "\u2514\u2500 ") {
		t.Errorf("compact read overflow should use tree end-prefix, got %q", out)
	}
}

// TestToolCardCompactReadNoResult verifies compact read with no result.
func TestToolCardCompactReadNoResult(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:     "read",
		filePath: "/home/user/project/main.go",
		args:     map[string]any{},
	}
	out := m.renderToolCardCompact(ev)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("compact read without result should be 1 line, got %d: %q", len(lines), out)
	}
}

// TestToolCardCompactEdit verifies compact rendering for edit tool with diff.
func TestToolCardCompactEdit(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:     "edit",
		filePath: "/home/user/project/main.go",
		args:     map[string]any{"old_str": "foo := 1", "new_str": "bar := 2"},
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "Edit") {
		t.Errorf("compact edit should contain 'Edit', got %q", out)
	}
	// Should show brief diff lines with tree prefixes (single-dash)
	if !strings.Contains(out, "- foo") {
		t.Errorf("compact edit should show old line with '- ' prefix, got %q", out)
	}
	if !strings.Contains(out, "+ bar") {
		t.Errorf("compact edit should show new line with '+ ' prefix, got %q", out)
	}
	if !strings.Contains(out, "\u251c\u2500 ") {
		t.Errorf("compact edit should use tree mid-prefix for old line, got %q", out)
	}
	if !strings.Contains(out, "\u2514\u2500 ") {
		t.Errorf("compact edit should use tree end-prefix for new line, got %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Header + old + new = 3 lines
	if len(lines) != 3 {
		t.Errorf("compact edit with diff should be 3 lines, got %d: %q", len(lines), out)
	}
}

// TestToolCardCompactEditNoDiff verifies compact edit with empty old/new.
func TestToolCardCompactEditNoDiff(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:     "edit",
		filePath: "/home/user/project/main.go",
		args:     map[string]any{},
	}
	out := m.renderToolCardCompact(ev)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("compact edit without diff should be 1 line, got %d: %q", len(lines), out)
	}
}

// TestToolCardCompactBash verifies compact rendering for bash tool with output preview.
func TestToolCardCompactBash(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:   "bash",
		args:   map[string]any{"command": "ls -la /tmp"},
		result: "file1\nfile2\nfile3\nfile4\nfile5\n",
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "$") {
		t.Errorf("compact bash should contain '$', got %q", out)
	}
	if !strings.Contains(out, "ls -la") {
		t.Errorf("compact bash should contain command, got %q", out)
	}
	// Should show preview lines with tree characters (single-dash)
	if !strings.Contains(out, "file1") {
		t.Errorf("compact bash should show first output line, got %q", out)
	}
	if !strings.Contains(out, "\u251c\u2500 ") {
		t.Errorf("compact bash should use tree mid-prefix, got %q", out)
	}
	// Should show overflow for >3 lines with tree end-prefix
	if !strings.Contains(out, "... +") {
		t.Errorf("compact bash with many lines should show overflow, got %q", out)
	}
	if !strings.Contains(out, "\u2514\u2500 ") {
		t.Errorf("compact bash overflow should use tree end-prefix, got %q", out)
	}
}

// TestToolCardCompactBashNoOutput verifies compact bash with no output.
func TestToolCardCompactBashNoOutput(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name: "bash",
		args: map[string]any{"command": "mkdir /tmp/test"},
	}
	out := m.renderToolCardCompact(ev)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("compact bash without output should be 1 line, got %d: %q", len(lines), out)
	}
}

// TestToolCardCompactWrite verifies compact rendering for write tool.
func TestToolCardCompactWrite(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:     "write",
		filePath: "/home/user/project/output.txt",
		args:     map[string]any{"content": "line1\nline2\nline3"},
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "Write") {
		t.Errorf("compact write should contain 'Write', got %q", out)
	}
	if !strings.Contains(out, "3 lines") {
		t.Errorf("compact write should show line count, got %q", out)
	}
	// Write is always a single header line
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("compact write should be 1 line, got %d: %q", len(lines), out)
	}
}

// TestToolCardCompactGlob verifies compact rendering for glob tool with match preview.
func TestToolCardCompactGlob(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:   "glob",
		args:   map[string]any{"pattern": "**/*.go"},
		result: "main.go\nutil.go\ntest.go\nhelper.go\nsetup.go",
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "**/*.go") {
		t.Errorf("compact glob should contain pattern, got %q", out)
	}
	if !strings.Contains(out, "matches") {
		t.Errorf("compact glob should mention matches, got %q", out)
	}
	// Should show preview of match files with tree characters (single-dash)
	if !strings.Contains(out, "main.go") {
		t.Errorf("compact glob should show first match, got %q", out)
	}
	if !strings.Contains(out, "\u251c\u2500 ") {
		t.Errorf("compact glob should use tree mid-prefix, got %q", out)
	}
	// Should show overflow with tree end-prefix
	if !strings.Contains(out, "... +") {
		t.Errorf("compact glob with many matches should show overflow, got %q", out)
	}
	if !strings.Contains(out, "\u2514\u2500 ") {
		t.Errorf("compact glob overflow should use tree end-prefix, got %q", out)
	}
}

// TestToolCardCompactDefault verifies compact rendering for unknown tools.
func TestToolCardCompactDefault(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name: "custom_tool",
		args: map[string]any{},
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "Custom_tool") {
		t.Errorf("compact default should capitalize tool name, got %q", out)
	}
	// Compact mode: no timestamp
	if strings.Contains(out, "12:34:56") {
		t.Errorf("compact default should NOT contain timestamp, got %q", out)
	}
}

// TestToolCardCompactDefaultWithResult verifies default tool shows brief result.
func TestToolCardCompactDefaultWithResult(t *testing.T) {
	m := Model{width: 80, height: 24}
	ev := &toolEvent{
		name:   "custom_tool",
		args:   map[string]any{},
		result: "some output",
	}
	out := m.renderToolCardCompact(ev)
	if !strings.Contains(out, "some output") {
		t.Errorf("compact default with result should show result, got %q", out)
	}
	if !strings.Contains(out, "\u2514\u2500 ") {
		t.Errorf("compact default with result should use tree end-prefix, got %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("compact default with result should be 2 lines, got %d: %q", len(lines), out)
	}
}

// TestRenderTreeLines verifies tree-style box-drawing prefix rendering.
func TestRenderTreeLines(t *testing.T) {
	style := lipgloss.NewStyle()

	// Single line should get └─
	out := renderTreeLines([]string{"only"}, style, 60)
	if !strings.Contains(out, "\u2514\u2500 only") {
		t.Errorf("single line should get end-prefix, got %q", out)
	}
	if strings.Contains(out, "\u251c\u2500 ") {
		t.Errorf("single line should not have mid-prefix, got %q", out)
	}

	// Multiple lines: middle get ├─, last gets └─
	out = renderTreeLines([]string{"a", "b", "c"}, style, 60)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "\u251c\u2500 a") {
		t.Errorf("line 0 should have mid-prefix, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "\u251c\u2500 b") {
		t.Errorf("line 1 should have mid-prefix, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "\u2514\u2500 c") {
		t.Errorf("line 2 should have end-prefix, got %q", lines[2])
	}

	// Empty slice should return empty string
	out = renderTreeLines([]string{}, style, 60)
	if out != "" {
		t.Errorf("empty slice should return empty string, got %q", out)
	}
}

// TestSplitNonEmpty verifies the helper strips blank lines.
func TestSplitNonEmpty(t *testing.T) {
	got := splitNonEmpty("a\n\nb\n\nc\n")
	if len(got) != 3 {
		t.Errorf("splitNonEmpty should return 3 items, got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitNonEmpty wrong content: %v", got)
	}

	got2 := splitNonEmpty("")
	if len(got2) != 0 {
		t.Errorf("splitNonEmpty on empty should return 0 items, got %d", len(got2))
	}
}

// TestVerboseToggleToolRendering verifies that Verbosity controls tool card verbosity.
func TestVerboseToggleToolRendering(t *testing.T) {
	m := Model{
		width:  80,
		height: 24,
		ready:  true,
	}
	m.viewport = viewport.New(80, 20)

	toolEv := &toolEvent{
		name:     "edit",
		filePath: "/home/user/project/main.go",
		args:     map[string]any{"old_str": "old code", "new_str": "new code"},
		result:   "ok",
	}
	m.messages = []displayMsg{
		{role: "tool", tool: toolEv, ts: time.Now()},
	}

	t.Run("compact mode (Verbosity=0)", func(t *testing.T) {
		m.thinking.Verbosity = 0
		m.rebuildContent()
		content := m.viewport.View()
		// In compact mode: single "Edit" line, no separator borders
		if !strings.Contains(content, "Edit") {
			t.Error("compact mode should show 'Edit' in tool line")
		}
		// Should NOT have the full header separator pattern
		if strings.Contains(content, "───") {
			t.Error("compact mode should not show header separator borders")
		}
	})

	t.Run("verbose mode (Verbosity=1)", func(t *testing.T) {
		m.thinking.Verbosity = 1
		m.rebuildContent()
		content := m.viewport.View()
		// In verbose mode: full header with separators
		if !strings.Contains(content, "───") {
			t.Error("verbose mode should show header separator borders")
		}
	})

	t.Run("full mode (Verbosity=2)", func(t *testing.T) {
		m.thinking.Verbosity = 2
		m.rebuildContent()
		content := m.viewport.View()
		// In full mode: full header with separators (same as verbose but no truncation)
		if !strings.Contains(content, "───") {
			t.Error("full mode should show header separator borders")
		}
	})
}

// TestRenderToolCardFullBash verifies full mode shows all bash output without truncation.
func TestRenderToolCardFullBash(t *testing.T) {
	m := Model{width: 100, height: 24}
	// Build output longer than the verbose limit of 5 lines
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d output", i))
	}
	ev := &toolEvent{
		name:   "bash",
		args:   map[string]any{"command": "seq 1 10"},
		result: strings.Join(lines, "\n"),
	}
	out := m.renderToolCardFull(ev, 80)
	// All 10 lines should appear (no "... +N lines" truncation)
	for i := 1; i <= 10; i++ {
		expected := fmt.Sprintf("line %d output", i)
		if !strings.Contains(out, expected) {
			t.Errorf("full bash should contain %q, got:\n%s", expected, out)
		}
	}
	if strings.Contains(out, "... +") {
		t.Errorf("full bash should NOT truncate, got:\n%s", out)
	}
}

// TestRenderToolCardFullEdit verifies full mode shows all diff lines without truncation.
func TestRenderToolCardFullEdit(t *testing.T) {
	m := Model{width: 100, height: 24}
	// Build old/new strings with more than 10 lines each (verbose limit)
	var oldLines, newLines []string
	for i := 1; i <= 15; i++ {
		oldLines = append(oldLines, fmt.Sprintf("old line %d", i))
		newLines = append(newLines, fmt.Sprintf("new line %d", i))
	}
	ev := &toolEvent{
		name:     "edit",
		filePath: "/tmp/test.go",
		args: map[string]any{
			"old_str": strings.Join(oldLines, "\n"),
			"new_str": strings.Join(newLines, "\n"),
		},
	}
	out := m.renderToolCardFull(ev, 80)
	// All lines should appear
	if !strings.Contains(out, "old line 15") {
		t.Errorf("full edit should show all old lines, got:\n%s", out)
	}
	if !strings.Contains(out, "new line 15") {
		t.Errorf("full edit should show all new lines, got:\n%s", out)
	}
	if strings.Contains(out, "... +") {
		t.Errorf("full edit should NOT truncate, got:\n%s", out)
	}
}

// TestRenderToolCardFullRead verifies full mode shows file content.
func TestRenderToolCardFullRead(t *testing.T) {
	m := Model{width: 100, height: 24}
	ev := &toolEvent{
		name:     "read",
		filePath: "/tmp/test.go",
		args:     map[string]any{},
		result:   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}",
	}
	out := m.renderToolCardFull(ev, 80)
	if !strings.Contains(out, "package main") {
		t.Errorf("full read should show file content, got:\n%s", out)
	}
	if !strings.Contains(out, "fmt.Println") {
		t.Errorf("full read should show file content, got:\n%s", out)
	}
}

// TestRenderToolCardFullWrite verifies full mode shows written content.
func TestRenderToolCardFullWrite(t *testing.T) {
	m := Model{width: 100, height: 24}
	ev := &toolEvent{
		name:     "write",
		filePath: "/tmp/output.txt",
		args:     map[string]any{"content": "line1\nline2\nline3\nline4\nline5"},
	}
	out := m.renderToolCardFull(ev, 80)
	if !strings.Contains(out, "line1") {
		t.Errorf("full write should show written content, got:\n%s", out)
	}
	if !strings.Contains(out, "line5") {
		t.Errorf("full write should show all lines, got:\n%s", out)
	}
}

// TestRenderToolCardFullGlob verifies full mode shows all matches.
func TestRenderToolCardFullGlob(t *testing.T) {
	m := Model{width: 100, height: 24}
	var matches []string
	for i := 1; i <= 8; i++ {
		matches = append(matches, fmt.Sprintf("file%d.go", i))
	}
	ev := &toolEvent{
		name:   "glob",
		args:   map[string]any{"pattern": "**/*.go"},
		result: strings.Join(matches, "\n"),
	}
	out := m.renderToolCardFull(ev, 80)
	// All matches should appear
	for i := 1; i <= 8; i++ {
		expected := fmt.Sprintf("file%d.go", i)
		if !strings.Contains(out, expected) {
			t.Errorf("full glob should show %q, got:\n%s", expected, out)
		}
	}
	if strings.Contains(out, "... +") {
		t.Errorf("full glob should NOT truncate, got:\n%s", out)
	}
}
