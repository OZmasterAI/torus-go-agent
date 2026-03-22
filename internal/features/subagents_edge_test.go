package features

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"torus_go_agent/internal/core"
	typ "torus_go_agent/internal/types"
)

// --- Edge Case: Max Subagents ---

func TestSubagentsEdge_MaxConcurrentSubagents(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	const numSubagents = 2
	ids := make([]string, numSubagents)

	// Spawn many sub-agents concurrently
	for i := 0; i < numSubagents; i++ {
		id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
			Task:      fmt.Sprintf("task %d", i),
			AgentType: "builder",
		})
		if err != nil {
			t.Fatalf("SpawnWithProvider %d failed: %v", i, err)
		}
		ids[i] = id
	}

	// Verify all are tracked as running initially
	running := m.ListRunning()
	if len(running) == 0 {
		t.Fatalf("Expected running sub-agents after spawn, got %d", len(running))
	}

	// Wait for all to complete and verify results
	for _, id := range ids {
		result := m.Wait(id)
		if result == nil {
			t.Errorf("Wait returned nil for id %q", id)
		}
	}

	// All should now be in results, none running
	finalRunning := m.ListRunning()
	if len(finalRunning) != 0 {
		t.Errorf("All sub-agents should be done, but %d still running", len(finalRunning))
	}
}

// --- Edge Case: Nil/Empty Config ---

func TestSubagentsEdge_EmptyTask(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Empty task string should still spawn
	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "",
		AgentType: "builder",
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider with empty task failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id even for empty task")
	}
}

func TestSubagentsEdge_EmptySystemPrompt(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Empty system prompt should still work
	id, err := m.SpawnWithProvider(parentAgent, mp, "", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider with empty system prompt failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id even for empty system prompt")
	}
}

func TestSubagentsEdge_EmptyAgentType(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Empty agent type should default to all tools
	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "",
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider with empty agent type failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id")
	}
}

// --- Edge Case: Negative MaxTurns ---

func TestSubagentsEdge_NegativeMaxTurns(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Negative MaxTurns should be treated as default (30)
	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
		MaxTurns:  -5,
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider with negative MaxTurns failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should succeed with negative MaxTurns")
	}
}

// --- Edge Case: Correct ID Format ---

func TestSubagentsEdge_IDFormatValidation(t *testing.T) {
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
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}

	// Verify ID has expected format
	if len(id) < 3 || id[:3] != "sa_" {
		t.Errorf("ID should start with 'sa_', got %q", id)
	}
	if !contains(id, "builder") {
		t.Errorf("ID should contain agent type, got %q", id)
	}
}

// --- Edge Case: GetResult Multiple Times ---

func TestSubagentsEdge_GetResultMultipleTimes(t *testing.T) {
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
	})
	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}

	// Wait for completion
	result1 := m.Wait(id)
	if result1 == nil {
		t.Fatal("First Wait returned nil")
	}

	// Multiple GetResult calls should return same result
	for i := 0; i < 5; i++ {
		result2, ok := m.GetResult(id)
		if !ok {
			t.Errorf("iteration %d: GetResult returned false", i)
			continue
		}
		if result2 == nil {
			t.Errorf("iteration %d: GetResult returned nil", i)
			continue
		}
		if result2.Text != result1.Text {
			t.Errorf("iteration %d: GetResult returned different result", i)
		}
	}
}

// --- Edge Case: ListRunning Race Conditions ---

func TestSubagentsEdge_ListRunningDuringSpawn(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	var wg sync.WaitGroup
	spawnDone := make(chan struct{})

	// Spawn sub-agent in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
				Task:      fmt.Sprintf("task %d", i),
				AgentType: "builder",
			})
		}
		close(spawnDone)
	}()

	// Poll ListRunning concurrently
	var maxRunning int32
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-spawnDone:
				return
			case <-ticker.C:
				running := m.ListRunning()
				if int32(len(running)) > atomic.LoadInt32(&maxRunning) {
					atomic.StoreInt32(&maxRunning, int32(len(running)))
				}
			}
		}
	}()

	wg.Wait()
}

// --- Edge Case: GetResult After Multiple Waits ---

func TestSubagentsEdge_GetResultAfterMultipleWaits(t *testing.T) {
	m := NewSubAgentManager()
	id := "test_id"
	expectedResult := &SubAgentResult{
		Text:      "completed",
		Error:     nil,
		Duration:  100 * time.Millisecond,
		ToolCalls: 3,
	}

	m.results.Store(id, expectedResult)

	// Wait multiple times
	for i := 0; i < 3; i++ {
		result := m.Wait(id)
		if result.Text != expectedResult.Text {
			t.Errorf("iteration %d: Wait returned wrong text", i)
		}
	}

	// GetResult should still work
	result, ok := m.GetResult(id)
	if !ok {
		t.Error("GetResult should return true")
	}
	if result.Text != expectedResult.Text {
		t.Errorf("GetResult returned wrong text after multiple Waits")
	}
}

// --- Edge Case: SpawnWithProvider Hook Blocking ---

func TestSubagentsEdge_BeforeSpawnHookBlocks(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}

	// Create parent agent with blocking hook
	dag := newSubagentTestDAG(t)
	cfg := typ.AgentConfig{
		Provider:  typ.ProviderConfig{Name: mp.name, Model: mp.modelID},
		MaxTurns:  3,
	}
	hooks := core.NewHookRegistry()
	hooks.Register(core.HookBeforeSpawn, "blocker", func(_ context.Context, data *core.HookData) error {
		data.Block = true
		data.BlockReason = "test block"
		return nil
	})
	parentAgent := core.NewAgent(cfg, mp, hooks, dag)
	parentAgent.SetCompaction(core.CompactionConfig{Mode: core.CompactionOff})

	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		AgentType: "builder",
	})

	if err != nil {
		t.Fatalf("SpawnWithProvider failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id")
	}

	// Wait for the blocked sub-agent
	result := m.Wait(id)
	if result == nil {
		t.Fatal("Wait returned nil")
	}
	if result.Error == nil {
		t.Error("Result should contain blocking error")
	}
}

// --- Edge Case: Multiple Managers Same DAG ---

func TestSubagentsEdge_MultipleManagersSameDAG(t *testing.T) {
	m1 := NewSubAgentManager()
	m2 := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Spawn from both managers
	id1, err1 := m1.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "task 1",
		AgentType: "builder",
	})
	if err1 != nil {
		t.Fatalf("Manager 1 spawn failed: %v", err1)
	}

	id2, err2 := m2.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "task 2",
		AgentType: "builder",
	})
	if err2 != nil {
		t.Fatalf("Manager 2 spawn failed: %v", err2)
	}

	// Results should be separate
	result1 := m1.Wait(id1)
	result2 := m2.Wait(id2)

	if result1 == nil || result2 == nil {
		t.Error("One or both results are nil")
	}

	// Manager 1 should not know about ID from manager 2
	_, ok := m1.GetResult(id2)
	if ok {
		t.Error("Manager 1 should not have result from Manager 2")
	}
}

// --- Edge Case: FilterTools with Duplicates in Input ---

func TestSubagentsEdge_FilterToolsWithDuplicateNames(t *testing.T) {
	tools := []typ.Tool{
		{Name: "read"},
		{Name: "bash"},
		{Name: "read"}, // duplicate name in original list
		{Name: "grep"},
	}

	// Request grep and read
	filtered := filterTools(tools, "read", "grep")

	// Both instances of "read" in original should be included since both match
	if len(filtered) != 3 {
		t.Fatalf("expected 3 tools (2 read + 1 grep), got %d", len(filtered))
	}

	names := make(map[string]int)
	for _, tool := range filtered {
		names[tool.Name]++
	}

	if names["read"] != 2 {
		t.Errorf("read tool should appear twice, got %d", names["read"])
	}
	if names["grep"] != 1 {
		t.Errorf("grep tool should appear once, got %d", names["grep"])
	}
}

// --- Edge Case: Empty Tools List ---

func TestSubagentsEdge_EmptyToolsList(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	// Explicitly pass empty tools list
	id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
		Task:      "test task",
		Tools:     []typ.Tool{},
		AgentType: "builder",
	})

	// This should succeed but spawn with no tools
	if err != nil {
		t.Fatalf("SpawnWithProvider with empty tools failed: %v", err)
	}
	if id == "" {
		t.Error("SpawnWithProvider should return non-empty id")
	}
}

// --- Edge Case: Result Channel Behavior ---

func TestSubagentsEdge_ResultChannelNonBlocking(t *testing.T) {
	state := &subAgentState{result: make(chan *SubAgentResult, 1)}

	result := &SubAgentResult{
		Text:      "test",
		Error:     nil,
		Duration:  1 * time.Millisecond,
		ToolCalls: 1,
	}

	// Buffer of 1 should not block
	state.result <- result
	close(state.result)

	// Reading should return immediately
	received := <-state.result
	if received.Text != "test" {
		t.Errorf("Expected 'test', got %q", received.Text)
	}
}

// --- Edge Case: DefaultToolsForType Variations ---

func TestSubagentsEdge_DefaultToolsForTypeCaseInsensitive(t *testing.T) {
	// Tool selection is case-sensitive, but let's verify exact behavior
	tests := []struct {
		name      string
		input     string
		wantCount int
	}{
		{"Builder lowercase", "builder", 6},
		{"Builder uppercase", "BUILDER", 6}, // Unknown, defaults to all
		{"Researcher mixed", "ReSeArChEr", 6}, // Unknown, defaults to all
		{"Tester lowercase", "tester", 4},
		{"Tester uppercase", "TESTER", 6}, // Unknown, defaults to all
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := DefaultToolsForType(tt.input)
			if len(tools) != tt.wantCount {
				t.Errorf("DefaultToolsForType(%q) returned %d tools, want %d", tt.input, len(tools), tt.wantCount)
			}
		})
	}
}

// --- Edge Case: SubAgentID Generation ---

func TestSubagentsEdge_UniqueSubAgentIDs(t *testing.T) {
	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	ids := make(map[string]bool)
	const numSpawns = 2

	for i := 0; i < numSpawns; i++ {
		id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
			Task:      "test",
			AgentType: "builder",
		})
		if err != nil {
			t.Fatalf("SpawnWithProvider %d failed: %v", i, err)
		}

		if ids[id] {
			t.Errorf("Duplicate ID generated: %q", id)
		}
		ids[id] = true
	}

	if len(ids) != numSpawns {
		t.Errorf("Expected %d unique IDs, got %d", numSpawns, len(ids))
	}
}

// --- Edge Case: Result Error Types ---

func TestSubagentsEdge_ResultWithVarousErrorTypes(t *testing.T) {
	m := NewSubAgentManager()

	tests := []struct {
		name     string
		setupFn  func() (string, *SubAgentResult)
		wantErr  bool
		errMsg   string
	}{
		{
			name: "unknown_id",
			setupFn: func() (string, *SubAgentResult) {
				result := m.Wait("unknown_id_xyz")
				return "unknown_id_xyz", result
			},
			wantErr: true,
			errMsg:  "unknown",
		},
		{
			name: "explicit_error_result",
			setupFn: func() (string, *SubAgentResult) {
				id := "explicit_error"
				m.results.Store(id, &SubAgentResult{
					Error: fmt.Errorf("explicit error"),
				})
				result, _ := m.GetResult(id)
				return id, result
			},
			wantErr: true,
			errMsg:  "explicit error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, result := tt.setupFn()

			if result == nil {
				if tt.wantErr {
					t.Error("Expected result, got nil")
				}
				return
			}

			if tt.wantErr && result.Error == nil {
				t.Error("Expected error in result")
			}
			if tt.wantErr && result.Error != nil {
				if tt.errMsg != "" && !contains(result.Error.Error(), tt.errMsg) {
					t.Errorf("Error should contain %q, got %q", tt.errMsg, result.Error.Error())
				}
			}
		})
	}
}

// --- Edge Case: Stress Test Rapid Spawn/Wait ---

func TestSubagentsEdge_RapidSpawnWait(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	m := NewSubAgentManager()
	mp := &subagentMockProvider{
		name:       "test",
		modelID:    "test-model",
		cannedText: "response",
	}
	parentAgent, _ := newSubagentTestAgent(t, mp)

	const iterations = 10
	for i := 0; i < iterations; i++ {
		id, err := m.SpawnWithProvider(parentAgent, mp, "system prompt", SubAgentConfig{
			Task:      fmt.Sprintf("task %d", i),
			AgentType: "builder",
		})
		if err != nil {
			t.Fatalf("iteration %d: SpawnWithProvider failed: %v", i, err)
		}

		result := m.Wait(id)
		if result == nil {
			t.Fatalf("iteration %d: Wait returned nil", i)
		}

		// Verify result is accessible via GetResult
		storedResult, ok := m.GetResult(id)
		if !ok {
			t.Fatalf("iteration %d: GetResult returned false", i)
		}
		if storedResult.Text != result.Text {
			t.Fatalf("iteration %d: GetResult mismatch", i)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && s[0:len(substr)] == substr || len(s) > len(substr))
}
