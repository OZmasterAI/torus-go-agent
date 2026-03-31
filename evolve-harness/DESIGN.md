# Evolve-Harness: System Design

## Directory Structure

```
evolve-harness/
├── CONTEXT.md              # Background and architecture (this project's README)
├── DESIGN.md               # This file — system design
├── PLAN.md                 # Writing plans for steps 1-5
├── proposer.md             # Proposer skill (instructions for Claude Code)
├── evaluate.py             # Evaluator script
├── run.sh                  # Orchestrator loop
├── tasks/                  # Task definitions
│   ├── coding/             # 30 coding tasks
│   │   ├── 001_fizzbuzz/
│   │   │   ├── prompt.txt
│   │   │   ├── test.sh
│   │   │   └── workspace/  # starting files (if any)
│   │   └── ...
│   ├── tool_use/           # 15 tool-use tasks
│   │   ├── 001_read_edit/
│   │   │   ├── prompt.txt
│   │   │   ├── verify.py   # checks tool sequence in trace
│   │   │   └── workspace/
│   │   └── ...
│   └── compression/        # 10 compression tasks
│       ├── 001_long_convo/
│       │   ├── conversation.json
│       │   ├── queries.json
│       │   └── expected.json
│       └── ...
└── search_db/              # Accumulated during search (gitignored)
    ├── harness_000/        # Baseline
    │   ├── src/            # Snapshot of optimizable Go files
    │   ├── scores.json
    │   └── traces/
    │       ├── coding_001/result.json
    │       └── ...
    └── harness_001/        # First proposer iteration
        └── ...
```

## Four Evaluation Dimensions

### 1. Coding (weight: 0.35)

**What:** Can the agent solve coding tasks end-to-end?
**Tasks:** 30 tasks across 3 difficulty tiers (10 easy, 10 medium, 10 hard).
**Scoring:** Binary pass/fail via `test.sh` exit code.
**Score:** `coding_pass_rate = passed / total`

Easy tasks: single-file, clear spec (fizzbuzz, string reverse, file parser).
Medium tasks: multi-file, requires reading existing code (fix a bug, add a feature).
Hard tasks: multi-step reasoning, ambiguous requirements, error recovery.

Each task provides:
- `prompt.txt` — the user instruction
- `test.sh` — verification script (runs tests, diffs output, checks file existence)
- `workspace/` — starting files copied into a temp dir before the agent runs

### 2. Context Efficiency (weight: 0.20)

**What:** How many tokens does the agent use to achieve the same results?
**Tasks:** Same 30 coding tasks.
**Scoring:** `efficiency = coding_pass_rate / (total_input_tokens / baseline_input_tokens)`

A harness that passes 80% of tasks in 50K tokens scores higher than one that passes 85% in 200K tokens. This creates pressure to compress well and route efficiently.

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
**Scoring:** Information retention rate.

Pipeline:
1. Feed conversation.json messages into the agent (via batch channel multi-turn mode — Step 1 extends batch channel for this)
2. Trigger compression (the conversation exceeds the configured threshold)
3. After compression, ask the queries from queries.json
4. Score answers against expected.json (exact match on key facts, or LLM-as-judge)

**Score:** `compression_retention = correct_answers / total_queries`

### Composite Score

```python
composite = (
    0.35 * coding_pass_rate +
    0.20 * context_efficiency +
    0.25 * tool_precision +
    0.20 * compression_retention
)
```

The proposer sees all four scores and can reason about trade-offs. Pareto-optimal solutions are tracked.

## Evaluator (evaluate.py)

Single Python script that:

1. Takes `--harness=N` (harness directory) and `--tasks=path` (task directory)
2. For each task:
   a. Copy workspace files to a temp directory
   b. Run `./torus-agent --no-setup --batch=prompt.txt --output=trace_dir/`
   c. Run `test.sh` or `verify.py` against the output
   d. Collect score + trace
3. Compute per-dimension and composite scores
4. Write `scores.json` to the harness directory

Needs:
- `ANTHROPIC_API_KEY` or equivalent in env
- go_sdk_agent binary built with the harness's modified source files
- Task files in expected structure

## Proposer Skill (proposer.md)

An instruction file given to Claude Code when it acts as the proposer. Contains:

1. **Goal statement**: "You are optimizing go_sdk_agent's harness code to maximize a composite score across coding, efficiency, tool-use, and compression."
2. **Filesystem map**: Where to find prior harnesses, traces, and scores in `search_db/`.
3. **Optimizable surfaces**: Exact file paths and knob descriptions (from CONTEXT.md).
4. **Constraints**: Don't modify provider code, don't change the batch channel, don't change tool implementations (only descriptions).
5. **Output format**: Write modified files to `search_db/harness_NNN/src/`.
6. **Strategy guidance**: "Read traces from failing tasks before proposing changes. Prefer additive changes over modifications to existing logic. State your hypothesis before each change."

## Orchestrator (run.sh)

```bash
#!/bin/bash
set -euo pipefail

MAX_ITER=${MAX_ITER:-10}
TASKS_DIR="evolve-harness/tasks"
SEARCH_DB="evolve-harness/search_db"

# Step 0: Evaluate baseline if not done
if [ ! -f "$SEARCH_DB/harness_000/scores.json" ]; then
    echo "Evaluating baseline..."
    snapshot_harness 000
    python evolve-harness/evaluate.py --harness=000 --tasks=$TASKS_DIR
fi

# Step 1..N: Proposer loop
for i in $(seq 1 $MAX_ITER); do
    HARNESS_ID=$(printf "%03d" $i)

    # 1. Run proposer
    claude -p "$(cat evolve-harness/proposer.md)" \
        --allowedTools Read,Write,Edit,Bash,Grep,Glob

    # 2. Apply modifications and rebuild
    apply_harness $HARNESS_ID
    go build -o torus-agent-eval ./cmd/

    # 3. Evaluate
    python evolve-harness/evaluate.py \
        --harness=$HARNESS_ID \
        --tasks=$TASKS_DIR \
        --binary=./torus-agent-eval

    # 4. Log
    echo "Harness $HARNESS_ID: $(cat $SEARCH_DB/harness_$HARNESS_ID/scores.json)"
done

echo "Search complete. Best harness:"
python -c "import json,glob; scores=[json.load(open(f)) for f in glob.glob('$SEARCH_DB/*/scores.json')]; print(max(scores, key=lambda s: s['composite']))"
```

## Multi-Turn Batch Mode (Extension)

For compression tasks, the batch channel needs to support multi-turn conversations. Extension:

```bash
./torus-agent --no-setup --batch=conversation.json --output=traces/comp_001/ --multi-turn
```

Where `conversation.json` is an array of user messages. The batch channel sends them sequentially, collecting traces for each turn. This triggers natural compression when the conversation grows long enough.

## Cost Estimate

Per iteration (30 coding + 15 tool + 10 compression tasks):
- ~55 agent runs per iteration
- ~5K-50K input tokens per run (varies with task difficulty)
- Proposer reads ~10M tokens of search_db per iteration
- Estimated: $15-40 per iteration at Opus pricing
- 10 iterations: $150-400 total

The coding tasks dominate cost because they involve multi-turn tool use. Compression tasks are cheaper (mostly context, fewer LLM calls).
