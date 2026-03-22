package types

import (
	"context"
	"encoding/json"
	"testing"
)

// TestRoleConstants verifies all role constants are defined correctly.
func TestRoleConstants(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{"RoleUser", RoleUser, "user"},
		{"RoleAssistant", RoleAssistant, "assistant"},
		{"RoleSystem", RoleSystem, "system"},
		{"RoleTool", RoleTool, "tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.role) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.role))
			}
		})
	}
}

// TestStreamEventTypeConstants verifies all stream event type constants.
func TestStreamEventTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		eventType StreamEventType
		expected string
	}{
		{"EventTextDelta", EventTextDelta, "text_delta"},
		{"EventToolUseStart", EventToolUseStart, "tool_use_start"},
		{"EventInputDelta", EventInputDelta, "input_delta"},
		{"EventContentBlockStop", EventContentBlockStop, "content_block_stop"},
		{"EventMessageStop", EventMessageStop, "message_stop"},
		{"EventError", EventError, "error"},
		{"EventUsage", EventUsage, "usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.eventType))
			}
		})
	}
}

// TestContentBlockJSON verifies ContentBlock JSON marshaling/unmarshaling.
func TestContentBlockJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   ContentBlock
		wantErr bool
	}{
		{
			name: "text content block",
			input: ContentBlock{
				Type: "text",
				Text: "Hello world",
			},
			wantErr: false,
		},
		{
			name: "tool_use content block",
			input: ContentBlock{
				Type:  "tool_use",
				ID:    "tool-123",
				Name:  "bash",
				Input: map[string]any{"command": "ls -la"},
			},
			wantErr: false,
		},
		{
			name: "tool_result content block",
			input: ContentBlock{
				Type:      "tool_result",
				ToolUseID: "tool-123",
				Content:   "file1.txt\nfile2.txt",
				IsError:   false,
			},
			wantErr: false,
		},
		{
			name: "tool_result with error",
			input: ContentBlock{
				Type:      "tool_result",
				ToolUseID: "tool-456",
				Content:   "command not found",
				IsError:   true,
			},
			wantErr: false,
		},
		{
			name: "empty content block",
			input: ContentBlock{
				Type: "text",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got ContentBlock
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.Type != tt.input.Type {
				t.Errorf("Type: expected %q, got %q", tt.input.Type, got.Type)
			}
			if got.Text != tt.input.Text {
				t.Errorf("Text: expected %q, got %q", tt.input.Text, got.Text)
			}
			if got.ID != tt.input.ID {
				t.Errorf("ID: expected %q, got %q", tt.input.ID, got.ID)
			}
			if got.Name != tt.input.Name {
				t.Errorf("Name: expected %q, got %q", tt.input.Name, got.Name)
			}
			if got.ToolUseID != tt.input.ToolUseID {
				t.Errorf("ToolUseID: expected %q, got %q", tt.input.ToolUseID, got.ToolUseID)
			}
			if got.Content != tt.input.Content {
				t.Errorf("Content: expected %q, got %q", tt.input.Content, got.Content)
			}
			if got.IsError != tt.input.IsError {
				t.Errorf("IsError: expected %v, got %v", tt.input.IsError, got.IsError)
			}
		})
	}
}

// TestMessageJSON verifies Message JSON marshaling/unmarshaling.
func TestMessageJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   Message
		wantErr bool
	}{
		{
			name: "user message with text",
			input: Message{
				Role: RoleUser,
				Content: []ContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
			wantErr: false,
		},
		{
			name: "assistant message with multiple blocks",
			input: Message{
				Role: RoleAssistant,
				Content: []ContentBlock{
					{Type: "text", Text: "I'll help"},
					{Type: "tool_use", ID: "tool-1", Name: "bash", Input: map[string]any{}},
				},
			},
			wantErr: false,
		},
		{
			name: "system message",
			input: Message{
				Role: RoleSystem,
				Content: []ContentBlock{
					{Type: "text", Text: "You are helpful"},
				},
			},
			wantErr: false,
		},
		{
			name: "tool message",
			input: Message{
				Role: RoleTool,
				Content: []ContentBlock{
					{Type: "tool_result", ToolUseID: "tool-1", Content: "result"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty message",
			input: Message{
				Role:    RoleUser,
				Content: []ContentBlock{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got Message
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.Role != tt.input.Role {
				t.Errorf("Role: expected %q, got %q", tt.input.Role, got.Role)
			}
			if len(got.Content) != len(tt.input.Content) {
				t.Errorf("Content length: expected %d, got %d", len(tt.input.Content), len(got.Content))
			}
		})
	}
}

// TestUsageJSON verifies Usage JSON marshaling/unmarshaling.
func TestUsageJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   Usage
		wantErr bool
	}{
		{
			name: "basic usage",
			input: Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
			wantErr: false,
		},
		{
			name: "usage with cache",
			input: Usage{
				InputTokens:      100,
				OutputTokens:     50,
				CacheReadTokens:  25,
				CacheWriteTokens: 10,
				TotalTokens:      175,
			},
			wantErr: false,
		},
		{
			name: "usage with cost",
			input: Usage{
				InputTokens:  1000,
				OutputTokens: 500,
				TotalTokens:  1500,
				Cost:         0.0123,
			},
			wantErr: false,
		},
		{
			name: "zero usage",
			input: Usage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got Usage
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.InputTokens != tt.input.InputTokens {
				t.Errorf("InputTokens: expected %d, got %d", tt.input.InputTokens, got.InputTokens)
			}
			if got.OutputTokens != tt.input.OutputTokens {
				t.Errorf("OutputTokens: expected %d, got %d", tt.input.OutputTokens, got.OutputTokens)
			}
			if got.CacheReadTokens != tt.input.CacheReadTokens {
				t.Errorf("CacheReadTokens: expected %d, got %d", tt.input.CacheReadTokens, got.CacheReadTokens)
			}
			if got.CacheWriteTokens != tt.input.CacheWriteTokens {
				t.Errorf("CacheWriteTokens: expected %d, got %d", tt.input.CacheWriteTokens, got.CacheWriteTokens)
			}
			if got.TotalTokens != tt.input.TotalTokens {
				t.Errorf("TotalTokens: expected %d, got %d", tt.input.TotalTokens, got.TotalTokens)
			}
			if got.Cost != tt.input.Cost {
				t.Errorf("Cost: expected %f, got %f", tt.input.Cost, got.Cost)
			}
		})
	}
}

// TestAssistantMessageJSON verifies AssistantMessage JSON marshaling/unmarshaling.
func TestAssistantMessageJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   AssistantMessage
		wantErr bool
	}{
		{
			name: "simple assistant response",
			input: AssistantMessage{
				Message: Message{
					Role: RoleAssistant,
					Content: []ContentBlock{
						{Type: "text", Text: "Hello user"},
					},
				},
				Model:      "claude-3-opus",
				StopReason: "end_turn",
				Usage: Usage{
					InputTokens:  50,
					OutputTokens: 25,
					TotalTokens:  75,
				},
			},
			wantErr: false,
		},
		{
			name: "response with tool use",
			input: AssistantMessage{
				Message: Message{
					Role: RoleAssistant,
					Content: []ContentBlock{
						{Type: "text", Text: "I'll run a command"},
						{Type: "tool_use", ID: "tool-999", Name: "bash", Input: map[string]any{"cmd": "echo test"}},
					},
				},
				Model:      "claude-3-sonnet",
				StopReason: "tool_use",
				Usage: Usage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
					Cost:         0.0045,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got AssistantMessage
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.Model != tt.input.Model {
				t.Errorf("Model: expected %q, got %q", tt.input.Model, got.Model)
			}
			if got.StopReason != tt.input.StopReason {
				t.Errorf("StopReason: expected %q, got %q", tt.input.StopReason, got.StopReason)
			}
			if got.Role != tt.input.Role {
				t.Errorf("Role: expected %q, got %q", tt.input.Role, got.Role)
			}
		})
	}
}

// TestStreamEventStructure verifies StreamEvent fields and types.
func TestStreamEventStructure(t *testing.T) {
	tests := []struct {
		name  string
		event StreamEvent
		check func(t *testing.T, e StreamEvent)
	}{
		{
			name: "text delta event",
			event: StreamEvent{
				Type:         EventTextDelta,
				Text:         "Hello ",
				ContentIndex: 0,
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventTextDelta {
					t.Errorf("Type: expected %q, got %q", EventTextDelta, e.Type)
				}
				if e.Text != "Hello " {
					t.Errorf("Text: expected %q, got %q", "Hello ", e.Text)
				}
				if e.ContentIndex != 0 {
					t.Errorf("ContentIndex: expected 0, got %d", e.ContentIndex)
				}
			},
		},
		{
			name: "tool use start event",
			event: StreamEvent{
				Type:         EventToolUseStart,
				ID:           "tool-abc",
				Name:         "bash",
				ContentIndex: 1,
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventToolUseStart {
					t.Errorf("Type: expected %q, got %q", EventToolUseStart, e.Type)
				}
				if e.ID != "tool-abc" {
					t.Errorf("ID: expected %q, got %q", "tool-abc", e.ID)
				}
				if e.Name != "bash" {
					t.Errorf("Name: expected %q, got %q", "bash", e.Name)
				}
			},
		},
		{
			name: "input delta event",
			event: StreamEvent{
				Type:         EventInputDelta,
				InputDelta:   `{"command":"ls"}`,
				ContentIndex: 1,
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventInputDelta {
					t.Errorf("Type: expected %q, got %q", EventInputDelta, e.Type)
				}
				if e.InputDelta != `{"command":"ls"}` {
					t.Errorf("InputDelta: expected %q, got %q", `{"command":"ls"}`, e.InputDelta)
				}
			},
		},
		{
			name: "content block stop event",
			event: StreamEvent{
				Type:         EventContentBlockStop,
				ContentIndex: 1,
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventContentBlockStop {
					t.Errorf("Type: expected %q, got %q", EventContentBlockStop, e.Type)
				}
				if e.ContentIndex != 1 {
					t.Errorf("ContentIndex: expected 1, got %d", e.ContentIndex)
				}
			},
		},
		{
			name: "message stop event",
			event: StreamEvent{
				Type:       EventMessageStop,
				StopReason: "end_turn",
				Response: &AssistantMessage{
					Message: Message{
						Role: RoleAssistant,
						Content: []ContentBlock{
							{Type: "text", Text: "Response"},
						},
					},
					Model:      "claude-3",
					StopReason: "end_turn",
				},
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventMessageStop {
					t.Errorf("Type: expected %q, got %q", EventMessageStop, e.Type)
				}
				if e.StopReason != "end_turn" {
					t.Errorf("StopReason: expected %q, got %q", "end_turn", e.StopReason)
				}
				if e.Response == nil {
					t.Error("Response: expected non-nil")
				}
			},
		},
		{
			name: "usage event",
			event: StreamEvent{
				Type: EventUsage,
				Usage: &Usage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
				},
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventUsage {
					t.Errorf("Type: expected %q, got %q", EventUsage, e.Type)
				}
				if e.Usage == nil {
					t.Error("Usage: expected non-nil")
				} else if e.Usage.TotalTokens != 150 {
					t.Errorf("TotalTokens: expected 150, got %d", e.Usage.TotalTokens)
				}
			},
		},
		{
			name: "error event",
			event: StreamEvent{
				Type:  EventError,
				Error: context.Canceled,
			},
			check: func(t *testing.T, e StreamEvent) {
				if e.Type != EventError {
					t.Errorf("Type: expected %q, got %q", EventError, e.Type)
				}
				if e.Error != context.Canceled {
					t.Errorf("Error: expected %v, got %v", context.Canceled, e.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.event)
		})
	}
}

// TestToolResultJSON verifies ToolResult JSON marshaling/unmarshaling.
func TestToolResultJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   ToolResult
		wantErr bool
	}{
		{
			name: "successful tool result",
			input: ToolResult{
				ToolUseID: "tool-123",
				Content:   "Success output",
				IsError:   false,
			},
			wantErr: false,
		},
		{
			name: "error tool result",
			input: ToolResult{
				ToolUseID: "tool-456",
				Content:   "Error message",
				IsError:   true,
			},
			wantErr: false,
		},
		{
			name: "empty tool result",
			input: ToolResult{
				ToolUseID: "tool-789",
				Content:   "",
				IsError:   false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got ToolResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.ToolUseID != tt.input.ToolUseID {
				t.Errorf("ToolUseID: expected %q, got %q", tt.input.ToolUseID, got.ToolUseID)
			}
			if got.Content != tt.input.Content {
				t.Errorf("Content: expected %q, got %q", tt.input.Content, got.Content)
			}
			if got.IsError != tt.input.IsError {
				t.Errorf("IsError: expected %v, got %v", tt.input.IsError, got.IsError)
			}
		})
	}
}

// TestToolStructure verifies Tool fields and Execute function signature.
func TestToolStructure(t *testing.T) {
	executeCalled := false
	mockExecute := func(args map[string]any) (*ToolResult, error) {
		executeCalled = true
		return &ToolResult{
			ToolUseID: "mock-tool",
			Content:   "executed",
			IsError:   false,
		}, nil
	}

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"param1": "string",
			"param2": 42,
		},
		Execute: mockExecute,
	}

	// Verify fields
	if tool.Name != "test_tool" {
		t.Errorf("Name: expected %q, got %q", "test_tool", tool.Name)
	}
	if tool.Description != "A test tool" {
		t.Errorf("Description: expected %q, got %q", "A test tool", tool.Description)
	}
	if len(tool.InputSchema) != 2 {
		t.Errorf("InputSchema length: expected 2, got %d", len(tool.InputSchema))
	}

	// Verify Execute function
	result, err := tool.Execute(map[string]any{})
	if err != nil {
		t.Errorf("Execute: unexpected error %v", err)
	}
	if !executeCalled {
		t.Error("Execute: function was not called")
	}
	if result == nil {
		t.Error("Execute: returned nil result")
	} else if result.Content != "executed" {
		t.Errorf("Execute: result content expected %q, got %q", "executed", result.Content)
	}
}

// TestToolJSON verifies Tool JSON marshaling (Execute field excluded).
func TestToolJSON(t *testing.T) {
	tool := Tool{
		Name:        "my_tool",
		Description: "Does something",
		InputSchema: map[string]any{"arg": "value"},
		Execute:     nil, // Will be omitted during JSON marshaling
	}

	// Marshal
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Unmarshal
	var got Tool
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify
	if got.Name != tool.Name {
		t.Errorf("Name: expected %q, got %q", tool.Name, got.Name)
	}
	if got.Description != tool.Description {
		t.Errorf("Description: expected %q, got %q", tool.Description, got.Description)
	}
	if len(got.InputSchema) != len(tool.InputSchema) {
		t.Errorf("InputSchema length: expected %d, got %d", len(tool.InputSchema), len(got.InputSchema))
	}
}

// TestProviderConfigJSON verifies ProviderConfig JSON marshaling/unmarshaling.
func TestProviderConfigJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   ProviderConfig
		wantErr bool
	}{
		{
			name: "anthropic provider",
			input: ProviderConfig{
				Name:      "anthropic",
				Model:     "claude-3-opus-20240229",
				BaseURL:   "https://api.anthropic.com",
				APIKey:    "sk-ant-xxx",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "openai provider",
			input: ProviderConfig{
				Name:      "openai",
				Model:     "gpt-4",
				BaseURL:   "https://api.openai.com/v1",
				APIKey:    "sk-xxx",
				MaxTokens: 8192,
			},
			wantErr: false,
		},
		{
			name: "local provider",
			input: ProviderConfig{
				Name:      "local",
				Model:     "llama2",
				BaseURL:   "http://localhost:8000",
				APIKey:    "",
				MaxTokens: 2048,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got ProviderConfig
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.Name != tt.input.Name {
				t.Errorf("Name: expected %q, got %q", tt.input.Name, got.Name)
			}
			if got.Model != tt.input.Model {
				t.Errorf("Model: expected %q, got %q", tt.input.Model, got.Model)
			}
			if got.BaseURL != tt.input.BaseURL {
				t.Errorf("BaseURL: expected %q, got %q", tt.input.BaseURL, got.BaseURL)
			}
			if got.APIKey != tt.input.APIKey {
				t.Errorf("APIKey: expected %q, got %q", tt.input.APIKey, got.APIKey)
			}
			if got.MaxTokens != tt.input.MaxTokens {
				t.Errorf("MaxTokens: expected %d, got %d", tt.input.MaxTokens, got.MaxTokens)
			}
		})
	}
}

// TestAgentConfigJSON verifies AgentConfig JSON marshaling/unmarshaling.
func TestAgentConfigJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   AgentConfig
		wantErr bool
	}{
		{
			name: "basic agent config",
			input: AgentConfig{
				Provider: ProviderConfig{
					Name:      "anthropic",
					Model:     "claude-3",
					BaseURL:   "https://api.anthropic.com",
					APIKey:    "sk-xxx",
					MaxTokens: 4096,
				},
				SystemPrompt:  "You are helpful",
				Tools:         []Tool{}, // Will not be marshaled
				MaxTurns:      10,
				ContextWindow: 200000,
				SmartRouting:  false,
			},
			wantErr: false,
		},
		{
			name: "agent with smart routing",
			input: AgentConfig{
				Provider: ProviderConfig{
					Name:      "openai",
					Model:     "gpt-4",
					BaseURL:   "https://api.openai.com/v1",
					APIKey:    "sk-xxx",
					MaxTokens: 8192,
				},
				SystemPrompt:      "You are an expert",
				Tools:             []Tool{},
				MaxTurns:          5,
				ContextWindow:     8192,
				SmartRouting:      true,
				SmartRoutingModel: "gpt-4-turbo",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal error: got %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Unmarshal
			var got AgentConfig
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip
			if got.Provider.Name != tt.input.Provider.Name {
				t.Errorf("Provider.Name: expected %q, got %q", tt.input.Provider.Name, got.Provider.Name)
			}
			if got.SystemPrompt != tt.input.SystemPrompt {
				t.Errorf("SystemPrompt: expected %q, got %q", tt.input.SystemPrompt, got.SystemPrompt)
			}
			if got.MaxTurns != tt.input.MaxTurns {
				t.Errorf("MaxTurns: expected %d, got %d", tt.input.MaxTurns, got.MaxTurns)
			}
			if got.ContextWindow != tt.input.ContextWindow {
				t.Errorf("ContextWindow: expected %d, got %d", tt.input.ContextWindow, got.ContextWindow)
			}
			if got.SmartRouting != tt.input.SmartRouting {
				t.Errorf("SmartRouting: expected %v, got %v", tt.input.SmartRouting, got.SmartRouting)
			}
			if got.SmartRoutingModel != tt.input.SmartRoutingModel {
				t.Errorf("SmartRoutingModel: expected %q, got %q", tt.input.SmartRoutingModel, got.SmartRoutingModel)
			}
		})
	}
}

// TestProviderInterface verifies the Provider interface contract.
func TestProviderInterface(t *testing.T) {
	// Create a mock implementation
	mockProvider := &mockProvider{
		name:    "mock",
		modelID: "mock-model",
	}

	// Verify all interface methods are callable
	ctx := context.Background()

	// Test Name method
	if mockProvider.Name() != "mock" {
		t.Errorf("Name: expected %q, got %q", "mock", mockProvider.Name())
	}

	// Test ModelID method
	if mockProvider.ModelID() != "mock-model" {
		t.Errorf("ModelID: expected %q, got %q", "mock-model", mockProvider.ModelID())
	}

	// Test Complete method
	msg, err := mockProvider.Complete(ctx, "system", []Message{}, []Tool{}, 1000)
	if err != nil {
		t.Errorf("Complete: unexpected error %v", err)
	}
	if msg == nil {
		t.Error("Complete: expected non-nil message")
	}

	// Test StreamComplete method
	ch, err := mockProvider.StreamComplete(ctx, "system", []Message{}, []Tool{}, 1000)
	if err != nil {
		t.Errorf("StreamComplete: unexpected error %v", err)
	}
	if ch == nil {
		t.Error("StreamComplete: expected non-nil channel")
	}

	// Drain the channel
	for range ch {
	}
}

// Mock implementation of Provider interface for testing
type mockProvider struct {
	name    string
	modelID string
}

func (m *mockProvider) Complete(ctx context.Context, systemPrompt string, messages []Message, tools []Tool, maxTokens int) (*AssistantMessage, error) {
	return &AssistantMessage{
		Message: Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: "text", Text: "mock response"},
			},
		},
		Model:      m.modelID,
		StopReason: "end_turn",
		Usage: Usage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	}, nil
}

func (m *mockProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []Tool, maxTokens int) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) ModelID() string {
	return m.modelID
}

// TestIntegrationMessageAndContentBlock verifies complex message structures.
func TestIntegrationMessageAndContentBlock(t *testing.T) {
	// Build a complex conversation
	messages := []Message{
		{
			Role: RoleUser,
			Content: []ContentBlock{
				{Type: "text", Text: "Please run a command"},
			},
		},
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: "text", Text: "I'll execute that for you"},
				{Type: "tool_use", ID: "tool-1", Name: "bash", Input: map[string]any{"command": "ls"}},
			},
		},
		{
			Role: RoleTool,
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "tool-1", Content: "file.txt", IsError: false},
			},
		},
	}

	// Marshal and unmarshal the entire conversation
	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got []Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify structure
	if len(got) != 3 {
		t.Errorf("Message count: expected 3, got %d", len(got))
	}

	if got[0].Role != RoleUser {
		t.Errorf("Message 0 role: expected %q, got %q", RoleUser, got[0].Role)
	}

	if len(got[1].Content) != 2 {
		t.Errorf("Message 1 content count: expected 2, got %d", len(got[1].Content))
	}

	if got[1].Content[1].Type != "tool_use" {
		t.Errorf("Message 1 block 1 type: expected %q, got %q", "tool_use", got[1].Content[1].Type)
	}

	if got[2].Content[0].ToolUseID != "tool-1" {
		t.Errorf("Tool result ToolUseID: expected %q, got %q", "tool-1", got[2].Content[0].ToolUseID)
	}
}

// TestRoleTypeConversion verifies Role type conversions.
func TestRoleTypeConversion(t *testing.T) {
	// Test conversion to string
	role := RoleAssistant
	if string(role) != "assistant" {
		t.Errorf("String conversion: expected %q, got %q", "assistant", string(role))
	}

	// Test conversion from string
	var newRole Role
	newRole = Role("user")
	if newRole != RoleUser {
		t.Errorf("Role creation: expected %q, got %q", RoleUser, newRole)
	}
}

// TestStreamEventTypeConversion verifies StreamEventType conversions.
func TestStreamEventTypeConversion(t *testing.T) {
	eventType := EventToolUseStart
	if string(eventType) != "tool_use_start" {
		t.Errorf("String conversion: expected %q, got %q", "tool_use_start", string(eventType))
	}

	// Test conversion from string
	var newEventType StreamEventType
	newEventType = StreamEventType("text_delta")
	if newEventType != EventTextDelta {
		t.Errorf("EventType creation: expected %q, got %q", EventTextDelta, newEventType)
	}
}
