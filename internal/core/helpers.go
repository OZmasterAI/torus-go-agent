package core

import "strings"

// HasToolUse checks if an assistant message wants to use tools.
func HasToolUse(msg *AssistantMessage) bool {
	for _, b := range msg.Content {
		if b.Type == "tool_use" {
			return true
		}
	}
	return false
}

// ExtractToolCalls returns all tool_use blocks from a response.
func ExtractToolCalls(msg *AssistantMessage) []ContentBlock {
	var calls []ContentBlock
	for _, b := range msg.Content {
		if b.Type == "tool_use" {
			calls = append(calls, b)
		}
	}
	return calls
}

// ExtractText returns concatenated text from a response.
func ExtractText(msg *AssistantMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}
