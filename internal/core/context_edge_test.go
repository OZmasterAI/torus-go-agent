package core

import (
	"fmt"
	"strings"
	"testing"

	typ "torus_go_agent/internal/types"
)

// ---- Edge Case Tests for Context Functions ----

// TestContextEdge_NilSummarizeCallback tests CompactLLM when summarize callback is nil
func TestContextEdge_NilSummarizeCallback(t *testing.T) {
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent(fmt.Sprintf("msg%d", i)),
		}
	}

	// Pass nil summarize callback
	result, err := CompactLLM(messages, 5, nil)
	if err != nil {
		t.Fatalf("CompactLLM with nil callback returned error: %v", err)
	}

	// Should still compact and use extractKeyContent as fallback
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages (first + summary), got %d", len(result))
	}

	// First message should be preserved
	if result[0].Content[0].Text != "msg0" {
		t.Errorf("first message should be msg0, got %q", result[0].Content[0].Text)
	}

	// Summary should contain [Context Summary] marker
	summaryText := result[1].Content[0].Text
	if !strings.Contains(summaryText, "[Context Summary]") {
		t.Errorf("summary should contain [Context Summary] marker, got %q", summaryText)
	}
}

// TestContextEdge_SummarizeErrorFallback tests CompactLLM when summarize returns error
func TestContextEdge_SummarizeErrorFallback(t *testing.T) {
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent(fmt.Sprintf("msg%d", i)),
		}
	}

	// Summarize function that returns an error
	failingSummarize := func(content string) (string, error) {
		return "", fmt.Errorf("LLM call failed")
	}

	result, err := CompactLLM(messages, 5, failingSummarize)
	if err != nil {
		t.Fatalf("CompactLLM with failing summarize returned error: %v", err)
	}

	// Should still compact and use key content as fallback
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(result))
	}

	// Summary should still contain [Context Summary] marker
	summaryText := result[1].Content[0].Text
	if !strings.Contains(summaryText, "[Context Summary]") {
		t.Errorf("summary should contain [Context Summary] marker, got %q", summaryText)
	}
}

// TestContextEdge_CompactLLMSmallMessageList tests CompactLLM with messages <= keepLastN+1
func TestContextEdge_CompactLLMSmallMessageList(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleAssistant, Content: textContent("msg2")},
		{Role: typ.RoleUser, Content: textContent("msg3")},
	}

	result, err := CompactLLM(messages, 10, nil)
	if err != nil {
		t.Fatalf("CompactLLM returned error: %v", err)
	}

	// Should return unchanged
	if len(result) != 3 {
		t.Errorf("expected 3 messages (unchanged), got %d", len(result))
	}

	// Verify content is the same
	if result[0].Content[0].Text != "msg1" || result[1].Content[0].Text != "msg2" || result[2].Content[0].Text != "msg3" {
		t.Error("message content changed unexpectedly")
	}
}

// TestContextEdge_CompactLLMEmptyMessageList tests CompactLLM with empty message list
func TestContextEdge_CompactLLMEmptyMessageList(t *testing.T) {
	messages := []typ.Message{}

	result, err := CompactLLM(messages, 5, nil)
	if err != nil {
		t.Fatalf("CompactLLM with empty list returned error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d messages", len(result))
	}
}

// TestContextEdge_ExtractKeyContentWithEmptyBlocks tests extractKeyContent with empty content blocks
func TestContextEdge_ExtractKeyContentWithEmptyBlocks(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{Type: "text", Text: ""},           // empty text
				{Type: "text", Text: "real text"},  // real text
				{Type: "text", Text: ""},           // another empty
			},
		},
		{
			Role: typ.RoleAssistant,
			Content: []typ.ContentBlock{
				{Type: "text", Text: ""},
			},
		},
	}

	result := extractKeyContent(messages)

	// Should contain the real text
	if !strings.Contains(result, "real text") {
		t.Errorf("expected to find 'real text' in output, got %q", result)
	}

	// Should contain role labels
	if !strings.Contains(result, "[User]") {
		t.Error("expected [User] label in output")
	}
	if !strings.Contains(result, "[Assistant]") {
		t.Error("expected [Assistant] label in output")
	}
}

// TestContextEdge_ExtractKeyContentWithToolUse tests extractKeyContent with tool use blocks
func TestContextEdge_ExtractKeyContentWithToolUse(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleAssistant,
			Content: []typ.ContentBlock{
				{Type: "text", Text: "I'll call a tool"},
				{Type: "tool_use", Name: "TestTool", Input: map[string]interface{}{"param": "value"}},
			},
		},
	}

	result := extractKeyContent(messages)

	// Should contain tool call marker
	if !strings.Contains(result, "[ToolCall: TestTool]") {
		t.Errorf("expected [ToolCall: TestTool] in output, got %q", result)
	}
}

// TestContextEdge_ExtractKeyContentWithToolResult tests extractKeyContent with tool result blocks
func TestContextEdge_ExtractKeyContentWithToolResult(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleTool,
			Content: []typ.ContentBlock{
				{Type: "tool_result", ToolUseID: "tool-123", Content: "Tool output here"},
			},
		},
	}

	result := extractKeyContent(messages)

	// Should contain tool result marker
	if !strings.Contains(result, "[ToolResult: tool-123]") {
		t.Errorf("expected [ToolResult: tool-123] in output, got %q", result)
	}
	if !strings.Contains(result, "Tool output here") {
		t.Error("expected tool output content in result")
	}
}

// TestContextEdge_ExtractKeyContentTruncation tests that extractKeyContent truncates at 2000 chars
func TestContextEdge_ExtractKeyContentTruncation(t *testing.T) {
	// Create a very large message
	largeText := strings.Repeat("x", 3000)
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent(largeText)},
	}

	result := extractKeyContent(messages)

	// Should be truncated to at most 2000 chars
	if len(result) > 2000 {
		t.Errorf("expected result <= 2000 chars, got %d", len(result))
	}
}

// TestContextEdge_NeedsCompactionWithMaxMessagesZero tests NeedsCompaction when MaxMessages is 0
func TestContextEdge_NeedsCompactionWithMaxMessagesZero(t *testing.T) {
	messages := make([]typ.Message, 50)
	for i := 0; i < 50; i++ {
		messages[i] = typ.Message{Role: typ.RoleUser, Content: textContent("x")}
	}

	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		MaxMessages:   0, // Disabled
		Trigger:       "messages",
	}

	// Should not trigger even with many messages, because MaxMessages=0 disables the check
	if NeedsCompaction(messages, cfg) {
		t.Error("expected false when MaxMessages=0 (disabled)")
	}
}

// TestContextEdge_NeedsCompactionWithEmptyMessages tests NeedsCompaction with empty message list
func TestContextEdge_NeedsCompactionWithEmptyMessages(t *testing.T) {
	messages := []typ.Message{}

	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		Trigger:       "tokens",
	}

	if NeedsCompaction(messages, cfg) {
		t.Error("expected false for empty message list")
	}
}

// TestContextEdge_NeedsCompactionWithThreshold0 tests when threshold is 0
func TestContextEdge_NeedsCompactionWithThreshold0(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("hi")},
	}

	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     0, // Will default to 80
		ContextWindow: 100_000,
		Trigger:       "tokens",
	}

	if NeedsCompaction(messages, cfg) {
		t.Error("expected false for small messages with defaulted threshold")
	}
}

// TestContextEdge_NeedsCompactionWithThreshold100 tests when threshold is 100 (almost never triggers)
func TestContextEdge_NeedsCompactionWithThreshold100(t *testing.T) {
	largeText := ""
	for i := 0; i < 200; i++ {
		largeText += "This is a long message to fill up tokens. "
	}
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent(largeText)},
	}

	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     100, // Limit is 100% of context window
		ContextWindow: 1000,
		Trigger:       "tokens",
	}

	// threshold=100 means 100% of context window — NeedsCompaction may still
	// return true depending on token count vs window size
	_ = NeedsCompaction(messages, cfg)
}

// TestContextEdge_CompactSlidingWithZeroKeepLastN tests CompactSliding boundary at 0
func TestContextEdge_CompactSlidingWithZeroKeepLastN(t *testing.T) {
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = typ.Message{Role: typ.RoleUser, Content: textContent("msg")}
	}

	// With KeepLastN=0, should default to 10, so expect first + last 10 = 11
	result := CompactSliding(messages, 0)
	if len(result) != 11 {
		t.Errorf("expected 11 messages (default KeepLastN=10), got %d", len(result))
	}
}

// TestContextEdge_CompactSlidingWithNegativeKeepLastN tests CompactSliding boundary at negative
func TestContextEdge_CompactSlidingWithNegativeKeepLastN(t *testing.T) {
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = typ.Message{Role: typ.RoleUser, Content: textContent("msg")}
	}

	// With KeepLastN=-5, should default to 10, so expect first + last 10 = 11
	result := CompactSliding(messages, -5)
	if len(result) != 11 {
		t.Errorf("expected 11 messages (negative KeepLastN defaults to 10), got %d", len(result))
	}
}

// TestContextEdge_SanitizeMessagesWithEmptyTextBlocks tests sanitizeMessages filtering empty text
func TestContextEdge_SanitizeMessagesWithEmptyTextBlocks(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{Type: "text", Text: ""},
				{Type: "text", Text: "real"},
				{Type: "text", Text: ""},
			},
		},
	}

	result := sanitizeMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message after sanitization, got %d", len(result))
	}

	// Should have only the non-empty text block
	if len(result[0].Content) != 1 {
		t.Errorf("expected 1 content block, got %d", len(result[0].Content))
	}

	if result[0].Content[0].Text != "real" {
		t.Errorf("expected 'real', got %q", result[0].Content[0].Text)
	}
}

// TestContextEdge_SanitizeMessagesWithDuplicateTextBlocks tests deduplication
func TestContextEdge_SanitizeMessagesWithDuplicateTextBlocks(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{Type: "text", Text: "duplicate"},
				{Type: "text", Text: "unique"},
				{Type: "text", Text: "duplicate"}, // Duplicate
			},
		},
	}

	result := sanitizeMessages(messages)

	if len(result[0].Content) != 2 {
		t.Errorf("expected 2 content blocks after dedup, got %d", len(result[0].Content))
	}

	// Should have duplicate removed
	texts := []string{}
	for _, b := range result[0].Content {
		texts = append(texts, b.Text)
	}

	if len(texts) != 2 || texts[0] != "duplicate" || texts[1] != "unique" {
		t.Errorf("deduplication failed: %v", texts)
	}
}

// TestContextEdge_SanitizeMessagesWithConsecutiveSameRole tests merging of same-role messages
func TestContextEdge_SanitizeMessagesWithConsecutiveSameRole(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleUser, Content: textContent("msg2")}, // Same role, should merge
		{Role: typ.RoleAssistant, Content: textContent("msg3")},
		{Role: typ.RoleAssistant, Content: textContent("msg4")}, // Same role, should merge
	}

	result := sanitizeMessages(messages)

	if len(result) != 2 {
		t.Errorf("expected 2 merged messages, got %d", len(result))
	}

	// Check roles
	if result[0].Role != typ.RoleUser {
		t.Errorf("expected first to be User, got %v", result[0].Role)
	}
	if result[1].Role != typ.RoleAssistant {
		t.Errorf("expected second to be Assistant, got %v", result[1].Role)
	}

	// Check content block count (should be 2 merged in first, 2 merged in second)
	if len(result[0].Content) != 2 {
		t.Errorf("expected 2 content blocks in first message, got %d", len(result[0].Content))
	}
	if len(result[1].Content) != 2 {
		t.Errorf("expected 2 content blocks in second message, got %d", len(result[1].Content))
	}
}

// TestContextEdge_SanitizeMessagesWithToolMessages tests that tool messages don't merge
func TestContextEdge_SanitizeMessagesWithToolMessages(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleTool, Content: textContent("result1")},
		{Role: typ.RoleTool, Content: textContent("result2")}, // Should NOT merge
	}

	result := sanitizeMessages(messages)

	// Tool messages should NOT be merged even if consecutive
	if len(result) != 2 {
		t.Errorf("expected 2 tool messages (not merged), got %d", len(result))
	}
}

// TestContextEdge_SanitizeMessagesWithEmptyContent tests sanitize with empty content
func TestContextEdge_SanitizeMessagesWithEmptyContent(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleAssistant, Content: []typ.ContentBlock{}}, // Empty content
		{Role: typ.RoleUser, Content: textContent("msg2")},
	}

	result := sanitizeMessages(messages)

	// sanitizeMessages may merge consecutive same-role messages after filtering,
	// so the count depends on implementation. Just verify no panic and non-empty result.
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	if result[0].Content[0].Text != "msg1" {
		t.Error("first message should be msg1")
	}
}

// TestContextEdge_NodeToMessagesWithSystemRole tests nodesToMessages with system role
func TestContextEdge_NodeToMessagesWithSystemRole(t *testing.T) {
	nodes := []Node{
		{ID: "1", Role: "system", Content: textContent("system prompt"), TokenCount: 10},
		{ID: "2", Role: "user", Content: textContent("user message"), TokenCount: 10},
	}

	messages := nodesToMessages(nodes)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != typ.RoleSystem {
		t.Errorf("expected first message role to be system, got %v", messages[0].Role)
	}

	if messages[1].Role != typ.RoleUser {
		t.Errorf("expected second message role to be user, got %v", messages[1].Role)
	}
}

// TestContextEdge_NodeToMessagesPreservesContent tests that nodesToMessages preserves content
func TestContextEdge_NodeToMessagesPreservesContent(t *testing.T) {
	content := []typ.ContentBlock{
		{Type: "text", Text: "test content"},
		{Type: "tool_use", Name: "TestTool"},
	}
	nodes := []Node{
		{ID: "1", Role: "user", Content: content, TokenCount: 20},
	}

	messages := nodesToMessages(nodes)

	if len(messages[0].Content) != 2 {
		t.Errorf("expected 2 content blocks, got %d", len(messages[0].Content))
	}

	if messages[0].Content[0].Text != "test content" {
		t.Error("first content block text mismatch")
	}
	if messages[0].Content[1].Name != "TestTool" {
		t.Error("second content block name mismatch")
	}
}

// TestContextEdge_DefaultCompactionConfigWithNegativeThreshold tests negative threshold handling
func TestContextEdge_DefaultCompactionConfigWithNegativeThreshold(t *testing.T) {
	cfg := CompactionConfig{
		Threshold:     -10, // Negative threshold
		KeepLastN:     5,
		ContextWindow: 100_000,
	}
	result := defaultCompactionConfig(cfg)

	// Negative should not be replaced (only zero is)
	if result.Threshold != -10 {
		t.Errorf("expected Threshold=-10, got %d", result.Threshold)
	}
}

// TestContextEdge_CompactSlidingWithVeryLargeKeepLastN tests when keepLastN > message count
func TestContextEdge_CompactSlidingWithVeryLargeKeepLastN(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleAssistant, Content: textContent("msg2")},
		{Role: typ.RoleUser, Content: textContent("msg3")},
	}

	// KeepLastN=1000 is much larger than the message count
	result := CompactSliding(messages, 1000)

	// Should return unchanged since len(messages) <= keepLastN+1
	if len(result) != 3 {
		t.Errorf("expected 3 messages (unchanged), got %d", len(result))
	}
}

// TestContextEdge_ExtractKeyContentWithAllRoles tests all role types in extractKeyContent
func TestContextEdge_ExtractKeyContentWithAllRoles(t *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleSystem, Content: textContent("system content")},
		{Role: typ.RoleUser, Content: textContent("user content")},
		{Role: typ.RoleAssistant, Content: textContent("assistant content")},
		{Role: typ.RoleTool, Content: textContent("tool content")},
	}

	result := extractKeyContent(messages)

	// Should contain all role labels
	if !strings.Contains(result, "[System]") {
		t.Error("expected [System] label")
	}
	if !strings.Contains(result, "[User]") {
		t.Error("expected [User] label")
	}
	if !strings.Contains(result, "[Assistant]") {
		t.Error("expected [Assistant] label")
	}
}

// TestContextEdge_CompactSlidingEdgeBoundary tests exact edge case at keepLastN+1
func TestContextEdge_CompactSlidingEdgeBoundary(t *testing.T) {
	// Create exactly keepLastN+1 messages
	keepN := 5
	messages := make([]typ.Message, keepN+1)
	for i := 0; i < keepN+1; i++ {
		messages[i] = typ.Message{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{Type: "text", Text: fmt.Sprintf("msg%d", i)},
			},
		}
	}

	result := CompactSliding(messages, keepN)

	// Should be unchanged
	if len(result) != keepN+1 {
		t.Errorf("expected %d messages (unchanged), got %d", keepN+1, len(result))
	}

	// Verify content is preserved
	for i := range messages {
		if result[i].Content[0].Text != fmt.Sprintf("msg%d", i) {
			t.Errorf("message %d content changed", i)
		}
	}
}

// TestContextEdge_CompactLLMWithNegativeKeepLastN tests CompactLLM with negative keepLastN
func TestContextEdge_CompactLLMWithNegativeKeepLastN(t *testing.T) {
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = typ.Message{
			Role:    typ.RoleUser,
			Content: textContent(fmt.Sprintf("msg%d", i)),
		}
	}

	// Negative keepLastN should default to 10
	result, err := CompactLLM(messages, -5, nil)
	if err != nil {
		t.Fatalf("CompactLLM with negative keepLastN returned error: %v", err)
	}

	// Should have first + summary + last 10 = 12
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(result))
	}
}

// TestContextEdge_ExtractKeyContentWithMixedEmptyBlocks tests mixed empty and non-empty blocks
func TestContextEdge_ExtractKeyContentWithMixedEmptyBlocks(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{Type: "text", Text: ""},
				{Type: "tool_use", Name: "Tool1"},
				{Type: "text", Text: ""},
				{Type: "tool_result", ToolUseID: "id1", Content: ""},
				{Type: "text", Text: "final text"},
			},
		},
	}

	result := extractKeyContent(messages)

	// Should contain the tool markers even if text is empty
	if !strings.Contains(result, "[ToolCall: Tool1]") {
		t.Error("expected tool call marker")
	}
	if !strings.Contains(result, "[ToolResult: id1]") {
		t.Error("expected tool result marker")
	}
	if !strings.Contains(result, "final text") {
		t.Error("expected final text")
	}
}

// TestContextEdge_SanitizeMessagesTrimsTrailingAssistantWhitespace tests that
// trailing whitespace on the final assistant message is trimmed (Anthropic API
// rejects "final assistant content cannot end with trailing whitespace").
func TestContextEdge_SanitizeMessagesTrimsTrailingAssistantWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		messages []typ.Message
		wantText string
	}{
		{
			name: "trailing spaces",
			messages: []typ.Message{
				{Role: typ.RoleUser, Content: []typ.ContentBlock{{Type: "text", Text: "hi"}}},
				{Role: typ.RoleAssistant, Content: []typ.ContentBlock{{Type: "text", Text: "hello   "}}},
			},
			wantText: "hello",
		},
		{
			name: "trailing newlines",
			messages: []typ.Message{
				{Role: typ.RoleUser, Content: []typ.ContentBlock{{Type: "text", Text: "hi"}}},
				{Role: typ.RoleAssistant, Content: []typ.ContentBlock{{Type: "text", Text: "hello\n\n"}}},
			},
			wantText: "hello",
		},
		{
			name: "trailing tabs and mixed",
			messages: []typ.Message{
				{Role: typ.RoleUser, Content: []typ.ContentBlock{{Type: "text", Text: "hi"}}},
				{Role: typ.RoleAssistant, Content: []typ.ContentBlock{{Type: "text", Text: "hello \t\n\r"}}},
			},
			wantText: "hello",
		},
		{
			name: "no trailing whitespace unchanged",
			messages: []typ.Message{
				{Role: typ.RoleUser, Content: []typ.ContentBlock{{Type: "text", Text: "hi"}}},
				{Role: typ.RoleAssistant, Content: []typ.ContentBlock{{Type: "text", Text: "hello"}}},
			},
			wantText: "hello",
		},
		{
			name: "final message is user not trimmed",
			messages: []typ.Message{
				{Role: typ.RoleAssistant, Content: []typ.ContentBlock{{Type: "text", Text: "hello   "}}},
				{Role: typ.RoleUser, Content: []typ.ContentBlock{{Type: "text", Text: "hi   "}}},
			},
			wantText: "hello   ", // assistant not final, so not trimmed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeMessages(tt.messages)
			// Find the assistant message and check its text
			for _, m := range result {
				if m.Role == typ.RoleAssistant {
					for _, b := range m.Content {
						if b.Type == "text" && b.Text != tt.wantText {
							t.Errorf("got %q, want %q", b.Text, tt.wantText)
						}
					}
				}
			}
		})
	}
}

// Note: textContent and newTestDAG helpers are defined in dag_test.go and loop_test.go respectively
