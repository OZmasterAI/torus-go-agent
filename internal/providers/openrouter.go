package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	t "torus_go_agent/internal/types"
)

const openrouterBaseURL = "https://openrouter.ai/api/v1"

// authStyle controls how the API key is sent in HTTP requests.
type authStyle int

const (
	authBearer authStyle = iota // Authorization: Bearer <key>
	authAPIKey                  // api-key: <key> (Azure OpenAI)
)

// OpenRouterProvider calls any OpenAI-compatible API (OpenRouter, NVIDIA NIM, OpenAI, Grok, Azure, etc).
type OpenRouterProvider struct {
	providerName string
	APIKey       string
	Model        string
	BaseURL      string
	endpointPath string    // override for chat completions path (default: "/chat/completions")
	auth         authStyle
	client       *http.Client
}

// chatEndpoint returns the full URL for the chat completions endpoint.
func (p *OpenRouterProvider) chatEndpoint() string {
	path := p.endpointPath
	if path == "" {
		path = "/chat/completions"
	}
	return p.BaseURL + path
}

// NewOpenRouterProvider creates a provider for OpenRouter models.
func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		providerName: "openrouter",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      openrouterBaseURL,
		client:       &http.Client{},
	}
}

// NewNvidiaProvider creates a provider for NVIDIA NIM API models.
func NewNvidiaProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		providerName: "nvidia",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://integrate.api.nvidia.com/v1",
		client:       &http.Client{},
	}
}

// NewNvidiaFreeRouter creates a Router that distributes requests across
// all free NVIDIA NIM chat models with equal weights and fallback.
func NewNvidiaFreeRouter(apiKey string) *Router {
	models := []string{
		"qwen/qwen3.5-122b-a10b",
		"z-ai/glm4.7",
		"z-ai/glm5",
		"stepfun-ai/step-3.5-flash",
		"minimaxai/minimax-m2.1",
		"minimaxai/minimax-m2.5",
		"deepseek-ai/deepseek-v3.2",
		"deepseek-ai/deepseek-v3.1",
		"deepseek-ai/deepseek-v3.1-terminus",
		"mistralai/devstral-2-123b-instruct-2512",
		"moonshotai/kimi-k2-thinking",
		"moonshotai/kimi-k2-instruct",
		"mistralai/mistral-large-3-675b-instruct-2512",
		"mistralai/magistral-small-2506",
		"mistralai/mamba-codestral-7b-v01",
		"mistralai/mistral-nemo-minitron-8b-8k-instruct",
		"bytedance/seed-oss-36b-instruct",
		"qwen/qwen3-coder-480b-a35b-instruct",
		"openai/gpt-oss-20b",
		"openai/gpt-oss-120b",
		"google/gemma-3-27b-it",
		"google/gemma-2-2b-it",
		"google/gemma-3n-e4b-it",
		"igenius/colosseum_355b_instruct_16k",
		"tiiuae/falcon3-7b-instruct",
		"igenius/italia_10b_instruct_16k",
		"qwen/qwen2.5-coder-7b-instruct",
		"qwen/qwen2-7b-instruct",
		"abacusai/dracarys-llama-3.1-70b-instruct",
		"thudm/chatglm3-6b",
		"baichuan-inc/baichuan2-13b-chat",
		"nvidia/nemotron-3-super-120b-a12b",
		"nvidia/nemotron-3-nano-30b-a3b",
		"nvidia/nvidia-nemotron-nano-9b-v2",
		"nvidia/llama-3.3-nemotron-super-49b-v1",
		"nvidia/llama-3.3-nemotron-super-49b-v1.5",
		"marin/marin-8b-instruct",
		"nv-mistralai/mistral-nemo-12b-instruct",
	}

	first := NewNvidiaProvider(apiKey, models[0])
	router := NewRouter(first)

	var entries []RoutingEntry
	entries = append(entries, RoutingEntry{Key: "nvidia:" + models[0], Weight: 1})
	var fallbacks []string
	fallbacks = append(fallbacks, "nvidia:"+models[0])

	for _, m := range models[1:] {
		p := NewNvidiaProvider(apiKey, m)
		router.AddProvider(p)
		entries = append(entries, RoutingEntry{Key: "nvidia:" + m, Weight: 1})
		fallbacks = append(fallbacks, "nvidia:"+m)
	}

	router.SetWeights(entries)
	router.SetFallbackOrder(fallbacks)
	return router
}

// NewOpenAIProvider creates a native provider for OpenAI models (gpt-4o, o1, etc).
func NewOpenAIProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		providerName: "openai",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://api.openai.com/v1",
		client:       &http.Client{},
	}
}

// NewGrokProvider creates a native provider for xAI Grok models.
func NewGrokProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		providerName: "grok",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://api.x.ai/v1",
		client:       &http.Client{},
	}
}

// NewAzureOpenAIProvider creates a provider for Azure-hosted OpenAI models.
// resource is your Azure resource name, deployment is the model deployment name,
// apiVersion is e.g. "2024-06-01".
func NewAzureOpenAIProvider(apiKey, resource, deployment, apiVersion string) *OpenRouterProvider {
	if apiVersion == "" {
		apiVersion = "2024-06-01"
	}
	baseURL := fmt.Sprintf("https://%s.openai.azure.com/openai/deployments/%s", resource, deployment)
	return &OpenRouterProvider{
		providerName: "azure",
		APIKey:       apiKey,
		Model:        deployment,
		BaseURL:      baseURL,
		endpointPath: "/chat/completions?api-version=" + apiVersion,
		auth:         authAPIKey,
		client:       &http.Client{},
	}
}

// setAuthHeader sets the appropriate auth header based on the provider's auth style.
func (p *OpenRouterProvider) setAuthHeader(req *http.Request) {
	switch p.auth {
	case authAPIKey:
		req.Header.Set("api-key", p.APIKey)
	default:
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
}

type openaiRequest struct {
	Model              string         `json:"model"`
	Messages           []openaiMsg    `json:"messages"`
	Tools              []openaiTool   `json:"tools,omitempty"`
	MaxTokens          int            `json:"max_tokens,omitempty"`
	Stream             bool           `json:"stream,omitempty"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type openaiMsg struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
	Model   string         `json:"model"`
}

type openaiChoice struct {
	Message      openaiRespMsg `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiRespMsg struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolCallFunc `json:"function"`
}

type openaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI streaming chunk types.
type openaiStreamChunk struct {
	ID      string               `json:"id"`
	Choices []openaiStreamChoice `json:"choices"`
	Usage   *openaiUsage         `json:"usage,omitempty"`
	Model   string               `json:"model"`
}

type openaiStreamChoice struct {
	Index        int               `json:"index"`
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type openaiStreamDelta struct {
	Role             string                 `json:"role,omitempty"`
	Content          string                 `json:"content,omitempty"`
	ReasoningContent string                 `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiStreamToolCall `json:"tool_calls,omitempty"`
}

type openaiStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openaiToolCallFunc `json:"function"`
}

// buildMessages converts our message types to OpenAI-format messages.
func buildMessages(systemPrompt string, messages []t.Message) []openaiMsg {
	var apiMsgs []openaiMsg

	if systemPrompt != "" {
		apiMsgs = append(apiMsgs, openaiMsg{Role: "system", Content: systemPrompt})
	}

	for _, m := range messages {
		if m.Role == t.RoleSystem {
			continue
		}
		if m.Role == t.RoleTool {
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					apiMsgs = append(apiMsgs, openaiMsg{
						Role:       "tool",
						Content:    b.Content,
						ToolCallID: b.ToolUseID,
					})
				}
			}
			continue
		}
		if m.Role == t.RoleAssistant {
			var textParts []string
			var toolCalls []openaiToolCall
			for _, b := range m.Content {
				if b.Type == "text" && b.Text != "" {
					textParts = append(textParts, b.Text)
				}
				if b.Type == "tool_use" {
					argsJSON, err := json.Marshal(b.Input)
					if err != nil {
						log.Printf("openrouter: failed to marshal tool input for %q: %v", b.Name, err)
					}
					toolCalls = append(toolCalls, openaiToolCall{
						ID:   b.ID,
						Type: "function",
						Function: openaiToolCallFunc{
							Name:      b.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			msg := openaiMsg{Role: "assistant"}
			if len(textParts) > 0 {
				msg.Content = strings.Join(textParts, "")
			}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			apiMsgs = append(apiMsgs, msg)
		} else {
			var content any
			if len(m.Content) == 1 && m.Content[0].Type == "text" {
				content = m.Content[0].Text
			} else {
				content = m.Content
			}
			apiMsgs = append(apiMsgs, openaiMsg{
				Role:    string(m.Role),
				Content: content,
			})
		}
	}

	// Last defense: if the final message is assistant, append a minimal user
	// nudge. Hooks and compression run after sanitizeMessages and can
	// re-introduce an assistant-ending sequence (prefill unsupported on many models).
	if len(apiMsgs) > 0 && apiMsgs[len(apiMsgs)-1].Role == "assistant" {
		apiMsgs = append(apiMsgs, openaiMsg{Role: "user", Content: "Continue."})
	}

	return apiMsgs
}

// buildTools converts our tool types to OpenAI-format tool declarations.
func buildTools(tools []t.Tool) []openaiTool {
	var apiTools []openaiTool
	for _, tl := range tools {
		apiTools = append(apiTools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        tl.Name,
				Description: tl.Description,
				Parameters:  tl.InputSchema,
			},
		})
	}
	return apiTools
}

// Complete sends a request to the OpenRouter (OpenAI-compatible) API.
func (p *OpenRouterProvider) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	req := openaiRequest{
		Model:     p.Model,
		Messages:  buildMessages(systemPrompt, messages),
		Tools:     buildTools(tools),
		MaxTokens: maxTokens,
	}

	// Enable thinking for NVIDIA NIM models that support it.
	if p.providerName == "nvidia" {
		req.ChatTemplateKwargs = map[string]any{"enable_thinking": true}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.chatEndpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.setAuthHeader(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s API error %d: %s", p.providerName, resp.StatusCode, string(respBody))
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]
	var blocks []t.ContentBlock

	// Reasoning/thinking content (DeepSeek, NIM, Grok)
	if choice.Message.ReasoningContent != "" {
		blocks = append(blocks, t.ContentBlock{Type: "thinking", Text: choice.Message.ReasoningContent})
	}

	// Text content
	if choice.Message.Content != "" {
		blocks = append(blocks, t.ContentBlock{Type: "text", Text: choice.Message.Content})
	}

	// Tool calls
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			log.Printf("openrouter: failed to unmarshal tool arguments for %q: %v", tc.Function.Name, err)
		}
		blocks = append(blocks, t.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: args,
		})
	}

	stopReason := choice.FinishReason
	if len(choice.Message.ToolCalls) > 0 && stopReason == "stop" {
		stopReason = "tool_use"
	}

	return &t.AssistantMessage{
		Message: t.Message{
			Role:    t.RoleAssistant,
			Content: blocks,
		},
		Model:      apiResp.Model,
		StopReason: stopReason,
		Usage: t.Usage{
			InputTokens:  apiResp.Usage.PromptTokens,
			OutputTokens: apiResp.Usage.CompletionTokens,
			TotalTokens:  apiResp.Usage.TotalTokens,
		},
	}, nil
}

func (p *OpenRouterProvider) Name() string   { return p.providerName }
func (p *OpenRouterProvider) ModelID() string { return p.Model }

// StreamComplete sends a streaming request to the OpenRouter (OpenAI-compatible) API.
func (p *OpenRouterProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	req := openaiRequest{
		Model:     p.Model,
		Messages:  buildMessages(systemPrompt, messages),
		Tools:     buildTools(tools),
		MaxTokens: maxTokens,
		Stream:    true,
	}

	// Enable thinking for NVIDIA NIM models that support it.
	if p.providerName == "nvidia" {
		req.ChatTemplateKwargs = map[string]any{"enable_thinking": true}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.chatEndpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.setAuthHeader(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("openrouter: failed to read error response body: %v", err)
		}
		return nil, fmt.Errorf("%s API error %d: %s", p.providerName, resp.StatusCode, string(respBody))
	}

	ch := make(chan t.StreamEvent, 32)
	go p.parseOpenAISSE(ctx, resp, ch)
	return ch, nil
}

// parseOpenAISSE reads SSE events from an OpenAI-compatible streaming response.
func (p *OpenRouterProvider) parseOpenAISSE(ctx context.Context, resp *http.Response, ch chan<- t.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()
	defer func() {
		if r := recover(); r != nil {
			ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("%s stream panic: %v", p.providerName, r)}
		}
	}()

	var (
		model      string
		textBuf    strings.Builder
		toolCalls  []t.ContentBlock  // accumulated tool calls
		toolArgs   []strings.Builder // accumulated arguments per tool call index
		usage      t.Usage
		stopReason string
		finished   bool // guard against duplicate finish chunks
	)

	aborted := false
	send := func(ev t.StreamEvent) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			if !aborted {
				aborted = true
				select {
				case ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("aborted: %w", ctx.Err())}:
				default:
					// channel full, receiver gone — drop the abort event
				}
			}
			return false
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk openaiStreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}

		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage != nil {
			usage = t.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
			}
			send(t.StreamEvent{Type: t.EventUsage, Usage: &usage})
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Text content
		if delta.Content != "" {
			textBuf.WriteString(delta.Content)
			if !send(t.StreamEvent{
				Type:         t.EventTextDelta,
				ContentIndex: 0,
				Text:         delta.Content,
			}) {
				return
			}
		}

		// Reasoning/thinking content (DeepSeek, NIM, Grok)
		if delta.ReasoningContent != "" {
			if !send(t.StreamEvent{
				Type:         t.EventThinkingDelta,
				ContentIndex: 0,
				Text:         delta.ReasoningContent,
			}) {
				return
			}
		}

		// Tool calls
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			for len(toolCalls) <= idx {
				toolCalls = append(toolCalls, t.ContentBlock{Type: "tool_use"})
				toolArgs = append(toolArgs, strings.Builder{})
			}
			if tc.ID != "" {
				toolCalls[idx].ID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCalls[idx].Name = tc.Function.Name
				contentIdx := idx + 1 // text block is index 0
				if !send(t.StreamEvent{
					Type:         t.EventToolUseStart,
					ContentIndex: contentIdx,
					ID:           tc.ID,
					Name:         tc.Function.Name,
				}) {
					return
				}
			}
			if tc.Function.Arguments != "" {
				toolArgs[idx].WriteString(tc.Function.Arguments)
				contentIdx := idx + 1
				if !send(t.StreamEvent{
					Type:         t.EventInputDelta,
					ContentIndex: contentIdx,
					InputDelta:   tc.Function.Arguments,
				}) {
					return
				}
			}
		}

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			if finished {
				continue // ignore duplicate finish chunks
			}
			finished = true
			stopReason = *choice.FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		send(t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("sse read: %w", err)})
		return
	}

	// Assemble final response
	var blocks []t.ContentBlock
	if textBuf.Len() > 0 {
		blocks = append(blocks, t.ContentBlock{Type: "text", Text: textBuf.String()})
	}
	for i, tc := range toolCalls {
		var args map[string]any
		if i < len(toolArgs) {
			if err := json.Unmarshal([]byte(toolArgs[i].String()), &args); err != nil {
				log.Printf("openrouter: failed to unmarshal stream tool arguments for %q: %v", tc.Name, err)
			}
		}
		tc.Input = args
		blocks = append(blocks, tc)
	}

	if len(toolCalls) > 0 && stopReason == "stop" {
		stopReason = "tool_use"
	}

	assembled := &t.AssistantMessage{
		Message:    t.Message{Role: t.RoleAssistant, Content: blocks},
		Model:      model,
		StopReason: stopReason,
		Usage:      usage,
	}

	send(t.StreamEvent{
		Type:       t.EventMessageStop,
		StopReason: stopReason,
		Response:   assembled,
	})
}

