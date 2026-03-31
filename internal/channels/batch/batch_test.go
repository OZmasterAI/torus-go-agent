package batch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	tp "torus_go_agent/internal/types"
)

// stubProvider returns a fixed text response via streaming.
type stubProvider struct {
	text  string
	usage tp.Usage
}

func (s *stubProvider) Name() string    { return "stub" }
func (s *stubProvider) ModelID() string { return "stub-model" }

func (s *stubProvider) Complete(_ context.Context, _ string, _ []tp.Message, _ []tp.Tool, _ int) (*tp.AssistantMessage, error) {
	return &tp.AssistantMessage{
		Message:    tp.Message{Role: tp.RoleAssistant, Content: []tp.ContentBlock{{Type: "text", Text: s.text}}},
		StopReason: "end_turn",
		Usage:      s.usage,
	}, nil
}

func (s *stubProvider) StreamComplete(_ context.Context, _ string, _ []tp.Message, _ []tp.Tool, _ int) (<-chan tp.StreamEvent, error) {
	ch := make(chan tp.StreamEvent, 4)
	go func() {
		defer close(ch)
		ch <- tp.StreamEvent{Type: tp.EventTextDelta, Text: s.text}
		ch <- tp.StreamEvent{
			Type: tp.EventMessageStop,
			Response: &tp.AssistantMessage{
				Message:    tp.Message{Role: tp.RoleAssistant, Content: []tp.ContentBlock{{Type: "text", Text: s.text}}},
				StopReason: "end_turn",
				Usage:      s.usage,
			},
		}
	}()
	return ch, nil
}

func TestBatch_MissingPromptFile(t *testing.T) {
	ch := &batchChannel{}
	Config.PromptFile = ""
	Config.OutputDir = ""
	err := ch.Start(nil, config.Config{}, nil)
	if err == nil {
		t.Fatal("expected error for missing prompt file")
	}
}

func TestBatch_EmptyPrompt(t *testing.T) {
	tmp := t.TempDir()
	promptFile := filepath.Join(tmp, "prompt.txt")
	os.WriteFile(promptFile, []byte(""), 0o644)

	ch := &batchChannel{}
	Config.PromptFile = promptFile
	Config.OutputDir = filepath.Join(tmp, "out")
	err := ch.Start(nil, config.Config{}, nil)
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestBatch_RunsAndWritesResult(t *testing.T) {
	tmp := t.TempDir()
	promptFile := filepath.Join(tmp, "prompt.txt")
	os.WriteFile(promptFile, []byte("say hello"), 0o644)
	outDir := filepath.Join(tmp, "out")

	prov := &stubProvider{
		text:  "Hello!",
		usage: tp.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}

	dagPath := filepath.Join(tmp, "test.db")
	dag, err := core.NewDAG(dagPath)
	if err != nil {
		t.Fatalf("dag: %v", err)
	}
	defer dag.Close()

	hooks := core.NewHookRegistry()
	agent := core.NewAgent(tp.AgentConfig{
		Provider:      tp.ProviderConfig{Name: "stub", Model: "stub-model"},
		SystemPrompt:  "test",
		Tools:         nil,
		MaxTurns:      5,
		ContextWindow: 100000,
	}, prov, hooks, dag)

	Config.PromptFile = promptFile
	Config.OutputDir = outDir

	bch := &batchChannel{}
	err = bch.Start(agent, config.Config{}, features.NewSkillRegistry(""))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify result.json was written
	resultPath := filepath.Join(outDir, "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if result.Prompt != "say hello" {
		t.Errorf("prompt = %q, want %q", result.Prompt, "say hello")
	}
	if result.Response != "Hello!" {
		t.Errorf("response = %q, want %q", result.Response, "Hello!")
	}
	if result.DurationMs <= 0 {
		t.Error("expected positive duration")
	}
	if result.TotalInput != 10 {
		t.Errorf("total_input = %d, want 10", result.TotalInput)
	}
	if len(result.Trace) == 0 {
		t.Error("expected non-empty trace")
	}
}
