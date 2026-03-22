package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"torus_go_agent/internal/core"
	typ "torus_go_agent/internal/types"
)

// TestNewTelemetryCollector verifies the collector is initialized correctly.
func TestNewTelemetryCollector(t *testing.T) {
	tc := NewTelemetryCollector()
	if tc == nil {
		t.Fatal("NewTelemetryCollector returned nil")
	}
	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 0 || metrics.ToolCalls != 0 || metrics.Turns != 0 {
		t.Fatalf("expected zero metrics, got %+v", metrics)
	}
}

// TestGetMetricsInitialState verifies initial metrics are zero-valued.
func TestGetMetricsInitialState(t *testing.T) {
	tc := NewTelemetryCollector()
	metrics := tc.GetMetrics()

	if metrics.LLMCalls != 0 {
		t.Errorf("expected LLMCalls=0, got %d", metrics.LLMCalls)
	}
	if metrics.LLMErrors != 0 {
		t.Errorf("expected LLMErrors=0, got %d", metrics.LLMErrors)
	}
	if metrics.ToolCalls != 0 {
		t.Errorf("expected ToolCalls=0, got %d", metrics.ToolCalls)
	}
	if metrics.ToolErrors != 0 {
		t.Errorf("expected ToolErrors=0, got %d", metrics.ToolErrors)
	}
	if metrics.TokensIn != 0 {
		t.Errorf("expected TokensIn=0, got %d", metrics.TokensIn)
	}
	if metrics.TokensOut != 0 {
		t.Errorf("expected TokensOut=0, got %d", metrics.TokensOut)
	}
	if metrics.Turns != 0 {
		t.Errorf("expected Turns=0, got %d", metrics.Turns)
	}
	if metrics.Compactions != 0 {
		t.Errorf("expected Compactions=0, got %d", metrics.Compactions)
	}
	if metrics.SubAgents != 0 {
		t.Errorf("expected SubAgents=0, got %d", metrics.SubAgents)
	}
	if metrics.TotalLLMTime != 0 {
		t.Errorf("expected TotalLLMTime=0, got %v", metrics.TotalLLMTime)
	}
	if metrics.TotalToolTime != 0 {
		t.Errorf("expected TotalToolTime=0, got %v", metrics.TotalToolTime)
	}
}

// TestGetSpansInitialState verifies initial spans list is empty.
func TestGetSpansInitialState(t *testing.T) {
	tc := NewTelemetryCollector()
	spans := tc.GetSpans()
	if spans == nil {
		t.Error("expected non-nil spans slice")
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
}

// TestSummaryEmptyMetrics verifies summary format with no activity.
func TestSummaryEmptyMetrics(t *testing.T) {
	tc := NewTelemetryCollector()
	summary := tc.Summary()

	expectedFields := []string{
		"llm_calls=0",
		"errors=0",
		"tool_calls=0",
		"tokens_in=0",
		"tokens_out=0",
		"turns=0",
		"compactions=0",
		"subagents=0",
	}
	for _, field := range expectedFields {
		if !strings.Contains(summary, field) {
			t.Errorf("summary missing field '%s': %s", field, summary)
		}
	}
}

// TestRegisterHooksBeforeLLMCall verifies LLM timing hooks are registered and triggered.
func TestRegisterHooksBeforeLLMCall(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	// Verify hooks are registered
	if hooks.Count(core.HookBeforeLLMCall) == 0 {
		t.Fatal("HookBeforeLLMCall not registered")
	}
	if hooks.Count(core.HookAfterLLMCall) == 0 {
		t.Fatal("HookAfterLLMCall not registered")
	}

	// Fire the before hook
	ctx := context.Background()
	hookData := &core.HookData{Meta: make(map[string]any)}
	err := hooks.Fire(ctx, core.HookBeforeLLMCall, hookData)
	if err != nil {
		t.Fatalf("Fire HookBeforeLLMCall returned error: %v", err)
	}

	// Verify timestamp was set
	if _, ok := hookData.Meta["_telem_llm_start"]; !ok {
		t.Error("_telem_llm_start not set in Meta")
	}
}

// TestRegisterHooksAfterLLMCall verifies LLM metrics are tracked.
func TestRegisterHooksAfterLLMCall(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Fire before hook to set start time
	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	err := hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
	if err != nil {
		t.Fatalf("Fire HookBeforeLLMCall returned error: %v", err)
	}

	// Small delay to ensure measurable duration
	time.Sleep(10 * time.Millisecond)

	// Fire after hook with tokens
	hookDataAfter := &core.HookData{
		Meta:      hookDataBefore.Meta,
		TokensIn:  150,
		TokensOut: 50,
	}
	err = hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)
	if err != nil {
		t.Fatalf("Fire HookAfterLLMCall returned error: %v", err)
	}

	// Verify metrics were updated
	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 1 {
		t.Errorf("expected LLMCalls=1, got %d", metrics.LLMCalls)
	}
	if metrics.TokensIn != 150 {
		t.Errorf("expected TokensIn=150, got %d", metrics.TokensIn)
	}
	if metrics.TokensOut != 50 {
		t.Errorf("expected TokensOut=50, got %d", metrics.TokensOut)
	}
	if metrics.TotalLLMTime <= 0 {
		t.Errorf("expected positive TotalLLMTime, got %v", metrics.TotalLLMTime)
	}

	// Verify span was recorded
	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "llm.call" {
		t.Errorf("expected span name 'llm.call', got %q", spans[0].Name)
	}
	if spans[0].Duration <= 0 {
		t.Errorf("expected positive span duration, got %v", spans[0].Duration)
	}
}

// TestRegisterHooksBeforeToolCall verifies tool timing start hook.
func TestRegisterHooksBeforeToolCall(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	if hooks.Count(core.HookBeforeToolCall) == 0 {
		t.Fatal("HookBeforeToolCall not registered")
	}

	ctx := context.Background()
	hookData := &core.HookData{Meta: make(map[string]any)}
	err := hooks.Fire(ctx, core.HookBeforeToolCall, hookData)
	if err != nil {
		t.Fatalf("Fire HookBeforeToolCall returned error: %v", err)
	}

	if _, ok := hookData.Meta["_telem_tool_start"]; !ok {
		t.Error("_telem_tool_start not set in Meta")
	}
}

// TestRegisterHooksAfterToolCall verifies tool call metrics are tracked.
func TestRegisterHooksAfterToolCall(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Fire before hook
	hookDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: "test_tool"}
	err := hooks.Fire(ctx, core.HookBeforeToolCall, hookDataBefore)
	if err != nil {
		t.Fatalf("Fire HookBeforeToolCall returned error: %v", err)
	}

	// Small delay
	time.Sleep(10 * time.Millisecond)

	// Fire after hook without error
	hookDataAfter := &core.HookData{
		Meta:       hookDataBefore.Meta,
		ToolName:   "test_tool",
		ToolResult: &typ.ToolResult{IsError: false},
	}
	err = hooks.Fire(ctx, core.HookAfterToolCall, hookDataAfter)
	if err != nil {
		t.Fatalf("Fire HookAfterToolCall returned error: %v", err)
	}

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}
	if metrics.ToolErrors != 0 {
		t.Errorf("expected ToolErrors=0, got %d", metrics.ToolErrors)
	}

	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "tool.test_tool" {
		t.Errorf("expected span name 'tool.test_tool', got %q", spans[0].Name)
	}
}

// TestRegisterHooksAfterToolCallWithError verifies tool error tracking.
func TestRegisterHooksAfterToolCallWithError(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Fire before hook
	hookDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: "failing_tool"}
	err := hooks.Fire(ctx, core.HookBeforeToolCall, hookDataBefore)
	if err != nil {
		t.Fatalf("Fire HookBeforeToolCall returned error: %v", err)
	}

	// Fire after hook with error
	hookDataAfter := &core.HookData{
		Meta:       hookDataBefore.Meta,
		ToolName:   "failing_tool",
		ToolResult: &typ.ToolResult{IsError: true, Content: "Tool failed"},
	}
	err = hooks.Fire(ctx, core.HookAfterToolCall, hookDataAfter)
	if err != nil {
		t.Fatalf("Fire HookAfterToolCall returned error: %v", err)
	}

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 1 {
		t.Errorf("expected ToolCalls=1, got %d", metrics.ToolCalls)
	}
	if metrics.ToolErrors != 1 {
		t.Errorf("expected ToolErrors=1, got %d", metrics.ToolErrors)
	}
}

// TestRegisterHooksOnTurnEnd verifies turn counting.
func TestRegisterHooksOnTurnEnd(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	if hooks.Count(core.HookOnTurnEnd) == 0 {
		t.Fatal("HookOnTurnEnd not registered")
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		hookData := &core.HookData{}
		err := hooks.Fire(ctx, core.HookOnTurnEnd, hookData)
		if err != nil {
			t.Fatalf("Fire HookOnTurnEnd returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.Turns != 3 {
		t.Errorf("expected Turns=3, got %d", metrics.Turns)
	}
}

// TestRegisterHooksOnError verifies error counting.
func TestRegisterHooksOnError(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	if hooks.Count(core.HookOnError) == 0 {
		t.Fatal("HookOnError not registered")
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		hookData := &core.HookData{}
		err := hooks.Fire(ctx, core.HookOnError, hookData)
		if err != nil {
			t.Fatalf("Fire HookOnError returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.LLMErrors != 2 {
		t.Errorf("expected LLMErrors=2, got %d", metrics.LLMErrors)
	}
}

// TestRegisterHooksPostCompact verifies compaction counting.
func TestRegisterHooksPostCompact(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	if hooks.Count(core.HookPostCompact) == 0 {
		t.Fatal("HookPostCompact not registered")
	}

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		hookData := &core.HookData{}
		err := hooks.Fire(ctx, core.HookPostCompact, hookData)
		if err != nil {
			t.Fatalf("Fire HookPostCompact returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.Compactions != 4 {
		t.Errorf("expected Compactions=4, got %d", metrics.Compactions)
	}
}

// TestRegisterHooksOnSubagentComplete verifies subagent counting.
func TestRegisterHooksOnSubagentComplete(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	if hooks.Count(core.HookOnSubagentComplete) == 0 {
		t.Fatal("HookOnSubagentComplete not registered")
	}

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		hookData := &core.HookData{}
		err := hooks.Fire(ctx, core.HookOnSubagentComplete, hookData)
		if err != nil {
			t.Fatalf("Fire HookOnSubagentComplete returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.SubAgents != 5 {
		t.Errorf("expected SubAgents=5, got %d", metrics.SubAgents)
	}
}

// TestMultipleLLMCalls verifies accumulation across multiple LLM calls.
func TestMultipleLLMCalls(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Simulate 3 LLM calls
	for i := 0; i < 3; i++ {
		hookDataBefore := &core.HookData{Meta: make(map[string]any)}
		err := hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
		if err != nil {
			t.Fatalf("Fire HookBeforeLLMCall returned error: %v", err)
		}

		time.Sleep(5 * time.Millisecond)

		hookDataAfter := &core.HookData{
			Meta:      hookDataBefore.Meta,
			TokensIn:  100 + i*10,
			TokensOut: 50 + i*5,
		}
		err = hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)
		if err != nil {
			t.Fatalf("Fire HookAfterLLMCall returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 3 {
		t.Errorf("expected LLMCalls=3, got %d", metrics.LLMCalls)
	}
	expectedTokensIn := 100 + 110 + 120 // 330
	if metrics.TokensIn != expectedTokensIn {
		t.Errorf("expected TokensIn=%d, got %d", expectedTokensIn, metrics.TokensIn)
	}
	expectedTokensOut := 50 + 55 + 60 // 165
	if metrics.TokensOut != expectedTokensOut {
		t.Errorf("expected TokensOut=%d, got %d", expectedTokensOut, metrics.TokensOut)
	}

	spans := tc.GetSpans()
	if len(spans) != 3 {
		t.Errorf("expected 3 spans, got %d", len(spans))
	}
}

// TestMultipleToolCalls verifies accumulation across multiple tool calls.
func TestMultipleToolCalls(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Simulate 2 successful and 1 failed tool call
	for i := 0; i < 3; i++ {
		hookDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: fmt.Sprintf("tool_%d", i)}
		err := hooks.Fire(ctx, core.HookBeforeToolCall, hookDataBefore)
		if err != nil {
			t.Fatalf("Fire HookBeforeToolCall returned error: %v", err)
		}

		time.Sleep(5 * time.Millisecond)

		isError := i == 2 // Last one fails
		hookDataAfter := &core.HookData{
			Meta:       hookDataBefore.Meta,
			ToolName:   hookDataBefore.ToolName,
			ToolResult: &typ.ToolResult{IsError: isError},
		}
		err = hooks.Fire(ctx, core.HookAfterToolCall, hookDataAfter)
		if err != nil {
			t.Fatalf("Fire HookAfterToolCall returned error: %v", err)
		}
	}

	metrics := tc.GetMetrics()
	if metrics.ToolCalls != 3 {
		t.Errorf("expected ToolCalls=3, got %d", metrics.ToolCalls)
	}
	if metrics.ToolErrors != 1 {
		t.Errorf("expected ToolErrors=1, got %d", metrics.ToolErrors)
	}

	spans := tc.GetSpans()
	if len(spans) != 3 {
		t.Errorf("expected 3 spans, got %d", len(spans))
	}
}

// TestSummaryWithMetrics verifies summary includes actual metrics.
func TestSummaryWithMetrics(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Add some metrics
	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
	time.Sleep(5 * time.Millisecond)
	hookDataAfter := &core.HookData{Meta: hookDataBefore.Meta, TokensIn: 200, TokensOut: 75}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

	// Add turns
	for i := 0; i < 2; i++ {
		hooks.Fire(ctx, core.HookOnTurnEnd, &core.HookData{})
	}

	summary := tc.Summary()

	if !strings.Contains(summary, "llm_calls=1") {
		t.Errorf("summary missing 'llm_calls=1': %s", summary)
	}
	if !strings.Contains(summary, "tokens_in=200") {
		t.Errorf("summary missing 'tokens_in=200': %s", summary)
	}
	if !strings.Contains(summary, "tokens_out=75") {
		t.Errorf("summary missing 'tokens_out=75': %s", summary)
	}
	if !strings.Contains(summary, "turns=2") {
		t.Errorf("summary missing 'turns=2': %s", summary)
	}
}

// TestConcurrentAccess verifies goroutine-safety of the collector.
func TestConcurrentAccess(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()
	done := make(chan bool, 10)

	// Spawn 10 goroutines firing hooks concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			// Fire LLM hooks
			hookDataBefore := &core.HookData{Meta: make(map[string]any)}
			hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
			time.Sleep(1 * time.Millisecond)
			hookDataAfter := &core.HookData{Meta: hookDataBefore.Meta, TokensIn: 100, TokensOut: 50}
			hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

			// Fire turn end hook
			hooks.Fire(ctx, core.HookOnTurnEnd, &core.HookData{})
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify metrics (order may vary but counts should be exact)
	metrics := tc.GetMetrics()
	if metrics.LLMCalls != 10 {
		t.Errorf("expected LLMCalls=10, got %d", metrics.LLMCalls)
	}
	if metrics.TokensIn != 1000 {
		t.Errorf("expected TokensIn=1000, got %d", metrics.TokensIn)
	}
	if metrics.Turns != 10 {
		t.Errorf("expected Turns=10, got %d", metrics.Turns)
	}
}

// TestGetSpansMakesACopy verifies that GetSpans returns a copy, not a reference.
func TestGetSpansMakesACopy(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Create a span
	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
	time.Sleep(5 * time.Millisecond)
	hookDataAfter := &core.HookData{Meta: hookDataBefore.Meta, TokensIn: 100, TokensOut: 50}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

	// Get spans and modify the returned slice
	spans1 := tc.GetSpans()
	if len(spans1) == 0 {
		t.Fatal("expected at least one span")
	}

	// Add a new span via the collector
	hookDataBefore2 := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore2)
	time.Sleep(5 * time.Millisecond)
	hookDataAfter2 := &core.HookData{Meta: hookDataBefore2.Meta, TokensIn: 100, TokensOut: 50}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter2)

	// The original spans slice should not have the new span
	if len(spans1) != 1 {
		t.Errorf("expected original spans to still be length 1, got %d", len(spans1))
	}

	// New call to GetSpans should include both
	spans2 := tc.GetSpans()
	if len(spans2) != 2 {
		t.Errorf("expected 2 spans in new call, got %d", len(spans2))
	}
}

// TestSpanMetadata verifies that span metadata is correctly captured.
func TestSpanMetadata(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Fire LLM hooks with specific token values
	hookDataBefore := &core.HookData{Meta: make(map[string]any)}
	hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
	time.Sleep(5 * time.Millisecond)
	hookDataAfter := &core.HookData{
		Meta:      hookDataBefore.Meta,
		TokensIn:  250,
		TokensOut: 100,
	}
	hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)

	spans := tc.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Meta["tokens_in"] != 250 {
		t.Errorf("expected tokens_in=250, got %v", span.Meta["tokens_in"])
	}
	if span.Meta["tokens_out"] != 100 {
		t.Errorf("expected tokens_out=100, got %v", span.Meta["tokens_out"])
	}
}

// TestToolSpanNames verifies tool spans have correct names.
func TestToolSpanNames(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	tools := []string{"create_file", "read_file", "execute_bash"}

	for _, toolName := range tools {
		hookDataBefore := &core.HookData{Meta: make(map[string]any), ToolName: toolName}
		hooks.Fire(ctx, core.HookBeforeToolCall, hookDataBefore)
		time.Sleep(1 * time.Millisecond)
		hookDataAfter := &core.HookData{
			Meta:       hookDataBefore.Meta,
			ToolName:   toolName,
			ToolResult: &typ.ToolResult{IsError: false},
		}
		hooks.Fire(ctx, core.HookAfterToolCall, hookDataAfter)
	}

	spans := tc.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for i, toolName := range tools {
		expectedSpanName := "tool." + toolName
		if spans[i].Name != expectedSpanName {
			t.Errorf("expected span %d name %q, got %q", i, expectedSpanName, spans[i].Name)
		}
	}
}

// TestAverageLLMTimeCalculation verifies average LLM time in summary.
func TestAverageLLMTimeCalculation(t *testing.T) {
	tc := NewTelemetryCollector()
	hooks := core.NewHookRegistry()
	tc.RegisterHooks(hooks)

	ctx := context.Background()

	// Create 2 LLM calls with known delays
	for i := 0; i < 2; i++ {
		hookDataBefore := &core.HookData{Meta: make(map[string]any)}
		hooks.Fire(ctx, core.HookBeforeLLMCall, hookDataBefore)
		time.Sleep(20 * time.Millisecond) // Intentionally longer for averaging
		hookDataAfter := &core.HookData{Meta: hookDataBefore.Meta, TokensIn: 100, TokensOut: 50}
		hooks.Fire(ctx, core.HookAfterLLMCall, hookDataAfter)
	}

	summary := tc.Summary()

	// The summary should include average time calculation
	if !strings.Contains(summary, "avg=") {
		t.Errorf("summary missing average time: %s", summary)
	}

	// Verify the format is reasonable
	metrics := tc.GetMetrics()
	if metrics.LLMCalls == 0 {
		t.Fatal("no LLM calls recorded")
	}
	if metrics.TotalLLMTime == 0 {
		t.Fatal("no LLM time recorded")
	}
}
