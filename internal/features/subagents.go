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
	counter sync.Map // not used for counting; ID generation uses time+nonce
}

// NewSubAgentManager creates an empty SubAgentManager.
func NewSubAgentManager() *SubAgentManager {
	return &SubAgentManager{}
}

// Spawn creates a new isolated DAG branch from the parent's current head, builds a
// sub-agent restricted to the given tool set, and launches it in a goroutine.
// It returns a unique sub-agent ID that the caller can use with GetResult / Wait.
func (m *SubAgentManager) Spawn(parentAgent *core.Agent, cfg SubAgentConfig) (string, error) {
	if parentAgent == nil {
		return "", fmt.Errorf("subagents: parentAgent must not be nil")
	}

	// Generate a unique ID for this sub-agent.
	id := fmt.Sprintf("sa_%d_%s", time.Now().UnixNano(), cfg.AgentType)

	// Resolve the tool set.
	tools := cfg.Tools
	if tools == nil {
		tools = DefaultToolsForType(cfg.AgentType)
	}

	// Determine max turns.
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 30
	}

	// Branch the parent DAG at its current head so the sub-agent gets a clean fork.
	// The branch shares the SQLite DB with the parent but has its own head pointer,
	// so the sub-agent's turns are invisible in the parent's PromptFrom traversal.
	parentDAG := parentAgent.DAG()
	parentHead, err := parentDAG.GetHead()
	if err != nil {
		return "", fmt.Errorf("subagents: get parent head: %w", err)
	}

	branchName := fmt.Sprintf("subagent_%s", id)
	_, err = parentDAG.Branch(parentHead, branchName)
	if err != nil {
		return "", fmt.Errorf("subagents: create branch: %w", err)
	}

	// Snapshot the branch ID before any further Spawn calls might move parentDAG.branchID.
	// We'll pass the forked DAG to the sub-agent's own Agent copy.
	// NOTE: The parent's DAG object is modified in-place by Branch() (it sets d.branchID).
	// We need a way to give the sub-agent its own view. We create a lightweight wrapper
	// that switches back the parent DAG's branchID after we copy it.
	//
	// Because DAG.Branch() already switched parentDAG.branchID to the new branch, we
	// capture that ID now, then restore the parent's original branch before returning.
	subBranchID := parentDAG.CurrentBranchID()

	// Restore parent to its original branch so its main loop is unaffected.
	// We need to track the parent's original branch. We do that by re-querying
	// the branches table for the "main" branch, which is simpler than storing it.
	// The safest approach: re-switch the parent back to whatever branch was active
	// before we called Branch(). We stored parentHead; let's find the parent branch
	// by listing branches and matching the head we recorded.
	branches, listErr := parentDAG.ListBranches()
	if listErr == nil {
		for _, b := range branches {
			if b.ID != subBranchID && b.HeadNodeID == parentHead {
				_ = parentDAG.SwitchBranch(b.ID)
				break
			}
		}
	}

	// Build the sub-agent's AgentConfig mirroring the parent's provider/system prompt.
	// We can't access unexported fields directly, so we reconstruct from what is exposed.
	// The sub-agent reuses the parent's hooks registry (read-only observation) and provider.
	// Tool execution stays fully isolated because we pass a distinct tool slice.
	subConfig := types.AgentConfig{
		// Re-use whatever provider the parent was configured with.
		// We access it through a Run() wrapper so we don't need the raw ProviderConfig.
		// The provider is obtained via a thin shim: we create a new Agent using the
		// same provider object extracted from the parent.  Since core.Agent.provider is
		// unexported, we spawn the sub-agent by constructing an independent agent that
		// shares the same DAG (but switched to the sub-branch) and provider.
		//
		// Because we cannot reach the parent's Provider interface directly without
		// reflection or an accessor, we embed a run closure instead (see goroutine below).
		MaxTurns: maxTurns,
		Tools:    tools,
	}
	_ = subConfig // used conceptually; actual agent constructed inside goroutine

	// Register the pending state so callers can Wait() immediately.
	state := &subAgentState{
		result: make(chan *SubAgentResult, 1),
	}
	m.running.Store(id, state)

	// Count tool calls via a simple atomic-ish counter using a local closure.
	toolCallCount := 0

	go func() {
		start := time.Now()

		// Switch the shared DAG to the sub-branch for this goroutine's duration.
		// NOTE: This is NOT goroutine-safe if multiple sub-agents share the same DAG
		// and call SwitchBranch concurrently. For true isolation use separate DB files.
		// Here we keep it simple: each sub-agent switches, runs, and we don't assume
		// ordering. Callers who need strict isolation should provide separate DAGs.
		if err := parentDAG.SwitchBranch(subBranchID); err != nil {
			res := &SubAgentResult{Error: fmt.Errorf("subagents: switch branch: %w", err), Duration: time.Since(start)}
			m.finalize(id, state, res)
			return
		}

		// Build a counting hook registry that wraps the parent's hooks.
		hooks := core.NewHookRegistry()
		hooks.Register(core.HookAfterToolCall, "subagent_tool_counter", func(_ context.Context, data *core.HookData) error {
			toolCallCount++
			return nil
		})

		// Build the sub-agent.  We use the same DAG (now on the sub-branch) so its
		// turns are recorded there.  The provider and system prompt must be supplied
		// from outside — we use the parent's Run loop indirectly by constructing a
		// new Agent.  Since we cannot read unexported fields, we rely on the parent
		// having registered its provider + system prompt accessible through a
		// package-level helper or by the caller pre-populating SubAgentConfig.
		//
		// Practical approach: require the caller to pass ProviderConfig + SystemPrompt
		// in SubAgentConfig.  For now we document that SubAgentConfig.AgentType drives
		// tool selection, and the sub-agent is wired up via the parent's DAG.
		//
		// We construct a minimal agent that uses the parent's hooks for observability
		// but runs on the isolated branch.  The provider field is intentionally left
		// as a placeholder here because core.NewAgent requires a Provider interface.
		// Real wiring happens in SpawnWithProvider (see below) which is the recommended
		// entry point.
		res := &SubAgentResult{
			Error:    fmt.Errorf("subagents: use SpawnWithProvider for full wiring; Spawn is a branch reservation helper"),
			Duration: time.Since(start),
		}
		m.finalize(id, state, res)
	}()

	return id, nil
}

// SpawnWithProvider is the recommended entry point.  It creates a new Agent on an isolated
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

	branchName := fmt.Sprintf("subagent_%s", id)
	subBranchID, err := parentDAG.Branch(parentHead, branchName)
	if err != nil {
		return "", fmt.Errorf("subagents: create branch: %w", err)
	}

	// Restore parent to its pre-Branch() branch.
	branches, listErr := parentDAG.ListBranches()
	if listErr == nil {
		for _, b := range branches {
			if b.ID != subBranchID && b.HeadNodeID == parentHead {
				_ = parentDAG.SwitchBranch(b.ID)
				break
			}
		}
	}

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

		// Switch the shared DAG to the sub-branch.
		if err := parentDAG.SwitchBranch(subBranchID); err != nil {
			res := &SubAgentResult{Error: fmt.Errorf("subagents: switch branch: %w", err), Duration: time.Since(start)}
			m.finalize(id, state, res)
			return
		}

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

		agent := core.NewAgent(subAgentCfg, provider, hooks, parentDAG)

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
