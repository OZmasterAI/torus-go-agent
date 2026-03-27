// telemetry.go collects timing and usage metrics from hook events and
// exposes them as structured spans and aggregate counters.
package features

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"torus_go_agent/internal/core"
)

// TelemetryCollector collects timing and usage metrics from hook events.
// All methods are goroutine-safe.
type TelemetryCollector struct {
	mu      sync.Mutex
	spans   []Span
	metrics Metrics
}

// Span represents a timed operation.
type Span struct {
	Name      string
	Start     time.Time
	Duration  time.Duration
	Meta      map[string]any
}

// Metrics holds aggregate counters.
type Metrics struct {
	LLMCalls      int
	LLMErrors     int
	ToolCalls     int
	ToolErrors    int
	TokensIn      int
	TokensOut     int
	Turns         int
	Compactions   int
	SubAgents     int
	TotalLLMTime  time.Duration
	TotalToolTime time.Duration
}

// NewTelemetryCollector creates a new collector.
func NewTelemetryCollector() *TelemetryCollector {
	return &TelemetryCollector{}
}

// GetMetrics returns a snapshot of current metrics.
func (tc *TelemetryCollector) GetMetrics() Metrics {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.metrics
}

// GetSpans returns a copy of all recorded spans.
func (tc *TelemetryCollector) GetSpans() []Span {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	out := make([]Span, len(tc.spans))
	copy(out, tc.spans)
	return out
}

// Summary returns a human-readable metrics summary.
func (tc *TelemetryCollector) Summary() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	m := tc.metrics
	avgLLM := time.Duration(0)
	if m.LLMCalls > 0 {
		avgLLM = m.TotalLLMTime / time.Duration(m.LLMCalls)
	}
	return fmt.Sprintf(
		"llm_calls=%d (errors=%d, avg=%s, total=%s) tool_calls=%d (errors=%d, total=%s) tokens_in=%d tokens_out=%d turns=%d compactions=%d subagents=%d",
		m.LLMCalls, m.LLMErrors, avgLLM, m.TotalLLMTime,
		m.ToolCalls, m.ToolErrors, m.TotalToolTime,
		m.TokensIn, m.TokensOut,
		m.Turns, m.Compactions, m.SubAgents,
	)
}

// RegisterHooks registers all telemetry hooks on the given registry.
// This is the single entry point — call once during setup.
func (tc *TelemetryCollector) RegisterHooks(hooks *core.HookRegistry) {
	// LLM timing
	hooks.RegisterPriority(core.HookBeforeLLMCall, "telemetry-llm-start", func(ctx context.Context, d *core.HookData) error {
		d.Meta["_telem_llm_start"] = time.Now()
		return nil
	}, 1) // priority 1 = runs first

	hooks.RegisterPriority(core.HookAfterLLMCall, "telemetry-llm-end", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		defer tc.mu.Unlock()
		tc.metrics.LLMCalls++
		tc.metrics.TokensIn += d.TokensIn
		tc.metrics.TokensOut += d.TokensOut
		if start, ok := d.Meta["_telem_llm_start"].(time.Time); ok {
			dur := time.Since(start)
			tc.metrics.TotalLLMTime += dur
			tc.spans = append(tc.spans, Span{Name: "llm.call", Start: start, Duration: dur, Meta: map[string]any{
				"tokens_in": d.TokensIn, "tokens_out": d.TokensOut,
			}})
		}
		return nil
	}, 1)

	// Tool timing
	hooks.RegisterPriority(core.HookBeforeToolCall, "telemetry-tool-start", func(ctx context.Context, d *core.HookData) error {
		d.Meta["_telem_tool_start"] = time.Now()
		return nil
	}, 1)

	hooks.RegisterPriority(core.HookAfterToolCall, "telemetry-tool-end", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		defer tc.mu.Unlock()
		tc.metrics.ToolCalls++
		if d.ToolResult != nil && d.ToolResult.IsError {
			tc.metrics.ToolErrors++
		}
		if start, ok := d.Meta["_telem_tool_start"].(time.Time); ok {
			dur := time.Since(start)
			tc.metrics.TotalToolTime += dur
			tc.spans = append(tc.spans, Span{Name: "tool." + d.ToolName, Start: start, Duration: dur})
		}
		return nil
	}, 1)

	// Turn counter
	hooks.Register(core.HookOnTurnEnd, "telemetry-turn", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		tc.metrics.Turns++
		tc.mu.Unlock()
		return nil
	})

	// Error counter
	hooks.Register(core.HookOnError, "telemetry-error", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		tc.metrics.LLMErrors++
		tc.mu.Unlock()
		return nil
	})

	// Compaction counter
	hooks.Register(core.HookPostCompact, "telemetry-compact", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		tc.metrics.Compactions++
		tc.mu.Unlock()
		return nil
	})

	// Sub-agent counter
	hooks.Register(core.HookOnSubagentComplete, "telemetry-subagent", func(ctx context.Context, d *core.HookData) error {
		tc.mu.Lock()
		tc.metrics.SubAgents++
		tc.mu.Unlock()
		return nil
	})

	log.Println("[telemetry] hooks registered")
}
