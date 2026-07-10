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
	configMu        sync.RWMutex // guards mutable config fields (SystemPrompt, Tools)
	provider        t.Provider
	hooks           *HookRegistry
	dag             *DAG
	compaction      CompactionConfig
	lastInputTokens int // actual input tokens from most recent API call
	steeringMode  string // "mild" (default) or "aggressive"
	activeFiles   []string       // recently-touched file paths from tool calls
	activeFilesMu sync.RWMutex   // guards activeFiles

	// promptCache is an incremental cache of the built context, keyed by head node
	// ID, so a turn only fetches/parses nodes appended since the last build instead
	// of re-walking and re-unmarshaling every ancestor (guarded by promptCacheMu).
	promptCacheMu    sync.Mutex
	promptCacheHead  string
	promptCacheNodes []Node

	// toolToken caches the token estimate of the (static) tool schemas so they are
	// not re-marshaled every turn; keyed by tool count (guarded by toolTokenMu).
	toolTokenMu  sync.Mutex
	toolTokenEst int
	toolTokenLen int
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

// Notify fires the on_notification hook with the given message and metadata.
func (a *Agent) Notify(ctx context.Context, message string, meta map[string]any) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["message"] = message
	a.hooks.Fire(ctx, HookOnNotification, &HookData{AgentID: "main", Meta: meta})
}

// SetConfig fires the on_config_change hook for runtime config mutations.
func (a *Agent) SetConfig(ctx context.Context, key string, value any) {
	a.hooks.Fire(ctx, HookOnConfigChange, &HookData{
		AgentID: "main",
		Meta:    map[string]any{"key": key, "value": value},
	})
}

// Run processes a user message and returns the final text.
// It uses Complete (non-streaming) by default, unless ForceStream is set.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	streaming := a.config.ForceStream
	ch := make(chan AgentEvent, 32)
	go a.runLoop(ctx, userMessage, ch, streaming)
	var finalText string
	var finalErr error
	for ev := range ch {
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
	go a.runLoop(ctx, userMessage, ch, true)
	return ch
}

func (a *Agent) runLoop(ctx context.Context, userMessage string, ch chan<- AgentEvent, streaming bool) {
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
	a.hooks.Fire(ctx, HookOnSessionStart, &HookData{AgentID: "main", Meta: map[string]any{"user_message": userMessage}})
	defer a.hooks.Fire(ctx, HookOnSessionEnd, &HookData{AgentID: "main", Meta: map[string]any{"user_message": userMessage}})

	head, headErr := a.dag.GetHead()
	if headErr != nil {
		log.Printf("[dag] GetHead error: %v", headErr)
	}
	userContent := []t.ContentBlock{{Type: "text", Text: userMessage}}
	userNodeID, err := a.dag.AddNode(head, t.RoleUser, userContent, "", "", 0)
	if err != nil {
		emit(AgentEvent{Type: EventAgentError, Error: fmt.Errorf("add user node: %w", err)})
		return
	}

	var finalText string
	continuations := 0

	for turn := 0; a.config.MaxTurns == 0 || turn < a.config.MaxTurns; turn++ {
		emit(AgentEvent{Type: EventAgentTurnStart, Turn: turn})
		a.hooks.Fire(ctx, HookOnTurnStart, &HookData{AgentID: "main", Meta: map[string]any{"turn": turn}})

		// Snapshot the system prompt once per turn; the reload goroutine may
		// rewrite a.config.SystemPrompt concurrently.
		sysPrompt := a.systemPrompt()

		currentHead, chErr := a.dag.GetHead()
		if chErr != nil {
			log.Printf("[dag] GetHead error: %v", chErr)
		}
		messages, err := a.buildPrompt(currentHead)
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
				messages = sanitizeMessages(messages)
			} else {
				currentHead, compHeadErr := a.dag.GetHead()
				if compHeadErr != nil {
					log.Printf("[dag] GetHead error after compaction: %v", compHeadErr)
				}
				messages, err = a.buildPrompt(currentHead)
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

		// Estimate the message tokens once and reuse it below (token-count hook and
		// prompt-cost budget), instead of re-marshaling the full history each time.
		msgTokens := EstimateTokens(messages)
		a.hooks.Fire(ctx, HookOnTokenCount, &HookData{AgentID: "main", TokensIn: msgTokens, Meta: map[string]any{"estimated": true}})

		llmData := &HookData{AgentID: "main", Messages: messages, Meta: map[string]any{}}
		a.hooks.Fire(ctx, HookBeforeLLMCall, llmData)
		emit(AgentEvent{Type: EventStatusUpdate, StatusHook: "before_llm_call"})
		if llmData.Block {
			log.Printf("[loop] LLM call blocked: %s", llmData.BlockReason)
			break
		}
		// If a before-LLM hook swapped the message slice, the cached estimate is
		// stale — recompute so the budget below stays exact.
		if !sameMessageSlice(llmData.Messages, messages) {
			msgTokens = EstimateTokens(llmData.Messages)
		}
		messages = llmData.Messages

		a.configMu.RLock()
		toolDefs := append([]t.Tool(nil), a.config.Tools...)
		a.configMu.RUnlock()

		activeProvider := a.provider
		if a.RouteProvider != nil {
			activeProvider = a.RouteProvider(userMessage)
		}

		// Compute dynamic max_tokens: leave room for input instead of blindly
		// sending the configured max. This prevents 400 errors on models where
		// max_completion_tokens == context_window (common with free OpenRouter models).
		maxTokens := a.config.Provider.MaxTokens
		if a.config.ContextWindow > 0 {
			// Equivalent to EstimatePromptCost(sysPrompt, messages, toolDefs) but
			// reuses the message estimate computed above and the cached tool-schema
			// estimate instead of re-marshaling both every turn.
			inputCost := EstimateTokensForText(sysPrompt) + msgTokens
			if len(toolDefs) > 0 {
				inputCost += a.estimateToolTokensCached(toolDefs)
			}
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
					timer.Stop()
				case <-ctx.Done():
					timer.Stop()
					llmErr = ctx.Err()
				}
				if llmErr == ctx.Err() {
					break
				}
				log.Printf("[loop] retrying LLM call (attempt %d/4) after transient error", attempt+1)
			}
			if streaming {
				streamCh, streamErr := activeProvider.StreamComplete(ctx, sysPrompt, messages, toolDefs, maxTokens)
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
			} else {
				var completeErr error
				resp, completeErr = activeProvider.Complete(ctx, sysPrompt, messages, toolDefs, maxTokens)
				if completeErr != nil {
					llmErr = completeErr
					var te *t.TransientError
					if errors.As(completeErr, &te) {
						continue
					}
					break
				}
				// Emit text deltas so callbacks still fire.
				for _, block := range resp.Content {
					if block.Type == "text" && block.Text != "" {
						emit(AgentEvent{Type: EventAgentTextDelta, Text: block.Text})
					}
				}
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
		if aliasErr := a.dag.SetAlias(nodeID, a.dag.NextAutoAlias()); aliasErr != nil {
			log.Printf("[dag] SetAlias error: %v", aliasErr)
		}
		if a.config.PersistThinking && len(thinkingBlocks) > 0 {
			if _, thinkErr := a.dag.AddNode(nodeID, "thinking", thinkingBlocks, "", "", 0); thinkErr != nil {
				log.Printf("[dag] AddNode thinking error: %v", thinkErr)
			}
		}

		// Auto-continue when the model hit its token limit mid-response. Handle this
		// before the tool-use branch so a truncated response mid-tool-call also
		// triggers continuation rather than executing a partial tool call. Capped at
		// maxContinuations consecutive continuations to prevent an infinite spin.
		if resp.StopReason == "length" || resp.StopReason == "max_tokens" {
			if continuations >= maxContinuations {
				log.Printf("[loop] continuation cap reached (%d consecutive truncations, stop_reason=%q); not auto-continuing further", maxContinuations, resp.StopReason)
			} else {
				continuations++
				log.Printf("[loop] response truncated (stop_reason=%q), auto-continuing (%d/%d)", resp.StopReason, continuations, maxContinuations)
				contHead, _ := a.dag.GetHead()
				contContent := []t.ContentBlock{{Type: "text", Text: "Continue."}}
				if _, contErr := a.dag.AddNode(contHead, t.RoleUser, contContent, "", "", 0); contErr != nil {
					log.Printf("[loop] failed to add continuation node: %v", contErr)
				}
				emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue
			}
		} else {
			// Reset the consecutive-continuation counter once a full (non-truncated)
			// response arrives.
			continuations = 0
		}

		if !HasToolUse(resp) {
			finalText = ExtractText(resp)
			// HookOnStop: can override stop decision via Block.
			stopData := &HookData{AgentID: "main", Response: resp, Meta: map[string]any{"final_text": finalText}}
			a.hooks.Fire(ctx, HookOnStop, stopData)
			if stopData.Block {
				if len(stopData.Messages) > 0 {
					for _, msg := range stopData.Messages {
						sHead, shErr := a.dag.GetHead()
						if shErr != nil {
							log.Printf("[dag] GetHead error: %v", shErr)
						}
						if _, snErr := a.dag.AddNode(sHead, msg.Role, msg.Content, "", "", 0); snErr != nil {
							log.Printf("[dag] AddNode error: %v", snErr)
						}
					}
				}
				finalText = ""
				emit(AgentEvent{Type: EventAgentTurnEnd, Turn: turn, Usage: &resp.Usage})
				a.hooks.Fire(ctx, HookOnTurnEnd, &HookData{AgentID: "main", Response: resp})
				continue
			}
			exitData := &HookData{AgentID: "main", Response: resp, Messages: nil, Meta: map[string]any{"final_text": finalText}}
			a.hooks.Fire(ctx, HookBeforeLoopExit, exitData)
			if exitData.Block && len(exitData.Messages) > 0 {
				for _, msg := range exitData.Messages {
					fHead, fhErr := a.dag.GetHead()
					if fhErr != nil {
						log.Printf("[dag] GetHead error: %v", fhErr)
					}
					if _, fnErr := a.dag.AddNode(fHead, msg.Role, msg.Content, "", "", 0); fnErr != nil {
						log.Printf("[dag] AddNode error: %v", fnErr)
					}
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
				sHead, shErr := a.dag.GetHead()
				if shErr != nil {
					log.Printf("[dag] GetHead error: %v", shErr)
				}
				if _, snErr := a.dag.AddNode(sHead, msg.Role, msg.Content, "", "", 0); snErr != nil {
					log.Printf("[dag] AddNode error: %v", snErr)
				}
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

// promptCacheWalkLimit bounds the incremental parent-walk. Beyond this many
// newly-appended nodes we fall back to a full rebuild rather than issuing one
// GetNode query per node.
const promptCacheWalkLimit = 256

// buildPrompt returns the message context ending at head, using an incremental
// cache keyed by the head node ID.
//
// Common case (head advanced by appending descendants of the previously-cached
// head): it walks parent pointers from head back to the cached head and only
// fetches/parses the newly-appended nodes, reusing the cached ancestor slice.
//
// Any other change (empty cache, branch switch, compaction fork, head rewind,
// or a walk longer than promptCacheWalkLimit) falls back to a full GetAncestors
// rebuild, which is identical to the previous a.dag.PromptFrom(head) behaviour.
// Because the incremental path only activates after literally reaching the cached
// head via parent links, the cached prefix is always a valid prefix of head's
// ancestor chain; every other case rebuilds fully, so the result is always
// correct without any explicit cache invalidation.
func (a *Agent) buildPrompt(head string) ([]t.Message, error) {
	if head == "" {
		return nil, nil
	}

	a.promptCacheMu.Lock()
	cachedHead := a.promptCacheHead
	cachedNodes := a.promptCacheNodes
	a.promptCacheMu.Unlock()

	if head == cachedHead && cachedNodes != nil {
		return nodesToMessagesCopy(cachedNodes), nil
	}

	// Try the incremental path: walk back from head until we reach cachedHead.
	if cachedHead != "" && cachedNodes != nil {
		var suffix []Node // collected newest-first
		cur := head
		reached := false
		for i := 0; cur != "" && i < promptCacheWalkLimit; i++ {
			if cur == cachedHead {
				reached = true
				break
			}
			n, err := a.dag.GetNode(cur)
			if err != nil {
				break
			}
			suffix = append(suffix, *n)
			cur = n.ParentID
		}
		if reached {
			full := make([]Node, 0, len(cachedNodes)+len(suffix))
			full = append(full, cachedNodes...)
			for j := len(suffix) - 1; j >= 0; j-- { // reverse into oldest-first
				full = append(full, suffix[j])
			}
			a.promptCacheMu.Lock()
			a.promptCacheHead = head
			a.promptCacheNodes = full
			a.promptCacheMu.Unlock()
			return nodesToMessagesCopy(full), nil
		}
	}

	// Full rebuild.
	nodes, err := a.dag.GetAncestors(head)
	if err != nil {
		return nil, err
	}
	a.promptCacheMu.Lock()
	a.promptCacheHead = head
	a.promptCacheNodes = nodes
	a.promptCacheMu.Unlock()
	return nodesToMessagesCopy(nodes), nil
}

// nodesToMessagesCopy converts DAG nodes to messages, giving each message a
// fresh Content slice so that in-place mutation by the caller (e.g. sanitize's
// trailing-whitespace trim) can never corrupt the cached ancestor nodes.
func nodesToMessagesCopy(nodes []Node) []t.Message {
	msgs := make([]t.Message, len(nodes))
	for i, n := range nodes {
		c := make([]t.ContentBlock, len(n.Content))
		copy(c, n.Content)
		msgs[i] = t.Message{Role: t.Role(n.Role), Content: c}
	}
	return msgs
}

// sameMessageSlice reports whether a and b are the same underlying slice (same
// length and backing array), i.e. an in-place hook did not replace it. Used to
// decide whether a cached token estimate is still valid.
func sameMessageSlice(a, b []t.Message) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

// estimateToolTokensCached returns the token estimate for the tool schemas,
// caching it keyed by tool count. Tool schemas are static within a session, so
// this avoids re-marshaling them every turn. AddTool changes the count, which
// invalidates the cache naturally.
func (a *Agent) estimateToolTokensCached(tools []t.Tool) int {
	a.toolTokenMu.Lock()
	defer a.toolTokenMu.Unlock()
	if a.toolTokenLen != len(tools) {
		a.toolTokenEst = estimateToolTokens(tools)
		a.toolTokenLen = len(tools)
	}
	return a.toolTokenEst
}

func consumeStreamEmit(streamCh <-chan t.StreamEvent, emit func(AgentEvent)) (*t.AssistantMessage, error) {
	var resp *t.AssistantMessage
	var textBuf strings.Builder
	for ev := range streamCh {
		switch ev.Type {
		case t.EventTextDelta:
			textBuf.WriteString(ev.Text)
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
		// Stream closed without an explicit stop event. If we accumulated any text,
		// synthesize a completed assistant message rather than failing the whole turn.
		if textBuf.Len() > 0 {
			return &t.AssistantMessage{
				Message:    t.Message{Content: []t.ContentBlock{{Type: "text", Text: textBuf.String()}}},
				StopReason: "end_turn",
			}, nil
		}
		return nil, fmt.Errorf("stream ended without response")
	}
	return resp, nil
}

func (a *Agent) findTool(name string) *t.Tool {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
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
func (a *Agent) SystemPrompt() string     { return a.systemPrompt() }
func (a *Agent) AddTool(tool t.Tool) {
	a.configMu.Lock()
	a.config.Tools = append(a.config.Tools, tool)
	a.configMu.Unlock()
}

// maxContinuations caps consecutive auto-"Continue." injections triggered by a
// truncated (length/max_tokens) response, preventing an infinite continuation spin.
const maxContinuations = 10

// maxActiveFiles caps the number of tracked file paths.
const maxActiveFiles = 50

// ActiveFiles returns a snapshot of recently-touched file paths.
func (a *Agent) ActiveFiles() []string {
	a.activeFilesMu.RLock()
	defer a.activeFilesMu.RUnlock()
	out := make([]string, len(a.activeFiles))
	copy(out, a.activeFiles)
	return out
}

// trackActiveFiles extracts file paths from tool args and adds them to the tracker.
func (a *Agent) trackActiveFiles(args map[string]any) {
	var paths []string
	if fp, ok := args["file_path"].(string); ok && fp != "" {
		paths = append(paths, fp)
	}
	if p, ok := args["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return
	}
	a.activeFilesMu.Lock()
	defer a.activeFilesMu.Unlock()
	seen := make(map[string]bool, len(a.activeFiles))
	for _, f := range a.activeFiles {
		seen[f] = true
	}
	for _, p := range paths {
		if !seen[p] {
			a.activeFiles = append(a.activeFiles, p)
			seen[p] = true
		}
	}
	// Evict oldest entries if over cap.
	if len(a.activeFiles) > maxActiveFiles {
		a.activeFiles = a.activeFiles[len(a.activeFiles)-maxActiveFiles:]
	}
}

// ReloadSystemPrompt updates the system prompt and fires HookOnInstructionsLoaded.
// Hooks can modify the prompt via AdditionalContext.
func (a *Agent) ReloadSystemPrompt(ctx context.Context, newPrompt string) {
	data := &HookData{
		AgentID: "main",
		Meta:    map[string]any{"prompt_length": len(newPrompt)},
	}
	a.hooks.Fire(ctx, HookOnInstructionsLoaded, data)
	if data.AdditionalContext != "" {
		newPrompt = newPrompt + "\n\n" + data.AdditionalContext
	}
	a.configMu.Lock()
	a.config.SystemPrompt = newPrompt
	a.configMu.Unlock()
}

// systemPrompt returns the current system prompt under a read lock. The reload
// goroutine (ReloadSystemPrompt) may rewrite it concurrently with the loop.
func (a *Agent) systemPrompt() string {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config.SystemPrompt
}
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
			head, headErr := a.dag.GetHead()
			if headErr != nil {
				log.Printf("[dag] GetHead error: %v", headErr)
			}
			if _, addErr := a.dag.AddNode(head, msg.Role, msg.Content, "", "", 0); addErr != nil {
				log.Printf("[dag] AddNode error: %v", addErr)
			}
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

		a.trackActiveFiles(tc.Input)

		afterTool := &HookData{AgentID: "main", ToolName: tc.Name, ToolArgs: tc.Input, ToolResult: result}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			result = afterTool.ToolResult
		}
		if result.IsError {
			a.hooks.Fire(ctx, HookPostToolUseFailure, &HookData{AgentID: "main", ToolName: tc.Name, ToolArgs: tc.Input, ToolResult: result})
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: tc.Name, ToolArgs: tc.Input, ToolResult: result})

		toolHead, thErr := a.dag.GetHead()
		if thErr != nil {
			log.Printf("[dag] GetHead error: %v", thErr)
		}
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: result.ToolUseID, Content: result.Content, IsError: result.IsError}}
		if _, tnErr := a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0); tnErr != nil {
			log.Printf("[dag] AddNode error: %v", tnErr)
		}
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
		a.trackActiveFiles(pc.tc.Input)
		afterTool := &HookData{AgentID: "main", ToolName: pc.tc.Name, ToolArgs: pc.tc.Input, ToolResult: results[i]}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			results[i] = afterTool.ToolResult
		}
		if results[i].IsError {
			a.hooks.Fire(ctx, HookPostToolUseFailure, &HookData{AgentID: "main", ToolName: pc.tc.Name, ToolArgs: pc.tc.Input, ToolResult: results[i]})
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: pc.tc.Name, ToolArgs: pc.tc.Input, ToolResult: results[i]})

		toolHead, thErr := a.dag.GetHead()
		if thErr != nil {
			log.Printf("[dag] GetHead error: %v", thErr)
		}
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: results[i].ToolUseID, Content: results[i].Content, IsError: results[i].IsError}}
		if _, tnErr := a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0); tnErr != nil {
			log.Printf("[dag] AddNode error: %v", tnErr)
		}
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
	var textBuf strings.Builder
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
			textBuf.WriteString(ev.Text)
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
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						er.result = &t.ToolResult{ToolUseID: id, Content: "cancelled", IsError: true}
						close(er.done)
						return
					}
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
		// Stream closed without an explicit stop event. If we accumulated any text,
		// synthesize a completed assistant message rather than failing the whole turn.
		if textBuf.Len() > 0 {
			return &t.AssistantMessage{
				Message:    t.Message{Content: []t.ContentBlock{{Type: "text", Text: textBuf.String()}}},
				StopReason: "end_turn",
			}, results, nil
		}
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

		a.trackActiveFiles(er.args)

		afterTool := &HookData{AgentID: "main", ToolName: er.name, ToolArgs: er.args, ToolResult: er.result}
		a.hooks.Fire(ctx, HookAfterToolCall, afterTool)
		if afterTool.ToolResult != nil {
			er.result = afterTool.ToolResult
		}
		if er.result.IsError {
			a.hooks.Fire(ctx, HookPostToolUseFailure, &HookData{AgentID: "main", ToolName: er.name, ToolArgs: er.args, ToolResult: er.result})
		}
		emit(AgentEvent{Type: EventAgentToolEnd, ToolName: er.name, ToolArgs: er.args, ToolResult: er.result})

		toolHead, thErr := a.dag.GetHead()
		if thErr != nil {
			log.Printf("[dag] GetHead error: %v", thErr)
		}
		toolContent := []t.ContentBlock{{Type: "tool_result", ToolUseID: er.result.ToolUseID, Content: er.result.Content, IsError: er.result.IsError}}
		if _, tnErr := a.dag.AddNode(toolHead, t.RoleTool, toolContent, "", "", 0); tnErr != nil {
			log.Printf("[dag] AddNode error: %v", tnErr)
		}
	}
}
