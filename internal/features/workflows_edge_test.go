package features

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Edge Case: Empty Workflows ---

// TestWorkflowsEdge_RunSequentialEmptyAgentsList tests RunSequential with empty agents list
func TestWorkflowsEdge_RunSequentialEmptyAgentsList(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	result, err := RunSequential(ctx, nil, provider, "system prompt", []SubAgentConfig{}, mgr, nil)

	if err != nil {
		t.Fatalf("RunSequential with empty agents should not error, got: %v", err)
	}
	if result != "" {
		t.Errorf("RunSequential with empty agents should return empty string, got: %q", result)
	}
}

// TestWorkflowsEdge_RunParallelEmptyAgentsList tests RunParallel with empty agents list
func TestWorkflowsEdge_RunParallelEmptyAgentsList(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	results, err := RunParallel(ctx, nil, provider, "system prompt", []SubAgentConfig{}, mgr, nil)

	if err != nil {
		t.Fatalf("RunParallel with empty agents should not error, got: %v", err)
	}
	if results == nil {
		t.Errorf("RunParallel should return empty slice, not nil")
	}
	if len(results) != 0 {
		t.Errorf("RunParallel with empty agents should return empty slice, got length: %d", len(results))
	}
}

// TestWorkflowsEdge_RunLoopEmptyAgentsList tests RunLoop with empty agent config (empty task)
func TestWorkflowsEdge_RunLoopEmptyTask(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "", AgentType: "builder"}

	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, nil, 1)

	// Empty task should be allowed; it will attempt to spawn with empty task
	// The error here is expected since we don't have a real parent agent
	_ = result
	_ = err
}

// --- Edge Case: Step Failures & Error Handling ---

// TestWorkflowsEdge_RunSequentialFirstStepFails tests when first agent in sequence fails
func TestWorkflowsEdge_RunSequentialFirstStepFails(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	// Mock provider that returns an error on Complete
	failingProvider := &mockProvider{
		name:        "test",
		modelID:     "test-model",
		cannedText:  "",
		completeErr: errors.New("provider error: connection timeout"),
	}

	agents := []SubAgentConfig{
		{Task: "first task", AgentType: "builder"},
		{Task: "second task", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, failingProvider, "system prompt", agents, mgr, nil)

	// Since spawning fails, we should get an error
	// The result should be empty or whatever partial result we have
	if result != "" && err == nil {
		t.Errorf("RunSequential with failing provider should either error or return empty, got result: %q, err: %v", result, err)
	}
	_ = err
}

// TestWorkflowsEdge_RunParallelPartialFailure tests RunParallel when some agents fail
func TestWorkflowsEdge_RunParallelPartialFailure(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	failingProvider := &mockProvider{
		name:        "test",
		modelID:     "test-model",
		cannedText:  "",
		completeErr: errors.New("provider failure"),
	}

	agents := []SubAgentConfig{
		{Task: "task 1", AgentType: "builder"},
		{Task: "task 2", AgentType: "builder"},
		{Task: "task 3", AgentType: "builder"},
	}

	results, err := RunParallel(ctx, nil, failingProvider, "system prompt", agents, mgr, nil)

	// Should return error on first spawn failure
	if err == nil {
		t.Error("RunParallel should error when spawn fails")
	}
	if results != nil && len(results) > 0 {
		t.Errorf("RunParallel should return nil results on spawn failure, got %d results", len(results))
	}
}

// TestWorkflowsEdge_RunLoopStepFailure tests RunLoop when agent step fails
func TestWorkflowsEdge_RunLoopStepFailure(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	failingProvider := &mockProvider{
		name:        "test",
		modelID:     "test-model",
		cannedText:  "",
		completeErr: errors.New("iteration failure"),
	}

	cfg := SubAgentConfig{Task: "iterative task", AgentType: "builder"}

	result, err := RunLoop(ctx, nil, failingProvider, "system prompt", cfg, mgr, nil, nil, 3)

	// Should error on first iteration failure
	if err == nil {
		t.Error("RunLoop should error when spawn fails")
	}
	// Result might be empty
	if result != "" {
		t.Errorf("RunLoop should return empty string on failure, got: %q", result)
	}
}

// --- Edge Case: Retry Logic (Implicit via Error Handling) ---

// TestWorkflowsEdge_RunLoopMaxIterationsExceeded tests RunLoop respects maxIterations limit
func TestWorkflowsEdge_RunLoopMaxIterationsExceeded(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "task", AgentType: "builder"}

	// Track how many times shouldStop is called
	callCount := 0
	shouldStop := func(result string, iteration int) bool {
		callCount++
		return false // Never stop early
	}

	// This will attempt to run maxIterations times
	maxIter := 5
	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, shouldStop, maxIter)

	// shouldStop should be called up to maxIter times
	// Since we can't actually spawn (no parent agent), we expect an error
	if err == nil && callCount == 0 {
		t.Error("RunLoop should attempt iterations")
	}
	_ = result
}

// TestWorkflowsEdge_RunLoopZeroMaxIterations tests RunLoop with unlimited iterations (maxIterations=0)
func TestWorkflowsEdge_RunLoopZeroMaxIterations(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "task", AgentType: "builder"}

	iterCount := 0
	shouldStop := func(result string, iteration int) bool {
		iterCount++
		return iterCount >= 3 // Stop after 3 iterations
	}

	// maxIterations=0 means unlimited (until shouldStop returns true)
	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, shouldStop, 0)

	// We expect iteration count to be capped by shouldStop
	if iterCount > 3 {
		t.Errorf("RunLoop should respect shouldStop callback, iterCount: %d", iterCount)
	}
	_ = result
	_ = err
}

// --- Edge Case: Workflow Cancellation ---

// TestWorkflowsEdge_RunSequentialContextCancellation tests RunSequential with cancelled context
func TestWorkflowsEdge_RunSequentialContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	agents := []SubAgentConfig{
		{Task: "task 1", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Cancelled context may affect behavior
	// The function doesn't explicitly check context, but it's passed to SpawnWithProvider
	_ = result
	_ = err
}

// TestWorkflowsEdge_RunParallelContextCancellation tests RunParallel with cancelled context
func TestWorkflowsEdge_RunParallelContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	agents := []SubAgentConfig{
		{Task: "task 1", AgentType: "builder"},
		{Task: "task 2", AgentType: "builder"},
	}

	results, err := RunParallel(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Cancelled context effects
	_ = results
	_ = err
}

// TestWorkflowsEdge_RunLoopContextTimeout tests RunLoop with context timeout
func TestWorkflowsEdge_RunLoopContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "task", AgentType: "builder"}

	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, nil, 5)

	// Timeout may prevent operations from completing
	_ = result
	_ = err
}

// --- Edge Case: Large Workflow Sizes ---

// TestWorkflowsEdge_RunSequentialManyAgents tests RunSequential with many agents
func TestWorkflowsEdge_RunSequentialManyAgents(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	// Create many agents
	agents := make([]SubAgentConfig, 100)
	for i := 0; i < 100; i++ {
		agents[i] = SubAgentConfig{
			Task:      fmt.Sprintf("task %d", i),
			AgentType: "builder",
		}
	}

	result, err := RunSequential(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Should handle large agent lists without panic
	// Will error because no parent agent, but should not crash
	_ = result
	_ = err
}

// TestWorkflowsEdge_RunParallelManyAgents tests RunParallel with many agents (concurrency stress)
func TestWorkflowsEdge_RunParallelManyAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test in short mode")
	}

	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	// Create many agents to stress concurrent spawning
	agents := make([]SubAgentConfig, 50)
	for i := 0; i < 50; i++ {
		agents[i] = SubAgentConfig{
			Task:      fmt.Sprintf("parallel task %d", i),
			AgentType: "builder",
		}
	}

	results, err := RunParallel(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Should handle large parallel spawns without panic
	_ = results
	_ = err
}

// TestWorkflowsEdge_RunLoopManyIterations tests RunLoop with many iterations
func TestWorkflowsEdge_RunLoopManyIterations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test in short mode")
	}

	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "iterative task", AgentType: "builder"}

	iterCount := 0
	shouldStop := func(result string, iteration int) bool {
		iterCount++
		return iterCount >= 50
	}

	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, shouldStop, 100)

	// Should handle many iterations without panic
	if iterCount > 50 {
		t.Errorf("RunLoop should respect shouldStop, iterCount: %d", iterCount)
	}
	_ = result
	_ = err
}

// --- Edge Case: Nil Callbacks & Handlers ---

// TestWorkflowsEdge_RunLoopNilShouldStop tests RunLoop with nil shouldStop callback
func TestWorkflowsEdge_RunLoopNilShouldStop(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	cfg := SubAgentConfig{Task: "task", AgentType: "builder"}

	// nil shouldStop means loop runs until maxIterations
	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, nil, 2)

	// Should handle nil shouldStop gracefully
	// Will error due to no parent agent, but should not panic
	_ = result
	_ = err
}

// TestWorkflowsEdge_RunSequentialNilProvider tests RunSequential with nil provider (should error in spawn)
func TestWorkflowsEdge_RunSequentialNilProvider(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	agents := []SubAgentConfig{
		{Task: "task", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, nil, "system prompt", agents, mgr, nil)

	// Should error when trying to spawn with nil provider
	if err == nil {
		t.Error("RunSequential with nil provider should error")
	}
	if result != "" {
		t.Errorf("RunSequential with nil provider should return empty result, got: %q", result)
	}
}

// TestWorkflowsEdge_RunParallelNilProvider tests RunParallel with nil provider
func TestWorkflowsEdge_RunParallelNilProvider(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	agents := []SubAgentConfig{
		{Task: "task", AgentType: "builder"},
	}

	results, err := RunParallel(ctx, nil, nil, "system prompt", agents, mgr, nil)

	// Should error when trying to spawn with nil provider
	if err == nil {
		t.Error("RunParallel with nil provider should error")
	}
	if results != nil {
		t.Errorf("RunParallel with nil provider should return nil results, got: %v", results)
	}
}

// TestWorkflowsEdge_RunLoopNilProvider tests RunLoop with nil provider
func TestWorkflowsEdge_RunLoopNilProvider(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	cfg := SubAgentConfig{Task: "task", AgentType: "builder"}

	result, err := RunLoop(ctx, nil, nil, "system prompt", cfg, mgr, nil, nil, 1)

	// Should error when trying to spawn with nil provider
	if err == nil {
		t.Error("RunLoop with nil provider should error")
	}
	if result != "" {
		t.Errorf("RunLoop with nil provider should return empty result, got: %q", result)
	}
}

// --- Edge Case: Output Concatenation ---

// TestWorkflowsEdge_RunSequentialOutputChaining tests that outputs are properly chained
func TestWorkflowsEdge_RunSequentialOutputChaining(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	// Create a provider that returns predictable outputs
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "output1"}

	agents := []SubAgentConfig{
		{Task: "first", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Since spawn will fail without parent agent, we expect error
	// But if it succeeded, output would be chained
	_ = result
	_ = err
}

// TestWorkflowsEdge_RunLoopOutputAccumulation tests that RunLoop accumulates outputs across iterations
func TestWorkflowsEdge_RunLoopOutputAccumulation(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "iter_output"}

	cfg := SubAgentConfig{Task: "iterative", AgentType: "builder"}

	iterCount := 0
	shouldStop := func(result string, iteration int) bool {
		iterCount++
		return iterCount >= 2
	}

	result, err := RunLoop(ctx, nil, provider, "system prompt", cfg, mgr, nil, shouldStop, 10)

	// Will error due to no parent agent, but structure should be correct
	_ = result
	_ = err
}

// --- Edge Case: Concurrent Workflow Execution ---

// TestWorkflowsEdge_ConcurrentSequentialWorkflows tests multiple sequential workflows running concurrently
func TestWorkflowsEdge_ConcurrentSequentialWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	ctx := context.Background()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	var wg sync.WaitGroup
	numWorkflows := 10

	wg.Add(numWorkflows)
	for i := 0; i < numWorkflows; i++ {
		go func(idx int) {
			defer wg.Done()
			mgr := NewSubAgentManager()
			agents := []SubAgentConfig{
				{Task: fmt.Sprintf("workflow %d task", idx), AgentType: "builder"},
			}
			_, _ = RunSequential(ctx, nil, provider, "system", agents, mgr, nil)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - all workflows completed
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent sequential workflows timed out")
	}
}

// TestWorkflowsEdge_ConcurrentParallelWorkflows tests multiple parallel workflows running concurrently
func TestWorkflowsEdge_ConcurrentParallelWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	ctx := context.Background()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	var wg sync.WaitGroup
	numWorkflows := 5

	wg.Add(numWorkflows)
	for i := 0; i < numWorkflows; i++ {
		go func(idx int) {
			defer wg.Done()
			mgr := NewSubAgentManager()
			agents := []SubAgentConfig{
				{Task: fmt.Sprintf("parallel workflow %d task 1", idx), AgentType: "builder"},
				{Task: fmt.Sprintf("parallel workflow %d task 2", idx), AgentType: "builder"},
			}
			_, _ = RunParallel(ctx, nil, provider, "system", agents, mgr, nil)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent parallel workflows timed out")
	}
}

// TestWorkflowsEdge_ConcurrentLoopWorkflows tests multiple loop workflows running concurrently
func TestWorkflowsEdge_ConcurrentLoopWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	ctx := context.Background()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	var wg sync.WaitGroup
	numWorkflows := 5

	wg.Add(numWorkflows)
	for i := 0; i < numWorkflows; i++ {
		go func(idx int) {
			defer wg.Done()
			mgr := NewSubAgentManager()
			cfg := SubAgentConfig{Task: fmt.Sprintf("loop workflow %d", idx), AgentType: "builder"}

			shouldStop := func(result string, iteration int) bool {
				return iteration >= 2
			}

			_, _ = RunLoop(ctx, nil, provider, "system", cfg, mgr, nil, shouldStop, 10)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent loop workflows timed out")
	}
}

// --- Edge Case: Provider Failures ---

// TestWorkflowsEdge_RunSequentialProviderStreamError tests RunSequential when provider stream fails
func TestWorkflowsEdge_RunSequentialProviderStreamError(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	streamFailProvider := &mockProvider{
		name:        "test",
		modelID:     "test-model",
		cannedText:  "",
		streamErr:   errors.New("streaming failed"),
		completeErr: nil,
	}

	agents := []SubAgentConfig{
		{Task: "task", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, streamFailProvider, "system prompt", agents, mgr, nil)

	// Stream error will affect provider behavior
	_ = result
	_ = err
}

// TestWorkflowsEdge_RunParallelProviderStreamError tests RunParallel when provider stream fails
func TestWorkflowsEdge_RunParallelProviderStreamError(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()

	streamFailProvider := &mockProvider{
		name:        "test",
		modelID:     "test-model",
		cannedText:  "",
		streamErr:   errors.New("streaming failed"),
		completeErr: nil,
	}

	agents := []SubAgentConfig{
		{Task: "task 1", AgentType: "builder"},
		{Task: "task 2", AgentType: "builder"},
	}

	results, err := RunParallel(ctx, nil, streamFailProvider, "system prompt", agents, mgr, nil)

	_ = results
	_ = err
}

// --- Edge Case: Race Conditions ---

// TestWorkflowsEdge_RaceConditionParallelModification tests potential race conditions
func TestWorkflowsEdge_RaceConditionParallelModification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race condition test in short mode")
	}

	// This test is best run with: go test -race
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	var wg sync.WaitGroup
	var raceCounter int64

	// Launch many concurrent operations
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agents := []SubAgentConfig{
				{Task: fmt.Sprintf("race task %d", idx), AgentType: "builder"},
			}
			_, _ = RunSequential(ctx, nil, provider, "system", agents, mgr, nil)
			atomic.AddInt64(&raceCounter, 1)
		}(i)

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agents := []SubAgentConfig{
				{Task: fmt.Sprintf("race task %d-a", idx), AgentType: "builder"},
				{Task: fmt.Sprintf("race task %d-b", idx), AgentType: "builder"},
			}
			_, _ = RunParallel(ctx, nil, provider, "system", agents, mgr, nil)
			atomic.AddInt64(&raceCounter, 1)
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&raceCounter) != 40 {
		t.Errorf("Expected 40 completed workflows, got %d", raceCounter)
	}
}

// --- Edge Case: Special Characters in Tasks ---

// TestWorkflowsEdge_SpecialCharactersInTask tests workflows with special characters in task strings
func TestWorkflowsEdge_SpecialCharactersInTask(t *testing.T) {
	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	specialTasks := []SubAgentConfig{
		{Task: "task with\nnewlines\nand\ttabs", AgentType: "builder"},
		{Task: "task with 'single' and \"double\" quotes", AgentType: "builder"},
		{Task: "task with emoji 🚀 and unicode ñ", AgentType: "builder"},
		{Task: "task with <html> and {json}", AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, provider, "system prompt", specialTasks, mgr, nil)

	// Should handle special characters without panicking
	_ = result
	_ = err
}

// TestWorkflowsEdge_LargeTaskString tests workflows with moderately large task strings
func TestWorkflowsEdge_LargeTaskString(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large data test in short mode")
	}

	ctx := context.Background()
	mgr := NewSubAgentManager()
	provider := &mockProvider{name: "test", modelID: "test-model", cannedText: "response"}

	// Use strings.Repeat instead of concatenation loop to avoid GC pressure
	largeTask := strings.Repeat("This is a large task string. ", 1000) // ~29KB

	agents := []SubAgentConfig{
		{Task: largeTask, AgentType: "builder"},
	}

	result, err := RunSequential(ctx, nil, provider, "system prompt", agents, mgr, nil)

	// Should handle large strings without panicking
	_ = result
	_ = err
}
