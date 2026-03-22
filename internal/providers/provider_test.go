package providers

import (
	"context"
	"errors"
	"testing"
	tp "torus_go_agent/internal/types"
)

// mockProvider implements tp.Provider for testing.
type mockProvider struct {
	name    string
	modelID string
	// When non-nil, Complete and StreamComplete return this error.
	completeErr error
	// Tracks how many times Complete was called.
	completeCalls int
	// Tracks how many times StreamComplete was called.
	streamCalls int
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) ModelID() string { return m.modelID }

func (m *mockProvider) Complete(_ context.Context, _ string, _ []tp.Message, _ []tp.Tool, _ int) (*tp.AssistantMessage, error) {
	m.completeCalls++
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return &tp.AssistantMessage{
		Message: tp.Message{Role: tp.RoleAssistant},
		Model:   m.name + ":" + m.modelID,
	}, nil
}

func (m *mockProvider) StreamComplete(_ context.Context, _ string, _ []tp.Message, _ []tp.Tool, _ int) (<-chan tp.StreamEvent, error) {
	m.streamCalls++
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	ch := make(chan tp.StreamEvent, 1)
	ch <- tp.StreamEvent{Type: tp.EventMessageStop, StopReason: "end"}
	close(ch)
	return ch, nil
}

func newMock(name, model string) *mockProvider {
	return &mockProvider{name: name, modelID: model}
}

func newFailingMock(name, model string) *mockProvider {
	return &mockProvider{name: name, modelID: model, completeErr: errors.New(name + " error")}
}

// --- Tests ---

func TestNewRouter(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	if r == nil {
		t.Fatal("NewRouter returned nil")
	}
	if r.Active() != p {
		t.Fatal("active provider should be the initial provider")
	}
	if len(r.providers) != 1 {
		t.Fatalf("expected 1 registered provider, got %d", len(r.providers))
	}
	key := "openai:gpt-4"
	if _, ok := r.providers[key]; !ok {
		t.Fatalf("expected provider key %q in map", key)
	}
}

func TestAddProvider(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	if len(r.providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(r.providers))
	}
	if _, ok := r.providers["anthropic:claude-4"]; !ok {
		t.Fatal("expected anthropic:claude-4 to be registered")
	}
	// Active should still be the initial provider.
	if r.Active() != p1 {
		t.Fatal("AddProvider should not change the active provider")
	}
}

func TestSwitch(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Switch to an existing provider.
	if err := r.Switch("anthropic", "claude-4"); err != nil {
		t.Fatalf("Switch to valid provider failed: %v", err)
	}
	if r.Active() != p2 {
		t.Fatal("after Switch, active provider should be anthropic:claude-4")
	}

	// Switch back.
	if err := r.Switch("openai", "gpt-4"); err != nil {
		t.Fatalf("Switch back failed: %v", err)
	}
	if r.Active() != p1 {
		t.Fatal("after Switch back, active should be openai:gpt-4")
	}

	// Switch to unknown provider should error.
	err := r.Switch("missing", "model")
	if err == nil {
		t.Fatal("expected error when switching to unregistered provider")
	}
	wantMsg := `provider "missing:model" not registered`
	if err.Error() != wantMsg {
		t.Fatalf("error = %q, want %q", err.Error(), wantMsg)
	}
}

func TestActive(t *testing.T) {
	p := newMock("grok", "grok-3")
	r := NewRouter(p)

	active := r.Active()
	if active.Name() != "grok" {
		t.Fatalf("Active().Name() = %q, want %q", active.Name(), "grok")
	}
	if active.ModelID() != "grok-3" {
		t.Fatalf("Active().ModelID() = %q, want %q", active.ModelID(), "grok-3")
	}
}

func TestSetWeightsAndIsWeighted(t *testing.T) {
	p1 := newMock("openai", "gpt-4")
	p2 := newMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)

	// Initially not weighted.
	if r.IsWeighted() {
		t.Fatal("new router should not be weighted")
	}

	// Enable weighted routing.
	r.SetWeights([]RoutingEntry{
		{Key: "openai:gpt-4", Weight: 80},
		{Key: "anthropic:claude-4", Weight: 20},
	})
	if !r.IsWeighted() {
		t.Fatal("IsWeighted should be true after SetWeights with entries")
	}

	// Disable by passing nil.
	r.SetWeights(nil)
	if r.IsWeighted() {
		t.Fatal("IsWeighted should be false after SetWeights(nil)")
	}

	// Disable by passing empty slice.
	r.SetWeights([]RoutingEntry{})
	if r.IsWeighted() {
		t.Fatal("IsWeighted should be false after SetWeights(empty)")
	}

	// Zero-weight entries should not count as weighted.
	r.SetWeights([]RoutingEntry{
		{Key: "openai:gpt-4", Weight: 0},
	})
	if r.IsWeighted() {
		t.Fatal("IsWeighted should be false when all weights are zero")
	}
}

func TestCompleteSuccess(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp.Model != "openai:gpt-4" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "openai:gpt-4")
	}
	if p.completeCalls != 1 {
		t.Fatalf("expected 1 Complete call, got %d", p.completeCalls)
	}
}

func TestCompleteFallback(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete should have fallen back successfully, got error: %v", err)
	}
	if resp.Model != "anthropic:claude-4" {
		t.Fatalf("resp.Model = %q, want fallback %q", resp.Model, "anthropic:claude-4")
	}
	if failing.completeCalls != 1 {
		t.Fatalf("primary should have been called once, got %d", failing.completeCalls)
	}
	if backup.completeCalls != 1 {
		t.Fatalf("fallback should have been called once, got %d", backup.completeCalls)
	}
}

func TestCompleteAllFail(t *testing.T) {
	p1 := newFailingMock("openai", "gpt-4")
	p2 := newFailingMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	_, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	// Should return the primary error, not the fallback error.
	if err.Error() != "openai error" {
		t.Fatalf("error = %q, want primary error %q", err.Error(), "openai error")
	}
}

func TestStreamCompleteSuccess(t *testing.T) {
	p := newMock("openai", "gpt-4")
	r := NewRouter(p)

	ch, err := r.StreamComplete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	event := <-ch
	if event.Type != tp.EventMessageStop {
		t.Fatalf("event.Type = %q, want %q", event.Type, tp.EventMessageStop)
	}
	if p.streamCalls != 1 {
		t.Fatalf("expected 1 StreamComplete call, got %d", p.streamCalls)
	}
}

func TestStreamCompleteFallback(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	ch, err := r.StreamComplete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete should have fallen back, got error: %v", err)
	}

	event := <-ch
	if event.Type != tp.EventMessageStop {
		t.Fatalf("expected EventMessageStop from fallback, got %q", event.Type)
	}
	if failing.streamCalls != 1 {
		t.Fatalf("primary stream should have been called once, got %d", failing.streamCalls)
	}
	if backup.streamCalls != 1 {
		t.Fatalf("fallback stream should have been called once, got %d", backup.streamCalls)
	}
}

func TestStreamCompleteAllFail(t *testing.T) {
	p1 := newFailingMock("openai", "gpt-4")
	p2 := newFailingMock("anthropic", "claude-4")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	_, err := r.StreamComplete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error when all stream providers fail")
	}
	if err.Error() != "openai error" {
		t.Fatalf("error = %q, want primary error %q", err.Error(), "openai error")
	}
}

func TestSetFallbackOrder(t *testing.T) {
	p1 := newFailingMock("a", "1")
	p2 := newFailingMock("b", "2")
	p3 := newMock("c", "3")
	r := NewRouter(p1)
	r.AddProvider(p2)
	r.AddProvider(p3)

	// Fallback order: a:1 -> b:2 -> c:3
	// Primary (a:1) fails, b:2 fails, c:3 succeeds.
	r.SetFallbackOrder([]string{"a:1", "b:2", "c:3"})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("expected c:3 fallback to succeed, got error: %v", err)
	}
	if resp.Model != "c:3" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "c:3")
	}
	// All three should have been called: primary a:1, fallback b:2, fallback c:3.
	if p1.completeCalls != 1 {
		t.Fatalf("a:1 calls = %d, want 1", p1.completeCalls)
	}
	if p2.completeCalls != 1 {
		t.Fatalf("b:2 calls = %d, want 1", p2.completeCalls)
	}
	if p3.completeCalls != 1 {
		t.Fatalf("c:3 calls = %d, want 1", p3.completeCalls)
	}
}

func TestSetFallbackOrderNil(t *testing.T) {
	failing := newFailingMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(failing)
	r.AddProvider(backup)

	// With no fallback order set, failure should return the primary error.
	_, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error with no fallback order")
	}

	// Set fallback, then clear it with nil.
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})
	r.SetFallbackOrder(nil)

	_, err = r.Complete(context.Background(), "system", nil, nil, 100)
	if err == nil {
		t.Fatal("expected error after clearing fallback order")
	}
}

func TestCompleteNoFallbackOnSuccess(t *testing.T) {
	primary := newMock("openai", "gpt-4")
	backup := newMock("anthropic", "claude-4")
	r := NewRouter(primary)
	r.AddProvider(backup)
	r.SetFallbackOrder([]string{"openai:gpt-4", "anthropic:claude-4"})

	resp, err := r.Complete(context.Background(), "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "openai:gpt-4" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "openai:gpt-4")
	}
	// Backup should never be called when primary succeeds.
	if backup.completeCalls != 0 {
		t.Fatalf("fallback should not be called on success, got %d calls", backup.completeCalls)
	}
}
