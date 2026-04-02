package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	t "torus_go_agent/internal/types"
)

// Agent is the DAG-based ReAct agent loop.
type Agent struct {
	config          t.AgentConfig
	provider        t.Provider
	hooks           *HookRegistry
	dag             *DAG
	compaction      CompactionConfig
	lastInputTokens int // actual input tokens from most recent API call
	steeringMode  string // "mild" (default) or "aggressive"
	Summarize     func(string) (string, error)
	OnStreamDelta func(delta string)
	OnToolUse     func(name string, args map[string]any, result *t.ToolResult)
	OnStatusUpdate func(hookName string)
	Steering      chan t.Message
	RouteProvider func(userMessage string) t.Provider
}

// NewAgent creates a new agent.
func NewAgent(config t.AgentConfig, provider t.Provider, hooks *HookRegistry, dag *DAG) *Agent {
	return &Agent{
		config:   config,
		provider: provider,
		hooks:    hooks,
		dag:      dag,
		compaction: CompactionConfig{
			Mode:          CompactionLLM,
			Threshold:     80,
			KeepLastN:     10,
			ContextWindow: config.ContextWindow,
		},
	}
}

func (a *Agent) SetCompaction(cfg CompactionConfig) { a.compaction = cfg }
func (a *Agent) GetCompaction() CompactionConfig     { return a.compaction }

// Run processes a user message and returns the final text.
// It wraps RunStream, draining events and calling OnStreamDelta/OnToolUse callbacks.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	var finalText string
	var finalErr error
	for ev := range a.RunStream(ctx, userMessage) {
		switch ev.Type {
		case EventAgentTextDelta:
			if a.OnStreamDelta != nil {
				a.OnStreamDelta(ev.Text)
			}
		case EventAgentToolEnd:
			if a.OnToolUse != nil {
				a.OnToolUse(ev.ToolName, ev.ToolArgs, ev.ToolResult)
			}
		case EventAgentDone:
			finalText = ev.Text
		case EventStatusUpdate:
			if a.OnStatusUpdate != nil {
				a.OnStatusUpdate(ev.StatusHook)
			}
		case EventAgentError:
			finalErr = ev.Error
		}
	}
	return finalText, finalErr
}

// RunStream processes a user message, emitting events on the returned channel.
// The channel is closed when the loop finishes.
func (a *Agent) RunStream(ctx context.Context, userMessage string) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)
	go a.runLoop(ctx, userMessage, ch)
	return ch
}

func (a *Agent) runLoop(ctx context.Context, userMessage string, ch chan<- AgentEvent) {
	defer close(ch)

	emit := func(ev AgentEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}

	inputData := &HookData{AgentID: "main", Meta: map[string]any{"input": userMessage}}
	a.hooks.Fire(ctx, HookOnUserInput, inputData)
	emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "on_user_input"})
	if inputData.Block {
		emit(AgentEvent{Type: EventAgentDone, Text: inputData.BlockReason})
		return
	}
	if modified, ok := inputData.Meta["input"].(string); ok {
		userMessage = modified
	}

	a.hooks.Fire(ctx, HookOnAgentStart, &HookData{AgentID: "main"})

	head, _ := a.dag.GetHead()
	userContent := []t.ContentBlock{{Type: "text", Text: userMessage}}
	userNodeID, err := a.dag.AddNode(head, t.RoleUser, userContent, "", "", 0)
	if err != nil {
		emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("add user node: %w", err)})
		return
	}

	var finalText string

	for turn := 0; a.config.MaxTurns == 0 || turn < a.config.MaxTurns; turn++ {
		emit(AgentEvent{Type: EventAgentTurnStart, Turn: turn})
		a.hooks.Fire(ctx, HookOnTurnStart, &HookData{AgentID: "main", Meta: map[string]any{"turn": turn}})

		currentHead, _ := a.dag.GetHead()
		messages, err := a.dag.PromptFrom(currentHead)
		if err != nil {
			emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("build context: %w", err)})
			return
		}
		messages = sanitizeMessages(messages)
		messages = MicroCompact(messages, a.compaction.KeepLastN)

		// Compression hooks fire first (squeeze before compaction)
		ctxData := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookBeforeContextBuild, ctxData)
		emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "before_context_build"})
		messages = ctxData.Messages
		afterCtx := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookAfterContextBuild, afterCtx)
		messages = afterCtx.Messages

		// Compaction: emergency fallback if still over threshold after compression
		if a.compaction.Mode != CompactionOff && NeedsCompaction(messages, a.compaction, a.lastInputTokens) {
			preCount := len(messages)
			a.hooks.Fire(ctx, HookPreCompact, &HookData{
				AgentID: "main", Messages: messages,
				Meta: map[string]any{"mode": string(a.compaction.Mode), "message_count": preCount},
			})
			emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "pre_compact"})
			if err := CompactDAG(a.dag, a.compaction, a.Summarize); err != nil {
				switch a.compaction.Mode {
				case CompactionSliding:
					messages = CompactSliding(messages, a.compaction.KeepLastN)
				case CompactionLLM:
					compacted, err := CompactLLM(messages, a.compaction.KeepLastN, a.Summarize)
					if err != nil {
						messages = CompactSliding(messages, a.compaction.KeepLastN)
					} else {
						messages = compacted
					}
				}
			} else {
				currentHead, _ = a.dag.GetHead()
				messages, err = a.dag.PromptFrom(currentHead)
				if err != nil {
					emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("reload after compaction: %w", err)})
					return
				}
				messages = sanitizeMessages(messages)
			}
			a.hooks.Fire(ctx, HookPostCompact, &HookData{
				AgentID: "main",
				Meta: map[string]any{"mode": string(a.compaction.Mode), "messages_before": preCount, "messages_after": len(messages), "persistent": true},
			})
			emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "post_compact"})
		}

		tokenEst := EstimateTokens(messages)
		a.hooks.Fire(ctx, HookOnTokenCount, &HookData{AgentID: "main", TokensIn: tokenEst, Meta: map[string]any{"estimated": true}})

		llmData := &HookData{AgentID: "main", Messages: messages, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeLLMCall, llmData)
		emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "before_llm_call"})
		if llmData.Block {
			log.Printf("[loop] LLM call blocked: %s", llmData.BlockReason)
			break
		}
		messages = llmData.Messages

		toolDefs := append([]t.Tool(nil), a.config.Tools...)

		activeProvider := a.provider
		if a.RouteProvider != nil {
			activeProvider = a.RouteProvider(userMessage)
		}

		// Compute dynamic max_tokens: leave room for input instead of blindly
		// sending the configured max. This prevents 400 errors on models where
		// max_completion_tokens == context_window (common with free OpenRouter models).
		maxTokens := a.config.Provider.MaxTokens
		if a.config.ContextWindow > 0 {
			inputCost := EstimatePromptCost(a.config.SystemPrompt, messages, toolDefs)
			available := a.config.ContextWindow - inputCost
			if available < 1024 {
				available = 1024 // absolute floor so we always get some output
			}
			if maxTokens <= 0 || maxTokens > available {
				maxTokens = available
			}
		}
		if maxTokens <= 0 {
			maxTokens = 8192
		}

		var resp *t.AssistantMessage
		var llmErr error
		var eagerResults map[string]*eagerResult
		for attempt := 0; attempt <= 3; attempt++ {
			if attempt > 0 {
				delay := time.Duration(1<<uint(attempt)) * time.Second
				if delay > 8*time.Second {
					delay = 8 * time.Second
				}
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					llmErr = ctx.Err()
				}
				if llmErr == ctx.Err() {
					break
				}
				log.Printf("[loop] retrying LLM call (attempt %d/4) after transient error", attempt+1)
			}
			streamCh, streamErr := activeProvider.StreamComplete(ctx, a.config.SystemPrompt, messages, toolDefs, maxTokens)
			if streamErr != nil {
				llmErr = streamErr
				var te *t.TransientError
				if errors.As(streamErr, &te) {
					continue
				}
				break
			}
			if a.config.ParallelTools {
				resp, eagerResults, llmErr = a.consumeStreamEager(ctx, streamCh, emit)
			} else {
				resp, llmErr = consumeStreamEmit(streamCh, emit)
			}
			if llmErr != nil {
				var te *t.TransientError
				if errors.As(llmErr, &te) {
					continue
				}
				break
			}
			break
		}
		if llmErr != nil {
			a.hooks.Fire(ctx, HookOnError, &HookData{AgentID: "main", Meta: map[string]any{"error": llmErr.Error()}})
			emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "on_error"})
			if turn == 0 {
				a.dag.RemoveNode(userNodeID)
			}
			emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("llm call: %w", llmErr)})
			return
		}

		a.lastInputTokens = resp.Usage.InputTokens
		afterLLM := &HookData{AgentID: "main", Response: resp, TokensIn: resp.Usage.InputTokens, TokensOut: resp.Usage.OutputTokens}
		a.hooks.Fire(ctx, HookAfterLLMCall, afterLLM)
		emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "after_llm_call"})
		if afterLLM.Response != nil {
			resp = afterLLM.Response
		}

		cleanContent, thinkingBlocks := FilterThinking(resp.Content)
		nodeID, err := a.dag.AddNode(currentHead, t.RoleAssistant, cleanContent, resp.Model, a.provider.Name(), resp.Usage.TotalTokens)
		if err != nil {
			emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("add assistant node: %w", err)})
			return
		}
		a.dag.SetAlias(nodeID, a.dag.NextAutoAlias())
		if a.config.PersistThinking && len(thinkingBlocks) > 0 {
			a.dag.AddNode(nodeID, "thinking", thinkingBlocks, "", "", 0)
		}

		if !HasToolUse(resp) {
			finalText = ExtractText(resp)
			exitData := &HookData{AgentID: "main", Response: resp, Messages: nil, Meta: map[string]any{"final_text": finalText}}
			a.hooks.Fire(ctx, HookBeforeLoopExit, exitData)
			if exitData.Block && len(exitData.Messages) > 0 {
				for _, msg := range exitData.Messages {
					fHead, _ := a.dag.GetHead()
					a.dag.AddNode(fHead, msg.Role, msg.Content, "", "", 0)
				}
				finalText = ""
				emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue
			}
			if a.drainSteering() > 0 {
				finalText = ""
				emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue
			}
			emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
			a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
			break
		}

		toolCalls := ExtractToolCalls(resp)
		if len(eagerResults) > 0 {
			a.finishEagerTools(ctx, toolCalls, eagerResults, emit)
		} else if a.config.ParallelTools && len(toolCalls) > 1 {
			a.executeToolsParallel(ctx, toolCalls, emit)
		} else {
			a.executeToolsSequential(ctx, toolCalls, emit)
		}

		steerData := &HookData{AgentID: "main", Response: resp, Messages: nil}
		a.hooks.Fire(ctx, HookAfterToolResult, steerData)
		if len(steerData.Messages) > 0 {
			for _, msg := range steerData.Messages {
				sHead, _ := a.dag.GetHead()
				a.dag.AddNode(sHead, msg.Role, msg.Content, "", "", 0)
			}
		}
		a.drainSteering()

		emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
		a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
	}

	if finalText == "" {
		a.hooks.Fire(ctx, HookOnStopFailure, &HookData{AgentID: "main", Meta: map[string]any{"reason": "max_turns_exhausted", "max_turns": a.config.MaxTurns}})
	}
	a.hooks.Fire(ctx, HookOnAgentEnd, &HookData{AgentID: "main", Meta: map[string]any{"text": finalText}})
	emit(AgentEvent{Type: EventAgentDone, Text: finalText})
}

func consumeStreamEmit(streamCh <-chan t.StreamEvent, emit func(AgentEvent)) (*t.AssistantMessage, error) {
	var resp *t.AssistantMessage
	for ev := range streamCh {
		switch ev.Type {
		case t.EventTextDelta:
			emit(AgentEvent{Type: EventAgentTextDelta, Text: ev.Text})
		case t.EventThinkingDelta:
			emit(AgentEvent{Type: EventAgentThinkingDelta, Text: ev.Text})
		case t.EventError:
			return nil, ev.Error
		case t.EventMessageStop:
			resp = ev.Response
		}
	}
	if resp == nil {
		return nil, fmt.Errorf("stream ended without response")
	}
	return resp, nil
}

func (a *Agent) findTool(name string) *t.Tool {
	for i := range a.config.Tools {
		if a.config.Tools[i].Name == name {
			return &a.config.Tools[i]
		}
	}
	return nil
}

func (a *Agent) DAG() *DAG                { return a.dag }
func (a *Agent) Hooks() *HookRegistry     { return a.hooks }
func (a *Agent) Provider() t.Provider     { return a.provider }
func (a *Agent) SystemPrompt() string     { return a.config.SystemPrompt }
func (a *Agent) AddTool(tool t.Tool)        { a.config.Tools = append(a.config.Tools, tool) }
func (a *Agent) SetSteeringMode(mode string) { a.steeringMode = mode }
func (a *Agent) GetSteeringMode() string {
	if a.steeringMode == "" {
		return "mild"
	}
	return a.steeringMode
}

func (a *Agent) drainSteering() int {
	if a.Steering == nil {
		return 0
	}
	n := 0
	for {
		select {
		case msg := <-a.Steering:
			head, _ := a.dag.GetHead()
			a.dag.AddNode(head, msg.Role, msg.Content, "", "", 0)
			n++
		default:
			return n
		}
	}
}

// parallelSafeTools lists tools whose Execute is side-effect-isolated and safe to run concurrently.
var parallelSafeTools = map[string]bool{
	"read": true, "glob": true, "grep": true, "bash": true,
}

// isParallelSafe returns true if the named tool can be executed concurrently.
func isParallelSafe(name string) bool { return parallelSafeTools[name] }

// maxParallelTools caps the number of concurrent tool executions (prevents resource exhaustion from e.g. many bash calls).
const maxParallelTools = 4

// preparedCall holds a tool call after before-hooks have been applied.
type preparedCall struct {
	tc      t.ContentBlock
	tool    *t.Tool
	blocked bool
	reason  string
}

// executeToolsSequential runs tool calls one at a time (original behavior).
func (a *Agent) executeToolsSequential(ctx context.Context, toolCalls []t.ContentBlock, emit func(AgentEvent)) {
	for _, tc := range toolCalls {
		if a.steeringMode == "aggressive" && a.drainSteering() > 0 {
			break
		}
		toolData := &HookData{AgentID: "main", ToolName: tc.Name, ToolArgs: tc.Input, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeToolCall, toolData)
		tc.Name = toolData.ToolName
		tc.Input = toolData.ToolArgs
		emit(AgentEvent{Type: EventAgentToolStart, ToolName: tc.Name, ToolArgs: tc.Input})

		var result *t.ToolResult
		if toolData.Block {
			result = &t.ToolResult{ToolUseID: tc.ID, Content: fmt.Sprintf("[BLOCKED] %s", toolData.BlockReason), IsError: true}
		} else {
			tool := a.findTool(tc.Name)
			if tool == nil {
				result = &t.ToolResult{ToolUseID: tc.ID, Content: fmt.Sprintf("Tool '%s' not found", tc.Name), IsError: true}
			} else {
				r, execErr := tool.Execute(tc.Input)
				if execErr != nil {
					result = &t.ToolResult{ToolUseID: tc.ID, Content: fmt.Sprintf("Tool error: %s", execErr.Error()), IsError: true}
				} else {
					result = r
					result.ToolUseID = tc.ID
				}
			}
		}

		afterTool := &HookData{AgentID: "main", ToolName: tc.Name, ToolArgs: tc.Input, ToolResult: result}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			result = afterTool.ToolResult
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: tc.Name, ToolArgs: tc.Input, ToolResult: result})

		toolHead, _ := a.dag.GetHead()
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: result.ToolUseID, Content: result.Content, IsError: result.IsError}}
		a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0)
	}
}

// executeToolsParallel runs tool calls concurrently where safe.
// Phase 1: before-hooks (sequential — hooks may mutate or block)
// Phase 2: execute (parallel for safe tools, sequential for unsafe)
// Phase 3: after-hooks + DAG writes (sequential — preserves ordering)
func (a *Agent) executeToolsParallel(ctx context.Context, toolCalls []t.ContentBlock, emit func(AgentEvent)) {
	// Check aggressive steering once before the batch
	if a.steeringMode == "aggressive" && a.drainSteering() > 0 {
		return
	}

	// Phase 1: before-hooks, sequential
	prepared := make([]preparedCall, len(toolCalls))
	for i, tc := range toolCalls {
		toolData := &HookData{AgentID: "main", ToolName: tc.Name, ToolArgs: tc.Input, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeToolCall, toolData)
		tc.Name = toolData.ToolName
		tc.Input = toolData.ToolArgs
		emit(AgentEvent{Type: EventAgentToolStart, ToolName: tc.Name, ToolArgs: tc.Input})

		p := preparedCall{tc: tc, blocked: toolData.Block, reason: toolData.BlockReason}
		if !p.blocked {
			p.tool = a.findTool(tc.Name)
		}
		prepared[i] = p
	}

	// Phase 2: execute
	results := make([]*t.ToolResult, len(prepared))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallelTools)

	for i, pc := range prepared {
		if pc.blocked {
			results[i] = &t.ToolResult{ToolUseID: pc.tc.ID, Content: fmt.Sprintf("[BLOCKED] %s", pc.reason), IsError: true}
			continue
		}
		if pc.tool == nil {
			results[i] = &t.ToolResult{ToolUseID: pc.tc.ID, Content: fmt.Sprintf("Tool '%s' not found", pc.tc.Name), IsError: true}
			continue
		}

		if isParallelSafe(pc.tc.Name) {
			wg.Add(1)
			go func(idx int, p preparedCall) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				r, err := p.tool.Execute(p.tc.Input)
				if err != nil {
					results[idx] = &t.ToolResult{ToolUseID: p.tc.ID, Content: fmt.Sprintf("Tool error: %s", err), IsError: true}
				} else {
					r.ToolUseID = p.tc.ID
					results[idx] = r
				}
			}(i, pc)
		} else {
			// Unsafe tools (write, edit) run inline to avoid file conflicts
			r, err := pc.tool.Execute(pc.tc.Input)
			if err != nil {
				results[i] = &t.ToolResult{ToolUseID: pc.tc.ID, Content: fmt.Sprintf("Tool error: %s", err), IsError: true}
			} else {
				r.ToolUseID = pc.tc.ID
				results[i] = r
			}
		}
	}
	wg.Wait()

	// Phase 3: after-hooks + DAG writes, sequential
	for i, pc := range prepared {
		afterTool := &HookData{AgentID: "main", ToolName: pc.tc.Name, ToolArgs: pc.tc.Input, ToolResult: results[i]}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			results[i] = afterTool.ToolResult
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: pc.tc.Name, ToolArgs: pc.tc.Input, ToolResult: results[i]})

		toolHead, _ := a.dag.GetHead()
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: results[i].ToolUseID, Content: results[i].Content, IsError: results[i].IsError}}
		a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0)
	}
}

// eagerResult holds a tool result dispatched during streaming.
type eagerResult struct {
	name   string
	args   map[string]any
	result *t.ToolResult
	done   chan struct{} // closed when result is ready
}

// consumeStreamEager processes the stream and starts tool execution as content blocks complete.
// Tool_use blocks are dispatched immediately on content_block_stop, overlapping with continued streaming.
// Returns the AssistantMessage, a map of eager results keyed by tool_use ID, and any error.
func (a *Agent) consumeStreamEager(ctx context.Context, streamCh <-chan t.StreamEvent, emit func(AgentEvent)) (*t.AssistantMessage, map[string]*eagerResult, error) {
	var resp *t.AssistantMessage
	type pendingTool struct {
		id   string
		name string
		buf  strings.Builder
	}
	pending := make(map[int]*pendingTool) // content index -> in-flight tool block
	results := make(map[string]*eagerResult)
	sem := make(chan struct{}, maxParallelTools)

	for ev := range streamCh {
		switch ev.Type {
		case t.EventTextDelta:
			emit(AgentEvent{Type: EventAgentTextDelta, Text: ev.Text})
		case t.EventThinkingDelta:
			emit(AgentEvent{Type: EventAgentThinkingDelta, Text: ev.Text})

		case t.EventToolUseStart:
			pending[ev.ContentIndex] = &pendingTool{id: ev.ID, name: ev.Name}

		case t.EventInputDelta:
			if pt, ok := pending[ev.ContentIndex]; ok {
				pt.buf.WriteString(ev.InputDelta)
			}

		case t.EventContentBlockStop:
			pt, ok := pending[ev.ContentIndex]
			if !ok {
				continue // text or thinking block — nothing to dispatch
			}
			delete(pending, ev.ContentIndex)

			var args map[string]any
			if s := pt.buf.String(); s != "" {
				_ = json.Unmarshal([]byte(s), &args)
			}

			// Before-hook (runs inline — hooks are fast, ≤5s timeout)
			toolData := &HookData{AgentID: "main", ToolName: pt.name, ToolArgs: args, Meta: map[string]any{}}
			a.hooks.Fire(ctx, HookBeforeToolCall, toolData)
			pt.name = toolData.ToolName
			args = toolData.ToolArgs

			emit(AgentEvent{Type: EventAgentToolStart, ToolName: pt.name, ToolArgs: args})

			er := &eagerResult{name: pt.name, args: args, done: make(chan struct{})}
			results[pt.id] = er

			if toolData.Block {
				er.result = &t.ToolResult{ToolUseID: pt.id, Content: fmt.Sprintf("[BLOCKED] %s", toolData.BlockReason), IsError: true}
				close(er.done)
				continue
			}

			tool := a.findTool(pt.name)
			if tool == nil {
				er.result = &t.ToolResult{ToolUseID: pt.id, Content: fmt.Sprintf("Tool '%s' not found", pt.name), IsError: true}
				close(er.done)
				continue
			}

			if isParallelSafe(pt.name) {
				go func(id string, tl *t.Tool, input map[string]any, er *eagerResult) {
					sem <- struct{}{}
					defer func() { <-sem }()
					r, err := tl.Execute(input)
					if err != nil {
						er.result = &t.ToolResult{ToolUseID: id, Content: fmt.Sprintf("Tool error: %s", err), IsError: true}
					} else {
						r.ToolUseID = id
						er.result = r
					}
					close(er.done)
				}(pt.id, tool, args, er)
			} else {
				// Unsafe tools (write, edit) run inline to avoid file conflicts
				r, err := tool.Execute(args)
				if err != nil {
					er.result = &t.ToolResult{ToolUseID: pt.id, Content: fmt.Sprintf("Tool error: %s", err), IsError: true}
				} else {
					r.ToolUseID = pt.id
					er.result = r
				}
				close(er.done)
			}

		case t.EventError:
			for _, er := range results {
				<-er.done
			}
			return nil, nil, ev.Error

		case t.EventMessageStop:
			resp = ev.Response
		}
	}

	// Wait for all in-flight tool executions
	for _, er := range results {
		<-er.done
	}

	if resp == nil {
		return nil, nil, fmt.Errorf("stream ended without response")
	}
	return resp, results, nil
}

// finishEagerTools runs after-hooks and writes DAG nodes for tools that were dispatched during streaming.
// Processes in the order from ExtractToolCalls to preserve DAG ordering.
func (a *Agent) finishEagerTools(ctx context.Context, toolCalls []t.ContentBlock, eagerResults map[string]*eagerResult, emit func(AgentEvent)) {
	for _, tc := range toolCalls {
		er, ok := eagerResults[tc.ID]
		if !ok {
			// Tool wasn't dispatched eagerly (shouldn't happen) — execute normally
			a.executeToolsSequential(ctx, []t.ContentBlock{tc}, emit)
			continue
		}

		afterTool := &HookData{AgentID: "main", ToolName: er.name, ToolArgs: er.args, ToolResult: er.result}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			er.result = afterTool.ToolResult
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: er.name, ToolArgs: er.args, ToolResult: er.result})

		toolHead, _ := a.dag.GetHead()
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: er.result.ToolUseID, Content: er.result.Content, IsError: er.result.IsError}}
		a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0)
	}
}
