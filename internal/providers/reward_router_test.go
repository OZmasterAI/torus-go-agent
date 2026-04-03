package providers

import (
	"context"
	"sync"
	"testing"

	tp "torus_go_agent/internal/types"
)

// --- helpers (reuse mockProvider from provider_test.go) ---

// rewardMock wraps mockProvider to also track the model key returned in responses.
// The standard mockProvider from provider_test.go is sufficient for most tests;
// this adds a configurable response text for reward-score parsing.
type rewardMock struct {
	name        string
	modelID     string
	respText    string
	completeErr error
	mu          sync.Mutex
	calls       int
}

func (m *rewardMock) Name() string    { return m.name }
func (m *rewardMock) ModelID() string { return m.modelID }

func (m *rewardMock) Complete(_ context.Context, _ string, msgs []tp.Message, _ []tp.Tool, _ int) (*tp.AssistantMessage, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.completeErr != nil {
		return nil, m.completeErr
	}
	text := m.respText
	if text == "" {
		text = "response"
	}
	return &tp.AssistantMessage{
		Message: tp.Message{
			Role:    tp.RoleAssistant,
			Content: []tp.ContentBlock{{Type: "text", Text: text}},
		},
		Model:      m.name + ":" + m.modelID,
		StopReason: "end_turn",
	}, nil
}

func (m *rewardMock) StreamComplete(_ context.Context, _ string, _ []tp.Message, _ []tp.Tool, _ int) (<-chan tp.StreamEvent, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	ch := make(chan tp.StreamEvent, 2)
	go func() {
		defer close(ch)
		resp := &tp.AssistantMessage{
			Message: tp.Message{
				Role:    tp.RoleAssistant,
				Content: []tp.ContentBlock{{Type: "text", Text: m.respText}},
			},
			Model:      m.name + ":" + m.modelID,
			StopReason: "end_turn",
		}
		ch <- tp.StreamEvent{Type: tp.EventTextDelta, Text: m.respText}
		ch <- tp.StreamEvent{Type: tp.EventMessageStop, Response: resp, StopReason: "end_turn"}
	}()
	return ch, nil
}

// newTestRewardRouter builds a RewardRouter backed by mock providers so no
// network calls are made. The underlying Router uses routerMock as the active
// provider and rewardModelMock replaces the real nvidia reward endpoint.
func newTestRewardRouter(routerMock *rewardMock, rewardModelMock *rewardMock) *RewardRouter {
	router := NewRouter(routerMock)
	rr := &RewardRouter{
		router:      router,
		rewardModel: nil, // replaced below
		scores:      make(map[string]*modelStats),
		updateEvery: 10,
	}
	// We cannot directly assign rewardModelMock to rr.rewardModel because the
	// field type is *OpenRouterProvider. Instead we test through the public API
	// and verify behavior via the router mock. The scoreAndUpdate path requires
	// a real OpenRouterProvider (HTTP), so we test it indirectly by injecting
	// scores directly for weight-recalculation tests.
	_ = rewardModelMock
	return rr
}

// --- T1: NewRewardRouter construction ---

func TestNewRewardRouter(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "test-model", respText: "hello"}
	router := NewRouter(mock)

	rr := NewRewardRouter(router, "test-api-key")

	if rr == nil {
		t.Fatal("NewRewardRouter returned nil")
	}
	if rr.router != router {
		t.Fatal("router field should reference the provided Router")
	}
	if rr.rewardModel == nil {
		t.Fatal("rewardModel should not be nil")
	}
	if rr.scores == nil {
		t.Fatal("scores map should be initialized")
	}
	if rr.updateEvery != 10 {
		t.Fatalf("updateEvery = %d, want 10", rr.updateEvery)
	}
	if rr.totalScored != 0 {
		t.Fatalf("totalScored = %d, want 0", rr.totalScored)
	}
}

func TestRewardRouterName(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "test-model", respText: "ok"}
	router := NewRouter(mock)
	rr := NewRewardRouter(router, "key")

	if got := rr.Name(); got != "nvidia" {
		t.Fatalf("Name() = %q, want %q", got, "nvidia")
	}
}

func TestRewardRouterModelID(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "llama-70b", respText: "ok"}
	router := NewRouter(mock)
	rr := NewRewardRouter(router, "key")

	if got := rr.ModelID(); got != "llama-70b" {
		t.Fatalf("ModelID() = %q, want %q", got, "llama-70b")
	}
}

// --- T1: Complete delegates to underlying router ---

func TestRewardRouterComplete(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "test-model", respText: "the answer"}
	rr := newTestRewardRouter(mock, nil)

	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "question"}}},
	}

	resp, err := rr.Complete(context.Background(), "system", msgs, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("Complete returned nil response")
	}
	if resp.Model != "nvidia:test-model" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "nvidia:test-model")
	}
	// Verify the response text came through from the mock.
	text := extractText(resp)
	if text != "the answer" {
		t.Fatalf("response text = %q, want %q", text, "the answer")
	}
	// Verify the underlying mock was called exactly once.
	mock.mu.Lock()
	calls := mock.calls
	mock.mu.Unlock()
	if calls != 1 {
		t.Fatalf("mock.calls = %d, want 1", calls)
	}
}

func TestRewardRouterCompleteError(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{
		name:        "nvidia",
		modelID:     "test-model",
		completeErr: errTest("provider down"),
	}
	rr := newTestRewardRouter(mock, nil)

	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "question"}}},
	}

	resp, err := rr.Complete(context.Background(), "system", msgs, nil, 100)
	if err == nil {
		t.Fatal("expected error from Complete when underlying provider fails")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
}

// --- T1: StreamComplete delegates to underlying router ---

func TestRewardRouterStreamComplete(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "test-model", respText: "streamed"}
	rr := newTestRewardRouter(mock, nil)

	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "question"}}},
	}

	ch, err := rr.StreamComplete(context.Background(), "system", msgs, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}
	if ch == nil {
		t.Fatal("StreamComplete returned nil channel")
	}

	var events []tp.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatal("expected stream events, got none")
	}

	// Verify we got the text delta and message stop events.
	var gotTextDelta, gotStop bool
	for _, ev := range events {
		if ev.Type == tp.EventTextDelta {
			gotTextDelta = true
			if ev.Text != "streamed" {
				t.Fatalf("text delta = %q, want %q", ev.Text, "streamed")
			}
		}
		if ev.Type == tp.EventMessageStop {
			gotStop = true
		}
	}
	if !gotTextDelta {
		t.Fatal("expected EventTextDelta in stream")
	}
	if !gotStop {
		t.Fatal("expected EventMessageStop in stream")
	}
}

func TestRewardRouterStreamCompleteError(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{
		name:        "nvidia",
		modelID:     "test-model",
		completeErr: errTest("stream fail"),
	}
	rr := newTestRewardRouter(mock, nil)

	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "q"}}},
	}

	ch, err := rr.StreamComplete(context.Background(), "system", msgs, nil, 100)
	if err == nil {
		t.Fatal("expected error from StreamComplete when underlying provider fails")
	}
	if ch != nil {
		t.Fatal("expected nil channel on error")
	}
}

// --- T1: recalculateWeights ---

func TestRecalculateWeightsNeedsAtLeastTwoModels(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "m1", respText: "ok"}
	rr := newTestRewardRouter(mock, nil)

	// Inject a single model score.
	rr.mu.Lock()
	rr.scores["model-a"] = &modelStats{totalScore: 5.0, count: 1}
	rr.mu.Unlock()

	// Should be a no-op with only 1 model.
	rr.recalculateWeights()

	if rr.router.IsWeighted() {
		t.Fatal("router should not be weighted with only 1 scored model")
	}
}

func TestRecalculateWeightsWithTwoModels(t *testing.T) {
	t.Parallel()

	m1 := &rewardMock{name: "nvidia", modelID: "m1", respText: "ok"}
	m2 := &rewardMock{name: "nvidia", modelID: "m2", respText: "ok"}

	router := NewRouter(m1)
	router.AddProvider(m2)

	rr := &RewardRouter{
		router:      router,
		scores:      make(map[string]*modelStats),
		updateEvery: 10,
	}

	// Model A has higher average score than model B.
	rr.mu.Lock()
	rr.scores["model-a"] = &modelStats{totalScore: 8.0, count: 2} // avg 4.0
	rr.scores["model-b"] = &modelStats{totalScore: 4.0, count: 2} // avg 2.0
	rr.mu.Unlock()

	rr.recalculateWeights()

	// After recalculation, router should be in weighted mode.
	if !rr.router.IsWeighted() {
		t.Fatal("router should be weighted after recalculateWeights with 2+ models")
	}
}

func TestRecalculateWeightsSkipsZeroCountModels(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "m1", respText: "ok"}
	rr := newTestRewardRouter(mock, nil)

	// One model has count=0 (never scored), the other has count=1.
	rr.mu.Lock()
	rr.scores["model-a"] = &modelStats{totalScore: 0, count: 0}
	rr.scores["model-b"] = &modelStats{totalScore: 5.0, count: 1}
	rr.mu.Unlock()

	// With only 1 valid model (count > 0), should not set weights.
	rr.recalculateWeights()

	if rr.router.IsWeighted() {
		t.Fatal("router should not be weighted when only 1 model has count > 0")
	}
}

func TestRecalculateWeightsHigherScoreGetsMoreWeight(t *testing.T) {
	t.Parallel()

	m1 := &rewardMock{name: "nvidia", modelID: "m1", respText: "ok"}
	m2 := &rewardMock{name: "nvidia", modelID: "m2", respText: "ok"}

	router := NewRouter(m1)
	router.AddProvider(m2)

	rr := &RewardRouter{
		router:      router,
		scores:      make(map[string]*modelStats),
		updateEvery: 10,
	}

	// Model A: avg = 9.0, model B: avg = 3.0
	// After shifting: A = 9-3+1 = 7, B = 3-3+1 = 1
	// Weights: A = 700, B = 100
	rr.mu.Lock()
	rr.scores["model-a"] = &modelStats{totalScore: 9.0, count: 1}
	rr.scores["model-b"] = &modelStats{totalScore: 3.0, count: 1}
	rr.mu.Unlock()

	rr.recalculateWeights()

	if !rr.router.IsWeighted() {
		t.Fatal("router should be weighted")
	}

	// Read the weights from the router to verify proportionality.
	rr.router.mu.RLock()
	weights := rr.router.weights
	rr.router.mu.RUnlock()

	if len(weights) != 2 {
		t.Fatalf("expected 2 weight entries, got %d", len(weights))
	}

	// Find weights by key.
	weightMap := make(map[string]int)
	for _, w := range weights {
		weightMap[w.Key] = w.Weight
	}

	wA := weightMap["nvidia:model-a"]
	wB := weightMap["nvidia:model-b"]

	if wA <= wB {
		t.Fatalf("model-a weight (%d) should be greater than model-b weight (%d)", wA, wB)
	}
	// Verify exact expected values: shifted A = 7.0 -> 700, B = 1.0 -> 100
	if wA != 700 {
		t.Fatalf("model-a weight = %d, want 700", wA)
	}
	if wB != 100 {
		t.Fatalf("model-b weight = %d, want 100", wB)
	}
}

// --- T1: Concurrent safety ---

func TestRewardRouterConcurrentComplete(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "test-model", respText: "ok"}
	rr := newTestRewardRouter(mock, nil)

	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "question"}}},
	}

	var wg sync.WaitGroup
	numGoroutines := 20

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			resp, err := rr.Complete(context.Background(), "system", msgs, nil, 100)
			if err != nil {
				t.Errorf("concurrent Complete returned error: %v", err)
				return
			}
			if resp == nil {
				t.Error("concurrent Complete returned nil response")
			}
		}()
	}
	wg.Wait()

	// All goroutines should have called through.
	mock.mu.Lock()
	calls := mock.calls
	mock.mu.Unlock()
	if calls != numGoroutines {
		t.Fatalf("mock.calls = %d, want %d", calls, numGoroutines)
	}
}

func TestRewardRouterConcurrentScoreUpdate(t *testing.T) {
	t.Parallel()

	mock := &rewardMock{name: "nvidia", modelID: "m1", respText: "ok"}
	rr := newTestRewardRouter(mock, nil)

	// Simulate concurrent score updates (the internal path that scoreAndUpdate writes to).
	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			modelKey := "model-a"
			if idx%2 == 0 {
				modelKey = "model-b"
			}
			rr.mu.Lock()
			stats, ok := rr.scores[modelKey]
			if !ok {
				stats = &modelStats{}
				rr.scores[modelKey] = stats
			}
			stats.totalScore += float64(idx)
			stats.count++
			rr.totalScored++
			rr.mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Verify counts are consistent.
	rr.mu.RLock()
	total := rr.totalScored
	countA := 0
	countB := 0
	if s, ok := rr.scores["model-a"]; ok {
		countA = s.count
	}
	if s, ok := rr.scores["model-b"]; ok {
		countB = s.count
	}
	rr.mu.RUnlock()

	if total != numGoroutines {
		t.Fatalf("totalScored = %d, want %d", total, numGoroutines)
	}
	if countA+countB != numGoroutines {
		t.Fatalf("countA (%d) + countB (%d) = %d, want %d", countA, countB, countA+countB, numGoroutines)
	}
}

// --- T1: helper functions ---

func TestLastUserMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []tp.Message
		want     string
	}{
		{
			name:     "empty messages",
			messages: nil,
			want:     "",
		},
		{
			name: "single user message",
			messages: []tp.Message{
				{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "hello"}}},
			},
			want: "hello",
		},
		{
			name: "returns last user message",
			messages: []tp.Message{
				{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "first"}}},
				{Role: tp.RoleAssistant, Content: []tp.ContentBlock{{Type: "text", Text: "reply"}}},
				{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "second"}}},
			},
			want: "second",
		},
		{
			name: "no user messages",
			messages: []tp.Message{
				{Role: tp.RoleAssistant, Content: []tp.ContentBlock{{Type: "text", Text: "assistant only"}}},
			},
			want: "",
		},
		{
			name: "user message with empty text",
			messages: []tp.Message{
				{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: ""}}},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lastUserMessage(tt.messages)
			if got != tt.want {
				t.Fatalf("lastUserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *tp.AssistantMessage
		want string
	}{
		{
			name: "single text block",
			msg: &tp.AssistantMessage{
				Message: tp.Message{
					Content: []tp.ContentBlock{{Type: "text", Text: "hello"}},
				},
			},
			want: "hello",
		},
		{
			name: "no text blocks",
			msg: &tp.AssistantMessage{
				Message: tp.Message{
					Content: []tp.ContentBlock{{Type: "tool_use", Text: ""}},
				},
			},
			want: "",
		},
		{
			name: "empty content",
			msg: &tp.AssistantMessage{
				Message: tp.Message{
					Content: []tp.ContentBlock{},
				},
			},
			want: "",
		},
		{
			name: "multiple blocks returns first text",
			msg: &tp.AssistantMessage{
				Message: tp.Message{
					Content: []tp.ContentBlock{
						{Type: "tool_use", Text: ""},
						{Type: "text", Text: "found it"},
						{Type: "text", Text: "second"},
					},
				},
			},
			want: "found it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractText(tt.msg)
			if got != tt.want {
				t.Fatalf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// errTest is a simple error type for test assertions.
type errTest string

func (e errTest) Error() string { return string(e) }
