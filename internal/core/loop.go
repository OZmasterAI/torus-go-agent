package core

import (
	"context"
	"fmt"
	"log"

)

// Agent is the DAG-based ReAct agent loop.
type Agent struct {
	config     AgentConfig
	provider   Provider
	hooks      *HookRegistry
	dag        *DAG
	compaction CompactionConfig
	Summarize  func(string) (string, error) // LLM summarize callback for compaction
}

// NewAgent creates a new agent.
func NewAgent(config AgentConfig, provider Provider, hooks *HookRegistry, dag *DAG) *Agent {
	if config.MaxTurns == 0 {
		config.MaxTurns = 30
	}
	return &Agent{
		config:   config,
		provider: provider,
		hooks:    hooks,
		dag:      dag,
		compaction: CompactionConfig{
			Mode:          CompactionLLM,
			Threshold:     80,
			KeepLastN:     10,
			ContextWindow: config.Provider.MaxTokens,
		},
	}
}

// SetCompaction configures the compaction settings.
func (a *Agent) SetCompaction(cfg CompactionConfig) { a.compaction = cfg }

// Run processes a user message through the ReAct loop and returns the final text.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
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

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		// Fire turn start
		a.hooks.Fire(ctx, HookOnTurnStart, &HookData{AgentID: "main", Meta: map[string]any{"turn": turn}})

		// Build context from DAG
		currentHead, _ := a.dag.GetHead()
		messages, err := a.dag.PromptFrom(currentHead)
		if err != nil {
			return "", fmt.Errorf("build context: %w", err)
		}

		// Compaction: check if context exceeds threshold
		if a.compaction.Mode != CompactionOff && NeedsCompaction(messages, a.compaction) {
			log.Printf("[loop] compaction triggered (%d messages)", len(messages))
			switch a.compaction.Mode {
			case CompactionSliding:
				messages = CompactSliding(messages, a.compaction.KeepLastN)
			case CompactionLLM:
				compacted, err := CompactLLM(messages, a.compaction.KeepLastN, a.Summarize)
				if err != nil {
					log.Printf("[loop] compaction error: %v, using sliding fallback", err)
					messages = CompactSliding(messages, a.compaction.KeepLastN)
				} else {
					messages = compacted
				}
			}
		}

		// Fire before context build
		ctxData := &HookData{AgentID: "main", Messages: messages}
		a.hooks.Fire(ctx, HookBeforeContextBuild, ctxData)
		messages = ctxData.Messages

		// Fire token count hook
		tokenEst := EstimateTokens(messages)
		a.hooks.Fire(ctx, HookOnTokenCount, &HookData{AgentID: "main", TokensIn: tokenEst, Meta: map[string]any{"estimated": true}})

		// Fire before LLM call
		llmData := &HookData{AgentID: "main", Messages: messages, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeLLMCall, llmData)
		if llmData.Block {
			log.Printf("[loop] LLM call blocked: %s", llmData.BlockReason)
			break
		}

		// Convert tools for provider
		var toolDefs []Tool
		for _, t := range a.config.Tools {
			toolDefs = append(toolDefs, t)
		}

		// Call LLM
		resp, err := a.provider.Complete(ctx, a.config.SystemPrompt, messages, toolDefs, a.config.Provider.MaxTokens)
		if err != nil {
			a.hooks.Fire(ctx, HookOnError, &HookData{AgentID: "main", Meta: map[string]any{"error": err.Error()}})
			return "", fmt.Errorf("llm call: %w", err)
		}

		// Fire after LLM call
		a.hooks.Fire(ctx, HookAfterLLMCall, &HookData{
			AgentID:   "main",
			Response:  resp,
			TokensIn:  resp.Usage.InputTokens,
			TokensOut: resp.Usage.OutputTokens,
		})

		// Add assistant response to DAG
		_, err = a.dag.AddNode(currentHead, RoleAssistant, resp.Content, resp.Model, a.provider.Name(), resp.Usage.TotalTokens)
		if err != nil {
			return "", fmt.Errorf("add assistant node: %w", err)
		}

		// Check for tool use
		if !HasToolUse(resp) {
			finalText = ExtractText(resp)
			a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
			break
		}

		// Execute tool calls
		toolCalls := ExtractToolCalls(resp)
		for _, tc := range toolCalls {
			// Fire before tool call
			toolData := &HookData{
				AgentID:  "main",
				ToolName: tc.Name,
				ToolArgs: tc.Input,
				Meta:     map[string]any{},
			}
			a.hooks.Fire(ctx, HookBeforeToolCall, toolData)

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

			// Fire after tool call
			a.hooks.Fire(ctx, HookAfterToolCall, &HookData{
				AgentID:    "main",
				ToolName:   tc.Name,
				ToolArgs:   tc.Input,
				ToolResult: result,
			})

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

		a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
		_ = userNodeID // used above
	}

	// Fire agent end
	a.hooks.Fire(ctx, HookOnAgentEnd, &HookData{AgentID: "main", Meta: map[string]any{"text": finalText}})

	return finalText, nil
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
