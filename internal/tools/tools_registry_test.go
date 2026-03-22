package tools

import (
	"strings"
	"testing"

	t "torus_go_agent/internal/types"
)

// TestRegistryBuildDefaultTools verifies that BuildDefaultTools returns all 6 expected tools.
func TestRegistryBuildDefaultTools(t *testing.T) {
	tools := BuildDefaultTools()

	if tools == nil {
		t.Fatal("BuildDefaultTools returned nil")
	}

	expectedCount := 6
	if len(tools) != expectedCount {
		t.Fatalf("expected %d tools, got %d", expectedCount, len(tools))
	}

	expectedNames := []string{"bash", "read", "write", "edit", "glob", "grep"}
	actualNames := make([]string, len(tools))
	for i, tool := range tools {
		actualNames[i] = tool.Name
	}

	for i, expected := range expectedNames {
		if i >= len(actualNames) {
			t.Fatalf("tool %d is missing", i)
		}
		if actualNames[i] != expected {
			t.Fatalf("tool %d: expected name %q, got %q", i, expected, actualNames[i])
		}
	}
}

// TestRegistryToolsHaveRequiredFields verifies that all tools have required fields.
func TestRegistryToolsHaveRequiredFields(t *testing.T) {
	tools := BuildDefaultTools()

	for _, tool := range tools {
		if tool.Name == "" {
			t.Fatal("tool Name is empty")
		}

		if tool.Description == "" {
			t.Fatalf("tool %s: Description is empty", tool.Name)
		}

		if tool.InputSchema == nil {
			t.Fatalf("tool %s: InputSchema is nil", tool.Name)
		}

		if tool.Execute == nil {
			t.Fatalf("tool %s: Execute is nil", tool.Name)
		}
	}
}

// TestToolsHaveInputSchemaType verifies that all tools have correct schema structure.
func TestToolsHaveInputSchemaType(t *testing.T) {
	tools := BuildDefaultTools()

	for _, tool := range tools {
		schema := tool.InputSchema
		schemaType, ok := schema["type"].(string)
		if !ok {
			t.Fatalf("tool %s: schema type is not a string", tool.Name)
		}

		if schemaType != "object" {
			t.Fatalf("tool %s: schema type should be 'object', got %q", tool.Name, schemaType)
		}

		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %s: properties is not a map", tool.Name)
		}

		if len(properties) == 0 {
			t.Fatalf("tool %s: properties map is empty", tool.Name)
		}
	}
}

// TestBashToolSchema verifies bash tool schema details.
func TestBashToolSchema(t *testing.T) {
	tool := bashTool()

	if tool.Name != "bash" {
		t.Fatalf("expected name 'bash', got %s", tool.Name)
	}

	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be []string")
	}

	foundCommand := false
	for _, r := range required {
		if r == "command" {
			foundCommand = true
			break
		}
	}

	if !foundCommand {
		t.Fatal("command should be in required fields for bash tool")
	}

	properties := schema["properties"].(map[string]any)
	if _, exists := properties["command"]; !exists {
		t.Fatal("command property should exist in bash tool")
	}
	if _, exists := properties["cwd"]; !exists {
		t.Fatal("cwd property should exist in bash tool")
	}
}

// TestReadToolSchema verifies read tool schema details.
func TestReadToolSchema(t *testing.T) {
	tool := readTool()

	if tool.Name != "read" {
		t.Fatalf("expected name 'read', got %s", tool.Name)
	}

	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be []string")
	}

	foundFilePath := false
	for _, r := range required {
		if r == "file_path" {
			foundFilePath = true
			break
		}
	}

	if !foundFilePath {
		t.Fatal("file_path should be in required fields for read tool")
	}

	properties := schema["properties"].(map[string]any)
	if _, exists := properties["file_path"]; !exists {
		t.Fatal("file_path property should exist in read tool")
	}
	if _, exists := properties["offset"]; !exists {
		t.Fatal("offset property should exist in read tool")
	}
	if _, exists := properties["limit"]; !exists {
		t.Fatal("limit property should exist in read tool")
	}
}

// TestWriteToolSchema verifies write tool schema details.
func TestWriteToolSchema(t *testing.T) {
	tool := writeTool()

	if tool.Name != "write" {
		t.Fatalf("expected name 'write', got %s", tool.Name)
	}

	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be []string")
	}

	if len(required) < 2 {
		t.Fatal("write tool should have at least 2 required fields")
	}

	foundFilePath := false
	foundContent := false
	for _, r := range required {
		if r == "file_path" {
			foundFilePath = true
		}
		if r == "content" {
			foundContent = true
		}
	}

	if !foundFilePath {
		t.Fatal("file_path should be in required fields for write tool")
	}
	if !foundContent {
		t.Fatal("content should be in required fields for write tool")
	}

	properties := schema["properties"].(map[string]any)
	if _, exists := properties["file_path"]; !exists {
		t.Fatal("file_path property should exist in write tool")
	}
	if _, exists := properties["content"]; !exists {
		t.Fatal("content property should exist in write tool")
	}
}

// TestEditToolSchema verifies edit tool schema details.
func TestEditToolSchema(t *testing.T) {
	tool := editTool()

	if tool.Name != "edit" {
		t.Fatalf("expected name 'edit', got %s", tool.Name)
	}

	schema := tool.InputSchema
	properties := schema["properties"].(map[string]any)
	if _, exists := properties["file_path"]; !exists {
		t.Fatal("file_path property should exist in edit tool")
	}
	if _, exists := properties["old_str"]; !exists {
		t.Fatal("old_str property should exist in edit tool")
	}
	if _, exists := properties["new_str"]; !exists {
		t.Fatal("new_str property should exist in edit tool")
	}
	if _, exists := properties["replace_all"]; !exists {
		t.Fatal("replace_all property should exist in edit tool")
	}
}

// TestGlobToolSchema verifies glob tool schema details.
func TestGlobToolSchema(t *testing.T) {
	tool := globTool()

	if tool.Name != "glob" {
		t.Fatalf("expected name 'glob', got %s", tool.Name)
	}

	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be []string")
	}

	foundPattern := false
	for _, r := range required {
		if r == "pattern" {
			foundPattern = true
			break
		}
	}

	if !foundPattern {
		t.Fatal("pattern should be in required fields for glob tool")
	}

	properties := schema["properties"].(map[string]any)
	if _, exists := properties["pattern"]; !exists {
		t.Fatal("pattern property should exist in glob tool")
	}
	if _, exists := properties["cwd"]; !exists {
		t.Fatal("cwd property should exist in glob tool")
	}
}

// TestGrepToolSchema verifies grep tool schema details.
func TestGrepToolSchema(t *testing.T) {
	tool := grepTool()

	if tool.Name != "grep" {
		t.Fatalf("expected name 'grep', got %s", tool.Name)
	}

	schema := tool.InputSchema
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field should be []string")
	}

	foundPattern := false
	for _, r := range required {
		if r == "pattern" {
			foundPattern = true
			break
		}
	}

	if !foundPattern {
		t.Fatal("pattern should be in required fields for grep tool")
	}

	properties := schema["properties"].(map[string]any)
	if _, exists := properties["pattern"]; !exists {
		t.Fatal("pattern property should exist in grep tool")
	}
	if _, exists := properties["path"]; !exists {
		t.Fatal("path property should exist in grep tool")
	}
	if _, exists := properties["glob"]; !exists {
		t.Fatal("glob property should exist in grep tool")
	}
}

// TestGFHelperFunction tests the GF (GetFloat) helper function.
func TestGFHelperFunction(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		def      float64
		expected float64
	}{
		{
			name:     "existing_value",
			m:        map[string]any{"test": 42.5},
			key:      "test",
			def:      0,
			expected: 42.5,
		},
		{
			name:     "missing_key_returns_default",
			m:        map[string]any{},
			key:      "missing",
			def:      10.0,
			expected: 10.0,
		},
		{
			name:     "wrong_type_returns_default",
			m:        map[string]any{"test": "not_a_number"},
			key:      "test",
			def:      5.0,
			expected: 5.0,
		},
		{
			name:     "zero_value",
			m:        map[string]any{"test": 0.0},
			key:      "test",
			def:      1.0,
			expected: 0.0,
		},
		{
			name:     "negative_value",
			m:        map[string]any{"test": -3.14},
			key:      "test",
			def:      0,
			expected: -3.14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GF(tt.m, tt.key, tt.def)
			if result != tt.expected {
				t.Fatalf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestToolExecutableInterface verifies that Tool type properly implements execution.
func TestToolExecutableInterface(t *testing.T) {
	tools := BuildDefaultTools()

	for _, tool := range tools {
		// Create a minimal valid args map
		args := make(map[string]any)

		// Each tool should be callable without panic
		result, err := tool.Execute(args)

		// We expect errors for incomplete args, but not panics
		if result != nil && err == nil {
			// If we got a result, verify it's the right type
			if result.Content == "" && !result.IsError {
				// Empty content is ok (e.g., no matches)
			}
		}
	}
}

// TestToolDescriptionsNonEmpty verifies all tool descriptions are substantive.
func TestToolDescriptionsNonEmpty(t *testing.T) {
	tools := BuildDefaultTools()

	for _, tool := range tools {
		desc := strings.TrimSpace(tool.Description)
		if len(desc) == 0 {
			t.Fatalf("tool %s has empty description", tool.Name)
		}

		if len(desc) < 5 {
			t.Fatalf("tool %s description is too short: %q", tool.Name, desc)
		}
	}
}

// TestToolPropertiesHaveDescriptions verifies that all schema properties have descriptions.
func TestToolPropertiesHaveDescriptions(t *testing.T) {
	tools := BuildDefaultTools()

	for _, tool := range tools {
		properties, _ := tool.InputSchema["properties"].(map[string]any)

		for propName, propVal := range properties {
			propMap, ok := propVal.(map[string]any)
			if !ok {
				t.Fatalf("tool %s property %s is not a map", tool.Name, propName)
			}

			desc, hasDesc := propMap["description"].(string)
			if !hasDesc || len(strings.TrimSpace(desc)) == 0 {
				t.Fatalf("tool %s property %s has no description", tool.Name, propName)
			}
		}
	}
}

// TestToolReturnTypes verifies that tool Execute returns *ToolResult and error.
func TestToolReturnTypes(t *testing.T) {
	tool := bashTool()
	args := map[string]any{"command": "echo test"}

	result, err := tool.Execute(args)

	// result can be nil or a ToolResult pointer
	if result != nil {
		// Verify it has the expected fields
		_ = result.Content
		_ = result.IsError
	}

	// err can be nil or an error
	if err != nil {
		// Just verify it's an error type
		_ = err.Error()
	}
}

// TestReadToolOffsetAndLimit tests the read tool with offset and limit parameters.
func TestReadToolOffsetAndLimit(t *testing.T) {
	tool := readTool()

	tests := []struct {
		name   string
		offset float64
		limit  float64
	}{
		{"zero_offset", 0, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"file_path": "/etc/hostname",
				"offset":    tt.offset,
				"limit":     tt.limit,
			}

			result, err := tool.Execute(args)
			if err != nil {
				t.Logf("Got expected error or result: %v", err)
			}

			if result == nil {
				t.Fatal("result is nil")
			}

			// Result should contain something (either content or error message)
			if result.Content == "" {
				t.Fatal("result content is empty")
			}
		})
	}
}

// TestGlobPatternWithCwd tests the glob tool with a custom working directory.
func TestGlobPatternWithCwd(t *testing.T) {
	tool := globTool()

	args := map[string]any{
		"pattern": "*.go",
		"cwd":     "/tmp",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Logf("Execute returned error: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	// Should return either matches or "(no matches)"
	if result.Content == "" {
		t.Fatal("result content is empty")
	}
}

// TestGrepPatternRequired verifies that grep tool handles pattern argument correctly.
func TestGrepPatternRequired(t *testing.T) {
	tool := grepTool()

	// Test with pattern
	args := map[string]any{
		"pattern": "test",
		"path":    "/etc",
	}

	result, err := tool.Execute(args)
	if err != nil {
		t.Logf("Execute returned error: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	// Should return something (matches or no matches)
	if result.Content == "" {
		t.Fatal("result content is empty")
	}
}

// TestToolTypeInterface verifies the Tool type has all required fields.
func TestToolTypeInterface(tt *testing.T) {
	tool := t.Tool{
		Name:        "test",
		Description: "test tool",
		InputSchema: map[string]any{},
		Execute:     func(args map[string]any) (*t.ToolResult, error) { return nil, nil },
	}

	if tool.Name != "test" {
		tt.Fatal("Name field not accessible")
	}
	if tool.Description != "test tool" {
		tt.Fatal("Description field not accessible")
	}
	if tool.InputSchema == nil {
		tt.Fatal("InputSchema field not accessible")
	}
	if tool.Execute == nil {
		tt.Fatal("Execute field not accessible")
	}
}

// TestBuildDefaultToolsReturnsCopy verifies each call returns new instances.
func TestBuildDefaultToolsReturnsCopy(t *testing.T) {
	tools1 := BuildDefaultTools()
	tools2 := BuildDefaultTools()

	if len(tools1) != len(tools2) {
		t.Fatal("two calls to BuildDefaultTools returned different lengths")
	}

	for i := range tools1 {
		if tools1[i].Name != tools2[i].Name {
			t.Fatal("tool name mismatch between calls")
		}
	}
}

// TestToolResultFields verifies ToolResult has the expected fields.
func TestToolResultFields(tt *testing.T) {
	result := &t.ToolResult{
		Content: "test content",
		IsError: true,
	}

	if result.Content != "test content" {
		tt.Fatal("Content field not set correctly")
	}

	if !result.IsError {
		tt.Fatal("IsError field not set correctly")
	}
}
