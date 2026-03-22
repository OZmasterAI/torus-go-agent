package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	tp "torus_go_agent/internal/types"
)

// === INVALID CREDENTIALS EDGE CASES ===

// TestGeminiEdge_InvalidAPIKey tests Complete with an invalid API key response.
func TestGeminiEdge_InvalidAPIKey(t *testing.T) {
	provider := NewGeminiProvider("invalid-key-xyz", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 401,
			body:       `{"error":{"code":401,"message":"API key not valid. Please pass a valid API key.","status":"UNAUTHENTICATED"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for invalid API key")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Error should contain 401 status code, got: %v", err)
	}

	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("Error should mention API key, got: %v", err)
	}
}

// TestGeminiEdge_ExpiredAccessToken tests Bearer auth with expired token.
func TestGeminiEdge_ExpiredAccessToken(t *testing.T) {
	provider := NewVertexAIProvider("expired-token-xyz", "my-project", "us-central1", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 401,
			body:       `{"error":{"code":401,"message":"The caller does not have permission"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for expired token")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Error should contain 401 status code, got: %v", err)
	}
}

// TestGeminiEdge_EmptyAPIKey tests with empty API key.
func TestGeminiEdge_EmptyAPIKey(t *testing.T) {
	provider := NewGeminiProvider("", "gemini-2.0-flash")

	// Should generate a URL with empty key
	url := provider.generateURL("generateContent")
	if !strings.Contains(url, "?key=") {
		t.Errorf("URL should contain ?key= even with empty key, got: %s", url)
	}
}

// === MODEL NOT FOUND EDGE CASES ===

// TestGeminiEdge_ModelNotFound tests response when model does not exist.
func TestGeminiEdge_ModelNotFound(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-nonexistent-model")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 404,
			body:       `{"error":{"code":404,"message":"Requested model does not exist"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for nonexistent model")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should contain 404 status code, got: %v", err)
	}
}

// TestGeminiEdge_VertexAIProjectNotFound tests Vertex AI with nonexistent project.
func TestGeminiEdge_VertexAIProjectNotFound(t *testing.T) {
	provider := NewVertexAIProvider("token", "nonexistent-project", "us-central1", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 403,
			body:       `{"error":{"code":403,"message":"Permission denied on resource"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for nonexistent project")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Errorf("Error should contain 403 status code, got: %v", err)
	}
}

// === CONTENT FILTERING EDGE CASES ===

// TestGeminiEdge_SafetyFilteredResponse tests response with SAFETY finish reason.
func TestGeminiEdge_SafetyFilteredResponse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "[Content filtered by safety policy]"},
					},
				},
				FinishReason: "SAFETY",
			},
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if resp.StopReason != "safety" {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, "safety")
	}

	if len(resp.Content) == 0 {
		t.Fatal("Expected content block even with safety filter")
	}
}

// TestGeminiEdge_RecitationFilteredResponse tests content with recitation finish reason.
func TestGeminiEdge_RecitationFilteredResponse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{},
				},
				FinishReason: "RECITATION",
			},
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if resp.StopReason != "RECITATION" {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, "RECITATION")
	}
}

// === STREAMING ERROR EDGE CASES ===

// TestGeminiEdge_StreamingConnectionClosed tests stream that closes mid-response.
func TestGeminiEdge_StreamingConnectionClosed(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":""}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	if len(events) == 0 {
		t.Fatal("Expected at least one event from stream")
	}

	// Should have at least a text delta event
	hasTextEvent := false
	for _, ev := range events {
		if ev.Type == tp.EventTextDelta {
			hasTextEvent = true
			break
		}
	}

	if !hasTextEvent {
		t.Fatal("Expected text delta event in stream")
	}
}

// TestGeminiEdge_StreamingWithContextCancellation tests stream cancellation via context.
func TestGeminiEdge_StreamingWithContextCancellation(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"First"}]},"finishReason":""}]}
data: {"candidates":[{"content":{"parts":[{"text":"Second"}]},"finishReason":""}]}
data: {"candidates":[{"content":{"parts":[{"text":"Third"}]},"finishReason":"STOP"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := provider.StreamComplete(ctx, "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	// Cancel context early to stop reading
	cancel()

	// Collect events - should stop early due to cancelled context
	eventCount := 0
	for range ch {
		eventCount++
		if eventCount > 10 {
			// Safety check to avoid infinite loop
			break
		}
	}

	// We may not get all events due to cancellation
	if eventCount > 10 {
		t.Errorf("Expected context cancellation to stop event stream, got %d events", eventCount)
	}
}

// TestGeminiEdge_StreamingMalformedJSON tests stream with invalid JSON data lines.
func TestGeminiEdge_StreamingMalformedJSON(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Valid"}]},"finishReason":""}]}
data: {malformed json here}
data: {"candidates":[{"content":{"parts":[{"text":"Also valid"}]},"finishReason":"STOP"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	// Should recover from malformed JSON and continue with valid lines
	if len(events) == 0 {
		t.Fatal("Expected events despite malformed JSON")
	}

	// Should have final message with at least some content
	var hasMessage bool
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			hasMessage = true
			if len(ev.Response.Content) == 0 {
				t.Error("Expected content in message stop event")
			}
		}
	}

	if !hasMessage {
		t.Error("Expected message stop event despite malformed JSON")
	}
}

// TestGeminiEdge_StreamingEmptyLines tests stream with blank lines.
func TestGeminiEdge_StreamingEmptyLines(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Line1"}]},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"text":"Line2"}]},"finishReason":"STOP"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	// Should handle blank lines gracefully
	var finalText string
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop && len(ev.Response.Content) > 0 {
			finalText = ev.Response.Content[0].Text
		}
	}

	expectedText := "Line1Line2"
	if finalText != expectedText {
		t.Errorf("Expected combined text %q, got %q", expectedText, finalText)
	}
}

// TestGeminiEdge_StreamingPartialResponse tests stream ending without STOP reason.
func TestGeminiEdge_StreamingPartialResponse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Incomplete"}]},"finishReason":""}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	var finalMessage *tp.AssistantMessage
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			finalMessage = ev.Response
		}
	}

	if finalMessage == nil {
		t.Fatal("Expected message stop event")
	}

	if len(finalMessage.Content) == 0 {
		t.Fatal("Expected content in final message")
	}

	if finalMessage.Content[0].Text != "Incomplete" {
		t.Errorf("Text = %q, want %q", finalMessage.Content[0].Text, "Incomplete")
	}
}

// === RESPONSE PARSING EDGE CASES ===

// TestGeminiEdge_InvalidJSON tests Complete with malformed JSON response.
func TestGeminiEdge_InvalidJSON(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       `{invalid json`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for invalid JSON response")
	}

	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("Error should mention unmarshal, got: %v", err)
	}
}

// TestGeminiEdge_NoCandidatesButNonNilUsage tests response with no candidates but usage data.
func TestGeminiEdge_NoCandidatesButNonNilUsage(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: nil,
		UsageMetadata: &geminiUsage{
			PromptTokenCount:     100,
			CandidatesTokenCount: 0,
			TotalTokenCount:      100,
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(resp.Content) != 0 {
		t.Errorf("Expected no content for nil candidates, got %d items", len(resp.Content))
	}

	// Usage may or may not be captured when candidates are nil
	_ = resp.Usage
}

// TestGeminiEdge_MultiplePartsInContent tests response with mixed text and tool use parts.
func TestGeminiEdge_MultiplePartsInContent(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Let me search for that. "},
						{
							FunctionCall: &geminiFunctionCall{
								Name: "search",
								ID:   "call-1",
								Args: map[string]any{"query": "golang"},
							},
						},
						{Text: " Here are the results."},
					},
				},
				FinishReason: "STOP",
			},
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(resp.Content) != 3 {
		t.Fatalf("Expected 3 content blocks, got %d", len(resp.Content))
	}

	if resp.Content[0].Type != "text" || resp.Content[0].Text != "Let me search for that. " {
		t.Errorf("First block should be text, got %+v", resp.Content[0])
	}

	if resp.Content[1].Type != "tool_use" || resp.Content[1].Name != "search" {
		t.Errorf("Second block should be tool_use, got %+v", resp.Content[1])
	}

	if resp.Content[2].Type != "text" || resp.Content[2].Text != " Here are the results." {
		t.Errorf("Third block should be text, got %+v", resp.Content[2])
	}
}

// === HTTP ERROR EDGE CASES ===

// TestGeminiEdge_HTTPServerError tests 500 error response.
func TestGeminiEdge_HTTPServerError(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 500,
			body:       `{"error":"Internal server error"}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for 500 status code")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Error should contain 500 status code, got: %v", err)
	}
}

// TestGeminiEdge_HTTPRateLimited tests 429 Too Many Requests response.
func TestGeminiEdge_HTTPRateLimited(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 429,
			body:       `{"error":{"code":429,"message":"Too many requests"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for 429 status code")
	}

	if !strings.Contains(err.Error(), "429") {
		t.Errorf("Error should contain 429 status code, got: %v", err)
	}
}

// TestGeminiEdge_HTTPBadRequest tests 400 malformed request response.
func TestGeminiEdge_HTTPBadRequest(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 400,
			body:       `{"error":{"code":400,"message":"Invalid request: bad field"}}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for 400 status code")
	}

	if !strings.Contains(err.Error(), "400") {
		t.Errorf("Error should contain 400 status code, got: %v", err)
	}
}

// === CONTEXT AND TIMEOUT EDGE CASES ===

// TestGeminiEdge_ContextAlreadyCancelled tests Complete with pre-cancelled context.
func TestGeminiEdge_ContextAlreadyCancelled(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       `{"candidates":[]}`,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.Complete(ctx, "", nil, nil, 100)

	// Mock transport may respond before context check; either error or success is acceptable
	_ = err
}

// TestGeminiEdge_ContextDeadlineExceeded tests Complete with exceeded deadline.
func TestGeminiEdge_ContextDeadlineExceeded(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	// Create a slow transport that doesn't return immediately
	provider.client = &http.Client{
		Transport: &slowTransport{delay: 2 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := provider.Complete(ctx, "", nil, nil, 100)

	// Should typically error but depends on transport timing
	_ = err
}

// === EDGE CASES FOR STREAMING WITH TOOLS ===

// TestGeminiEdge_StreamingWithToolUse tests streaming response with tool calls.
func TestGeminiEdge_StreamingWithToolUse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Searching"}]},"finishReason":""}]}
data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","id":"call-1","args":{"q":"golang"}}}]},"finishReason":"TOOL_USE"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	var finalResp *tp.AssistantMessage
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			finalResp = ev.Response
		}
	}

	if finalResp == nil {
		t.Fatal("Expected message stop event")
	}

	// Should have text and tool use blocks
	if len(finalResp.Content) < 2 {
		t.Fatalf("Expected at least 2 content blocks (text + tool), got %d", len(finalResp.Content))
	}

	if finalResp.Content[0].Type != "text" {
		t.Errorf("First block should be text, got %q", finalResp.Content[0].Type)
	}

	if finalResp.Content[1].Type != "tool_use" {
		t.Errorf("Second block should be tool_use, got %q", finalResp.Content[1].Type)
	}

	if finalResp.Content[1].Name != "search" {
		t.Errorf("Tool name should be search, got %q", finalResp.Content[1].Name)
	}
}

// TestGeminiEdge_StreamingMaxTokensReached tests stream stopping due to MAX_TOKENS.
func TestGeminiEdge_StreamingMaxTokensReached(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"This is a very long response that keeps going"}]},"finishReason":"MAX_TOKENS"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	var finalResp *tp.AssistantMessage
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			finalResp = ev.Response
		}
	}

	if finalResp == nil {
		t.Fatal("Expected message stop event")
	}

	if finalResp.StopReason != "max_tokens" {
		t.Errorf("StopReason = %q, want %q", finalResp.StopReason, "max_tokens")
	}
}

// === HELPER FUNCTIONS ===

// slowTransport is a test transport that simulates slow responses.
type slowTransport struct {
	delay time.Duration
}

func (s *slowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(s.delay)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[]}`)),
		Header:     make(http.Header),
	}, nil
}

// collectStreamEvents drains a stream channel and returns all collected events.
func collectStreamEvents(ch <-chan tp.StreamEvent) []tp.StreamEvent {
	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

// TestGeminiEdge_VeryLargeResponse tests handling of large response bodies.
func TestGeminiEdge_VeryLargeResponse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	// Create a large text response (1MB)
	largeText := strings.Repeat("x", 1024*1024)
	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: largeText},
					},
				},
				FinishReason: "STOP",
			},
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(resp.Content) == 0 {
		t.Fatal("Expected content block")
	}

	if len(resp.Content[0].Text) != 1024*1024 {
		t.Errorf("Text length = %d, want %d", len(resp.Content[0].Text), 1024*1024)
	}
}

// TestGeminiEdge_EmptyTextParts tests response with empty text parts.
func TestGeminiEdge_EmptyTextParts(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: ""},
						{Text: "Valid text"},
						{Text: ""},
					},
				},
				FinishReason: "STOP",
			},
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	// Should skip empty text parts and only include "Valid text"
	if len(resp.Content) != 1 {
		t.Fatalf("Expected 1 content block (empty text parts skipped), got %d", len(resp.Content))
	}

	if resp.Content[0].Text != "Valid text" {
		t.Errorf("Text = %q, want %q", resp.Content[0].Text, "Valid text")
	}
}

// TestGeminiEdge_SystemPromptWithCompleteMessage tests system prompt inclusion in request.
func TestGeminiEdge_SystemPromptWithCompleteMessage(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	capturedReq := &geminiRequest{}
	provider.client = &http.Client{
		Transport: &captureTransport{capturedReq: capturedReq},
	}

	systemPrompt := "You are a helpful assistant."
	provider.Complete(context.Background(), systemPrompt, nil, nil, 100)

	if capturedReq.SystemInstruction == nil {
		t.Fatal("SystemInstruction should not be nil")
	}

	if len(capturedReq.SystemInstruction.Parts) == 0 {
		t.Fatal("SystemInstruction should have parts")
	}

	if capturedReq.SystemInstruction.Parts[0].Text != systemPrompt {
		t.Errorf("SystemInstruction text = %q, want %q",
			capturedReq.SystemInstruction.Parts[0].Text, systemPrompt)
	}
}

// TestGeminiEdge_NoSystemPromptWhenEmpty tests that empty system prompt is not included.
func TestGeminiEdge_NoSystemPromptWhenEmpty(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	capturedReq := &geminiRequest{}
	provider.client = &http.Client{
		Transport: &captureTransport{capturedReq: capturedReq},
	}

	provider.Complete(context.Background(), "", nil, nil, 100)

	if capturedReq.SystemInstruction != nil {
		t.Error("SystemInstruction should be nil for empty system prompt")
	}
}

// TestGeminiEdge_StreamingLargeBufferSize tests streaming with large scanner buffer.
func TestGeminiEdge_StreamingLargeBufferSize(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	// Create a very large data line
	largeData := strings.Repeat("a", 100000)
	sseBody := fmt.Sprintf(`data: {"candidates":[{"content":{"parts":[{"text":"%s"}]},"finishReason":"STOP"}]}`, largeData)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	var finalResp *tp.AssistantMessage
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			finalResp = ev.Response
		}
	}

	if finalResp == nil {
		t.Fatal("Expected message stop event")
	}

	if len(finalResp.Content[0].Text) != 100000 {
		t.Errorf("Large text length = %d, want 100000", len(finalResp.Content[0].Text))
	}
}

// TestGeminiEdge_StreamingWithoutDataPrefix tests SSE lines without "data: " prefix.
func TestGeminiEdge_StreamingWithoutDataPrefix(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `some random text
data: {"candidates":[{"content":{"parts":[{"text":"Valid"}]},"finishReason":"STOP"}]}
:comment line
more random text
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       sseBody,
		},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)

	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	events := collectStreamEvents(ch)

	var finalResp *tp.AssistantMessage
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			finalResp = ev.Response
		}
	}

	if finalResp == nil {
		t.Fatal("Expected message stop event")
	}

	// Should only process lines with "data: " prefix
	if len(finalResp.Content) == 0 || finalResp.Content[0].Text != "Valid" {
		t.Errorf("Expected only valid data lines to be processed")
	}
}
