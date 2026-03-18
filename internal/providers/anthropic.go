package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	t "go_sdk_agent/internal/types"
)

const anthropicBaseURL = "https://api.anthropic.com/v1"
const anthropicVersion = "2023-06-01"

// AnthropicProvider calls the Anthropic Messages API.
type AnthropicProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

// NewAnthropicProvider creates a provider for Anthropic/Claude models.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: anthropicBaseURL,
		client:  &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    any                 `json:"system,omitempty"` // string or []systemBlock for caching
	Messages  []anthropicMsg      `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Stream    bool                `json:"stream,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// systemBlock is used for prompt caching — system prompt as a content block with cache_control.
type systemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *cacheControl     `json:"cache_control,omitempty"`
}

// contentBlockWithCache wraps a text content block with optional cache_control.
type contentBlockWithCache struct {
	Type         string            `json:"type"`
	Text         string            `json:"text,omitempty"`
	CacheControl *cacheControl     `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicResponse struct {
	ID         string               `json:"id"`
	Type       string               `json:"type"`
	Role       string               `json:"role"`
	Content    []anthropicContent   `json:"content"`
	Model      string               `json:"model"`
	StopReason string               `json:"stop_reason"`
	Usage      anthropicUsage       `json:"usage"`
}

type anthropicContent struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Complete sends a non-streaming request to the Anthropic Messages API.
func (p *AnthropicProvider) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	// Cap maxTokens per model (Sonnet: 64k, Opus: 32k, Haiku: 8k)
	if maxTokens > 64000 {
		maxTokens = 64000
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	// Convert messages
	var apiMsgs []anthropicMsg
	for _, m := range messages {
		if m.Role == t.RoleSystem {
			continue
		}
		// Tool results: Anthropic wants role:"user" with tool_result content blocks
		if m.Role == t.RoleTool {
			var results []map[string]any
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					tr := map[string]any{
						"type":        "tool_result",
						"tool_use_id": b.ToolUseID,
						"content":     b.Content,
					}
					if b.IsError {
						tr["is_error"] = true
					}
					results = append(results, tr)
				}
			}
			if len(results) > 0 {
				apiMsgs = append(apiMsgs, anthropicMsg{Role: "user", Content: results})
			}
			continue
		}
		var content any
		if len(m.Content) == 1 && m.Content[0].Type == "text" {
			content = m.Content[0].Text
		} else {
			content = m.Content
		}
		apiMsgs = append(apiMsgs, anthropicMsg{
			Role:    string(m.Role),
			Content: content,
		})
	}

	// Convert tools
	var apiTools []anthropicTool
	for _, t := range tools {
		apiTools = append(apiTools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	// Build system prompt — OAuth requires Claude Code identity prefix
	var systemBlocks []systemBlock
	if IsOAuthToken(p.APIKey) {
		systemBlocks = append(systemBlocks, systemBlock{
			Type:         "text",
			Text:         "You are Claude Code, Anthropic's official CLI for Claude.",
			CacheControl: &cacheControl{Type: "ephemeral"},
		})
	}
	if systemPrompt != "" {
		systemBlocks = append(systemBlocks, systemBlock{
			Type:         "text",
			Text:         systemPrompt,
			CacheControl: &cacheControl{Type: "ephemeral"},
		})
	}
	var system any
	if len(systemBlocks) > 0 {
		system = systemBlocks
	}

	// Prompt caching: mark last 2 messages with cache_control (max 4 total; system uses up to 2)
	cacheStart := len(apiMsgs) - 2
	if cacheStart < 0 {
		cacheStart = 0
	}
	for i := cacheStart; i < len(apiMsgs); i++ {
		if textStr, ok := apiMsgs[i].Content.(string); ok {
			apiMsgs[i].Content = []contentBlockWithCache{{
				Type: "text", Text: textStr,
				CacheControl: &cacheControl{Type: "ephemeral"},
			}}
		}
	}

	req := anthropicRequest{
		Model:     p.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  apiMsgs,
		Tools:     apiTools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if IsOAuthToken(p.APIKey) {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		httpReq.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		httpReq.Header.Set("user-agent", "claude-cli/1.0.0")
		httpReq.Header.Set("x-app", "cli")
	} else {
		httpReq.Header.Set("x-api-key", p.APIKey)
	}
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Convert to core types
	var blocks []t.ContentBlock
	for _, c := range apiResp.Content {
		block := t.ContentBlock{Type: c.Type}
		switch c.Type {
		case "text":
			block.Text = c.Text
		case "tool_use":
			block.ID = c.ID
			block.Name = c.Name
			block.Input = c.Input
		}
		blocks = append(blocks, block)
	}

	return &t.AssistantMessage{
		Message: t.Message{
			Role:    t.RoleAssistant,
			Content: blocks,
		},
		Model:      apiResp.Model,
		StopReason: apiResp.StopReason,
		Usage: t.Usage{
			InputTokens:      apiResp.Usage.InputTokens,
			OutputTokens:     apiResp.Usage.OutputTokens,
			CacheReadTokens:  apiResp.Usage.CacheReadInputTokens,
			CacheWriteTokens: apiResp.Usage.CacheCreationInputTokens,
			TotalTokens:      apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
		},
	}, nil
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string { return "anthropic" }

// ModelID returns the current model.
func (p *AnthropicProvider) ModelID() string { return p.Model }

// setHeaders configures auth and version headers on an Anthropic API request.
func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if IsOAuthToken(p.APIKey) {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
		req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		req.Header.Set("user-agent", "claude-cli/1.0.0")
		req.Header.Set("x-app", "cli")
	} else {
		req.Header.Set("x-api-key", p.APIKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)
}

// SSE event structs for Anthropic streaming.
type sseMessageStart struct {
	Message struct {
		ID    string         `json:"id"`
		Model string         `json:"model"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
}

type sseContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type sseContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type sseContentBlockStop struct {
	Index int `json:"index"`
}

type sseMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage anthropicUsage `json:"usage"`
}

// StreamComplete sends a streaming request to the Anthropic Messages API.
// It returns a channel of StreamEvents. The final EventMessageStop carries the
// accumulated AssistantMessage. The channel is closed when the stream ends.
func (p *AnthropicProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	if maxTokens > 64000 {
		maxTokens = 64000
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	// Convert messages (same logic as Complete)
	var apiMsgs []anthropicMsg
	for _, m := range messages {
		if m.Role == t.RoleSystem {
			continue
		}
		if m.Role == t.RoleTool {
			var results []map[string]any
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					tr := map[string]any{
						"type":        "tool_result",
						"tool_use_id": b.ToolUseID,
						"content":     b.Content,
					}
					if b.IsError {
						tr["is_error"] = true
					}
					results = append(results, tr)
				}
			}
			if len(results) > 0 {
				apiMsgs = append(apiMsgs, anthropicMsg{Role: "user", Content: results})
			}
			continue
		}
		var content any
		if len(m.Content) == 1 && m.Content[0].Type == "text" {
			content = m.Content[0].Text
		} else {
			content = m.Content
		}
		apiMsgs = append(apiMsgs, anthropicMsg{Role: string(m.Role), Content: content})
	}

	var apiTools []anthropicTool
	for _, tl := range tools {
		apiTools = append(apiTools, anthropicTool{
			Name:        tl.Name,
			Description: tl.Description,
			InputSchema: tl.InputSchema,
		})
	}

	var systemBlocks []systemBlock
	if IsOAuthToken(p.APIKey) {
		systemBlocks = append(systemBlocks, systemBlock{
			Type: "text", Text: "You are Claude Code, Anthropic's official CLI for Claude.",
			CacheControl: &cacheControl{Type: "ephemeral"},
		})
	}
	if systemPrompt != "" {
		systemBlocks = append(systemBlocks, systemBlock{
			Type: "text", Text: systemPrompt,
			CacheControl: &cacheControl{Type: "ephemeral"},
		})
	}
	var system any
	if len(systemBlocks) > 0 {
		system = systemBlocks
	}

	cacheStart := len(apiMsgs) - 2
	if cacheStart < 0 {
		cacheStart = 0
	}
	for i := cacheStart; i < len(apiMsgs); i++ {
		if textStr, ok := apiMsgs[i].Content.(string); ok {
			apiMsgs[i].Content = []contentBlockWithCache{{
				Type: "text", Text: textStr,
				CacheControl: &cacheControl{Type: "ephemeral"},
			}}
		}
	}

	req := anthropicRequest{
		Model:     p.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  apiMsgs,
		Tools:     apiTools,
		Stream:    true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan t.StreamEvent, 32)
	go p.parseAnthropicSSE(ctx, resp, ch)
	return ch, nil
}

// parseAnthropicSSE reads SSE events from the response body and sends StreamEvents on ch.
func (p *AnthropicProvider) parseAnthropicSSE(ctx context.Context, resp *http.Response, ch chan<- t.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()
	defer func() {
		if r := recover(); r != nil {
			ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("anthropic stream panic: %v", r)}
		}
	}()

	var (
		model      string
		stopReason string
		usage      t.Usage
		blocks     []t.ContentBlock
		inputJSONs []strings.Builder // accumulated tool input per block index
	)

	send := func(ev t.StreamEvent) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "message_start":
			var ev sseMessageStart
			if json.Unmarshal([]byte(data), &ev) == nil {
				model = ev.Message.Model
				usage.InputTokens = ev.Message.Usage.InputTokens
				usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
				usage.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens
			}

		case "content_block_start":
			var ev sseContentBlockStart
			if json.Unmarshal([]byte(data), &ev) == nil {
				// Grow blocks slice to fit
				for len(blocks) <= ev.Index {
					blocks = append(blocks, t.ContentBlock{})
				}
				for len(inputJSONs) <= ev.Index {
					inputJSONs = append(inputJSONs, strings.Builder{})
				}
				blocks[ev.Index] = t.ContentBlock{
					Type: ev.ContentBlock.Type,
					ID:   ev.ContentBlock.ID,
					Name: ev.ContentBlock.Name,
				}
				if ev.ContentBlock.Type == "tool_use" {
					if !send(t.StreamEvent{
						Type:         t.EventToolUseStart,
						ContentIndex: ev.Index,
						ID:           ev.ContentBlock.ID,
						Name:         ev.ContentBlock.Name,
					}) {
						return
					}
				}
			}

		case "content_block_delta":
			var ev sseContentBlockDelta
			if json.Unmarshal([]byte(data), &ev) == nil {
				switch ev.Delta.Type {
				case "text_delta":
					if ev.Index < len(blocks) {
						blocks[ev.Index].Text += ev.Delta.Text
					}
					if !send(t.StreamEvent{
						Type:         t.EventTextDelta,
						ContentIndex: ev.Index,
						Text:         ev.Delta.Text,
					}) {
						return
					}
				case "input_json_delta":
					if ev.Index < len(inputJSONs) {
						inputJSONs[ev.Index].WriteString(ev.Delta.PartialJSON)
					}
					if !send(t.StreamEvent{
						Type:         t.EventInputDelta,
						ContentIndex: ev.Index,
						InputDelta:   ev.Delta.PartialJSON,
					}) {
						return
					}
				}
			}

		case "content_block_stop":
			var ev sseContentBlockStop
			if json.Unmarshal([]byte(data), &ev) == nil {
				// Finalize tool input JSON
				if ev.Index < len(blocks) && blocks[ev.Index].Type == "tool_use" && ev.Index < len(inputJSONs) {
					var args map[string]any
					_ = json.Unmarshal([]byte(inputJSONs[ev.Index].String()), &args)
					blocks[ev.Index].Input = args
				}
				if !send(t.StreamEvent{Type: t.EventContentBlockStop, ContentIndex: ev.Index}) {
					return
				}
			}

		case "message_delta":
			var ev sseMessageDelta
			if json.Unmarshal([]byte(data), &ev) == nil {
				stopReason = ev.Delta.StopReason
				usage.OutputTokens = ev.Usage.OutputTokens
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
				send(t.StreamEvent{Type: t.EventUsage, Usage: &usage})
			}

		case "message_stop":
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
			return

		case "error":
			send(t.StreamEvent{
				Type:  t.EventError,
				Error: fmt.Errorf("anthropic stream error: %s", data),
			})
			return
		}
	}

	if err := scanner.Err(); err != nil {
		send(t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("sse read: %w", err)})
	}
}

// HasToolUse checks if the response wants to use tools.
func HasToolUse(msg *t.AssistantMessage) bool {
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

// ExtractToolCalls returns all tool_use blocks from a response.
func ExtractToolCalls(msg *t.AssistantMessage) []t.ContentBlock {
	var calls []t.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}
	return calls
}

// ExtractText returns concatenated text from a response.
func ExtractText(msg *t.AssistantMessage) string {
	var parts []string
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}
