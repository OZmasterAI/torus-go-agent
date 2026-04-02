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
		if parentAgent != nil {
			parentAgent.Hooks().Fire(ctx, core.HookOnTaskCreated, &core.HookData{
				AgentID: cfg.AgentType,
				Meta:    map[string]any{"agent_type": cfg.AgentType, "task": cfg.Task},
			})
		}
		id, err := subMgr.SpawnWithProvider(parentAgent, provider, systemPrompt, cfg)
		if err != nil {
			return result, err
		}
		res := subMgr.Wait(id)
		if parentAgent != nil {
			parentAgent.Hooks().Fire(ctx, core.HookOnTaskCompleted, &core.HookData{
				AgentID: cfg.AgentType,
				Meta: map[string]any{
					"agent_type": cfg.AgentType,
					"text":       res.Text,
					"error":      res.Error,
					"duration":   res.Duration.String(),
				},
			})
		}
		subMgr.DeleteResult(id)
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
		if parentAgent != nil {
			parentAgent.Hooks().Fire(ctx, core.HookOnTaskCreated, &core.HookData{
				AgentID: cfg.AgentType,
				Meta:    map[string]any{"agent_type": cfg.AgentType, "task": cfg.Task},
			})
		}
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
			if parentAgent != nil {
				parentAgent.Hooks().Fire(ctx, core.HookOnTaskCompleted, &core.HookData{
					AgentID: agentID,
					Meta: map[string]any{
						"text":     res.Text,
						"error":    res.Error,
						"duration": res.Duration.String(),
					},
				})
			}
			results[idx] = *res
		}(i, id)
	}
	wg.Wait()
	for _, id := range ids {
		subMgr.DeleteResult(id)
	}
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
		if parentAgent != nil {
			parentAgent.Hooks().Fire(ctx, core.HookOnTaskCreated, &core.HookData{
				AgentID: iterCfg.AgentType,
				Meta:    map[string]any{"agent_type": iterCfg.AgentType, "task": iterCfg.Task},
			})
		}
		id, err := subMgr.SpawnWithProvider(parentAgent, provider, systemPrompt, iterCfg)
		if err != nil {
			return result, err
		}
		res := subMgr.Wait(id)
		if parentAgent != nil {
			parentAgent.Hooks().Fire(ctx, core.HookOnTaskCompleted, &core.HookData{
				AgentID: iterCfg.AgentType,
				Meta: map[string]any{
					"agent_type": iterCfg.AgentType,
					"text":       res.Text,
					"error":      res.Error,
					"duration":   res.Duration.String(),
				},
			})
		}
		subMgr.DeleteResult(id)
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
