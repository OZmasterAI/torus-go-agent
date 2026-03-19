package core

import (
	"context"
	"fmt"
	"log"
)

// Agent is the DAG-based ReAct agent loop.
type Agent struct {
	config        AgentConfig
	provider      Provider
	smartProvider Provider          // optional cheaper model for simple messages
	hooks         *HookRegistry
	dag           *DAG
	compaction    CompactionConfig
	Summarize     func(string) (string, error) // LLM summarize callback for compaction
	OnStreamDelta func(delta string)                                    // called for each text delta during streaming; nil = use Complete
	OnToolUse     func(name string, args map[string]any, result *ToolResult) // called after each tool execution; nil = silent
	Steering      chan Message // async steering: any goroutine can push messages, polled between tool rounds and at exit
}

// SetSmartProvider sets the provider used for simple messages when smart routing is enabled.
func (a *Agent) SetSmartProvider(p Provider) { a.smartProvider = p }

// NewAgent creates a new agent.
func NewAgent(config AgentConfig, provider Provider, hooks *HookRegistry, dag *DAG) *Agent {
	// MaxTurns: 0 = unlimited, >0 = cap. Default 30 set by caller (cmd/main.go).
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

// SetCompaction configures the compaction settings.
func (a *Agent) SetCompaction(cfg CompactionConfig) { a.compaction = cfg }

// GetCompaction returns the current compaction settings.
func (a *Agent) GetCompaction() CompactionConfig { return a.compaction }

// Run processes a user message through the ReAct loop and returns the final text.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	// on_user_input: can transform or block user input before anything else
	inputData := &HookData{
		AgentID: "main",
		Meta:    map[string]any{"input": userMessage},
	}
	a.hooks.Fire(ctx, HookOnUserInput, inputData)
	if inputData.Block {
		return inputData.BlockReason, nil
	}
	if modified, ok := inputData.Meta["input"].(string); ok {
		userMessage = modified
	}

	// Fire agent start
	a.hooks.Fire(ctx, HookOnAgentStart, &HookData{AgentID: "main"})

	// Add user message to DAG
	head, _ := a.dag.GetHead()
	userContent := []ContentBlock{{Type: "text", Text: userMessage}}
	userNodeID, err := a.dag.AddNode(head, RoleUser, userContent, "", "", 0)
	if err != nil {
		return "", fmt.Errorf("add user node: %w", err)
	}

	var finalText string

	for turn := 0; a.config.MaxTurns == 0 || turn < a.config.MaxTurns; turn++ {
		// Fire turn start
		a.hooks.Fire(ctx, HookOnTurnStart, &HookData{AgentID: "main", Meta: map[string]any{"turn": turn}})

		// Build context from DAG
		currentHead, _ := a.dag.GetHead()
		messages, err := a.dag.PromptFrom(currentHead)
		if err != nil {
			return "", fmt.Errorf("build context: %w", err)
		}

		// Sanitize messages: remove empty blocks, dedup identical blocks, merge consecutive same-role
		messages = sanitizeMessages(messages)

		// Compaction: check if context exceeds threshold
		if a.compaction.Mode != CompactionOff && NeedsCompaction(messages, a.compaction) {
			preCount := len(messages)
			log.Printf("[loop] compaction triggered (%d messages)", preCount)
			a.hooks.Fire(ctx, HookPreCompact, &HookData{
				AgentID:  "main",
				Messages: messages,
				Meta:     map[string]any{"mode": string(a.compaction.Mode), "message_count": preCount},
			})

			// Use DAG-persistent compaction: creates a new branch with summary + last N.
			// Falls back to in-memory compaction if CompactDAG fails.
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
				// DAG compaction succeeded — reload from the new branch
				currentHead, _ = a.dag.GetHead()
				messages, err = a.dag.PromptFrom(currentHead)
				if err != nil {
					return "", fmt.Errorf("reload after compaction: %w", err)
				}
				messages = sanitizeMessages(messages)
			}

			a.hooks.Fire(ctx, HookPostCompact, &HookData{
				AgentID: "main",
				Meta: map[string]any{
					"mode":            string(a.compaction.Mode),
					"messages_before": preCount,
					"messages_after":  len(messages),
					"persistent":      true,
				},
			})
		}

		// Fire before context build
		ctxData := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookBeforeContextBuild, ctxData)
		messages = ctxData.Messages

		// Fire after context build (final prompt assembled — can transform)
		afterCtx := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookAfterContextBuild, afterCtx)
		messages = afterCtx.Messages

		// Fire token count hook
		tokenEst := EstimateTokens(messages)
		a.hooks.Fire(ctx, HookOnTokenCount, &HookData{AgentID: "main", TokensIn: tokenEst, Meta: map[string]any{"estimated": true}})

		// Fire before LLM call (can transform messages or block)
		llmData := &HookData{AgentID: "main", Messages: messages, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeLLMCall, llmData)
		if llmData.Block {
			log.Printf("[loop] LLM call blocked: %s", llmData.BlockReason)
			break
		}
		messages = llmData.Messages

		// Convert tools for provider
		var toolDefs []Tool
		for _, t := range a.config.Tools {
			toolDefs = append(toolDefs, t)
		}

		// Smart routing: pick provider based on message complexity
		activeProvider := a.provider
		if a.smartProvider != nil && a.config.SmartRouting {
			routed := RouteMessage(userMessage, a.config)
			if routed != a.config.Provider.Model {
				activeProvider = a.smartProvider
				log.Printf("[loop] smart routing: using %s for simple message", routed)
			}
		}

		// Call LLM — use streaming if callback is set, otherwise non-streaming
		var resp *AssistantMessage
		var llmErr error
		if a.OnStreamDelta != nil {
			var ch <-chan StreamEvent
			ch, llmErr = activeProvider.StreamComplete(ctx, a.config.SystemPrompt, messages, toolDefs, a.config.Provider.MaxTokens)
			if llmErr == nil {
				resp, llmErr = consumeStream(ch, a.OnStreamDelta)
			}
		} else {
			resp, llmErr = activeProvider.Complete(ctx, a.config.SystemPrompt, messages, toolDefs, a.config.Provider.MaxTokens)
		}
		if llmErr != nil {
			a.hooks.Fire(ctx, HookOnError, &HookData{AgentID: "main", Meta: map[string]any{"error": llmErr.Error()}})
			// Roll back the user node to prevent dangling orphans in the DAG
			if turn == 0 {
				a.dag.RemoveNode(userNodeID)
			}
			return "", fmt.Errorf("llm call: %w", llmErr)
		}

		// Fire after LLM call (can transform response)
		afterLLM := &HookData{
			AgentID:   "main",
			Response:  resp,
			TokensIn:  resp.Usage.InputTokens,
			TokensOut: resp.Usage.OutputTokens,
		}
		a.hooks.Fire(ctx, HookAfterLLMCall, afterLLM)
		if afterLLM.Response != nil {
			resp = afterLLM.Response
		}

		// Add assistant response to DAG
		_, err = a.dag.AddNode(currentHead, RoleAssistant, resp.Content, resp.Model, a.provider.Name(), resp.Usage.TotalTokens)
		if err != nil {
			return "", fmt.Errorf("add assistant node: %w", err)
		}

		// Check for tool use
		if !HasToolUse(resp) {
			finalText = ExtractText(resp)

			// before_loop_exit: hooks can inject follow-up messages + set Block to force another turn
			exitData := &HookData{
				AgentID:  "main",
				Response: resp,
				Messages: nil, // hooks populate this with follow-up messages
				Meta:     map[string]any{"final_text": finalText},
			}
			a.hooks.Fire(ctx, HookBeforeLoopExit, exitData)

			if exitData.Block && len(exitData.Messages) > 0 {
				// Hook wants another turn — add follow-up messages to DAG
				for _, msg := range exitData.Messages {
					fHead, _ := a.dag.GetHead()
					a.dag.AddNode(fHead, msg.Role, msg.Content, "", "", 0)
				}
				finalText = "" // reset — we're continuing
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue // back to top of loop
			}

			// Drain async steering channel — if messages arrived, force another turn
			if a.drainSteering() > 0 {
				finalText = ""
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue
			}

			a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
			break
		}

		// Execute tool calls
		toolCalls := ExtractToolCalls(resp)
		for _, tc := range toolCalls {
			// Fire before tool call (can transform args, rename tool, or block)
			toolData := &HookData{
				AgentID:  "main",
				ToolName: tc.Name,
				ToolArgs: tc.Input,
				Meta:     map[string]any{},
			}
			a.hooks.Fire(ctx, HookBeforeToolCall, toolData)
			tc.Name = toolData.ToolName   // read back possibly-modified name
			tc.Input = toolData.ToolArgs  // read back possibly-modified args

			var result *ToolResult
			if toolData.Block {
				result = &ToolResult{
					ToolUseID: tc.ID,
					Content:   fmt.Sprintf("[BLOCKED] %s", toolData.BlockReason),
					IsError:   true,
				}
			} else {
				// Find and execute tool
				tool := a.findTool(tc.Name)
				if tool == nil {
					result = &ToolResult{
						ToolUseID: tc.ID,
						Content:   fmt.Sprintf("Tool '%s' not found", tc.Name),
						IsError:   true,
					}
				} else {
					r, err := tool.Execute(tc.Input)
					if err != nil {
						result = &ToolResult{
							ToolUseID: tc.ID,
							Content:   fmt.Sprintf("Tool error: %s", err.Error()),
							IsError:   true,
						}
					} else {
						result = r
						result.ToolUseID = tc.ID
					}
				}
			}

			// Fire after tool call (can transform result)
			afterTool := &HookData{
				AgentID:    "main",
				ToolName:   tc.Name,
				ToolArgs:   tc.Input,
				ToolResult: result,
			}
			a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
			if afterTool.ToolResult != nil {
				result = afterTool.ToolResult
			}

			// Notify TUI/channel about tool use
			if a.OnToolUse != nil {
				a.OnToolUse(tc.Name, tc.Input, result)
			}

			// Add tool result to DAG
			toolHead, _ := a.dag.GetHead()
			toolContent := []ContentBlock{{
				Type:      "tool_result",
				ToolUseID: result.ToolUseID,
				Content:   result.Content,
				IsError:   result.IsError,
			}}
			a.dag.AddNode(toolHead, RoleTool, toolContent, "", "", 0)
		}

		// after_tool_result: fires after ALL tool results are in DAG, before next LLM turn.
		// Hooks can inject steering messages to guide the LLM's next reasoning step.
		steerData := &HookData{
			AgentID:  "main",
			Response: resp,
			Messages: nil, // hooks populate this with steering messages
		}
		a.hooks.Fire(ctx, HookAfterToolResult, steerData)
		if len(steerData.Messages) > 0 {
			for _, msg := range steerData.Messages {
				sHead, _ := a.dag.GetHead()
				a.dag.AddNode(sHead, msg.Role, msg.Content, "", "", 0)
			}
		}

		// Drain async steering channel (non-blocking)
		a.drainSteering()

		a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
		_ = userNodeID // used above
	}

	// Fire stop failure if loop exhausted without a final response
	if finalText == "" {
		a.hooks.Fire(ctx, HookOnStopFailure, &HookData{
			AgentID: "main",
			Meta:    map[string]any{"reason": "max_turns_exhausted", "max_turns": a.config.MaxTurns},
		})
	}

	// Fire agent end
	a.hooks.Fire(ctx, HookOnAgentEnd, &HookData{AgentID: "main", Meta: map[string]any{"text": finalText}})

	return finalText, nil
}

// consumeStream reads all events from a StreamComplete channel, calls onDelta
// for text deltas, and returns the assembled AssistantMessage from message_stop.
func consumeStream(ch <-chan StreamEvent, onDelta func(string)) (*AssistantMessage, error) {
	var resp *AssistantMessage
	for ev := range ch {
		switch ev.Type {
		case EventTextDelta:
			if onDelta != nil {
				onDelta(ev.Text)
			}
		case EventError:
			return nil, ev.Error
		case EventMessageStop:
			resp = ev.Response
		}
	}
	if resp == nil {
		return nil, fmt.Errorf("stream ended without response")
	}
	return resp, nil
}

func (a *Agent) findTool(name string) *Tool {
	for i := range a.config.Tools {
		if a.config.Tools[i].Name == name {
			return &a.config.Tools[i]
		}
	}
	return nil
}

// DAG returns the conversation DAG.
func (a *Agent) DAG() *DAG { return a.dag }

// Hooks returns the hook registry.
func (a *Agent) Hooks() *HookRegistry { return a.hooks }

// AddTool appends a tool to the agent's config after creation.
func (a *Agent) AddTool(t Tool) { a.config.Tools = append(a.config.Tools, t) }

// drainSteering non-blocking reads all messages from the Steering channel
// and adds them to the DAG. Returns the number of messages drained.
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
