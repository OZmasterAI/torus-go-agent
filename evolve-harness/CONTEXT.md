# Evolve-Harness: Context Document

## What This Is

An outer-loop optimization system for go_sdk_agent, inspired by Stanford IRIS Lab's Meta-Harness paper (arXiv:2603.28052, March 2026). The core idea: use a coding agent (Claude Code) to iteratively discover better harness configurations by reading raw execution traces from prior attempts.

## Why It Matters

Meta-Harness showed that giving a proposer agent access to a full filesystem of prior code + raw traces + scores produces dramatically better results than compressed summaries or scalar scores alone (50.0 vs 34.9 median accuracy on text classification). The key is causal reasoning: the proposer can trace "this task failed because X prompt directive caused the model to run Y command at turn 8" rather than just seeing "score: 58.9%".

## What Meta-Harness Does (The Algorithm)

```
Input: task set X, frozen model M, proposer agent P
1. Seed search_db/ with baseline harness (code + traces + scores)
2. For t = 1..N iterations:
   a. Proposer P reads search_db/ (all prior code, traces, scores)
   b. P proposes new harness modification (writes Python/Go files)
   c. Evaluate new harness on task set X → collect traces + scores
   d. Add (code, traces, scores) to search_db/
3. Return best harness from search_db/
```

No gradients. No RL. No fine-tuning. Just a coding agent iteratively debugging its own system with perfect memory.

## go_sdk_agent Architecture (What We're Optimizing)

go_sdk_agent is a Go-based LLM agent harness with:

- **Providers**: Anthropic, OpenRouter, OpenAI, Gemini, etc. (frozen — not optimized)
- **Core loop** (`internal/core/loop.go`): ReAct loop — build context, call LLM, execute tools, repeat
- **Compression** (`internal/core/compression.go`, `compression_ops.go`): Operation-based scoring, zone budgeting, per-tool truncation
- **Context** (`internal/core/context.go`): Compaction strategies (sliding, LLM-summarize, DAG-based)
- **Tools** (`internal/tools/`): 6 default tools (bash, read, write, edit, glob, grep) + MCP tools
- **Channels** (`internal/channels/`): TUI, HTTP, Telegram, Batch (new)
- **DAG** (`internal/core/dag.go`): Persistent conversation tree with branching
- **Hooks** (`internal/core/hooks.go`): 30 named hook points for observability/mutation

### Optimizable Surfaces

| Surface | File(s) | Knobs |
|---|---|---|
| System prompt | `providers/anthropic.go` | Prompt text, cache breakpoint count (currently 2 system + 2 message) |
| Compression scoring | `core/compression_ops.go` | 5 signal weights (all 0.20), boundary threshold (0.20) |
| Compression zones | `core/compression.go` | ArchivePct (25%), zone2=25% hardcoded, template threshold (0.3), per-op cap (50%) |
| Per-tool truncation | `core/compression.go` | `DefaultToolLimits` — head/tail line counts per tool type |
| Compaction config | `core/context.go` | Threshold (0-100%), KeepLastN, trigger mode, summarizer |
| Compression config | `core/compression.go` | KeepFirst, KeepLast (10), MinMessages, ArchivePct (25%) |
| Tool descriptions | `tools/tools.go` | Description text shapes model tool choice |
| Smart routing | `features/routing.go` | `complexityKeywords`, byte/word thresholds, `IsSimpleMessage()` logic |
| Skill prompts | `config/skills/*.md` | Instruction content prepended to user messages |

### Event System

`AgentEvent` types emitted by `RunStream()`:
- `turn_start` / `turn_end` (with `Usage`: input/output/cache tokens + cost)
- `tool_start` / `tool_end` (with tool name, args, result)
- `text_delta` / `thinking_delta`
- `done` (final text) / `error`

## Step 0: Batch Channel (DONE)

**Files created:**
- `internal/channels/batch/batch.go` — 170 lines, implements `Channel` interface
- `internal/channels/batch/batch_test.go` — 3 tests (missing file, empty prompt, full run)
- `cmd/main.go` — 3 edits (import, flag parsing, skip-setup)

**Usage:**
```bash
./torus-agent --no-setup --batch=prompt.txt --output=traces/001/
```

**Output:** `result.json` containing prompt, response, turns, token counts, cost, duration, tool call count, and full trace array (turn events, tool events, usage per turn).

**Trace format:**
```json
{
  "prompt": "...",
  "response": "...",
  "turns": 3,
  "total_input_tokens": 15234,
  "total_output_tokens": 892,
  "total_cost": 0.0234,
  "duration_ms": 4521,
  "tool_calls": 5,
  "trace": [
    {"time": "...", "type": "turn_start", "turn": 1},
    {"time": "...", "type": "tool_start", "tool_name": "read", "tool_args": {"file_path": "/foo"}},
    {"time": "...", "type": "tool_end", "tool_name": "read", "tool_result": "..."},
    {"time": "...", "type": "turn_end", "turn": 1, "usage": {"input_tokens": 5078, ...}},
    {"time": "...", "type": "done", "text": "..."}
  ]
}
```

## Branch

All work is on `evolve-harness` branch, forked from `master` at `9643bd1`.
