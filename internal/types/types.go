// Package types holds shared type definitions used by core, providers, and other packages.
package types
import "context"

// Role identifies the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// ContentBlock is a piece of content within a message.
type ContentBlock struct {
	Type      string         `json:"type"`                 // "text", "tool_use", "tool_result"
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`         // tool_use ID
	Name      string         `json:"name,omitempty"`       // tool name
	Input     map[string]any `json:"input,omitempty"`      // tool arguments
	ToolUseID string         `json:"tool_use_id,omitempty"`// for tool_result
	Content   string         `json:"content,omitempty"`    // tool result text
	IsError   bool           `json:"is_error,omitempty"`
}

// Message is a conversation message.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_creation_input_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
}

// AssistantMessage is an LLM response with metadata.
type AssistantMessage struct {
	Message
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      Usage  `json:"usage"`
}

// ToolResult is the output from executing a tool.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// Tool defines an executable tool the agent can call.
type Tool struct {
	Name        string                                           `json:"name"`
	Description string                                           `json:"description"`
	InputSchema map[string]any                                   `json:"input_schema"`
	Execute     func(args map[string]any) (*ToolResult, error)   `json:"-"`
}

// ProviderConfig holds connection details for an LLM provider.
type ProviderConfig struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	MaxTokens int    `json:"max_tokens"`
}

// AgentConfig is the top-level configuration for an agent.
type AgentConfig struct {
	Provider          ProviderConfig `json:"provider"`
	SystemPrompt      string         `json:"system_prompt"`
	Tools             []Tool         `json:"-"`
	MaxTurns          int            `json:"max_turns"`
	CompactionModel   string         `json:"compaction_model,omitempty"`
	SmartRouting      bool           `json:"smart_routing"`
	SmartRoutingModel string         `json:"smart_routing_model,omitempty"`
}

// Provider is the interface all LLM providers implement.
type Provider interface {
	Complete(ctx context.Context, systemPrompt string, messages []Message, tools []Tool, maxTokens int) (*AssistantMessage, error)
	Name() string
	ModelID() string
}
