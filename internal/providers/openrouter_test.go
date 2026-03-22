package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	tp "torus_go_agent/internal/types"
)

// newMockHTTPResponse creates a mock HTTP response with the given status code and body string.
func newMockHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// TestNewOpenRouterProvider verifies provider name and model ID are set correctly.
func TestNewOpenRouterProvider(t *testing.T) {
	apiKey := "test-key"
	model := "mistral-7b"

	p := NewOpenRouterProvider(apiKey, model)

	if p == nil {
		t.Fatal("NewOpenRouterProvider returned nil")
	}
	if p.Name() != "openrouter" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "openrouter")
	}
	if p.ModelID() != model {
		t.Fatalf("ModelID() = %q, want %q", p.ModelID(), model)
	}
	if p.APIKey != apiKey {
		t.Fatalf("APIKey = %q, want %q", p.APIKey, apiKey)
	}
	if p.BaseURL != openrouterBaseURL {
		t.Fatalf("BaseURL = %q, want %q", p.BaseURL, openrouterBaseURL)
	}
	if p.client == nil {
		t.Fatal("client should not be nil")
	}
}

// TestNewNvidiaProvider verifies NVIDIA provider is configured correctly.
func TestNewNvidiaProvider(t *testing.T) {
	apiKey := "nvidia-key"
	model := "llama-70b"

	p := NewNvidiaProvider(apiKey, model)

	if p == nil {
		t.Fatal("NewNvidiaProvider returned nil")
	}
	if p.Name() != "nvidia" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "nvidia")
	}
	if p.ModelID() != model {
		t.Fatalf("ModelID() = %q, want %q", p.ModelID(), model)
	}
	if p.BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Fatalf("BaseURL = %q, want NIM endpoint", p.BaseURL)
	}
}

// TestNewOpenAIProvider verifies OpenAI provider is configured correctly.
func TestNewOpenAIProvider(t *testing.T) {
	apiKey := "sk-..."
	model := "gpt-4o"

	p := NewOpenAIProvider(apiKey, model)

	if p == nil {
		t.Fatal("NewOpenAIProvider returned nil")
	}
	if p.Name() != "openai" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "openai")
	}
	if p.ModelID() != model {
		t.Fatalf("ModelID() = %q, want %q", p.ModelID(), model)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want OpenAI endpoint", p.BaseURL)
	}
}

// TestNewGrokProvider verifies Grok provider is configured correctly.
func TestNewGrokProvider(t *testing.T) {
	apiKey := "grok-key"
	model := "grok-2"

	p := NewGrokProvider(apiKey, model)

	if p == nil {
		t.Fatal("NewGrokProvider returned nil")
	}
	if p.Name() != "grok" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "grok")
	}
	if p.ModelID() != model {
		t.Fatalf("ModelID() = %q, want %q", p.ModelID(), model)
	}
	if p.BaseURL != "https://api.x.ai/v1" {
		t.Fatalf("BaseURL = %q, want Grok endpoint", p.BaseURL)
	}
}

// TestNewAzureOpenAIProvider verifies Azure provider configuration.
func TestNewAzureOpenAIProvider(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		resource   string
		deployment string
		apiVersion string
		wantName   string
		wantModel  string
		wantURL    string
	}{
		{
			name:       "with explicit api version",
			apiKey:     "azure-key",
			resource:   "myresource",
			deployment: "gpt-4-deployment",
			apiVersion: "2024-08-01",
			wantName:   "azure",
			wantModel:  "gpt-4-deployment",
			wantURL:    "https://myresource.openai.azure.com/openai/deployments/gpt-4-deployment",
		},
		{
			name:       "with default api version",
			apiKey:     "azure-key",
			resource:   "another",
			deployment: "my-deployment",
			apiVersion: "",
			wantName:   "azure",
			wantModel:  "my-deployment",
			wantURL:    "https://another.openai.azure.com/openai/deployments/my-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAzureOpenAIProvider(tt.apiKey, tt.resource, tt.deployment, tt.apiVersion)

			if p == nil {
				t.Fatal("NewAzureOpenAIProvider returned nil")
			}
			if p.Name() != tt.wantName {
				t.Fatalf("Name() = %q, want %q", p.Name(), tt.wantName)
			}
			if p.ModelID() != tt.wantModel {
				t.Fatalf("ModelID() = %q, want %q", p.ModelID(), tt.wantModel)
			}
			if p.BaseURL != tt.wantURL {
				t.Fatalf("BaseURL = %q, want %q", p.BaseURL, tt.wantURL)
			}
			if tt.apiVersion != "" && !strings.Contains(p.endpointPath, tt.apiVersion) {
				t.Fatalf("endpointPath should contain api-version=%q", tt.apiVersion)
			}
			// Default version should be set if empty
			if tt.apiVersion == "" && !strings.Contains(p.endpointPath, "2024-06-01") {
				t.Fatalf("endpointPath should contain default version 2024-06-01")
			}
			if p.auth != authAPIKey {
				t.Fatalf("auth style should be authAPIKey for Azure, got %v", p.auth)
			}
		})
	}
}

// TestSetAuthHeaderBearer verifies Bearer token auth header.
func TestSetAuthHeaderBearer(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "model")
	req, _ := http.NewRequest("POST", "http://test", nil)

	p.setAuthHeader(req)

	authHeader := req.Header.Get("Authorization")
	expectedAuth := "Bearer test-key"
	if authHeader != expectedAuth {
		t.Fatalf("Authorization header = %q, want %q", authHeader, expectedAuth)
	}
}

// TestSetAuthHeaderAPIKey verifies API key header for Azure.
func TestSetAuthHeaderAPIKey(t *testing.T) {
	p := NewAzureOpenAIProvider("azure-key", "res", "deploy", "2024-06-01")
	req, _ := http.NewRequest("POST", "http://test", nil)

	p.setAuthHeader(req)

	apiKeyHeader := req.Header.Get("api-key")
	if apiKeyHeader != "azure-key" {
		t.Fatalf("api-key header = %q, want %q", apiKeyHeader, "azure-key")
	}

	// Authorization header should not be set for API key auth
	authHeader := req.Header.Get("Authorization")
	if authHeader != "" {
		t.Fatalf("Authorization header should be empty for Azure, got %q", authHeader)
	}
}

// TestChatEndpointDefault verifies default endpoint path.
func TestChatEndpointDefault(t *testing.T) {
	p := NewOpenRouterProvider("key", "model")
	endpoint := p.chatEndpoint()

	expected := openrouterBaseURL + "/chat/completions"
	if endpoint != expected {
		t.Fatalf("chatEndpoint() = %q, want %q", endpoint, expected)
	}
}

// TestChatEndpointCustom verifies custom endpoint path.
func TestChatEndpointCustom(t *testing.T) {
	p := NewAzureOpenAIProvider("key", "res", "deploy", "2024-06-01")
	endpoint := p.chatEndpoint()

	// Azure has a custom endpointPath with api-version query param
	if !strings.Contains(endpoint, "api-version=2024-06-01") {
		t.Fatalf("chatEndpoint() should contain api-version param, got %q", endpoint)
	}
}

// TestOpenRouterCompleteBasicTextResponse verifies Complete with a simple text response.
func TestOpenRouterCompleteBasicTextResponse(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-3.5")

	// Mock response body
	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Model: "gpt-3.5-turbo",
	}

	respBodyJSON, _ := json.Marshal(respBody)

	// Inject mock HTTP client
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
			{Type: "text", Text: "Hello!"},
		},
	}

	result, err := p.Complete(ctx, "You are helpful", []tp.Message{msg}, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if result.Model != "gpt-3.5-turbo" {
		t.Fatalf("Model = %q, want %q", result.Model, "gpt-3.5-turbo")
	}

	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("expected 1 text content block, got %d", len(result.Content))
	}

	if result.Content[0].Text != "Hello, world!" {
		t.Fatalf("content = %q, want %q", result.Content[0].Text, "Hello, world!")
	}

	if result.Usage.InputTokens != 10 {
		t.Fatalf("InputTokens = %d, want 10", result.Usage.InputTokens)
	}

	if result.Usage.OutputTokens != 5 {
		t.Fatalf("OutputTokens = %d, want 5", result.Usage.OutputTokens)
	}
}

// TestOpenRouterCompleteWithToolCalls verifies Complete with tool calls in response.
func TestOpenRouterCompleteWithToolCalls(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	respBody := openaiResponse{
		Choices: []openaiChoice{
			{
				Message: openaiRespMsg{
					Role:    "assistant",
					Content: "I'll call a tool",
					ToolCalls: []openaiToolCall{
						{
							ID:   "call-123",
							Type: "function",
							Function: openaiToolCallFunc{
								Name:      "get_weather",
								Arguments: `{"location":"NYC"}`,
							},
						},
					},
				},
				FinishReason: "stop",
			},
		},
		Usage: openaiUsage{
			PromptTokens:     20,
			CompletionTokens: 10,
			TotalTokens:      30,
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
			{Type: "text", Text: "What is the weather?"},
		},
	}

	result, err := p.Complete(ctx, "You are helpful", []tp.Message{msg}, nil, 100)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	// Verify tool_use content blocks are present
	toolBlocks := 0
	textBlocks := 0
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			toolBlocks++
			if block.ID != "call-123" {
				t.Fatalf("tool call ID = %q, want %q", block.ID, "call-123")
			}
			if block.Name != "get_weather" {
				t.Fatalf("tool name = %q, want %q", block.Name, "get_weather")
			}
		} else if block.Type == "text" {
			textBlocks++
		}
	}

	if toolBlocks != 1 {
		t.Fatalf("expected 1 tool_use block, got %d", toolBlocks)
	}

	if result.StopReason != "tool_use" {
		t.Fatalf("StopReason should be tool_use when tool calls present, got %q", result.StopReason)
	}
}

// TestOpenRouterCompleteHTTPError verifies error handling for HTTP failures.
func TestOpenRouterCompleteHTTPError(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	errorResp := `{"error": {"message": "Invalid API key"}}`

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 401,
			body:       errorResp,
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error for non-200 status")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should mention status 401, got: %v", err)
	}
}

// TestOpenRouterCompleteNoChoices verifies error when response has no choices.
func TestOpenRouterCompleteNoChoices(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	respBody := openaiResponse{
		Choices: []openaiChoice{},
		Usage: openaiUsage{
			PromptTokens:     10,
			CompletionTokens: 0,
			TotalTokens:      10,
		},
	}

	respBodyJSON, _ := json.Marshal(respBody)

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       string(respBodyJSON),
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error when choices is empty")
	}

	if !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("error should mention 'no choices', got: %v", err)
	}
}

// TestOpenRouterCompleteInvalidJSON verifies error handling for malformed response.
func TestOpenRouterCompleteInvalidJSON(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 200,
			body:       `{invalid json}`,
		},
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("Complete should return error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("error should mention unmarshal, got: %v", err)
	}
}

// mockTransport implements http.RoundTripper for testing.
type mockTransport struct {
	statusCode int
	body       string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return newMockHTTPResponse(m.statusCode, m.body), nil
}

// TestOpenRouterStreamCompleteBasic verifies streaming with text chunks.
func TestOpenRouterStreamCompleteBasic(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	// Build SSE stream response
	sseBody := `data: {"id":"1","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
data: {"id":"2","model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}
data: {"id":"3","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}
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
		t.Fatal("expected stream events, got none")
	}

	// Verify we got text delta events
	var textDeltas []string
	for _, ev := range events {
		if ev.Type == tp.EventTextDelta {
			textDeltas = append(textDeltas, ev.Text)
		}
	}

	if len(textDeltas) == 0 {
		t.Fatal("expected text delta events, got none")
	}

	fullText := strings.Join(textDeltas, "")
	if fullText != "Hello world" {
		t.Fatalf("accumulated text = %q, want %q", fullText, "Hello world")
	}

	// Verify final message stop event
	var messageStop tp.StreamEvent
	for _, ev := range events {
		if ev.Type == tp.EventMessageStop {
			messageStop = ev
			break
		}
	}

	if messageStop.Type != tp.EventMessageStop {
		t.Fatal("expected EventMessageStop in stream")
	}

	if messageStop.StopReason != "stop" {
		t.Fatalf("StopReason = %q, want %q", messageStop.StopReason, "stop")
	}
}

// TestOpenRouterStreamCompleteHTTPError verifies error handling for stream HTTP failures.
func TestOpenRouterStreamCompleteHTTPError(t *testing.T) {
	p := NewOpenRouterProvider("test-key", "gpt-4")

	p.client = &http.Client{
		Transport: &mockTransport{
			statusCode: 429,
			body:       `{"error": "rate limited"}`,
		},
	}

	ctx := context.Background()
	_, err := p.StreamComplete(ctx, "system", nil, nil, 100)

	if err == nil {
		t.Fatal("StreamComplete should return error for non-200 status")
	}

	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error should mention 429, got: %v", err)
	}
}

// TestProviderInterfaceImplementation verifies that OpenRouterProvider implements tp.Provider.
func TestProviderInterfaceImplementation(t *testing.T) {
	var _ tp.Provider = (*OpenRouterProvider)(nil)

	// Verify all required methods exist and have correct signatures.
	p := NewOpenRouterProvider("key", "model")

	// Name should return string
	if name := p.Name(); name != "openrouter" {
		t.Fatalf("Name() = %q", name)
	}

	// ModelID should return string
	if id := p.ModelID(); id != "model" {
		t.Fatalf("ModelID() = %q", id)
	}
}
