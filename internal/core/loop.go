package core

import (
	"context"
	"fmt"
	"log"

	t "torus_go_agent/internal/types"
)

// Agent is the DAG-based ReAct agent loop.
type Agent struct {
	config        t.AgentConfig
	provider      t.Provider
	hooks         *HookRegistry
	dag           *DAG
	compaction    CompactionConfig
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

		if a.compaction.Mode != CompactionOff && NeedsCompaction(messages, a.compaction) {
			preCount := len(messages)
			log.Printf("[loop] compaction triggered (%d messages)", preCount)
			a.hooks.Fire(ctx, HookPreCompact, &HookData{
				AgentID: "main", Messages: messages,
				Meta: map[string]any{"mode": string(a.compaction.Mode), "message_count": preCount},
			})
			emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "pre_compact"})
			if err := CompactDAG(a.dag, a.compaction, a.Summarize); err != nil {
				log.Printf("[loop] DAG compaction failed: %v, falling back to in-memory", err)
				switch a.compaction.Mode {
				case CompactionSliding:
					messages = CompactSliding(messages, a.compaction.KeepLastN)
				case CompactionLLM:
					compacted, err := CompactLLM(messages, a.compaction.KeepLastN, a.Summarize)
					if err != nil {
						log.Printf("[loop] LLM compaction error: %v, using sliding fallback", err)
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

		ctxData := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookBeforeContextBuild, ctxData)
		emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "before_context_build"})
		messages = ctxData.Messages
		afterCtx := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookAfterContextBuild, afterCtx)
		messages = afterCtx.Messages

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

		var resp *t.AssistantMessage
		var llmErr error
		streamCh, streamErr := activeProvider.StreamComplete(ctx, a.config.SystemPrompt, messages, toolDefs, a.config.Provider.MaxTokens)
		if streamErr != nil {
			llmErr = streamErr
		} else {
			resp, llmErr = consumeStreamEmit(streamCh, emit)
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
		for _, tc := range toolCalls {
			// Aggressive steering: check before each tool, skip remaining if user steered
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
