package providers

import (
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

// OpenRouterProvider calls the OpenRouter API (OpenAI-compatible format).
type OpenRouterProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

// NewOpenRouterProvider creates a provider for OpenRouter models.
func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: openrouterBaseURL,
		client:  &http.Client{},
	}
}

type openaiRequest struct {
	Model    string        `json:"model"`
	Messages []openaiMsg   `json:"messages"`
	Tools    []openaiTool  `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
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
	for _, t := range tools {
		apiTools = append(apiTools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
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

	// Debug: log request body to stderr
	fmt.Fprintf(os.Stderr, "[openrouter] request size: %d bytes, model: %s, messages: %d, tools: %d\n", len(body), p.Model, len(apiMsgs), len(apiTools))

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

func (p *OpenRouterProvider) Name() string   { return "openrouter" }
func (p *OpenRouterProvider) ModelID() string { return p.Model }

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
