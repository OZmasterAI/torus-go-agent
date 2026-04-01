# Evolve-Harness: Implementation Plans (Steps 1-5)

## Prerequisites

- [x] Step 0: Batch channel (`internal/channels/batch/batch.go`) — DONE
- [x] `--workdir` flag for workspace isolation — DONE
- [x] Branch: `evolve-harness`

---

## Step 1: Task Set + Multi-Turn Batch

**Goal:** Create 40 evaluation tasks with automated scoring, plus multi-turn batch mode.

### 1A. Multi-Turn Batch Extension (MUST be first — compression tasks depend on it)

Extend batch channel to support `--multi-turn` flag.

**File:** `internal/channels/batch/batch.go`

**Changes (~40 lines):**
```go
// Add to Config struct:
MultiTurn bool

// In Start(), before the agent loop:
if Config.MultiTurn {
    return b.runMultiTurn(agent, prompt, outputDir)
}

// New method:
func (b *batchChannel) runMultiTurn(agent *core.Agent, rawPrompt string, outputDir string) error {
    // 1. Parse rawPrompt as JSON array of strings (each is a user message)
    // 2. For each message: agent.Run(ctx, msg) — collect response + trace per turn
    // 3. Write result.json with per-turn traces and cumulative metrics
}
```

**Flag wiring in cmd/main.go:**
```go
if arg == "--multi-turn" {
    batchchan.Config.MultiTurn = true
}
```

**Tests:**
```go
func TestBatch_MultiTurn_ParsesJSONArray(t *testing.T)   // valid JSON array → runs N turns
func TestBatch_MultiTurn_InvalidJSON(t *testing.T)       // garbage input → error
func TestBatch_MultiTurn_TracksAllTurns(t *testing.T)    // result.json has events from all turns
```

### 1B. Coding Tasks (15 harness-sensitive tasks)

Each task in `evolve-harness/tasks/coding/NNN_name/` with:
- `prompt.txt` — user instruction
- `test.sh` — exits 0=pass, 1=fail
- `workspace/` — starting files

#### Long-Horizon (5) — context management determines success

```
001_multi_file_fix/
    # workspace: 6 Go files with 3 related bugs across files.
    # Agent must read all files, understand dependencies, fix in correct order.
    # Mid-task context exceeds 50% window — what gets compressed matters.
    # test.sh: go test ./...

002_iterative_debug/
    # workspace: Go project that builds but fails 3 tests.
    # Agent must: run tests → read failure → fix → re-run → fix next → re-run.
    # 10+ turns guaranteed. Earlier test output must survive compression.
    # test.sh: go test ./... (all 3 must pass)

003_chain_edits/
    # workspace: 8 Python files. prompt: "Rename class User to Account everywhere."
    # Must grep, then edit each file. Order matters (imports break if partial).
    # Compression must preserve which files were already edited.
    # test.sh: python3 -m pytest && ! grep -r "class User" *.py

004_config_cascade/
    # workspace: app with config.yaml, 3 modules reading it, tests.
    # prompt: "Add a 'timeout' field, wire it through all modules, add tests."
    # Must track which modules are done across many turns.
    # test.sh: go test ./... && grep -q "timeout" config.yaml

005_doc_from_code/
    # workspace: 10 Go files with complex logic, no comments.
    # prompt: "Write a DESIGN.md explaining the architecture."
    # Must read all files (lots of context), then synthesize.
    # test.sh: test -f DESIGN.md && wc -w DESIGN.md | awk '$1 >= 200'
```

#### Tool-Sensitive (5) — wrong tool = failure or context waste

```
006_preserve_formatting/
    # workspace: carefully formatted YAML with comments.
    # prompt: "Change the port from 8080 to 9090."
    # write tool destroys comments/formatting. edit preserves them.
    # test.sh: grep -q "9090" config.yaml && grep -q "# Main server port" config.yaml

007_large_file_search/
    # workspace: 2000-line Go file + 5 small files.
    # prompt: "Find all functions that return (string, error)."
    # Reading the 2000-line file wastes context. grep is the right tool.
    # test.sh: diff expected_functions.txt <(sort output.txt)

008_targeted_edit/
    # workspace: 500-line Python file with one typo on line 247.
    # prompt: "Fix the typo in utils.py"
    # write rewrites 500 lines (wasteful). edit changes 1 line.
    # test.sh: python3 utils.py  (typo causes NameError)

009_multi_grep_edit/
    # workspace: 12 files, 4 contain "FIXME" comments with instructions.
    # prompt: "Address all FIXME comments."
    # Must grep → read relevant → edit. Not read all 12 files.
    # test.sh: ! grep -r "FIXME" *.py && python3 -m pytest

010_bash_only_task/
    # workspace: none. prompt: "Create a directory structure: src/{lib,bin,test}/ with a Makefile"
    # bash is the right tool for mkdir/touch. read/write are wrong.
    # test.sh: test -d src/lib && test -d src/bin && test -f Makefile
```

#### Error-Recovery (5) — first attempt fails, must adapt

```
011_misleading_error/
    # workspace: Python project. The actual bug is in deps, not the reported file.
    # prompt: "Fix the import error in main.py" (but main.py is fine; utils.py is broken)
    # Agent must read error, realize misdirection, search wider.
    # test.sh: python3 main.py

012_build_fix_cycle/
    # workspace: Go project with 3 compile errors. Each fix reveals the next.
    # Agent must: build → read error → fix → build → read error → fix → build.
    # If compression drops earlier build output, agent re-introduces old bugs.
    # test.sh: go build ./... && go test ./...

013_wrong_assumption/
    # workspace: files named "database.py" (actually a mock) and "cache.py" (actual DB logic).
    # prompt: "Fix the database connection timeout."
    # Agent will likely read database.py first (wrong file). Must recover.
    # test.sh: python3 -m pytest test_connection.py

014_partial_success/
    # workspace: 4 test files, 2 are passing, 2 are failing.
    # prompt: "Make all tests pass."
    # If agent fixes one then breaks a passing test, must notice and fix both.
    # test.sh: python3 -m pytest (all 4 must pass)

015_ambiguous_spec/
    # workspace: Go HTTP server. prompt: "Add authentication."
    # Deliberately vague. Good harness framing → agent asks clarifying then implements basic.
    # Bad framing → agent builds something over-complex that doesn't compile.
    # test.sh: go build ./... && curl -s localhost:0/health (must return 401 without auth header)
```

### 1C. Tool-Use Tasks (15 tasks)

Each in `evolve-harness/tasks/tool_use/NNN_name/`.

(Same as original plan — these are already harness-sensitive by design.)

```
001_read_then_edit/    # Required: ["read", "edit"]. Banned: ["write"]
002_search_codebase/   # Required: ["grep"]. Banned: ["bash"]
003_find_files/        # Required: ["glob"]. Banned: ["bash"]
004_create_new_file/   # Required: ["write"]. Banned: ["bash"]
005_multi_file_edit/   # Required: ["glob", "read", "edit"]
006_run_tests/         # Required: ["bash"]
007_read_no_reread/    # verify: read file exactly once
008_edit_not_rewrite/  # Required: ["edit"]. Banned: ["write"]
009_search_then_fix/   # Required: ["grep", "edit"]
010_bash_for_system/   # Required: ["bash"]
011_glob_pattern/      # Required: ["glob"]
012_sequential_ops/    # Required: ["read", "edit"] (not write)
013_error_recovery/    # verify: graceful error handling, no excessive retries
014_minimal_tools/     # verify: exactly 1 tool call for a simple question
015_skill_trigger/     # verify: skill was triggered from trace
```

### 1D. Compression Tasks (10 tasks)

Each in `evolve-harness/tasks/compression/NNN_name/`.

Conversations are realistic multi-turn sessions. Queries test retention of specific facts.

```
001_long_coding/       # 40 turns of coding. Keywords: file names, function names, error messages
002_multi_topic/       # 30 turns, 3 topics. Keywords: topic-specific terms
003_error_chain/       # 25 turns debugging. Keywords: root cause, fix description
004_decision_trail/    # 20 turns decision-making. Keywords: options considered, chosen option, reason
005_code_review/       # 35 turns code review. Keywords: files reviewed, issues found
006_refactor_history/  # 30 turns refactoring. Keywords: old names, new names, moved files
007_config_changes/    # 20 turns config work. Keywords: setting names, values set
008_test_debugging/    # 25 turns test fixing. Keywords: test names, failure modes
009_architecture/      # 30 turns arch discussion. Keywords: patterns chosen, rejected alternatives
010_mixed_tools/       # 35 turns with varied tools. Keywords: tool outputs, file contents
```

Each `queries.json`:
```json
[
  {"question": "What file contained the original bug?", "keywords": ["utils.py", "TypeError"]},
  {"question": "What function was renamed?", "keywords": ["processData", "handleInput"]},
  {"question": "Why did we reject option B?", "keywords": ["latency", "500ms", "unacceptable"]}
]
```

3-5 queries per task, 3-5 keywords per query. Scoring: case-insensitive substring match.

---

## Step 2: Evaluator

**Goal:** Python script that runs the agent against all tasks and produces scores.

### File: `evolve-harness/evaluate.py`

```python
#!/usr/bin/env python3
"""Evaluate a harness against the task suite."""

import argparse, json, os, shutil, subprocess, sys, tempfile
from pathlib import Path

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--harness", required=True)
    parser.add_argument("--tasks", required=True)
    parser.add_argument("--binary", default="./torus-agent")
    parser.add_argument("--search-db", default="evolve-harness/search_db")
    parser.add_argument("--baseline-tpp", type=float, default=0)  # tokens_per_pass for baseline
    args = parser.parse_args()

    harness_dir = f"{args.search_db}/harness_{args.harness}"
    traces_dir = f"{harness_dir}/traces"
    os.makedirs(traces_dir, exist_ok=True)

    coding = evaluate_coding_tasks(args.binary, f"{args.tasks}/coding", traces_dir)
    tools = evaluate_tool_tasks(args.binary, f"{args.tasks}/tool_use", traces_dir)
    compression = evaluate_compression_tasks(args.binary, f"{args.tasks}/compression", traces_dir)

    scores = compute_scores(coding, tools, compression, args.baseline_tpp)
    write_scores(harness_dir, scores)

def run_agent(binary, prompt_file, output_dir, workdir=None, extra_flags=None):
    """Run the agent binary in batch mode. Returns result.json as dict or None on failure."""
    cmd = [binary, "--no-setup", f"--batch={prompt_file}", f"--output={output_dir}"]
    if workdir:
        cmd.append(f"--workdir={workdir}")
    if extra_flags:
        cmd.extend(extra_flags)
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    result_path = os.path.join(output_dir, "result.json")
    if os.path.exists(result_path):
        return json.load(open(result_path))
    return None

def evaluate_coding_tasks(binary, tasks_dir, traces_dir):
    """Returns list of {task, passed, tokens_in, tokens_out, cost, duration_ms}."""

def evaluate_tool_tasks(binary, tasks_dir, traces_dir):
    """Returns list of {task, passed, tool_score, details}."""

def evaluate_compression_tasks(binary, tasks_dir, traces_dir):
    """Returns list of {task, keywords_found, keywords_total}."""

def check_keywords(response_text, keywords):
    """Case-insensitive substring match. Returns count of keywords found."""
    found = 0
    lower = response_text.lower()
    for kw in keywords:
        if kw.lower() in lower:
            found += 1
    return found

def compute_scores(coding, tools, compression, baseline_tpp):
    """Compute all metrics and composite score."""
    coding_pass_rate = sum(1 for c in coding if c["passed"]) / max(len(coding), 1)
    tasks_passed = max(sum(1 for c in coding if c["passed"]), 1)
    total_tokens = sum(c["tokens_in"] for c in coding)
    tokens_per_pass = total_tokens / tasks_passed

    if baseline_tpp > 0:
        efficiency_score = min(baseline_tpp / tokens_per_pass, 1.0)
    else:
        efficiency_score = 1.0  # first run IS the baseline

    tool_precision = sum(t["tool_score"] for t in tools) / max(sum(t.get("max_score", 2.0) for t in tools), 1)
    kw_found = sum(c["keywords_found"] for c in compression)
    kw_total = max(sum(c["keywords_total"] for c in compression), 1)
    compression_retention = kw_found / kw_total

    composite = (0.35 * coding_pass_rate + 0.20 * efficiency_score +
                 0.25 * tool_precision + 0.20 * compression_retention)

    return {
        "coding_pass_rate": coding_pass_rate,
        "tokens_per_pass": tokens_per_pass,
        "efficiency_score": efficiency_score,
        "tool_precision": tool_precision,
        "compression_retention": compression_retention,
        "composite": composite,
        "total_input_tokens": total_tokens,
        "total_output_tokens": sum(c.get("tokens_out", 0) for c in coding),
        "total_cost": sum(c.get("cost", 0) for c in coding + tools + compression),
    }

def write_scores(harness_dir, scores):
    with open(f"{harness_dir}/scores.json", "w") as f:
        json.dump(scores, f, indent=2)
```

### Tests: `evolve-harness/test_evaluate.py`

```python
def test_check_keywords():         # "foo bar" contains ["foo", "baz"] → 1/2
def test_compute_scores_baseline(): # known inputs → expected composite
def test_compute_scores_improved():  # fewer tokens → higher efficiency
def test_run_agent_missing_binary(): # graceful failure
```

---

## Step 3: Proposer Skill

**Goal:** Instruction file for Claude Code as the proposer.

### File: `evolve-harness/proposer.md`

```markdown
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
```

---

## Step 4: Orchestrator

**Goal:** Shell script that runs the full search loop.

### File: `evolve-harness/run.sh`

```bash
#!/bin/bash
set -euo pipefail

SEARCH_DB="evolve-harness/search_db"
TASKS="evolve-harness/tasks"
MAX_ITER=${MAX_ITER:-10}
BINARY=${BINARY:-./torus-agent}

snapshot_harness() {
    local ID=$1
    mkdir -p "$SEARCH_DB/harness_$ID/src/"{core,providers,tools,features}
    cp internal/core/compression.go internal/core/compression_ops.go internal/core/context.go \
       "$SEARCH_DB/harness_$ID/src/core/"
    cp internal/providers/anthropic.go "$SEARCH_DB/harness_$ID/src/providers/"
    cp internal/tools/tools.go "$SEARCH_DB/harness_$ID/src/tools/"
    cp internal/features/routing.go "$SEARCH_DB/harness_$ID/src/features/"
}

apply_harness() {
    local ID=$1
    cp "$SEARCH_DB/harness_$ID/src/core/"*.go internal/core/
    cp "$SEARCH_DB/harness_$ID/src/providers/"*.go internal/providers/
    cp "$SEARCH_DB/harness_$ID/src/tools/"*.go internal/tools/
    cp "$SEARCH_DB/harness_$ID/src/features/"*.go internal/features/
}

# === Baseline ===
if [ ! -f "$SEARCH_DB/harness_000/scores.json" ]; then
    echo "=== Evaluating baseline ==="
    snapshot_harness 000
    go build -o "$BINARY" ./cmd/
    python3 evolve-harness/evaluate.py --harness=000 --tasks="$TASKS" --binary="$BINARY" --search-db="$SEARCH_DB"
    echo "Baseline: $(cat $SEARCH_DB/harness_000/scores.json | python3 -c 'import sys,json; s=json.load(sys.stdin); print(f"composite={s[\"composite\"]:.3f}")')"
fi

# Get baseline tokens_per_pass for efficiency scoring
BASELINE_TPP=$(python3 -c "import json; print(json.load(open('$SEARCH_DB/harness_000/scores.json'))['tokens_per_pass'])")

# === Search Loop ===
for i in $(seq 1 $MAX_ITER); do
    ID=$(printf "%03d" $i)
    echo ""
    echo "=== Iteration $ID ==="

    # 1. Run proposer
    echo "Running proposer..."
    claude -p "$(cat evolve-harness/proposer.md)

Current iteration: $ID
Search database: $SEARCH_DB/
Write new harness to: $SEARCH_DB/harness_$ID/src/

Read the search_db to understand what's been tried. Propose your modification." \
        --allowedTools Read,Write,Edit,Grep,Glob

    # 2. Apply + build
    if [ ! -d "$SEARCH_DB/harness_$ID/src" ]; then
        echo "Proposer didn't write files — skipping"
        continue
    fi

    apply_harness "$ID"
    if ! go build -o "$BINARY" ./cmd/ 2>"$SEARCH_DB/harness_$ID/build_error.txt"; then
        echo "Build failed — see $SEARCH_DB/harness_$ID/build_error.txt"
        echo '{"error": "build_failed", "composite": 0}' > "$SEARCH_DB/harness_$ID/scores.json"
        apply_harness "000"
        continue
    fi

    # 3. Evaluate
    echo "Evaluating..."
    python3 evolve-harness/evaluate.py \
        --harness="$ID" --tasks="$TASKS" --binary="$BINARY" \
        --search-db="$SEARCH_DB" --baseline-tpp="$BASELINE_TPP"

    # 4. Revert to baseline source
    apply_harness "000"

    # 5. Report
    echo "Harness $ID: $(cat $SEARCH_DB/harness_$ID/scores.json | python3 -c 'import sys,json; s=json.load(sys.stdin); print(f"composite={s.get(\"composite\",0):.3f} coding={s.get(\"coding_pass_rate\",0):.2f} tools={s.get(\"tool_precision\",0):.2f} compression={s.get(\"compression_retention\",0):.2f}")')"
done

# === Summary ===
echo ""
echo "=== Search Complete ==="
python3 -c "
import json, glob, os
scores = []
for f in sorted(glob.glob('$SEARCH_DB/*/scores.json')):
    s = json.load(open(f))
    if 'composite' in s and s['composite'] > 0:
        hid = os.path.basename(os.path.dirname(f))
        scores.append((s['composite'], hid, s))
scores.sort(reverse=True)
print('Top 5 harnesses:')
for composite, hid, s in scores[:5]:
    print(f'  {hid}: composite={composite:.3f}  coding={s[\"coding_pass_rate\"]:.2f}  efficiency={s[\"efficiency_score\"]:.2f}  tools={s[\"tool_precision\"]:.2f}  compression={s[\"compression_retention\"]:.2f}')
best_id = scores[0][1] if scores else 'none'
print(f'\nBest: {best_id}')
print(f'Apply with: apply_harness {best_id.split(\"_\")[1]}')
"
```

---

## Step 5: End-to-End Integration

**Goal:** Run the full pipeline once (MVP) to validate everything works.

### MVP Scope
- 5 coding tasks (2 long-horizon, 2 tool-sensitive, 1 error-recovery)
- 3 tool-use tasks
- 2 compression tasks
- 3 proposer iterations

### Checklist

- [ ] Build baseline binary
- [ ] Run evaluator on MVP tasks → verify `scores.json` is correct
- [ ] Verify traces contain expected events (turns, tools, usage)
- [ ] Run proposer once → verify it reads `search_db/`, writes valid Go files, writes `hypothesis.md`
- [ ] Build with proposer's modifications → verify compilation
- [ ] Re-evaluate → verify new `scores.json` (different from baseline)
- [ ] Run full orchestrator for 3 iterations → verify loop completes
- [ ] Review proposer's `hypothesis.md` files → verify causal reasoning from traces
- [ ] Check Pareto frontier output → verify ranking

### Post-MVP
After MVP validates the pipeline:
1. Expand to full 40-task suite
2. Run 10 iterations
3. Apply best harness to `master` branch
4. Compare before/after on real usage

---

## Implementation Order (Revised)

| Step | What | Depends On | Files |
|------|------|-----------|-------|
| **1A** | Multi-turn batch extension | Step 0 | 1 edit + tests |
| **1B** | 5 coding tasks (MVP) | Step 0 | 15 files |
| **1C** | 3 tool-use tasks (MVP) | Step 0 | 9 files |
| **1D** | 2 compression tasks (MVP) | Step 1A | 4 files |
| **2** | evaluate.py | Steps 1A-D | 2 files |
| **3** | proposer.md | Step 2 | 1 file |
| **4** | run.sh | Steps 2-3 | 1 file |
| **5** | End-to-end MVP test | Steps 1-4 | validation |
| **6** | Expand to full 40 tasks | Step 5 | ~90 files |

Start: **1A → 1B → 1C → 1D → 2 → 3 → 4 → 5**
