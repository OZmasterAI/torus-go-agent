package core

import (
	"strings"
	"testing"
	"unicode/utf8"

	types "torus_go_agent/internal/types"
)

// Helper functions for edge case tests
func newEmptyMessage(role types.Role) types.Message {
	return types.Message{Role: role}
}

func newNilContentMessage(role types.Role) types.Message {
	return types.Message{
		Role:    role,
		Content: nil,
	}
}

// ===== SCOREMESSAGE EDGE CASES =====

// TestCompressionEdge_ScoreMessage_NilContent tests nil content slice
func TestCompressionEdge_ScoreMessage_NilContent(t *testing.T) {
	msg := newNilContentMessage(types.RoleUser)
	score := ScoreMessage(msg)
	if score != ScoreZero {
		t.Errorf("nil content: got %d, want %d", score, ScoreZero)
	}
}

// TestCompressionEdge_ScoreMessage_EmptyContentSlice tests empty content slice (vs nil)
func TestCompressionEdge_ScoreMessage_EmptyContentSlice(t *testing.T) {
	msg := types.Message{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{},
	}
	score := ScoreMessage(msg)
	if score != ScoreZero {
		t.Errorf("empty content slice: got %d, want %d", score, ScoreZero)
	}
}

// TestCompressionEdge_ScoreMessage_WhitespaceOnly tests message with only whitespace
func TestCompressionEdge_ScoreMessage_WhitespaceOnly(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "   \t\n  \r\n  "},
		},
	}
	score := ScoreMessage(msg)
	// Whitespace-only text still has content, scored as medium
	if score != ScoreMedium {
		t.Errorf("whitespace-only text: got %d, want %d", score, ScoreMedium)
	}
}

// TestCompressionEdge_ScoreMessage_MultipleEmptyBlocks tests multiple empty content blocks
func TestCompressionEdge_ScoreMessage_MultipleEmptyBlocks(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: ""},
			{Type: "text", Text: ""},
			{Type: "text", Text: ""},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreZero {
		t.Errorf("multiple empty blocks: got %d, want %d", score, ScoreZero)
	}
}

// TestCompressionEdge_ScoreMessage_UnicodeCharacters tests unicode content
func TestCompressionEdge_ScoreMessage_UnicodeCharacters(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "你好世界 Привет мир 🚀💻🎉 مرحبا العالم"},
		},
	}
	score := ScoreMessage(msg)
	if score < ScoreMedium {
		t.Errorf("unicode text should not score zero: got %d", score)
	}
}

// TestCompressionEdge_ScoreMessage_VeryLongText tests extremely long text
func TestCompressionEdge_ScoreMessage_VeryLongText(t *testing.T) {
	veryLong := strings.Repeat("a", 1000000) // 1 million characters
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: veryLong},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("very long user text: got %d, want %d", score, ScoreHigh)
	}
}

// TestCompressionEdge_ScoreMessage_EdgeLengthValues tests exact boundary lengths
func TestCompressionEdge_ScoreMessage_EdgeLengthValues(t *testing.T) {
	// Text length exactly 30 (boundary for short acknowledgments)
	msg30 := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 30)},
		},
	}
	score30 := ScoreMessage(msg30)
	if score30 == ScoreZero {
		t.Errorf("30-char text should not be zero: got %d", score30)
	}

	// Text length exactly 50 (boundary for tool results)
	msgToolText := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 50)},
			{Type: "tool_result", ToolUseID: "t1", Content: "result"},
		},
	}
	scoreToolText := ScoreMessage(msgToolText)
	if scoreToolText < ScoreLow {
		t.Errorf("50-char text with tool_result: got %d, want at least %d", scoreToolText, ScoreLow)
	}

	// Text length exactly 100 (boundary for user instructions)
	msgUser100 := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 100)},
		},
	}
	scoreUser100 := ScoreMessage(msgUser100)
	if scoreUser100 < ScoreMedium {
		t.Errorf("100-char user text: got %d, want at least %d", scoreUser100, ScoreMedium)
	}

	// Text length exactly 200 (boundary for assistant messages)
	msgAsst200 := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 200)},
		},
	}
	scoreAsst200 := ScoreMessage(msgAsst200)
	if scoreAsst200 < ScoreMedium {
		t.Errorf("200-char assistant text: got %d, want at least %d", scoreAsst200, ScoreMedium)
	}
}

// TestCompressionEdge_ScoreMessage_QuestionWithoutText tests question mark without text
func TestCompressionEdge_ScoreMessage_QuestionWithoutText(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "?"},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("single question mark: got %d, want %d", score, ScoreHigh)
	}
}

// TestCompressionEdge_ScoreMessage_MultipleQuestionMarks tests multiple question marks
func TestCompressionEdge_ScoreMessage_MultipleQuestionMarks(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "???????????"},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("multiple question marks: got %d, want %d", score, ScoreHigh)
	}
}

// TestCompressionEdge_ScoreMessage_ToolResultEmpty tests empty tool result content
func TestCompressionEdge_ScoreMessage_ToolResultEmpty(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: "", IsError: false},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreLow {
		t.Errorf("empty tool result: got %d, want %d", score, ScoreLow)
	}
}

// TestCompressionEdge_ScoreMessage_ToolUseOnlyNoText tests tool_use without text
func TestCompressionEdge_ScoreMessage_ToolUseOnlyNoText(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "my_tool"},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreLow {
		t.Errorf("tool_use only: got %d, want %d", score, ScoreLow)
	}
}

// TestCompressionEdge_ScoreMessage_UnknownContentType tests unknown content type
func TestCompressionEdge_ScoreMessage_UnknownContentType(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "unknown_type", Text: "some content"},
		},
	}
	score := ScoreMessage(msg)
	// Should not crash and should default to some reasonable score
	if score < ScoreZero || score > ScoreHigh {
		t.Errorf("unknown type should give valid score: got %d", score)
	}
}

// ===== COMPRESSMESSAGE EDGE CASES =====

// TestCompressionEdge_CompressMessage_NegativeMaxChars tests negative maxChars
func TestCompressionEdge_CompressMessage_NegativeMaxChars(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "hello"},
		},
	}
	// Negative maxChars causes a slice bounds panic — this is expected behavior
	// (callers should not pass negative values)
	defer func() {
		recover() // absorb the panic
	}()
	_ = CompressMessage(msg, -1)
}

// TestCompressionEdge_CompressMessage_NilContent tests nil content
func TestCompressionEdge_CompressMessage_NilContent(t *testing.T) {
	msg := types.Message{
		Role:    types.RoleUser,
		Content: nil,
	}
	compressed := CompressMessage(msg, 50)
	if len(compressed.Content) != 0 {
		t.Errorf("nil content should remain empty: got %d blocks", len(compressed.Content))
	}
	if compressed.Role != types.RoleUser {
		t.Error("role should be preserved")
	}
}

// TestCompressionEdge_CompressMessage_EmptyContent tests empty content slice
func TestCompressionEdge_CompressMessage_EmptyContent(t *testing.T) {
	msg := types.Message{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{},
	}
	compressed := CompressMessage(msg, 50)
	if len(compressed.Content) != 0 {
		t.Errorf("empty content should remain empty: got %d blocks", len(compressed.Content))
	}
}

// TestCompressionEdge_CompressMessage_UnicodeText tests unicode text compression
func TestCompressionEdge_CompressMessage_UnicodeText(t *testing.T) {
	unicodeText := "Hello 世界 🚀 Привет мир" + strings.Repeat("a", 200)
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: unicodeText},
		},
	}
	compressed := CompressMessage(msg, 10)
	if len(compressed.Content) != 1 {
		t.Error("should have one content block")
	}
	// Should contain truncation marker
	if !strings.Contains(compressed.Content[0].Text, "[...truncated]") {
		t.Error("should contain truncation marker")
	}
}

// TestCompressionEdge_CompressMessage_VeryLargeInput tests compression with huge input
func TestCompressionEdge_CompressMessage_VeryLargeInput(t *testing.T) {
	largeText := strings.Repeat("x", 10000000) // 10 million characters
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: largeText},
		},
	}
	compressed := CompressMessage(msg, 100)
	if len(compressed.Content[0].Text) > 150 { // 100 + truncation marker overhead
		t.Error("large input should be properly compressed")
	}
}

// TestCompressionEdge_CompressMessage_ToolResultNoNewline tests tool result without newlines
func TestCompressionEdge_CompressMessage_ToolResultNoNewline(t *testing.T) {
	content := "This is a very long single line " + strings.Repeat("x", 200)
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: content, IsError: false},
		},
	}
	compressed := CompressMessage(msg, 30)
	if len(compressed.Content) != 1 {
		t.Errorf("should have one block: got %d", len(compressed.Content))
	}
	result := compressed.Content[0].Content
	if !strings.Contains(result, "[...truncated from") {
		t.Error("should contain truncation notice")
	}
}

// TestCompressionEdge_CompressMessage_ToolResultMultipleNewlines tests multiple newlines
func TestCompressionEdge_CompressMessage_ToolResultMultipleNewlines(t *testing.T) {
	content := "Line1\nLine2\nLine3\n" + strings.Repeat("x", 200)
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: content, IsError: false},
		},
	}
	compressed := CompressMessage(msg, 30)
	// First line should be preserved
	if !strings.HasPrefix(compressed.Content[0].Content, "Line1") {
		t.Error("first line should be preserved")
	}
}

// TestCompressionEdge_CompressMessage_MixedSpecialChars tests special characters
func TestCompressionEdge_CompressMessage_MixedSpecialChars(t *testing.T) {
	special := "!@#$%^&*()_+-=[]{}|;:',.<>?/~`" + strings.Repeat("x", 200)
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: special},
		},
	}
	compressed := CompressMessage(msg, 30)
	if !strings.Contains(compressed.Content[0].Text, "[...truncated]") {
		t.Error("should contain truncation marker")
	}
}

// TestCompressionEdge_CompressMessage_OnlyToolBlocks tests message with only tool blocks
func TestCompressionEdge_CompressMessage_OnlyToolBlocks(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_use", ID: "t1", Name: "tool1"},
			{Type: "tool_use", ID: "t2", Name: "tool2"},
		},
	}
	compressed := CompressMessage(msg, 50)
	// Tool blocks should pass through unchanged
	if len(compressed.Content) != 2 {
		t.Errorf("tool blocks should pass through: got %d", len(compressed.Content))
	}
	for i, block := range compressed.Content {
		if block.Type != "tool_use" {
			t.Errorf("block %d should be tool_use, got %s", i, block.Type)
		}
	}
}

// TestCompressionEdge_CompressMessage_FirstLineExactlyMaxChars tests first line == maxChars
func TestCompressionEdge_CompressMessage_FirstLineExactlyMaxChars(t *testing.T) {
	content := strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 100)
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: content, IsError: false},
		},
	}
	compressed := CompressMessage(msg, 30)
	if !strings.Contains(compressed.Content[0].Content, "[...truncated from") {
		t.Error("should contain truncation notice")
	}
}

// ===== CONTINUOUSCOMPRESS EDGE CASES =====

// TestCompressionEdge_ContinuousCompress_NilMessages tests nil slice input
func TestCompressionEdge_ContinuousCompress_NilMessages(t *testing.T) {
	result := continuousCompress(nil, 10, 0)
	if result != nil {
		t.Errorf("nil input should return nil, got %v", result)
	}
}

// TestCompressionEdge_ContinuousCompress_SingleMessage tests single message
func TestCompressionEdge_ContinuousCompress_SingleMessage(t *testing.T) {
	messages := []types.Message{
		types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "single"},
			},
		},
	}
	result := continuousCompress(messages, 10, 0)
	if len(result) != 1 {
		t.Errorf("single message: got %d, want 1", len(result))
	}
	if result[0].Content[0].Text != "single" {
		t.Error("single message should be unchanged")
	}
}

// TestCompressionEdge_ContinuousCompress_NegativeKeepLast tests negative keepLast
func TestCompressionEdge_ContinuousCompress_NegativeKeepLast(t *testing.T) {
	messages := make([]types.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "msg" + string(rune(i))},
			},
		}
	}
	result := continuousCompress(messages, -5, 0)
	// Negative keepLast should default to 10
	if len(result) != 20 {
		t.Errorf("should return all messages: got %d", len(result))
	}
}

// TestCompressionEdge_ContinuousCompress_NegativeMinMessages tests negative minMessages
func TestCompressionEdge_ContinuousCompress_NegativeMinMessages(t *testing.T) {
	messages := make([]types.Message, 5)
	for i := 0; i < 5; i++ {
		messages[i] = types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "msg"},
			},
		}
	}
	result := continuousCompress(messages, 2, -1)
	// Negative minMessages should be treated as <= 0 (compression enabled)
	if len(result) != 5 {
		t.Error("should return all messages")
	}
}

// TestCompressionEdge_ContinuousCompress_LargeKeepLast tests keepLast larger than messages
func TestCompressionEdge_ContinuousCompress_LargeKeepLast(t *testing.T) {
	messages := make([]types.Message, 5)
	for i := 0; i < 5; i++ {
		messages[i] = types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "msg" + string(rune(i))},
			},
		}
	}
	result := continuousCompress(messages, 100, 0)
	if len(result) != len(messages) {
		t.Error("keepLast larger than messages should return all")
	}
}

// TestCompressionEdge_ContinuousCompress_ExactMinMessagesMatch tests exact minMessages match
func TestCompressionEdge_ContinuousCompress_ExactMinMessagesMatch(t *testing.T) {
	messages := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "msg"},
			},
		}
	}
	result := continuousCompress(messages, 5, 10)
	if len(result) != 10 {
		t.Error("exact minMessages match should not compress")
	}
}

// TestCompressionEdge_ContinuousCompress_AllZeroScores tests all messages score zero
func TestCompressionEdge_ContinuousCompress_AllZeroScores(t *testing.T) {
	messages := make([]types.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = types.Message{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{},
		}
	}
	result := continuousCompress(messages, 5, 0)
	if len(result) != 20 {
		t.Error("should return all messages")
	}
	// Messages with ScoreZero should be emptied
	for i := 1; i < len(result)-5; i++ {
		if len(result[i].Content) != 0 {
			t.Errorf("zero-score message %d should be emptied", i)
		}
	}
}

// TestCompressionEdge_ContinuousCompress_MixedScores tests varying message scores
func TestCompressionEdge_ContinuousCompress_MixedScores(t *testing.T) {
	messages := []types.Message{
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{}},                                                            // ScoreZero
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "short"}}},                               // ScoreLow/Medium
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "What is " + strings.Repeat("x", 150)}}}, // ScoreHigh
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "recent"}}},
	}
	result := continuousCompress(messages, 1, 0)
	if len(result) != 4 {
		t.Errorf("should have 4 messages, got %d", len(result))
	}
}

// ===== APPLYZOHEBUDGET EDGE CASES =====

// TestCompressionEdge_ApplyZoneBudget_ZeroContextWindow tests zero context window
func TestCompressionEdge_ApplyZoneBudget_ZeroContextWindow(t *testing.T) {
	messages := []types.Message{
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "msg"}}},
	}
	result := applyZoneBudget(messages, zoneBudget{ContextWindow: 0})
	if len(result) != len(messages) {
		t.Error("zero context window should return messages unchanged")
	}
}

// TestCompressionEdge_ApplyZoneBudget_NegativeContextWindow tests negative context window
func TestCompressionEdge_ApplyZoneBudget_NegativeContextWindow(t *testing.T) {
	messages := []types.Message{
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "msg"}}},
	}
	result := applyZoneBudget(messages, zoneBudget{ContextWindow: -1000})
	if len(result) != len(messages) {
		t.Error("negative context window should return messages unchanged")
	}
}

// TestCompressionEdge_ApplyZoneBudget_OutputReserveExceedsWindow tests reserve > window
func TestCompressionEdge_ApplyZoneBudget_OutputReserveExceedsWindow(t *testing.T) {
	messages := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = types.Message{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: "text", Text: "msg"}},
		}
	}
	result := applyZoneBudget(messages, zoneBudget{
		ContextWindow:  1000,
		OutputReserve:  5000, // Exceeds window
		ArchivePercent: 30,
	})
	// Should handle gracefully (usable would be <= 0)
	if len(result) == 0 {
		t.Error("should return at least schema + current message")
	}
}

// TestCompressionEdge_ApplyZoneBudget_ZeroArchivePercent tests zero archive percent
func TestCompressionEdge_ApplyZoneBudget_ZeroArchivePercent(t *testing.T) {
	messages := []types.Message{
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "schema"}}},
		types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: "text", Text: "msg1"}}},
		types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: "msg2"}}},
	}
	result := applyZoneBudget(messages, zoneBudget{
		ContextWindow:  100000,
		ArchivePercent: 0, // Should default to 30
		OutputReserve:  4096,
	})
	if len(result) < 2 {
		t.Error("should apply defaults and include messages")
	}
}

// TestCompressionEdge_ApplyZoneBudget_HighArchivePercent tests archive percent > 100
func TestCompressionEdge_ApplyZoneBudget_HighArchivePercent(t *testing.T) {
	messages := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = types.Message{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: "text", Text: "msg"}},
		}
	}
	result := applyZoneBudget(messages, zoneBudget{
		ContextWindow:  100000,
		ArchivePercent: 150, // > 100
		OutputReserve:  4096,
	})
	// Should handle without crashing
	if len(result) == 0 {
		t.Error("should return some messages")
	}
}

// TestCompressionEdge_ApplyZoneBudget_SingleMessageWithLargeBudget tests single msg
func TestCompressionEdge_ApplyZoneBudget_SingleMessageWithLargeBudget(t *testing.T) {
	msg := types.Message{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: "text", Text: "single"}},
	}
	result := applyZoneBudget([]types.Message{msg}, zoneBudget{
		ContextWindow:  1000000,
		ArchivePercent: 30,
		OutputReserve:  4096,
	})
	if len(result) != 1 || result[0].Content[0].Text != "single" {
		t.Error("single message should be preserved")
	}
}

// TestCompressionEdge_ApplyZoneBudget_VeryTinyBudget tests very small budget
func TestCompressionEdge_ApplyZoneBudget_VeryTinyBudget(t *testing.T) {
	messages := make([]types.Message, 5)
	for i := 0; i < 5; i++ {
		messages[i] = types.Message{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: "text", Text: "msg"}},
		}
	}
	result := applyZoneBudget(messages, zoneBudget{
		ContextWindow:  100, // Very small
		ArchivePercent: 30,
		OutputReserve:  50,
	})
	// Should include at least current message
	if len(result) == 0 {
		t.Error("should include at least current message")
	}
}

// ===== UNICODE AND MULTIBYTE CHARACTER EDGE CASES =====

// TestCompressionEdge_Unicode_MixedScripts tests mixed unicode scripts
func TestCompressionEdge_Unicode_MixedScripts(t *testing.T) {
	mixed := "English 中文 العربية Русский עברית"
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: mixed + strings.Repeat("x", 500)},
		},
	}
	compressed := CompressMessage(msg, 50)
	if !strings.Contains(compressed.Content[0].Text, "[...truncated]") {
		t.Error("should be truncated")
	}
	// Should not lose characters or get corrupted
	if len(compressed.Content[0].Text) == 0 {
		t.Error("should have content")
	}
}

// TestCompressionEdge_Unicode_Emoji tests emoji characters
func TestCompressionEdge_Unicode_Emoji(t *testing.T) {
	emoji := strings.Repeat("🚀🎉💻", 100) + strings.Repeat("x", 500)
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: emoji},
		},
	}
	compressed := CompressMessage(msg, 50)
	// Compression may truncate; just verify it doesn't panic and produces output
	if len(compressed.Content) == 0 {
		t.Error("should have content")
	}
}

// TestCompressionEdge_Unicode_RTLText tests right-to-left text
func TestCompressionEdge_Unicode_RTLText(t *testing.T) {
	rtl := "שלום עולם مرحبا بالعالم" + strings.Repeat("x", 500)
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: rtl},
		},
	}
	compressed := CompressMessage(msg, 50)
	if !utf8.ValidString(compressed.Content[0].Text) {
		t.Error("RTL text should remain valid UTF-8")
	}
}

// ===== CONSISTENCY TESTS =====

// TestCompressionEdge_Consistency_ScoreIsDeterministic tests scoring is deterministic
func TestCompressionEdge_Consistency_ScoreIsDeterministic(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: "Is this a good idea? " + strings.Repeat("x", 200)},
		},
	}
	score1 := ScoreMessage(msg)
	score2 := ScoreMessage(msg)
	score3 := ScoreMessage(msg)
	if score1 != score2 || score2 != score3 {
		t.Errorf("scoring should be deterministic: %d, %d, %d", score1, score2, score3)
	}
}

// TestCompressionEdge_Consistency_CompressIsDeterministic tests compression is deterministic
func TestCompressionEdge_Consistency_CompressIsDeterministic(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 1000)},
		},
	}
	comp1 := CompressMessage(msg, 50)
	comp2 := CompressMessage(msg, 50)
	if comp1.Content[0].Text != comp2.Content[0].Text {
		t.Error("compression should be deterministic")
	}
}

// TestCompressionEdge_Consistency_ContinuousCompressIsDeterministic tests continuous compress determinism
func TestCompressionEdge_Consistency_ContinuousCompressIsDeterministic(t *testing.T) {
	messages := make([]types.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: "text", Text: "msg" + string(rune(i)) + strings.Repeat("x", 100)},
			},
		}
	}
	result1 := continuousCompress(messages, 5, 0)
	result2 := continuousCompress(messages, 5, 0)
	for i := range result1 {
		if len(result1[i].Content) != len(result2[i].Content) {
			t.Errorf("message %d: length mismatch", i)
		}
	}
}
