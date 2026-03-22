package features

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"torus_go_agent/internal/core"
	typ "torus_go_agent/internal/types"
)

// TestTelemetryEdge_DisabledTelemetry verifies behavior when no hooks are registered.
// This tests the case where telemetry is completely disabled.
func TestTelemetryEdge_DisabledTelemetry(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	// Do NOT register hooks
	ctx := context.Background()

	// Manually fire hooks without registration (simulating missing setup)
	hookData := &core.HookData{Meta: make(map[string]any)}

	// These should not fail, but should not update metrics either
	err := hooks.Fire(ctx, core.HookBeforeLLMCall, hookData)
	if err != nil {
		t.Fatalf("Fire on unregistered hook returned error: %v", err)
	}

	// Metrics should remain zero since hooks aren't registered
	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 0 {
		t.Errorf("expected LLMCalls=0 when telemetry disabled, got %d", metrics.LLMCalls)
	}
	if metrics.TokensIn != 0 {
		t.Errorf("expected TokensIn=0 when telemetry disabled, got %d", metrics.TokensIn)
	}

	spans := tc.GetSpans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans when telemetry disabled, got %d", len(spans))
	}
}

// TestTelemetryEdge_HighFrequencyEvents tests rapid-fire hook calls.
// This stresses concurrent access with many events in short time.
func TestTelemetryEdge_HighFrequencyEvents(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	numEvents := 1000
	var wg sync.WaitGroup

	// Fire 1000 LLM events as fast as possible
	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			hookDataBefore := &core.HookData{Meta: make(map[string]any)}
			hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)

			// Minimal delay
			hookDataAfter := &core.HookData{
				Meta:      hookDataBefore.Meta,
				TokensIn:  1,
				TokensOut: 1,
			}
			hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)
		}(i)
	}

	wg.Wait()

	metrics := tc.GetMetrics()
	if metrics.LLMCalls != numEvents {
		t.Errorf("expected LLMCalls=%d, got %d", numEvents, metrics.LLMCalls)
	}
	if metrics.TokensIn != numEvents {
		t.Errorf("expected TokensIn=%d, got %d", numEvents, metrics.TokensIn)
	}

	spans := tc.GetSpans()
	if len(spans) != numEvents {
		t.Errorf("expected %d spans, got %d", numEvents, len(spans))
	}
}

// TestTelemetryEdge_SpanNesting tests interleaved LLM and tool calls.
// This simulates nested spans: LLM starts, tool starts/ends, LLM ends.
func TestTelemetryEdge_SpanNesting(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// LLM call #1 starts
	llmData1Before := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, llmData1Before)
	time.Sleep(5 * time.Millisecond)

	// Tool call starts while LLM is still active
	toolDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: "nested_tool"}
	hooks.Fire(ctx, core.HookBeforeToolCall, toolDataBefore)
	time.Sleep(5 * time.Millisecond)

	// Tool call ends
	toolDataAfter := &core.HookData{
		Meta:       toolDataBefore.Meta,
		ToolName:   "nested_tool",
		ToolResult: &typ.ToolResult{IsError: false},
	}
	hooks.Fire(ctx, core.HookAfterToolCall, toolDataAfter)
	time.Sleep(5 * time.Millisecond)

	// LLM call #1 ends
	llmData1After := &core.HookData{
		Meta:      llmData1Before.Meta,
		TokensIn:  100,
		TokensOut: 50,
	}
	hooks.Fire(ctx, core.HookAfterLLMCall, llmData1After)

	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 1 {
		t.Errorf("expected LLMCalls=1, got %d", metrics.LLMCalls)
	}
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}

	spans := tc.GetSpans()
	if len(spans) != 2 {
		t.Errorf("expected 2 spans (LLM + tool), got %d", len(spans))
	}

	// Spans should be in order: tool span should start after LLM span starts
	// but end before LLM span ends (nested timing)
	if spans[0].Name != "tool.nested_tool" {
		t.Errorf("expected first span to be tool, got %q", spans[0].Name)
	}
	if spans[1].Name != "llm.call" {
		t.Errorf("expected second span to be llm, got %q", spans[1].Name)
	}
}

// TestTelemetryEdge_MissingStartMetadata tests AfterLLMCall without corresponding Before.
// This simulates a scenario where the start timestamp is lost or missing.
func TestTelemetryEdge_MissingStartMetadata(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Fire after hook WITHOUT firing before hook first
	// This means Meta will not have _telem_llm_start
	hookDataAfter := &core.HookData{
		Meta:      make(map[string]any), // Empty, no start timestamp
		TokensIn:  100,
		TokensOut: 50,
	}

	err := hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)
	if err != nil {
		t.Fatalf("Fire HookAfterLLMCall with missing metadata returned error: %v", err)
	}

	metrics := tc.GetMetrics()
	// LLM call should still be counted
	if metrics.LLMCalls != 1 {
		t.Errorf("expected LLMCalls=1, got %d", metrics.LLMCalls)
	}
	// Tokens should still be counted
	if metrics.TokensIn != 100 {
		t.Errorf("expected TokensIn=100, got %d", metrics.TokensIn)
	}

	// But no span should be recorded (since we couldn't calculate duration)
	spans := tc.GetSpans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans when start metadata missing, got %d", len(spans))
	}
}

// TestTelemetryEdge_MissingToolStartMetadata tests tool after hook without before.
func TestTelemetryEdge_MissingToolStartMetadata(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Fire after tool hook WITHOUT before
	hookDataAfter := &core.HookData{
		Meta:       make(map[string]any),
		ToolName:   "orphan_tool",
		ToolResult: &typ.ToolResult{IsError: false},
	}

	err := hooks.Fire(ctx, core.HookAfterToolCall, hookDataAfter)
	if err != nil {
		t.Fatalf("Fire HookAfterToolCall with missing metadata returned error: %v", err)
	}

	metrics := tc.GetMetrics()
	// Tool call should still be counted
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}

	// But no span should be recorded
	spans := tc.GetSpans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans when start metadata missing, got %d", len(spans))
	}
}

// TestTelemetryEdge_ZeroDurationSpan tests a span with effectively zero duration.
func TestTelemetryEdge_ZeroDurationSpan(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Capture start time before firing
	startTime := time.Now()

	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)

	// Immediately fire after (no delay, potentially zero duration)
	hookDataAfter := &core.HookData{
		Meta:      hookDataBefore.Meta,
		TokensIn:  100,
		TokensOut: 50,
	}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 1 {
		t.Errorf("expected LLMCalls=1, got %d", metrics.LLMCalls)
	}

	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(spans))
	}

	// Duration should be >= 0 (even if very small)
	span := spans[0]
	if span.Duration < 0 {
		t.Errorf("expected non-negative duration, got %v", span.Duration)
	}

	// Start time should be reasonable
	if span.Start.Before(startTime) || span.Start.After(time.Now()) {
		t.Errorf("span start time %v is not within test execution window", span.Start)
	}
}

// TestTelemetryEdge_LargeTokenCounts tests very large token counts.
func TestTelemetryEdge_LargeTokenCounts(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	largeTokens := 1_000_000

	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)

	hookDataAfter := &core.HookData{
		Meta:      hookDataBefore.Meta,
		TokensIn:  largeTokens,
		TokensOut: largeTokens / 2,
	}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

	metrics := tc.GetMetrics()
	if metrics.TokensIn != largeTokens {
		t.Errorf("expected TokensIn=%d, got %d", largeTokens, metrics.TokensIn)
	}
	if metrics.TokensOut != largeTokens/2 {
		t.Errorf("expected TokensOut=%d, got %d", largeTokens/2, metrics.TokensOut)
	}

	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Meta["tokens_in"] != largeTokens {
		t.Errorf("expected span tokens_in=%d, got %v", largeTokens, spans[0].Meta["tokens_in"])
	}
}

// TestTelemetryEdge_AllErrorMetrics tests all error-tracking hooks together.
func TestTelemetryEdge_AllErrorMetrics(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Fire a tool error
	toolDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: "error_tool"}
	hooks.Fire(ctx, core.HookBeforeToolCall, toolDataBefore)
	toolDataAfter := &core.HookData{
		Meta:       toolDataBefore.Meta,
		ToolName:   "error_tool",
		ToolResult: &typ.ToolResult{IsError: true, Content: "Tool failed"},
	}
	hooks.Fire(ctx, core.HookAfterToolCall, toolDataAfter)

	// Fire an LLM error hook
	hooks.Fire(ctx, core.HookOnError, &core.HookData{})

	metrics := tc.GetMetrics()
	if metrics.ToolErrors != 1 {
		t.Errorf("expected ToolErrors=1, got %d", metrics.ToolErrors)
	}
	if metrics.LLMErrors != 1 {
		t.Errorf("expected LLMErrors=1, got %d", metrics.LLMErrors)
	}

	// Summary should reflect both errors
	summary := tc.Summary()
	if !strings.Contains(summary, "errors=1") {
		t.Errorf("summary missing tool error count: %s", summary)
	}
}

// TestTelemetryEdge_MultipleErrorsInSequence tests multiple error hooks in sequence.
func TestTelemetryEdge_MultipleErrorsInSequence(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Fire 5 error hooks
	for i := 0; i < 5; i++ {
		hooks.Fire(ctx, core.HookOnError, &core.HookData{})
	}

	metrics := tc.GetMetrics()
	if metrics.LLMErrors != 5 {
		t.Errorf("expected LLMErrors=5, got %d", metrics.LLMErrors)
	}
}

// TestTelemetryEdge_MixedErrorAndSuccessTools tests tool calls with mixed results.
func TestTelemetryEdge_MixedErrorAndSuccessTools(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	toolNames := []string{"tool_a", "tool_b", "tool_c", "tool_d", "tool_e"}
	errorIndices := []int{1, 3} // tools[1] and tools[3] will fail

	for i, toolName := range toolNames {
		toolDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: toolName}
		hooks.Fire(ctx, core.HookBeforeToolCall, toolDataBefore)

		isError := false
		for _, errIdx := range errorIndices {
			if i == errIdx {
				isError = true
				break
			}
		}

		toolDataAfter := &core.HookData{
			Meta:       toolDataBefore.Meta,
			ToolName:   toolName,
			ToolResult: &typ.ToolResult{IsError: isError},
		}
		hooks.Fire(ctx, core.HookAfterToolCall, toolDataAfter)
	}

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 5 {
		t.Errorf("expected ToolCalls=5, got %d", metrics.ToolCalls)
	}
	if metrics.ToolErrors != 2 {
		t.Errorf("expected ToolErrors=2, got %d", metrics.ToolErrors)
	}
}

// TestTelemetryEdge_CompactionAndSubagentTogether tests simultaneous non-error hooks.
func TestTelemetryEdge_CompactionAndSubagentTogether(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Fire multiple hook types in interleaved order
	for i := 0; i < 3; i++ {
		hooks.Fire(ctx, core.HookPostCompact, &core.HookData{})
		hooks.Fire(ctx, core.HookOnSubagentComplete, &core.HookData{})
		hooks.Fire(ctx, core.HookOnTurnEnd, &core.HookData{})
	}

	metrics := tc.GetMetrics()
	if metrics.Compactions != 3 {
		t.Errorf("expected Compactions=3, got %d", metrics.Compactions)
	}
	if metrics.SubAgents != 3 {
		t.Errorf("expected SubAgents=3, got %d", metrics.SubAgents)
	}
	if metrics.Turns != 3 {
		t.Errorf("expected Turns=3, got %d", metrics.Turns)
	}
}

// TestTelemetryEdge_SummaryFormatConsistency verifies Summary format remains consistent.
func TestTelemetryEdge_SummaryFormatConsistency(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Add diverse metrics
	for i := 0; i < 2; i++ {
		llmBefore := &core.HookData{Meta: make(map[string]any)}
		hooks.Fire(ctx, core.HookBeforeLLMCall, llmBefore)
		time.Sleep(2 * time.Millisecond)
		llmAfter := &core.HookData{Meta: llmBefore.Meta, TokensIn: 100, TokensOut: 50}
		hooks.Fire(ctx, core.HookAfterLLMCall, llmAfter)

		toolBefore := &core.HookData{Meta: make(map[string]any), ToolName: "test_tool"}
		hooks.Fire(ctx, core.HookBeforeToolCall, toolBefore)
		time.Sleep(1 * time.Millisecond)
		toolAfter := &core.HookData{
			Meta:       toolBefore.Meta,
			ToolName:   "test_tool",
			ToolResult: &typ.ToolResult{IsError: false},
		}
		hooks.Fire(ctx, core.HookAfterToolCall, toolAfter)
	}

	hooks.Fire(ctx, core.HookOnTurnEnd, &core.HookData{})
	hooks.Fire(ctx, core.HookOnError, &core.HookData{})
	hooks.Fire(ctx, core.HookPostCompact, &core.HookData{})
	hooks.Fire(ctx, core.HookOnSubagentComplete, &core.HookData{})

	summary := tc.Summary()

	// Check all expected fields are present and in correct format
	expectedPatterns := []string{
		"llm_calls=",
		"errors=",
		"avg=",
		"total=",
		"tool_calls=",
		"tokens_in=",
		"tokens_out=",
		"turns=",
		"compactions=",
		"subagents=",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(summary, pattern) {
			t.Errorf("summary missing expected pattern '%s': %s", pattern, summary)
		}
	}

	// Summary should be a single line (for logging purposes)
	if strings.Count(summary, "\n") > 0 {
		t.Errorf("summary should be single line, but contains newlines: %s", summary)
	}
}

// TestTelemetryEdge_ConcurrentMetricsAndSpans tests concurrent reads while writes occur.
func TestTelemetryEdge_ConcurrentMetricsAndSpans(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	done := make(chan bool, 20)

	// 10 goroutines writing metrics
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			for j := 0; j < 5; j++ {
				hookBefore := &core.HookData{Meta: make(map[string]any)}
				hooks.Fire(ctx, core.HookBeforeLLMCall, hookBefore)
				time.Sleep(time.Microsecond) // minimal delay
				hookAfter := &core.HookData{Meta: hookBefore.Meta, TokensIn: 10, TokensOut: 5}
				hooks.Fire(ctx, core.HookAfterLLMCall, hookAfter)
			}
		}(i)
	}

	// 10 goroutines reading metrics concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			for j := 0; j < 10; j++ {
				_ = tc.GetMetrics()
				_ = tc.GetSpans()
				_ = tc.Summary()
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Wait for all
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should have completed without deadlock or race
	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 50 {
		t.Errorf("expected LLMCalls=50, got %d", metrics.LLMCalls)
	}
	if metrics.TokensIn != 500 {
		t.Errorf("expected TokensIn=500, got %d", metrics.TokensIn)
	}

	spans := tc.GetSpans()
	if len(spans) != 50 {
		t.Errorf("expected 50 spans, got %d", len(spans))
	}
}

// TestTelemetryEdge_EmptyToolName tests tool span with empty tool name.
func TestTelemetryEdge_EmptyToolName(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	toolBefore := &core.HookData{Meta: make(map[string]any), ToolName: ""}
	hooks.Fire(ctx, core.HookBeforeToolCall, toolBefore)
	time.Sleep(1 * time.Millisecond)

	toolAfter := &core.HookData{
		Meta:       toolBefore.Meta,
		ToolName:   "",
		ToolResult: &typ.ToolResult{IsError: false},
	}
	hooks.Fire(ctx, core.HookAfterToolCall, toolAfter)

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}

	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(spans))
	}

	// Span name should be "tool." with empty suffix
	if spans[0].Name != "tool." {
		t.Errorf("expected span name 'tool.', got %q", spans[0].Name)
	}
}

// TestTelemetryEdge_ToolResultNil tests tool after hook with nil ToolResult.
func TestTelemetryEdge_ToolResultNil(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	toolBefore := &core.HookData{Meta: make(map[string]any), ToolName: "nil_result_tool"}
	hooks.Fire(ctx, core.HookBeforeToolCall, toolBefore)

	// Fire after with nil ToolResult
	toolAfter := &core.HookData{
		Meta:       toolBefore.Meta,
		ToolName:   "nil_result_tool",
		ToolResult: nil, // Edge case: nil result
	}

	err := hooks.Fire(ctx, core.HookAfterToolCall, toolAfter)
	if err != nil {
		t.Fatalf("Fire with nil ToolResult returned error: %v", err)
	}

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}
	// Should not count as error when result is nil (not explicitly marked as error)
	if metrics.ToolErrors != 0 {
		t.Errorf("expected ToolErrors=0 for nil result, got %d", metrics.ToolErrors)
	}
}

// TestTelemetryEdge_GetMetricsReturnsSnapshot verifies GetMetrics returns independent snapshot.
func TestTelemetryEdge_GetMetricsReturnsSnapshot(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Get initial metrics
	metrics1 := tc.GetMetrics()
	if metrics1.LLMCalls != 0 {
		t.Fatalf("initial metrics should be zero")
	}

	// Add an LLM call
	hookBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookBefore)
	hookAfter := &core.HookData{Meta: hookBefore.Meta, TokensIn: 100, TokensOut: 50}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookAfter)

	// Original metrics object should not change (it's a snapshot)
	if metrics1.LLMCalls != 0 {
		t.Errorf("old metrics snapshot should not be affected by new calls")
	}

	// New call should show the update
	metrics2 := tc.GetMetrics()
	if metrics2.LLMCalls != 1 {
		t.Errorf("new metrics should reflect the LLM call, got %d", metrics2.LLMCalls)
	}
}

// TestTelemetryEdge_SpanStartTimesAreMonotonic verifies span start times increase.
func TestTelemetryEdge_SpanStartTimesAreMonotonic(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)
	ctx := context.Background()

	// Create 5 spans in sequence
	for i := 0; i < 5; i++ {
		hookBefore := &core.HookData{Meta: make(map[string]any)}
		hooks.Fire(ctx, core.HookBeforeLLMCall, hookBefore)
		time.Sleep(1 * time.Millisecond)
		hookAfter := &core.HookData{Meta: hookBefore.Meta, TokensIn: 100, TokensOut: 50}
		hooks.Fire(ctx, core.HookAfterLLMCall, hookAfter)
	}

	spans := tc.GetSpans()
	if len(spans) != 5 {
		t.Fatalf("expected 5 spans, got %d", len(spans))
	}

	// Verify monotonic ordering
	for i := 1; i < len(spans); i++ {
		if spans[i].Start.Before(spans[i-1].Start) {
			t.Errorf("span %d start time (%v) is before span %d (%v)", i, spans[i].Start, i-1, spans[i-1].Start)
		}
	}
}
