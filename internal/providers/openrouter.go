package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	t "go_sdk_agent/internal/types"
)

const openrouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterProvider calls any OpenAI-compatible API (OpenRouter, NVIDIA NIM, etc).
type OpenRouterProvider struct {
	providerName string
	APIKey       string
	Model        string
	BaseURL      string
	client       *http.Client
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

type openaiRequest struct {
	Model     string       `json:"model"`
	Messages  []openaiMsg  `json:"messages"`
	Tools     []openaiTool `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
	Stream    bool         `json:"stream,omitempty"`
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
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
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
	Role      string                 `json:"role,omitempty"`
	Content   string                 `json:"content,omitempty"`
	ToolCalls []openaiStreamToolCall `json:"tool_calls,omitempty"`
}

type openaiStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openaiToolCallFunc `json:"function"`
}

// Complete sends a request to the OpenRouter (OpenAI-compatible) API.
func (p *OpenRouterProvider) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	var apiMsgs []openaiMsg

	// System prompt as first message
	if systemPrompt != "" {
		apiMsgs = append(apiMsgs, openaiMsg{Role: "system", Content: systemPrompt})
	}

	// Convert messages
	for _, m := range messages {
		if m.Role == t.RoleSystem {
			continue
		}
		if m.Role == t.RoleTool {
			// Tool results in OpenAI format
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
		// Check if assistant message contains tool_use blocks
		if m.Role == t.RoleAssistant {
			var textParts []string
			var toolCalls []openaiToolCall
			for _, b := range m.Content {
				if b.Type == "text" && b.Text != "" {
					textParts = append(textParts, b.Text)
				}
				if b.Type == "tool_use" {
					argsJSON, _ := json.Marshal(b.Input)
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

	// Convert tools
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

	req := openaiRequest{
		Model:     p.Model,
		Messages:  apiMsgs,
		Tools:     apiTools,
		MaxTokens: maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	_ = len(body) // debug: request size available if needed

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

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
		// Dump the request for debugging
		fmt.Fprintf(os.Stderr, "[openrouter] ERROR %d. Request body:\n%s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("openrouter API error %d: %s", resp.StatusCode, string(respBody))
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

	// Text content
	if choice.Message.Content != "" {
		blocks = append(blocks, t.ContentBlock{Type: "text", Text: choice.Message.Content})
	}

	// Tool calls
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
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
					argsJSON, _ := json.Marshal(b.Input)
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
			apiMsgs = append(apiMsgs, openaiMsg{Role: string(m.Role), Content: content})
		}
	}

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

	req := openaiRequest{
		Model:     p.Model,
		Messages:  apiMsgs,
		Tools:     apiTools,
		MaxTokens: maxTokens,
		Stream:    true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	_ = len(body) // debug: stream request size available if needed

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "[openrouter] ERROR %d. Request body:\n%s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("openrouter API error %d: %s", resp.StatusCode, string(respBody))
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
			ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("openrouter stream panic: %v", r)}
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
				ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("aborted: %w", ctx.Err())}
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
			_ = json.Unmarshal([]byte(toolArgs[i].String()), &args)
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

// ExtractTextOpenAI extracts text from an OpenAI-format response.
func ExtractTextOpenAI(msg *t.AssistantMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}
