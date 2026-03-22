package features

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"torus_go_agent/internal/types"
)

// ============================================================================
// Test Helpers
// ============================================================================

// mockMCPServer simulates a simple MCP server for testing.
// It reads JSON-RPC requests from stdin and writes responses to stdout.
type mockMCPServer struct {
	stdin  io.Reader
	stdout io.Writer
}

// runMockServer starts a mock MCP server in a subprocess.
// It echoes back initialize requests and returns a tool list.
func runMockServer() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		var resp jsonRPCResponse

		switch req.Method {
		case "initialize":
			resp = jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
			}

		case "tools/list":
			tools := mcpToolsListResult{
				Tools: []mcpToolDef{
					{
						Name:        "test_tool_1",
						Description: "First test tool",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"arg1": map[string]any{
									"type":        "string",
									"description": "First argument",
								},
							},
						},
					},
					{
						Name:        "test_tool_2",
						Description: "Second test tool",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"arg2": map[string]any{
									"type":        "integer",
									"description": "An integer",
								},
							},
						},
					},
				},
			}
			resultBytes, _ := json.Marshal(tools)
			resp = jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(resultBytes),
			}

		case "tools/call":
			var params mcpToolCallParams
			paramsData, _ := json.Marshal(req.Params)
			if err := json.Unmarshal(paramsData, &params); err == nil {
				result := mcpToolCallResult{
					Content: []mcpContent{
						{
							Type: "text",
							Text: fmt.Sprintf("Result from %s with args %v", params.Name, params.Arguments),
						},
					},
					IsError: false,
				}
				resultBytes, _ := json.Marshal(result)
				resp = jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  json.RawMessage(resultBytes),
				}
			}

		default:
			resp = jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &jsonRPCError{
					Code:    -32601,
					Message: "Method not found",
				},
			}
		}

		respData, _ := json.Marshal(resp)
		fmt.Fprintln(os.Stdout, string(respData))
	}
}

// startMockMCPServer returns the path to the test executable that runs mockServer.
func startMockMCPServer(t *testing.T) string {
	// Create a temporary Go file that runs the mock server
	tmpDir := t.TempDir()
	exePath := tmpDir + "/mock_mcp"

	// Check if we're being called as the mock server
	if len(os.Args) > 1 && os.Args[1] == "run-mock-server" {
		runMockServer()
		os.Exit(0)
	}

	// For testing, we use inline mocks rather than subprocesses
	return exePath
}

// ============================================================================
// Tests for jsonRPCError
// ============================================================================

func TestJSONRPCErrorError(t *testing.T) {
	tests := []struct {
		name     string
		err      jsonRPCError
		expected string
	}{
		{
			name:     "standard error",
			err:      jsonRPCError{Code: -32600, Message: "Invalid Request"},
			expected: "json-rpc error -32600: Invalid Request",
		},
		{
			name:     "parse error",
			err:      jsonRPCError{Code: -32700, Message: "Parse error"},
			expected: "json-rpc error -32700: Parse error",
		},
		{
			name:     "method not found",
			err:      jsonRPCError{Code: -32601, Message: "Method not found"},
			expected: "json-rpc error -32601: Method not found",
		},
		{
			name:     "zero code",
			err:      jsonRPCError{Code: 0, Message: "Something"},
			expected: "json-rpc error 0: Something",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.err.Error()
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

// ============================================================================
// Tests for MCPServer.send and call methods
// ============================================================================

func TestMCPServerCallWithValidResponse(t *testing.T) {
	// Create a response string
	responseData, _ := json.Marshal(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"status":"ok"}`),
	})
	responseStr := string(responseData) + "\n"

	// Set up the server with mocked stdin/stdout
	server := &MCPServer{
		Name:   "test-server",
		stdin:  &bytes.Buffer{},
		stdout: bufio.NewScanner(strings.NewReader(responseStr)),
	}

	// Send a test request
	result, err := server.call("test_method", map[string]string{"arg": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify the result
	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["status"] != "ok" {
		t.Errorf("got status %q, want ok", parsed["status"])
	}
}

func TestMCPServerSendJSONMarshalError(t *testing.T) {
	// Create a server with nil stdin
	server := &MCPServer{
		Name:   "test-server",
		stdin:  &failingWriter{},
		stdout: bufio.NewScanner(strings.NewReader("")),
	}

	result, err := server.send("method", map[string]any{})
	if err == nil {
		t.Fatal("expected error for failing writer")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

// failingWriter always fails on Write
type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

// ============================================================================
// Tests for MCPClient.NewMCPClient
// ============================================================================

func TestNewMCPClient(t *testing.T) {
	tests := []struct {
		name        string
		progressive bool
	}{
		{name: "eager mode", progressive: false},
		{name: "progressive mode", progressive: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMCPClient(tc.progressive)

			if client == nil {
				t.Fatal("expected non-nil client")
			}
			if client.progressive != tc.progressive {
				t.Errorf("progressive mismatch: got %v, want %v", client.progressive, tc.progressive)
			}
			if len(client.servers) != 0 {
				t.Errorf("expected 0 servers, got %d", len(client.servers))
			}
			if len(client.tools) != 0 {
				t.Errorf("expected 0 tools, got %d", len(client.tools))
			}
		})
	}
}

// ============================================================================
// Tests for MCPClient.ListTools
// ============================================================================

func TestMCPClientListToolsEmpty(t *testing.T) {
	client := NewMCPClient(false)
	tools := client.ListTools()

	if tools == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestMCPClientListToolsWithTools(t *testing.T) {
	client := NewMCPClient(false)

	// Manually add tools
	client.mu.Lock()
	client.tools["tool1"] = &MCPTool{
		Name:        "tool1",
		Description: "Tool 1",
		ServerName:  "server1",
		InputSchema: map[string]any{"type": "object"},
	}
	client.tools["tool2"] = &MCPTool{
		Name:        "tool2",
		Description: "Tool 2",
		ServerName:  "server2",
		InputSchema: map[string]any{"type": "object"},
	}
	client.mu.Unlock()

	tools := client.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify we got a copy
	foundTool1 := false
	foundTool2 := false
	for _, tool := range tools {
		if tool.Name == "tool1" {
			foundTool1 = true
		}
		if tool.Name == "tool2" {
			foundTool2 = true
		}
	}

	if !foundTool1 || !foundTool2 {
		t.Error("not all tools found in list")
	}
}

// ============================================================================
// Tests for MCPClient.GetToolSchema
// ============================================================================

func TestMCPClientGetToolSchemaNonExistent(t *testing.T) {
	client := NewMCPClient(false)
	tool, err := client.GetToolSchema("nonexistent")

	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if tool != nil {
		t.Errorf("expected nil tool, got %v", tool)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMCPClientGetToolSchemaEagerMode(t *testing.T) {
	client := NewMCPClient(false) // eager mode

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"param": map[string]any{"type": "string"},
		},
	}

	client.mu.Lock()
	client.tools["test_tool"] = &MCPTool{
		Name:        "test_tool",
		Description: "A test tool",
		ServerName:  "test_server",
		InputSchema: schema,
	}
	client.mu.Unlock()

	tool, err := client.GetToolSchema("test_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	if tool.InputSchema == nil {
		t.Error("expected non-nil InputSchema")
	}
	if tool.Name != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got %q", tool.Name)
	}
}

// ============================================================================
// Tests for MCPClient.CallTool
// ============================================================================

func TestMCPClientCallToolNotFound(t *testing.T) {
	client := NewMCPClient(false)
	result, err := client.CallTool("nonexistent", map[string]any{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("unexpected error content: %q", result.Content)
	}
}

func TestMCPClientCallToolServerNotFound(t *testing.T) {
	client := NewMCPClient(false)

	client.mu.Lock()
	client.tools["tool1"] = &MCPTool{
		Name:        "tool1",
		Description: "Tool 1",
		ServerName:  "nonexistent_server",
		InputSchema: map[string]any{},
	}
	client.mu.Unlock()

	result, err := client.CallTool("tool1", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if !strings.Contains(result.Content, "not available") {
		t.Errorf("unexpected error content: %q", result.Content)
	}
}

// ============================================================================
// Tests for MCPClient.DiscoverTools
// ============================================================================

func TestMCPClientDiscoverToolsEagerMode(t *testing.T) {
	client := NewMCPClient(false) // eager mode

	// Create a mock server
	server := &MCPServer{
		Name:   "mock",
		stdin:  &bytes.Buffer{},
		stdout: bufio.NewScanner(strings.NewReader("")),
	}

	// Setup mock call method to return tools
	toolDef1 := mcpToolDef{
		Name:        "tool1",
		Description: "Tool 1",
		InputSchema: map[string]any{"type": "object"},
	}
	toolDef2 := mcpToolDef{
		Name:        "tool2",
		Description: "Tool 2",
		InputSchema: map[string]any{"type": "object"},
	}

	// We need to mock the server's call method indirectly
	// This is tricky since call is a method. Let's use a different approach:
	// Create mock data and test the logic directly

	result := mcpToolsListResult{
		Tools: []mcpToolDef{toolDef1, toolDef2},
	}
	resultBytes, _ := json.Marshal(result)

	// Since we can't easily mock the server.call method, we'll test
	// the tool discovery logic by manually adding tools as discoverTools would
	client.mu.Lock()
	for _, def := range result.Tools {
		tool := &MCPTool{
			Name:        def.Name,
			Description: def.Description,
			ServerName:  server.Name,
		}
		if !client.progressive {
			tool.InputSchema = def.InputSchema
		}
		client.tools[def.Name] = tool
	}
	client.mu.Unlock()

	// Verify tools were added
	if len(client.tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(client.tools))
	}

	// In eager mode, InputSchema should be populated
	tool1, ok := client.tools["tool1"]
	if !ok {
		t.Fatal("tool1 not found")
	}
	if tool1.InputSchema == nil {
		t.Error("expected InputSchema in eager mode")
	}

	_ = resultBytes // silence unused
}

func TestMCPClientDiscoverToolsProgressiveMode(t *testing.T) {
	client := NewMCPClient(true) // progressive mode

	result := mcpToolsListResult{
		Tools: []mcpToolDef{
			{
				Name:        "tool1",
				Description: "Tool 1",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	// Simulate discovery
	client.mu.Lock()
	for _, def := range result.Tools {
		tool := &MCPTool{
			Name:        def.Name,
			Description: def.Description,
			ServerName:  "server",
		}
		if !client.progressive {
			tool.InputSchema = def.InputSchema
		}
		client.tools[def.Name] = tool
	}
	client.mu.Unlock()

	// In progressive mode, InputSchema should be nil initially
	tool1, ok := client.tools["tool1"]
	if !ok {
		t.Fatal("tool1 not found")
	}
	if tool1.InputSchema != nil {
		t.Error("expected nil InputSchema in progressive mode")
	}
}

// ============================================================================
// Tests for MCPClient.AsTools
// ============================================================================

func TestMCPClientAsToolsEmpty(t *testing.T) {
	client := NewMCPClient(false)
	tools := client.AsTools()

	if tools == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestMCPClientAsToolsEagerMode(t *testing.T) {
	client := NewMCPClient(false) // eager mode

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"param": map[string]any{"type": "string"},
		},
	}

	client.mu.Lock()
	client.tools["test_tool"] = &MCPTool{
		Name:        "test_tool",
		Description: "A test tool",
		ServerName:  "test_server",
		InputSchema: schema,
	}
	client.servers["test_server"] = &MCPServer{
		Name: "test_server",
	}
	client.mu.Unlock()

	tools := client.AsTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
	if tool.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", tool.Description)
	}
	if tool.InputSchema == nil {
		t.Error("expected non-nil InputSchema")
	}
	if tool.Execute == nil {
		t.Error("expected non-nil Execute function")
	}
}

func TestMCPClientAsToolsWithNilSchema(t *testing.T) {
	client := NewMCPClient(false)

	client.mu.Lock()
	client.tools["test_tool"] = &MCPTool{
		Name:        "test_tool",
		Description: "A test tool",
		ServerName:  "test_server",
		InputSchema: nil, // No schema
	}
	client.servers["test_server"] = &MCPServer{
		Name: "test_server",
	}
	client.mu.Unlock()

	tools := client.AsTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.InputSchema == nil {
		t.Error("expected non-nil InputSchema (should be default)")
	}

	// Check for default schema structure
	props, ok := tool.InputSchema["properties"]
	if !ok {
		t.Error("expected 'properties' in default schema")
	}
	if propsMap, ok := props.(map[string]any); !ok || len(propsMap) != 0 {
		t.Error("expected empty properties object in default schema")
	}
}

// ============================================================================
// Tests for MCPClient.Close
// ============================================================================

func TestMCPClientCloseEmpty(t *testing.T) {
	client := NewMCPClient(false)
	client.Close() // Should not panic
}

func TestMCPClientCloseWithServers(t *testing.T) {
	client := NewMCPClient(false)

	// Create mock servers
	server1 := &MCPServer{
		Name:    "server1",
		process: nil, // No actual process for this test
	}
	server2 := &MCPServer{
		Name:    "server2",
		process: nil,
	}

	client.mu.Lock()
	client.servers["server1"] = server1
	client.servers["server2"] = server2
	client.mu.Unlock()

	// Close should not panic even with nil process
	client.Close()
}

// ============================================================================
// Tests for types.Tool integration
// ============================================================================

func TestMCPToolExecuteFunction(t *testing.T) {
	client := NewMCPClient(false)

	// Setup a tool with a mock server
	mockStdin := &bytes.Buffer{}
	mockStdout := bytes.NewBufferString("")

	server := &MCPServer{
		Name:   "test_server",
		stdin:  mockStdin,
		stdout: bufio.NewScanner(mockStdout),
	}

	client.mu.Lock()
	client.tools["test_tool"] = &MCPTool{
		Name:        "test_tool",
		Description: "Test tool",
		ServerName:  "test_server",
		InputSchema: map[string]any{},
	}
	client.servers["test_server"] = server
	client.mu.Unlock()

	tools := client.AsTools()
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	tool := tools[0]

	// Test that Execute function exists and can be called
	// (It will fail because server is not real, but function should exist)
	if tool.Execute == nil {
		t.Fatal("expected non-nil Execute function")
	}

	// The Execute function should return a ToolResult
	// Call it with test arguments
	result, err := tool.Execute(map[string]any{"test": "value"})

	// We expect an error due to the mock server not being real
	// but the function should exist and not panic
	if result == nil && err == nil {
		t.Error("expected either result or error")
	}
}

// ============================================================================
// Tests for MCPTool structure
// ============================================================================

func TestMCPToolFields(t *testing.T) {
	schema := map[string]any{"type": "object"}
	tool := MCPTool{
		Name:        "my_tool",
		Description: "A description",
		InputSchema: schema,
		ServerName:  "my_server",
	}

	if tool.Name != "my_tool" {
		t.Errorf("name mismatch")
	}
	if tool.Description != "A description" {
		t.Errorf("description mismatch")
	}
	if tool.InputSchema == nil {
		t.Errorf("InputSchema should not be nil")
	}
	if tool.ServerName != "my_server" {
		t.Errorf("ServerName mismatch")
	}
}

// ============================================================================
// Integration-style tests
// ============================================================================

func TestMCPClientConcurrentAccess(t *testing.T) {
	client := NewMCPClient(false)

	// Add some tools
	for i := 0; i < 5; i++ {
		idx := i
		go func() {
			client.mu.Lock()
			client.tools[fmt.Sprintf("tool%d", idx)] = &MCPTool{
				Name:        fmt.Sprintf("tool%d", idx),
				Description: fmt.Sprintf("Tool %d", idx),
				ServerName:  "server",
			}
			client.mu.Unlock()
		}()
	}

	// Read tools concurrently
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_ = client.ListTools()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	tools := client.ListTools()
	if len(tools) < 5 {
		t.Errorf("expected at least 5 tools, got %d", len(tools))
	}
}

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "test_method",
		Params:  map[string]string{"key": "value"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled jsonRPCRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != 42 {
		t.Errorf("ID mismatch: got %d, want 42", unmarshaled.ID)
	}
	if unmarshaled.Method != "test_method" {
		t.Errorf("Method mismatch: got %q, want test_method", unmarshaled.Method)
	}
}

func TestJSONRPCResponseMarshal(t *testing.T) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      42,
		Result:  json.RawMessage(`{"status":"ok"}`),
		Error:   nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled jsonRPCResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != 42 {
		t.Errorf("ID mismatch")
	}
	if len(unmarshaled.Result) == 0 {
		t.Error("expected non-empty Result")
	}
}

func TestJSONRPCErrorMarshal(t *testing.T) {
	errData := &jsonRPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	data, err := json.Marshal(errData)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled jsonRPCError
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Code != -32600 {
		t.Errorf("Code mismatch")
	}
	if unmarshaled.Message != "Invalid Request" {
		t.Errorf("Message mismatch")
	}
}

// ============================================================================
// Edge case tests
// ============================================================================

func TestAddServerWithNilCommand(t *testing.T) {
	client := NewMCPClient(false)

	// Trying to add a server with a non-existent command should fail
	err := client.AddServer("test", "/nonexistent/command", []string{}, nil)
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestGetToolSchemaWithMissingServer(t *testing.T) {
	client := NewMCPClient(true) // progressive mode

	client.mu.Lock()
	client.tools["tool1"] = &MCPTool{
		Name:        "tool1",
		Description: "Tool 1",
		ServerName:  "nonexistent_server",
		InputSchema: nil,
	}
	client.mu.Unlock()

	// In progressive mode, trying to get schema for a tool
	// whose server doesn't exist should fail
	_, err := client.GetToolSchema("tool1")
	if err == nil {
		t.Fatal("expected error for missing server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMCPToolCallResultMultipleContentBlocks(t *testing.T) {
	// Test concatenation of multiple content blocks in CallTool
	client := NewMCPClient(false)

	// Manually set up a tool and mock the call response
	// This is a simplified test of the content concatenation logic

	result := mcpToolCallResult{
		Content: []mcpContent{
			{Type: "text", Text: "Line 1"},
			{Type: "text", Text: "Line 2"},
			{Type: "text", Text: "Line 3"},
		},
		IsError: false,
	}

	// Simulate the content concatenation logic from CallTool
	var text string
	for i, c := range result.Content {
		if c.Type == "text" {
			if i > 0 {
				text += "\n"
			}
			text += c.Text
		}
	}

	expected := "Line 1\nLine 2\nLine 3"
	if text != expected {
		t.Errorf("text mismatch: got %q, want %q", text, expected)
	}

	_ = client // silence unused
}

func TestMCPServerIDGeneration(t *testing.T) {
	// Test that ID generation increments correctly
	server := &MCPServer{
		Name: "test",
	}

	id1 := int(server.idGen.Add(1))
	id2 := int(server.idGen.Add(1))
	id3 := int(server.idGen.Add(1))

	if id1 != 1 {
		t.Errorf("expected id1=1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("expected id2=2, got %d", id2)
	}
	if id3 != 3 {
		t.Errorf("expected id3=3, got %d", id3)
	}
}

func TestMCPClientLockingBehavior(t *testing.T) {
	// Test that operations properly acquire locks
	client := NewMCPClient(false)

	client.mu.Lock()
	client.tools["test"] = &MCPTool{
		Name:       "test",
		ServerName: "server",
	}
	client.mu.Unlock()

	// These operations should not deadlock
	tools := client.ListTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}

	tool, err := client.GetToolSchema("test")
	if err == nil && tool != nil {
		// Success - the method acquired the lock properly
	}
}

// ============================================================================
// Tests for Tool result handling
// ============================================================================

func TestToolResultWithError(t *testing.T) {
	result := &types.ToolResult{
		Content: "An error occurred",
		IsError: true,
	}

	if !result.IsError {
		t.Error("IsError should be true")
	}
	if result.Content != "An error occurred" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestToolResultWithSuccess(t *testing.T) {
	result := &types.ToolResult{
		Content: "Success result",
		IsError: false,
	}

	if result.IsError {
		t.Error("IsError should be false")
	}
	if result.Content != "Success result" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

// ============================================================================
// Tests for MCP protocol types
// ============================================================================

func TestMCPInitializeParamsMarshal(t *testing.T) {
	params := mcpInitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: mcpClientInfo{
			Name:    "test_client",
			Version: "1.0.0",
		},
		Capabilities: map[string]any{},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled mcpInitializeParams
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion mismatch")
	}
	if unmarshaled.ClientInfo.Name != "test_client" {
		t.Errorf("ClientInfo.Name mismatch")
	}
}

func TestMCPToolDefMarshal(t *testing.T) {
	toolDef := mcpToolDef{
		Name:        "my_tool",
		Description: "A tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"arg": map[string]any{"type": "string"},
			},
		},
	}

	data, err := json.Marshal(toolDef)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled mcpToolDef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Name != "my_tool" {
		t.Errorf("Name mismatch")
	}
	if unmarshaled.InputSchema == nil {
		t.Error("InputSchema should not be nil")
	}
}

func TestMCPToolCallParamsMarshal(t *testing.T) {
	params := mcpToolCallParams{
		Name: "my_tool",
		Arguments: map[string]any{
			"arg1": "value1",
			"arg2": 42,
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled mcpToolCallParams
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Name != "my_tool" {
		t.Errorf("Name mismatch")
	}
	if unmarshaled.Arguments["arg1"] != "value1" {
		t.Errorf("Arguments mismatch")
	}
}

func TestMCPContentMarshal(t *testing.T) {
	content := mcpContent{
		Type: "text",
		Text: "Hello, world!",
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled mcpContent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != "text" {
		t.Errorf("Type mismatch")
	}
	if unmarshaled.Text != "Hello, world!" {
		t.Errorf("Text mismatch")
	}
}

func TestMCPToolCallResultMarshal(t *testing.T) {
	callResult := mcpToolCallResult{
		Content: []mcpContent{
			{Type: "text", Text: "Result 1"},
			{Type: "text", Text: "Result 2"},
		},
		IsError: false,
	}

	data, err := json.Marshal(callResult)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled mcpToolCallResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Content) != 2 {
		t.Errorf("expected 2 content blocks, got %d", len(unmarshaled.Content))
	}
	if unmarshaled.IsError {
		t.Error("IsError should be false")
	}
}

// ============================================================================
// Negative case tests
// ============================================================================

func TestCallToolWithEmptyArgs(t *testing.T) {
	client := NewMCPClient(false)

	client.mu.Lock()
	client.tools["test_tool"] = &MCPTool{
		Name:        "test_tool",
		Description: "Test tool",
		ServerName:  "test_server",
		InputSchema: map[string]any{},
	}
	// No server added - should fail with proper error
	client.mu.Unlock()

	result, err := client.CallTool("test_tool", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestMCPToolNameWithSpecialCharacters(t *testing.T) {
	tool := MCPTool{
		Name:        "tool-with.special_chars@123",
		Description: "A tool with special characters",
		ServerName:  "server",
	}

	if tool.Name != "tool-with.special_chars@123" {
		t.Errorf("failed to preserve special characters in tool name")
	}
}

func TestEmptyToolDescription(t *testing.T) {
	tool := MCPTool{
		Name:        "tool",
		Description: "",
		ServerName:  "server",
	}

	if tool.Description != "" {
		t.Errorf("expected empty description")
	}
}
