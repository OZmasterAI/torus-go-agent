package core

import (
	"testing"

	typ "torus_go_agent/internal/types"
)

// TestEstimateTokensEmpty tests EstimateTokens with an empty message slice.
func TestEstimateTokensEmpty(t *testing.T) {
	messages := []typ.Message{}
	tokens := EstimateTokens(messages)
	if tokens != 0 {
		t.Errorf("EstimateTokens([]) = %d, want 0", tokens)
	}
}

// TestEstimateTokensSingleMessage tests EstimateTokens with a single message.
func TestEstimateTokensSingleMessage(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Hello, world!",
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	// "Hello, world!" is 13 chars. JSON marshalling adds overhead.
	// Rough estimate: len(JSON) / 3.5 should be > 0
	if tokens <= 0 {
		t.Errorf("EstimateTokens(single message) = %d, want > 0", tokens)
	}
}

// TestEstimateTokensMultipleMessages tests EstimateTokens with multiple messages.
func TestEstimateTokensMultipleMessages(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "First message",
				},
			},
		},
		{
			Role: typ.RoleAssistant,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Second message",
				},
			},
		},
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Third message",
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	if tokens <= 0 {
		t.Errorf("EstimateTokens(multiple messages) = %d, want > 0", tokens)
	}
	// Verify that more messages result in more tokens
	singleMsg := []typ.Message{messages[0]}
	singleTokens := EstimateTokens(singleMsg)
	if tokens <= singleTokens {
		t.Errorf("EstimateTokens(3 messages) = %d should be > EstimateTokens(1 message) = %d", tokens, singleTokens)
	}
}

// TestEstimateTokensMultipleContentBlocks tests EstimateTokens with multiple content blocks in a single message.
func TestEstimateTokensMultipleContentBlocks(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleAssistant,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Here's a function:",
				},
				{
					Type: "tool_use",
					ID:   "tool_use_123",
					Name: "code_interpreter",
					Input: map[string]any{
						"code": "print('hello')",
					},
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	if tokens <= 0 {
		t.Errorf("EstimateTokens(multiple blocks) = %d, want > 0", tokens)
	}
}

// TestEstimateTokensToolResult tests EstimateTokens with tool result blocks.
func TestEstimateTokensToolResult(t *testing.T) {
	messages := []typ.Message{
		{
			Role: typ.RoleTool,
			Content: []typ.ContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: "tool_use_123",
					Content:   "Result of the tool execution",
					IsError:   false,
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	if tokens <= 0 {
		t.Errorf("EstimateTokens(tool result) = %d, want > 0", tokens)
	}
}

// TestEstimateTokensLongContent tests EstimateTokens with longer content.
func TestEstimateTokensLongContent(t *testing.T) {
	longText := string(make([]byte, 1000)) // 1000 bytes of null characters
	for i := 0; i < 1000; i++ {
		longText = longText[:i] + "a" + longText[i+1:]
	}
	longText = longText[:1000]

	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: longText,
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	// 1000 bytes / 3.5 ≈ 285 tokens minimum
	if tokens < 200 {
		t.Errorf("EstimateTokens(long content ~1000 bytes) = %d, want >= ~200", tokens)
	}
}

// TestEstimateTokensForTextEmpty tests EstimateTokensForText with an empty string.
func TestEstimateTokensForTextEmpty(t *testing.T) {
	tokens := EstimateTokensForText("")
	if tokens != 0 {
		t.Errorf("EstimateTokensForText(\"\") = %d, want 0", tokens)
	}
}

// TestEstimateTokensForTextShort tests EstimateTokensForText with short text.
func TestEstimateTokensForTextShort(t *testing.T) {
	tokens := EstimateTokensForText("Hello")
	// 5 chars / 3.5 ≈ 1 token
	if tokens <= 0 {
		t.Errorf("EstimateTokensForText(\"Hello\") = %d, want > 0", tokens)
	}
}

// TestEstimateTokensForTextMedium tests EstimateTokensForText with medium text.
func TestEstimateTokensForTextMedium(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. " +
		"This is a medium length text for testing token estimation. " +
		"It should be around 100-150 characters."
	tokens := EstimateTokensForText(text)
	// ~170 chars / 3.5 ≈ 48 tokens
	if tokens <= 30 || tokens >= 100 {
		t.Errorf("EstimateTokensForText(medium text ~170 chars) = %d, want ~40-60", tokens)
	}
}

// TestEstimateTokensForTextLong tests EstimateTokensForText with long text.
func TestEstimateTokensForTextLong(t *testing.T) {
	// Generate a 5000 character string
	longText := ""
	for i := 0; i < 500; i++ {
		longText += "The quick brown fox jumps over the lazy dog. "
	}
	longText = longText[:5000]

	tokens := EstimateTokensForText(longText)
	// 5000 chars / 3.5 ≈ 1428 tokens
	if tokens < 1000 || tokens > 2000 {
		t.Errorf("EstimateTokensForText(long text ~5000 chars) = %d, want ~1000-2000", tokens)
	}
}

// TestEstimateTokensForTextProportional tests that EstimateTokensForText is proportional to text length.
func TestEstimateTokensForTextProportional(t *testing.T) {
	text1 := "Hello world"
	text2 := "Hello world Hello world Hello world Hello world" // Same text repeated 4 times (approximately)

	tokens1 := EstimateTokensForText(text1)
	tokens2 := EstimateTokensForText(text2)

	if tokens2 <= tokens1 {
		t.Errorf("EstimateTokensForText(longer text) = %d should be > %d", tokens2, tokens1)
	}
	// tokens2 should be roughly 4x tokens1 (allowing some slack)
	ratio := float64(tokens2) / float64(tokens1)
	if ratio < 3.5 || ratio > 4.5 {
		t.Logf("Ratio of tokens for 4x text: %.2f (expected ~4.0)", ratio)
	}
}

// TestEstimatePromptCostEmpty tests EstimatePromptCost with empty inputs.
func TestEstimatePromptCostEmpty(t *testing.T) {
	cost := EstimatePromptCost("", []typ.Message{}, []typ.Tool{})
	if cost != 0 {
		t.Errorf("EstimatePromptCost(\"\", [], []) = %d, want 0", cost)
	}
}

// TestEstimatePromptCostSystemPromptOnly tests EstimatePromptCost with only a system prompt.
func TestEstimatePromptCostSystemPromptOnly(t *testing.T) {
	systemPrompt := "You are a helpful assistant."
	cost := EstimatePromptCost(systemPrompt, []typ.Message{}, []typ.Tool{})
	// len("You are a helpful assistant.") = 28 chars / 3.5 ≈ 8 tokens
	if cost <= 0 {
		t.Errorf("EstimatePromptCost(systemPrompt, [], []) = %d, want > 0", cost)
	}
	expectedMin := len(systemPrompt) / 4 // rough lower bound
	if cost < expectedMin {
		t.Errorf("EstimatePromptCost(systemPrompt) = %d, want >= ~%d", cost, expectedMin)
	}
}

// TestEstimatePromptCostWithMessages tests EstimatePromptCost with system prompt and messages.
func TestEstimatePromptCostWithMessages(t *testing.T) {
	systemPrompt := "You are a helpful assistant."
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Hello, how are you?",
				},
			},
		},
	}
	cost := EstimatePromptCost(systemPrompt, messages, []typ.Tool{})

	systemCost := EstimatePromptCost(systemPrompt, []typ.Message{}, []typ.Tool{})
	messageCost := EstimatePromptCost("", messages, []typ.Tool{})

	if cost != systemCost+messageCost {
		t.Errorf("EstimatePromptCost(system + messages) = %d, want %d (system) + %d (messages) = %d",
			cost, systemCost, messageCost, systemCost+messageCost)
	}
}

// TestEstimatePromptCostWithTools tests EstimatePromptCost with system prompt, messages, and tools.
func TestEstimatePromptCostWithTools(t *testing.T) {
	systemPrompt := "You are a helpful assistant."
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "What's the weather?",
				},
			},
		},
	}
	tools := []typ.Tool{
		{
			Name:        "get_weather",
			Description: "Get current weather for a location",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	cost := EstimatePromptCost(systemPrompt, messages, tools)

	costWithoutTools := EstimatePromptCost(systemPrompt, messages, []typ.Tool{})

	if cost <= costWithoutTools {
		t.Errorf("EstimatePromptCost(with tools) = %d should be > EstimatePromptCost(without tools) = %d",
			cost, costWithoutTools)
	}
}

// TestEstimatePromptCostMultipleTools tests EstimatePromptCost with multiple tools.
func TestEstimatePromptCostMultipleTools(t *testing.T) {
	systemPrompt := "You are a helpful assistant with tool access."
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Execute a task",
				},
			},
		},
	}
	tools := []typ.Tool{
		{
			Name:        "tool_one",
			Description: "First tool",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "tool_two",
			Description: "Second tool",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "tool_three",
			Description: "Third tool with a longer description for testing",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"param1": map[string]any{"type": "string"},
					"param2": map[string]any{"type": "number"},
				},
			},
		},
	}

	cost := EstimatePromptCost(systemPrompt, messages, tools)
	costWithFewerTools := EstimatePromptCost(systemPrompt, messages, tools[:2])

	if cost <= costWithFewerTools {
		t.Errorf("EstimatePromptCost(3 tools) = %d should be > EstimatePromptCost(2 tools) = %d",
			cost, costWithFewerTools)
	}
}

// TestEstimatePromptCostComplexScenario tests EstimatePromptCost with a realistic complex scenario.
func TestEstimatePromptCostComplexScenario(t *testing.T) {
	systemPrompt := `You are a research assistant with access to multiple tools.
Your goal is to help users find information and execute tasks.
Always prioritize accuracy and cite sources.`

	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Find recent research papers on machine learning",
				},
			},
		},
		{
			Role: typ.RoleAssistant,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "I'll search for recent machine learning papers.",
				},
				{
					Type: "tool_use",
					ID:   "search_001",
					Name: "search_papers",
					Input: map[string]any{
						"query": "machine learning",
						"limit": 10,
					},
				},
			},
		},
		{
			Role: typ.RoleTool,
			Content: []typ.ContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: "search_001",
					Content:   "Found 25 papers on machine learning from 2024-2025",
					IsError:   false,
				},
			},
		},
	}

	tools := []typ.Tool{
		{
			Name:        "search_papers",
			Description: "Search academic papers by query",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "get_paper_details",
			Description: "Get detailed information about a specific paper",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paper_id": map[string]any{"type": "string"},
				},
				"required": []string{"paper_id"},
			},
		},
	}

	cost := EstimatePromptCost(systemPrompt, messages, tools)

	// Verify that the cost is a reasonable positive number
	if cost <= 0 {
		t.Errorf("EstimatePromptCost(complex scenario) = %d, want > 0", cost)
	}

	// Rough sanity check: should be at least 100 tokens (system prompt + messages + tools)
	if cost < 50 {
		t.Logf("Warning: EstimatePromptCost(complex scenario) = %d seems low", cost)
	}
}

// TestEstimateTokensFallback tests EstimateTokens fallback behavior.
// This is a white-box test to ensure the fallback path is functional.
func TestEstimateTokensFallback(t *testing.T) {
	// Create a message with normal content to test the happy path
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Normal message",
				},
			},
		},
	}
	tokens := EstimateTokens(messages)
	if tokens <= 0 {
		t.Errorf("EstimateTokens(normal message) = %d, want > 0", tokens)
	}

	// Messages with only Content field populated should also work
	messages2 := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type:    "text",
					Content: "Content field populated",
				},
			},
		},
	}
	tokens2 := EstimateTokens(messages2)
	if tokens2 <= 0 {
		t.Errorf("EstimateTokens(message with Content field) = %d, want > 0", tokens2)
	}
}

// BenchmarkEstimateTokens benchmarks EstimateTokens performance.
func BenchmarkEstimateTokens(b *testing.B) {
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "This is a sample message for benchmarking token estimation.",
				},
			},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(messages)
	}
}

// BenchmarkEstimateTokensForText benchmarks EstimateTokensForText performance.
func BenchmarkEstimateTokensForText(b *testing.B) {
	text := "This is a sample text for benchmarking token estimation. " +
		"It contains multiple sentences to simulate real usage patterns. " +
		"Performance should be O(n) where n is the length of the text."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokensForText(text)
	}
}

// BenchmarkEstimatePromptCost benchmarks EstimatePromptCost performance.
func BenchmarkEstimatePromptCost(b *testing.B) {
	systemPrompt := "You are a helpful assistant."
	messages := []typ.Message{
		{
			Role: typ.RoleUser,
			Content: []typ.ContentBlock{
				{
					Type: "text",
					Text: "Hello, assistant!",
				},
			},
		},
	}
	tools := []typ.Tool{
		{
			Name:        "sample_tool",
			Description: "A sample tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimatePromptCost(systemPrompt, messages, tools)
	}
}
