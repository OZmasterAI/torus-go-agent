package providers

// OpenAIChatGPTProvider calls OpenAI's Responses API through the ChatGPT
// subscription backend (https://chatgpt.com/backend-api/codex/responses), the
// path used by "Sign in with ChatGPT". This is distinct from the API-key
// chat/completions path (OpenRouterProvider / NewOpenAIProvider): the request
// is the Responses schema (instructions/input/tools) and the response is a
// stream of response.* SSE events terminated by response.completed (no
// [DONE] sentinel).
//
// Mandatory quirks for the subscription backend (a wrong value here = 400/403):
//   - store: false
//   - instructions must be non-empty
//   - include: ["reasoning.encrypted_content"] (nothing is stored server-side)
//   - originator: codex_cli_rs (whitelisted; others -> 403)

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"strings"

	t "torus_go_agent/internal/types"
)

const (
	openaiChatGPTBaseURL = "https://chatgpt.com/backend-api/codex"
	openaiCodexVersion   = "0.0.1"
)

// OpenAIChatGPTProvider implements types.Provider against the ChatGPT backend.
type OpenAIChatGPTProvider struct {
	accessToken string // fallback token if the credential store is unavailable
	accountID   string // fallback account id
	Model       string
	BaseURL     string
	sessionID   string
	client      *http.Client
}

// NewOpenAIChatGPTProvider builds a subscription-backed OpenAI provider.
func NewOpenAIChatGPTProvider(accessToken, accountID, model string) *OpenAIChatGPTProvider {
	if model == "" {
		model = "gpt-5"
	}
	return &OpenAIChatGPTProvider{
		accessToken: accessToken,
		accountID:   accountID,
		Model:       model,
		BaseURL:     openaiChatGPTBaseURL,
		sessionID:   newUUIDv4(),
		client:      &http.Client{}, // no overall timeout: streams run long
	}
}

func (p *OpenAIChatGPTProvider) Name() string    { return "openai" }
func (p *OpenAIChatGPTProvider) ModelID() string { return p.Model }

// auth returns the freshest available token + account id, refreshing via the
// stored credentials when possible and falling back to construction values.
func (p *OpenAIChatGPTProvider) auth() (token, account string) {
	if creds, err := GetOpenAIAuth(); err == nil && creds.Access != "" {
		account = creds.AccountID
		if account == "" {
			account = p.accountID
		}
		return creds.Access, account
	}
	return p.accessToken, p.accountID
}

// ── Responses API request types ───────────────────────────────────────────────

type responsesRequest struct {
	Model             string          `json:"model"`
	Instructions      string          `json:"instructions"`
	Input             []responsesItem `json:"input"`
	Tools             []responsesTool `json:"tools,omitempty"`
	ToolChoice        string          `json:"tool_choice,omitempty"`
	ParallelToolCalls bool            `json:"parallel_tool_calls"`
	Store             bool            `json:"store"`
	Stream            bool            `json:"stream"`
	Include           []string        `json:"include,omitempty"`
	MaxOutputTokens   int             `json:"max_output_tokens,omitempty"`
}

type responsesItem struct {
	Type string `json:"type"` // "message" | "function_call" | "function_call_output" | "reasoning"
	// message
	Role    string             `json:"role,omitempty"`
	Content []responsesContent `json:"content,omitempty"`
	// function_call
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	// function_call_output
	Output string `json:"output,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"` // "input_text" (user) | "output_text" (assistant)
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string         `json:"type"` // "function"
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// chatgptToolAcc accumulates a streaming function call.
type chatgptToolAcc struct {
	id      string
	name    string
	args    strings.Builder
	index   int
	stopped bool // guards against emitting content_block_stop twice
}

func buildResponsesInput(messages []t.Message) []responsesItem {
	var items []responsesItem
	for _, m := range messages {
		switch m.Role {
		case t.RoleSystem:
			continue // folded into instructions
		case t.RoleTool:
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					items = append(items, responsesItem{
						Type:   "function_call_output",
						CallID: b.ToolUseID,
						Output: b.Content,
					})
				}
			}
		case t.RoleAssistant:
			var text strings.Builder
			flush := func() {
				if text.Len() > 0 {
					items = append(items, responsesItem{
						Type: "message", Role: "assistant",
						Content: []responsesContent{{Type: "output_text", Text: text.String()}},
					})
					text.Reset()
				}
			}
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					text.WriteString(b.Text)
				// Reasoning ("thinking") blocks are intentionally NOT echoed back as
				// `reasoning` input items: the agent loop strips thinking before
				// replay, so the encrypted carrier never reaches here. Wiring the
				// store:false reasoning round-trip end-to-end is deferred.
				case "tool_use":
					flush()
					args, err := json.Marshal(b.Input)
					if err != nil {
						log.Printf("openai-chatgpt: marshal tool input for %q: %v", b.Name, err)
					}
					items = append(items, responsesItem{
						Type: "function_call", Name: b.Name,
						Arguments: string(args), CallID: b.ID,
					})
				}
			}
			flush()
		default: // user
			var text strings.Builder
			for _, b := range m.Content {
				if b.Type == "text" {
					text.WriteString(b.Text)
				}
			}
			items = append(items, responsesItem{
				Type: "message", Role: "user",
				Content: []responsesContent{{Type: "input_text", Text: text.String()}},
			})
		}
	}
	return items
}

func buildResponsesTools(tools []t.Tool) []responsesTool {
	var out []responsesTool
	for _, tl := range tools {
		out = append(out, responsesTool{
			Type: "function", Name: tl.Name,
			Description: tl.Description, Parameters: tl.InputSchema,
		})
	}
	return out
}

// ── Provider interface ────────────────────────────────────────────────────────

// Complete drains a streamed response into a single AssistantMessage.
func (p *OpenAIChatGPTProvider) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	ch, err := p.StreamComplete(ctx, systemPrompt, messages, tools, maxTokens)
	if err != nil {
		return nil, err
	}
	var final *t.AssistantMessage
	for ev := range ch {
		switch ev.Type {
		case t.EventMessageStop:
			final = ev.Response
		case t.EventError:
			if ev.Error != nil {
				return nil, ev.Error
			}
		}
	}
	if final == nil {
		return nil, fmt.Errorf("openai chatgpt: no completion received")
	}
	return final, nil
}

// StreamComplete streams a Responses-API request through the ChatGPT backend.
func (p *OpenAIChatGPTProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	// Mirror gemini.go / anthropic.go: a non-positive maxTokens means "unset",
	// so fall back to a sane default rather than sending 0 (or omitting the cap).
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	instructions := systemPrompt
	if strings.TrimSpace(instructions) == "" {
		instructions = "You are a helpful coding assistant."
	}
	reqBody := responsesRequest{
		Model:             p.Model,
		Instructions:      instructions,
		Input:             buildResponsesInput(messages),
		Tools:             buildResponsesTools(tools),
		ToolChoice:        "auto",
		ParallelToolCalls: false,
		Store:             false,
		Stream:            true,
		Include:           []string{"reasoning.encrypted_content"},
		MaxOutputTokens:   maxTokens,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &t.TransientError{Err: fmt.Errorf("http request: %w", err)}
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		apiErr := fmt.Errorf("openai chatgpt API error %d: %s", resp.StatusCode, string(b))
		if isTransientStatus(resp.StatusCode) {
			return nil, &t.TransientError{Err: apiErr}
		}
		return nil, apiErr
	}

	ch := make(chan t.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.parseResponsesSSE(resp.Body, ch)
	}()
	return ch, nil
}

func (p *OpenAIChatGPTProvider) setHeaders(req *http.Request) {
	token, account := p.auth()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if account != "" {
		req.Header.Set("chatgpt-account-id", account)
	}
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", openaiOriginator)
	req.Header.Set("session-id", p.sessionID)
	req.Header.Set("User-Agent", fmt.Sprintf("codex_cli_rs/%s (%s %s)", openaiCodexVersion, runtime.GOOS, runtime.GOARCH))
}

func lastChatgptTool(m map[string]*chatgptToolAcc, order []string) *chatgptToolAcc {
	if len(order) == 0 {
		return nil
	}
	return m[order[len(order)-1]]
}

// isTransientReadErr reports whether an SSE read error is worth a whole-turn
// retry. Most mid-stream read failures are network interruptions and are
// transient; bufio.ErrTooLong (one frame exceeded the buffer) and context
// cancellation/deadline are permanent — retrying would only hit the same wall.
func isTransientReadErr(err error) bool {
	if errors.Is(err, bufio.ErrTooLong) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// isTransientOpenAIError reports whether an OpenAI Responses error (identified by
// its code and/or message) is worth a whole-turn retry. Rate limits, server
// errors and overload are transient; content-policy, invalid-request and quota
// failures are permanent (retrying only re-spends tokens and re-fails). Unlike
// the Anthropic wire vocabulary, OpenAI uses codes like rate_limit_exceeded and
// server_error, so this must not reuse anthropic.go's isTransientSSEError.
func isTransientOpenAIError(code, message string) bool {
	s := strings.ToLower(code + " " + message)
	switch {
	case strings.Contains(s, "rate_limit"),
		strings.Contains(s, "server_error"),
		strings.Contains(s, "overloaded"),
		strings.Contains(s, "service_unavailable"),
		strings.Contains(s, "temporarily"),
		strings.Contains(s, "try again"):
		return true
	}
	return false
}

// parseResponsesSSE converts the response.* SSE event stream into StreamEvents.
func (p *OpenAIChatGPTProvider) parseResponsesSSE(r io.Reader, ch chan<- t.StreamEvent) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var textBuf, reasoningBuf strings.Builder
	toolByID := map[string]*chatgptToolAcc{}
	var toolOrder []string
	blockIndex := 0
	stopReason := "end_turn"
	var usage *t.Usage
	sawTerminal := false

	// dispatch handles one fully-assembled SSE event frame (possibly built from
	// several concatenated `data:` lines). It returns true when a terminal error
	// was surfaced and parsing must stop.
	dispatch := func(data string) (stop bool) {
		if data == "" || data == "[DONE]" {
			return false
		}

		var ev struct {
			Type      string `json:"type"`
			Delta     string `json:"delta"`
			Arguments string `json:"arguments"`
			// Top-level fields of an "error" frame:
			// {"type":"error","code":...,"message":...}. The Responses API puts
			// these at the frame root, not nested under "error".
			Code    string `json:"code"`
			Message string `json:"message"`
			Item    struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				Name      string `json:"name"`
				CallID    string `json:"call_id"`
				Arguments string `json:"arguments"`
			} `json:"item"`
			Response struct {
				Status string `json:"status"`
				Usage  struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
				IncompleteDetails struct {
					Reason string `json:"reason"`
				} `json:"incomplete_details"`
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false // skip keep-alives / unparseable fragments
		}

		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				textBuf.WriteString(ev.Delta)
				ch <- t.StreamEvent{Type: t.EventTextDelta, Text: ev.Delta, ContentIndex: blockIndex}
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if ev.Delta != "" {
				reasoningBuf.WriteString(ev.Delta)
				// Consumers read thinking text from .Text (see loop.go EventThinkingDelta).
				ch <- t.StreamEvent{Type: t.EventThinkingDelta, Text: ev.Delta, ContentIndex: blockIndex}
			}
		case "response.output_item.added":
			switch ev.Item.Type {
			case "function_call":
				id := ev.Item.CallID
				if id == "" {
					id = ev.Item.ID
				}
				blockIndex++
				ta := &chatgptToolAcc{id: id, name: ev.Item.Name, index: blockIndex}
				toolByID[id] = ta
				toolOrder = append(toolOrder, id)
				ch <- t.StreamEvent{Type: t.EventToolUseStart, ID: id, Name: ev.Item.Name, ContentIndex: blockIndex}
			}
		case "response.function_call_arguments.delta":
			if ta := lastChatgptTool(toolByID, toolOrder); ta != nil && ev.Delta != "" {
				ta.args.WriteString(ev.Delta)
				ch <- t.StreamEvent{Type: t.EventInputDelta, InputDelta: ev.Delta, ContentIndex: ta.index}
			}
		case "response.function_call_arguments.done":
			if ta := lastChatgptTool(toolByID, toolOrder); ta != nil {
				if ta.args.Len() == 0 && ev.Arguments != "" {
					ta.args.WriteString(ev.Arguments)
					ch <- t.StreamEvent{Type: t.EventInputDelta, InputDelta: ev.Arguments, ContentIndex: ta.index}
				}
				if !ta.stopped {
					ta.stopped = true
					ch <- t.StreamEvent{Type: t.EventContentBlockStop, ContentIndex: ta.index}
				}
			}
		case "response.output_item.done":
			switch ev.Item.Type {
			case "function_call":
				id := ev.Item.CallID
				if id == "" {
					id = ev.Item.ID
				}
				if ta := toolByID[id]; ta != nil {
					if ta.name == "" {
						ta.name = ev.Item.Name
					}
					if ta.args.Len() == 0 && ev.Item.Arguments != "" {
						ta.args.WriteString(ev.Item.Arguments)
						ch <- t.StreamEvent{Type: t.EventInputDelta, InputDelta: ev.Item.Arguments, ContentIndex: ta.index}
					}
					// Finalize even if function_call_arguments.done never fired
					// (e.g. a zero-argument call), so eager dispatch still triggers.
					if !ta.stopped {
						ta.stopped = true
						ch <- t.StreamEvent{Type: t.EventContentBlockStop, ContentIndex: ta.index}
					}
				}
			}
		case "response.completed", "response.incomplete":
			sawTerminal = true
			if ev.Type == "response.incomplete" || ev.Response.IncompleteDetails.Reason == "max_output_tokens" {
				stopReason = "max_tokens"
			}
			if ev.Response.Usage.TotalTokens > 0 || ev.Response.Usage.InputTokens > 0 {
				usage = &t.Usage{
					InputTokens:  ev.Response.Usage.InputTokens,
					OutputTokens: ev.Response.Usage.OutputTokens,
					TotalTokens:  ev.Response.Usage.TotalTokens,
				}
			}
		case "response.failed":
			// Surface the server-provided failure reason and only mark it
			// transient when actually retryable. A permanent failure (content
			// policy / invalid request) must NOT be a TransientError, or the
			// whole turn is retried futilely.
			reason := ev.Response.Error.Message
			if reason == "" {
				reason = ev.Response.IncompleteDetails.Reason
			}
			msg := "openai chatgpt response failed"
			if reason != "" {
				msg = "openai chatgpt response failed: " + reason
			}
			err := errors.New(msg)
			if isTransientOpenAIError(ev.Response.Error.Code, reason) {
				ch <- t.StreamEvent{Type: t.EventError, Error: &t.TransientError{Err: err}}
			} else {
				ch <- t.StreamEvent{Type: t.EventError, Error: err}
			}
			return true
		case "error":
			// The error frame's code+message live at the top level, not under
			// "error"; surface them and only retry when the reason is transient.
			emsg := ev.Message
			if emsg == "" && len(ev.Error) > 0 {
				emsg = string(ev.Error)
			}
			var errText string
			switch {
			case ev.Code != "" && emsg != "":
				errText = fmt.Sprintf("openai chatgpt stream error [%s]: %s", ev.Code, emsg)
			case emsg != "":
				errText = "openai chatgpt stream error: " + emsg
			case ev.Code != "":
				errText = "openai chatgpt stream error [" + ev.Code + "]"
			default:
				errText = "openai chatgpt stream error"
			}
			err := errors.New(errText)
			if isTransientOpenAIError(ev.Code, emsg) {
				ch <- t.StreamEvent{Type: t.EventError, Error: &t.TransientError{Err: err}}
			} else {
				ch <- t.StreamEvent{Type: t.EventError, Error: err}
			}
			return true
		}
		return false
	}

	// SSE framing: a single event's data may span multiple `data:` lines joined
	// by "\n", and is dispatched on the blank-line boundary. Accumulate here and
	// flush per event so multi-line frames parse as one JSON object.
	var dataBuf strings.Builder
	flush := func() (stop bool) {
		if dataBuf.Len() == 0 {
			return false
		}
		data := dataBuf.String()
		dataBuf.Reset()
		return dispatch(data)
	}

	terminated := false
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" { // event boundary
			if flush() {
				terminated = true
				break
			}
			continue
		}
		// Comment (":") and "event:" lines carry no data. The event kind is read
		// from the JSON `type` field, so the SSE `event:` field is ignored.
		if strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "event:") {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			seg := strings.TrimPrefix(strings.TrimPrefix(trimmed, "data:"), " ")
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(seg)
			continue
		}
		// Unknown field line — ignore.
	}
	// A stream may end (EOF) without a trailing blank line; flush any pending frame.
	if !terminated && flush() {
		terminated = true
	}
	if terminated {
		return
	}
	if err := scanner.Err(); err != nil {
		streamErr := fmt.Errorf("stream read: %w", err)
		if isTransientReadErr(err) {
			ch <- t.StreamEvent{Type: t.EventError, Error: &t.TransientError{Err: streamErr}}
		} else {
			ch <- t.StreamEvent{Type: t.EventError, Error: streamErr}
		}
		return
	}

	// Assemble the accumulated assistant message. The reasoning summary is kept
	// as a human-readable thinking block for display; the encrypted-reasoning
	// round-trip (store:false) is deferred — the agent loop strips thinking
	// blocks (FilterThinking) before history is replayed, so echoing encrypted
	// reasoning back is not yet wired end-to-end.
	var blocks []t.ContentBlock
	if reasoningBuf.Len() > 0 {
		blocks = append(blocks, t.ContentBlock{Type: "thinking", Text: reasoningBuf.String()})
	}
	if textBuf.Len() > 0 {
		blocks = append(blocks, t.ContentBlock{Type: "text", Text: textBuf.String()})
	}
	hasTool := false
	for _, id := range toolOrder {
		ta := toolByID[id]
		if ta == nil {
			continue
		}
		var input map[string]any
		if s := ta.args.String(); s != "" {
			if err := json.Unmarshal([]byte(s), &input); err != nil {
				log.Printf("openai-chatgpt: bad tool args for %q: %v", ta.name, err)
			}
		}
		blocks = append(blocks, t.ContentBlock{Type: "tool_use", ID: ta.id, Name: ta.name, Input: input})
		hasTool = true
	}
	if hasTool {
		stopReason = "tool_use"
	}

	if !sawTerminal && len(blocks) == 0 {
		ch <- t.StreamEvent{Type: t.EventError, Error: &t.TransientError{Err: fmt.Errorf("openai chatgpt: stream closed before completion")}}
		return
	}

	if usage != nil {
		ch <- t.StreamEvent{Type: t.EventUsage, Usage: usage}
	}
	resp := &t.AssistantMessage{
		Message:    t.Message{Role: t.RoleAssistant, Content: blocks},
		Model:      p.Model,
		StopReason: stopReason,
	}
	if usage != nil {
		resp.Usage = *usage
	}
	ch <- t.StreamEvent{Type: t.EventMessageStop, StopReason: stopReason, Response: resp}
}

// newUUIDv4 generates a random RFC-4122 v4 UUID for the session-id header.
func newUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
