// Package features provides optional capabilities that can be added to an agent.
// mcp.go implements an MCP (Model Context Protocol) client that connects to
// external MCP servers via stdio transport: spawn subprocess, communicate via
// JSON-RPC over stdin/stdout.
package features

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"torus_go_agent/internal/types"
)

// ---------------------------------------------------------------------------
// JSON-RPC wire types
// ---------------------------------------------------------------------------

// jsonRPCRequest is a JSON-RPC 2.0 request message.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response message.
type jsonRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int              `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message)
}

// ---------------------------------------------------------------------------
// MCP protocol helper types (subset used for initialize / tools/list / tools/call)
// ---------------------------------------------------------------------------

type mcpInitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ClientInfo      mcpClientInfo  `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpToolsListResult struct {
	Tools []mcpToolDef `json:"tools"`
}

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ---------------------------------------------------------------------------
// MCPServer — one live subprocess
// ---------------------------------------------------------------------------

// MCPServer represents a connection to a single MCP server subprocess.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string

	process *exec.Cmd
	stdin   io.Writer
	stdout  *bufio.Scanner
	mu      sync.Mutex
	idGen   atomic.Int64
}

// send writes a JSON-RPC request and reads back the response.
// Caller must hold s.mu.
func (s *MCPServer) send(method string, params any) (*jsonRPCResponse, error) {
	id := int(s.idGen.Add(1))

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err = s.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write to server %q: %w", s.Name, err)
	}

	// Read lines until we get the response for this ID.
	// (Some servers emit notifications before the response.)
	for s.stdout.Scan() {
		line := s.stdout.Text()
		if line == "" {
			continue
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			// Not a valid JSON-RPC response — could be a log line; skip.
			continue
		}

		// Responses without an ID are notifications; skip them.
		if resp.ID == 0 && resp.Error == nil && resp.Result == nil {
			continue
		}

		if resp.ID != id {
			// Response for a different pending request; skip.
			// (In single-threaded stdio this shouldn't normally happen.)
			continue
		}

		return &resp, nil
	}

	if err := s.stdout.Err(); err != nil {
		return nil, fmt.Errorf("read from server %q: %w", s.Name, err)
	}
	return nil, fmt.Errorf("server %q closed stdout before responding", s.Name)
}

// call is the public wrapper: acquires the mutex, calls send, returns result bytes.
func (s *MCPServer) call(method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp, err := s.send(method, params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// close terminates the subprocess.
func (s *MCPServer) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil {
		_ = s.process.Process.Kill()
		_ = s.process.Wait()
	}
}

// ---------------------------------------------------------------------------
// MCPTool — a tool discovered from an MCP server
// ---------------------------------------------------------------------------

// MCPTool is a tool discovered from an MCP server.
type MCPTool struct {
	Name        string
	Description string
	// InputSchema is nil when progressive=true until GetToolSchema is called.
	InputSchema map[string]any
	ServerName  string
}

// ---------------------------------------------------------------------------
// MCPClient — manages multiple MCP servers
// ---------------------------------------------------------------------------

// MCPClient manages connections to one or more MCP servers and exposes their
// tools in the types.Tool format that the agent loop expects.
type MCPClient struct {
	mu          sync.RWMutex
	servers     map[string]*MCPServer
	tools       map[string]*MCPTool
	progressive bool // if true, InputSchema is loaded lazily
}

// NewMCPClient creates a new MCP client.
//
// progressive: when true, tools/list only stores names+descriptions; the full
// InputSchema is fetched lazily via GetToolSchema on first use. This saves
// ~7-9% context by not loading all schemas upfront.
func NewMCPClient(progressive bool) *MCPClient {
	return &MCPClient{
		servers:     make(map[string]*MCPServer),
		tools:       make(map[string]*MCPTool),
		progressive: progressive,
	}
}

// AddServer spawns the subprocess, performs the MCP initialize handshake, then
// calls tools/list to discover available tools.
func (c *MCPClient) AddServer(name, command string, args []string, env map[string]string) error {
	cmd := exec.Command(command, args...)

	// Apply extra environment variables.
	if len(env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe for %q: %w", name, err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe for %q: %w", name, err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start server %q (%s): %w", name, command, err)
	}

	srv := &MCPServer{
		Name:    name,
		Command: command,
		Args:    args,
		Env:     env,
		process: cmd,
		stdin:   stdinPipe,
		stdout:  bufio.NewScanner(stdoutPipe),
	}

	// Increase scanner buffer for large tool schemas.
	srv.stdout.Buffer(make([]byte, 1024*1024), 1024*1024)

	// --- MCP initialize handshake ---
	initParams := mcpInitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: mcpClientInfo{
			Name:    "torus_go_agent",
			Version: "1.0.0",
		},
		Capabilities: map[string]any{},
	}

	_, err = srv.call("initialize", initParams)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("initialize server %q: %w", name, err)
	}

	// Send initialized notification (no response expected).
	srv.mu.Lock()
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	notifData, _ := json.Marshal(notif)
	notifData = append(notifData, '\n')
	_, _ = srv.stdin.Write(notifData)
	srv.mu.Unlock()

	// --- Discover tools ---
	if err := c.discoverTools(srv); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("tools/list from server %q: %w", name, err)
	}

	c.mu.Lock()
	c.servers[name] = srv
	c.mu.Unlock()

	return nil
}

// discoverTools calls tools/list and stores the results.
func (c *MCPClient) discoverTools(srv *MCPServer) error {
	raw, err := srv.call("tools/list", nil)
	if err != nil {
		return err
	}

	var result mcpToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse tools/list response: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, def := range result.Tools {
		tool := &MCPTool{
			Name:        def.Name,
			Description: def.Description,
			ServerName:  srv.Name,
		}
		if !c.progressive {
			// Eager: store full schema immediately.
			tool.InputSchema = def.InputSchema
		}
		// In progressive mode InputSchema remains nil until GetToolSchema.
		c.tools[def.Name] = tool
	}

	return nil
}

// ListTools returns all discovered tools.
//
// When progressive=true, the returned MCPTool entries have InputSchema=nil;
// call GetToolSchema to fetch the schema on demand.
func (c *MCPClient) ListTools() []MCPTool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]MCPTool, 0, len(c.tools))
	for _, t := range c.tools {
		out = append(out, *t)
	}
	return out
}

// GetToolSchema returns the full MCPTool including InputSchema.
//
// In progressive mode the schema is fetched from the server the first time
// this is called (and cached for subsequent calls). In eager mode it is
// returned directly from cache.
func (c *MCPClient) GetToolSchema(name string) (*MCPTool, error) {
	c.mu.RLock()
	tool, ok := c.tools[name]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}

	// Already have schema.
	if tool.InputSchema != nil {
		return tool, nil
	}

	// Progressive: fetch schema via tools/list filtered to this tool.
	// MCP spec doesn't define a per-tool schema endpoint, so we re-call
	// tools/list and extract the matching entry.
	c.mu.RLock()
	srv, ok := c.servers[tool.ServerName]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q for tool %q not found", tool.ServerName, name)
	}

	raw, err := srv.call("tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list to fetch schema for %q: %w", name, err)
	}

	var result mcpToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list for schema: %w", err)
	}

	for _, def := range result.Tools {
		if def.Name == name {
			c.mu.Lock()
			tool.InputSchema = def.InputSchema
			c.mu.Unlock()
			return tool, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found in tools/list response", name)
}

// CallTool sends a tools/call JSON-RPC request to the appropriate server and
// returns the result as a *types.ToolResult.
func (c *MCPClient) CallTool(name string, args map[string]any) (*types.ToolResult, error) {
	c.mu.RLock()
	tool, ok := c.tools[name]
	c.mu.RUnlock()
	if !ok {
		return &types.ToolResult{
			Content: fmt.Sprintf("MCP tool %q not found", name),
			IsError: true,
		}, nil
	}

	c.mu.RLock()
	srv, ok := c.servers[tool.ServerName]
	c.mu.RUnlock()
	if !ok {
		return &types.ToolResult{
			Content: fmt.Sprintf("MCP server %q not available", tool.ServerName),
			IsError: true,
		}, nil
	}

	raw, err := srv.call("tools/call", mcpToolCallParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return &types.ToolResult{
			Content: fmt.Sprintf("MCP call error: %s", err.Error()),
			IsError: true,
		}, nil
	}

	var result mcpToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return &types.ToolResult{
			Content: fmt.Sprintf("MCP parse error: %s", err.Error()),
			IsError: true,
		}, nil
	}

	// Concatenate all text content blocks.
	var text string
	for i, c := range result.Content {
		if c.Type == "text" {
			if i > 0 {
				text += "\n"
			}
			text += c.Text
		}
	}

	return &types.ToolResult{
		Content: text,
		IsError: result.IsError,
	}, nil
}

// AsTools converts all MCP tools to types.Tool format so they can be registered
// directly in AgentConfig.Tools. The Execute function calls CallTool.
//
// In progressive mode, GetToolSchema is called per tool to ensure InputSchema
// is populated before constructing the types.Tool (the agent needs the schema
// to describe the tool to the LLM).
func (c *MCPClient) AsTools() []types.Tool {
	c.mu.RLock()
	names := make([]string, 0, len(c.tools))
	for n := range c.tools {
		names = append(names, n)
	}
	c.mu.RUnlock()

	out := make([]types.Tool, 0, len(names))
	for _, name := range names {
		toolName := name // capture for closure

		var schema map[string]any
		if c.progressive {
			// Fetch (and cache) the full schema now, before building the tool.
			full, err := c.GetToolSchema(toolName)
			if err == nil && full != nil {
				schema = full.InputSchema
			}
		} else {
			c.mu.RLock()
			if t, ok := c.tools[toolName]; ok {
				schema = t.InputSchema
			}
			c.mu.RUnlock()
		}

		if schema == nil {
			schema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}

		c.mu.RLock()
		desc := c.tools[toolName].Description
		c.mu.RUnlock()

		out = append(out, types.Tool{
			Name:        toolName,
			Description: desc,
			InputSchema: schema,
			Execute: func(args map[string]any) (*types.ToolResult, error) {
				return c.CallTool(toolName, args)
			},
		})
	}
	return out
}

// Close terminates all managed subprocesses.
func (c *MCPClient) Close() {
	c.mu.RLock()
	srvs := make([]*MCPServer, 0, len(c.servers))
	for _, s := range c.servers {
		srvs = append(srvs, s)
	}
	c.mu.RUnlock()

	for _, s := range srvs {
		s.close()
	}
}
