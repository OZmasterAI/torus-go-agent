package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	tp "torus_go_agent/internal/types"
)

// mockGeminiTransport implements http.RoundTripper for testing Gemini provider.
type mockGeminiTransport struct {
	statusCode int
	body       string
}

func (m *mockGeminiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
	}, nil
}

// TestNewGeminiProvider verifies that NewGeminiProvider creates a valid provider.
func TestNewGeminiProvider(t *testing.T) {
	apiKey := "test-api-key"
	model := "gemini-2.0-flash"

	provider := NewGeminiProvider(apiKey, model)

	if provider == nil {
		t.Fatal("NewGeminiProvider returned nil")
	}

	if provider.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "gemini")
	}

	if provider.ModelID() != model {
		t.Errorf("ModelID() = %q, want %q", provider.ModelID(), model)
	}

	if provider.APIKey != apiKey {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, apiKey)
	}

	if provider.Model != model {
		t.Errorf("Model = %q, want %q", provider.Model, model)
	}

	if provider.auth != geminiAuthAPIKey {
		t.Errorf("auth = %v, want %v", provider.auth, geminiAuthAPIKey)
	}

	if provider.baseURL != geminiBaseURL {
		t.Errorf("baseURL = %q, want %q", provider.baseURL, geminiBaseURL)
	}

	if provider.modelPrefix != "models/" {
		t.Errorf("modelPrefix = %q, want %q", provider.modelPrefix, "models/")
	}
}

// TestNewVertexAIProvider verifies that NewVertexAIProvider creates a valid Vertex AI provider.
func TestNewVertexAIProvider(t *testing.T) {
	accessToken := "test-access-token"
	project := "my-project"
	region := "us-central1"
	model := "gemini-2.0-flash"

	provider := NewVertexAIProvider(accessToken, project, region, model)

	if provider == nil {
		t.Fatal("NewVertexAIProvider returned nil")
	}

	if provider.Name() != "vertex" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "vertex")
	}

	if provider.ModelID() != model {
		t.Errorf("ModelID() = %q, want %q", provider.ModelID(), model)
	}

	if provider.APIKey != accessToken {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, accessToken)
	}

	if provider.auth != geminiAuthBearer {
		t.Errorf("auth = %v, want %v", provider.auth, geminiAuthBearer)
	}

	expectedBaseURL := "https://us-central1-aiplatform.googleapis.com/v1"
	if provider.baseURL != expectedBaseURL {
		t.Errorf("baseURL = %q, want %q", provider.baseURL, expectedBaseURL)
	}

	expectedPrefix := "projects/my-project/locations/us-central1/publishers/google/models/"
	if provider.modelPrefix != expectedPrefix {
		t.Errorf("modelPrefix = %q, want %q", provider.modelPrefix, expectedPrefix)
	}
}

// TestGenerateURLGemini verifies URL generation for Gemini (API key auth).
func TestGenerateURLGemini(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	tests := []struct {
		action string
		want   string
	}{
		{
			action: "generateContent",
			want:   "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=test-key",
		},
		{
			action: "streamGenerateContent",
			want:   "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:streamGenerateContent?alt=sse&key=test-key",
		},
	}

	for _, tt := range tests {
		got := provider.generateURL(tt.action)
		if got != tt.want {
			t.Errorf("generateURL(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// TestGenerateURLVertexAI verifies URL generation for Vertex AI (Bearer auth).
func TestGenerateURLVertexAI(t *testing.T) {
	provider := NewVertexAIProvider("access-token", "my-project", "us-central1", "gemini-2.0-flash")

	tests := []struct {
		action string
		want   string
	}{
		{
			action: "generateContent",
			want:   "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent",
		},
		{
			action: "streamGenerateContent",
			want:   "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:streamGenerateContent?alt=sse",
		},
	}

	for _, tt := range tests {
		got := provider.generateURL(tt.action)
		if got != tt.want {
			t.Errorf("generateURL(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// TestSetGeminiAuthAPIKey verifies API key auth header handling.
func TestSetGeminiAuthAPIKey(t *testing.T) {
	provider := NewGeminiProvider("my-key", "gemini-2.0-flash")
	req, _ := http.NewRequest("POST", "https://example.com", nil)

	provider.setGeminiAuth(req)

	// API key auth should not set Authorization header (uses query param instead).
	if auth := req.Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization header should be empty for API key auth, got %q", auth)
	}
}

// TestSetGeminiAuthBearer verifies Bearer token auth header handling.
func TestSetGeminiAuthBearer(t *testing.T) {
	provider := NewVertexAIProvider("my-token", "project", "region", "model")
	req, _ := http.NewRequest("POST", "https://example.com", nil)

	provider.setGeminiAuth(req)

	expectedAuth := "Bearer my-token"
	if auth := req.Header.Get("Authorization"); auth != expectedAuth {
		t.Errorf("Authorization = %q, want %q", auth, expectedAuth)
	}
}

// TestGeminiCompleteSuccess verifies a successful non-streaming completion.
func TestGeminiCompleteSuccess(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Hello, world!"},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: &geminiUsage{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	resp, err := provider.Complete(context.Background(), "system prompt", nil, nil, 100)

	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if resp.Model != "gemini-2.0-flash" {
		t.Errorf("Model = %q, want %q", resp.Model, "gemini-2.0-flash")
	}

	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}

	if resp.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", resp.Content[0].Type, "text")
	}

	if resp.Content[0].Text != "Hello, world!" {
		t.Errorf("Content[0].Text = %q, want %q", resp.Content[0].Text, "Hello, world!")
	}

	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, "end_turn")
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", resp.Usage.InputTokens)
	}

	if resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens = %d, want 5", resp.Usage.OutputTokens)
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
}

// TestGeminiCompleteWithToolUse verifies completion with tool use blocks.
func TestGeminiCompleteWithToolUse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{
							FunctionCall: &geminiFunctionCall{
								Name: "get_weather",
								ID:   "call-123",
								Args: map[string]any{"location": "NYC"},
							},
						},
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

	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}

	if resp.Content[0].Type != "tool_use" {
		t.Errorf("Content[0].Type = %q, want %q", resp.Content[0].Type, "tool_use")
	}

	if resp.Content[0].Name != "get_weather" {
		t.Errorf("Content[0].Name = %q, want %q", resp.Content[0].Name, "get_weather")
	}

	if resp.Content[0].ID != "call-123" {
		t.Errorf("Content[0].ID = %q, want %q", resp.Content[0].ID, "call-123")
	}

	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, "end_turn")
	}
}

// TestGeminiCompleteAPIError verifies error handling for API responses.
func TestGeminiCompleteAPIError(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 401,
			body:       `{"error":"Invalid API key"}`,
		},
	}

	_, err := provider.Complete(context.Background(), "", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for API error response")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Error should contain status code 401, got: %v", err)
	}
}

// TestGeminiCompleteEmptyResponse verifies handling of empty candidates.
func TestGeminiCompleteEmptyResponse(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{},
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
		t.Errorf("Content should be empty for empty candidates, got %d items", len(resp.Content))
	}
}

// TestGeminiStreamCompleteSuccess verifies SSE streaming response parsing.
func TestGeminiStreamCompleteSuccess(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":""}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}
data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}]}
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

	var events []tp.StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	if len(events) == 0 {
		t.Fatal("Expected at least one event from stream")
	}

	var messageStop *tp.StreamEvent
	for i := range events {
		if events[i].Type == tp.EventMessageStop {
			messageStop = &events[i]
			break
		}
	}

	if messageStop == nil {
		t.Fatal("Expected EventMessageStop event")
	}

	if messageStop.Response.Model != "gemini-2.0-flash" {
		t.Errorf("Model = %q, want %q", messageStop.Response.Model, "gemini-2.0-flash")
	}

	if len(messageStop.Response.Content) == 0 {
		t.Fatal("Expected content in final response")
	}

	expectedText := "Hello world"
	if messageStop.Response.Content[0].Text != expectedText {
		t.Errorf("Text = %q, want %q", messageStop.Response.Content[0].Text, expectedText)
	}
}

// TestGeminiStreamCompleteAPIError verifies SSE error handling.
func TestGeminiStreamCompleteAPIError(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{
			statusCode: 400,
			body:       `{"error":"Bad request"}`,
		},
	}

	_, err := provider.StreamComplete(context.Background(), "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Expected error for API error response")
	}

	if !strings.Contains(err.Error(), "400") {
		t.Errorf("Error should contain status code 400, got: %v", err)
	}
}

// TestToGeminiRole verifies role mapping.
func TestToGeminiRole(t *testing.T) {
	tests := []struct {
		role tp.Role
		want string
	}{
		{tp.RoleAssistant, "model"},
		{tp.RoleTool, "user"},
		{tp.RoleUser, "user"},
	}

	for _, tt := range tests {
		got := toGeminiRole(tt.role)
		if got != tt.want {
			t.Errorf("toGeminiRole(%v) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

// TestGeminiStopReason verifies stop reason mapping.
func TestGeminiStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   string
	}{
		{"STOP", "end_turn"},
		{"MAX_TOKENS", "max_tokens"},
		{"SAFETY", "safety"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		got := geminiStopReason(tt.reason)
		if got != tt.want {
			t.Errorf("geminiStopReason(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

// TestToGeminiContents verifies message conversion to Gemini format.
func TestToGeminiContents(t *testing.T) {
	messages := []tp.Message{
		{
			Role: tp.RoleUser,
			Content: []tp.ContentBlock{
				{Type: "text", Text: "Hello"},
			},
		},
		{
			Role: tp.RoleAssistant,
			Content: []tp.ContentBlock{
				{Type: "text", Text: "Hi there"},
			},
		},
	}

	contents := toGeminiContents(messages)

	if len(contents) != 2 {
		t.Fatalf("Expected 2 contents, got %d", len(contents))
	}

	if contents[0].Role != "user" {
		t.Errorf("contents[0].Role = %q, want %q", contents[0].Role, "user")
	}

	if contents[1].Role != "model" {
		t.Errorf("contents[1].Role = %q, want %q", contents[1].Role, "model")
	}

	if contents[0].Parts[0].Text != "Hello" {
		t.Errorf("contents[0].Parts[0].Text = %q, want %q", contents[0].Parts[0].Text, "Hello")
	}
}

// TestToGeminiTools verifies tool conversion to Gemini format.
func TestToGeminiTools(t *testing.T) {
	tools := []tp.Tool{
		{
			Name:        "get_weather",
			Description: "Get weather for a location",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
				},
			},
		},
	}

	geminiTools := toGeminiTools(tools)

	if len(geminiTools) != 1 {
		t.Fatalf("Expected 1 tool declaration, got %d", len(geminiTools))
	}

	if len(geminiTools[0].FunctionDeclarations) != 1 {
		t.Fatalf("Expected 1 function declaration, got %d", len(geminiTools[0].FunctionDeclarations))
	}

	fd := geminiTools[0].FunctionDeclarations[0]
	if fd.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", fd.Name, "get_weather")
	}

	if fd.Description != "Get weather for a location" {
		t.Errorf("Description = %q, want %q", fd.Description, "Get weather for a location")
	}
}

// TestToGeminiToolsEmpty verifies empty tools handling.
func TestToGeminiToolsEmpty(t *testing.T) {
	geminiTools := toGeminiTools(nil)

	if len(geminiTools) != 0 {
		t.Errorf("Expected no tools for nil input, got %d", len(geminiTools))
	}

	geminiTools = toGeminiTools([]tp.Tool{})

	if len(geminiTools) != 0 {
		t.Errorf("Expected no tools for empty input, got %d", len(geminiTools))
	}
}

// captureTransport captures the request body for inspection.
type captureTransport struct {
	capturedReq *geminiRequest
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	json.Unmarshal(body, c.capturedReq)

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{{Text: "ok"}},
				},
			},
		},
	}
	respBodyJSON, _ := json.Marshal(mockResp)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(respBodyJSON)),
		Header:     make(http.Header),
	}, nil
}

// captureStreamTransport captures request and returns streaming response.
type captureStreamTransport struct {
	capturedReq *geminiRequest
}

func (c *captureStreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	json.Unmarshal(body, c.capturedReq)

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`data: {"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)),
		Header:     make(http.Header),
	}, nil
}

// TestGeminiCompleteDefaultMaxTokens verifies default max tokens handling.
func TestGeminiCompleteDefaultMaxTokens(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	capturedReq := &geminiRequest{}
	provider.client = &http.Client{
		Transport: &captureTransport{capturedReq: capturedReq},
	}

	provider.Complete(context.Background(), "", nil, nil, 0)

	if capturedReq.GenerationConfig == nil {
		t.Fatal("GenerationConfig should not be nil")
	}

	if capturedReq.GenerationConfig.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want 8192", capturedReq.GenerationConfig.MaxOutputTokens)
	}
}

// TestGeminiStreamCompleteDefaultMaxTokens verifies default max tokens in streaming.
func TestGeminiStreamCompleteDefaultMaxTokens(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash")

	capturedReq := &geminiRequest{}
	provider.client = &http.Client{
		Transport: &captureStreamTransport{capturedReq: capturedReq},
	}

	ch, _ := provider.StreamComplete(context.Background(), "", nil, nil, 0)
	for range ch {
	}

	if capturedReq.GenerationConfig == nil {
		t.Fatal("GenerationConfig should not be nil")
	}

	if capturedReq.GenerationConfig.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want 8192", capturedReq.GenerationConfig.MaxOutputTokens)
	}
}

// TestGeminiStreamThinkingDelta verifies that parts with thought:true emit EventThinkingDelta.
func TestGeminiStreamThinkingDelta(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.5-flash")

	// Simulate Gemini SSE: first chunk has a thinking part, second has regular text.
	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"Let me think...","thought":true}]},"finishReason":""}]}
data: {"candidates":[{"content":{"parts":[{"text":"The answer is 42."}]},"finishReason":"STOP"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{statusCode: 200, body: sseBody},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var thinkingDeltas []tp.StreamEvent
	var textDeltas []tp.StreamEvent
	var messageStop *tp.StreamEvent

	for ev := range ch {
		switch ev.Type {
		case tp.EventThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, ev)
		case tp.EventTextDelta:
			textDeltas = append(textDeltas, ev)
		case tp.EventMessageStop:
			messageStop = &ev
		}
	}

	// Verify thinking delta was emitted.
	if len(thinkingDeltas) != 1 {
		t.Fatalf("Expected 1 thinking delta, got %d", len(thinkingDeltas))
	}
	if thinkingDeltas[0].Text != "Let me think..." {
		t.Errorf("Thinking delta text = %q, want %q", thinkingDeltas[0].Text, "Let me think...")
	}

	// Verify text delta was emitted for the non-thinking part.
	if len(textDeltas) != 1 {
		t.Fatalf("Expected 1 text delta, got %d", len(textDeltas))
	}
	if textDeltas[0].Text != "The answer is 42." {
		t.Errorf("Text delta text = %q, want %q", textDeltas[0].Text, "The answer is 42.")
	}

	// Verify final assembled response contains only regular text (thinking not in textBuf).
	if messageStop == nil {
		t.Fatal("Expected EventMessageStop")
	}
	if messageStop.Response == nil {
		t.Fatal("EventMessageStop.Response is nil")
	}
	if len(messageStop.Response.Content) != 1 {
		t.Fatalf("Expected 1 content block in final response, got %d", len(messageStop.Response.Content))
	}
	if messageStop.Response.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", messageStop.Response.Content[0].Type, "text")
	}
	if messageStop.Response.Content[0].Text != "The answer is 42." {
		t.Errorf("Content[0].Text = %q, want %q", messageStop.Response.Content[0].Text, "The answer is 42.")
	}
}

// TestGeminiCompleteThinkingPart verifies that non-streaming responses tag thinking parts correctly.
func TestGeminiCompleteThinkingPart(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.5-flash")

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Internal reasoning here", Thought: true},
						{Text: "The answer is 42."},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: &geminiUsage{
			PromptTokenCount:     10,
			CandidatesTokenCount: 20,
			TotalTokenCount:      30,
		},
	}

	respBodyJSON, _ := json.Marshal(mockResp)

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{statusCode: 200, body: string(respBodyJSON)},
	}

	resp, err := provider.Complete(context.Background(), "", nil, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(resp.Content))
	}

	// First block should be "thinking" type.
	if resp.Content[0].Type != "thinking" {
		t.Errorf("Content[0].Type = %q, want %q", resp.Content[0].Type, "thinking")
	}
	if resp.Content[0].Text != "Internal reasoning here" {
		t.Errorf("Content[0].Text = %q, want %q", resp.Content[0].Text, "Internal reasoning here")
	}

	// Second block should be regular "text" type.
	if resp.Content[1].Type != "text" {
		t.Errorf("Content[1].Type = %q, want %q", resp.Content[1].Type, "text")
	}
	if resp.Content[1].Text != "The answer is 42." {
		t.Errorf("Content[1].Text = %q, want %q", resp.Content[1].Text, "The answer is 42.")
	}
}

// TestGeminiStreamThinkingOnly verifies a stream with only thinking parts (no regular text).
func TestGeminiStreamThinkingOnly(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.5-flash")

	sseBody := `data: {"candidates":[{"content":{"parts":[{"text":"thinking only","thought":true}]},"finishReason":"STOP"}]}
`

	provider.client = &http.Client{
		Transport: &mockGeminiTransport{statusCode: 200, body: sseBody},
	}

	ch, err := provider.StreamComplete(context.Background(), "", nil, nil, 100)
	if err != nil {
		t.Fatalf("StreamComplete returned error: %v", err)
	}

	var thinkingCount int
	var textCount int
	var messageStop *tp.StreamEvent

	for ev := range ch {
		switch ev.Type {
		case tp.EventThinkingDelta:
			thinkingCount++
		case tp.EventTextDelta:
			textCount++
		case tp.EventMessageStop:
			messageStop = &ev
		}
	}

	if thinkingCount != 1 {
		t.Errorf("Expected 1 thinking delta, got %d", thinkingCount)
	}
	if textCount != 0 {
		t.Errorf("Expected 0 text deltas, got %d", textCount)
	}

	// Final response should have no content blocks (thinking not added to textBuf).
	if messageStop == nil {
		t.Fatal("Expected EventMessageStop")
	}
	if len(messageStop.Response.Content) != 0 {
		t.Errorf("Expected 0 content blocks in final response (thinking excluded from textBuf), got %d", len(messageStop.Response.Content))
	}
}
