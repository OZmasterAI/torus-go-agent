package core

import (
	"strings"
)

// AgentEventType identifies the kind of agent-level event.
type AgentEventType string

const (
	EventAgentTextDelta AgentEventType = "text_delta"   // streaming text fragment
	EventAgentToolStart AgentEventType = "tool_start"   // tool execution starting
	EventAgentToolEnd   AgentEventType = "tool_end"     // tool execution finished
	EventAgentTurnStart AgentEventType = "turn_start"   // new loop turn beginning
	EventAgentTurnEnd   AgentEventType = "turn_end"     // loop turn finished
	EventAgentDone      AgentEventType = "done"         // final text ready
	EventAgentError     AgentEventType = "error"        // fatal error
)

// AgentEvent is a single event emitted by RunStream.
type AgentEvent struct {
	Type       AgentEventType
	Text       string          // done: final text, text_delta: fragment
	Turn       int             // turn_start/turn_end: turn number
	ToolName   string          // tool_start/tool_end: tool name
	ToolArgs   map[string]any  // tool_start: arguments
	ToolResult *ToolResult     // tool_end: result
	Error      error           // error: the error
}

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
