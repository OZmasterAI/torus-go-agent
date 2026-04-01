# Meta-Harness Proposer

## Your Task
Optimize go_sdk_agent's harness code to maximize:
  composite = 0.35*coding + 0.20*efficiency + 0.25*tools + 0.20*compression

## Search Database
evolve-harness/search_db/ contains all prior attempts:
  harness_000/  — baseline (unmodified go_sdk_agent)
  harness_001/  — first iteration
  ...

Each directory contains:
  src/core/compression.go       — compression scoring weights, zone budgets
  src/core/compression_ops.go   — operation boundary detection, scoring signals
  src/core/context.go           — compaction strategy, keepLastN, threshold
  src/providers/anthropic.go    — system prompt, cache breakpoints
  src/tools/tools.go            — tool descriptions
  src/features/routing.go       — smart routing thresholds
  scores.json                   — per-dimension and composite scores
  detailed_results.json         — per-task breakdowns
  traces/                       — result.json per task (turn-by-turn events)
  hypothesis.md                 — (your) reasoning for this iteration's changes

## Process
1. Read scores.json for ALL prior harnesses — understand the trajectory
2. Identify the worst-performing dimension
3. Read traces from 3-5 FAILING tasks in that dimension
4. Read the source code that controls that dimension
5. Form a hypothesis: "Task X fails because [specific mechanism]"
6. Write hypothesis.md FIRST
7. Make a MINIMAL modification to one file
8. Write modified files to search_db/harness_NNN/src/{package}/

## Rules
- ONE change per iteration. Do not bundle fixes.
- ADDITIVE over destructive. Meta-Harness found 5/7 modification attempts regressed.
- Read RAW TRACES (tool calls, turn-by-turn). Scores alone tell you nothing about WHY.
- If 2+ prior iterations regressed, explain the confound before trying again.
- Do not modify: batch.go, loop.go, dag.go, hook implementations, tool Execute functions.
- Do not introduce new Go files. Only modify existing ones listed above.
- Copy ALL source files for the harness, not just the one you changed — the evaluator
  applies ALL files from the harness directory.

## Knob Reference (key tunable parameters)

### compression.go
- DefaultToolLimits: head/tail line counts per tool (bash: 30/15, read: 60/20, etc.)
- UnifiedCompressConfig: KeepFirst, KeepLast (10), MinMessages, ArchivePct (25%)
- Template threshold: score < 0.3 → archive, >= 0.3 → template one-liner
- Per-op cap: 50% of zone2Effective budget

### compression_ops.go
- ScoreOperation weights: recency (0.20), file_overlap (0.20), outcome (0.20), op_type (0.20), causal (0.20)
- boundaryThreshold: 0.20
- Tool type transition weight: 0.35, file scope weight: 0.30, intent signal weight: 0.20

### context.go
- CompactionConfig: Threshold (0-100%), KeepLastN (10), Trigger ("tokens"/"messages"/"both")

### anthropic.go
- applyCacheControl: marks last 2 messages (could be 3)
- System prompt blocks: identity + custom, both with cache_control

### tools.go
- Tool descriptions: text that shapes which tool the model chooses
- Tool names and input schemas

### routing.go
- IsSimpleMessage: maxSimpleBytes, maxSimpleWords, complexityKeywords list

## Strategy Tips
- Start with the lowest-scoring dimension
- For coding failures: check if compression drops critical context (read traces for tool_end
  events that get archived — their results disappear from the model's view)
- For tool-use failures: check tool descriptions — if the model picks write instead of edit,
  the edit description may be unclear
- For compression failures: check KeepLast, ArchivePct, and template threshold — too
  aggressive archiving loses facts
- For efficiency: check per-tool truncation limits — overly generous limits waste tokens
- Compare your hypothesis against the raw trace BEFORE committing to a change

## Output Format
For each iteration, write to search_db/harness_NNN/:
1. hypothesis.md — your reasoning (required)
2. src/core/compression.go — copy from baseline or modify
3. src/core/compression_ops.go — copy from baseline or modify
4. src/core/context.go — copy from baseline or modify
5. src/providers/anthropic.go — copy from baseline or modify
6. src/tools/tools.go — copy from baseline or modify
7. src/features/routing.go — copy from baseline or modify
