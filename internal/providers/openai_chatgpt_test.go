package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	tp "torus_go_agent/internal/types"
)

// mockChatGPTTransport is an http.RoundTripper that captures the outgoing
// request body and replays a canned SSE stream, so tests can assert what the
// provider actually sent without touching the network.
type mockChatGPTTransport struct {
	statusCode   int
	body         string
	capturedBody []byte
}

func (m *mockChatGPTTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		m.capturedBody, _ = io.ReadAll(req.Body)
	}
	sc := m.statusCode
	if sc == 0 {
		sc = 200
	}
	return &http.Response{
		StatusCode: sc,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
	}, nil
}

// collectResponsesSSE feeds a synthetic SSE payload straight into
// parseResponsesSSE and drains the resulting StreamEvents.
func collectResponsesSSE(sse string) (events []tp.StreamEvent, final *tp.AssistantMessage) {
	p := NewOpenAIChatGPTProvider("test-token", "test-account", "gpt-5")
	ch := make(chan tp.StreamEvent, 128)
	go func() {
		p.parseResponsesSSE(strings.NewReader(sse), ch)
		close(ch)
	}()
	for ev := range ch {
		events = append(events, ev)
		if ev.Type == tp.EventMessageStop {
			final = ev.Response
		}
	}
	return events, final
}

func firstError(events []tp.StreamEvent) error {
	for _, ev := range events {
		if ev.Type == tp.EventError {
			return ev.Error
		}
	}
	return nil
}

func joinTextDeltas(events []tp.StreamEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Type == tp.EventTextDelta {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

// minimal well-formed terminal frame reused across tests.
const sseCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"total_tokens\":3}}}\n\n"

// ── Finding #1: max_output_tokens is populated (and defaulted for maxTokens<=0) ──

func TestOpenAIChatGPTMaxOutputTokens(t *testing.T) {
	// t.Setenv forbids t.Parallel; use a temp HOME so no real credential file is
	// found and auth() deterministically falls back to the constructor token.
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name      string
		maxTokens int
		want      float64 // JSON numbers decode to float64
	}{
		{"positive value is forwarded", 4096, 4096},
		{"large value is forwarded", 200000, 200000},
		{"zero defaults to 8192", 0, 8192},
		{"negative defaults to 8192", -1, 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := &mockChatGPTTransport{statusCode: 200, body: sseCompleted}
			p := NewOpenAIChatGPTProvider("test-token", "test-account", "gpt-5")
			p.client = &http.Client{Transport: mt}

			msgs := []tp.Message{{Role: tp.RoleUser, Content: []tp.ContentBlock{{Type: "text", Text: "hi"}}}}
			ch, err := p.StreamComplete(context.Background(), "system", msgs, nil, tt.maxTokens)
			if err != nil {
				t.Fatalf("StreamComplete returned error: %v", err)
			}
			for range ch { // drain so the request completes
			}

			var body map[string]any
			if err := json.Unmarshal(mt.capturedBody, &body); err != nil {
				t.Fatalf("captured body is not valid JSON: %v (body=%q)", err, string(mt.capturedBody))
			}
			got, ok := body["max_output_tokens"]
			if !ok {
				t.Fatalf("max_output_tokens missing from request body: %s", string(mt.capturedBody))
			}
			if got != tt.want {
				t.Errorf("max_output_tokens = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── Finding #5: multi-line data: frames are concatenated into one event ──

func TestParseResponsesSSEMultiLineData(t *testing.T) {
	t.Parallel()

	singleLine := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello, world\"}\n\n" + sseCompleted

	// The same event, split across two data: lines per the SSE spec.
	multiLine := "data: {\"type\":\"response.output_text.delta\",\n" +
		"data: \"delta\":\"Hello, world\"}\n" +
		"\n" + sseCompleted

	tests := []struct {
		name string
		sse  string
	}{
		{"single line (common case unchanged)", singleLine},
		{"split across two data lines", multiLine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events, final := collectResponsesSSE(tt.sse)
			if got := joinTextDeltas(events); got != "Hello, world" {
				t.Errorf("streamed text = %q, want %q", got, "Hello, world")
			}
			if final == nil {
				t.Fatal("no final message assembled")
			}
			var text string
			for _, b := range final.Content {
				if b.Type == "text" {
					text += b.Text
				}
			}
			if text != "Hello, world" {
				t.Errorf("assembled text = %q, want %q", text, "Hello, world")
			}
		})
	}
}

// ── Finding #3: error frame code+message live at the top level and are surfaced ──

func TestParseResponsesSSEErrorEvent(t *testing.T) {
	t.Parallel()

	sse := "data: {\"type\":\"error\",\"code\":\"server_error\",\"message\":\"upstream boom\"}\n\n"
	events, _ := collectResponsesSSE(sse)

	err := firstError(events)
	if err == nil {
		t.Fatal("expected an error event, got none")
	}
	if !strings.Contains(err.Error(), "server_error") {
		t.Errorf("error %q does not include the code %q", err.Error(), "server_error")
	}
	if !strings.Contains(err.Error(), "upstream boom") {
		t.Errorf("error %q does not include the message %q", err.Error(), "upstream boom")
	}
}

// ── Finding #2: response.failed is only transient when actually retryable ──

func TestParseResponsesSSEResponseFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sse           string
		wantTransient bool
		wantSubstr    string
	}{
		{
			name:          "content policy failure is permanent",
			sse:           "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"content_policy_violation\",\"message\":\"blocked by content policy\"}}}\n\n",
			wantTransient: false,
			wantSubstr:    "blocked by content policy",
		},
		{
			name:          "rate limit failure is retryable",
			sse:           "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"rate_limit_error\",\"message\":\"slow down\"}}}\n\n",
			wantTransient: true,
			wantSubstr:    "slow down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events, _ := collectResponsesSSE(tt.sse)
			err := firstError(events)
			if err == nil {
				t.Fatal("expected an error event, got none")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error %q does not include failure reason %q", err.Error(), tt.wantSubstr)
			}
			var te *tp.TransientError
			isTransient := errors.As(err, &te)
			if isTransient != tt.wantTransient {
				t.Errorf("transient = %v, want %v (err=%q)", isTransient, tt.wantTransient, err.Error())
			}
		})
	}
}
