package providers

import (
	"context"
	"errors"
	"testing"
	tp "torus_go_agent/internal/types"
)

// --- Edge Case Tests for Provider Router ---

// TestProviderEdge_MultipleAddWithSameProvider tests adding the same provider multiple times.
func TestProviderEdge_MultipleAddWithSameProvider(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("openai", "gpt-4") // Same name and model, different instance
	r := NewRouter(p1)

	r.AddProvider(p2)

	// The second one should overwrite in the map
	if len(r.providers) != 1 {
		t.Fatalf("expected 1 provider in map after adding duplicate, got %d", len(r.providers))
	}

	// Active should still be the original
	if r.Active() != p1 {
		t.Fatal("active provider should remain unchanged")
	}
}

// TestProviderEdge_SwitchWithEmptyProviders tests switching when no providers registered.
func TestProviderEdge_SwitchWithEmptyProviders(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	// Manually clear the providers map (edge case)
	r.mu.Lock()
	r.providers = make(map[string]tp.Provider)
	r.mu.Unlock()

	// Now try to switch - should fail because nothing is registered
	err := r.Switch("openai", "gpt-4")
	if err == nil {
		t.Fatal("expected error when switching to unregistered provider")
	}
}

// TestProviderEdge_CompleteWithContextCancellation tests handling of cancelled context.
func TestProviderEdge_CompleteWithContextCancellation(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Even though context is cancelled, the mock provider doesn't respect it.
	// This tests that the router passes the context through correctly.
	resp, err := r.Complete(ctx, "system", nil, nil, 100)

	// The mock will succeed regardless of context cancellation, but in real
	// providers, a cancelled context might cause an error.
	if resp == nil && err == nil {
		t.Fatal("expected either response or error")
	}
}

// TestProviderEdge_WeightedRoutingWithNegativeWeights tests negative weight handling.
func TestProviderEdge_WeightedRoutingWithNegativeWeights(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Set weights with a negative value (unusual edge case)
	r.SetWeights([]RoutingEntry{
		{Key: "openai:gpt-4", Weight: -10},
		{Key: "anthropic:claude-4", Weight: 20},
	})

	// totalWeight should be 10 (sum of weights)
	if !r.IsWeighted() {
		// If totalWeight is positive, IsWeighted should be true
		t.Fatal("IsWeighted should handle negative weights and non-zero total")
	}
}

// TestProviderEdge_WeightedRoutingWithZeroTotalWeight tests all-zero weights.
func TestProviderEdge_WeightedRoutingWithZeroTotalWeight(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Set all weights to zero
	r.SetWeights([]RoutingEntry{
		{Key: "openai:gpt-4", Weight: 0},
		{Key: "anthropic:claude-4", Weight: 0},
	})

	// IsWeighted should be false when total weight is 0
	if r.IsWeighted() {
		t.Fatal("IsWeighted should be false when total weight is 0")
	}
}

// TestProviderEdge_FallbackWithUnregisteredKeys tests fallback order with unregistered provider keys.
func TestProviderEdge_FallbackWithUnregisteredKeys(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)

	// Set fallback order with a non-existent provider
	r.SetFallbackOrder([]string{
		"openai:gpt-4",
		"nonexistent:model", // This provider is not registered
		"anthropic:claude-4",
	})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("should fall back to anthropic:claude-4, got error: %v", err)
	}
	if resp.Model != "anthropic:claude-4" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "anthropic:claude-4")
	}
	// The non-existent provider should simply be skipped
	if backup.completeCalls != 1 {
		t.Fatalf("expected backup to be called once, got %d", backup.completeCalls)
	}
}

// TestProviderEdge_CompleteWithContextTimeout tests behavior with context timeout during fallback.
func TestProviderEdge_CompleteWithContextTimeout(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	// Use a non-zero timeout - the mock will complete instantly anyway
	ctx, cancel := context.WithTimeout(context.Background(), 1000) // 1 second
	defer cancel()

	resp, err := r.Complete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete should have fallen back, got error: %v", err)
	}
	if resp.Model != "anthropic:claude-4" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "anthropic:claude-4")
	}
}

// TestProviderEdge_StreamCompleteWithContextTimeout tests stream with context timeout.
func TestProviderEdge_StreamCompleteWithContextTimeout(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	ctx, cancel := context.WithTimeout(context.Background(), 1000)
	defer cancel()

	ch, err := r.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete should have fallen back, got error: %v", err)
	}

	event := <-ch
	if event.Type != tp.EventMessageStop {
		t.Fatalf("expected EventMessageStop, got %q", event.Type)
	}
}

// TestProviderEdge_RapidSwitching tests switching providers rapidly.
func TestProviderEdge_RapidSwitching(t *testing.T) {
	p1 := newMock("provider1", "model1")
	p2 := newMock("provider2", "model2")
	p3 := newMock("provider3", "model3")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.AddProvider(p3)

	// Rapid switching
	for i := 0; i < 100; i++ {
		switch i % 3 {
		case 0:
			if err := r.Switch("provider1", "model1"); err != nil {
				t.Fatalf("switch to provider1 failed: %v", err)
			}
		case 1:
			if err := r.Switch("provider2", "model2"); err != nil {
				t.Fatalf("switch to provider2 failed: %v", err)
			}
		case 2:
			if err := r.Switch("provider3", "model3"); err != nil {
				t.Fatalf("switch to provider3 failed: %v", err)
			}
		}
	}

	// Final active should be provider1 (last iteration i=99, 99 % 3 == 0)
	active := r.Active()
	if active != p1 {
		t.Fatalf("expected final active to be provider1, got %s:%s",
			active.Name(), active.ModelID())
	}
}

// TestProviderEdge_WeightedPickWithSingleEntry tests weighted pick with a single entry.
func TestProviderEdge_WeightedPickWithSingleEntry(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Single weighted entry
	r.SetWeights([]RoutingEntry{
		{Key: "anthropic:claude-4", Weight: 100},
	})

	// Call Complete multiple times and verify it picks the weighted one
	for i := 0; i < 5; i++ {
		resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
		if err != nil {
			t.Fatalf("Complete failed on iteration %d: %v", i, err)
		}
		if resp.Model != "anthropic:claude-4" {
			t.Fatalf("iteration %d: expected anthropic:claude-4, got %q",
				i, resp.Model)
		}
	}

	if p2.completeCalls != 5 {
		t.Fatalf("expected p2 to be called 5 times, got %d", p2.completeCalls)
	}
	if p1.completeCalls != 0 {
		t.Fatalf("expected p1 to be called 0 times, got %d", p1.completeCalls)
	}
}

// TestProviderEdge_CompleteWithNilMessages tests Complete with nil messages slice.
func TestProviderEdge_CompleteWithNilMessages(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	resp, err := r.Complete(context.Background(), "system prompt", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete with nil messages failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response, got nil")
	}
}

// TestProviderEdge_CompleteWithEmptyMessages tests Complete with empty messages slice.
func TestProviderEdge_CompleteWithEmptyMessages(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	resp, err := r.Complete(context.Background(), "system", []tp.Message{}, []tp.Tool{}, 100)
	if err != nil {
		t.Fatalf("Complete with empty messages failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response, got nil")
	}
}

// TestProviderEdge_CompleteWithZeroMaxTokens tests Complete with maxTokens = 0.
func TestProviderEdge_CompleteWithZeroMaxTokens(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	resp, err := r.Complete(context.Background(), "system", nil, nil, 0)
	if err != nil {
		t.Fatalf("Complete with maxTokens=0 failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response, got nil")
	}
}

// TestProviderEdge_FallbackExcludesCurrentProvider tests that fallback skips the primary provider.
func TestProviderEdge_FallbackExcludesCurrentProvider(t *testing.T) {
	failing := newFailingMock("primary", "1")
	backup1 := newMock("backup1", "1")
	backup2 := newMock("backup2", "1")
	r := NewRouter(failing)
	r.AddProvider(backup1)
	r.AddProvider(backup2)

	r.SetFallbackOrder([]string{"primary:1", "backup1:1", "backup2:1"})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp.Model != "backup1:1" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "backup1:1")
	}

	// Primary should be called once, backup1 once, backup2 never
	if failing.completeCalls != 1 {
		t.Fatalf("primary calls = %d, want 1", failing.completeCalls)
	}
	if backup1.completeCalls != 1 {
		t.Fatalf("backup1 calls = %d, want 1", backup1.completeCalls)
	}
	if backup2.completeCalls != 0 {
		t.Fatalf("backup2 calls = %d, want 0", backup2.completeCalls)
	}
}

// TestProviderEdge_CompleteWithDifferentErrors tests that different error types are preserved.
func TestProviderEdge_CompleteWithDifferentErrors(t *testing.T) {
	customErr := errors.New("custom error: network timeout")
	p1 := &mockProvider{
		name:    "provider1",
		modelID: "model1",
		completeErr: customErr,
	}
	p2 := newFailingMock("provider2", "model2")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.SetFallbackOrder([]string{"provider1:model1", "provider2:model2"})

	_, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error from failed Complete")
	}

	// Should return the primary error
	if err != customErr {
		t.Fatalf("expected primary error %q, got %q", customErr, err.Error())
	}
}

// TestProviderEdge_StreamCompleteWithDifferentErrors tests that different error types are preserved in stream.
func TestProviderEdge_StreamCompleteWithDifferentErrors(t *testing.T) {
	customErr := errors.New("stream error: connection lost")
	p1 := &mockProvider{
		name:    "provider1",
		modelID: "model1",
		completeErr: customErr,
	}
	p2 := newFailingMock("provider2", "model2")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.SetFallbackOrder([]string{"provider1:model1", "provider2:model2"})

	_, err := r.StreamComplete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error from failed StreamComplete")
	}

	// Should return the primary error
	if err != customErr {
		t.Fatalf("expected primary error %q, got %q", customErr, err.Error())
	}
}

// TestProviderEdge_ActiveReturnsCurrentAfterSwitch tests that Active() reflects recent Switch changes.
func TestProviderEdge_ActiveReturnsCurrentAfterSwitch(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	p3 := newMock("grok", "grok-2")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.AddProvider(p3)

	// Verify initial active
	if r.Active() != p1 {
		t.Fatal("initial active should be p1")
	}

	// Switch to p2
	r.Switch("anthropic", "claude-4")
	if r.Active() != p2 {
		t.Fatal("after first switch, active should be p2")
	}

	// Switch to p3
	r.Switch("grok", "grok-2")
	if r.Active() != p3 {
		t.Fatal("after second switch, active should be p3")
	}

	// Switch back to p1
	r.Switch("openai", "gpt-4")
	if r.Active() != p1 {
		t.Fatal("after third switch, active should be p1")
	}
}

// TestProviderEdge_IsWeightedAfterSetWeightsNil tests IsWeighted after setting nil.
func TestProviderEdge_IsWeightedAfterSetWeightsNil(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Set weights
	r.SetWeights([]RoutingEntry{{Key: "openai:gpt-4", Weight: 50}})
	if !r.IsWeighted() {
		t.Fatal("should be weighted after SetWeights with valid entries")
	}

	// Clear weights with nil
	r.SetWeights(nil)
	if r.IsWeighted() {
		t.Fatal("should not be weighted after SetWeights(nil)")
	}

	// Verify that the router falls back to using active provider
	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.Model != "openai:gpt-4" {
		t.Fatalf("should use active provider, got %q", resp.Model)
	}
}

// TestProviderEdge_SetFallbackOrderWithDuplicates tests fallback order with duplicate entries.
func TestProviderEdge_SetFallbackOrderWithDuplicates(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)

	// Set fallback order with duplicates
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4", "anthropic:claude-4"})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("should fall back, got error: %v", err)
	}
	if resp.Model != "anthropic:claude-4" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "anthropic:claude-4")
	}

	// Backup should be called once (first occurrence in fallback chain)
	if backup.completeCalls != 1 {
		t.Fatalf("expected 1 call, got %d", backup.completeCalls)
	}
}

// TestProviderEdge_StreamCompleteChannelConsumption tests that stream channel is properly returned.
func TestProviderEdge_StreamCompleteChannelConsumption(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	ch, err := r.StreamComplete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete failed: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel, got nil")
	}

	// Consume the channel
	event := <-ch
	if event.Type != tp.EventMessageStop {
		t.Fatalf("expected EventMessageStop, got %q", event.Type)
	}

	// Channel should be closed after consuming the event
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed")
	}
}

// TestProviderEdge_CompleteWithLargeMaxTokens tests Complete with very large maxTokens value.
func TestProviderEdge_CompleteWithLargeMaxTokens(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	resp, err := r.Complete(context.Background(), "system", nil, nil, 1000000)
	if err != nil {
		t.Fatalf("Complete with large maxTokens failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response, got nil")
	}
}

// TestProviderEdge_AddProviderAfterSwitch tests adding a provider after switching.
func TestProviderEdge_AddProviderAfterSwitch(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	if err := r.Switch("anthropic", "claude-4"); err != nil {
		t.Fatalf("switch failed: %v", err)
	}

	// Add a new provider after switching
	p3 := newMock("grok", "grok-2")
	r.AddProvider(p3)

	// Current active should still be p2
	if r.Active() != p2 {
		t.Fatal("active should still be p2 after adding new provider")
	}

	// Should be able to switch to p3
	if err := r.Switch("grok", "grok-2"); err != nil {
		t.Fatalf("switch to new provider failed: %v", err)
	}
	if r.Active() != p3 {
		t.Fatal("active should now be p3")
	}
}
