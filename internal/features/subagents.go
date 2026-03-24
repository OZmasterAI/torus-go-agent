package features

import (
	"context"
	"fmt"
	"sync"
	"time"

	"torus_go_agent/internal/core"
	"torus_go_agent/internal/tools"
	"torus_go_agent/internal/types"
)

// Tool is a local alias to avoid repeating the full qualified name throughout this file.
type Tool = types.Tool

// SubAgentConfig specifies how a sub-agent should be created and what it should do.
type SubAgentConfig struct {
	// Task is the user message sent to the sub-agent.
	Task string
	// AgentType controls the default restricted tool set when Tools is nil.
	// Recognised values: "builder", "researcher", "tester". Anything else gets all 6 tools.
	AgentType string
	// Tools is an explicit restricted tool set. When non-nil it overrides AgentType tool selection.
	Tools []Tool
	// MaxTurns caps the sub-agent's ReAct loop. 0 uses the parent's default (30).
	MaxTurns int
}

// SubAgentResult holds the outcome of a completed sub-agent run.
type SubAgentResult struct {
	Text      string
	Error     error
	Duration  time.Duration
	ToolCalls int
}

// subAgentState is the internal per-instance tracking record.
type subAgentState struct {
	result chan *SubAgentResult // closed (and written once) when done
}

// SubAgentManager tracks goroutine-based sub-agents with thread-safe maps.
type SubAgentManager struct {
	running sync.Map // map[string]*subAgentState  — entries present while goroutine alive
	results sync.Map // map[string]*SubAgentResult — persisted after goroutine finishes
}

// NewSubAgentManager creates an empty SubAgentManager.
func NewSubAgentManager() *SubAgentManager {
	return &SubAgentManager{}
}

// SpawnWithProvider is the entry point for launching sub-agents.  It creates a new Agent on an isolated
// DAG branch with the given provider, system prompt, and tool set, then runs it in a goroutine.
func (m *SubAgentManager) SpawnWithProvider(
	parentAgent *core.Agent,
	provider types.Provider,
	systemPrompt string,
	cfg SubAgentConfig,
) (string, error) {
	if parentAgent == nil {
		return "", fmt.Errorf("subagents: parentAgent must not be nil")
	}
	if provider == nil {
		return "", fmt.Errorf("subagents: provider must not be nil")
	}

	id := fmt.Sprintf("sa_%d_%s", time.Now().UnixNano(), cfg.AgentType)

	tools := cfg.Tools
	if tools == nil {
		tools = DefaultToolsForType(cfg.AgentType)
	}
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 30
	}

	parentDAG := parentAgent.DAG()
	parentHead, err := parentDAG.GetHead()
	if err != nil {
		return "", fmt.Errorf("subagents: get parent head: %w", err)
	}

	// Save parent branch, create sub-branch (Branch switches branchID), then restore.
	parentBranchID := parentDAG.CurrentBranchID()
	branchName := fmt.Sprintf("subagent_%s", id)
	subBranchID, err := parentDAG.Branch(parentHead, branchName)
	if err != nil {
		return "", fmt.Errorf("subagents: create branch: %w", err)
	}
	_ = parentDAG.SwitchBranch(parentBranchID)

	// Fork an independent DAG for the sub-agent (shares DB, own branchID).
	subDAG := parentDAG.Fork(subBranchID)

	state := &subAgentState{result: make(chan *SubAgentResult, 1)}
	m.running.Store(id, state)

	// Fire before_spawn hook on parent's hooks (can block)
	spawnData := &core.HookData{
		AgentID: id,
		Meta:    map[string]any{"agent_type": cfg.AgentType, "task": cfg.Task, "branch": subBranchID},
	}
	parentAgent.Hooks().Fire(context.Background(), core.HookBeforeSpawn, spawnData)
	if spawnData.Block {
		res := &SubAgentResult{Error: fmt.Errorf("subagents: spawn blocked: %s", spawnData.BlockReason)}
		m.finalize(id, state, res)
		return id, nil
	}

	go func() {
		start := time.Now()

		// Fire after_spawn once the goroutine starts
		parentAgent.Hooks().Fire(context.Background(), core.HookAfterSpawn, &core.HookData{
			AgentID: id,
			Meta:    map[string]any{"agent_type": cfg.AgentType, "task": cfg.Task, "branch": subBranchID},
		})

		// Count tool calls.
		toolCallCount := 0
		hooks := core.NewHookRegistry()
		hooks.Register(core.HookAfterToolCall, "subagent_tool_counter", func(_ context.Context, data *core.HookData) error {
			toolCallCount++
			return nil
		})

		subAgentCfg := types.AgentConfig{
			Provider:     types.ProviderConfig{MaxTokens: 8192},
			SystemPrompt: systemPrompt,
			Tools:        tools,
			MaxTurns:     maxTurns,
		}

		agent := core.NewAgent(subAgentCfg, provider, hooks, subDAG)

		text, runErr := agent.Run(context.Background(), cfg.Task)

		res := &SubAgentResult{
			Text:      text,
			Error:     runErr,
			Duration:  time.Since(start),
			ToolCalls: toolCallCount,
		}

		// Fire on_subagent_complete
		parentAgent.Hooks().Fire(context.Background(), core.HookOnSubagentComplete, &core.HookData{
			AgentID: id,
			Meta: map[string]any{
				"agent_type": cfg.AgentType,
				"text":       text,
				"error":      runErr,
				"duration":   res.Duration.String(),
				"tool_calls": toolCallCount,
			},
		})

		m.finalize(id, state, res)
	}()

	return id, nil
}

// finalize stores the result, closes the channel, and removes from running map.
func (m *SubAgentManager) finalize(id string, state *subAgentState, res *SubAgentResult) {
	m.results.Store(id, res)
	state.result <- res
	close(state.result)
	m.running.Delete(id)
}

// GetResult returns the result for the given ID if the sub-agent has completed.
// Returns nil, false if the sub-agent is still running or the ID is unknown.
func (m *SubAgentManager) GetResult(id string) (*SubAgentResult, bool) {
	v, ok := m.results.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*SubAgentResult), true
}

// ListRunning returns the IDs of all sub-agents that have not yet finished.
func (m *SubAgentManager) ListRunning() []string {
	var ids []string
	m.running.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// Wait blocks until the sub-agent with the given ID completes and returns its result.
// If the ID is unknown or the sub-agent already finished, it returns the stored result immediately.
func (m *SubAgentManager) Wait(id string) *SubAgentResult {
	// If already done, return immediately.
	if res, ok := m.GetResult(id); ok {
		return res
	}

	// Look up the running state.
	v, ok := m.running.Load(id)
	if !ok {
		// Unknown ID — return a sentinel error result.
		return &SubAgentResult{Error: fmt.Errorf("subagents: unknown sub-agent id %q", id)}
	}

	state := v.(*subAgentState)
	res := <-state.result
	return res
}

// DefaultToolsForType returns the restricted tool set for a given agent type.
// It always returns a fresh copy from BuildDefaultTools() so callers cannot mutate the originals.
//
//   - "builder"    — all 6 tools (bash, read, write, edit, glob, grep)
//   - "researcher" — read, glob, grep only (no write/edit/bash)
//   - "tester"     — bash, read, glob, grep (no write/edit)
//   - anything else — all 6 tools
func DefaultToolsForType(agentType string) []Tool {
	all := tools.BuildDefaultTools()
	switch agentType {
	case "researcher":
		return filterTools(all, "read", "glob", "grep")
	case "tester":
		return filterTools(all, "bash", "read", "glob", "grep")
	default: // "builder" and any unknown type
		return all
	}
}

// filterTools returns a subset of tools matching the given names, preserving order.
func filterTools(tools []Tool, names ...string) []Tool {
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}
	out := make([]Tool, 0, len(names))
	for _, t := range tools {
		if allowed[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
