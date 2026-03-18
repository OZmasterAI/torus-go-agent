// Re-exports from types package for backward compatibility.
package core

import t "go_sdk_agent/internal/types"

// Re-export all types so existing code in core still compiles.
type Role = t.Role
type ContentBlock = t.ContentBlock
type Message = t.Message
type Usage = t.Usage
type AssistantMessage = t.AssistantMessage
type ToolResult = t.ToolResult
type Tool = t.Tool
type ProviderConfig = t.ProviderConfig
type AgentConfig = t.AgentConfig

const (
	RoleUser      = t.RoleUser
	RoleAssistant = t.RoleAssistant
	RoleSystem    = t.RoleSystem
	RoleTool      = t.RoleTool
)
type Provider = t.Provider
