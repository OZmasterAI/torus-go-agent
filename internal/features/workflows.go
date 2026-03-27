// workflows.go provides higher-level orchestration primitives built on
// SubAgentManager: sequential pipelines, parallel fan-out, and iterative
// loops with configurable stop conditions.
package features

import (
	"context"
	"sync"

	"torus_go_agent/internal/core"
	"torus_go_agent/internal/types"
)

// RunSequential runs agents one after another. Each agent receives the previous
// agent's output appended to the original input. Returns the final agent's output.
func RunSequential(
	ctx context.Context,
	dag *core.DAG,
	provider types.Provider,
	systemPrompt string,
	agents []SubAgentConfig,
	subMgr *SubAgentManager,
	parentAgent *core.Agent,
) (string, error) {
	result := ""
	for _, cfg := range agents {
		if result != "" {
			cfg.Task = cfg.Task + "\n\n[Previous agent output]:\n" + result
		}
		id, err := subMgr.SpawnWithProvider(parentAgent, provider, systemPrompt, cfg)
		if err != nil {
			return result, err
		}
		res := subMgr.Wait(id)
		if res.Error != nil {
			return result, res.Error
		}
		result = res.Text
	}
	return result, nil
}

// RunParallel runs agents concurrently and returns all results.
func RunParallel(
	ctx context.Context,
	dag *core.DAG,
	provider types.Provider,
	systemPrompt string,
	agents []SubAgentConfig,
	subMgr *SubAgentManager,
	parentAgent *core.Agent,
) ([]SubAgentResult, error) {
	ids := make([]string, 0, len(agents))
	for _, cfg := range agents {
		id, err := subMgr.SpawnWithProvider(parentAgent, provider, systemPrompt, cfg)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	results := make([]SubAgentResult, len(ids))
	var wg sync.WaitGroup
	wg.Add(len(ids))
	for i, id := range ids {
		go func(idx int, agentID string) {
			defer wg.Done()
			res := subMgr.Wait(agentID)
			results[idx] = *res
		}(i, id)
	}
	wg.Wait()
	return results, nil
}

// RunLoop repeats a single agent configuration until shouldStop returns true
// or maxIterations is reached (0 = no limit). The previous result is appended
// to the task on each iteration.
func RunLoop(
	ctx context.Context,
	dag *core.DAG,
	provider types.Provider,
	systemPrompt string,
	cfg SubAgentConfig,
	subMgr *SubAgentManager,
	parentAgent *core.Agent,
	shouldStop func(result string, iteration int) bool,
	maxIterations int,
) (string, error) {
	result := ""
	for i := 0; maxIterations == 0 || i < maxIterations; i++ {
		iterCfg := cfg
		if result != "" {
			iterCfg.Task = cfg.Task + "\n\n[Previous iteration output]:\n" + result
		}
		id, err := subMgr.SpawnWithProvider(parentAgent, provider, systemPrompt, iterCfg)
		if err != nil {
			return result, err
		}
		res := subMgr.Wait(id)
		if res.Error != nil {
			return result, res.Error
		}
		result = res.Text
		if shouldStop != nil && shouldStop(result, i) {
			break
		}
	}
	return result, nil
}
