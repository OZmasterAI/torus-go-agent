package core

import (
	"context"
	"sort"
	"sync"
)

type HookPoint string

const (
	HookBeforeLLMCall      HookPoint = "before_llm_call"
	HookAfterLLMCall       HookPoint = "after_llm_call"
	HookBeforeToolCall     HookPoint = "before_tool_call"
	HookAfterToolCall      HookPoint = "after_tool_call"
	HookAfterToolResult    HookPoint = "after_tool_result"
	HookBeforeContextBuild HookPoint = "before_context_build"
	HookAfterContextBuild  HookPoint = "after_context_build"
	HookOnTokenCount       HookPoint = "on_token_count"
	HookOnError            HookPoint = "on_error"
	HookOnAgentStart       HookPoint = "on_agent_start"
	HookOnAgentEnd         HookPoint = "on_agent_end"
	HookOnTurnStart        HookPoint = "on_turn_start"
	HookOnTurnEnd          HookPoint = "on_turn_end"
	HookOnStopFailure      HookPoint = "on_stop_failure"
	HookPreCompact         HookPoint = "pre_compact"
	HookPostCompact        HookPoint = "post_compact"
	HookBeforeNewBranch    HookPoint = "before_new_branch"
	HookAfterNewBranch     HookPoint = "after_new_branch"
	HookPreClear           HookPoint = "pre_clear"
	HookPostClear          HookPoint = "post_clear"
	HookBeforeLoopExit     HookPoint = "before_loop_exit"
)

type HookData struct {
	Point       HookPoint
	AgentID     string
	ToolName    string
	ToolArgs    map[string]any
	ToolResult  *ToolResult
	Messages    []Message
	Response    *AssistantMessage
	TokensIn    int
	TokensOut   int
	Block       bool
	BlockReason string
	Meta        map[string]any
}

type HookFn func(ctx context.Context, data *HookData) error

type hookEntry struct {
	name     string
	fn       HookFn
	priority int // lower runs first
}

type HookRegistry struct {
	mu    sync.RWMutex
	hooks map[HookPoint][]hookEntry
}

func NewHookRegistry() *HookRegistry {
	return &HookRegistry{hooks: make(map[HookPoint][]hookEntry)}
}

// Register adds a hook handler with default priority (100).
func (r *HookRegistry) Register(point HookPoint, name string, fn HookFn) {
	r.RegisterPriority(point, name, fn, 100)
}

// RegisterPriority adds a hook handler with explicit priority (lower runs first).
func (r *HookRegistry) RegisterPriority(point HookPoint, name string, fn HookFn, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[point] = append(r.hooks[point], hookEntry{name: name, fn: fn, priority: priority})
	sort.Slice(r.hooks[point], func(i, j int) bool {
		return r.hooks[point][i].priority < r.hooks[point][j].priority
	})
}

func (r *HookRegistry) Fire(ctx context.Context, point HookPoint, data *HookData) error {
	r.mu.RLock()
	entries := r.hooks[point]
	r.mu.RUnlock()
	data.Point = point
	for _, entry := range entries {
		if err := entry.fn(ctx, data); err != nil {
			return err
		}
		if data.Block {
			return nil
		}
	}
	return nil
}

func (r *HookRegistry) Count(point HookPoint) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks[point])
}
