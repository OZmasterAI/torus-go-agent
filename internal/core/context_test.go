package core

import (
	"testing"

	typ "torus_go_agent/internal/types"
)

// ---- 1. defaultCompactionConfig Tests ----

func TestDefaultCompactionConfig_ZeroThreshold(tt *testing.T) {
	cfg := CompactionConfig{
		Threshold:     0,
		KeepLastN:     5,
		ContextWindow: 100_000,
	}
	result := defaultCompactionConfig(cfg)
	if result.Threshold != 80 {
		tt.Errorf("expected Threshold=80, got %d", result.Threshold)
	}
}

func TestDefaultCompactionConfig_ZeroKeepLastN(tt *testing.T) {
	cfg := CompactionConfig{
		Threshold:     85,
		KeepLastN:     0,
		ContextWindow: 100_000,
	}
	result := defaultCompactionConfig(cfg)
	if result.KeepLastN != 10 {
		tt.Errorf("expected KeepLastN=10, got %d", result.KeepLastN)
	}
}

func TestDefaultCompactionConfig_ZeroContextWindow(tt *testing.T) {
	cfg := CompactionConfig{
		Threshold:     85,
		KeepLastN:     5,
		ContextWindow: 0,
	}
	result := defaultCompactionConfig(cfg)
	if result.ContextWindow != 200_000 {
		tt.Errorf("expected ContextWindow=200000, got %d", result.ContextWindow)
	}
}

func TestDefaultCompactionConfig_AllDefaults(tt *testing.T) {
	cfg := CompactionConfig{}
	result := defaultCompactionConfig(cfg)
	if result.Threshold != 80 {
		tt.Errorf("expected Threshold=80, got %d", result.Threshold)
	}
	if result.KeepLastN != 10 {
		tt.Errorf("expected KeepLastN=10, got %d", result.KeepLastN)
	}
	if result.ContextWindow != 200_000 {
		tt.Errorf("expected ContextWindow=200000, got %d", result.ContextWindow)
	}
}

func TestDefaultCompactionConfig_PreservesNonZeroValues(tt *testing.T) {
	cfg := CompactionConfig{
		Threshold:     75,
		KeepLastN:     15,
		ContextWindow: 100_000,
	}
	result := defaultCompactionConfig(cfg)
	if result.Threshold != 75 {
		tt.Errorf("expected Threshold=75, got %d", result.Threshold)
	}
	if result.KeepLastN != 15 {
		tt.Errorf("expected KeepLastN=15, got %d", result.KeepLastN)
	}
	if result.ContextWindow != 100_000 {
		tt.Errorf("expected ContextWindow=100000, got %d", result.ContextWindow)
	}
}

// ---- 2. NeedsCompaction Tests ----

func TestNeedsCompaction_CompactionOff(tt *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("hello")},
		{Role: typ.RoleAssistant, Content: textContent("hi")},
	}
	cfg := CompactionConfig{
		Mode:          CompactionOff,
		Threshold:     50,
		ContextWindow: 1000,
	}
	if NeedsCompaction(messages, cfg) {
		tt.Error("expected false when CompactionOff")
	}
}

func TestNeedsCompaction_TokenBased_BelowThreshold(tt *testing.T) {
	// Small messages that won't hit the threshold
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("hi")},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		Trigger:       "tokens",
	}
	if NeedsCompaction(messages, cfg) {
		tt.Error("expected false for small messages below threshold")
	}
}

func TestNeedsCompaction_TokenBased_AboveThreshold(tt *testing.T) {
	// Create messages that will exceed token threshold
	// With ContextWindow=1000 and Threshold=50, limit is 500 tokens
	// EstimateTokens divides JSON length by 3.5
	// So we need enough content to exceed 500 tokens (~1750 chars)
	largeText := ""
	for i := 0; i < 200; i++ {
		largeText += "This is a long message to fill up tokens. "
	}
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent(largeText)},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     50,
		ContextWindow: 1000,
		Trigger:       "tokens",
	}
	if !NeedsCompaction(messages, cfg) {
		tt.Error("expected true when tokens exceed threshold")
	}
}

func TestNeedsCompaction_MessageBased_BelowLimit(tt *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleAssistant, Content: textContent("msg2")},
		{Role: typ.RoleUser, Content: textContent("msg3")},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		MaxMessages:   10,
		Trigger:       "messages",
	}
	if NeedsCompaction(messages, cfg) {
		tt.Error("expected false when message count below MaxMessages")
	}
}

func TestNeedsCompaction_MessageBased_AboveLimit(tt *testing.T) {
	messages := make([]typ.Message, 15)
	for i := 0; i < 15; i++ {
		if i%2 == 0 {
			messages[i] = typ.Message{Role: typ.RoleUser, Content: textContent("user msg")}
		} else {
			messages[i] = typ.Message{Role: typ.RoleAssistant, Content: textContent("assistant msg")}
		}
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		MaxMessages:   10,
		Trigger:       "messages",
	}
	if !NeedsCompaction(messages, cfg) {
		tt.Error("expected true when message count exceeds MaxMessages")
	}
}

func TestNeedsCompaction_BothMode_TokensHit(tt *testing.T) {
	// Create messages that hit tokens but not message count
	largeText := ""
	for i := 0; i < 200; i++ {
		largeText += "This is a long message to fill up tokens. "
	}
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent(largeText)},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     50,
		ContextWindow: 1000,
		MaxMessages:   10,
		Trigger:       "both",
	}
	if !NeedsCompaction(messages, cfg) {
		tt.Error("expected true when tokens hit in 'both' mode")
	}
}

func TestNeedsCompaction_BothMode_MessagesHit(tt *testing.T) {
	// Create messages that hit message count but keep tokens low
	messages := make([]typ.Message, 15)
	for i := 0; i < 15; i++ {
		if i%2 == 0 {
			messages[i] = typ.Message{Role: typ.RoleUser, Content: textContent("x")}
		} else {
			messages[i] = typ.Message{Role: typ.RoleAssistant, Content: textContent("y")}
		}
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     90,
		ContextWindow: 100_000,
		MaxMessages:   10,
		Trigger:       "both",
	}
	if !NeedsCompaction(messages, cfg) {
		tt.Error("expected true when messages hit in 'both' mode")
	}
}

func TestNeedsCompaction_BothMode_BothMissed(tt *testing.T) {
	// Small messages, well below limits
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("a")},
		{Role: typ.RoleAssistant, Content: textContent("b")},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		MaxMessages:   10,
		Trigger:       "both",
	}
	if NeedsCompaction(messages, cfg) {
		tt.Error("expected false when both triggers missed")
	}
}

func TestNeedsCompaction_DefaultTrigger_IsTokens(tt *testing.T) {
	// When Trigger is empty string, should default to "tokens"
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("hi")},
	}
	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		Trigger:       "", // empty string, should default to "tokens"
	}
	if NeedsCompaction(messages, cfg) {
		tt.Error("expected false for small messages with default trigger")
	}
}

// ---- 3. CompactSliding Tests ----

func TestCompactSliding_UnchangedWhenSmall(tt *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("msg1")},
		{Role: typ.RoleAssistant, Content: textContent("msg2")},
	}
	result := CompactSliding(messages, 10)
	if len(result) != 2 {
		tt.Errorf("expected unchanged slice, got len=%d", len(result))
	}
	if result[0].Content[0].Text != "msg1" {
		tt.Error("first message changed")
	}
}

func TestCompactSliding_KeepsFirstAndLastN(tt *testing.T) {
	// Create 20 messages: indices 0-19
	// With KeepLastN=5, we should keep:
	//   - message[0] (first)
	//   - messages[15-19] (last 5)
	// Total: 6 messages
	messages := make([]typ.Message, 20)
	for i := 0; i < 20; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent("msg" + string(rune('0'+byte(i%10)))),
		}
	}

	result := CompactSliding(messages, 5)

	// Should have 6 messages: first + last 5
	if len(result) != 6 {
		tt.Fatalf("expected 6 messages, got %d", len(result))
	}

	// First should be original message 0
	if result[0].Content[0].Text != "msg0" {
		tt.Errorf("first message should be 'msg0', got %q", result[0].Content[0].Text)
	}

	// Last 5 should be original messages 15-19
	expectedLastTexts := []string{"msg5", "msg6", "msg7", "msg8", "msg9"}
	for i, expectedText := range expectedLastTexts {
		actualText := result[i+1].Content[0].Text
		if actualText != expectedText {
			tt.Errorf("position %d: expected %q, got %q", i+1, expectedText, actualText)
		}
	}
}

func TestCompactSliding_DefaultKeepLastN(tt *testing.T) {
	// Create 50 messages
	messages := make([]typ.Message, 50)
	for i := 0; i < 50; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent("msg"),
		}
	}

	// With KeepLastN=0 (will default to 10), expect first + last 10 = 11 messages
	result := CompactSliding(messages, 0)
	if len(result) != 11 {
		tt.Fatalf("expected 11 messages (default KeepLastN=10), got %d", len(result))
	}
}

func TestCompactSliding_NegativeKeepLastN(tt *testing.T) {
	// Negative KeepLastN should also default to 10
	messages := make([]typ.Message, 50)
	for i := 0; i < 50; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent("msg"),
		}
	}

	result := CompactSliding(messages, -5)
	if len(result) != 11 {
		tt.Fatalf("expected 11 messages (negative KeepLastN defaults to 10), got %d", len(result))
	}
}

func TestCompactSliding_ExactBoundary(tt *testing.T) {
	// Test the boundary case: len(messages) == keepLastN+1
	// With 11 messages and KeepLastN=10, should return unchanged
	messages := make([]typ.Message, 11)
	for i := 0; i < 11; i++ {
		messages[i] = typ.Message{
			Role:    typ.RoleUser,
			Content: textContent("msg"),
		}
	}

	result := CompactSliding(messages, 10)
	if len(result) != 11 {
		tt.Errorf("expected unchanged at boundary, got len=%d", len(result))
	}

	// Verify it's the exact same slice (not a copy)
	for i := range messages {
		if result[i].Content[0].Text != messages[i].Content[0].Text {
			tt.Error("slice content changed at boundary")
		}
	}
}

func TestCompactSliding_OneOverBoundary(tt *testing.T) {
	// With 12 messages and KeepLastN=10, should compact to 11 (first + last 10)
	messages := make([]typ.Message, 12)
	for i := 0; i < 12; i++ {
		idx := i
		messages[i] = typ.Message{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{{
				Type: "text",
				Text: "msg" + string(rune('0'+byte(idx%10))),
			}},
		}
	}

	result := CompactSliding(messages, 10)
	if len(result) != 11 {
		tt.Fatalf("expected 11 messages, got %d", len(result))
	}

	// First should be msg0
	if result[0].Content[0].Text != "msg0" {
		tt.Errorf("first should be msg0, got %q", result[0].Content[0].Text)
	}
	// Last should be msg1 (last message in original was msg1 at index 11)
	if result[10].Content[0].Text != "msg1" {
		tt.Errorf("last should be msg1, got %q", result[10].Content[0].Text)
	}
}

func TestCompactSliding_PreservesRoles(tt *testing.T) {
	// Verify that roles are preserved through compaction
	messages := make([]typ.Message, 15)
	for i := 0; i < 15; i++ {
		role := typ.RoleUser
		if i%2 == 1 {
			role = typ.RoleAssistant
		}
		messages[i] = typ.Message{
			Role:    role,
			Content: textContent("msg"),
		}
	}

	result := CompactSliding(messages, 3)
	// Should have first message (role=user) + last 3
	if result[0].Role != typ.RoleUser {
		tt.Errorf("first role should be user, got %v", result[0].Role)
	}
	// Last 3 messages from original are indices 12, 13, 14
	// Index 12: 12%2==0 -> RoleUser
	// Index 13: 13%2==1 -> RoleAssistant
	// Index 14: 14%2==0 -> RoleUser
	if result[1].Role != typ.RoleUser {
		tt.Errorf("result[1] should be user, got %v", result[1].Role)
	}
	if result[2].Role != typ.RoleAssistant {
		tt.Errorf("result[2] should be assistant, got %v", result[2].Role)
	}
	if result[3].Role != typ.RoleUser {
		tt.Errorf("result[3] should be user, got %v", result[3].Role)
	}
}

func TestCompactSliding_EmptySlice(tt *testing.T) {
	messages := []typ.Message{}
	result := CompactSliding(messages, 5)
	if len(result) != 0 {
		tt.Errorf("expected empty result, got len=%d", len(result))
	}
}

func TestCompactSliding_SingleMessage(tt *testing.T) {
	messages := []typ.Message{
		{Role: typ.RoleUser, Content: textContent("only")},
	}
	result := CompactSliding(messages, 5)
	if len(result) != 1 {
		tt.Errorf("expected 1 message, got %d", len(result))
	}
	if result[0].Content[0].Text != "only" {
		tt.Error("single message changed")
	}
}

// ---- 4. Integration Tests with DAG (bonus) ----

func TestCompactSliding_WithDAG_Consistency(tt *testing.T) {
	// Create a DAG with a few nodes
	dag := newTestDAG(tt)

	id1, _ := dag.AddNode("", typ.RoleUser, textContent("msg1"), "", "", 10)
	id2, _ := dag.AddNode(id1, typ.RoleAssistant, textContent("msg2"), "", "", 20)
	id3, _ := dag.AddNode(id2, typ.RoleUser, textContent("msg3"), "", "", 15)

	// Get ancestors (full message chain)
	ancestors, _ := dag.GetAncestors(id3)
	messages := nodesToMessages(ancestors)

	// Compact with KeepLastN=1
	result := CompactSliding(messages, 1)

	// Should have first + last 1 = 2 messages
	if len(result) != 2 {
		tt.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Content[0].Text != "msg1" {
		tt.Error("first message should be preserved")
	}
	if result[1].Content[0].Text != "msg3" {
		tt.Error("last message should be preserved")
	}
}

func TestNeedsCompaction_WithDAG(tt *testing.T) {
	// Create a DAG with messages
	dag := newTestDAG(tt)

	// Add a few small messages
	id1, _ := dag.AddNode("", typ.RoleUser, textContent("hi"), "", "", 5)
	id2, _ := dag.AddNode(id1, typ.RoleAssistant, textContent("hey"), "", "", 5)

	ancestors, _ := dag.GetAncestors(id2)
	messages := nodesToMessages(ancestors)

	cfg := CompactionConfig{
		Mode:          CompactionSliding,
		Threshold:     80,
		ContextWindow: 100_000,
		Trigger:       "tokens",
	}

	// Should not trigger compaction for small messages
	if NeedsCompaction(messages, cfg) {
		tt.Error("should not trigger compaction for small DAG")
	}
}
