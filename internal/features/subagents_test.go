package features

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"torus_go_agent/internal/core"
	typ "torus_go_agent/internal/types"
)

// --- Mock Provider ---

// subagentMockProvider implements types.Provider and returns canned responses for subagent tests.
type subagentMockProvider struct {
	name         string
	modelID      string
	cannedText   string
	cannedUsage  typ.Usage
	streamErr    error
	completeResp *typ.AssistantMessage
	completeErr  error
}

func (m *subagentMockProvider) Name() string    { return m.name }
func (m *subagentMockProvider) ModelID() string { return m.modelID }

func (m *subagentMockProvider) Complete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (*typ.AssistantMessage, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	if m.completeResp != nil {
		return m.completeResp, nil
	}
	return &typ.AssistantMessage{
		Message: typ.Message{
			Role:    typ.RoleAssistant,
			Content: []typ.ContentBlock{{Type: "text", Text: m.cannedText}},
		},
		Model:      m.modelID,
		StopReason: "end_turn",
		Usage:      m.cannedUsage,
	}, nil
}

func (m *subagentMockProvider) StreamComplete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (<-chan typ.StreamEvent, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan typ.StreamEvent, 4)
	go func() {
		defer close(ch)
		if m.cannedText != "" {
			ch <- typ.StreamEvent{Type: typ.EventTextDelta, Text: m.cannedText}
		}
		resp := &typ.AssistantMessage{
			Message: typ.Message{
				Role:    typ.RoleAssistant,
				Content: []typ.ContentBlock{{Type: "text", Text: m.cannedText}},
			},
			Model:      m.modelID,
			StopReason: "end_turn",
			Usage:      m.cannedUsage,
		}
		ch <- typ.StreamEvent{Type: typ.EventMessageStop, Response: resp}
	}()
	return ch, nil
}

// --- Test Helpers ---

func newSubagentTestDAG(t *testing.T) *core.DAG {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dag, err := core.NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	t.Cleanup(func() { dag.Close() })
	return dag
}

func newSubagentTestAgent(t *testing.T, mp *subagentMockProvider) (*core.Agent, *core.DAG) {
	t.Helper()
	dag := newSubagentTestDAG(t)
	cfg := typ.AgentConfig{
		Provider:  typ.ProviderConfig{Name: mp.name, Model: mp.modelID, MaxTokens: 1024},
		MaxTurns:  3,
	}
	hooks := core.NewHookRegistry()
	agent := core.NewAgent(cfg, mp, hooks, dag)
	// Disable LLM compaction so tests never need a Summarize function.
	agent.SetCompaction(core.CompactionConfig{Mode: core.CompactionOff})
	return agent, dag
}

// --- Tests ---

func TestNewSubAgentManager(t *testing.T) {
	tests := []struct {
		name string
		want *SubAgentManager
	}{
		{
			name: "creates an empty manager",
			want: &SubAgentManager{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSubAgentManager()
			if got == nil {
				t.Fatal("NewSubAgentManager returned nil")
			}
			// Verify both maps are empty
			running := got.ListRunning()
			if len(running) != 0 {
				t.Errorf("ListRunning() = %v, want empty", running)
			}
		})
	}
}

func TestDefaultToolsForType_Builder(t *testing.T) {
	tools := DefaultToolsForType("builder")
	if len(tools) != 6 {
		t.Fatalf("builder should have 6 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	expected := []string{"bash", "read", "write", "edit", "glob", "grep"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("builder missing tool: %s", name)
		}
	}
}

func TestDefaultToolsForType_Researcher(t *testing.T) {
	tools := DefaultToolsForType("researcher")
	if len(tools) != 3 {
		t.Fatalf("researcher should have 3 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	expected := []string{"read", "glob", "grep"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("researcher missing tool: %s", name)
		}
	}
	// Verify excluded tools
	excluded := []string{"bash", "write", "edit"}
	for _, name := range excluded {
		if names[name] {
			t.Errorf("researcher should not have tool: %s", name)
		}
	}
}

func TestDefaultToolsForType_Tester(t *testing.T) {
	tools := DefaultToolsForType("tester")
	if len(tools) != 4 {
		t.Fatalf("tester should have 4 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	expected := []string{"bash", "read", "glob", "grep"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("tester missing tool: %s", name)
		}
	}
	// Verify excluded tools
	excluded := []string{"write", "edit"}
	for _, name := range excluded {
		if names[name] {
			t.Errorf("tester should not have tool: %s", name)
		}
	}
}

func TestDefaultToolsForType_Unknown(t *testing.T) {
	tools := DefaultToolsForType("unknown")
	if len(tools) != 6 {
		t.Fatalf("unknown type should default to all 6 tools, got %d", len(tools))
	}
}

func TestDefaultToolsForType_FreshCopy(t *testing.T) {
	tools1 := DefaultToolsForType("researcher")
	tools2 := DefaultToolsForType("researcher")

	// They should have the same content but be different slices
	if len(tools1) != len(tools2) {
		t.Fatalf("copies have different lengths: %d vs %d", len(tools1), len(tools2))
	}

	// Modifying one should not affect the other
	if &tools1[0] == &tools2[0] {
		t.Error("DefaultToolsForType should return fresh copies, not shared slices")
	}
}

func TestSpawnWithProvider_NilParentAgent(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	id, err := m.SpawnWithProvider(nil, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
	})

	if err == nil {
		t.Error("SpawnWithProvider with nil parentAgent should error")
	}
	if id != "" {
		t.Errorf("SpawnWithProvider with nil parentAgent should return empty id, got %q", id)
	}
	if err.Error() != "subagents: parentAgent must not be nil" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSpawnWithProvider_NilProvider(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{name: "test", modelID: "test-model"}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	id, err := m.SpawnWithProvider(parentAgent, nil, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
	})

	if err == nil {
		t.Error("SpawnWithProvider with nil provider should error")
	}
	if id != "" {
		t.Errorf("SpawnWithProvider with nil provider should return empty id, got %q", id)
	}
	if err.Error() != "subagents: provider must not be nil" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSpawnWithProvider_ValidSpawn(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "test response",
		cannedUsage: typ.Usage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	id, err := m.SpawnWithProvider(parentAgent, mp, "test system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
		MaxTurns:  5,
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id")
	}

	// ID should contain "sa_" prefix
	if len(id) < 3 || id[:3] != "sa_" {
		t.Errorf("ID should start with 'sa_', got %q", id)
	}
}

func TestSpawnWithProvider_DefaultMaxTurns(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
		MaxTurns:  0, // Should default to 30
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}
	if id == "" {
		t.Fatal("SpawnWithProvider returned empty id")
	}
}

func TestSpawnWithProvider_CustomTools(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	customTools := []typ.Tool{
		{
			Name:        "custom",
			Description: "custom tool",
			Execute: func(args map[string]any) (*typ.ToolResult, error) {
				return &typ.ToolResult{Content: "custom result"}, nil
			},
		},
	}

	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		Tools:     customTools,
		MaxTurns:  5,
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider with custom tools failed: %v", err)
	}
	if id == "" {
		t.Fatal("SpawnWithProvider returned empty id")
	}
}

func TestGetResult_Unknown(t *testing.T) {
	m := NewSubAgentManager()

	result, ok := m.GetResult("unknown_id")

	if ok {
		t.Error("GetResult should return false for unknown ID")
	}
	if result != nil {
		t.Error("GetResult should return nil for unknown ID")
	}
}

func TestListRunning_Empty(t *testing.T) {
	m := NewSubAgentManager()

	running := m.ListRunning()

	if len(running) != 0 {
		t.Errorf("ListRunning on empty manager should return empty slice, got %v", running)
	}
}

func TestWait_UnknownID(t *testing.T) {
	m := NewSubAgentManager()

	result := m.Wait("unknown_id")

	if result == nil {
		t.Fatal("Wait should never return nil")
	}
	if result.Error == nil {
		t.Error("Wait with unknown ID should have an error")
	}
}

func TestWait_AlreadyCompleted(t *testing.T) {
	m := NewSubAgentManager()
	id := "test_id"
	expectedResult := &SubAgentResult{
		Text:      "completed",
		Error:     nil,
		Duration:  100 * time.Millisecond,
		ToolCalls: 2,
	}

	// Store a result directly (simulating a completed sub-agent)
	m.results.Store(id, expectedResult)

	result := m.Wait(id)

	if result == nil {
		t.Fatal("Wait should not return nil")
	}
	if result.Text != expectedResult.Text {
		t.Errorf("Wait returned wrong text: got %q, want %q", result.Text, expectedResult.Text)
	}
	if result.ToolCalls != expectedResult.ToolCalls {
		t.Errorf("Wait returned wrong ToolCalls: got %d, want %d", result.ToolCalls, expectedResult.ToolCalls)
	}
}

func TestSpawnAndWait_Integration(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "integration test response",
		cannedUsage: typ.Usage{
			InputTokens:  5,
			OutputTokens: 15,
			TotalTokens:  20,
		},
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "integration test",
		AgentType: "builder",
		MaxTurns:  2,
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}

	// Wait for the sub-agent to complete with a timeout
	done := make(chan *SubAgentResult, 1)
	go func() {
		done <- m.Wait(id)
	}()

	select {
	case result := <-done:
		if result == nil {
			t.Fatal("Wait returned nil")
		}
		if result.Text != "integration test response" {
			t.Errorf("Wait returned unexpected text: got %q", result.Text)
		}
		if result.Duration <= 0 {
			t.Error("Wait returned non-positive duration")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Wait timed out after 10 seconds")
	}

	// After waiting, result should be retrievable via GetResult
	result, ok := m.GetResult(id)
	if !ok {
		t.Error("GetResult should return true after Wait completes")
	}
	if result == nil {
		t.Fatal("GetResult returned nil after Wait")
	}
}

func TestFilterTools(t *testing.T) {
	tests := []struct {
		name      string
		tools     []typ.Tool
		names     []string
		wantLen   int
		wantNames []string
	}{
		{
			name: "filter subset",
			tools: []typ.Tool{
				{Name: "a"},
				{Name: "b"},
				{Name: "c"},
				{Name: "d"},
			},
			names:     []string{"a", "c"},
			wantLen:   2,
			wantNames: []string{"a", "c"},
		},
		{
			name: "filter empty result",
			tools: []typ.Tool{
				{Name: "a"},
				{Name: "b"},
			},
			names:     []string{"x", "y"},
			wantLen:   0,
			wantNames: []string{},
		},
		{
			name:      "filter empty input",
			tools:     []typ.Tool{},
			names:     []string{"a"},
			wantLen:   0,
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterTools(tt.tools, tt.names...)
			if len(got) != tt.wantLen {
				t.Fatalf("filterTools returned %d tools, want %d", len(got), tt.wantLen)
			}
			for i, tool := range got {
				if tool.Name != tt.wantNames[i] {
					t.Errorf("filterTools[%d].Name = %q, want %q", i, tool.Name, tt.wantNames[i])
				}
			}
		})
	}
}

func TestFilterTools_OrderPreserved(t *testing.T) {
	tools := []typ.Tool{
		{Name: "bash"},
		{Name: "read"},
		{Name: "write"},
		{Name: "edit"},
		{Name: "glob"},
		{Name: "grep"},
	}

	filtered := filterTools(tools, "grep", "bash", "read")

	// Order should match the original list, not the requested names
	if len(filtered) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(filtered))
	}
	if filtered[0].Name != "bash" {
		t.Errorf("first tool should be bash, got %s", filtered[0].Name)
	}
	if filtered[1].Name != "read" {
		t.Errorf("second tool should be read, got %s", filtered[1].Name)
	}
	if filtered[2].Name != "grep" {
		t.Errorf("third tool should be grep, got %s", filtered[2].Name)
	}
}

func TestSubAgentConfig_ZeroValue(t *testing.T) {
	cfg := SubAgentConfig{}

	if cfg.Task != "" {
		t.Errorf("zero-value Task should be empty, got %q", cfg.Task)
	}
	if cfg.AgentType != "" {
		t.Errorf("zero-value AgentType should be empty, got %q", cfg.AgentType)
	}
	if cfg.Tools != nil {
		t.Errorf("zero-value Tools should be nil, got %v", cfg.Tools)
	}
	if cfg.MaxTurns != 0 {
		t.Errorf("zero-value MaxTurns should be 0, got %d", cfg.MaxTurns)
	}
}

func TestSubAgentResult_ZeroValue(t *testing.T) {
	result := SubAgentResult{}

	if result.Text != "" {
		t.Errorf("zero-value Text should be empty, got %q", result.Text)
	}
	if result.Error != nil {
		t.Errorf("zero-value Error should be nil, got %v", result.Error)
	}
	if result.Duration != 0 {
		t.Errorf("zero-value Duration should be 0, got %v", result.Duration)
	}
	if result.ToolCalls != 0 {
		t.Errorf("zero-value ToolCalls should be 0, got %d", result.ToolCalls)
	}
}
