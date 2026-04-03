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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiAuthStyle controls how authentication is handled.
type geminiAuthStyle int

const (
	geminiAuthAPIKey geminiAuthStyle = iota // ?key= query param (Gemini API)
	geminiAuthBearer                        // Authorization: Bearer (Vertex AI)
)

// GeminiProvider calls the Google Gemini API or Vertex AI natively.
type GeminiProvider struct {
	providerName string
	APIKey       string // API key or OAuth access token
	Model        string
	baseURL      string          // e.g. "https://generativelanguage.googleapis.com/v1beta"
	modelPrefix  string          // e.g. "models/" (Gemini) or full path for Vertex
	auth         geminiAuthStyle
	client       *http.Client
}

// NewGeminiProvider creates a native provider for Google Gemini models.
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	return &GeminiProvider{
		providerName: "gemini",
		APIKey:       apiKey,
		Model:        model,
		baseURL:      geminiBaseURL,
		modelPrefix:  "models/",
		auth:         geminiAuthAPIKey,
		client:       &http.Client{},
	}
}

// NewVertexAIProvider creates a native provider for Vertex AI (Gemini on Google Cloud).
// accessToken is a Google OAuth2 access token. region is e.g. "us-central1".
func NewVertexAIProvider(accessToken, project, region, model string) *GeminiProvider {
	baseURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1", region)
	modelPrefix := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/", project, region)
	return &GeminiProvider{
		providerName: "vertex",
		APIKey:       accessToken,
		Model:        model,
		baseURL:      baseURL,
		modelPrefix:  modelPrefix,
		auth:         geminiAuthBearer,
		client:       &http.Client{},
	}
}

func (p *GeminiProvider) Name() string   { return p.providerName }
func (p *GeminiProvider) ModelID() string { return p.Model }

// generateURL builds the endpoint URL for a given action (generateContent or streamGenerateContent).
func (p *GeminiProvider) generateURL(action string) string {
	url := fmt.Sprintf("%s/%s%s:%s", p.baseURL, p.modelPrefix, p.Model, action)
	if action == "streamGenerateContent" {
		if p.auth == geminiAuthAPIKey {
			url += "?alt=sse&key=" + p.APIKey
		} else {
			url += "?alt=sse"
		}
	} else if p.auth == geminiAuthAPIKey {
		url += "?key=" + p.APIKey
	}
	return url
}

// setGeminiAuth sets auth headers on the request.
func (p *GeminiProvider) setGeminiAuth(req *http.Request) {
	if p.auth == geminiAuthBearer {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	// API key auth uses query param, already in URL
}

// --- Gemini request/response types ---

type geminiRequest struct {
	Contents         []geminiContent       `json:"contents"`
	Tools            []geminiToolDecl      `json:"tools,omitempty"`
	SystemInstruction *geminiContent       `json:"system_instruction,omitempty"`
	GenerationConfig *geminiGenConfig      `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	Thought          bool                  `json:"thought,omitempty"`
	ThoughtSignature string                `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse   `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	ID   string         `json:"id,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFuncResponse struct {
	Name     string         `json:"name"`
	ID       string         `json:"id,omitempty"`
	Response map[string]any `json:"response"`
}

type geminiToolDecl struct {
	FunctionDeclarations []geminiFuncDecl `json:"functionDeclarations"`
}

type geminiFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int              `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinking  `json:"thinkingConfig,omitempty"`
}

type geminiThinking struct {
	IncludeThoughts bool `json:"includeThoughts"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// --- Conversion: our types → Gemini ---

func toGeminiContents(messages []t.Message) []geminiContent {
	var contents []geminiContent
	for _, m := range messages {
		if m.Role == t.RoleSystem {
			continue
		}
		gc := geminiContent{Role: toGeminiRole(m.Role)}

		if m.Role == t.RoleTool {
			// Tool results → functionResponse parts
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					gc.Parts = append(gc.Parts, geminiPart{
						FunctionResponse: &geminiFuncResponse{
							Name: b.Name,
							ID:   b.ToolUseID,
							Response: map[string]any{"result": b.Content},
						},
					})
				}
			}
		} else if m.Role == t.RoleAssistant {
			// Assistant messages may contain text + function calls
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					if b.Text != "" {
						gc.Parts = append(gc.Parts, geminiPart{Text: b.Text})
					}
				case "tool_use":
					gc.Parts = append(gc.Parts, geminiPart{
						FunctionCall: &geminiFunctionCall{
							Name: b.Name,
							ID:   b.ID,
							Args: b.Input,
						},
					})
				}
			}
		} else {
			// User messages
			for _, b := range m.Content {
				if b.Text != "" {
					gc.Parts = append(gc.Parts, geminiPart{Text: b.Text})
				} else if b.Type != "" {
					log.Printf("[gemini] warning: dropping unsupported content block type %q in user message", b.Type)
				}
			}
		}

		if len(gc.Parts) > 0 {
			contents = append(contents, gc)
		}
	}
	return contents
}

func toGeminiRole(role t.Role) string {
	switch role {
	case t.RoleAssistant:
		return "model"
	case t.RoleTool:
		return "user" // Gemini sends function responses as user role
	default:
		return "user"
	}
}

func toGeminiTools(tools []t.Tool) []geminiToolDecl {
	if len(tools) == 0 {
		return nil
	}
	var decls []geminiFuncDecl
	for _, tl := range tools {
		decls = append(decls, geminiFuncDecl{
			Name:        tl.Name,
			Description: tl.Description,
			Parameters:  tl.InputSchema,
		})
	}
	return []geminiToolDecl{{FunctionDeclarations: decls}}
}

// --- Conversion: Gemini response → our types ---

func fromGeminiResponse(resp *geminiResponse, model string) *t.AssistantMessage {
	if len(resp.Candidates) == 0 {
		return &t.AssistantMessage{
			Message: t.Message{Role: t.RoleAssistant},
			Model:   model,
		}
	}

	cand := resp.Candidates[0]
	var blocks []t.ContentBlock

	for _, part := range cand.Content.Parts {
		if part.Text != "" {
			blockType := "text"
			if part.Thought {
				blockType = "thinking"
			}
			blocks = append(blocks, t.ContentBlock{Type: blockType, Text: part.Text})
		}
		if part.FunctionCall != nil {
			blocks = append(blocks, t.ContentBlock{
				Type:  "tool_use",
				ID:    part.FunctionCall.ID,
				Name:  part.FunctionCall.Name,
				Input: part.FunctionCall.Args,
			})
		}
	}

	stopReason := geminiStopReason(cand.FinishReason)
	usage := t.Usage{}
	if resp.UsageMetadata != nil {
		usage.InputTokens = resp.UsageMetadata.PromptTokenCount
		usage.OutputTokens = resp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = resp.UsageMetadata.TotalTokenCount
	}

	return &t.AssistantMessage{
		Message:    t.Message{Role: t.RoleAssistant, Content: blocks},
		Model:      model,
		StopReason: stopReason,
		Usage:      usage,
	}
}

func geminiStopReason(reason string) string {
	switch reason {
	case "STOP":
		return "end_turn"
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY":
		return "safety"
	default:
		return reason
	}
}

// --- Complete (non-streaming) ---

func (p *GeminiProvider) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	req := geminiRequest{
		Contents: toGeminiContents(messages),
		Tools:    toGeminiTools(tools),
		GenerationConfig: &geminiGenConfig{
			MaxOutputTokens: maxTokens,
			ThinkingConfig:  &geminiThinking{IncludeThoughts: true},
		},
	}
	if systemPrompt != "" {
		req.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.generateURL("generateContent"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	p.setGeminiAuth(httpReq)

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
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return fromGeminiResponse(&apiResp, p.Model), nil
}

// --- StreamComplete (SSE streaming) ---

func (p *GeminiProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	req := geminiRequest{
		Contents: toGeminiContents(messages),
		Tools:    toGeminiTools(tools),
		GenerationConfig: &geminiGenConfig{
			MaxOutputTokens: maxTokens,
			ThinkingConfig:  &geminiThinking{IncludeThoughts: true},
		},
	}
	if systemPrompt != "" {
		req.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.generateURL("streamGenerateContent"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	p.setGeminiAuth(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan t.StreamEvent, 32)
	go p.parseGeminiSSE(ctx, resp, ch)
	return ch, nil
}

func (p *GeminiProvider) parseGeminiSSE(ctx context.Context, resp *http.Response, ch chan<- t.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()
	defer func() {
		if r := recover(); r != nil {
			ch <- t.StreamEvent{Type: t.EventError, Error: fmt.Errorf("gemini stream panic: %v", r)}
		}
	}()

	var (
		textBuf    strings.Builder
		toolCalls  []t.ContentBlock
		usage      t.Usage
		stopReason string
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

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk geminiResponse
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}

		// Usage
		if chunk.UsageMetadata != nil {
			usage = t.Usage{
				InputTokens:  chunk.UsageMetadata.PromptTokenCount,
				OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:  chunk.UsageMetadata.TotalTokenCount,
			}
			send(t.StreamEvent{Type: t.EventUsage, Usage: &usage})
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		cand := chunk.Candidates[0]
		if cand.FinishReason != "" {
			stopReason = geminiStopReason(cand.FinishReason)
		}

		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				if part.Thought {
					if !send(t.StreamEvent{
						Type:         t.EventThinkingDelta,
						ContentIndex: 0,
						Text:         part.Text,
					}) {
						return
					}
				} else {
					textBuf.WriteString(part.Text)
					if !send(t.StreamEvent{
						Type:         t.EventTextDelta,
						ContentIndex: 0,
						Text:         part.Text,
					}) {
						return
					}
				}
			}
			if part.FunctionCall != nil {
				idx := len(toolCalls)
				toolCalls = append(toolCalls, t.ContentBlock{
					Type:  "tool_use",
					ID:    part.FunctionCall.ID,
					Name:  part.FunctionCall.Name,
					Input: part.FunctionCall.Args,
				})
				contentIdx := idx + 1 // text is index 0
				if !send(t.StreamEvent{
					Type:         t.EventToolUseStart,
					ContentIndex: contentIdx,
					ID:           part.FunctionCall.ID,
					Name:         part.FunctionCall.Name,
				}) {
					return
				}
				// Function call args arrive complete in Gemini (not streamed like OpenAI)
				argsJSON, marshalErr := json.Marshal(part.FunctionCall.Args)
				if marshalErr != nil {
					log.Printf("[gemini] warning: could not marshal function call args for %q: %v", part.FunctionCall.Name, marshalErr)
					argsJSON = []byte("{}")  
				}
				if !send(t.StreamEvent{
					Type:         t.EventInputDelta,
					ContentIndex: contentIdx,
					InputDelta:   string(argsJSON),
				}) {
					return
				}
				if !send(t.StreamEvent{
					Type:         t.EventContentBlockStop,
					ContentIndex: contentIdx,
				}) {
					return
				}
			}
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
	blocks = append(blocks, toolCalls...)

	if len(toolCalls) > 0 && stopReason == "end_turn" {
		stopReason = "tool_use"
	}

	assembled := &t.AssistantMessage{
		Message:    t.Message{Role: t.RoleAssistant, Content: blocks},
		Model:      p.Model,
		StopReason: stopReason,
		Usage:      usage,
	}

	send(t.StreamEvent{
		Type:       t.EventMessageStop,
		StopReason: stopReason,
		Response:   assembled,
	})
}
