package core

import (
	"strings"
	"testing"

	t "torus_go_agent/internal/types"
)

func TestMicroCompact_TruncatesOldToolResults(tt *testing.T) {
	// 15 messages: system + 4 tool exchanges (user+assistant+tool each) + 2 recent
	var msgs []t.Message
	msgs = append(msgs, t.Message{Role: t.RoleSystem, Content: []t.ContentBlock{{Type: "text", Text: "system"}}})

	// Old tool results (outside keepLast=4 window)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, t.Message{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "do something"}}})
		msgs = append(msgs, t.Message{Role: t.RoleAssistant, Content: []t.ContentBlock{
			{Type: "tool_use", ID: "tu_" + string(rune('a'+i)), Name: "grep"},
		}})
		msgs = append(msgs, t.Message{Role: t.RoleTool, Content: []t.ContentBlock{
			{Type: "tool_result", ToolUseID: "tu_" + string(rune('a'+i)), Content: strings.Repeat("match line\n", 100)},
		}})
	}

	// Recent messages (inside keepLast window)
	msgs = append(msgs, t.Message{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "recent"}}})
	msgs = append(msgs, t.Message{Role: t.RoleAssistant, Content: []t.ContentBlock{
		{Type: "tool_use", ID: "tu_recent", Name: "read"},
	}})
	msgs = append(msgs, t.Message{Role: t.RoleTool, Content: []t.ContentBlock{
		{Type: "tool_result", ToolUseID: "tu_recent", Content: strings.Repeat("file content\n", 100)},
	}})

	result := MicroCompact(msgs, 4)

	if len(result) != len(msgs) {
		tt.Fatalf("message count changed: %d -> %d", len(msgs), len(result))
	}

	// System prompt untouched
	if result[0].Content[0].Text != "system" {
		tt.Error("system prompt was modified")
	}

	// Old tool results should be truncated
	for i := 1; i < len(result)-4; i++ {
		for _, b := range result[i].Content {
			if b.Type == "tool_result" && len(b.Content) > microCompactThreshold {
				tt.Errorf("message[%d] tool_result not truncated: %d chars", i, len(b.Content))
			}
		}
	}

	// Recent tool result should be untouched
	last := result[len(result)-1]
	for _, b := range last.Content {
		if b.Type == "tool_result" && !strings.Contains(b.Content, "file content") {
			tt.Error("recent tool result was truncated")
		}
	}
}

func TestMicroCompact_SmallResultsUntouched(tt *testing.T) {
	msgs := []t.Message{
		{Role: t.RoleSystem, Content: []t.ContentBlock{{Type: "text", Text: "sys"}}},
		{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "q"}}},
		{Role: t.RoleAssistant, Content: []t.ContentBlock{{Type: "tool_use", ID: "t1", Name: "bash"}}},
		{Role: t.RoleTool, Content: []t.ContentBlock{{Type: "tool_result", ToolUseID: "t1", Content: "short output"}}},
		// keepLast window:
		{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "next"}}},
		{Role: t.RoleAssistant, Content: []t.ContentBlock{{Type: "text", Text: "ok"}}},
	}

	result := MicroCompact(msgs, 2)

	// Small tool result (< threshold) should be untouched
	for _, b := range result[3].Content {
		if b.Type == "tool_result" && b.Content != "short output" {
			tt.Errorf("small tool result was modified: %q", b.Content)
		}
	}
}

func TestMicroCompact_PreservesToolName(tt *testing.T) {
	msgs := []t.Message{
		{Role: t.RoleSystem, Content: []t.ContentBlock{{Type: "text", Text: "sys"}}},
		{Role: t.RoleAssistant, Content: []t.ContentBlock{{Type: "tool_use", ID: "t1", Name: "grep"}}},
		{Role: t.RoleTool, Content: []t.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: strings.Repeat("search result line\n", 80)},
		}},
		// keepLast=1
		{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "recent"}}},
	}

	result := MicroCompact(msgs, 1)
	for _, b := range result[2].Content {
		if b.Type == "tool_result" && !strings.Contains(b.Content, "grep:") {
			tt.Errorf("truncated result should contain tool name, got: %q", b.Content)
		}
	}
}

func TestMicroCompact_TooFewMessages(tt *testing.T) {
	msgs := []t.Message{
		{Role: t.RoleSystem, Content: []t.ContentBlock{{Type: "text", Text: "sys"}}},
		{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: "hi"}}},
	}
	result := MicroCompact(msgs, 10)
	if len(result) != len(msgs) {
		tt.Error("should return original when too few messages")
	}
}
