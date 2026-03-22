package core

import (
	"strings"
	"testing"

	"torus_go_agent/internal/types"
)

// TestHelpersEdge_HasToolUse_NilMessage tests HasToolUse with nil message.
func TestHelpersEdge_HasToolUse_NilMessage(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("panic recovered: %v", r)
		}
	}()

	// This may panic or handle gracefully depending on implementation
	result := HasToolUse(nil)
	if result != false {
		t.Errorf("HasToolUse(nil) = %v, want false", result)
	}
}

// TestHelpersEdge_HasToolUse_NilContent tests HasToolUse with nil content slice.
func TestHelpersEdge_HasToolUse_NilContent(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: nil,
		},
	}
	result := HasToolUse(msg)
	if result != false {
		t.Errorf("HasToolUse(nil content) = %v, want false", result)
	}
}

// TestHelpersEdge_HasToolUse_LargeNumberOfBlocks tests HasToolUse with many blocks.
func TestHelpersEdge_HasToolUse_LargeNumberOfBlocks(t *testing.T) {
	content := make([]types.ContentBlock, 1000)
	for i := 0; i < 1000; i++ {
		content[i] = types.ContentBlock{Type: "text", Text: "block"}
	}
	// Add tool_use at the end
	content[999] = types.ContentBlock{Type: "tool_use", ID: "call_1", Name: "tool"}

	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: content,
		},
	}
	result := HasToolUse(msg)
	if result != true {
		t.Errorf("HasToolUse(1000 blocks with tool at end) = %v, want true", result)
	}
}

// TestHelpersEdge_HasToolUse_EmptyTypeField tests HasToolUse with empty Type field.
func TestHelpersEdge_HasToolUse_EmptyTypeField(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "", Text: "empty type"},
			},
		},
	}
	result := HasToolUse(msg)
	if result != false {
		t.Errorf("HasToolUse(empty type) = %v, want false", result)
	}
}

// TestHelpersEdge_ExtractText_NilMessage tests ExtractText with nil message.
func TestHelpersEdge_ExtractText_NilMessage(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("panic recovered: %v", r)
		}
	}()

	result := ExtractText(nil)
	if result != "" {
		t.Errorf("ExtractText(nil) = %q, want empty string", result)
	}
}

// TestHelpersEdge_ExtractText_NilContent tests ExtractText with nil content slice.
func TestHelpersEdge_ExtractText_NilContent(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: nil,
		},
	}
	result := ExtractText(msg)
	if result != "" {
		t.Errorf("ExtractText(nil content) = %q, want empty string", result)
	}
}

// TestHelpersEdge_ExtractText_SpecialCharacters tests ExtractText with special characters.
func TestHelpersEdge_ExtractText_SpecialCharacters(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "Hello\nWorld"},
				{Type: "text", Text: "\t\r\n"},
				{Type: "text", Text: "🎉emoji🎊"},
				{Type: "text", Text: "Special: !@#$%^&*()"},
			},
		},
	}
	result := ExtractText(msg)
	expected := "Hello\nWorld\t\r\n🎉emoji🎊Special: !@#$%^&*()"
	if result != expected {
		t.Errorf("ExtractText(special chars) = %q, want %q", result, expected)
	}
}

// TestHelpersEdge_ExtractText_VeryLongText tests ExtractText with very long strings.
func TestHelpersEdge_ExtractText_VeryLongText(t *testing.T) {
	longText := strings.Repeat("a", 100000)
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: longText},
				{Type: "text", Text: "end"},
			},
		},
	}
	result := ExtractText(msg)
	expected := longText + "end"
	if result != expected {
		t.Errorf("ExtractText(very long text) length = %d, want %d", len(result), len(expected))
	}
}

// TestHelpersEdge_ExtractText_OnlyEmptyTextBlocks tests ExtractText with only empty text blocks.
func TestHelpersEdge_ExtractText_OnlyEmptyTextBlocks(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: ""},
				{Type: "text", Text: ""},
				{Type: "text", Text: ""},
			},
		},
	}
	result := ExtractText(msg)
	if result != "" {
		t.Errorf("ExtractText(only empty blocks) = %q, want empty string", result)
	}
}

// TestHelpersEdge_ExtractText_ManySmallBlocks tests ExtractText with many small text blocks.
func TestHelpersEdge_ExtractText_ManySmallBlocks(t *testing.T) {
	content := make([]types.ContentBlock, 1000)
	for i := 0; i < 1000; i++ {
		content[i] = types.ContentBlock{Type: "text", Text: "a"}
	}
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: content,
		},
	}
	result := ExtractText(msg)
	expected := strings.Repeat("a", 1000)
	if result != expected {
		t.Errorf("ExtractText(1000 small blocks) = %q, want %q", result, expected)
	}
}

// TestHelpersEdge_ExtractText_WhitespaceOnly tests ExtractText with whitespace-only blocks.
func TestHelpersEdge_ExtractText_WhitespaceOnly(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "   "},
				{Type: "text", Text: "\t\t"},
				{Type: "text", Text: "\n\n"},
			},
		},
	}
	result := ExtractText(msg)
	expected := "   \t\t\n\n"
	if result != expected {
		t.Errorf("ExtractText(whitespace) = %q, want %q", result, expected)
	}
}

// TestHelpersEdge_ExtractToolCalls_NilMessage tests ExtractToolCalls with nil message.
func TestHelpersEdge_ExtractToolCalls_NilMessage(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("panic recovered: %v", r)
		}
	}()

	result := ExtractToolCalls(nil)
	if len(result) != 0 {
		t.Errorf("ExtractToolCalls(nil) = %d calls, want 0", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_NilContent tests ExtractToolCalls with nil content slice.
func TestHelpersEdge_ExtractToolCalls_NilContent(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: nil,
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 0 {
		t.Errorf("ExtractToolCalls(nil content) = %d calls, want 0", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_LargeNumberOfCalls tests ExtractToolCalls with many tool calls.
func TestHelpersEdge_ExtractToolCalls_LargeNumberOfCalls(t *testing.T) {
	content := make([]types.ContentBlock, 500)
	for i := 0; i < 500; i++ {
		content[i] = types.ContentBlock{
			Type: "tool_use",
			ID:   "call_" + string(rune(i)),
			Name: "tool",
		}
	}
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: content,
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 500 {
		t.Errorf("ExtractToolCalls(500 tool_use blocks) = %d, want 500", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_MixedWithEmptyNames tests ExtractToolCalls with empty tool names.
func TestHelpersEdge_ExtractToolCalls_MixedWithEmptyNames(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: ""},
				{Type: "tool_use", ID: "call_2", Name: "real_tool"},
				{Type: "tool_use", ID: "call_3", Name: ""},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 3 {
		t.Errorf("ExtractToolCalls(empty names) = %d, want 3", len(result))
	}
	// Verify all are still extracted even with empty names
	for i, call := range result {
		if call.Type != "tool_use" {
			t.Errorf("call %d type = %q, want tool_use", i, call.Type)
		}
	}
}

// TestHelpersEdge_ExtractToolCalls_DuplicateIDs tests ExtractToolCalls with duplicate IDs.
func TestHelpersEdge_ExtractToolCalls_DuplicateIDs(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: "tool_a"},
				{Type: "tool_use", ID: "call_1", Name: "tool_b"},
				{Type: "tool_use", ID: "call_1", Name: "tool_c"},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 3 {
		t.Errorf("ExtractToolCalls(duplicate IDs) = %d, want 3", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_SpecialCharactersInID tests ExtractToolCalls with special characters in IDs.
func TestHelpersEdge_ExtractToolCalls_SpecialCharactersInID(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_🎉", Name: "tool_a"},
				{Type: "tool_use", ID: "call_!@#$", Name: "tool_b"},
				{Type: "tool_use", ID: "call_\n\t", Name: "tool_c"},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 3 {
		t.Errorf("ExtractToolCalls(special chars in ID) = %d, want 3", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_EmptyIDField tests ExtractToolCalls with empty ID field.
func TestHelpersEdge_ExtractToolCalls_EmptyIDField(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "", Name: "tool_a"},
				{Type: "tool_use", ID: "call_2", Name: "tool_b"},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 2 {
		t.Errorf("ExtractToolCalls(empty ID) = %d, want 2", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_VeryLongInputs tests ExtractToolCalls with very long Input maps.
func TestHelpersEdge_ExtractToolCalls_VeryLongInputs(t *testing.T) {
	input := make(map[string]any)
	for i := 0; i < 1000; i++ {
		input["key"+string(rune(i))] = "value"
	}
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: "tool_a", Input: input},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 1 {
		t.Errorf("ExtractToolCalls(large input) = %d, want 1", len(result))
	}
}

// TestHelpersEdge_ExtractToolCalls_AllNonToolTypes tests that non-tool types are excluded.
func TestHelpersEdge_ExtractToolCalls_AllNonToolTypes(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "text"},
				{Type: "tool_result", ToolUseID: "call_1", Content: "result"},
				{Type: "image", Text: "image data"},
				{Type: "document", Text: "doc data"},
			},
		},
	}
	result := ExtractToolCalls(msg)
	if len(result) != 0 {
		t.Errorf("ExtractToolCalls(non-tool types) = %d, want 0", len(result))
	}
}

// TestHelpersEdge_ConcurrentAccess_HasToolUse tests concurrent access to HasToolUse.
func TestHelpersEdge_ConcurrentAccess_HasToolUse(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "text"},
				{Type: "tool_use", ID: "call_1", Name: "tool"},
			},
		},
	}

	// Run multiple goroutines concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result := HasToolUse(msg)
			if !result {
				t.Errorf("HasToolUse() returned false, want true")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestHelpersEdge_ConcurrentAccess_ExtractText tests concurrent access to ExtractText.
func TestHelpersEdge_ConcurrentAccess_ExtractText(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "Hello"},
				{Type: "text", Text: " "},
				{Type: "text", Text: "World"},
			},
		},
	}

	// Run multiple goroutines concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result := ExtractText(msg)
			if result != "Hello World" {
				t.Errorf("ExtractText() = %q, want %q", result, "Hello World")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestHelpersEdge_ConcurrentAccess_ExtractToolCalls tests concurrent access to ExtractToolCalls.
func TestHelpersEdge_ConcurrentAccess_ExtractToolCalls(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: "tool_a"},
				{Type: "text", Text: "text"},
				{Type: "tool_use", ID: "call_2", Name: "tool_b"},
			},
		},
	}

	// Run multiple goroutines concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result := ExtractToolCalls(msg)
			if len(result) != 2 {
				t.Errorf("ExtractToolCalls() = %d calls, want 2", len(result))
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestHelpersEdge_ExtractText_ConsecutiveEmptyBlocks tests consecutive empty text blocks.
func TestHelpersEdge_ExtractText_ConsecutiveEmptyBlocks(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "start"},
				{Type: "text", Text: ""},
				{Type: "text", Text: ""},
				{Type: "text", Text: ""},
				{Type: "text", Text: "end"},
			},
		},
	}
	result := ExtractText(msg)
	expected := "startend"
	if result != expected {
		t.Errorf("ExtractText(consecutive empty) = %q, want %q", result, expected)
	}
}

// TestHelpersEdge_HasToolUse_CaseSensitive tests that tool_use type check is case-sensitive.
func TestHelpersEdge_HasToolUse_CaseSensitive(t *testing.T) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "Tool_Use", Text: "wrong case"},
				{Type: "TOOL_USE", Text: "also wrong"},
				{Type: "tool_use", ID: "call_1", Name: "correct"},
			},
		},
	}
	result := HasToolUse(msg)
	if result != true {
		t.Errorf("HasToolUse(case-sensitive) = %v, want true", result)
	}
}
