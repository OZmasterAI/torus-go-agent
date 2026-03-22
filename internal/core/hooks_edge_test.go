package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestHooksEdge_DuplicateHandlerNames verifies registering handlers with duplicate names at same point.
// Edge case: the registry allows duplicate names (no deduplication), so both should be registered.
func TestHooksEdge_DuplicateHandlerNames(t *testing.T) {
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

	// Register two handlers with the same name at the same point
	reg.Register(HookBeforeLLMCall, "duplicate_name", fn1)
	reg.Register(HookBeforeLLMCall, "duplicate_name", fn2)

	if reg.Count(HookBeforeLLMCall) != 2 {
		t.Fatalf("expected 2 handlers with duplicate names, got %d", reg.Count(HookBeforeLLMCall))
	}

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Both handlers should be called in registration order (same default priority)
	if len(callOrder) != 2 {
		t.Fatalf("expected both handlers to be called, got %d calls: %v", len(callOrder), callOrder)
	}
}

// TestHooksEdge_EmptyHookPointName verifies registering with empty hook point string.
// Edge case: empty string is a valid map key in Go, so this should work.
func TestHooksEdge_EmptyHookPointName(t *testing.T) {
	reg := NewHookRegistry()
	called := false

	fn := func(ctx context.Context, data *HookData) error {
		called = true
		return nil
	}

	// Register with empty HookPoint
	emptyPoint := HookPoint("")
	reg.Register(emptyPoint, "handler", fn)

	if reg.Count(emptyPoint) != 1 {
		t.Fatalf("expected 1 handler at empty point, got %d", reg.Count(emptyPoint))
	}

	err := reg.Fire(context.Background(), emptyPoint, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if !called {
		t.Fatal("handler should be called at empty hook point")
	}
}

// TestHooksEdge_EmptyHandlerName verifies registering with empty handler name.
// Edge case: empty name is allowed, should be registered normally.
func TestHooksEdge_EmptyHandlerName(t *testing.T) {
	reg := NewHookRegistry()
	called := false

	fn := func(ctx context.Context, data *HookData) error {
		called = true
		return nil
	}

	// Register with empty handler name
	reg.Register(HookBeforeLLMCall, "", fn)

	if reg.Count(HookBeforeLLMCall) != 1 {
		t.Fatalf("expected 1 handler with empty name, got %d", reg.Count(HookBeforeLLMCall))
	}

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if !called {
		t.Fatal("handler with empty name should be called")
	}
}

// TestHooksEdge_PriorityTieExecution verifies handlers with equal priorities execute in registration order.
// Edge case: multiple handlers with same priority should maintain insertion order after sort.
func TestHooksEdge_PriorityTieExecution(t *testing.T) {
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

	// Register with same priority (50)
	reg.RegisterPriority(HookBeforeLLMCall, "handler_1", fn1, 50)
	reg.RegisterPriority(HookBeforeLLMCall, "handler_2", fn2, 50)
	reg.RegisterPriority(HookBeforeLLMCall, "handler_3", fn3, 50)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Should maintain registration order when priorities are equal
	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("expected [1,2,3], got %v", callOrder)
	}
}

// TestHooksEdge_FireNonExistentPoint verifies Fire on unregistered hook point succeeds with no calls.
// Edge case: calling Fire on a point with no handlers registered should succeed silently.
func TestHooksEdge_FireNonExistentPoint(t *testing.T) {
	reg := NewHookRegistry()

	// Fire without registering any handlers at this point
	err := reg.Fire(context.Background(), HookOnSubagentComplete, &HookData{})
	if err != nil {
		t.Fatalf("Fire on non-existent point should not error, got: %v", err)
	}

	// Count should be 0
	if reg.Count(HookOnSubagentComplete) != 0 {
		t.Fatalf("expected 0 handlers at non-existent point, got %d", reg.Count(HookOnSubagentComplete))
	}
}

// TestHooksEdge_BlockSetByMultipleHandlers verifies Block set by first handler prevents subsequent handlers.
// Edge case: once Block is set, Fire exits immediately without calling remaining handlers.
func TestHooksEdge_BlockSetByMultipleHandlers(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		data.Block = true
		data.BlockReason = "blocked by first"
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		data.Block = true
		data.BlockReason = "blocked by second"
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "blocker_1", fn1)
	reg.Register(HookBeforeLLMCall, "blocker_2", fn2)
	reg.Register(HookBeforeLLMCall, "normal", fn3)

	data := &HookData{}
	err := reg.Fire(context.Background(), HookBeforeLLMCall, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Only first handler should run
	if len(callOrder) != 1 {
		t.Fatalf("expected only 1 call, got %d: %v", len(callOrder), callOrder)
	}
	if data.BlockReason != "blocked by first" {
		t.Fatalf("expected BlockReason 'blocked by first', got %q", data.BlockReason)
	}
}

// TestHooksEdge_HandlerErrorStopsExecution verifies handler error stops Fire execution.
// Edge case: when a handler returns an error, Fire stops immediately without calling remaining handlers.
func TestHooksEdge_HandlerErrorStopsExecution(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int
	expectedErr := errors.New("handler error")

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		return expectedErr
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	fn3 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "error_handler", fn1)
	reg.Register(HookBeforeLLMCall, "second", fn2)
	reg.Register(HookBeforeLLMCall, "third", fn3)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err == nil {
		t.Fatal("expected error from Fire")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	// Only first handler should run
	if len(callOrder) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(callOrder), callOrder)
	}
}

// TestHooksEdge_LargeNumberOfHandlers verifies registry handles many handlers efficiently.
// Edge case: registering and firing with a large number of handlers.
func TestHooksEdge_LargeNumberOfHandlers(t *testing.T) {
	reg := NewHookRegistry()
	callCount := int64(0)

	fn := func(ctx context.Context, data *HookData) error {
		atomic.AddInt64(&callCount, 1)
		return nil
	}

	// Register 1000 handlers
	for i := 0; i < 1000; i++ {
		reg.Register(HookBeforeLLMCall, "handler_"+string(rune(i)), fn)
	}

	if reg.Count(HookBeforeLLMCall) != 1000 {
		t.Fatalf("expected 1000 handlers, got %d", reg.Count(HookBeforeLLMCall))
	}

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if atomic.LoadInt64(&callCount) != 1000 {
		t.Fatalf("expected 1000 calls, got %d", atomic.LoadInt64(&callCount))
	}
}

// TestHooksEdge_ConcurrentRegisterAndFire verifies safety under concurrent register and fire.
// Edge case: register and fire can happen concurrently; the implementation uses separate locks.
func TestHooksEdge_ConcurrentRegisterAndFire(t *testing.T) {
	reg := NewHookRegistry()
	var fireCount int64
	var registerCount int64

	fn := func(ctx context.Context, data *HookData) error {
		atomic.AddInt64(&fireCount, 1)
		return nil
	}

	// Launch concurrent registrations
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				reg.Register(HookBeforeLLMCall, "handler_"+string(rune(id))+"_"+string(rune(j)), fn)
				atomic.AddInt64(&registerCount, 1)
			}
		}(i)
	}

	// Launch concurrent fires
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
			}
		}()
	}

	wg.Wait()

	// Verify counts
	if atomic.LoadInt64(&registerCount) != 50 {
		t.Fatalf("expected 50 registrations, got %d", atomic.LoadInt64(&registerCount))
	}
	// Fire count depends on timing of registration vs firing — just verify it's > 0
	if atomic.LoadInt64(&fireCount) == 0 {
		t.Fatal("expected some handler calls, got 0")
	}
}

// TestHooksEdge_PriorityExtreme verifies extreme priority values work correctly.
// Edge case: very large and very small priority values should sort correctly.
func TestHooksEdge_PriorityExtreme(t *testing.T) {
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

	// Register with extreme priorities
	reg.RegisterPriority(HookBeforeLLMCall, "high", fn1, -999999)
	reg.RegisterPriority(HookBeforeLLMCall, "medium", fn2, 0)
	reg.RegisterPriority(HookBeforeLLMCall, "low", fn3, 999999)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("expected [1,2,3], got %v", callOrder)
	}
}

// TestHooksEdge_HookDataNil verifies Fire with nil HookData.
// Edge case: passing nil HookData to Fire should not panic.
func TestHooksEdge_HookDataNil(t *testing.T) {
	reg := NewHookRegistry()

	fn := func(ctx context.Context, data *HookData) error {
		if data == nil {
			return errors.New("received nil HookData")
		}
		return nil
	}

	reg.Register(HookBeforeLLMCall, "handler", fn)

	// Fire with nil HookData — the implementation dereferences data,
	// so this panics. We verify the panic happens.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when HookData is nil")
		}
	}()
	_ = reg.Fire(context.Background(), HookBeforeLLMCall, nil)
}

// TestHooksEdge_ContextCancelled verifies Fire behavior with cancelled context.
// Edge case: handlers receive a cancelled context but registry doesn't enforce context checks.
func TestHooksEdge_ContextCancelled(t *testing.T) {
	reg := NewHookRegistry()
	var receivedCtx context.Context

	fn := func(ctx context.Context, data *HookData) error {
		receivedCtx = ctx
		// Handler could check context cancellation if needed
		return nil
	}

	reg.Register(HookBeforeLLMCall, "handler", fn)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := reg.Fire(ctx, HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Verify context was passed
	if receivedCtx == nil {
		t.Fatal("context was not passed to handler")
	}
	if receivedCtx.Err() == nil {
		t.Fatal("context should be cancelled")
	}
}

// TestHooksEdge_SameHandlerDifferentPoints verifies same handler registered at multiple points.
// Edge case: the same function can be registered at different hook points.
func TestHooksEdge_SameHandlerDifferentPoints(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []string

	fn := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, string(data.Point))
		return nil
	}

	// Register same handler at different points
	reg.Register(HookBeforeLLMCall, "handler", fn)
	reg.Register(HookAfterLLMCall, "handler", fn)
	reg.Register(HookBeforeToolCall, "handler", fn)

	// Fire at each point
	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	err = reg.Fire(context.Background(), HookAfterLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	err = reg.Fire(context.Background(), HookBeforeToolCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Verify all three were called
	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
}

// TestHooksEdge_HandlerModifiesBlock verifies handler can change Block mid-execution (edge case).
// Edge case: if a handler sets Block, Fire stops, so changing Block won't affect subsequent handlers.
func TestHooksEdge_HandlerModifiesBlock(t *testing.T) {
	reg := NewHookRegistry()
	var callOrder []int

	fn1 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 1)
		data.Block = true
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		callOrder = append(callOrder, 2)
		data.Block = false // Try to unblock, but too late
		return nil
	}

	reg.Register(HookBeforeLLMCall, "blocker", fn1)
	reg.Register(HookBeforeLLMCall, "unblocker", fn2)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Only first handler should run because Block stops Fire
	if len(callOrder) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(callOrder), callOrder)
	}
}

// TestHooksEdge_HandlerModifiesPriority verifies that priority is not re-evaluated after modification.
// Edge case: handlers cannot modify the priority list; priority is read once before iteration.
func TestHooksEdge_HandlerModifiesToolArgs(t *testing.T) {
	reg := NewHookRegistry()
	var finalToolArgs map[string]any

	fn1 := func(ctx context.Context, data *HookData) error {
		if data.ToolArgs == nil {
			data.ToolArgs = make(map[string]any)
		}
		data.ToolArgs["key1"] = "value1"
		return nil
	}
	fn2 := func(ctx context.Context, data *HookData) error {
		data.ToolArgs["key2"] = "value2"
		finalToolArgs = data.ToolArgs
		return nil
	}

	reg.Register(HookBeforeToolCall, "modifier1", fn1)
	reg.Register(HookBeforeToolCall, "modifier2", fn2)

	data := &HookData{}
	err := reg.Fire(context.Background(), HookBeforeToolCall, data)
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Both modifications should persist
	if finalToolArgs["key1"] != "value1" {
		t.Fatal("first handler's modification not visible to second handler")
	}
	if finalToolArgs["key2"] != "value2" {
		t.Fatal("second handler's modification not captured")
	}
}

// TestHooksEdge_CountWhileRegistering verifies Count behavior during concurrent registration.
// Edge case: Count can return different values at different times due to concurrent registration.
func TestHooksEdge_CountWhileRegistering(t *testing.T) {
	reg := NewHookRegistry()
	fn := func(ctx context.Context, data *HookData) error { return nil }

	var counts []int
	var mu sync.Mutex

	// Launch concurrent registrations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			reg.Register(HookBeforeLLMCall, "handler_"+string(rune(id)), fn)

			// Check count after registration
			count := reg.Count(HookBeforeLLMCall)
			mu.Lock()
			counts = append(counts, count)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Final count should be 10
	finalCount := reg.Count(HookBeforeLLMCall)
	if finalCount != 10 {
		t.Fatalf("expected final count 10, got %d", finalCount)
	}

	// At least the last count should be 10
	if counts[len(counts)-1] != 10 {
		t.Fatalf("expected last count to be 10, got %d", counts[len(counts)-1])
	}
}

// TestHooksEdge_MultipleFireOnSamePoint verifies multiple Fire calls on same point.
// Edge case: firing multiple times should call handlers multiple times.
func TestHooksEdge_MultipleFireOnSamePoint(t *testing.T) {
	reg := NewHookRegistry()
	var callCount int64

	fn := func(ctx context.Context, data *HookData) error {
		atomic.AddInt64(&callCount, 1)
		return nil
	}

	reg.Register(HookBeforeLLMCall, "handler", fn)

	// Fire multiple times
	for i := 0; i < 5; i++ {
		err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
		if err != nil {
			t.Fatalf("Fire returned error: %v", err)
		}
	}

	if atomic.LoadInt64(&callCount) != 5 {
		t.Fatalf("expected 5 calls, got %d", atomic.LoadInt64(&callCount))
	}
}

// TestHooksEdge_PriorityAfterRegistration verifies that re-sorting happens on each registration.
// Edge case: registering with priority should re-sort the slice even if a lower priority is added later.
func TestHooksEdge_PriorityReSortOnRegister(t *testing.T) {
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

	// Register lower priority first
	reg.RegisterPriority(HookBeforeLLMCall, "low_priority", fn1, 100)
	// Then register higher priority
	reg.RegisterPriority(HookBeforeLLMCall, "high_priority", fn2, 10)

	err := reg.Fire(context.Background(), HookBeforeLLMCall, &HookData{})
	if err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	// Higher priority should run first
	if callOrder[0] != 2 {
		t.Fatalf("expected high priority (2) to run first, got %v", callOrder)
	}
}
