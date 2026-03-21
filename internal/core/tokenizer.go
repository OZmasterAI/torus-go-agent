package core

import "encoding/json"

// Tokenizer provides token counting for messages before sending to the LLM.
// Since no native Go tokenizer exists for Claude models, we use a calibrated
// character-based estimate. Empirically, len(JSON) / 3.5 is more accurate than
// the commonly cited / 4 for Claude's tokenizer.

const charsPerToken = 3.5

// EstimateTokens returns a calibrated token estimate for a slice of messages.
// It marshals the messages to JSON and divides by charsPerToken.
// This is the fast path — O(n) in message content size, no network calls.
func EstimateTokens(messages []Message) int {
	data, err := json.Marshal(messages)
	if err != nil {
		// Fall back to rough sum of content lengths.
		total := 0
		for _, m := range messages {
			for _, b := range m.Content {
				total += len(b.Text) + len(b.Content)
			}
		}
		return int(float64(total) / charsPerToken)
	}
	return int(float64(len(data)) / charsPerToken)
}

// EstimateTokensForText returns a calibrated token estimate for a single string.
func EstimateTokensForText(text string) int {
	return int(float64(len(text)) / charsPerToken)
}

// EstimatePromptCost returns a total token estimate for a full request:
// system prompt + messages + tool schemas combined.
// Tools are marshaled to JSON to account for their schema definitions.
func EstimatePromptCost(systemPrompt string, messages []Message, tools []Tool) int {
	total := EstimateTokensForText(systemPrompt)
	total += EstimateTokens(messages)
	if len(tools) > 0 {
		total += estimateToolTokens(tools)
	}
	return total
}

// estimateToolTokens marshals the tool definitions (name, description, schema)
// to JSON and estimates their token cost.
func estimateToolTokens(tools []Tool) int {
	// Marshal only the JSON-visible fields (Execute is tagged json:"-").
	data, err := json.Marshal(tools)
	if err != nil {
		// Rough fallback: sum name + description lengths.
		total := 0
		for _, t := range tools {
			total += len(t.Name) + len(t.Description)
		}
		return int(float64(total) / charsPerToken)
	}
	return int(float64(len(data)) / charsPerToken)
}

