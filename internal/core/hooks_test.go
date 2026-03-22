package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"torus_go_agent/internal/types"
)

// TestNewHookRegistry verifies registry initialization.
func TestNewHookRegistry(t *testing.T) {
	reg := NewHookRegistry()
	if reg == nil {
		t.Fatal("NewHookRegistry returned nil")
	}
	if reg.hooks == nil {
		t.Fatal("hooks map not initialized")
	}
	if len(reg.hooks) != 0 {
		t.Fatalf("expected empty hooks map, got %d entries", len(reg.hooks))
	}
}

// TestRegisterSingle verifies registering a single handler.
func TestRegisterSingle(t *testing.T) {
	reg := NewHookRegistry()
	called := false
	fn := func(ctx context.Context, data *HookData) error {
		called = true
		return nil
	}

	reg.Register(HookBeforeLLMCall, "test_handler", fn)

	if reg.Count(HookBeforeLLMCall) != 1 {
		t.Fatalf("expected 1 handler, got %d", reg.Count(HookBeforeLLMCall))
	}

	// Fire the hook to verify handler works
	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

// TestRegisterMultiple verifies registering multiple handlers at the same point.
func TestRegisterMultiple(t *testing.T) {
	reg := NewHookRegistry()
	point := HookAfterLLMCall

	fn1 := func(ctx context.Context, data *HookData) error { return nil }
	fn2 := func(ctx context.Context, data *HookData) error { return nil }
	fn3 := func(ctx context.Context, data *HookData) error { return nil }

	reg.Register(point, "handler_1", fn1)
	reg.Register(point, "handler_2", fn2)
	reg.Register(point, "handler_3", fn3)

	if reg.Count(point) != 3 {
		t.Fatalf("expected 3 handlers, got %d", reg.Count(point))
	}
}

// TestRegisterMultiplePoints verifies registering handlers at different points.
func TestRegisterMultiplePoints(t *testing.T) {
	reg := NewHookRegistry()
	fn := func(ctx context.Context, data *HookData) error { return nil }

	reg.Register(HookBeforeLLMCall, "handler_1", fn)
	reg.Register(HookAfterLLMCall, "handler_2", fn)
	reg.Register(HookBeforeToolCall, "handler_3", fn)

	if reg.Count(HookBeforeLLMCall) != 1 {
		t.Fatalf("HookBeforeLLMCall: expected 1, got %d", reg.Count(HookBeforeLLMCall))
	}
	if reg.Count(HookAfterLLMCall) != 1 {
		t.Fatalf("HookAfterLLMCall: expected 1, got %d", reg.Count(HookAfterLLMCall))
	}
	if reg.Count(HookBeforeToolCall) != 1 {
		t.Fatalf("HookBeforeToolCall: expected 1, got %d", reg.Count(HookBeforeToolCall))
	}
}

// TestFireCallsHandlersInOrder verifies handlers are called in registration order with default priority.
func TestFireCallsHandlersInOrder(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "first", fn1)
	reg.Register(HookBeforeLLMCall, "second", fn2)
	reg.Register(HookBeforeLLMCall, "third", fn3)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("handlers not called in order: %v", callOrder)
	}
}

// TestFirePassesHookData verifies HookData is correctly passed to handlers.
func TestFirePassesHookData(t *testing.T) {
	reg := NewHookRegistry()
	var receivedData *HookData

	fn := func(ctx context.Context, data *HookData) error {
		receivedData = data
		return nil
	}

	reg.Register(HookBeforeLLMCall, "test", fn)

	originalData := &HookData{
		Point:       HookBeforeLLMCall,
		AgentID:     "agent123",
		ToolName:    "search",
		ToolArgs:    map[string]any{"query": "test"},
		TokensIn:    100,
		TokensOut:   50,
		Block:       false,
		BlockReason: "",
		Meta:        map[string]any{"key": "value"},
	}

	err := reg.Fire(context.Background(), HookBeforeLLMCall, originalData)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if receivedData == nil {
		t.Fatal("HookData not passed to handler")
	}
	if receivedData.AgentID != "agent123" {
		t.Fatalf("AgentID mismatch: expected 'agent123', got '%s'", receivedData.AgentID)
	}
	if receivedData.ToolName != "search" {
		t.Fatalf("ToolName mismatch: expected 'search', got '%s'", receivedData.ToolName)
	}
	if receivedData.TokensIn != 100 {
		t.Fatalf("TokensIn mismatch: expected 100, got %d", receivedData.TokensIn)
	}
	if receivedData.TokensOut != 50 {
		t.Fatalf("TokensOut mismatch: expected 50, got %d", receivedData.TokensOut)
	}
}

// TestFireSetsHookPoint verifies Fire sets the Point field.
func TestFireSetsHookPoint(t *testing.T) {
	reg := NewHookRegistry()
	var receivedPoint HookPoint

	fn := func(ctx context.Context, data *HookData) error {
		receivedPoint = data.Point
		return nil
	}

	reg.Register(HookAfterToolCall, "test", fn)

	data := &HookData{}
	err := reg.Fire(context.Background(), HookAfterToolCall, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if receivedPoint != HookAfterToolCall {
		t.Fatalf("Point not set: expected %q, got %q", HookAfterToolCall, receivedPoint)
	}
}

// TestFireHandlerError verifies Fire returns error from handler.
func TestFireHandlerError(t *testing.T) {
	reg := NewHookRegistry()
	expectedErr := errors.New("handler error")

	fn := func(ctx context.Context, data *HookData) error {
		return expectedErr
	}

	reg.Register(HookBeforeLLMCall, "failing", fn)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err == nil {
		t.Fatal("Fire should return error from handler")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

// TestFireStopsOnBlockFlag verifies Fire stops when Block is set.
func TestFireStopsOnBlockFlag(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		data.Block = true
		data.BlockReason = "stopped by handler 1"
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "blocker", fn1)
	reg.Register(HookBeforeLLMCall, "second", fn2)
	reg.Register(HookBeforeLLMCall, "third", fn3)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if len(callOrder) != 1 {
		t.Fatalf("expected 1 call (should stop), got %d calls: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != 1 {
		t.Fatalf("expected first handler to run, got %v", callOrder)
	}
}

// TestFirePreservesBlockData verifies BlockReason is preserved after Block is set.
func TestFirePreservesBlockData(t *testing.T) {
	reg := NewHookRegistry()
	var receivedData *HookData

	fn1 := func(ctx context.Context, data *HookData) error {
		data.Block = true
		data.BlockReason = "custom block reason"
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		receivedData = data
		return nil
	}

	reg.Register(HookBeforeLLMCall, "blocker", fn1)
	reg.Register(HookBeforeLLMCall, "observer", fn2)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Since Block stops Fire, the second handler never runs, so receivedData remains nil
	if receivedData != nil {
		t.Fatal("second handler should not be called when Block is set")
	}
}

// TestRegisterPriority verifies handlers are called in priority order (lower first).
func TestRegisterPriority(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	// Register in reverse order with explicit priorities
	reg.RegisterPriority(HookBeforeLLMCall, "high_priority", fn1, 10)
	reg.RegisterPriority(HookBeforeLLMCall, "medium_priority", fn2, 50)
	reg.RegisterPriority(HookBeforeLLMCall, "low_priority", fn3, 100)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("handlers not called in priority order: %v", callOrder)
	}
}

// TestRegisterPriorityMixed verifies mixed registration order still respects priority.
func TestRegisterPriorityMixed(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	// Register with mixed priorities in random order
	reg.RegisterPriority(HookBeforeLLMCall, "handler_2", fn2, 50)
	reg.RegisterPriority(HookBeforeLLMCall, "handler_1", fn1, 10)
	reg.RegisterPriority(HookBeforeLLMCall, "handler_3", fn3, 100)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("expected [1,2,3], got %v", callOrder)
	}
}

// TestCount verifies Count returns correct handler count.
func TestCount(t *testing.T) {
	reg := NewHookRegistry()

	fn := func(ctx context.Context, data *HookData) error { return nil }

	if reg.Count(HookBeforeLLMCall) != 0 {
		t.Fatalf("expected 0 handlers initially, got %d", reg.Count(HookBeforeLLMCall))
	}

	reg.Register(HookBeforeLLMCall, "h1", fn)
	if reg.Count(HookBeforeLLMCall) != 1 {
		t.Fatalf("expected 1 handler, got %d", reg.Count(HookBeforeLLMCall))
	}

	reg.Register(HookBeforeLLMCall, "h2", fn)
	if reg.Count(HookBeforeLLMCall) != 2 {
		t.Fatalf("expected 2 handlers, got %d", reg.Count(HookBeforeLLMCall))
	}

	reg.Register(HookBeforeLLMCall, "h3", fn)
	if reg.Count(HookBeforeLLMCall) != 3 {
		t.Fatalf("expected 3 handlers, got %d", reg.Count(HookBeforeLLMCall))
	}
}

// TestCountDifferentPoints verifies Count is independent per point.
func TestCountDifferentPoints(t *testing.T) {
	reg := NewHookRegistry()
	fn := func(ctx context.Context, data *HookData) error { return nil }

	reg.Register(HookBeforeLLMCall, "h1", fn)
	reg.Register(HookBeforeLLMCall, "h2", fn)
	reg.Register(HookAfterLLMCall, "h3", fn)

	if reg.Count(HookBeforeLLMCall) != 2 {
		t.Fatalf("HookBeforeLLMCall: expected 2, got %d", reg.Count(HookBeforeLLMCall))
	}
	if reg.Count(HookAfterLLMCall) != 1 {
		t.Fatalf("HookAfterLLMCall: expected 1, got %d", reg.Count(HookAfterLLMCall))
	}
}

// TestFireNoHandlers verifies Fire succeeds with no handlers registered.
func TestFireNoHandlers(t *testing.T) {
	reg := NewHookRegistry()
	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire should not error with no handlers, got: %v", err)
	}
}

// TestFireContextPassing verifies context is passed correctly to handlers.
func TestFireContextPassing(t *testing.T) {
	reg := NewHookRegistry()
	var receivedCtx context.Context

	fn := func(ctx context.Context, data *HookData) error {
		receivedCtx = ctx
		return nil
	}

	reg.Register(HookBeforeLLMCall, "test", fn)

	testCtx := context.WithValue(context.Background(), "test_key", "test_value")
	err := reg.Fire(testCtx, HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if receivedCtx.Value("test_key") != "test_value" {
		t.Fatal("context value not passed correctly")
	}
}

// TestFireWithToolResult verifies ToolResult is passed correctly.
func TestFireWithToolResult(t *testing.T) {
	reg := NewHookRegistry()
	var receivedResult *types.ToolResult

	fn := func(ctx context.Context, data *HookData) error {
		receivedResult = data.ToolResult
		return nil
	}

	reg.Register(HookAfterToolResult, "test", fn)

	toolResult := &types.ToolResult{
		ToolUseID: "tool_123",
		Content:   "result content",
		IsError:   false,
	}

	data := &HookData{ToolResult: toolResult}
	err := reg.Fire(context.Background(), HookAfterToolResult, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if receivedResult == nil {
		t.Fatal("ToolResult not passed")
	}
	if receivedResult.ToolUseID != "tool_123" {
		t.Fatalf("ToolUseID mismatch: expected 'tool_123', got '%s'", receivedResult.ToolUseID)
	}
	if receivedResult.Content != "result content" {
		t.Fatalf("Content mismatch: expected 'result content', got '%s'", receivedResult.Content)
	}
}

// TestFireWithMessages verifies Messages slice is passed correctly.
func TestFireWithMessages(t *testing.T) {
	reg := NewHookRegistry()
	var receivedMessages []types.Message

	fn := func(ctx context.Context, data *HookData) error {
		receivedMessages = data.Messages
		return nil
	}

	reg.Register(HookBeforeContextBuild, "test", fn)

	messages := []types.Message{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "hi"}}},
	}

	data := &HookData{Messages: messages}
	err := reg.Fire(context.Background(), HookBeforeContextBuild, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if len(receivedMessages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(receivedMessages))
	}
	if len(receivedMessages[0].Content) != 1 || receivedMessages[0].Content[0].Text != "hello" {
		t.Fatalf("first message mismatch: expected 'hello'")
	}
}

// TestConcurrentRegister verifies concurrent Register calls are safe.
func TestConcurrentRegister(t *testing.T) {
	reg := NewHookRegistry()
	fn := func(ctx context.Context, data *HookData) error { return nil }

	// Launch concurrent registrations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			reg.Register(HookBeforeLLMCall, "handler"+string(rune(id)), fn)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if reg.Count(HookBeforeLLMCall) != 10 {
		t.Fatalf("expected 10 handlers, got %d", reg.Count(HookBeforeLLMCall))
	}
}

// TestConcurrentFire verifies concurrent Fire calls are safe.
func TestConcurrentFire(t *testing.T) {
	reg := NewHookRegistry()
	var callCount int64

	fn := func(ctx context.Context, data *HookData) error {
		atomic.AddInt64(&callCount, 1)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "handler", fn)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
			if err != nil {
				t.Errorf("Fire returned error: %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if atomic.LoadInt64(&callCount) != 10 {
		t.Fatalf("expected 10 calls, got %d", atomic.LoadInt64(&callCount))
	}
}

// TestMultipleHookPoints verifies all major hook points work.
func TestMultipleHookPoints(t *testing.T) {
	reg := NewHookRegistry()
	fn := func(ctx context.Context, data *HookData) error { return nil }

	points := []HookPoint{
		HookBeforeLLMCall,
		HookAfterLLMCall,
		HookBeforeToolCall,
		HookAfterToolCall,
		HookAfterToolResult,
		HookBeforeContextBuild,
		HookAfterContextBuild,
		HookOnError,
		HookOnAgentStart,
		HookOnAgentEnd,
	}

	for _, point := range points {
		reg.Register(point, "test", fn)
	}

	for _, point := range points {
		if reg.Count(point) != 1 {
			t.Fatalf("point %q: expected 1 handler, got %d", point, reg.Count(point))
		}
		err := reg.Fire(context.Background(), point, &HookData{})
		if err != nil {
			t.Fatalf("Fire for point %q returned error: %v", point, err)
		}
	}
}

// TestFireBlockWithNoError verifies Block stops execution without error.
func TestFireBlockWithNoError(t *testing.T) {
	reg := NewHookRegistry()

	fn1 := func(ctx context.Context, data *HookData) error {
		data.Block = true
		return nil
	}
	fn2Called := false
	fn2 := func(ctx context.Context, data *HookData) error {
		fn2Called = true
		return nil
	}

	reg.Register(HookBeforeLLMCall, "blocker", fn1)
	reg.Register(HookBeforeLLMCall, "second", fn2)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire should not error on Block, got: %v", err)
	}
	if fn2Called {
		t.Fatal("second handler should not be called when Block is set")
	}
}

// TestHandlerModifiesMetadata verifies handlers can modify HookData.Meta.
func TestHandlerModifiesMetadata(t *testing.T) {
	reg := NewHookRegistry()

	fn1 := func(ctx context.Context, data *HookData) error {
		if data.Meta == nil {
			data.Meta = make(map[string]any)
		}
		data.Meta["modified"] = true
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		data.Meta["extra"] = "value"
		return nil
	}

	reg.Register(HookBeforeLLMCall, "modifier_1", fn1)
	reg.Register(HookBeforeLLMCall, "modifier_2", fn2)

	data := &HookData{}
	err := reg.Fire(context.Background(), HookBeforeLLMCall, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if data.Meta["modified"] != true {
		t.Fatal("Meta['modified'] not set by first handler")
	}
	if data.Meta["extra"] != "value" {
		t.Fatal("Meta['extra'] not set by second handler")
	}
}

// TestPriorityNegative verifies negative priorities work.
func TestPriorityNegative(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}

	reg.RegisterPriority(HookBeforeLLMCall, "negative", fn1, -100)
	reg.RegisterPriority(HookBeforeLLMCall, "positive", fn2, 100)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if callOrder[0] != 1 {
		t.Fatalf("negative priority should run first, got %v", callOrder)
	}
}

// TestFireStopsOnFirstError verifies Fire returns immediately on handler error.
func TestFireStopsOnFirstError(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return errors.New("error from handler 1")
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "error_handler", fn1)
	reg.Register(HookBeforeLLMCall, "second", fn2)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err == nil {
		t.Fatal("expected error from Fire")
	}

	if len(callOrder) != 1 {
		t.Fatalf("expected only 1 call before error, got %d", len(callOrder))
	}
}
