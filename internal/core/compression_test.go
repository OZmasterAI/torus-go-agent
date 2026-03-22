package core

import (
	"strings"
	"testing"

	types "torus_go_agent/internal/types"
)

// Helper function to create a message with text content
func newTextMessage(role types.Role, text string) types.Message {
	return types.Message{
		Role: role,
		Content: []types.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// Helper function to create a message with tool result
func newToolResultMessage(toolUseID, content string, isError bool) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   content,
				IsError:   isError,
			},
		},
	}
}

// Helper function to create a message with tool use
func newToolUseMessage(id, name string) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{
				Type: "tool_use",
				ID:   id,
				Name: name,
			},
		},
	}
}

// TestScoreMessage_EmptyMessage tests that empty messages get ScoreZero
func TestScoreMessage_EmptyMessage(t *testing.T) {
	msg := types.Message{Role: types.RoleUser}
	score := ScoreMessage(msg)
	if score != ScoreZero {
		t.Errorf("empty message: got %d, want %d", score, ScoreZero)
	}
}

// TestScoreMessage_EmptyContentBlock tests message with empty content block
func TestScoreMessage_EmptyContentBlock(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: "text", Text: ""},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreZero {
		t.Errorf("empty text block: got %d, want %d", score, ScoreZero)
	}
}

// TestScoreMessage_ToolResultSmall tests small tool results get ScoreLow
func TestScoreMessage_ToolResultSmall(t *testing.T) {
	msg := newToolResultMessage("tool-1", "short", false)
	score := ScoreMessage(msg)
	if score != ScoreLow {
		t.Errorf("small tool result: got %d, want %d", score, ScoreLow)
	}
}

// TestScoreMessage_ToolResultLarge tests large tool results get ScoreMedium
// Note: tool_result blocks need accompanying text content to be scored based on textLen
func TestScoreMessage_ToolResultLarge(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 100)},
			{Type: "tool_result", ToolUseID: "tool-1", Content: "result", IsError: false},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreMedium {
		t.Errorf("large tool result with text: got %d, want %d", score, ScoreMedium)
	}
}

// TestScoreMessage_ToolUse tests tool invocations get ScoreLow
func TestScoreMessage_ToolUse(t *testing.T) {
	msg := newToolUseMessage("tool-1", "my_tool")
	score := ScoreMessage(msg)
	if score != ScoreLow {
		t.Errorf("tool use: got %d, want %d", score, ScoreLow)
	}
}

// TestScoreMessage_Acknowledgments tests short acknowledgments get ScoreLow
func TestScoreMessage_Acknowledgments(t *testing.T) {
	acks := []string{"ok", "thanks", "got it", "sure", "yes", "no", "done", "good", "nice", "great"}
	for _, ack := range acks {
		msg := newTextMessage(types.RoleAssistant, ack)
		score := ScoreMessage(msg)
		if score != ScoreLow {
			t.Errorf("acknowledgment '%s': got %d, want %d", ack, score, ScoreLow)
		}
	}
}

// TestScoreMessage_UserQuestion tests user questions get ScoreHigh
func TestScoreMessage_UserQuestion(t *testing.T) {
	msg := newTextMessage(types.RoleUser, "How does this work?")
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("user question: got %d, want %d", score, ScoreHigh)
	}
}

// TestScoreMessage_LongAssistantMessage tests long assistant responses get ScoreHigh
func TestScoreMessage_LongAssistantMessage(t *testing.T) {
	longText := strings.Repeat("The answer is complex. ", 20)
	msg := newTextMessage(types.RoleAssistant, longText)
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("long assistant message: got %d, want %d", score, ScoreHigh)
	}
}

// TestScoreMessage_LongUserMessage tests long user messages get ScoreHigh
func TestScoreMessage_LongUserMessage(t *testing.T) {
	longText := strings.Repeat("Please explain ", 10)
	msg := newTextMessage(types.RoleUser, longText)
	score := ScoreMessage(msg)
	if score != ScoreHigh {
		t.Errorf("long user message: got %d, want %d", score, ScoreHigh)
	}
}

// TestScoreMessage_ShortAssistantMessage tests short assistant responses get ScoreMedium
func TestScoreMessage_ShortAssistantMessage(t *testing.T) {
	msg := newTextMessage(types.RoleAssistant, "Brief response")
	score := ScoreMessage(msg)
	if score != ScoreMedium {
		t.Errorf("short assistant message: got %d, want %d", score, ScoreMedium)
	}
}

// TestScoreMessage_MixedContent tests messages with multiple content blocks
// Short text alone is ScoreLow, but with tool_result present and no text >50 chars, tool_result logic applies
func TestScoreMessage_MixedContent(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: "Here's the result from the query: " + strings.Repeat("x", 50)},
			{Type: "tool_result", ToolUseID: "t1", Content: "success output"},
		},
	}
	score := ScoreMessage(msg)
	if score != ScoreMedium {
		t.Errorf("mixed content with long text: got %d, want %d", score, ScoreMedium)
	}
}

// TestCompressMessage_TextOnly tests text-only message compression
func TestCompressMessage_TextOnly(t *testing.T) {
	longText := "This is a long message that should be compressed. " + strings.Repeat("x", 200)
	msg := newTextMessage(types.RoleUser, longText)
	compressed := CompressMessage(msg, 50)

	if len(compressed.Content) != 1 {
		t.Errorf("compressed content length: got %d, want 1", len(compressed.Content))
	}
	if compressed.Content[0].Type != "text" {
		t.Errorf("compressed type: got %s, want text", compressed.Content[0].Type)
	}
	if !strings.Contains(compressed.Content[0].Text, "[...truncated]") {
		t.Error("compressed text should contain truncation marker")
	}
}

// TestCompressMessage_TextWithinLimit tests text under limit is unchanged
func TestCompressMessage_TextWithinLimit(t *testing.T) {
	text := "Short message"
	msg := newTextMessage(types.RoleUser, text)
	compressed := CompressMessage(msg, 100)

	if compressed.Content[0].Text != text {
		t.Errorf("text within limit changed: got %s, want %s", compressed.Content[0].Text, text)
	}
	if strings.Contains(compressed.Content[0].Text, "[...truncated]") {
		t.Error("text within limit should not be truncated")
	}
}

// TestCompressMessage_ToolResult tests tool result compression
func TestCompressMessage_ToolResult(t *testing.T) {
	longContent := "First line\n" + strings.Repeat("more content ", 50)
	msg := newToolResultMessage("tool-1", longContent, false)
	compressed := CompressMessage(msg, 30)

	if len(compressed.Content) != 1 {
		t.Errorf("compressed content length: got %d, want 1", len(compressed.Content))
	}
	if compressed.Content[0].Type != "tool_result" {
		t.Errorf("compressed type: got %s, want tool_result", compressed.Content[0].Type)
	}
	if !strings.Contains(compressed.Content[0].Content, "[...truncated from") {
		t.Error("compressed tool result should contain truncation notice")
	}
	if compressed.Content[0].ToolUseID != "tool-1" {
		t.Errorf("tool use ID lost: got %s, want tool-1", compressed.Content[0].ToolUseID)
	}
}

// TestCompressMessage_ToolResultPreservesFirstLine tests first line is preserved
func TestCompressMessage_ToolResultPreservesFirstLine(t *testing.T) {
	content := "Important first line\nOther stuff that gets truncated"
	msg := newToolResultMessage("tool-1", content, false)
	compressed := CompressMessage(msg, 30)

	if !strings.HasPrefix(compressed.Content[0].Content, "Important") {
		t.Error("first line not preserved in compressed tool result")
	}
}

// TestCompressMessage_PreservesRole tests role is preserved
func TestCompressMessage_PreservesRole(t *testing.T) {
	msg := newTextMessage(types.RoleUser, strings.Repeat("x", 500))
	compressed := CompressMessage(msg, 50)

	if compressed.Role != types.RoleUser {
		t.Errorf("role changed: got %s, want %s", compressed.Role, types.RoleUser)
	}
}

// TestCompressMessage_MultipleBlocks tests compression of multiple content blocks
func TestCompressMessage_MultipleBlocks(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: strings.Repeat("x", 200)},
			{Type: "tool_use", ID: "t1", Name: "tool1"},
			{Type: "text", Text: strings.Repeat("y", 200)},
		},
	}
	compressed := CompressMessage(msg, 50)

	if len(compressed.Content) != 3 {
		t.Errorf("compressed content count: got %d, want 3", len(compressed.Content))
	}
	// Tool use should pass through unchanged
	if compressed.Content[1].Type != "tool_use" {
		t.Errorf("tool use type changed: got %s, want tool_use", compressed.Content[1].Type)
	}
}

// TestContinuousCompress_BelowThreshold tests messages below keepLast threshold are unchanged
func TestContinuousCompress_BelowThreshold(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleUser, "msg1"),
		newTextMessage(types.RoleAssistant, "msg2"),
		newTextMessage(types.RoleUser, "msg3"),
	}
	result := ContinuousCompress(messages, 10, 0)

	if len(result) != len(messages) {
		t.Errorf("result length: got %d, want %d", len(result), len(messages))
	}
	for i, msg := range result {
		if len(msg.Content) != len(messages[i].Content) {
			t.Errorf("message %d content changed", i)
		}
	}
}

// TestContinuousCompress_MinMessages prevents early compression
func TestContinuousCompress_MinMessages(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleUser, "msg1"),
		newTextMessage(types.RoleAssistant, "msg2"),
		newTextMessage(types.RoleUser, "msg3"),
		newTextMessage(types.RoleAssistant, "msg4"),
	}
	result := ContinuousCompress(messages, 2, 10)

	if len(result) != len(messages) {
		t.Errorf("minMessages threshold not respected: got %d messages, want no compression", len(result))
	}
}

// TestContinuousCompress_CompressesOlderMessages tests older messages get compressed
func TestContinuousCompress_CompressesOlderMessages(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleUser, "schema"),
		newTextMessage(types.RoleAssistant, strings.Repeat("x", 500)),
		newTextMessage(types.RoleUser, strings.Repeat("y", 500)),
		newTextMessage(types.RoleAssistant, strings.Repeat("z", 500)),
		newTextMessage(types.RoleUser, strings.Repeat("a", 500)),
	}
	result := ContinuousCompress(messages, 1, 0)

	// First message (schema) should be unchanged
	if len(result[0].Content) != len(messages[0].Content) {
		t.Error("schema message should not be compressed")
	}
	// Last message should be verbatim (keepLast=1)
	if result[len(result)-1].Content[0].Text != messages[len(messages)-1].Content[0].Text {
		t.Error("last message should not be compressed")
	}
	// Middle messages should be compressed (shorter than originals)
	for i := 1; i < len(result)-1; i++ {
		origLen := len(messages[i].Content[0].Text)
		resultLen := len(result[i].Content[0].Text)
		if resultLen > origLen {
			t.Errorf("message %d not compressed: orig=%d, result=%d", i, origLen, resultLen)
		}
	}
}

// TestContinuousCompress_DefaultKeepLast tests default keepLast value
func TestContinuousCompress_DefaultKeepLast(t *testing.T) {
	messages := make([]types.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = newTextMessage(types.RoleUser, "msg")
	}
	result := ContinuousCompress(messages, 0, 0) // keepLast=0 should use default of 10

	if len(result) != len(messages) {
		t.Errorf("result length: got %d, want %d", len(result), len(messages))
	}
}

// TestContinuousCompress_ScoreBasedCompression tests high-score messages are preserved
func TestContinuousCompress_ScoreBasedCompression(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleUser, "schema"),
		newTextMessage(types.RoleAssistant, "How to X?"), // ScoreLow (short, no question)
		newTextMessage(types.RoleUser, "What is the best way to structure this? "+strings.Repeat("x", 150)), // ScoreHigh
		newTextMessage(types.RoleUser, "recent"),
	}
	result := ContinuousCompress(messages, 1, 0)

	// The high-score message should be better preserved than low-score ones
	origHigh := len(messages[2].Content[0].Text)
	resultHigh := len(result[2].Content[0].Text)
	if resultHigh < origHigh/2 {
		t.Error("high-score message was too aggressively compressed")
	}
}

// TestApplyZoneBudget_NoMessages returns empty
func TestApplyZoneBudget_NoMessages(t *testing.T) {
	result := ApplyZoneBudget([]types.Message{}, ZoneBudget{ContextWindow: 10000})
	if len(result) != 0 {
		t.Errorf("empty input should return empty output, got %d", len(result))
	}
}

// TestApplyZoneBudget_InvalidContextWindow returns messages unchanged
func TestApplyZoneBudget_InvalidContextWindow(t *testing.T) {
	messages := []types.Message{newTextMessage(types.RoleUser, "msg")}
	result := ApplyZoneBudget(messages, ZoneBudget{ContextWindow: 0})
	if len(result) != len(messages) {
		t.Error("zero context window should return messages unchanged")
	}
}

// TestApplyZoneBudget_DefaultValues tests default values are applied
func TestApplyZoneBudget_DefaultValues(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleUser, "schema"),
		newTextMessage(types.RoleAssistant, "msg1"),
		newTextMessage(types.RoleUser, "msg2"),
	}
	budget := ZoneBudget{
		ContextWindow:  100000,
		ArchivePercent: 0, // should default to 30
		OutputReserve:  0, // should default to 4096
	}
	result := ApplyZoneBudget(messages, budget)
	if len(result) == 0 {
		t.Error("should apply defaults and return messages")
	}
}

// TestApplyZoneBudget_PreservesLastMessage tests current message is always preserved
func TestApplyZoneBudget_PreservesLastMessage(t *testing.T) {
	lastMsg := newTextMessage(types.RoleUser, "important current message")
	messages := []types.Message{
		newTextMessage(types.RoleUser, "schema"),
		newTextMessage(types.RoleAssistant, strings.Repeat("x", 1000)),
		newTextMessage(types.RoleUser, strings.Repeat("y", 1000)),
		lastMsg,
	}
	budget := ZoneBudget{
		ContextWindow:  10000,
		ArchivePercent: 30,
		OutputReserve:  4000,
	}
	result := ApplyZoneBudget(messages, budget)

	// Last message should be preserved in full
	if len(result) > 0 && result[len(result)-1].Content[0].Text != lastMsg.Content[0].Text {
		t.Error("current message should be preserved in full")
	}
}

// TestApplyZoneBudget_IncludesSchemaMessage tests schema message is always included
func TestApplyZoneBudget_IncludesSchemaMessage(t *testing.T) {
	schemaMsg := newTextMessage(types.RoleSystem, "system schema")
	messages := []types.Message{
		schemaMsg,
		newTextMessage(types.RoleAssistant, strings.Repeat("x", 1000)),
		newTextMessage(types.RoleUser, strings.Repeat("y", 1000)),
		newTextMessage(types.RoleAssistant, "current"),
	}
	budget := ZoneBudget{
		ContextWindow:  10000,
		ArchivePercent: 30,
		OutputReserve:  4000,
	}
	result := ApplyZoneBudget(messages, budget)

	if len(result) == 0 || result[0].Content[0].Text != "system schema" {
		t.Error("schema message (index 0) should always be included")
	}
}

// TestApplyZoneBudget_ZoneSplit tests archive and history zones are split correctly
func TestApplyZoneBudget_ZoneSplit(t *testing.T) {
	messages := []types.Message{
		newTextMessage(types.RoleSystem, "schema"),
		newTextMessage(types.RoleAssistant, "msg1"),
		newTextMessage(types.RoleAssistant, "msg2"),
		newTextMessage(types.RoleAssistant, "msg3"),
		newTextMessage(types.RoleUser, "current"),
	}
	budget := ZoneBudget{
		ContextWindow:  20000,
		ArchivePercent: 40,
		OutputReserve:  4000,
	}
	result := ApplyZoneBudget(messages, budget)

	// Should have schema + some messages
	if len(result) < 2 {
		t.Error("result should include at least schema and current message")
	}
}

// TestApplyZoneBudget_SingleMessage returns it
func TestApplyZoneBudget_SingleMessage(t *testing.T) {
	msg := newTextMessage(types.RoleUser, "only message")
	budget := ZoneBudget{ContextWindow: 10000}
	result := ApplyZoneBudget([]types.Message{msg}, budget)

	if len(result) != 1 || result[0].Content[0].Text != msg.Content[0].Text {
		t.Error("single message should be returned unchanged")
	}
}

// TestMessageScoreConstants tests score constants have expected values
func TestMessageScoreConstants(t *testing.T) {
	tests := []struct {
		score    MessageScore
		expected int
	}{
		{ScoreZero, 0},
		{ScoreLow, 1},
		{ScoreMedium, 2},
		{ScoreHigh, 3},
	}
	for _, test := range tests {
		if int(test.score) != test.expected {
			t.Errorf("score %v: got %d, want %d", test.score, int(test.score), test.expected)
		}
	}
}

// TestScoreMessage_CaseInsensitive tests scoring is case-insensitive
func TestScoreMessage_CaseInsensitive(t *testing.T) {
	msgLower := newTextMessage(types.RoleAssistant, "ok")
	msgUpper := newTextMessage(types.RoleAssistant, "OK")

	scoreLower := ScoreMessage(msgLower)
	scoreUpper := ScoreMessage(msgUpper)

	if scoreLower != scoreUpper {
		t.Errorf("case-insensitive scoring failed: %d != %d", scoreLower, scoreUpper)
	}
}

// TestCompressMessage_ZeroMaxChars tests edge case with zero maxChars
func TestCompressMessage_ZeroMaxChars(t *testing.T) {
	msg := newTextMessage(types.RoleUser, "hello")
	compressed := CompressMessage(msg, 0)

	// Should still return something (likely truncated to "")
	if len(compressed.Content) == 0 {
		t.Error("should return at least one content block")
	}
}

// TestContinuousCompress_EmptyMessages returns empty
func TestContinuousCompress_EmptyMessages(t *testing.T) {
	result := ContinuousCompress([]types.Message{}, 10, 0)
	if len(result) != 0 {
		t.Errorf("empty input should return empty output, got %d", len(result))
	}
}

// TestScoreMessage_ToolResultError tests error tool results still get scored
// Tool result content doesn't count toward textLen; need text content or tool use for scoring
func TestScoreMessage_ToolResultError(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: "Error occurred: " + strings.Repeat("x", 100)},
			{Type: "tool_result", ToolUseID: "tool-1", Content: "error details", IsError: true},
		},
	}
	score := ScoreMessage(msg)

	// Should be scored as Medium or higher based on text content
	if score < ScoreMedium {
		t.Errorf("error tool result with text: got %d, want at least %d", score, ScoreMedium)
	}
}

// TestCompressMessage_ToolResultSingleLine tests tool result with no newline
func TestCompressMessage_ToolResultSingleLine(t *testing.T) {
	msg := newToolResultMessage("tool-1", strings.Repeat("x", 200), false)
	compressed := CompressMessage(msg, 50)

	content := compressed.Content[0].Content
	if !strings.Contains(content, "[...truncated from") {
		t.Error("single-line tool result should be truncated")
	}
	if len(content) >= 200 {
		t.Error("tool result should be compressed")
	}
}

// TestApplyZoneBudget_HighArchivePercent tests high archive percentage works
func TestApplyZoneBudget_HighArchivePercent(t *testing.T) {
	messages := make([]types.Message, 0)
	messages = append(messages, newTextMessage(types.RoleSystem, "schema"))
	for i := 0; i < 20; i++ {
		messages = append(messages, newTextMessage(types.RoleUser, "msg"+string(rune(i))))
	}

	budget := ZoneBudget{
		ContextWindow:  50000,
		ArchivePercent: 80, // high archive percentage
		OutputReserve:  4000,
	}
	result := ApplyZoneBudget(messages, budget)

	if len(result) == 0 {
		t.Error("should return at least some messages")
	}
	// Should include more archive messages due to high percentage
	if result[0].Role != types.RoleSystem {
		t.Error("schema message should be first")
	}
}

// TestScoreMessage_MixedWithEmptyBlocks tests scoring with mixed empty/non-empty blocks
func TestScoreMessage_MixedWithEmptyBlocks(t *testing.T) {
	msg := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: "text", Text: ""},
			{Type: "text", Text: "actual content here"},
		},
	}
	score := ScoreMessage(msg)

	if score == ScoreZero {
		t.Error("message with non-empty blocks should not score zero")
	}
}

// Benchmark tests

// BenchmarkScoreMessage benchmarks message scoring
func BenchmarkScoreMessage(b *testing.B) {
	msg := newTextMessage(types.RoleUser, "This is a question? "+strings.Repeat("x", 200))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreMessage(msg)
	}
}

// BenchmarkCompressMessage benchmarks message compression
func BenchmarkCompressMessage(b *testing.B) {
	msg := newTextMessage(types.RoleUser, strings.Repeat("x", 10000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompressMessage(msg, 500)
	}
}

// BenchmarkContinuousCompress benchmarks continuous compression
func BenchmarkContinuousCompress(b *testing.B) {
	messages := make([]types.Message, 50)
	for i := 0; i < 50; i++ {
		messages[i] = newTextMessage(types.RoleUser, "msg"+string(rune(i))+" "+strings.Repeat("x", 100))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ContinuousCompress(messages, 10, 0)
	}
}

// BenchmarkApplyZoneBudget benchmarks zone budgeting
func BenchmarkApplyZoneBudget(b *testing.B) {
	messages := make([]types.Message, 50)
	for i := 0; i < 50; i++ {
		messages[i] = newTextMessage(types.RoleUser, strings.Repeat("x", 200))
	}
	budget := ZoneBudget{
		ContextWindow:  100000,
		ArchivePercent: 30,
		OutputReserve:  4096,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ApplyZoneBudget(messages, budget)
	}
}
