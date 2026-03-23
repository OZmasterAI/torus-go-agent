package core

import (
	"strings"
	"testing"

	types "torus_go_agent/internal/types"
)

func TestGroupOperations_BasicTwoOps(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("You are a helpful agent.")}, // messages[0] system
		{Role: types.RoleUser, Content: textContent("fix the auth bug")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "text", Text: "I'll read the file"},
			{Type: "tool_use", ID: "t1", Name: "read", Input: map[string]any{"file_path": "auth.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "func auth()..."},
		}},
		{Role: types.RoleAssistant, Content: textContent("Fixed the auth bug.")},
		{Role: types.RoleUser, Content: textContent("now add tests")},
		{Role: types.RoleAssistant, Content: textContent("Done, tests added.")},
	}

	ops := GroupOperations(messages)
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}

	// Op 1: fix auth bug
	if ops[0].Intent != "fix the auth bug" {
		t.Errorf("op[0] intent = %q, want 'fix the auth bug'", ops[0].Intent)
	}
	if len(ops[0].Files) == 0 || ops[0].Files[0] != "auth.go" {
		t.Errorf("op[0] files = %v, want [auth.go]", ops[0].Files)
	}
	if ops[0].Outcome != "Fixed the auth bug." {
		t.Errorf("op[0] outcome = %q", ops[0].Outcome)
	}

	// Op 2: add tests
	if ops[1].Intent != "now add tests" {
		t.Errorf("op[1] intent = %q", ops[1].Intent)
	}
}

func TestGroupOperations_SkipsToolResultUserMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("do something")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "bash", Input: map[string]any{"command": "ls"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "file1.go"},
		}},
		{Role: types.RoleAssistant, Content: textContent("Here are the files.")},
	}

	ops := GroupOperations(messages)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation (tool_result user msg shouldn't start new op), got %d", len(ops))
	}
}

func TestGroupOperations_EmptyMessages(t *testing.T) {
	ops := GroupOperations(nil)
	if len(ops) != 0 {
		t.Fatalf("expected 0 operations for nil, got %d", len(ops))
	}
}

func TestGroupOperations_ExtractsMultipleFiles(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("refactor the code")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "read", Input: map[string]any{"file_path": "main.go"}},
			{Type: "tool_use", ID: "t2", Name: "edit", Input: map[string]any{"file_path": "config.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "..."},
			{Type: "tool_result", ToolUseID: "t2", Content: "..."},
		}},
		{Role: types.RoleAssistant, Content: textContent("Refactored.")},
	}

	ops := GroupOperations(messages)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if len(ops[0].Files) != 2 {
		t.Errorf("expected 2 files, got %v", ops[0].Files)
	}
}

func TestGroupOperations_ExtractsToolNames(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("check something")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "bash"},
			{Type: "tool_use", ID: "t2", Name: "read"},
			{Type: "tool_use", ID: "t3", Name: "bash"},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "ok"},
		}},
		{Role: types.RoleAssistant, Content: textContent("Done.")},
	}

	ops := GroupOperations(messages)
	if len(ops[0].Tools) < 2 {
		t.Errorf("expected at least 2 unique tools, got %v", ops[0].Tools)
	}
}

func TestGroupOperations_StartAndEndIndices(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},       // 0
		{Role: types.RoleUser, Content: textContent("task one")},           // 1
		{Role: types.RoleAssistant, Content: textContent("done one")},      // 2
		{Role: types.RoleUser, Content: textContent("task two")},           // 3
		{Role: types.RoleAssistant, Content: textContent("done two")},      // 4
	}

	ops := GroupOperations(messages)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}

	if ops[0].StartIdx != 1 || ops[0].EndIdx != 3 {
		t.Errorf("op[0] indices: start=%d end=%d, want start=1 end=3", ops[0].StartIdx, ops[0].EndIdx)
	}
	if ops[1].StartIdx != 3 || ops[1].EndIdx != 5 {
		t.Errorf("op[1] indices: start=%d end=%d, want start=3 end=5", ops[1].StartIdx, ops[1].EndIdx)
	}
}

func TestGroupOperations_MessagesSliceContents(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("hello")},
		{Role: types.RoleAssistant, Content: textContent("world")},
	}

	ops := GroupOperations(messages)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if len(ops[0].Messages) != 2 {
		t.Errorf("expected 2 messages in op, got %d", len(ops[0].Messages))
	}
}

func TestGroupOperations_DeduplicatesFiles(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("edit the file")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "read", Input: map[string]any{"file_path": "main.go"}},
			{Type: "tool_use", ID: "t2", Name: "edit", Input: map[string]any{"file_path": "main.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "..."},
			{Type: "tool_result", ToolUseID: "t2", Content: "..."},
		}},
		{Role: types.RoleAssistant, Content: textContent("Edited.")},
	}

	ops := GroupOperations(messages)
	if len(ops[0].Files) != 1 {
		t.Errorf("expected 1 deduplicated file, got %v", ops[0].Files)
	}
}

func TestGroupOperations_OnlySystemMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
	}

	ops := GroupOperations(messages)
	if len(ops) != 0 {
		t.Fatalf("expected 0 operations for system-only, got %d", len(ops))
	}
}

func TestRenderOperationTemplate_Basic(t *testing.T) {
	op := Operation{
		Intent:  "fix the auth bug",
		Files:   []string{"auth.go", "auth_test.go"},
		Tools:   []string{"read", "edit"},
		Outcome: "Fixed it. Tests passing.",
	}
	tmpl := RenderOperationTemplate(op)
	if !strings.Contains(tmpl, "fix the auth bug") {
		t.Error("template should contain intent")
	}
	if !strings.Contains(tmpl, "read, edit") {
		t.Error("template should contain tool names")
	}
	if !strings.Contains(tmpl, "auth.go") {
		t.Error("template should contain file names")
	}
	if !strings.Contains(tmpl, "Fixed it") {
		t.Error("template should contain outcome")
	}
}

func TestRenderOperationTemplate_NoTools(t *testing.T) {
	op := Operation{
		Intent:  "explain the code",
		Outcome: "The code does X.",
	}
	tmpl := RenderOperationTemplate(op)
	if !strings.Contains(tmpl, "explain the code") {
		t.Error("should contain intent")
	}
	// Should not have an empty "Actions:" line
	if strings.Contains(tmpl, "Actions: \n") {
		t.Error("should not have empty actions line")
	}
}

func TestOperationToMessage(t *testing.T) {
	op := Operation{
		Intent:  "fix bug",
		Tools:   []string{"read"},
		Files:   []string{"main.go"},
		Outcome: "Fixed.",
	}
	msg := OperationToMessage(op)
	if msg.Role != types.RoleAssistant {
		t.Errorf("expected assistant role, got %s", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Type != "text" {
		t.Error("expected single text content block")
	}
	if !strings.Contains(msg.Content[0].Text, "fix bug") {
		t.Error("message text should contain intent")
	}
}

// --- ScoreOperation tests ---

func TestScoreOperation_RecentHighFileOverlap(t *testing.T) {
	op := Operation{
		Files: []string{"auth.go", "main.go"},
		Tools: []string{"edit", "read"},
		Messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: "text", Text: strings.Repeat("x", 300)}}},
		},
	}
	activeFiles := []string{"auth.go", "config.go"}
	score := ScoreOperation(op, 0, 10, activeFiles)
	if score <= 0.4 {
		t.Errorf("recent op with file overlap should score > 0.4, got %.2f", score)
	}
}

func TestScoreOperation_OldNoFileOverlap(t *testing.T) {
	op := Operation{
		Files: []string{"readme.md"},
		Tools: []string{"read"},
	}
	activeFiles := []string{"auth.go", "config.go"}
	score := ScoreOperation(op, 9, 10, activeFiles)
	if score >= 0.3 {
		t.Errorf("old op with no file overlap should score < 0.3, got %.2f", score)
	}
}

func TestScoreOperation_MutationBoost(t *testing.T) {
	editOp := Operation{Tools: []string{"edit", "write"}}
	readOp := Operation{Tools: []string{"read", "glob"}}
	activeFiles := []string{}
	editScore := ScoreOperation(editOp, 5, 10, activeFiles)
	readScore := ScoreOperation(readOp, 5, 10, activeFiles)
	if editScore <= readScore {
		t.Errorf("mutation op (%.2f) should score higher than read-only op (%.2f)", editScore, readScore)
	}
}

func TestScoreOperation_DecayIsExponential(t *testing.T) {
	// Use an operation with zero non-recency signals to isolate decay behavior.
	// No files, no tools, no outcome => only recency contributes to score.
	op := Operation{}
	activeFiles := []string{}
	score0 := ScoreOperation(op, 0, 20, activeFiles)
	score4 := ScoreOperation(op, 4, 20, activeFiles)
	score8 := ScoreOperation(op, 8, 20, activeFiles)
	// Exponential: score at age 4 should be ~half of age 0 (half-life=4)
	ratio := score4 / score0
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("half-life ratio at age 4 should be ~0.5, got %.2f (scores: %.3f, %.3f)", ratio, score0, score4)
	}
	// Score at age 8 should be ~quarter of age 0
	ratio8 := score8 / score0
	if ratio8 < 0.15 || ratio8 > 0.35 {
		t.Errorf("ratio at age 8 should be ~0.25, got %.2f", ratio8)
	}
}

func TestScoreOperation_JaccardFileOverlap(t *testing.T) {
	op := Operation{Files: []string{"a.go", "b.go", "c.go"}}
	// 2 out of 4 unique files overlap
	activeFiles := []string{"a.go", "b.go", "d.go"}
	score := ScoreOperation(op, 0, 10, activeFiles)

	noOverlapOp := Operation{Files: []string{"x.go", "y.go"}}
	noScore := ScoreOperation(noOverlapOp, 0, 10, activeFiles)

	if score <= noScore {
		t.Errorf("overlapping files (%.2f) should score higher than no overlap (%.2f)", score, noScore)
	}
}

// --- Working Memory tests ---

func TestAppendWorkingMemory_Basic(t *testing.T) {
	sysMsg := types.Message{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{{Type: "text", Text: "You are a helpful agent."}},
	}
	ops := []Operation{
		{Intent: "fix auth bug", Outcome: "Tests passing", Files: []string{"auth.go"}},
		{Intent: "add logging", Outcome: "Done", Files: []string{"log.go"}},
	}
	result := AppendWorkingMemory(sysMsg, ops)
	text := result.Content[0].Text
	if !strings.Contains(text, "You are a helpful agent.") {
		t.Error("should preserve original system prompt")
	}
	if !strings.Contains(text, "Working Memory") {
		t.Error("should have Working Memory header")
	}
	if !strings.Contains(text, "fix auth bug") {
		t.Error("should contain first op intent")
	}
	if !strings.Contains(text, "add logging") {
		t.Error("should contain second op intent")
	}
	if !strings.Contains(text, "auth.go") {
		t.Error("should contain file names")
	}
}

func TestAppendWorkingMemory_EmptyOps(t *testing.T) {
	sysMsg := types.Message{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{{Type: "text", Text: "System prompt."}},
	}
	result := AppendWorkingMemory(sysMsg, nil)
	if result.Content[0].Text != "System prompt." {
		t.Error("empty ops should return unchanged system message")
	}
}

func TestAppendWorkingMemory_DoesNotMutateOriginal(t *testing.T) {
	sysMsg := types.Message{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{{Type: "text", Text: "Original."}},
	}
	ops := []Operation{{Intent: "do something", Outcome: "Done"}}
	_ = AppendWorkingMemory(sysMsg, ops)
	if sysMsg.Content[0].Text != "Original." {
		t.Error("should not mutate original message")
	}
}

func TestAppendWorkingMemory_TruncatesLongOutcome(t *testing.T) {
	sysMsg := types.Message{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{{Type: "text", Text: "System."}},
	}
	longOutcome := strings.Repeat("word ", 100)
	ops := []Operation{{Intent: "big task", Outcome: longOutcome}}
	result := AppendWorkingMemory(sysMsg, ops)
	// The one-liner outcome should be truncated, not the full 500 chars
	if len(result.Content[0].Text) > len("System.")+300 {
		t.Error("one-liner should truncate long outcomes")
	}
}

func TestRenderWorkingMemoryOneLiner_Full(t *testing.T) {
	op := Operation{
		Intent:  "fix auth bug",
		Outcome: "Tests passing",
		Files:   []string{"auth.go", "auth_test.go"},
	}
	line := RenderWorkingMemoryOneLiner(op)
	if !strings.Contains(line, "- fix auth bug") {
		t.Error("should start with '- <intent>'")
	}
	if !strings.Contains(line, "[Tests passing]") {
		t.Error("should contain outcome in brackets")
	}
	if !strings.Contains(line, "(auth.go, auth_test.go)") {
		t.Error("should contain files in parens")
	}
}

func TestRenderWorkingMemoryOneLiner_NoOutcome(t *testing.T) {
	op := Operation{Intent: "explore code", Files: []string{"main.go"}}
	line := RenderWorkingMemoryOneLiner(op)
	if strings.Contains(line, "[]") {
		t.Error("should not have empty brackets when no outcome")
	}
	if !strings.Contains(line, "(main.go)") {
		t.Error("should contain files")
	}
}

func TestRenderWorkingMemoryOneLiner_TruncatesAt80(t *testing.T) {
	longOutcome := strings.Repeat("x", 120)
	op := Operation{Intent: "task", Outcome: longOutcome}
	line := RenderWorkingMemoryOneLiner(op)
	if !strings.Contains(line, "...") {
		t.Error("should truncate long outcome with ellipsis")
	}
	// The truncated outcome should be 80 chars + "..."
	if strings.Contains(line, longOutcome) {
		t.Error("should not contain full long outcome")
	}
}

// --- Boundary detection tests ---

func TestGroupOperations_SameFilesSameOp(t *testing.T) {
	// Two user messages working on the same files should stay in one operation
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("fix auth.go")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "read", Input: map[string]any{"file_path": "auth.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "code here"},
		}},
		{Role: types.RoleAssistant, Content: textContent("I see the issue.")},
		{Role: types.RoleUser, Content: textContent("looks good, now edit it")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t2", Name: "edit", Input: map[string]any{"file_path": "auth.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t2", Content: "edited"},
		}},
		{Role: types.RoleAssistant, Content: textContent("Fixed.")},
	}

	ops := GroupOperations(messages)
	// "looks good, now edit it" works on same file (auth.go) — should stay in same op
	// The boundary detector should see file overlap > 0.2 and no tool transition
	if len(ops) != 1 {
		t.Errorf("same-file continuation should be 1 op, got %d", len(ops))
	}
}

func TestGroupOperations_DifferentFilesNewOp(t *testing.T) {
	// Two user messages working on completely different files should be separate operations
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("fix auth.go")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "edit", Input: map[string]any{"file_path": "auth.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "done"},
		}},
		{Role: types.RoleAssistant, Content: textContent("Fixed auth.")},
		{Role: types.RoleUser, Content: textContent("now work on logging")},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t2", Name: "edit", Input: map[string]any{"file_path": "logger.go"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t2", Content: "done"},
		}},
		{Role: types.RoleAssistant, Content: textContent("Added logging.")},
	}

	ops := GroupOperations(messages)
	if len(ops) < 2 {
		t.Errorf("different-file tasks should be separate ops, got %d", len(ops))
	}
}

func TestGroupOperations_IntentSignalBoundary(t *testing.T) {
	// Assistant says "now let's move on" → should trigger boundary
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: textContent("system")},
		{Role: types.RoleUser, Content: textContent("do task A")},
		{Role: types.RoleAssistant, Content: textContent("Done with A. Now let's move on to something else.")},
		{Role: types.RoleUser, Content: textContent("do task B")},
		{Role: types.RoleAssistant, Content: textContent("Done with B.")},
	}

	ops := GroupOperations(messages)
	if len(ops) < 2 {
		t.Errorf("intent transition phrase should trigger boundary, got %d ops", len(ops))
	}
}

// --- Causal dependency scoring tests ---

func TestScoreOperation_CausalDependencyBoost(t *testing.T) {
	op := Operation{
		Files: []string{"auth.go"},
		Tools: []string{"read"},
	}
	laterOps := []Operation{
		{Files: []string{"auth.go"}, Tools: []string{"edit"}, Intent: "fix the bug found earlier"},
	}
	activeFiles := []string{}

	withCausal := ScoreOperation(op, 5, 10, activeFiles, laterOps)
	withoutCausal := ScoreOperation(op, 5, 10, activeFiles)

	if withCausal <= withoutCausal {
		t.Errorf("causal dependency should boost score: with=%.3f, without=%.3f", withCausal, withoutCausal)
	}
}

func TestScoreOperation_NoCausalNoDifference(t *testing.T) {
	op := Operation{
		Files: []string{"readme.md"},
		Tools: []string{"read"},
	}
	laterOps := []Operation{
		{Files: []string{"auth.go"}, Tools: []string{"edit"}},
	}
	activeFiles := []string{}

	withLater := ScoreOperation(op, 5, 10, activeFiles, laterOps)
	withoutLater := ScoreOperation(op, 5, 10, activeFiles)

	// No file overlap, no keyword match — causal should be 0, minimal difference
	diff := withLater - withoutLater
	if diff > 0.05 {
		t.Errorf("no causal link should have minimal impact: diff=%.3f", diff)
	}
}
