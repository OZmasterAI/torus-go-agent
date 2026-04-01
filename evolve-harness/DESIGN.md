# Evolve-Harness: System Design

## Directory Structure

```
evolve-harness/
├── CONTEXT.md              # Background and architecture
├── DESIGN.md               # This file — system design
├── PLAN.md                 # Implementation plans for steps 1-5
├── proposer.md             # Proposer skill (instructions for Claude Code)
├── evaluate.py             # Evaluator script
├── run.sh                  # Orchestrator loop
├── tasks/                  # Task definitions
│   ├── coding/             # 15 harness-sensitive coding tasks
│   │   ├── 001_long_debug/
│   │   │   ├── prompt.txt
│   │   │   ├── test.sh
│   │   │   └── workspace/
│   │   └── ...
│   ├── tool_use/           # 15 tool-use tasks
│   │   ├── 001_read_edit/
│   │   │   ├── prompt.txt
│   │   │   ├── verify.py
│   │   │   └── workspace/
│   │   └── ...
│   └── compression/        # 10 compression tasks
│       ├── 001_long_convo/
│       │   ├── conversation.json
│       │   ├── queries.json
│       │   └── expected.json
│       └── ...
└── search_db/              # Accumulated during search (gitignored)
    ├── harness_000/
    │   ├── src/            # Preserves package paths
    │   │   ├── core/compression.go
    │   │   ├── core/compression_ops.go
    │   │   ├── core/context.go
    │   │   ├── providers/anthropic.go
    │   │   ├── tools/tools.go
    │   │   └── features/routing.go
    │   ├── scores.json
    │   └── traces/
    │       ├── coding_001/result.json
    │       └── ...
    └── harness_001/
        └── ...
```

## Four Evaluation Dimensions

### 1. Coding (weight: 0.35)

**What:** Can the agent solve tasks that stress harness-quality?
**Tasks:** 15 tasks designed to be sensitive to compression, context management, tool routing, and prompt framing — NOT pure model capability tests.

**Key principle:** A task like "write fizzbuzz" tests the model, not the harness. The model passes or fails regardless of compression weights. Good tasks are ones where harness decisions (how much context to keep, which tool to suggest, when to compress) make the difference between pass and fail.

**Task categories (5 each):**

**Long-horizon (5):** Tasks requiring 10+ turns where context management determines success.
- Workspace has 5+ files. Agent must read, plan, then modify multiple files.
- Context exceeds 50% of window by mid-task — compression quality matters.
- Errors are seeded that require reading earlier tool output to fix.

**Tool-sensitive (5):** Tasks where using the wrong tool causes failure or waste.
- Edit tasks where `write` would destroy formatting but `edit` preserves it.
- Search tasks where `bash: grep` misses results that `grep` tool catches.
- Tasks where redundant reads waste context budget.

**Error-recovery (5):** Tasks where the first approach fails and the agent must adapt.
- Workspace has misleading file names. Agent must recover from wrong assumptions.
- Build errors that require reading compiler output and fixing iteratively.
- Tasks where the prompt is deliberately ambiguous — harness framing shapes model behavior.

**Scoring:** Binary pass/fail via `test.sh` exit code.
**Score:** `coding_pass_rate = passed / total`

Each task provides:
- `prompt.txt` — user instruction
- `test.sh` — verification (runs tests, diffs output, checks files)
- `workspace/` — starting files copied to temp dir

### 2. Context Efficiency (weight: 0.20)

**What:** How many tokens does the agent spend per successful task?
**Tasks:** Same 15 coding tasks.
**Metric:** `tokens_per_pass = total_input_tokens / max(tasks_passed, 1)`

Lower is better. This is NOT a ratio against baseline — it's an absolute metric. The proposer sees both `coding_pass_rate` and `tokens_per_pass` as separate numbers and can reason about the tradeoff.

Normalized for scoring: `efficiency_score = baseline_tokens_per_pass / tokens_per_pass` (capped at 1.0). A harness that uses half the tokens per pass scores 1.0; one that uses 2x scores 0.5.

### 3. Tool-Use Precision (weight: 0.25)

**What:** Does the agent use the right tools in the right way?
**Tasks:** 15 tasks designed to test specific tool-use patterns.
**Scoring:** Per-task: `task_pass * tool_score`. Tool score checks:

| Check | Points | Example |
|---|---|---|
| Used required tool | +1.0 | Task says "edit file" → must use `edit`, not `write` |
| Avoided banned tool | +0.5 | Task says "search" → should use `grep`, not `bash: grep` |
| Tool order correct | +0.3 | Read before edit (not blind edit) |
| No redundant calls | +0.2 | Didn't read the same file 3 times |

**Score:** `tool_precision = sum(task_scores) / max_possible`

Each task provides:
- `prompt.txt` — instruction
- `verify.py` — reads `result.json` trace, checks tool sequence
- Optional: `required_tools.json`, `banned_tools.json`

### 4. Compression Quality (weight: 0.20)

**What:** After compression, can the agent still answer questions about earlier conversation?
**Tasks:** 10 tasks, each with a multi-turn conversation + post-compression queries.

**Pipeline:**
1. Set `ContextWindow=8000` and `ContinuousCompression=true` to force compression early.
2. Feed `conversation.json` messages via batch multi-turn mode.
3. After all messages (compression has fired multiple times), ask queries from `queries.json`.
4. Score answers via keyword extraction: each expected answer lists 3-5 key terms, actual answer gets 1 point per term present.

**Why keyword scoring:** LLM-as-judge adds noise and cost per iteration. Exact match is too brittle. Keywords are deterministic, cheap, and sufficient for "did compression preserve this fact?"

**Score:** `compression_retention = total_keywords_found / total_keywords_expected`

Each task provides:
- `conversation.json` — array of `{"role": "user", "content": "..."}` messages
- `queries.json` — array of `{"question": "...", "keywords": ["term1", "term2", ...]}`
- No `expected.json` needed — keywords are in `queries.json`

### Composite Score

```python
composite = (
    0.35 * coding_pass_rate +
    0.20 * efficiency_score +
    0.25 * tool_precision +
    0.20 * compression_retention
)
```

The proposer sees ALL raw metrics (pass rate, tokens_per_pass, per-task tool scores, per-query keyword hits) plus the composite. It can reason about trade-offs.

## Evaluator (evaluate.py)

Single Python script:

1. Takes `--harness=N`, `--tasks=path`, `--binary=path`, `--search-db=path`
2. For each coding task:
   a. Copy workspace/ to temp dir
   b. Run: `binary --no-setup --batch=prompt.txt --output=trace_dir/ --workdir=temp_dir/`
   c. Run: `test.sh` in temp dir
   d. Collect pass/fail + token counts from result.json
3. For each tool-use task:
   a. Same as coding, but also run `verify.py` on result.json
4. For each compression task:
   a. Run: `binary --no-setup --batch=conversation.json --output=trace_dir/ --multi-turn --context-window=8000`
   b. For each query: run agent with query, check keywords in response
5. Compute per-dimension and composite scores
6. Write `scores.json` + `detailed_results.json`

## Proposer Skill (proposer.md)

Instructions for Claude Code acting as the proposer. Key sections:
1. Goal statement with composite formula
2. Filesystem map (where in search_db/ to find code, traces, scores)
3. Optimizable surfaces with exact file paths and knob descriptions
4. Constraints (what NOT to modify)
5. Output format (write to `search_db/harness_NNN/src/{package}/`)
6. Strategy: read traces first, state hypothesis, prefer additive changes, one change per iteration

## Orchestrator (run.sh)

```bash
snapshot_harness() {
    mkdir -p "$SEARCH_DB/harness_$1/src/core"
    mkdir -p "$SEARCH_DB/harness_$1/src/providers"
    mkdir -p "$SEARCH_DB/harness_$1/src/tools"
    mkdir -p "$SEARCH_DB/harness_$1/src/features"
    cp internal/core/compression.go internal/core/compression_ops.go internal/core/context.go "$SEARCH_DB/harness_$1/src/core/"
    cp internal/providers/anthropic.go "$SEARCH_DB/harness_$1/src/providers/"
    cp internal/tools/tools.go "$SEARCH_DB/harness_$1/src/tools/"
    cp internal/features/routing.go "$SEARCH_DB/harness_$1/src/features/"
}

apply_harness() {
    cp "$SEARCH_DB/harness_$1/src/core/"*.go internal/core/
    cp "$SEARCH_DB/harness_$1/src/providers/"*.go internal/providers/
    cp "$SEARCH_DB/harness_$1/src/tools/"*.go internal/tools/
    cp "$SEARCH_DB/harness_$1/src/features/"*.go internal/features/
}
```

Main loop: baseline eval → proposer → apply + build → evaluate → revert → report. Build failures skip the iteration. After all iterations, print Pareto frontier.

## Batch Channel Flags

```bash
# Single task with workspace
./torus-agent --no-setup --batch=prompt.txt --output=traces/001/ --workdir=/tmp/workspace/

# Multi-turn (compression evaluation)
./torus-agent --no-setup --batch=conversation.json --output=traces/comp/ --multi-turn
```

`--workdir` changes the process working directory before the agent loop starts, so all tool operations (read, write, edit, bash) operate on the task's workspace files.

## Cost Estimate

Per iteration (15 coding + 15 tool + 10 compression = 40 tasks):
- ~40 agent runs per iteration
- ~5K-50K input tokens per run
- Proposer reads search_db (~10M tokens) per iteration
- Estimated: $15-30 per iteration at Opus pricing
- 10 iterations: $150-300 total
- MVP (10 tasks, 3 iterations): ~$30
