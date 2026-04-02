package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloadSystemPrompt(t *testing.T) {
	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)
	agent.config.SystemPrompt = "original prompt"

	agent.ReloadSystemPrompt(context.Background(), "updated prompt")
	if agent.config.SystemPrompt != "updated prompt" {
		t.Errorf("prompt = %q, want %q", agent.config.SystemPrompt, "updated prompt")
	}
}

func TestReloadSystemPrompt_HookInjectsContext(t *testing.T) {
	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)

	agent.Hooks().Register(HookOnInstructionsLoaded, "inject", func(_ context.Context, data *HookData) error {
		data.AdditionalContext = "## Extra Rules\nBe concise."
		return nil
	})

	agent.ReloadSystemPrompt(context.Background(), "base prompt")
	want := "base prompt\n\n## Extra Rules\nBe concise."
	if agent.config.SystemPrompt != want {
		t.Errorf("prompt = %q, want %q", agent.config.SystemPrompt, want)
	}
}

func TestReloadSystemPrompt_HookFires(t *testing.T) {
	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)

	var fired bool
	var promptLen int
	agent.Hooks().Register(HookOnInstructionsLoaded, "test", func(_ context.Context, data *HookData) error {
		fired = true
		if v, ok := data.Meta["prompt_length"].(int); ok {
			promptLen = v
		}
		return nil
	})

	agent.ReloadSystemPrompt(context.Background(), "hello world")
	if !fired {
		t.Error("HookOnInstructionsLoaded did not fire")
	}
	if promptLen != 11 {
		t.Errorf("prompt_length = %d, want 11", promptLen)
	}
}

func TestPromptReloader_DetectsChange(t *testing.T) {
	// Create a temp file with initial content.
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	os.WriteFile(path, []byte("initial"), 0644)

	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)
	agent.config.SystemPrompt = "initial"

	reloader := NewPromptReloader(agent, []string{path}, 50*time.Millisecond, func() string {
		data, _ := os.ReadFile(path)
		return string(data)
	})
	reloader.Start()
	defer reloader.Stop()

	// Modify the file.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte("reloaded prompt"), 0644)

	// Wait for the reloader to pick it up.
	time.Sleep(200 * time.Millisecond)

	if agent.config.SystemPrompt != "reloaded prompt" {
		t.Errorf("prompt = %q, want %q", agent.config.SystemPrompt, "reloaded prompt")
	}
}

func TestPromptReloader_Stop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	os.WriteFile(path, []byte("initial"), 0644)

	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)

	reloader := NewPromptReloader(agent, []string{path}, 50*time.Millisecond, func() string {
		data, _ := os.ReadFile(path)
		return string(data)
	})
	reloader.Start()
	reloader.Stop()

	// Modify after stop -- should NOT reload.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte("should not load"), 0644)
	time.Sleep(200 * time.Millisecond)

	// Prompt should still be the original since we stopped before the change.
	if agent.config.SystemPrompt == "should not load" {
		t.Error("reloader should not have fired after Stop()")
	}
}

func TestPromptReloader_MissingFile(t *testing.T) {
	mp := &mockProvider{name: "mock", modelID: "m1", cannedText: "ok"}
	agent, _ := newTestAgent(t, mp)
	agent.config.SystemPrompt = "original"

	// Watch a non-existent file -- should not panic or reload.
	reloader := NewPromptReloader(agent, []string{"/tmp/nonexistent_reload_test_file"}, 50*time.Millisecond, func() string {
		return "should not be called"
	})
	reloader.Start()
	defer reloader.Stop()

	time.Sleep(200 * time.Millisecond)
	if agent.config.SystemPrompt != "original" {
		t.Error("prompt should not change when watching non-existent file")
	}
}
