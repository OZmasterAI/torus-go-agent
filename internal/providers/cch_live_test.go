package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tp "torus_go_agent/internal/types"
)

// TestCCH_LiveAnthropicAPI verifies that our CCH attestation is accepted
// by the real Anthropic API when using an OAuth token.
func TestCCH_LiveAnthropicAPI(t *testing.T) {
	key, err := GetAnthropicKey()
	if err != nil || !IsOAuthToken(key) {
		key = os.Getenv("ANTHROPIC_API_KEY")
		if key == "" || !IsOAuthToken(key) {
			t.Skip("no OAuth token available — skipping live CCH test")
		}
	}
	t.Logf("Using OAuth token: %s...%s", key[:15], key[len(key)-4:])

	p := NewAnthropicProvider(key, "claude-haiku-4-5-20251001")

	messages := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "Say just the word 'pong'. Nothing else."}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch, err := p.StreamComplete(ctx, "", messages, nil, 64)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "403") || strings.Contains(errStr, "401") {
			t.Fatalf("CCH attestation REJECTED by Anthropic API: %v", err)
		}
		t.Fatalf("request failed: %v", err)
	}

	var text string
	var gotStop bool
	for ev := range ch {
		switch ev.Type {
		case tp.EventTextDelta:
			text += ev.Text
		case tp.EventError:
			if strings.Contains(ev.Error.Error(), "403") || strings.Contains(ev.Error.Error(), "401") {
				t.Fatalf("CCH attestation REJECTED during stream: %v", ev.Error)
			}
			t.Fatalf("stream error: %v", ev.Error)
		case tp.EventMessageStop:
			gotStop = true
			t.Logf("Model: %s", ev.Response.Model)
			t.Logf("Usage: in=%d out=%d cache_read=%d cache_write=%d",
				ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens,
				ev.Response.Usage.CacheReadTokens, ev.Response.Usage.CacheWriteTokens)
		}
	}

	text = strings.TrimSpace(text)
	t.Logf("Response: %q", text)

	if !gotStop {
		t.Fatal("stream ended without message_stop")
	}
	if text == "" {
		t.Fatal("empty response text")
	}

	t.Log("PASS: CCH attestation accepted by Anthropic API")
}
