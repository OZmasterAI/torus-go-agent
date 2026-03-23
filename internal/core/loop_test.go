package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	typ "torus_go_agent/internal/types"
)

// --- Mock Provider ---

// mockProvider implements types.Provider and returns canned responses.
type mockProvider struct {
	name    string
	modelID string
	// cannedText is returned as a text-delta + message_stop sequence.
	cannedText string
	// cannedUsage is attached to the message_stop event.
	cannedUsage typ.Usage
	// streamErr, if non-nil, is returned from StreamComplete instead of a channel.
	streamErr error
	// completeResp is returned by Complete (not used by the loop, but required by interface).
	completeResp *typ.AssistantMessage
	// completeErr is returned by Complete.
	completeErr error
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) ModelID() string { return m.modelID }

func (m *mockProvider) Complete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (*typ.AssistantMessage, error) {
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

func (m *mockProvider) StreamComplete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (<-chan typ.StreamEvent, error) {
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

// newTestDAG creates a temporary SQLite DAG for use in tests.
func newTestDAG(t *testing.T) *DAG {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dag, err := NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	t.Cleanup(func() { dag.Close() })
	return dag
}

// newTestAgent builds an Agent wired to a mock provider with sane defaults.
func newTestAgent(t *testing.T, mp *mockProvider) (*Agent, *DAG) {
	t.Helper()
	dag := newTestDAG(t)
	cfg := typ.AgentConfig{
		Provider:  typ.ProviderConfig{Name: mp.name, Model: mp.modelID, MaxTokens: 1024},
		MaxTurns:  3,
	}
	hooks := NewHookRegistry()
	agent := NewAgent(cfg, mp, hooks, dag)
	// Disable LLM compaction so tests never need a Summarize function.
	agent.SetCompaction(CompactionConfig{Mode: CompactionOff})
	return agent, dag
}

// collectEvents drains a RunStream channel into a slice.
func collectEvents(ch <-chan AgentEvent) []AgentEvent {
	var evs []AgentEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

// findEvents returns all events of the given type from a slice.
func findEvents(evs []AgentEvent, typ AgentEventType) []AgentEvent {
	var out []AgentEvent
	for _, ev := range evs {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

// --- Tests ---

// TestNewAgent verifies that NewAgent wires up config and provider correctly.
func TestNewAgent(t *testing.T) {
	mp := &mockProvider{name: "mock", modelID: "mock-model-1", cannedText: "hello"}
	agent, dag := newTestAgent(t, mp)

	if agent == nil {
		t.Fatal("NewAgent returned nil")
	}
	if agent.provider != typ.Provider(mp) {
		t.Error("provider not stored on agent")
	}
	if agent.dag != dag {
		t.Error("dag not stored on agent")
	}
	if agent.config.Provider.Name != "mock" {
		t.Errorf("config.Provider.Name = %q, want %q", agent.config.Provider.Name, "mock")
	}
	if agent.hooks == nil {
		t.Error("hooks should be non-nil")
	}
}

// TestRunStream_EventSequence verifies that a clean run emits:
// TurnStart → TextDelta → TurnEnd (with Usage) → Done.
func TestRunStream_EventSequence(t *testing.T) {
	usage := typ.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Cost: 0.001}
	mp := &mockProvider{
		name:        "mock",
		modelID:     "mock-model-1",
		cannedText:  "Hello, world!",
		cannedUsage: usage,
	}
	agent, _ := newTestAgent(t, mp)

	evs := collectEvents(agent.RunStream(context.Background(), "hi"))

	// Must include at least one TurnStart.
	turnStarts := findEvents(evs, EventAgentTurnStart)
	if len(turnStarts) == 0 {
		t.Error("expected at least one EventAgentTurnStart")
	}

	// Must include at least one TextDelta with the canned text.
	deltas := findEvents(evs, EventAgentTextDelta)
	if len(deltas) == 0 {
		t.Error("expected at least one EventAgentTextDelta")
	}
	if len(deltas) > 0 && deltas[0].Text != "Hello, world!" {
		t.Errorf("TextDelta.Text = %q, want %q", deltas[0].Text, "Hello, world!")
	}

	// Must include at least one TurnEnd.
	turnEnds := findEvents(evs, EventAgentTurnEnd)
	if len(turnEnds) == 0 {
		t.Error("expected at least one EventAgentTurnEnd")
	}

	// Must end with EventAgentDone.
	if len(evs) == 0 || evs[len(evs)-1].Type != EventAgentDone {
		t.Errorf("last event = %v, want EventAgentDone", func() AgentEventType {
			if len(evs) == 0 {
				return "(none)"
			}
			return evs[len(evs)-1].Type
		}())
	}

	// Done event should carry the final text.
	doneEvs := findEvents(evs, EventAgentDone)
	if len(doneEvs) > 0 && doneEvs[0].Text != "Hello, world!" {
		t.Errorf("Done.Text = %q, want %q", doneEvs[0].Text, "Hello, world!")
	}
}

// TestRunStream_TurnEnd_CarriesUsage verifies that EventAgentTurnEnd carries
// the Usage struct returned by the provider.
func TestRunStream_TurnEnd_CarriesUsage(t *testing.T) {
	usage := typ.Usage{InputTokens: 20, OutputTokens: 8, TotalTokens: 28, Cost: 0.002}
	mp := &mockProvider{
		name:        "mock",
		modelID:     "mock-model-1",
		cannedText:  "response text",
		cannedUsage: usage,
	}
	agent, _ := newTestAgent(t, mp)

	evs := collectEvents(agent.RunStream(context.Background(), "test"))

	turnEnds := findEvents(evs, EventAgentTurnEnd)
	if len(turnEnds) == 0 {
		t.Fatal("no EventAgentTurnEnd found")
	}
	tu := turnEnds[0].Usage
	if tu == nil {
		t.Fatal("TurnEnd.Usage is nil")
	}
	if tu.InputTokens != usage.InputTokens {
		t.Errorf("Usage.InputTokens = %d, want %d", tu.InputTokens, usage.InputTokens)
	}
	if tu.OutputTokens != usage.OutputTokens {
		t.Errorf("Usage.OutputTokens = %d, want %d", tu.OutputTokens, usage.OutputTokens)
	}
	if tu.TotalTokens != usage.TotalTokens {
		t.Errorf("Usage.TotalTokens = %d, want %d", tu.TotalTokens, usage.TotalTokens)
	}
}

// TestRunStream_LLMError_EmitsEventAgentError verifies that when the provider
// returns an error from StreamComplete, the loop emits EventAgentError (not a
// panic or silent failure).
func TestRunStream_LLMError_EmitsEventAgentError(t *testing.T) {
	provErr := errors.New("upstream timeout")
	mp := &mockProvider{
		name:      "mock",
		modelID:   "mock-model-1",
		streamErr: provErr,
	}
	agent, _ := newTestAgent(t, mp)

	evs := collectEvents(agent.RunStream(context.Background(), "will fail"))

	errEvs := findEvents(evs, EventAgentError)
	if len(errEvs) == 0 {
		t.Fatal("expected EventAgentError, got none")
	}
	if errEvs[0].Error == nil {
		t.Fatal("EventAgentError.Error is nil")
	}
	if !errors.Is(errEvs[0].Error, provErr) {
		t.Errorf("error chain does not wrap provErr: got %v", errEvs[0].Error)
	}
}

// TestRunStream_StreamChannelError_EmitsEventAgentError verifies that an error
// delivered inside the stream channel (EventError event) is surfaced as
// EventAgentError.
func TestRunStream_StreamChannelError_EmitsEventAgentError(t *testing.T) {
	chanErr := errors.New("stream decode error")
	mp := &mockProvider{name: "mock", modelID: "mock-model-1"}
	// Override StreamComplete to return a channel that sends an error event.
	dag := newTestDAG(t)
	cfg := typ.AgentConfig{
		Provider: typ.ProviderConfig{Name: "mock", Model: "mock-model-1", MaxTokens: 512},
		MaxTurns: 1,
	}
	hooks := NewHookRegistry()
	agent := NewAgent(cfg, mp, hooks, dag)
	agent.SetCompaction(CompactionConfig{Mode: CompactionOff})

	// Swap provider with one whose channel sends an error event.
	errorInChanProvider := &channelErrorProvider{err: chanErr}
	agent.provider = errorInChanProvider

	evs := collectEvents(agent.RunStream(context.Background(), "will error in stream"))

	errEvs := findEvents(evs, EventAgentError)
	if len(errEvs) == 0 {
		t.Fatal("expected EventAgentError from channel error event, got none")
	}
	if errEvs[0].Error == nil {
		t.Fatal("EventAgentError.Error is nil")
	}
}

// channelErrorProvider sends a single error event on the stream channel.
type channelErrorProvider struct{ err error }

func (c *channelErrorProvider) Name() string    { return "channel-error" }
func (c *channelErrorProvider) ModelID() string { return "channel-error-model" }
func (c *channelErrorProvider) Complete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (*typ.AssistantMessage, error) {
	return nil, c.err
}
func (c *channelErrorProvider) StreamComplete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (<-chan typ.StreamEvent, error) {
	ch := make(chan typ.StreamEvent, 1)
	ch <- typ.StreamEvent{Type: typ.EventError, Error: c.err}
	close(ch)
	return ch, nil
}

// thinkingMockProvider returns a response that includes both text and thinking blocks.
type thinkingMockProvider struct {
	name    string
	modelID string
}

func (m *thinkingMockProvider) Name() string    { return m.name }
func (m *thinkingMockProvider) ModelID() string { return m.modelID }
func (m *thinkingMockProvider) Complete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (*typ.AssistantMessage, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *thinkingMockProvider) StreamComplete(_ context.Context, _ string, _ []typ.Message, _ []typ.Tool, _ int) (<-chan typ.StreamEvent, error) {
	ch := make(chan typ.StreamEvent, 4)
	go func() {
		defer close(ch)
		ch <- typ.StreamEvent{Type: typ.EventTextDelta, Text: "visible reply"}
		ch <- typ.StreamEvent{Type: typ.EventThinkingDelta, Text: "internal reasoning"}
		resp := &typ.AssistantMessage{
			Message: typ.Message{
				Role: typ.RoleAssistant,
				Content: []typ.ContentBlock{
					{Type: "thinking", Text: "internal reasoning"},
					{Type: "text", Text: "visible reply"},
				},
			},
			Model:      m.modelID,
			StopReason: "end_turn",
			Usage:      typ.Usage{TotalTokens: 10},
		}
		ch <- typ.StreamEvent{Type: typ.EventMessageStop, Response: resp}
	}()
	return ch, nil
}

// TestPersistThinking_Enabled verifies that when PersistThinking is true,
// thinking blocks are stored as a separate DAG node.
func TestPersistThinking_Enabled(t *testing.T) {
	mp := &thinkingMockProvider{name: "mock", modelID: "think-model"}
	dag := newTestDAG(t)
	cfg := typ.AgentConfig{
		Provider:        typ.ProviderConfig{Name: mp.name, Model: mp.modelID, MaxTokens: 1024},
		MaxTurns:        3,
		PersistThinking: true,
	}
	hooks := NewHookRegistry()
	agent := NewAgent(cfg, mp, hooks, dag)
	agent.SetCompaction(CompactionConfig{Mode: CompactionOff})

	evs := collectEvents(agent.RunStream(context.Background(), "think about this"))

	// Run must complete successfully.
	doneEvs := findEvents(evs, EventAgentDone)
	if len(doneEvs) == 0 {
		t.Fatal("no EventAgentDone")
	}

	// The DAG should have: user node, assistant node (clean), thinking node.
	// Walk from head backward to count nodes.
	head, _ := dag.GetHead()
	messages, err := dag.PromptFrom(head)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}

	// Find a node with role "thinking"
	foundThinking := false
	for _, msg := range messages {
		if string(msg.Role) == "thinking" {
			foundThinking = true
			if len(msg.Content) == 0 || msg.Content[0].Text == "" {
				t.Error("thinking node has empty content")
			}
		}
	}
	if !foundThinking {
		t.Error("expected a thinking node in DAG when PersistThinking=true")
	}
}

// TestPersistThinking_Disabled verifies that when PersistThinking is false,
// no thinking node is stored in the DAG.
func TestPersistThinking_Disabled(t *testing.T) {
	mp := &thinkingMockProvider{name: "mock", modelID: "think-model"}
	dag := newTestDAG(t)
	cfg := typ.AgentConfig{
		Provider:        typ.ProviderConfig{Name: mp.name, Model: mp.modelID, MaxTokens: 1024},
		MaxTurns:        3,
		PersistThinking: false,
	}
	hooks := NewHookRegistry()
	agent := NewAgent(cfg, mp, hooks, dag)
	agent.SetCompaction(CompactionConfig{Mode: CompactionOff})

	evs := collectEvents(agent.RunStream(context.Background(), "think about this"))

	doneEvs := findEvents(evs, EventAgentDone)
	if len(doneEvs) == 0 {
		t.Fatal("no EventAgentDone")
	}

	head, _ := dag.GetHead()
	messages, err := dag.PromptFrom(head)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}

	for _, msg := range messages {
		if string(msg.Role) == "thinking" {
			t.Error("found thinking node in DAG when PersistThinking=false")
		}
	}
}

// TestAutoAlias verifies that after AddNode, the returned node ID can be
// aliased via NextAutoAlias and resolved back.
func TestAutoAlias(t *testing.T) {
	dag := newTestDAG(t)

	head, _ := dag.GetHead()
	nodeID, err := dag.AddNode(head, typ.RoleAssistant, []typ.ContentBlock{{Type: "text", Text: "hi"}}, "m", "p", 0)
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	alias := dag.NextAutoAlias()
	if alias != "a1" {
		t.Errorf("first auto-alias = %q, want %q", alias, "a1")
	}
	if err := dag.SetAlias(nodeID, alias); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	resolved, err := dag.ResolveAlias(alias)
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if resolved != nodeID {
		t.Errorf("ResolveAlias(%q) = %q, want %q", alias, resolved, nodeID)
	}
}

// TestAutoAlias_Increments verifies that successive aliases are a1, a2, a3.
func TestAutoAlias_Increments(t *testing.T) {
	dag := newTestDAG(t)

	for i := 1; i <= 3; i++ {
		head, _ := dag.GetHead()
		nodeID, err := dag.AddNode(head, typ.RoleAssistant, []typ.ContentBlock{{Type: "text", Text: fmt.Sprintf("turn %d", i)}}, "m", "p", 0)
		if err != nil {
			t.Fatalf("AddNode %d: %v", i, err)
		}
		alias := dag.NextAutoAlias()
		wantAlias := fmt.Sprintf("a%d", i)
		if alias != wantAlias {
			t.Errorf("iteration %d: NextAutoAlias() = %q, want %q", i, alias, wantAlias)
		}
		if err := dag.SetAlias(nodeID, alias); err != nil {
			t.Fatalf("SetAlias %d: %v", i, err)
		}
	}
}

// TestRunStream_AutoAlias_SetAfterAssistantNode verifies that the loop itself
// calls SetAlias on the assistant node produced each turn.
func TestRunStream_AutoAlias_SetAfterAssistantNode(t *testing.T) {
	mp := &mockProvider{
		name:       "mock",
		modelID:    "mock-model-1",
		cannedText: "aliased response",
	}
	agent, dag := newTestAgent(t, mp)

	evs := collectEvents(agent.RunStream(context.Background(), "alias test"))

	// The run must succeed.
	doneEvs := findEvents(evs, EventAgentDone)
	if len(doneEvs) == 0 {
		t.Fatal("no EventAgentDone — run did not complete successfully")
	}

	// There should be at least one alias in the node_aliases table.
	alias := "a1"
	resolvedID, err := dag.ResolveAlias(alias)
	if err != nil {
		t.Fatalf("ResolveAlias(%q) after RunStream: %v", alias, err)
	}
	if resolvedID == "" {
		t.Errorf("alias %q resolved to empty node ID", alias)
	}
}

// TestRun_ReturnsText verifies the synchronous Run wrapper returns the final text.
func TestRun_ReturnsText(t *testing.T) {
	mp := &mockProvider{
		name:       "mock",
		modelID:    "mock-model-1",
		cannedText: "the answer",
	}
	agent, _ := newTestAgent(t, mp)

	got, err := agent.Run(context.Background(), "question")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got != "the answer" {
		t.Errorf("Run() = %q, want %q", got, "the answer")
	}
}

// TestRun_PropagatesError verifies that Run returns the LLM error.
func TestRun_PropagatesError(t *testing.T) {
	mp := &mockProvider{
		name:      "mock",
		modelID:   "mock-model-1",
		streamErr: errors.New("provider down"),
	}
	agent, _ := newTestAgent(t, mp)

	_, err := agent.Run(context.Background(), "will fail")
	if err == nil {
		t.Fatal("Run should have returned an error, got nil")
	}
}
