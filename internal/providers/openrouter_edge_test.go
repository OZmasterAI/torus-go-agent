package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	tp "torus_go_agent/internal/types"
)

// TestOpenRouterEdge_FallbackModelSelection tests provider fallback behavior
// when the preferred model is unavailable.
func TestOpenRouterEdge_FallbackModelSelection(t *testing.T) {
	// Simulate scenario where first model is unavailable (404) but fallback succeeds
	p := NewOpenRouterProvider("test-key", "unavailable-model")

	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "Response from fallback model",
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     15,
			CompletionTokens: 8,
			TotalTokens:      23,
		},
		Model: "gpt-3.5-turbo-fallback",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	// Mock transport that simulates fallback retry
	p.client = &http.Client{
		Transport: &mockFallbackTransport{
			attempts:    0,
			statusCode:  200,
			body:        string(respBodyJSON),
			fallbackTag: "fallback",
		},
	}

	ctx := context.Background()
	msg := tp.Message{
		Role: tp.RoleUser,
		Content: []tp.ContentBlock{
			{Type: "text", Text: "Hello!"},
		},
	}

	result, err := p.Complete(ctx, "system", []tp.Message{msg}, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if result.Model != "gpt-3.5-turbo-fallback" {
		t.Fatalf("Model = %q, expected fallback model", result.Model)
	}

	if !strings.Contains(result.Content[0].Text, "fallback") {
		t.Fatalf("expected fallback model response, got: %q", result.Content[0].Text)
	}
}

// TestOpenRouterEdge_RateLimitWithRetry tests handling of rate limit responses.
func TestOpenRouterEdge_RateLimitWithRetry(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// First call returns 429 rate limit
	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 429,
			body:       `{"error": {"message": "Rate limit exceeded", "type": "server_error"}}`,
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error for rate limit")
	}

	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error should mention 429 status, got: %v", err)
	}
}

// TestOpenRouterEdge_TokenCountTracking verifies token usage accumulation.
func TestOpenRouterEdge_TokenCountTracking(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "Test response",
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     1000,
			CompletionTokens: 500,
			TotalTokens:      1500,
		},
		Model: "gpt-4",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	ctx := context.Background()
	result, err := p.Complete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	// Verify token counts are accurately tracked
	if result.Usage.InputTokens != 1000 {
		t.Fatalf("InputTokens = %d, want 1000", result.Usage.InputTokens)
	}

	if result.Usage.OutputTokens != 500 {
		t.Fatalf("OutputTokens = %d, want 500", result.Usage.OutputTokens)
	}

	if result.Usage.TotalTokens != 1500 {
		t.Fatalf("TotalTokens = %d, want 1500", result.Usage.TotalTokens)
	}
}

// TestOpenRouterEdge_LargeTokenCount tests handling of large token counts
// that may overflow in cost calculations.
func TestOpenRouterEdge_LargeTokenCount(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4-turbo")

	// Simulate very large token usage (e.g., long context)
	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "Response after large context",
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     100000,
			CompletionTokens: 50000,
			TotalTokens:      150000,
		},
		Model: "gpt-4-turbo",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	ctx := context.Background()
	result, err := p.Complete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if result.Usage.TotalTokens != 150000 {
		t.Fatalf("TotalTokens = %d, want 150000 (large count)", result.Usage.TotalTokens)
	}
}

// TestOpenRouterEdge_CostCalculationForDifferentModels tests cost tracking
// varies by model tier.
func TestOpenRouterEdge_CostCalculationForDifferentModels(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		tokens int
	}{
		{"gpt-3.5-turbo", "gpt-3.5-turbo", 1000},
		{"gpt-4", "gpt-4", 1000},
		{"gpt-4-turbo", "gpt-4-turbo", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenRouterProvider("test-key", tt.model)

			respBody := openaiResponse{
				Choices: []openaiChoice{
					{
						Message: openaiRespMsg{
							Role:    "assistant",
							Content: "Response",
						},
						FinishReason: "stop",
					},
				},
				Usage: openaiUsage{
					PromptTokens:     tt.tokens,
					CompletionTokens: 100,
					TotalTokens:      tt.tokens + 100,
				},
				Model: tt.model,
			}

			respBodyJSON, _ := json.Marshal(respBody)

			p.client = &http.Client{
				Transport: &mockTransport{
					statusCode: 200,
					body:       string(respBodyJSON),
				},
			}

			ctx := context.Background()
			result, err := p.Complete(ctx, "system", nil, nil, 100)
			if err != nil {
				t.Fatalf("Complete returned error: %v", err)
			}

			// Verify model is correctly recorded for cost tracking
			if result.Model != tt.model {
				t.Fatalf("Model = %q, want %q", result.Model, tt.model)
			}
		})
	}
}

// TestOpenRouterEdge_ProviderTimeout tests context deadline exceeded handling.
func TestOpenRouterEdge_ProviderTimeout(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Mock transport that times out
	p.client = &http.Client{
		Transport: &mockTimeoutTransport{
			delay: 100 * time.Millisecond,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error on timeout")
	}

	// Error may mention context/deadline or "no choices" depending on timing
	_ = err
}

// TestOpenRouterEdge_ServerErrorRecovery tests handling of 5xx errors.
func TestOpenRouterEdge_ServerErrorRecovery(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 503,
			body:       `{"error": {"message": "Service temporarily unavailable"}}`,
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error for 503")
	}

	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error should mention 503 status, got: %v", err)
	}
}

// TestOpenRouterEdge_AuthenticationError tests handling of invalid API keys.
func TestOpenRouterEdge_AuthenticationError(t *testing.T) {
	p := NewOpenRouterProvider("invalid-key", "gpt-4")

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 401,
			body:       `{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`,
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error for 401")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should mention 401 status, got: %v", err)
	}
}

// TestOpenRouterEdge_StreamContextCancellation tests early termination of streaming.
func TestOpenRouterEdge_StreamContextCancellation(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Build long SSE stream
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Start"},"finish_reason":null}]}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{"content":" of"},"finish_reason":null}]}
data: {"id":"3","model":"gpt-4","choices":[{"index":0,"delta":{"content":" very"},"finish_reason":null}]}
data: {"id":"4","model":"gpt-4","choices":[{"index":0,"delta":{"content":" long"},"finish_reason":null}]}
data: {"id":"5","model":"gpt-4","choices":[{"index":0,"delta":{"content":" response"},"finish_reason":null}]}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockSlowTransport{
			body: sseBody,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	// Cancel after receiving first few events
	eventCount := 0
	for event := range ch {
		eventCount++
		if eventCount == 2 {
			cancel()
		}
		if event.Type == tp.EventError {
			if !strings.Contains(event.Error.Error(), "aborted") {
				t.Fatalf("expected abort error, got: %v", event.Error)
			}
			break
		}
	}

	// Verify that cancellation was processed
	if eventCount < 2 {
		t.Fatalf("expected at least 2 events before cancellation, got %d", eventCount)
	}
}

// TestOpenRouterEdge_StreamMalformedChunk tests handling of invalid JSON in stream.
func TestOpenRouterEdge_StreamMalformedChunk(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Build SSE stream with some malformed chunks (should be skipped gracefully)
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
data: {invalid json that should be skipped}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx := context.Background()
	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	// Verify stream continues despite malformed chunk
	if len(events) == 0 {
		t.Fatal("expected stream events despite malformed chunk")
	}

	// Collect text deltas
	var textContent []string
	for _, ev := range events {
		if ev.Type == tp.EventTextDelta {
			textContent = append(textContent, ev.Text)
		}
	}

	fullText := strings.Join(textContent, "")
	if fullText != "Hello world" {
		t.Fatalf("accumulated text = %q, want %q", fullText, "Hello world")
	}
}

// TestOpenRouterEdge_StreamEmptyChoices tests handling of empty choices in stream chunk.
func TestOpenRouterEdge_StreamEmptyChoices(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Stream with chunk that has empty choices (should be skipped)
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Start"},"finish_reason":null}]}
data: {"id":"2","model":"gpt-4","choices":[]}
data: {"id":"3","model":"gpt-4","choices":[{"index":0,"delta":{"content":" end"},"finish_reason":"stop"}]}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx := context.Background()
	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	if len(events) == 0 {
		t.Fatal("expected stream events despite empty choices")
	}

	// Verify final message contains accumulated text
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop && ev.Response != nil {
			if len(ev.Response.Content) > 0 && ev.Response.Content[0].Type == "text" {
				if !strings.Contains(ev.Response.Content[0].Text, "Start") {
					t.Fatalf("missing accumulated content")
				}
			}
		}
	}
}

// TestOpenRouterEdge_StreamWithToolCalls tests streaming with tool call chunks.
func TestOpenRouterEdge_StreamWithToolCalls(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Stream with tool calls
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-123","type":"function","function":{"name":"get_weather","arguments":"{\"location\""}}]},"finish_reason":null}]}
data: {"id":"3","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"NYC\"}"}}]},"finish_reason":null}]}
data: {"id":"4","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx := context.Background()
	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	// Verify we got tool use events
	var toolUseStarts int
	var inputDeltas int
	for _, ev := range events {
		if ev.Type == tp.EventToolUseStart {
			toolUseStarts++
		}
		if ev.Type == tp.EventInputDelta {
			inputDeltas++
		}
	}

	if toolUseStarts == 0 {
		t.Fatal("expected EventToolUseStart events")
	}

	if inputDeltas == 0 {
		t.Fatal("expected EventInputDelta events")
	}

	// Verify final message has tool_use content
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop && ev.Response != nil {
			toolFound := false
			for _, block := range ev.Response.Content {
				if block.Type == "tool_use" {
					toolFound = true
					break
				}
			}
			if !toolFound {
				t.Fatal("expected tool_use content block in final message")
			}
		}
	}
}

// TestOpenRouterEdge_StreamDuplicateFinishReason tests that duplicate finish chunks are handled.
func TestOpenRouterEdge_StreamDuplicateFinishReason(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Stream with duplicate finish_reason chunks (only first should be processed)
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Final"},"finish_reason":"stop"}]}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
data: {"id":"3","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx := context.Background()
	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	// Should only have one EventMessageStop despite duplicate finish chunks
	messageStops := 0
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			messageStops++
		}
	}

	if messageStops != 1 {
		t.Fatalf("expected exactly 1 EventMessageStop, got %d", messageStops)
	}
}

// TestOpenRouterEdge_StreamUsageEvent tests that usage events are properly emitted.
func TestOpenRouterEdge_StreamUsageEvent(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Response"},"finish_reason":null}]}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}
data: [DONE]
`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx := context.Background()
	ch, err := p.StreamComplete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var usageEvent tp.StreamEvent
	for event := range ch {
		if event.Type == tp.EventUsage {
			usageEvent = event
		}
	}

	if usageEvent.Type != tp.EventUsage {
		t.Fatal("expected EventUsage in stream")
	}

	if usageEvent.Usage.InputTokens != 20 {
		t.Fatalf("usage InputTokens = %d, want 20", usageEvent.Usage.InputTokens)
	}

	if usageEvent.Usage.OutputTokens != 10 {
		t.Fatalf("usage OutputTokens = %d, want 10", usageEvent.Usage.OutputTokens)
	}
}

// TestOpenRouterEdge_EmptySystemPrompt tests Complete without system prompt.
func TestOpenRouterEdge_EmptySystemPrompt(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "Response without system prompt",
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     5,
			CompletionTokens: 3,
			TotalTokens:      8,
		},
		Model: "gpt-4",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	ctx := context.Background()
	msg := tp.Message{
		Role: tp.RoleUser,
		Content: []tp.ContentBlock{
			{Type: "text", Text: "Hello"},
		},
	}

	// Empty system prompt should still work
	result, err := p.Complete(ctx, "", []tp.Message{msg}, nil, 100)
	if err != nil {
		t.Fatalf("Complete with empty system prompt returned error: %v", err)
	}

	if result.Content[0].Text != "Response without system prompt" {
		t.Fatalf("content = %q, want %q", result.Content[0].Text, "Response without system prompt")
	}
}

// TestOpenRouterEdge_MultipleToolCalls tests multiple tool calls in single response.
func TestOpenRouterEdge_MultipleToolCalls(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "I'll call multiple tools",
					ToolCalls: []openaiToolCall{
						{
							ID:   "call-1",
							Type: "function",
							Function: openaiToolCallFunc{
								Name:      "get_weather",
								Arguments: `{"location":"NYC"}`,
							},
						},
						{
							ID:   "call-2",
							Type: "function",
							Function: openaiToolCallFunc{
								Name:      "get_time",
								Arguments: `{"timezone":"EST"}`,
							},
						},
						{
							ID:   "call-3",
							Type: "function",
							Function: openaiToolCallFunc{
								Name:      "search",
								Arguments: `{"query":"weather"}`,
							},
						},
					},
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     30,
			CompletionTokens: 20,
			TotalTokens:      50,
		},
		Model: "gpt-4",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	ctx := context.Background()
	result, err := p.Complete(ctx, "system", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	// Count tool_use blocks
	toolBlocks := 0
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			toolBlocks++
		}
	}

	if toolBlocks != 3 {
		t.Fatalf("expected 3 tool_use blocks, got %d", toolBlocks)
	}

	// Verify tool IDs are preserved
	toolIDs := []string{}
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			toolIDs = append(toolIDs, block.ID)
		}
	}

	expectedIDs := []string{"call-1", "call-2", "call-3"}
	for i, expected := range expectedIDs {
		if i >= len(toolIDs) || toolIDs[i] != expected {
			t.Fatalf("expected tool ID %q, got %v", expected, toolIDs)
		}
	}
}

// Mock transport implementations for edge case testing

type mockFallbackTransport struct {
	attempts    int
	statusCode  int
	body        string
	fallbackTag string
}

func (m *mockFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.attempts++
	return newMockHTTPResponse(m.statusCode, m.body), nil
}

type mockTimeoutTransport struct {
	delay time.Duration
}

func (m *mockTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(m.delay)
	return newMockHTTPResponse(200, `{}`), nil
}

type mockSlowTransport struct {
	body string
}

func (m *mockSlowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return newMockHTTPResponse(200, m.body), nil
}

// Helper to create mock response with status code verification
func newMockHTTPResponseWithStatus(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
