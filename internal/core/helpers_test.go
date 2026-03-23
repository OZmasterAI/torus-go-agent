package core

import (
	"testing"

	"torus_go_agent/internal/types"
)

// TestHasToolUse tests the HasToolUse function with various message states.
func TestHasToolUse(t *testing.T) {
	tests := []struct {
		name     string
		msg      *types.AssistantMessage
		expected bool
	}{
		{
			name: "with tool_use block",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "I'll help you with this."},
						{Type: "tool_use", ID: "call_1", Name: "read_file"},
					},
				},
			},
			expected: true,
		},
		{
			name: "without tool_use block",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Here's your answer"},
					},
				},
			},
			expected: false,
		},
		{
			name: "empty content",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{},
				},
			},
			expected: false,
		},
		{
			name: "multiple tool_use blocks",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "tool_use", ID: "call_1", Name: "read_file"},
						{Type: "text", Text: "and then"},
						{Type: "tool_use", ID: "call_2", Name: "write_file"},
					},
				},
			},
			expected: true,
		},
		{
			name: "only tool_use blocks",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "tool_use", ID: "call_1", Name: "some_tool"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := HasToolUse(tc.msg)
			if result != tc.expected {
				t.Errorf("HasToolUse() = %v, want %v", result, tc.expected)
			}
		})
	}
}

// TestExtractText tests the ExtractText function with various block combinations.
func TestExtractText(t *testing.T) {
	tests := []struct {
		name     string
		msg      *types.AssistantMessage
		expected string
	}{
		{
			name: "single text block",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Hello, world!"},
					},
				},
			},
			expected: "Hello, world!",
		},
		{
			name: "multiple text blocks concatenated",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Hello, "},
						{Type: "text", Text: "world!"},
					},
				},
			},
			expected: "Hello, world!",
		},
		{
			name: "mixed blocks with text and tool_use",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Let me read that file."},
						{Type: "tool_use", ID: "call_1", Name: "read_file"},
						{Type: "text", Text: " The content is important."},
					},
				},
			},
			expected: "Let me read that file. The content is important.",
		},
		{
			name: "empty content",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{},
				},
			},
			expected: "",
		},
		{
			name: "only tool_use blocks",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "tool_use", ID: "call_1", Name: "read_file"},
						{Type: "tool_use", ID: "call_2", Name: "write_file"},
					},
				},
			},
			expected: "",
		},
		{
			name: "text blocks with empty strings",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Hello"},
						{Type: "text", Text: ""},
						{Type: "text", Text: " world"},
					},
				},
			},
			expected: "Hello world",
		},
		{
			name: "block type mismatch",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "tool_result", ToolUseID: "call_1", Content: "result"},
						{Type: "text", Text: "Just text"},
					},
				},
			},
			expected: "Just text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractText(tc.msg)
			if result != tc.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestExtractToolCalls tests the ExtractToolCalls function with various scenarios.
func TestExtractToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		msg      *types.AssistantMessage
		expected int
		verify   func(t *testing.T, calls []types.ContentBlock)
	}{
		{
			name: "single tool call",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "I'll read the file."},
						{Type: "tool_use", ID: "call_1", Name: "read_file", Input: map[string]any{"path": "/tmp/file"}},
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, calls []types.ContentBlock) {
				if calls[0].ID != "call_1" {
					t.Errorf("expected ID call_1, got %s", calls[0].ID)
				}
				if calls[0].Name != "read_file" {
					t.Errorf("expected name read_file, got %s", calls[0].Name)
				}
			},
		},
		{
			name: "multiple tool calls",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "tool_use", ID: "call_1", Name: "read_file"},
						{Type: "text", Text: "now writing..."},
						{Type: "tool_use", ID: "call_2", Name: "write_file"},
						{Type: "tool_use", ID: "call_3", Name: "delete_file"},
					},
				},
			},
			expected: 3,
			verify: func(t *testing.T, calls []types.ContentBlock) {
				ids := []string{"call_1", "call_2", "call_3"}
				for i, id := range ids {
					if calls[i].ID != id {
						t.Errorf("expected ID %s at index %d, got %s", id, i, calls[i].ID)
					}
				}
			},
		},
		{
			name: "no tool calls",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Just a plain response"},
					},
				},
			},
			expected: 0,
			verify:   func(t *testing.T, calls []types.ContentBlock) {},
		},
		{
			name: "empty content",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{},
				},
			},
			expected: 0,
			verify:   func(t *testing.T, calls []types.ContentBlock) {},
		},
		{
			name: "mixed content types",
			msg: &types.AssistantMessage{
				Message: types.Message{
					Content: []types.ContentBlock{
						{Type: "text", Text: "Starting..."},
						{Type: "tool_use", ID: "call_1", Name: "tool_a"},
						{Type: "tool_result", ToolUseID: "call_0", Content: "some result"},
						{Type: "tool_use", ID: "call_2", Name: "tool_b"},
						{Type: "text", Text: "Ending..."},
					},
				},
			},
			expected: 2,
			verify: func(t *testing.T, calls []types.ContentBlock) {
				if calls[0].Name != "tool_a" || calls[1].Name != "tool_b" {
					t.Errorf("expected tool_a and tool_b, got %s and %s", calls[0].Name, calls[1].Name)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractToolCalls(tc.msg)
			if len(result) != tc.expected {
				t.Errorf("ExtractToolCalls() returned %d calls, want %d", len(result), tc.expected)
			}
			tc.verify(t, result)
		})
	}
}

// TestEventAgentThinkingDelta verifies the constant exists and has the expected value.
func TestEventAgentThinkingDelta(t *testing.T) {
	if EventAgentThinkingDelta != "thinking_delta" {
		t.Errorf("EventAgentThinkingDelta = %q, want %q", EventAgentThinkingDelta, "thinking_delta")
	}
	// Must be distinct from EventAgentTextDelta.
	if EventAgentThinkingDelta == EventAgentTextDelta {
		t.Error("EventAgentThinkingDelta must differ from EventAgentTextDelta")
	}
}

// TestFilterThinking tests that FilterThinking correctly separates thinking blocks.
func TestFilterThinking(t *testing.T) {
	tests := []struct {
		name          string
		blocks        []types.ContentBlock
		wantClean     int
		wantThinking  int
	}{
		{
			name:          "nil input",
			blocks:        nil,
			wantClean:     0,
			wantThinking:  0,
		},
		{
			name:          "empty input",
			blocks:        []types.ContentBlock{},
			wantClean:     0,
			wantThinking:  0,
		},
		{
			name: "no thinking blocks",
			blocks: []types.ContentBlock{
				{Type: "text", Text: "Hello"},
				{Type: "tool_use", ID: "call_1", Name: "read_file"},
			},
			wantClean:    2,
			wantThinking: 0,
		},
		{
			name: "only thinking blocks",
			blocks: []types.ContentBlock{
				{Type: "thinking", Text: "Let me reason..."},
				{Type: "thinking", Text: "So the answer is..."},
			},
			wantClean:    0,
			wantThinking: 2,
		},
		{
			name: "mixed blocks",
			blocks: []types.ContentBlock{
				{Type: "thinking", Text: "Reasoning here"},
				{Type: "text", Text: "The answer is 42"},
				{Type: "tool_use", ID: "call_1", Name: "calc"},
				{Type: "thinking", Text: "More reasoning"},
			},
			wantClean:    2,
			wantThinking: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clean, thinking := FilterThinking(tc.blocks)
			if len(clean) != tc.wantClean {
				t.Errorf("clean count = %d, want %d", len(clean), tc.wantClean)
			}
			if len(thinking) != tc.wantThinking {
				t.Errorf("thinking count = %d, want %d", len(thinking), tc.wantThinking)
			}
			// Verify no thinking blocks in clean slice.
			for _, b := range clean {
				if b.Type == "thinking" {
					t.Error("clean slice contains a thinking block")
				}
			}
			// Verify all thinking blocks are thinking type.
			for _, b := range thinking {
				if b.Type != "thinking" {
					t.Errorf("thinking slice contains block of type %q", b.Type)
				}
			}
		})
	}
}

// BenchmarkHasToolUse benchmarks the HasToolUse function.
func BenchmarkHasToolUse(b *testing.B) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "Some text"},
				{Type: "text", Text: "More text"},
				{Type: "tool_use", ID: "call_1", Name: "read_file"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HasToolUse(msg)
	}
}

// BenchmarkExtractText benchmarks the ExtractText function.
func BenchmarkExtractText(b *testing.B) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "Hello"},
				{Type: "text", Text: " "},
				{Type: "text", Text: "world"},
				{Type: "tool_use", ID: "call_1", Name: "some_tool"},
				{Type: "text", Text: "!"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractText(msg)
	}
}

// BenchmarkExtractToolCalls benchmarks the ExtractToolCalls function.
func BenchmarkExtractToolCalls(b *testing.B) {
	msg := &types.AssistantMessage{
		Message: types.Message{
			Content: []types.ContentBlock{
				{Type: "text", Text: "Some text"},
				{Type: "tool_use", ID: "call_1", Name: "tool_a"},
				{Type: "text", Text: "More text"},
				{Type: "tool_use", ID: "call_2", Name: "tool_b"},
				{Type: "tool_use", ID: "call_3", Name: "tool_c"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractToolCalls(msg)
	}
}
